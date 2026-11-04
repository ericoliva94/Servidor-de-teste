// Package console atende a porta de administracao: uma linha de texto por
// comando, resposta em texto.
//
// Porte de ConsoleSocket.pas (193 linhas). A origem tinha um problema que
// manda o desenho inteiro: a porta aceitava qualquer conexao, de qualquer
// endereco, sem autenticacao nenhuma, e executava o comando recebido com
// poder de Admin. O unico requisito era que ALGUM Admin estivesse online, e o
// comando rodava como esse Admin — escolhido pela varredura do vetor de
// conexoes (ConsoleSocket.pas:159-173). Qualquer um que alcancasse a porta
// tinha o servidor.
//
// Aqui:
//
//   - o token e obrigatorio. Sem token configurado o console nao sobe, e diz
//     por que no log. Falhar fechado e o unico jeito honesto de portar isso;
//
//   - o endereco padrao e 127.0.0.1, nao 0.0.0.0;
//
//   - o comando nao roda "como um Admin sorteado": roda como o proprio
//     servidor, e os comandos que precisam de alvo exigem o nome do jogador;
//
//   - o enquadramento e por linha. A origem fazia UM recv de 256 bytes e
//     tratava o que viesse como o comando inteiro: um comando partido em dois
//     segmentos TCP era truncado e executado pela metade;
//
//   - a resposta e o resultado do comando. A origem mandava "OK" sempre,
//     mesmo para comando inexistente ou que falhou (ConsoleSocket.pas:188);
//
//   - cada conexao roda no seu goroutine, com prazo de leitura. A origem
//     atendia dentro do laco de accept, entao um cliente parado travava a
//     porta inteira; e quando o accept falhava ela fazia "Sleep(10); Continue"
//     para sempre, sem log e sem recuperacao.
package console

import (
	"bufio"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// DefaultAddr e onde o console escuta quando o .ini nao diz.
//
// A origem lia o endereco do .ini e aceitava 0.0.0.0. Sem autenticacao, isso
// e uma porta de administracao aberta para a rede.
const DefaultAddr = "127.0.0.1:5555"

// MaxLine e o tamanho maximo de uma linha de comando.
//
// A origem tinha "Buffer: array[0..255]" e recusava o que passasse de 200
// bytes — depois do recv, e sem resposta nenhuma: o painel ficava sem saber.
const MaxLine = 200

// ReadTimeout e o prazo de cada leitura.
const ReadTimeout = 30 * time.Second

// MaxConns e quantas conexoes simultaneas o console aceita.
const MaxConns = 8

// ErrNoToken diz que o console nao pode subir sem token.
var ErrNoToken = errors.New("console: sem token configurado")

// Runner executa um comando e devolve o texto da resposta.
type Runner interface {
	// RunConsole roda o comando (sem a barra) com os parametros dados.
	RunConsole(ctx context.Context, nome string, args []string) string
}

// Server e o servidor do console.
type Server struct {
	Addr  string
	Token string
	Run   Runner
	Log   *slog.Logger

	mu    sync.Mutex
	ln    net.Listener
	vagas chan struct{}
}

// Listen abre a porta. Devolve ErrNoToken quando nao ha token.
func (s *Server) Listen() error {
	if strings.TrimSpace(s.Token) == "" {
		return ErrNoToken
	}
	if s.Run == nil {
		return errors.New("console: sem executor de comando")
	}
	addr := s.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("console: escutar em %s: %w", addr, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.vagas = make(chan struct{}, MaxConns)
	s.mu.Unlock()
	s.log().Info("porta aberta", "servidor", "console", "endereco", ln.Addr().String())
	return nil
}

// Addr devolve o endereco em que o console escuta, vazio se nao escuta.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Serve atende ate o contexto encerrar. Chama Listen se preciso.
func (s *Server) Serve(ctx context.Context) error {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
		s.mu.Lock()
		ln = s.ln
		s.mu.Unlock()
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.log().Info("console encerrado")
				return nil
			}
			// A origem fazia "Sleep(10); Continue" sem olhar o erro: um
			// listener morto virava laco infinito silencioso.
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return fmt.Errorf("console: accept: %w", err)
		}
		select {
		case s.vagas <- struct{}{}:
		default:
			// Cheio: responde e fecha, em vez de deixar o cliente esperando.
			_, _ = c.Write([]byte("ERRO console ocupado\n"))
			_ = c.Close()
			continue
		}
		go func(c net.Conn) {
			defer func() {
				// Um comando que entre em panico nao pode levar o console
				// junto. Na origem a excecao de um comando subia por
				// HandleClient e por Execute e matava a thread: o painel
				// parava de responder ate o servidor reiniciar.
				if r := recover(); r != nil {
					s.log().Error("panico no console", "valor", r)
				}
				<-s.vagas
				_ = c.Close()
			}()
			s.atender(ctx, c)
		}(c)
	}
}

