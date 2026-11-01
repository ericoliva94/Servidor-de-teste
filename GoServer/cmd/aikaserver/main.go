// Command aikaserver sobe o servidor Aika portado para Go.
//
// Estado: a fatia vertical do login. O servidor abre a porta 8831, atende o
// pacote $81 validando o token contra a tabela accounts e responde o $82.
// As demais portas (canais, token HTTP, console) entram nas fases seguintes;
// ver GoServer/PORTE.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"aika/internal/auth"
	"aika/internal/config"
	"aika/internal/game"
	"aika/internal/gamedata"
	"aika/internal/netsrv"
	"aika/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		iniPath  = flag.String("ini", "AikaServer.ini", "caminho do AikaServer.ini")
		dataDir  = flag.String("data", dataDirDefault(), "pasta com as tabelas .bin")
		logLevel = flag.String("log", "info", "nivel de log: debug, info, warn, error")
		skipDB   = flag.Bool("no-db", false, "sobe sem banco (so para conferir a carga)")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	slog.SetDefault(log)

	cfg, err := config.LoadServer(*iniPath)
	if err != nil {
		return err
	}
	log.Info("configuracao lida",
		"ini", *iniPath, "versao", cfg.Version,
		"max_usuarios", cfg.MaxUsers, "canais", cfg.Channels)

	// As tabelas estaticas sobem antes das portas: sem elas o servidor aceita
	// jogador e depois nao sabe responder.
	tables, err := gamedata.Load(*dataDir)
	if err != nil {
		return fmt.Errorf("carregar %s: %w", *dataDir, err)
	}
	for _, t := range tables.Loaded {
		log.Info("tabela carregada", "detalhe", t.String())
	}
	if len(tables.Missing) > 0 {
		// Arquivo ausente e aviso, nao erro: o Delphi tambem seguia em frente.
		log.Warn("tabelas ausentes", "arquivos", tables.Missing)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		accounts auth.Accounts
		chars    game.Characters
	)
	if *skipDB {
		log.Warn("subindo sem banco: nenhum login sera aceito")
		accounts = noAccounts{}
		chars = noChars{}
	} else {
		db, err := store.Open(ctx, store.Config{
			Server:   cfg.MySQLServer,
			Port:     cfg.MySQLPort,
			User:     cfg.MySQLUser,
			Password: cfg.MySQLPassword,
			Database: cfg.MySQLDatabase,
			Log:      log,
		})
		if err != nil {
			return err
		}
		defer db.Close()
		log.Info("mysql conectado",
			"servidor", cfg.MySQLServer, "porta", cfg.MySQLPort, "banco", cfg.MySQLDatabase)
		accounts = store.Accounts{DB: db}
		chars = store.Characters{DB: db}
	}

	loginSrv := netsrv.New(netsrv.Config{
		Name:     "login",
		Addr:     cfg.LoginAddr,
		MaxConns: int(cfg.MaxUsers),
		Log:      log,
	}, &auth.Handler{Accounts: accounts})

	channelSrv := netsrv.New(netsrv.Config{
		Name:     "canal",
		Addr:     cfg.ChannelAddr,
		MaxConns: int(cfg.MaxUsers),
		Log:      log,
	}, &game.Handler{
		Channel:       0,
		Accounts:      accounts,
		Chars:         chars,
		Log:           log,
		ServerVersion: uint16(cfg.Version),
	})

	// Abre as portas antes de anunciar: o original so descobria falha de bind
	// pelo log, depois de dizer que havia iniciado.
	for _, srv := range []*netsrv.Server{loginSrv, channelSrv} {
		if err := srv.Listen(); err != nil {
			return err
		}
	}
	log.Info("servidor pronto",
		"login", loginSrv.Addr().String(),
		"canal", channelSrv.Addr().String())

	// Um erro em qualquer uma das portas encerra as duas.
	errc := make(chan error, 2)
	ctx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()
	for _, srv := range []*netsrv.Server{loginSrv, channelSrv} {
		go func(s *netsrv.Server) { errc <- s.Serve(ctx) }(srv)
	}

	var firstErr error
	for range 2 {
		if err := <-errc; err != nil && !errors.Is(err, context.Canceled) {
			if firstErr == nil {
				firstErr = err
			}
			cancelAll()
		}
	}
	log.Info("servidor encerrado")
	return firstErr
}

// noChars devolve lista vazia. Usado com -no-db.
type noChars struct{}

func (noChars) ByAccount(context.Context, uint32) ([store.MaxCharacterSlots]store.Character, []string, error) {
	var zero [store.MaxCharacterSlots]store.Character
	return zero, nil, nil
}

// noAccounts recusa todo login. Usado com -no-db.
type noAccounts struct{}

func (noAccounts) ByUsername(context.Context, string) (auth.Account, bool, error) {
	return auth.Account{}, false, nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: l,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(time.Now().Format("15:04:05"))
			}
			return a
		},
	}))
}

// dataDirDefault acha a pasta Data: quem roda de dentro de GoServer a
// encontra em ../Server/Bin/Data, e quem roda de Server/Bin a tem ao lado.
func dataDirDefault() string {
	for _, p := range []string{
		"Data",
		filepath.Join("..", "Server", "Bin", "Data"),
		filepath.Join("..", "..", "Server", "Bin", "Data"),
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "Data"
}
