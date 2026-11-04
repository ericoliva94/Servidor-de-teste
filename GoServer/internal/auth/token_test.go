package auth

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// memAccounts e uma tabela de contas em memoria.
type memAccounts struct {
	byName map[string]Account
	err    error
}

func (m memAccounts) ByUsername(_ context.Context, u string) (Account, bool, error) {
	if m.err != nil {
		return Account{}, false, m.err
	}
	a, ok := m.byName[u]
	return a, ok, nil
}

// loginPacket monta um $81 em claro, do tamanho da struct do protocolo.
func loginPacket(user, token string) []byte {
	var p packets.CheckTokenPacket
	netsrv.PutCString(p.Username[:], user)
	netsrv.PutCString(p.Token[:], token)
	buf, err := netsrv.Marshal(CodeCheckToken, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// asPacket transforma bytes cifrados no Packet que o Framer entregaria.
func asPacket(t *testing.T, raw []byte) protocol.Packet {
	t.Helper()
	f := protocol.NewFramer()
	if err := f.Feed(raw); err != nil {
		t.Fatal(err)
	}
	pkt, ok, err := f.Next()
	if err != nil || !ok {
		t.Fatalf("Next: ok=%v err=%v", ok, err)
	}
	return pkt
}

func testHandler(accs Accounts, now time.Time) *Handler {
	return &Handler{
		Accounts: accs,
		Now:      func() time.Time { return now },
		Uptime:   func() uint32 { return 0x11223344 },
	}
}

var agora = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func contas() memAccounts {
	return memAccounts{byName: map[string]Account{
		"teste": {
			ID: 42, Username: "teste", Token: "abc123",
			TokenCreated: agora.Add(-10 * time.Second), Status: 0, Nation: 2,
		},
		"banido": {
			ID: 43, Username: "banido", Token: "tok",
			TokenCreated: agora, Status: StatusBanned,
		},
	}}
}

func TestCheckAcceptsValidToken(t *testing.T) {
	h := testHandler(contas(), agora)
	acc, err := h.check(context.Background(), asPacket(t, loginPacket("teste", "abc123")))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if acc.ID != 42 || acc.Nation != 2 {
		t.Errorf("conta = %+v", acc)
	}
}

// O original passava usuario e token por AnsiLowerCase(Trim(...)).
func TestCheckNormalizesCase(t *testing.T) {
	h := testHandler(contas(), agora)
	for _, c := range []struct{ user, token string }{
		{"TESTE", "ABC123"},
		{"  teste  ", "  abc123  "},
		{"Teste", "Abc123"},
	} {
		if _, err := h.check(context.Background(), asPacket(t, loginPacket(c.user, c.token))); err != nil {
			t.Errorf("usuario %q token %q: %v", c.user, c.token, err)
		}
	}
}

func TestCheckRejections(t *testing.T) {
	cases := []struct {
		name  string
		user  string
		token string
		now   time.Time
		want  error
	}{
		{"conta inexistente", "ninguem", "abc123", agora, ErrNoSuchAccount},
		{"usuario vazio", "", "abc123", agora, ErrNoSuchAccount},
		{"token errado", "teste", "outro", agora, ErrBadToken},
		{"token vazio", "teste", "", agora, ErrBadToken},
		{"token expirado", "teste", "abc123", agora.Add(TokenLifetime), ErrTokenExpired},
		{"conta suspensa", "banido", "tok", agora, ErrBanned},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := testHandler(contas(), c.now)
			_, err := h.check(context.Background(), asPacket(t, loginPacket(c.user, c.token)))
			if !errors.Is(err, c.want) {
				t.Errorf("erro = %v, queria %v", err, c.want)
			}
		})
	}
}

// O limite e estrito: exatamente 300 s ja expirou.
func TestTokenLifetimeBoundary(t *testing.T) {
	pkt := asPacket(t, loginPacket("teste", "abc123"))
	// A conta foi criada 10 s antes de "agora".
	criado := agora.Add(-10 * time.Second)

	if _, err := testHandler(contas(), criado.Add(TokenLifetime-time.Nanosecond)).
		check(context.Background(), pkt); err != nil {
		t.Errorf("logo antes do limite deveria valer: %v", err)
	}
	if _, err := testHandler(contas(), criado.Add(TokenLifetime)).
		check(context.Background(), pkt); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("no limite exato deveria expirar: %v", err)
	}
}

// O original comparava "(Now - CreationTime) < 300s" e aceitava diferenca
// negativa, entao uma data adiantada no banco deixava o token valido para
// sempre.
func TestTokenCreatedInFutureIsRejected(t *testing.T) {
	accs := contas()
	a := accs.byName["teste"]
	a.TokenCreated = agora.Add(24 * time.Hour)
	accs.byName["teste"] = a

	h := testHandler(accs, agora)
	_, err := h.check(context.Background(), asPacket(t, loginPacket("teste", "abc123")))
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("erro = %v, queria ErrTokenExpired", err)
	}
}

