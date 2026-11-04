package store

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

func TestNamedTranslatesInOrder(t *testing.T) {
	q, args, err := Named(
		"UPDATE characters SET level = :level, experience = :exp WHERE id = :id",
		sql.Named("id", 42), sql.Named("level", 7), sql.Named("exp", int64(1234)))
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE characters SET level = ?, experience = ? WHERE id = ?"
	if q != want {
		t.Errorf("query:\n got %q\nwant %q", q, want)
	}
	if !reflect.DeepEqual(args, []any{7, int64(1234), 42}) {
		t.Errorf("args na ordem errada: %#v", args)
	}
}

func TestNamedAcceptsMap(t *testing.T) {
	q, args, err := Named("SELECT * FROM items WHERE owner_id = :owner AND slot_type = :st",
		map[string]any{"owner": 9, "st": 1})
	if err != nil {
		t.Fatal(err)
	}
	if q != "SELECT * FROM items WHERE owner_id = ? AND slot_type = ?" {
		t.Errorf("query = %q", q)
	}
	if !reflect.DeepEqual(args, []any{9, 1}) {
		t.Errorf("args = %#v", args)
	}
}

// O mesmo parametro usado duas vezes tem que virar dois marcadores, com o
// valor repetido.
func TestNamedRepeatedParameter(t *testing.T) {
	q, args, err := Named(
		"SELECT * FROM friends WHERE a = :id OR b = :id",
		sql.Named("id", 5))
	if err != nil {
		t.Fatal(err)
	}
	if q != "SELECT * FROM friends WHERE a = ? OR b = ?" {
		t.Errorf("query = %q", q)
	}
	if !reflect.DeepEqual(args, []any{5, 5}) {
		t.Errorf("args = %#v", args)
	}
}

// Sem :nome na query, os argumentos passam direto: o estilo posicional com ?
// continua valendo.
func TestNamedPassesPositionalThrough(t *testing.T) {
	q, args, err := Named("SELECT * FROM items WHERE owner_id = ? LIMIT ?", 3, 64)
	if err != nil {
		t.Fatal(err)
	}
	if q != "SELECT * FROM items WHERE owner_id = ? LIMIT ?" {
		t.Errorf("query = %q", q)
	}
	if !reflect.DeepEqual(args, []any{3, 64}) {
		t.Errorf("args = %#v", args)
	}
}

// O risco real do tradutor: dois-pontos dentro de string, comentario ou
// identificador nao e parametro. Trocar por "?" ali corromperia a query.
func TestNamedIgnoresQuotedAndCommented(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
		args  []any
	}{
		{
			"apostrofo",
			"SELECT * FROM t WHERE nome = 'as :10 horas' AND id = :id",
			"SELECT * FROM t WHERE nome = 'as :10 horas' AND id = ?",
			[]any{1},
		},
		{
			"apostrofo duplicado dentro da string",
			"SELECT 'ele disse ''oi :nome''' AS x, id FROM t WHERE id = :id",
			"SELECT 'ele disse ''oi :nome''' AS x, id FROM t WHERE id = ?",
			[]any{1},
		},
		{
			"escape com barra",
			`SELECT 'a\' :nome' AS x FROM t WHERE id = :id`,
			`SELECT 'a\' :nome' AS x FROM t WHERE id = ?`,
			[]any{1},
		},
		{
			"aspas duplas",
			`SELECT "texto :nome" FROM t WHERE id = :id`,
			`SELECT "texto :nome" FROM t WHERE id = ?`,
			[]any{1},
		},
		{
			"acento grave",
			"SELECT `coluna :estranha` FROM t WHERE id = :id",
			"SELECT `coluna :estranha` FROM t WHERE id = ?",
			[]any{1},
		},
		{
			"comentario de linha com --",
			"SELECT 1 -- nao e :param\nWHERE id = :id",
			"SELECT 1 -- nao e :param\nWHERE id = ?",
			[]any{1},
		},
		{
			"comentario de linha com #",
			"SELECT 1 # nem :isso\nWHERE id = :id",
			"SELECT 1 # nem :isso\nWHERE id = ?",
			[]any{1},
		},
		{
			"comentario de bloco",
			"SELECT 1 /* :oculto */ WHERE id = :id",
			"SELECT 1 /* :oculto */ WHERE id = ?",
			[]any{1},
		},
		{
			"dois-pontos duplo nao e parametro",
			"SELECT id::text FROM t WHERE id = :id",
			"SELECT id::text FROM t WHERE id = ?",
			[]any{1},
		},
		{
			"dois-pontos solto",
			"SELECT TIME('10:30') FROM t WHERE id = :id",
			"SELECT TIME('10:30') FROM t WHERE id = ?",
			[]any{1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, args, err := Named(c.query, sql.Named("id", 1))
			if err != nil {
				t.Fatal(err)
			}
			if q != c.want {
				t.Errorf("query:\n got %q\nwant %q", q, c.want)
			}
			if !reflect.DeepEqual(args, c.args) {
				t.Errorf("args = %#v, queria %#v", args, c.args)
			}
		})
	}
}

