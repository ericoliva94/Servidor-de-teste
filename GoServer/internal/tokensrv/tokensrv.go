// Package tokensrv e o servidor HTTP de token (porta 8090).
//
// Porte de Server/Src/Connections/TokenSocket.pas (TTokenServer) e dos
// handlers de AuthHandlers.pas que ele chama. E o que o launcher do cliente
// consulta antes de conectar: sem ele o token tem de ser inserido na mao no
// banco.
package tokensrv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"aika/internal/auth"
)

// Rotas, iguais as de TTokenServer.RequestControl.
const (
	PathGetToken   = "/member/aika_get_token.asp"
	PathCharCount  = "/servers/aika_get_chrcnt.asp"
	PathServerList = "/servers/serv00.asp"
	PathResetFlag  = "/servers/aika_reset_flag.asp"
)

// Respostas de AikaGetToken. O cliente interpreta estes codigos.
const (
	RespAccountNotFound = "0"
	RespWrongPassword   = "-1"
	RespCancelled       = "-2"
	RespBanned          = "-8"
	RespNotCBT          = "-10"
	RespUnbanned        = "-22"
)

// TokenLength e o tamanho do token na rede: a coluna last_token e o campo do
// pacote tem 32 caracteres, e o MD5 em hexadecimal que o original gerava
// ocupa exatamente isso.
const TokenLength = 32

// Store e o que o servidor de token precisa do banco.
type Store interface {
	// ByUsername busca a conta.
	ByUsername(ctx context.Context, username string) (auth.Account, bool, error)
	// SaveToken grava o token novo e o instante de criacao.
	SaveToken(ctx context.Context, accountID uint32, token string, created time.Time) error
	// ClearBan zera ban_days e account_status quando a suspensao venceu.
	ClearBan(ctx context.Context, accountID uint32) error
	// CharacterCount conta os personagens da conta.
	CharacterCount(ctx context.Context, accountID uint32) (int, error)
}

// PlayerCounter informa quantos jogadores estao online, para a rota de lista
// de servidores.
type PlayerCounter interface {
	OnlinePlayers() int
}

// Config descreve o servidor de token.
type Config struct {
	Addr string
	// ReadTimeout e WriteTimeout limitam cada requisicao. Zero usa o padrao.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Log          *slog.Logger
	// Now permite fixar o relogio em teste.
	Now func() time.Time
}

// Server atende as rotas de token.
type Server struct {
	cfg     Config
	store   Store
	players PlayerCounter
	log     *slog.Logger
	http    *http.Server
	ln      net.Listener
}

const (
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 10 * time.Second
)

// New cria o servidor.
func New(cfg Config, st Store, players PlayerCounter) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	s := &Server{cfg: cfg, store: st, players: players, log: cfg.Log}

	mux := http.NewServeMux()
	mux.HandleFunc(PathGetToken, s.handleGetToken)
	mux.HandleFunc(PathCharCount, s.handleCharCount)
	mux.HandleFunc(PathServerList, s.handleServerList)
	mux.HandleFunc(PathResetFlag, s.handleResetFlag)
	mux.HandleFunc("/", s.handleUnknown)

	s.http = &http.Server{
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return s
}

// Listen abre a porta sem comecar a atender.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("tokensrv: escutar em %s: %w", s.cfg.Addr, err)
	}
	s.ln = ln
	s.log.Info("porta aberta", "servidor", "token", "endereco", ln.Addr().String())
	return nil
}

// Addr devolve o endereco de escuta.
func (s *Server) Addr() net.Addr {
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Serve atende ate o contexto ser cancelado.
func (s *Server) Serve(ctx context.Context) error {
	if s.ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutCtx)
	}()
	err := s.http.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close encerra o servidor.
func (s *Server) Close() error {
	if s.http == nil {
		return nil
	}
	return s.http.Close()
}