func TestCheckPropagatesStoreError(t *testing.T) {
	boom := errors.New("banco fora")
	h := testHandler(memAccounts{err: boom}, agora)
	_, err := h.check(context.Background(), asPacket(t, loginPacket("teste", "abc123")))
	if !errors.Is(err, boom) {
		t.Errorf("erro = %v, queria o erro do banco", err)
	}
}

// Pacote menor que 76 bytes e recusado, como "if Length(Buffer) < 76".
func TestCheckRejectsShortPacket(t *testing.T) {
	raw := make([]byte, 40)
	binary.LittleEndian.PutUint16(raw[6:], CodeCheckToken)
	if err := protocol.Encrypt(raw, len(raw)); err != nil {
		t.Fatal(err)
	}
	h := testHandler(contas(), agora)
	_, err := h.check(context.Background(), asPacket(t, raw))
	if !errors.Is(err, ErrPacketTooShort) {
		t.Errorf("erro = %v, queria ErrPacketTooShort", err)
	}
}

// Ponta a ponta: sobe o servidor, conecta um cliente, manda o $81 e confere a
// resposta $82 decifrada.
func TestLoginServerEndToEnd(t *testing.T) {
	h := testHandler(contas(), agora)
	srv := netsrv.New(netsrv.Config{
		Name:        "login",
		Addr:        "127.0.0.1:0",
		IdleTimeout: 2 * time.Second,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, h)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write(loginPacket("teste", "abc123")); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	raw := make([]byte, 256)
	n, err := io.ReadAtLeast(conn, raw, binary.Size(packets.ResponseLoginPacket{}))
	if err != nil {
		t.Fatalf("lendo a resposta: %v", err)
	}

	size := int(binary.LittleEndian.Uint16(raw[:2]))
	if want := binary.Size(packets.ResponseLoginPacket{}); size != want {
		t.Fatalf("tamanho da resposta = %d, queria %d", size, want)
	}
	if _, err := protocol.Decrypt(raw[:n], size); err != nil {
		t.Fatal(err)
	}
	if code := binary.LittleEndian.Uint16(raw[6:]); code != CodeResponseLogin {
		t.Errorf("codigo = %#x, queria %#x", code, CodeResponseLogin)
	}

	var reply packets.ResponseLoginPacket
	if err := netsrv.Unmarshal(protocol.Packet{Data: raw[:n], Size: size}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Index != 42 {
		t.Errorf("Index = %d, queria 42", reply.Index)
	}
	if reply.Nation != 2 {
		t.Errorf("Nation = %d, queria 2", reply.Nation)
	}
	if reply.Time != 0x11223344 {
		t.Errorf("Time = %#x", reply.Time)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Serve: %v", err)
	}
}

// Token errado: a conexao e encerrada sem resposta.
func TestLoginServerRejectsBadToken(t *testing.T) {
	h := testHandler(contas(), agora)
	srv := netsrv.New(netsrv.Config{
		Name: "login", Addr: "127.0.0.1:0",
		IdleTimeout: 2 * time.Second,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, h)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(loginPacket("teste", "errado")); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	if _, err := conn.Read(buf); err == nil {
		t.Error("nao deveria vir resposta para token errado")
	}
}

// Lixo aleatorio na porta de login nao pode derrubar o servidor.
func TestLoginServerSurvivesGarbage(t *testing.T) {
	h := testHandler(contas(), agora)
	srv := netsrv.New(netsrv.Config{
		Name: "login", Addr: "127.0.0.1:0",
		IdleTimeout: time.Second,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, h)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	for i := 0; i < 30; i++ {
		conn, err := net.Dial("tcp", srv.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		junk := make([]byte, 1+i*7%100)
		for j := range junk {
			junk[j] = byte(i*31 + j)
		}
		conn.Write(junk)
		conn.Close()
	}

	// O servidor tem de continuar aceitando um login legitimo depois disso.
	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("servidor parou de aceitar: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(loginPacket("teste", "abc123")); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	if _, err := io.ReadAtLeast(conn, buf, 12); err != nil {
		t.Errorf("login legitimo falhou depois do lixo: %v", err)
	}

	cancel()
	<-done
}

// O limite de conexoes recusa o excedente sem derrubar nada.
func TestConnectionLimit(t *testing.T) {
	srv := netsrv.New(netsrv.Config{
		Name: "login", Addr: "127.0.0.1:0", MaxConns: 2,
		IdleTimeout: 2 * time.Second,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, netsrv.HandlerFunc(func(context.Context, *netsrv.Conn, protocol.Packet) error {
		return nil
	}))
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	var keep []net.Conn
	defer func() {
		for _, c := range keep {
			c.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", srv.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		keep = append(keep, c)
	}
	// Espera as duas serem contabilizadas.
	for i := 0; i < 100 && srv.Conns() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.Conns(); got != 2 {
		t.Fatalf("conexoes = %d, queria 2", got)
	}

	// A terceira e aceita no TCP e fechada em seguida pelo limite.
	c3, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c3.Close()
	c3.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := c3.Read(buf); err == nil {
		t.Error("a conexao excedente deveria ser fechada")
	}
}
