package game

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"aika/internal/auth"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

var agora = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

type memAccounts map[string]auth.Account

func (m memAccounts) ByUsername(_ context.Context, u string) (auth.Account, bool, error) {
	a, ok := m[u]
	return a, ok, nil
}

type memChars struct {
	chars  map[uint32][store.MaxCharacterSlots]store.Character
	avisos []string
	err    error
}

func (m memChars) ByAccount(_ context.Context, id uint32) ([store.MaxCharacterSlots]store.Character, []string, error) {
	if m.err != nil {
		var zero [store.MaxCharacterSlots]store.Character
		return zero, nil, m.err
	}
	return m.chars[id], m.avisos, nil
}

func contaValida() auth.Account {
	return auth.Account{
		ID: 7, Username: "teste", Token: "tok123",
		TokenCreated: agora.Add(-5 * time.Second), Nation: 2,
	}
}

func personagens() map[uint32][store.MaxCharacterSlots]store.Character {
	var arr [store.MaxCharacterSlots]store.Character
	arr[0] = store.Character{
		ID: 100, Slot: 0, Name: "Guerreiro", ClassInfo: 1, Level: 42,
		Exp: 123456, Gold: 999, Strength: 50, Agility: 20,
		Intelligence: 10, Constitution: 30, Luck: 5,
		Altura: 3, Tronco: 2, Perna: 1, Corpo: 4,
		NumericToken: "1234", NumericErrors: 2,
	}
	arr[2] = store.Character{ID: 102, Slot: 2, Name: "Mago", ClassInfo: 4, Level: 1}
	return map[uint32][store.MaxCharacterSlots]store.Character{7: arr}
}

func quieto() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testHandler() *Handler {
	return &Handler{
		Channel:       0,
		Accounts:      memAccounts{"teste": contaValida()},
		Chars:         memChars{chars: personagens()},
		Now:           func() time.Time { return agora },
		Log:           quieto(),
		ServerVersion: 305,
	}
}

// loginPacket monta um $685 com os campos nos offsets que CheckLogin le.
func loginPacket(user, token string, version uint16) []byte {
	buf := make([]byte, 320) // acima do minimo de 104 e do offset 227+32
	binary.LittleEndian.PutUint16(buf[offLoginVersion:], version)
	netsrv.PutCString(buf[offLoginUsername:offLoginUsername+loginFieldSize], user)
	netsrv.PutCString(buf[offLoginTokenLegacy:offLoginTokenLegacy+loginFieldSize], token)
	binary.LittleEndian.PutUint16(buf[6:], CodeCheckLogin)
	if err := protocol.Encrypt(buf, len(buf)); err != nil {
		panic(err)
	}
	return buf
}

// serve sobe o canal e devolve o endereco.
func serve(t *testing.T, h *Handler) (string, func()) {
	t.Helper()
	srv := netsrv.New(netsrv.Config{
		Name: "canal", Addr: "127.0.0.1:0",
		IdleTimeout: 2 * time.Second, Log: quieto(),
	}, h)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.Serve(ctx); close(done) }()
	return srv.Addr().String(), func() {
		cancel()
		<-done
	}
}

// readPacket le um pacote inteiro e devolve o codigo e o conteudo decifrado.
func readPacket(t *testing.T, c net.Conn) (uint16, []byte) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		t.Fatalf("lendo o tamanho: %v", err)
	}
	size := int(binary.LittleEndian.Uint16(head))
	if size < protocol.HeaderSize || size > protocol.MaxPacketSize {
		t.Fatalf("tamanho invalido: %d", size)
	}
	buf := make([]byte, size)
	copy(buf, head)
	if _, err := io.ReadFull(c, buf[2:]); err != nil {
		t.Fatalf("lendo o corpo: %v", err)
	}
	if _, err := protocol.Decrypt(buf, size); err != nil {
		t.Fatal(err)
	}
	return binary.LittleEndian.Uint16(buf[6:]), buf
}

