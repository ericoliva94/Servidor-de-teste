package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exemplo = `[Server]
Version=305
MAX_USERS=1000
Channels=2
[MySQL]
Database=aika_db4
Server=127.0.0.1
Port=3306
Username=root
Password=
; comentario
# outro comentario

[GAMESERVERCONF]
LEVEL_CAP=50
EXP_MULTIPLIER=2
`

func TestParseINI(t *testing.T) {
	ini, err := ParseINI(strings.NewReader(exemplo))
	if err != nil {
		t.Fatal(err)
	}
	if got := ini.String("Server", "Version", ""); got != "305" {
		t.Errorf("Version = %q", got)
	}
	if got := ini.Int("Server", "MAX_USERS", 0); got != 1000 {
		t.Errorf("MAX_USERS = %d", got)
	}
	if got := ini.String("MySQL", "Database", ""); got != "aika_db4" {
		t.Errorf("Database = %q", got)
	}
	if got := ini.Int("GAMESERVERCONF", "LEVEL_CAP", 0); got != 50 {
		t.Errorf("LEVEL_CAP = %d", got)
	}
}

// O TIniFile do Delphi nao distingue maiusculas em seccao nem em chave.
func TestParseINIIsCaseInsensitive(t *testing.T) {
	ini, _ := ParseINI(strings.NewReader(exemplo))
	for _, c := range []struct{ sec, key string }{
		{"server", "version"},
		{"SERVER", "VERSION"},
		{"Server", "VeRsIoN"},
	} {
		if got := ini.String(c.sec, c.key, ""); got != "305" {
			t.Errorf("[%s] %s = %q", c.sec, c.key, got)
		}
	}
}

// Valor vazio ou invalido devolve o padrao em vez de derrubar a carga. O
// original fazia StrToInt direto, que levanta excecao.
func TestIntFallsBackOnInvalid(t *testing.T) {
	ini, _ := ParseINI(strings.NewReader("[MySQL]\nPassword=\nPort=abc\n"))
	if got := ini.Int("MySQL", "Port", 3306); got != 3306 {
		t.Errorf("porta invalida = %d, queria o padrao 3306", got)
	}
	if got := ini.Int("MySQL", "Password", 7); got != 7 {
		t.Errorf("valor vazio = %d, queria o padrao 7", got)
	}
	if got := ini.Int("Nada", "Nenhuma", 9); got != 9 {
		t.Errorf("chave ausente = %d, queria 9", got)
	}
	if got := ini.String("Nada", "Nenhuma", "padrao"); got != "padrao" {
		t.Errorf("chave ausente = %q", got)
	}
}

// Senha vazia e legitima: o exemplo do pacote vem assim.
func TestEmptyPasswordIsKept(t *testing.T) {
	ini, _ := ParseINI(strings.NewReader(exemplo))
	if got := ini.String("MySQL", "Password", "naoexiste"); got != "" {
		t.Errorf("senha = %q, queria vazia", got)
	}
}

func TestFromINI(t *testing.T) {
	ini, _ := ParseINI(strings.NewReader(exemplo))
	cfg := FromINI(ini)
	if cfg.Version != 305 || cfg.Channels != 2 {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.MaxUsers != 1000 {
		t.Errorf("MaxUsers = %d", cfg.MaxUsers)
	}
	if cfg.MySQLDatabase != "aika_db4" || cfg.MySQLPort != 3306 {
		t.Errorf("mysql = %+v", cfg)
	}
	if cfg.LoginAddr != ":8831" {
		t.Errorf("LoginAddr = %q", cfg.LoginAddr)
	}
}

// MAX_USERS acima do vetor Players (ARRAY [1..1000]) tem de ser limitado: no
// original o valor ia direto para MAX_CONNECTIONS e todo
// "for i := 1 to MAX_CONNECTIONS do ... Players[i]" passaria do fim do vetor.
func TestFromINIClampsMaxUsers(t *testing.T) {
	ini, _ := ParseINI(strings.NewReader("[Server]\nMAX_USERS=50000\n"))
	if got := FromINI(ini).MaxUsers; got != 1000 {
		t.Errorf("MaxUsers = %d, queria 1000", got)
	}
	ini2, _ := ParseINI(strings.NewReader("[Server]\nMAX_USERS=-5\n"))
	if got := FromINI(ini2).MaxUsers; got != 0 {
		t.Errorf("MaxUsers negativo = %d, queria 0", got)
	}
}

// O arquivo real do pacote tem de ser lido sem erro.
func TestLoadRealExampleIni(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Server", "Bin", "AikaServer.ini.example")
	if _, err := os.Stat(p); err != nil {
		t.Skip("AikaServer.ini.example ausente")
	}
	cfg, err := LoadServer(p)
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.Version == 0 {
		t.Error("Version nao foi lida")
	}
	if cfg.MySQLDatabase == "" {
		t.Error("Database nao foi lido")
	}
	t.Logf("configuracao do pacote: %+v", cfg)
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := LoadServer(filepath.Join(t.TempDir(), "nao_existe.ini")); err == nil {
		t.Error("arquivo ausente deveria dar erro")
	}
}

// Arquivo torto nao pode derrubar a leitura.
func TestParseINITolerateGarbage(t *testing.T) {
	entradas := []string{
		"", "[", "]", "[sem fim", "chave sem secao=1",
		"=valor sem chave", "[a]\n\n\n[b]\n=\n", strings.Repeat("x", 10000),
	}
	for _, e := range entradas {
		if _, err := ParseINI(strings.NewReader(e)); err != nil {
			t.Errorf("entrada %q: %v", truncar(e), err)
		}
	}
}

func truncar(s string) string {
	if len(s) > 30 {
		return s[:30] + "..."
	}
	return s
}
