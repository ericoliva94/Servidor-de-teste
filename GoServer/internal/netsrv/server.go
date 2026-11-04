// Package netsrv e o servidor TCP que recebe os clientes.
//
// Porte da parte de rede de Server/Src/Connections/LoginSocket.pas e
// ServerSocket.pas. O original tinha um vetor de slots de tamanho fixo
// (Connections: ARRAY [1..200]), uma thread de accept e uma de recv varrendo
// todos os slots com select(). Aqui cada conexao tem a sua goroutine, o que
// dispensa a varredura, e o limite de conexoes e explicito.
package netsrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"aika/internal/protocol"
)

// Handler trata um pacote inteiro, ja decifrado.
//
// Devolver um erro encerra a conexao. Devolver resposta nao vazia manda a
// resposta antes de qualquer encerramento.
type Handler interface {
	HandlePacket(ctx context.Context, c *Conn, p protocol.Packet) error
}

// HandlerFunc adapta uma funcao a Handler.
type HandlerFunc func(ctx context.Context, c *Conn, p protocol.Packet) error

// HandlePacket chama f.
func (f HandlerFunc) HandlePacket(ctx context.Context, c *Conn, p protocol.Packet) error {
	return f(ctx, c, p)
}

// DisconnectHandler e implementado pelo Handler que precisa saber quando uma
// conexao termina.
//
// Existe porque o original limpava o slot do jogador no Connection.Destroy,
// chamado de dezenas de pontos espalhados, e quando um deles nao chamava o
// jogador ficava para sempre na lista de visiveis dos outros: eles viam um
// boneco parado que nao respondia. Aqui a saida e um lugar so, no fim da
// goroutine da conexao, e roda mesmo que o tratamento tenha entrado em panico.
type DisconnectHandler interface {
	OnDisconnect(c *Conn)
}

// Config descreve o servidor.
type Config struct {
	// Name aparece no log.
	Name string
	// Addr e o endereco de escuta, por exemplo ":8831".
	Addr string
	// MaxConns limita as conexoes simultaneas. Zero usa o padrao.
	MaxConns int
	// IdleTimeout fecha a conexao parada. Zero usa o padrao.
	//
	// O original nao tinha isto: conexao que caia antes do login ficava com o
	// slot preso, e com 200 slots o login parava de aceitar gente.
	IdleTimeout time.Duration
	// WriteTimeout limita cada envio.
	WriteTimeout time.Duration
	Log          *slog.Logger
}

const (
	defaultMaxConns     = 200 // o tamanho do vetor Connections do original
	defaultIdleTimeout  = 60 * time.Second
	defaultWriteTimeout = 10 * time.Second
	readChunk           = 4096
)

func (c *Config) withDefaults() {
	if c.Name == "" {
		c.Name = "servidor"
	}
	if c.MaxConns <= 0 {
		c.MaxConns = defaultMaxConns
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.Log == nil {
		c.Log = slog.Default()
	}
}

// ErrTooManyConns indica recusa por limite de conexoes.
var ErrTooManyConns = errors.New("netsrv: limite de conexoes atingido")

// Server escuta e atende conexoes.
type Server struct {
	cfg     Config
	handler Handler
	log     *slog.Logger

	ln     net.Listener
	conns  atomic.Int64
	wg     sync.WaitGroup
	closed atomic.Bool
}

// New cria o servidor.
func New(cfg Config, h Handler) *Server {
	cfg.withDefaults()
	return &Server{cfg: cfg, handler: h, log: cfg.Log}
}

// Addr devolve o endereco em que o servidor esta escutando.
func (s *Server) Addr() net.Addr {
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Conns devolve quantas conexoes estao abertas.
func (s *Server) Conns() int64 { return s.conns.Load() }

// Listen abre o socket de escuta sem comecar a aceitar.
//
// Separado de Serve para o chamador saber que a porta subiu antes de seguir —
// o original so descobria falha de bind pelo log.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("netsrv: %s: escutar em %s: %w", s.cfg.Name, s.cfg.Addr, err)
	}
	s.ln = ln
	s.log.Info("porta aberta", "servidor", s.cfg.Name, "endereco", ln.Addr().String())
	return nil
}