// O caminho feliz: $685 valido devolve o $901 com os personagens.
func TestCheckLoginSendsCharacterList(t *testing.T) {
	addr, stop := serve(t, testHandler())
	defer stop()

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(loginPacket("teste", "tok123", 305)); err != nil {
		t.Fatal(err)
	}

	code, raw := readPacket(t, c)
	if code != CodeCharList {
		t.Fatalf("codigo = %#x, queria %#x", code, CodeCharList)
	}

	var pkt packets.SendToCharListPacket
	if err := netsrv.Unmarshal(
		protocol.Packet{Data: append(raw, make([]byte, protocol.MaxPacketSize-len(raw))...),
			Size: len(raw)}, &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.AcountID != 7 {
		t.Errorf("AcountID = %d, queria 7", pkt.AcountID)
	}

	c0 := pkt.CharactersData[0]
	if got := netsrv.CString(c0.Name[:]); got != "Guerreiro" {
		t.Errorf("nome do slot 0 = %q", got)
	}
	// O original manda Level-1.
	if c0.Level != 41 {
		t.Errorf("nivel do slot 0 = %d, queria 41 (42-1)", c0.Level)
	}
	if c0.Nation != 2 {
		t.Errorf("nacao = %d, queria 2", c0.Nation)
	}
	if c0.Attributes.Str != 50 || c0.Attributes.Cons != 30 {
		t.Errorf("atributos = %+v", c0.Attributes)
	}
	if c0.Size.Altura != 3 || c0.Size.Corpo != 4 {
		t.Errorf("tamanhos = %+v", c0.Size)
	}
	if c0.NumRegister != 1 {
		t.Errorf("NumRegister = %d, queria 1 (tem numerica)", c0.NumRegister)
	}
	if c0.NumericError != 2 {
		t.Errorf("NumericError = %d", c0.NumericError)
	}
	if c0.Gold != 999 || c0.Exp != 123456 {
		t.Errorf("ouro=%d exp=%d", c0.Gold, c0.Exp)
	}
}

// Slot vazio tem nivel zero, nao 65535: o original faz "Level - 1" num WORD.
func TestEmptySlotLevelDoesNotUnderflow(t *testing.T) {
	addr, stop := serve(t, testHandler())
	defer stop()

	c, _ := net.Dial("tcp", addr)
	defer c.Close()
	c.Write(loginPacket("teste", "tok123", 305))

	_, raw := readPacket(t, c)
	var pkt packets.SendToCharListPacket
	netsrv.Unmarshal(protocol.Packet{
		Data: append(raw, make([]byte, protocol.MaxPacketSize-len(raw))...),
		Size: len(raw)}, &pkt)

	// O slot 1 nao tem personagem.
	if got := pkt.CharactersData[1].Level; got != 0 {
		t.Errorf("nivel do slot vazio = %d, queria 0", got)
	}
	if got := netsrv.CString(pkt.CharactersData[1].Name[:]); got != "" {
		t.Errorf("nome do slot vazio = %q", got)
	}
	// Nivel 1 continua indo como 0.
	if got := pkt.CharactersData[2].Level; got != 0 {
		t.Errorf("nivel 1 foi como %d, queria 0", got)
	}
}

func TestCheckLoginRejections(t *testing.T) {
	cases := []struct {
		name  string
		user  string
		token string
		conta auth.Account
		want  error
	}{
		{"conta inexistente", "ninguem", "tok123", contaValida(), ErrAccountNotFound},
		{"token errado", "teste", "errado", contaValida(), ErrInvalidToken},
		{"token vazio no banco", "teste", "tok123",
			func() auth.Account { a := contaValida(); a.Token = ""; return a }(), ErrInvalidToken},
		{"token expirado", "teste", "tok123",
			func() auth.Account {
				a := contaValida()
				a.TokenCreated = agora.Add(-auth.TokenLifetime)
				return a
			}(), ErrInvalidToken},
		{"token com data no futuro", "teste", "tok123",
			func() auth.Account {
				a := contaValida()
				a.TokenCreated = agora.Add(time.Hour)
				return a
			}(), ErrInvalidToken},
		{"conta bloqueada", "teste", "tok123",
			func() auth.Account { a := contaValida(); a.Status = auth.StatusBanned; return a }(),
			ErrAccountBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler()
			h.Accounts = memAccounts{"teste": tc.conta}

			var conn netsrv.Conn
			s := NewSession(0)
			pkt := asPacket(t, loginPacket(tc.user, tc.token, 305))
			err := h.checkLogin(context.Background(), &conn, s, pkt)
			if !errors.Is(err, tc.want) {
				t.Errorf("erro = %v, queria %v", err, tc.want)
			}
			if s.Authenticated {
				t.Error("a sessao nao deveria ficar autenticada")
			}
		})
	}
}

