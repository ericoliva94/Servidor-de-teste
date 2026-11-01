// Package config le o AikaServer.ini.
//
// Porte de TLoad.InitServerConf e TLoad.InitServers (Load.pas), que usavam
// TIniFile. O formato e o mesmo arquivo do servidor Delphi, para os dois
// poderem conviver na mesma pasta.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"aika/internal/entity"
)

// INI e um arquivo .ini lido em memoria. As seccoes e chaves nao distinguem
// maiusculas, como o TIniFile do Delphi.
type INI struct {
	sections map[string]map[string]string
}

// LoadINI le o arquivo.
func LoadINI(path string) (*INI, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	defer f.Close()
	return ParseINI(f)
}

// ParseINI le de qualquer fonte.
func ParseINI(r interface{ Read([]byte) (int, error) }) (*INI, error) {
	ini := &INI{sections: map[string]map[string]string{}}
	cur := ""
	ini.sections[cur] = map[string]string{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if ini.sections[cur] == nil {
				ini.sections[cur] = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		ini.sections[cur][strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("config: ler ini: %w", err)
	}
	return ini, nil
}

// String devolve o valor, ou def quando a chave nao existe.
func (i *INI) String(section, key, def string) string {
	if i == nil {
		return def
	}
	s := i.sections[strings.ToLower(section)]
	if s == nil {
		return def
	}
	if v, ok := s[strings.ToLower(key)]; ok {
		return v
	}
	return def
}

// Int devolve o valor como inteiro, ou def quando ausente ou invalido.
//
// O original fazia StrToInt direto, que levanta excecao com valor invalido ou
// chave vazia e derrubava a carga de configuracao.
func (i *INI) Int(section, key string, def int) int {
	raw := i.String(section, key, "")
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

// Server e a configuracao que o servidor precisa para subir.
type Server struct {
	Version  int
	MaxUsers uint16
	Channels int

	MySQLServer   string
	MySQLPort     int
	MySQLDatabase string
	MySQLUser     string
	MySQLPassword string

	LoginAddr   string
	ChannelAddr string
	TokenAddr   string
	ConsoleAddr string
}

// Portas padrao, iguais as do Delphi (ver ALTERACOES.md).
const (
	DefaultLoginPort   = 8831
	DefaultChannelPort = 8822
	DefaultTokenPort   = 8090
	DefaultConsolePort = 5555
)

// LoadServer monta a configuracao a partir do ini.
func LoadServer(path string) (Server, error) {
	ini, err := LoadINI(path)
	if err != nil {
		return Server{}, err
	}
	return FromINI(ini), nil
}

// FromINI monta a configuracao de um ini ja lido.
func FromINI(ini *INI) Server {
	// MAX_USERS e limitado ao tamanho real do vetor Players. No original o
	// valor ia direto para MAX_CONNECTIONS sem limite, e todo
	// "for i := 1 to MAX_CONNECTIONS do ... Players[i]" passaria do fim do
	// vetor com um valor acima de 1000 no arquivo.
	maxUsers := entity.ClampMaxConnections(ini.Int("Server", "MAX_USERS", 1000))

	return Server{
		Version:  ini.Int("Server", "Version", 0),
		MaxUsers: maxUsers,
		Channels: ini.Int("Server", "Channels", 1),

		MySQLServer:   ini.String("MySQL", "Server", "127.0.0.1"),
		MySQLPort:     ini.Int("MySQL", "Port", 3306),
		MySQLDatabase: ini.String("MySQL", "Database", "aika_db4"),
		MySQLUser:     ini.String("MySQL", "Username", "root"),
		MySQLPassword: ini.String("MySQL", "Password", ""),

		LoginAddr:   fmt.Sprintf(":%d", DefaultLoginPort),
		ChannelAddr: fmt.Sprintf(":%d", DefaultChannelPort),
		TokenAddr:   fmt.Sprintf(":%d", DefaultTokenPort),
		ConsoleAddr: fmt.Sprintf("127.0.0.1:%d", DefaultConsolePort),
	}
}