func TestNamedRejectsMissingAndUnused(t *testing.T) {
	if _, _, err := Named("SELECT * FROM t WHERE a = :a AND b = :b", sql.Named("a", 1)); err == nil {
		t.Error("parametro ausente deveria dar erro")
	}
	if _, _, err := Named("SELECT * FROM t WHERE a = :a",
		sql.Named("a", 1), sql.Named("sobrando", 2)); err == nil {
		t.Error("parametro nao usado deveria dar erro")
	}
	if _, _, err := Named("SELECT * FROM t WHERE a = :a", 1); err == nil {
		t.Error("argumento posicional com :nome na query deveria dar erro")
	}
}

// Nenhuma query, por mais torta que seja, pode derrubar o servidor.
func TestNamedNeverPanics(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	alphabet := []byte("abc :'\"`#-/*\\\n0123456789_?,()")
	for i := 0; i < 20000; i++ {
		n := r.IntN(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[r.IntN(len(alphabet))]
		}
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panico em %q: %v", string(b), p)
				}
			}()
			_, _, _ = Named(string(b), sql.Named("x", 1))
		}()
	}
}

// Literal nao fechado nao pode entrar em laco infinito nem em panico.
func TestNamedUnterminatedLiteral(t *testing.T) {
	for _, q := range []string{"SELECT 'aberto", `SELECT "aberto`, "SELECT `aberto", "SELECT /* aberto"} {
		if _, _, err := Named(q); err != nil {
			t.Errorf("%q: erro inesperado %v", q, err)
		}
	}
}

func TestDSN(t *testing.T) {
	cfg := Config{
		Server: "127.0.0.1", Port: 3306,
		User: "root", Password: "segredo", Database: "aika_db4",
	}
	cfg.withDefaults()
	got := cfg.DSN()
	want := "root:segredo@tcp(127.0.0.1:3306)/aika_db4?charset=utf8mb4&parseTime=true&loc=Local&sql_mode='NO_ENGINE_SUBSTITUTION'&timeout=10s"
	if got != want {
		t.Errorf("DSN:\n got %q\nwant %q", got, want)
	}
}

func TestIsConnectionLost(t *testing.T) {
	lost := []string{
		"invalid connection", "broken pipe", "connection refused",
		"connection reset by peer", "Error 1053: server has gone away",
		"driver: bad connection", "unexpected EOF",
	}
	for _, m := range lost {
		if !isConnectionLost(errString(m)) {
			t.Errorf("%q deveria contar como conexao perdida", m)
		}
	}
	kept := []string{
		"Error 1062: Duplicate entry 'x' for key 'PRIMARY'",
		"Error 1054: Unknown column 'foo' in 'field list'",
		"Error 1064: You have an error in your SQL syntax",
	}
	for _, m := range kept {
		if isConnectionLost(errString(m)) {
			t.Errorf("%q nao deveria ser repetido", m)
		}
	}
	if isConnectionLost(nil) {
		t.Error("nil nao e conexao perdida")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// Sem servidor no ar, Open tem que devolver erro depois das tentativas, sem
// panico e sem o laco infinito do "while not(Connected)" do Delphi.
func TestOpenFailsCleanlyWithoutServer(t *testing.T) {
	// Porta 1 nao tem servico; o connect falha de imediato.
	cfg := Config{
		Server: "127.0.0.1", Port: 1,
		User: "u", Password: "p", Database: "d",
		ConnectRetries: 2,
		ConnectTimeout: 300 * time.Millisecond,
		RetryDelay:     10 * time.Millisecond,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	start := time.Now()
	db, err := Open(context.Background(), cfg)
	if err == nil {
		db.Close()
		t.Fatal("deveria falhar sem servidor")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("demorou %v: parece laco de repeticao sem limite", took)
	}
	t.Logf("erro esperado: %v", err)
}

// O contexto cancelado tem que interromper a espera entre tentativas.
func TestOpenRespectsContext(t *testing.T) {
	cfg := Config{
		Server: "127.0.0.1", Port: 1,
		User: "u", Password: "p", Database: "d",
		ConnectRetries: 50,
		ConnectTimeout: 100 * time.Millisecond,
		RetryDelay:     5 * time.Second,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Open(ctx, cfg); err == nil {
		t.Fatal("deveria falhar")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("ignorou o cancelamento: %v", took)
	}
}

// Close em *DB nil nao pode entrar em panico.
func TestCloseNilDB(t *testing.T) {
	var db *DB
	if err := db.Close(); err != nil {
		t.Errorf("Close em nil: %v", err)
	}
}