func TestCheckLoginRejectsShortPacket(t *testing.T) {
	h := testHandler()
	raw := make([]byte, 40)
	binary.LittleEndian.PutUint16(raw[6:], CodeCheckLogin)
	if err := protocol.Encrypt(raw, len(raw)); err != nil {
		t.Fatal(err)
	}
	var conn netsrv.Conn
	err := h.checkLogin(context.Background(), &conn, NewSession(0), asPacket(t, raw))
	if !errors.Is(err, ErrShortLoginPacket) {
		t.Errorf("erro = %v, queria ErrShortLoginPacket", err)
	}
}

// O token do offset 227 e preferido quando casa com o guardado.
func TestModernTokenOffsetIsAccepted(t *testing.T) {
	h := testHandler()
	buf := make([]byte, 320)
	binary.LittleEndian.PutUint16(buf[offLoginVersion:], 305)
	netsrv.PutCString(buf[offLoginUsername:offLoginUsername+loginFieldSize], "teste")
	// Deixa o offset antigo vazio e preenche so o moderno.
	netsrv.PutCString(buf[offLoginTokenModern:offLoginTokenModern+loginFieldSize], "tok123")
	binary.LittleEndian.PutUint16(buf[6:], CodeCheckLogin)
	if err := protocol.Encrypt(buf, len(buf)); err != nil {
		t.Fatal(err)
	}

	// Conexao de teste com o outro lado drenado, para o envio do $901 ter
	// para onde ir.
	esq, dir := net.Pipe()
	defer esq.Close()
	defer dir.Close()
	go io.Copy(io.Discard, dir)

	conn := netsrv.NewConnForTest(esq)
	s := NewSession(0)
	if err := h.checkLogin(context.Background(), conn, s, asPacket(t, buf)); err != nil {
		t.Fatalf("o token moderno deveria ser aceito: %v", err)
	}
	if !s.Authenticated {
		t.Error("a sessao deveria ficar autenticada")
	}
	if s.Status != CharList {
		t.Errorf("estado = %v, queria CharList", s.Status)
	}
}

// Erro do banco ao buscar personagens nao pode virar lista vazia em silencio.
func TestCharacterStoreErrorPropagates(t *testing.T) {
	boom := errors.New("banco fora")
	h := testHandler()
	h.Chars = memChars{err: boom}

	var conn netsrv.Conn
	err := h.checkLogin(context.Background(), &conn,
		NewSession(0), asPacket(t, loginPacket("teste", "tok123", 305)))
	if !errors.Is(err, boom) {
		t.Errorf("erro = %v, queria o erro do banco", err)
	}
}

// Codigo sem handler nao derruba a conexao nem entra em panico.
func TestUnknownCodeIsIgnored(t *testing.T) {
	h := testHandler()
	raw := make([]byte, 64)
	binary.LittleEndian.PutUint16(raw[6:], 0x4242)
	if err := protocol.Encrypt(raw, len(raw)); err != nil {
		t.Fatal(err)
	}
	var conn netsrv.Conn
	if err := h.HandlePacket(context.Background(), &conn, asPacket(t, raw)); err != nil {
		t.Errorf("codigo desconhecido deveria ser ignorado: %v", err)
	}
	if conn.Closing() {
		t.Error("a conexao nao deveria ser encerrada")
	}
}

func TestStatusString(t *testing.T) {
	if got := CharList.String(); got != "CharList" {
		t.Errorf("CharList = %q", got)
	}
	if got := Status(99).String(); got != "Status(?)" {
		t.Errorf("fora da faixa = %q", got)
	}
}

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