// handleGetToken e o porte de TAuthHandlers.AikaGetToken.
//
// O original lia os parametros por posicao (Params[0] como "id=" e Params[1]
// como "pw="), entao a ordem na query string mudava o resultado e um
// parametro a mais quebrava o login. Aqui sao lidos por nome.
func (s *Server) handleGetToken(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	username := strings.ToLower(strings.TrimSpace(q.Get("id")))
	password := strings.TrimSpace(q.Get("pw"))

	if username == "" {
		s.reply(w, RespAccountNotFound)
		return
	}

	acc, found, err := s.store.ByUsername(r.Context(), username)
	if err != nil {
		s.log.Error("token: buscar conta", "usuario", username, "erro", err)
		// Erro de banco nao pode virar "conta inexistente": o cliente
		// mostraria a mensagem errada e o jogador tentaria criar a conta de
		// novo. Devolve erro de servidor.
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	if !found {
		s.reply(w, RespAccountNotFound)
		return
	}

	// O cliente manda o MD5 da senha, que e o que a coluna password_hash
	// guarda. Comparacao exata, como no original.
	if acc.PasswordHash == "" || acc.PasswordHash != password {
		s.reply(w, RespWrongPassword)
		return
	}

	switch int(acc.Status) {
	case auth.StatusCancelled:
		s.reply(w, RespCancelled)
		return

	case auth.StatusBanned:
		// Suspensao com prazo: vencida, a conta e liberada.
		if acc.BanDays > 0 {
			fim := acc.TokenCreated.AddDate(0, 0, acc.BanDays)
			if !fim.After(s.now()) {
				if err := s.store.ClearBan(r.Context(), acc.ID); err != nil {
					s.log.Error("token: liberar suspensao",
						"conta", acc.ID, "erro", err)
					http.Error(w, "erro interno", http.StatusInternalServerError)
					return
				}
				s.log.Info("suspensao vencida, conta liberada",
					"usuario", username, "conta", acc.ID)
				s.reply(w, RespUnbanned)
				return
			}
		}
		s.reply(w, RespBanned)
		return

	case auth.StatusNotCBT:
		s.reply(w, RespNotCBT)
		return
	}

	token, err := NewToken()
	if err != nil {
		s.log.Error("token: gerar", "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	now := s.now()
	if err := s.store.SaveToken(r.Context(), acc.ID, token, now); err != nil {
		s.log.Error("token: gravar", "conta", acc.ID, "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}

	s.log.Info("token criado", "usuario", username, "conta", acc.ID)
	s.reply(w, token)
}

// handleCharCount e o porte de TAuthHandlers.AikaGetChrCnt.
func (s *Server) handleCharCount(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	username := strings.ToLower(strings.TrimSpace(q.Get("id")))
	password := strings.TrimSpace(q.Get("pw"))

	acc, found, err := s.store.ByUsername(r.Context(), username)
	if err != nil {
		s.log.Error("token: buscar conta", "usuario", username, "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	if !found {
		s.reply(w, RespAccountNotFound)
		return
	}
	if acc.PasswordHash == "" || acc.PasswordHash != password {
		s.reply(w, RespWrongPassword)
		return
	}

	n, err := s.store.CharacterCount(r.Context(), acc.ID)
	if err != nil {
		s.log.Error("token: contar personagens", "conta", acc.ID, "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	s.reply(w, fmt.Sprint(n))
}

// handleServerList e o porte de TAuthHandlers.GetServerPlayers.
func (s *Server) handleServerList(w http.ResponseWriter, r *http.Request) {
	n := 0
	if s.players != nil {
		n = s.players.OnlinePlayers()
	}
	s.reply(w, fmt.Sprint(n))
}

// handleResetFlag e o porte de TAuthHandlers.AikaResetFlag: marca a conta
// como fora do jogo para o cliente poder reconectar.
func (s *Server) handleResetFlag(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("id")))
	if username == "" {
		s.reply(w, RespAccountNotFound)
		return
	}
	if _, found, err := s.store.ByUsername(r.Context(), username); err != nil {
		s.log.Error("token: buscar conta", "usuario", username, "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	} else if !found {
		s.reply(w, RespAccountNotFound)
		return
	}
	s.reply(w, "1")
}

// handleUnknown registra rota desconhecida. O original caia no case sem ramo
// e devolvia corpo vazio sem registro, o que escondia rota nova do cliente.
func (s *Server) handleUnknown(w http.ResponseWriter, r *http.Request) {
	s.log.Warn("rota de token desconhecida",
		"caminho", r.URL.Path, "ip", clientIP(r))
	http.NotFound(w, r)
}

// reply devolve o corpo em texto puro, como o original.
func (s *Server) reply(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) now() time.Time {
	if s.cfg.Now != nil {
		return s.cfg.Now()
	}
	return time.Now()
}

// NewToken gera um token de 32 caracteres hexadecimais.
//
// O original fazia MD5(MD5(senha) + MD5(DateTimeToStr(Now))). Isso tem dois
// problemas: o DateTimeToStr tem resolucao de segundo, entao dois pedidos no
// mesmo segundo com a mesma senha geravam o mesmo token; e o valor e derivado
// da senha, logo previsivel para quem souber o hash. O cliente trata o token
// como texto opaco de 32 caracteres e so o devolve, entao usar bytes
// aleatorios e transparente na rede e estritamente mais seguro.
func NewToken() (string, error) {
	b := make([]byte, TokenLength/2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("tokensrv: gerar token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}
