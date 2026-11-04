package console

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// executor guarda o que foi pedido e devolve uma resposta fixa.
type executor struct {
	mu       sync.Mutex
	chamadas []string
	resposta string
}

func (e *executor) RunConsole(_ context.Context, nome string, args []string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.chamadas = append(e.chamadas, strings.TrimSpace(nome+" "+strings.Join(args, " ")))
	if e.resposta != "" {
		return e.resposta
	}
	return "feito " + nome
}

func (e *executor) vistas() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.chamadas))
	copy(out, e.chamadas)
	return out
}

func quieto() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// consoleDeTeste sobe o console num porta livre.
func consoleDeTeste(t *testing.T, token string) (*Server, *executor) {
	t.Helper()
	ex := &executor{}
	s := &Server{Addr: "127.0.0.1:0", Token: token, Run: ex, Log: quieto()}
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Serve(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return s, ex
}

// cliente abre uma conexao e devolve um leitor de linhas.
func cliente(t *testing.T, s *Server) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.DialTimeout("tcp", s.Address(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	return c, bufio.NewReader(c)
}

func manda(t *testing.T, c net.Conn, linha string) {
	t.Helper()
	if _, err := fmt.Fprintf(c, "%s\n", linha); err != nil {
		t.Fatal(err)
	}
}

func leLinha(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	linha, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("ler resposta: %v (parcial %q)", err, linha)
	}
	return strings.TrimSpace(linha)
}

// Sem token o console nao sobe. A origem abria a porta sempre.
func TestSemTokenNaoSobe(t *testing.T) {
	s := &Server{Addr: "127.0.0.1:0", Run: &executor{}, Log: quieto()}
	if err := s.Listen(); err != ErrNoToken {
		t.Fatalf("erro = %v, queria ErrNoToken", err)
	}
	if s.Address() != "" {
		t.Errorf("abriu em %s", s.Address())
	}
	// Token so com espacos tambem nao vale.
	s.Token = "   "
	if err := s.Listen(); err != ErrNoToken {
		t.Errorf("erro = %v", err)
	}
}

func TestSemExecutorNaoSobe(t *testing.T) {
	s := &Server{Addr: "127.0.0.1:0", Token: "x", Log: quieto()}
	if err := s.Listen(); err == nil {
		t.Error("deveria recusar sem executor")
		s.Close()
	}
}

// O comando so roda depois do AUTH.
func TestComandoAntesDoAuthERecusado(t *testing.T) {
	s, ex := consoleDeTeste(t, "segredo")
	c, br := cliente(t, s)

	manda(t, c, "/additem 100")
	resp := leLinha(t, br)
	if !strings.HasPrefix(resp, "ERRO") {
		t.Errorf("resposta = %q", resp)
	}
	if n := len(ex.vistas()); n != 0 {
		t.Errorf("%d comandos rodaram sem autenticacao", n)
	}
}

func TestTokenErradoERecusado(t *testing.T) {
	s, ex := consoleDeTeste(t, "segredo")
	for _, tok := range []string{"errado", "", "SEGREDO", "segred", "segredoo"} {
		c, br := cliente(t, s)
		manda(t, c, "AUTH "+tok)
		resp := leLinha(t, br)
		if !strings.HasPrefix(resp, "ERRO") {
			t.Errorf("token %q: resposta = %q", tok, resp)
		}
		c.Close()
	}
	if n := len(ex.vistas()); n != 0 {
		t.Errorf("%d comandos rodaram", n)
	}
}

func TestAuthEComando(t *testing.T) {
	s, ex := consoleDeTeste(t, "segredo")
	c, br := cliente(t, s)

	manda(t, c, "AUTH segredo")
	if resp := leLinha(t, br); !strings.HasPrefix(resp, "OK") {
		t.Fatalf("auth: %q", resp)
	}
	manda(t, c, "/addgold Alfa 1000")
	if resp := leLinha(t, br); resp != "feito addgold" {
		t.Errorf("resposta = %q", resp)
	}
	vistas := ex.vistas()
	if len(vistas) != 1 || vistas[0] != "addgold Alfa 1000" {
		t.Errorf("chamadas = %q", vistas)
	}
}

// A barra e opcional: a origem saia em silencio quando a linha nao comecava
// com '/'.
func TestBarraEhOpcional(t *testing.T) {
	s, ex := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	manda(t, c, "who")
	leLinha(t, br)
	manda(t, c, "/who")
	leLinha(t, br)
	if vistas := ex.vistas(); len(vistas) != 2 ||
		vistas[0] != "who" || vistas[1] != "who" {
		t.Errorf("chamadas = %q", vistas)
	}
}

// Varios comandos na mesma conexao. A origem fazia um recv e fechava.
func TestVariosComandosNaMesmaConexao(t *testing.T) {
	s, ex := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	for i := 0; i < 5; i++ {
		manda(t, c, fmt.Sprintf("/dmg Alfa %d", i))
		if resp := leLinha(t, br); resp != "feito dmg" {
			t.Fatalf("%d: %q", i, resp)
		}
	}
	if n := len(ex.vistas()); n != 5 {
		t.Errorf("%d comandos", n)
	}
}

// A origem respondia "OK" sempre, mesmo para comando que falhou.
func TestRespostaEhOResultadoDoComando(t *testing.T) {
	s, ex := consoleDeTeste(t, "s")
	ex.resposta = "Comando invalido."
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	manda(t, c, "/naoexiste")
	if resp := leLinha(t, br); resp != "Comando invalido." {
		t.Errorf("resposta = %q, queria o resultado do comando", resp)
	}
}

// Linha acima do limite e recusada com resposta, nao em silencio.
func TestLinhaLongaERecusadaComResposta(t *testing.T) {
	s, _ := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	manda(t, c, "/additem "+strings.Repeat("9", MaxLine+50))
	resp := leLinha(t, br)
	if !strings.HasPrefix(resp, "ERRO") {
		t.Errorf("resposta = %q", resp)
	}
}

// Comando partido em dois pedacos: a origem truncava no primeiro recv.
func TestComandoPartidoEmDoisPedacos(t *testing.T) {
	s, ex := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	if _, err := c.Write([]byte("/addgold Al")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := c.Write([]byte("fa 500\n")); err != nil {
		t.Fatal(err)
	}
	leLinha(t, br)
	vistas := ex.vistas()
	if len(vistas) != 1 || vistas[0] != "addgold Alfa 500" {
		t.Errorf("chamadas = %q", vistas)
	}
}

// Ultima linha sem o \n ainda vale como comando.
func TestLinhaSemFimDeLinha(t *testing.T) {
	s, ex := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)

	if _, err := c.Write([]byte("/who")); err != nil {
		t.Fatal(err)
	}
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	// A resposta chega antes do fechamento.
	if resp := leLinha(t, br); resp != "feito who" {
		t.Errorf("resposta = %q", resp)
	}
	if vistas := ex.vistas(); len(vistas) != 1 {
		t.Errorf("chamadas = %q", vistas)
	}
}

func TestQuitFecha(t *testing.T) {
	s, _ := consoleDeTeste(t, "s")
	c, br := cliente(t, s)
	manda(t, c, "AUTH s")
	leLinha(t, br)
	manda(t, c, "quit")
	if resp := leLinha(t, br); !strings.HasPrefix(resp, "OK") {
		t.Errorf("resposta = %q", resp)
	}
	// A conexao fecha.
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := br.ReadString('\n'); err == nil {
		t.Error("a conexao deveria ter fechado")
	}
}

// Um comando que entra em panico nao derruba o console. Na origem a excecao
// subia por HandleClient e por Execute e matava a thread.
type executorQuePanica struct{ vezes int }

func (e *executorQuePanica) RunConsole(context.Context, string, []string) string {
	e.vezes++
	panic("boom")
}

func TestPanicoNoComandoNaoDerrubaOConsole(t *testing.T) {
	ex := &executorQuePanica{}
	s := &Server{Addr: "127.0.0.1:0", Token: "s", Run: ex, Log: quieto()}
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Serve(ctx)

	// Primeira conexao: panica.
	c1, br1 := cliente(t, s)
	manda(t, c1, "AUTH s")
	leLinha(t, br1)
	manda(t, c1, "/boom")
	_ = c1.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = br1.ReadString('\n')
	c1.Close()

	// Segunda conexao: o console continua de pe.
	c2, br2 := cliente(t, s)
	manda(t, c2, "AUTH s")
	if resp := leLinha(t, br2); !strings.HasPrefix(resp, "OK") {
		t.Errorf("o console nao sobreviveu ao panico: %q", resp)
	}
}

// Conexoes alem do limite recebem resposta e fecham.
func TestLimiteDeConexoes(t *testing.T) {
	s, _ := consoleDeTeste(t, "s")
	var abertas []net.Conn
	for i := 0; i < MaxConns; i++ {
		c, br := cliente(t, s)
		manda(t, c, "AUTH s")
		leLinha(t, br)
		abertas = append(abertas, c)
	}
	c, br := cliente(t, s)
	resp := leLinha(t, br)
	if !strings.Contains(resp, "ocupado") {
		t.Errorf("resposta = %q", resp)
	}
	c.Close()
	for _, c := range abertas {
		c.Close()
	}
}

func TestFecharDuasVezes(t *testing.T) {
	s := &Server{Addr: "127.0.0.1:0", Token: "s", Run: &executor{}, Log: quieto()}
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Error(err)
	}
	if err := s.Close(); err != nil {
		t.Error(err)
	}
}

// Serve chama Listen quando a porta ainda nao esta aberta.
func TestServeAbreSozinho(t *testing.T) {
	s := &Server{Addr: "127.0.0.1:0", Token: "s", Run: &executor{}, Log: quieto()}
	ctx, cancel := context.WithCancel(context.Background())
	erro := make(chan error, 1)
	go func() { erro <- s.Serve(ctx) }()
	// Espera a porta abrir.
	for i := 0; i < 50 && s.Address() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Address() == "" {
		cancel()
		t.Fatal("a porta nao abriu")
	}
	cancel()
	if err := <-erro; err != nil {
		t.Errorf("Serve = %v", err)
	}
}

func TestServeSemTokenDevolveErro(t *testing.T) {
	s := &Server{Addr: "127.0.0.1:0", Run: &executor{}, Log: quieto()}
	if err := s.Serve(context.Background()); err != ErrNoToken {
		t.Errorf("= %v", err)
	}
}