// Serve aceita conexoes ate o contexto ser cancelado ou Close ser chamado.
func (s *Server) Serve(ctx context.Context) error {
	if s.ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		nc, err := s.ln.Accept()
		if err != nil {
			if s.closed.Load() {
				s.wg.Wait()
				return nil
			}
			// Erro temporario de accept nao pode matar o laco: era o Exit no
			// meio do accept do original, que derrubava a thread e o servidor
			// parava de aceitar sem avisar.
			s.log.Error("accept falhou", "servidor", s.cfg.Name, "erro", err)
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return nil
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}

		if n := s.conns.Add(1); n > int64(s.cfg.MaxConns) {
			s.conns.Add(-1)
			s.log.Warn("conexao recusada pelo limite",
				"servidor", s.cfg.Name, "ip", remoteIP(nc), "limite", s.cfg.MaxConns)
			nc.Close()
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.conns.Add(-1)
			s.serveConn(ctx, nc)
		}()
	}
}

// Close para de aceitar e espera as conexoes em curso.
func (s *Server) Close() {
	if s.closed.Swap(true) {
		return
	}
	if s.ln != nil {
		s.ln.Close()
	}
}

// serveConn atende uma conexao do inicio ao fim.
func (s *Server) serveConn(ctx context.Context, nc net.Conn) {
	c := &Conn{
		nc:           nc,
		framer:       protocol.NewFramer(),
		ip:           remoteIP(nc),
		writeTimeout: s.cfg.WriteTimeout,
		log:          s.log,
	}
	defer nc.Close()

	// A saida da conexao avisa o handler antes de qualquer outra coisa do
	// defer: e o unico ponto em que o jogador sai do canal.
	if dh, ok := s.handler.(DisconnectHandler); ok {
		defer func() {
			defer func() {
				if p := recover(); p != nil {
					s.log.Error("panico ao desconectar",
						"servidor", s.cfg.Name, "ip", c.ip, "panico", p)
				}
			}()
			dh.OnDisconnect(c)
		}()
	}

	// Uma excecao no tratamento de um pacote derrubava a thread do jogador no
	// original, deixando o socket aberto e o slot preso. Aqui o panico fica
	// contido nesta conexao.
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("panico ao tratar conexao",
				"servidor", s.cfg.Name, "ip", c.ip, "panico", p,
				"pilha", string(debug.Stack()))
		}
	}()

	buf := make([]byte, readChunk)
	for {
		if err := nc.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout)); err != nil {
			return
		}
		n, err := nc.Read(buf)
		if n > 0 {
			if err := c.framer.Feed(buf[:n]); err != nil {
				s.log.Warn("fluxo descartado", "servidor", s.cfg.Name,
					"ip", c.ip, "erro", err)
				return
			}
			if !s.drain(ctx, c) {
				return
			}
		}
		if err != nil {
			// Fim normal (cliente fechou) ou inatividade: nao e erro de log.
			return
		}
		if c.closing.Load() {
			return
		}
	}
}

// drain trata todos os pacotes completos que ja chegaram. Devolve false
// quando a conexao deve ser encerrada.
func (s *Server) drain(ctx context.Context, c *Conn) bool {
	for {
		pkt, ok, err := c.framer.Next()
		if err != nil {
			s.log.Warn("pacote invalido", "servidor", s.cfg.Name,
				"ip", c.ip, "erro", err)
			return false
		}
		if !ok {
			return true
		}
		if s.handler == nil {
			continue
		}
		if err := s.handler.HandlePacket(ctx, c, pkt); err != nil {
			s.log.Info("conexao encerrada pelo handler", "servidor", s.cfg.Name,
				"ip", c.ip, "codigo", fmt.Sprintf("%#x", pkt.Code), "motivo", err)
			return false
		}
		if c.closing.Load() {
			return false
		}
	}
}

func remoteIP(nc net.Conn) string {
	if nc == nil || nc.RemoteAddr() == nil {
		return ""
	}
	if h, _, err := net.SplitHostPort(nc.RemoteAddr().String()); err == nil {
		return h
	}
	return nc.RemoteAddr().String()
}
