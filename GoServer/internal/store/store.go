// Package store e o acesso ao MySQL.
//
// Porte de Server/Src/Connections/SQL.pas (TQuery). O original abria uma
// conexao FireDAC por objeto TQuery e reimplementava a mao o pool, a
// reconexao e o retry; aqui o pool e a reconexao vem do database/sql e o que
// sobra e o que e especifico do servidor: o sql_mode da sessao, a traducao
// dos parametros nomeados e a regra de quando repetir uma query.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

// Config descreve a conexao. Equivale aos MYSQL_* de GlobalDefs.pas.
type Config struct {
	Server   string
	Port     int
	User     string
	Password string
	Database string

	// MaxOpenConns limita as conexoes simultaneas. 0 usa o padrao abaixo.
	MaxOpenConns int
	// ConnectTimeout limita cada tentativa de conexao inicial.
	ConnectTimeout time.Duration
	// ConnectRetries e quantas vezes tentar conectar antes de desistir.
	//
	// O Delphi tentava para sempre num while not(Connected), o que deixava o
	// servidor pendurado em silencio quando o banco nao subia. Aqui ha um
	// limite e o erro volta para quem chamou.
	ConnectRetries int
	// RetryDelay e a pausa entre tentativas.
	RetryDelay time.Duration

	Log *slog.Logger
}

const (
	defaultMaxOpenConns   = 16
	defaultConnectTimeout = 10 * time.Second
	defaultConnectRetries = 5
	// Mesma pausa do SQL_CONNECT_RETRY_DELAY do Delphi.
	defaultRetryDelay = 2 * time.Second
	// Mesmo limite do SQL_MAX_TRIES do Delphi.
	maxQueryTries = 3
)

func (c *Config) withDefaults() {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = defaultMaxOpenConns
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = defaultConnectTimeout
	}
	if c.ConnectRetries == 0 {
		c.ConnectRetries = defaultConnectRetries
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = defaultRetryDelay
	}
	if c.Log == nil {
		c.Log = slog.Default()
	}
}

// DSN monta a string de conexao do go-sql-driver/mysql.
//
// O sql_mode vai na propria DSN, nao num comando apos conectar: o pool abre
// conexoes novas a qualquer momento, e cada uma precisa do mesmo sql_mode. O
// Delphi rodava o SET uma vez por objeto TQuery, o que nao cobre conexao
// reaberta.
//
// NO_ENGINE_SUBSTITUTION e o modo que o servidor precisa porque o codigo
// legado monta valores como texto (” em coluna numerica, datas '1899-12-30',
// posicoes com virgula decimal); no modo estrito do MySQL 8 esses INSERT e
// UPDATE falham e o dado do jogador nao e salvo.
func (c Config) DSN() string {
	host := net.JoinHostPort(c.Server, fmt.Sprint(c.Port))
	params := strings.Join([]string{
		"charset=utf8mb4",
		"parseTime=true",
		"loc=Local",
		"sql_mode='NO_ENGINE_SUBSTITUTION'",
		"timeout=" + c.ConnectTimeout.String(),
	}, "&")
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s",
		c.User, c.Password, host, c.Database, params)
}

// DB e a conexao com o banco, segura para uso concorrente.
type DB struct {
	sql *sql.DB
	cfg Config
	log *slog.Logger
}

// Open conecta e confere que o banco responde.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	return openWith(ctx, cfg, "mysql", cfg.DSN())
}

// openWith existe para os testes poderem injetar um driver falso.
func openWith(ctx context.Context, cfg Config, driver, dsn string) (*DB, error) {
	cfg.withDefaults()
	h, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: abrir %s: %w", driver, err)
	}
	h.SetMaxOpenConns(cfg.MaxOpenConns)
	h.SetMaxIdleConns(cfg.MaxOpenConns)
	h.SetConnMaxLifetime(time.Hour)

	db := &DB{sql: h, cfg: cfg, log: cfg.Log}

	var lastErr error
	for try := 1; try <= cfg.ConnectRetries; try++ {
		pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
		lastErr = h.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			return db, nil
		}
		cfg.Log.Error("mysql: conexao falhou",
			"tentativa", try, "de", cfg.ConnectRetries, "erro", lastErr)
		if try == cfg.ConnectRetries {
			break
		}
		select {
		case <-ctx.Done():
			h.Close()
			return nil, ctx.Err()
		case <-time.After(cfg.RetryDelay):
		}
	}
	h.Close()
	return nil, fmt.Errorf("store: sem conexao com o mysql apos %d tentativas: %w",
		cfg.ConnectRetries, lastErr)
}

// Close encerra o pool.
func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

// SQL expoe o *sql.DB para quem precisa de transacao ou recurso proprio.
func (db *DB) SQL() *sql.DB { return db.sql }

// Query roda um SELECT, repetindo quando a conexao caiu.
func (db *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q, a, err := Named(query, args...)
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	err = db.retry(ctx, q, func() error {
		r, err := db.sql.QueryContext(ctx, q, a...)
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	return rows, err
}

// Exec roda INSERT/UPDATE/DELETE, repetindo quando a conexao caiu.
func (db *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	q, a, err := Named(query, args...)
	if err != nil {
		return nil, err
	}
	var res sql.Result
	err = db.retry(ctx, q, func() error {
		r, err := db.sql.ExecContext(ctx, q, a...)
		if err != nil {
			return err
		}
		res = r
		return nil
	})
	return res, err
}

// retry implementa a regra do TQuery.Run: repete so quando a conexao caiu.
//
// Erro do proprio comando (sintaxe, chave duplicada, coluna inexistente) nao
// melhora com repeticao, entao volta na primeira tentativa. Toda falha e
// registrada: no Delphi os erros de escrita eram engolidos sem log e o dado
// desaparecia sem aviso.
func (db *DB) retry(ctx context.Context, query string, run func() error) error {
	var lastErr error
	for try := 1; try <= maxQueryTries; try++ {
		lastErr = run()
		if lastErr == nil {
			return nil
		}
		db.log.Error("mysql: query falhou",
			"tentativa", try, "erro", lastErr, "sql", truncate(query, 300))

		if !isConnectionLost(lastErr) {
			return lastErr
		}
		if try == maxQueryTries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(db.cfg.RetryDelay):
		}
	}
	return lastErr
}

// isConnectionLost diz se vale repetir: equivale ao ekServerGone do FireDAC.
func isConnectionLost(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	// O driver devolve estes como texto; sao as quedas de conexao do MySQL.
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"invalid connection",
		"broken pipe",
		"connection refused",
		"connection reset",
		"server has gone away",
		"bad connection",
		"eof",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