// Close fecha a porta.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	return err
}

// atender le linhas de uma conexao: a primeira e o token.
func (s *Server) atender(ctx context.Context, c net.Conn) {
	ip := enderecoDe(c)
	br := bufio.NewReaderSize(c, MaxLine+2)
	autenticado := false

	for {
		if ctx.Err() != nil {
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(ReadTimeout))
		linha, err := lerLinha(br)
		if err != nil {
			if errors.Is(err, errLinhaLonga) {
				s.responder(c, "ERRO linha acima de %d bytes", MaxLine)
				return
			}
			return
		}
		linha = strings.TrimSpace(linha)
		if linha == "" {
			continue
		}

		if !autenticado {
			// A primeira linha e "AUTH <token>". A origem nao tinha nada
			// disso: o primeiro recv ja era um comando com poder de Admin.
			campos := strings.SplitN(linha, " ", 2)
			if len(campos) != 2 || !strings.EqualFold(campos[0], "auth") {
				s.responder(c, "ERRO mande AUTH <token> primeiro")
				s.log().Warn("console sem autenticacao", "ip", ip)
				return
			}
			if subtle.ConstantTimeCompare(
				[]byte(strings.TrimSpace(campos[1])), []byte(s.Token)) != 1 {
				s.responder(c, "ERRO token invalido")
				s.log().Warn("console com token invalido", "ip", ip)
				return
			}
			autenticado = true
			s.responder(c, "OK autenticado")
			s.log().Info("console autenticado", "ip", ip)
			continue
		}

		if strings.EqualFold(linha, "quit") || strings.EqualFold(linha, "exit") {
			s.responder(c, "OK tchau")
			return
		}

		// A origem exigia que a linha comecasse com '/' e saia em silencio
		// quando nao comecava. Aqui a barra e opcional e a recusa e dita.
		linha = strings.TrimPrefix(linha, "/")
		campos := strings.Fields(linha)
		if len(campos) == 0 {
			continue
		}
		nome, args := campos[0], campos[1:]
		s.log().Info("comando do console", "ip", ip, "comando", nome,
			"parametros", len(args))

		resp := s.Run.RunConsole(ctx, nome, args)
		s.responder(c, "%s", resp)
	}
}

func (s *Server) responder(c net.Conn, f string, a ...any) {
	_ = c.SetWriteDeadline(time.Now().Add(ReadTimeout))
	// Uma resposta por linha: o painel le linha a linha, entao quebra de
	// linha dentro do texto viraria duas respostas.
	texto := strings.ReplaceAll(fmt.Sprintf(f, a...), "\n", " ")
	_, _ = c.Write([]byte(texto + "\n"))
}

func (s *Server) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

var errLinhaLonga = errors.New("console: linha longa")

// lerLinha le uma linha de no maximo MaxLine bytes.
func lerLinha(br *bufio.Reader) (string, error) {
	linha, err := br.ReadString('\n')
	if len(linha) > MaxLine {
		return "", errLinhaLonga
	}
	if err != nil {
		if linha != "" && errors.Is(err, io.EOF) {
			// Ultima linha sem o \n: ainda e um comando.
			return linha, nil
		}
		return "", err
	}
	return linha, nil
}

func enderecoDe(c net.Conn) string {
	if a := c.RemoteAddr(); a != nil {
		if host, _, err := net.SplitHostPort(a.String()); err == nil {
			return host
		}
		return a.String()
	}
	return "?"
}
