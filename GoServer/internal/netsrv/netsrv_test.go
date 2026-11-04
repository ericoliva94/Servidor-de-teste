package netsrv

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"aika/internal/protocol"
)

// pacoteTeste tem o cabecalho de 12 bytes do protocolo seguido de corpo.
type pacoteTeste struct {
	Header [protocol.HeaderSize]byte
	Valor  uint32
	Nome   [16]byte
}

func quieto() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCString(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{'a', 'b', 'c', 0, 'x', 'y'}, "abc"},
		{[]byte{0, 'a'}, ""},
		{[]byte{'c', 'h', 'e', 'i', 'o'}, "cheio"}, // campo sem terminador
		{nil, ""},
	}
	for _, c := range cases {
		if got := CString(c.in); got != c.want {
			t.Errorf("CString(%v) = %q, queria %q", c.in, got, c.want)
		}
	}
}

func TestPutCString(t *testing.T) {
	dst := make([]byte, 8)
	for i := range dst {
		dst[i] = 0xFF
	}
	PutCString(dst, "oi")
	if got := CString(dst); got != "oi" {
		t.Errorf("leu %q", got)
	}
	// O resto tem de ficar zerado, nao com o lixo anterior.
	for i := 3; i < len(dst); i++ {
		if dst[i] != 0 {
			t.Errorf("byte %d = %#x, queria 0", i, dst[i])
		}
	}
	// Texto maior que o campo e truncado sem estourar.
	PutCString(dst, "texto bem mais longo que o campo")
	if len(dst) != 8 {
		t.Fatal("o campo mudou de tamanho")
	}
}

func TestMarshalSetsSizeAndCode(t *testing.T) {
	var p pacoteTeste
	p.Valor = 0xCAFE
	PutCString(p.Nome[:], "aika")

	buf, err := Marshal(0x301, p)
	if err != nil {
		t.Fatal(err)
	}
	want := binary.Size(p)
	if len(buf) != want {
		t.Fatalf("buffer = %d bytes, queria %d", len(buf), want)
	}
	if got := int(binary.LittleEndian.Uint16(buf[:2])); got != want {
		t.Errorf("tamanho no cabecalho = %d, queria %d", got, want)
	}
	// O codigo fica cifrado: Encrypt altera tudo a partir do offset 4. So
	// depois de decifrar e que da para le-lo — e por isso que o Framer le o
	// codigo depois do Decrypt, nao antes.
	if got := binary.LittleEndian.Uint16(buf[6:]); got == 0x301 {
		t.Error("o codigo deveria estar cifrado no buffer enviado")
	}
	if _, err := protocol.Decrypt(buf, len(buf)); err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint16(buf[6:]); got != 0x301 {
		t.Errorf("codigo depois de decifrar = %#x, queria 0x301", got)
	}
	var volta pacoteTeste
	if err := Unmarshal(protocol.Packet{Data: buf, Size: len(buf)}, &volta); err != nil {
		t.Fatal(err)
	}
	if volta.Valor != 0xCAFE {
		t.Errorf("Valor = %#x", volta.Valor)
	}
	if got := CString(volta.Nome[:]); got != "aika" {
		t.Errorf("Nome = %q", got)
	}
}

func TestMarshalRejectsTooSmall(t *testing.T) {
	type curto struct{ A uint16 }
	if _, err := Marshal(1, curto{}); err == nil {
		t.Error("struct menor que o cabecalho deveria dar erro")
	}
}

// A cauda zerada do Framer permite ler uma struct maior que o pacote recebido
// sem estourar o buffer.
func TestUnmarshalFromZeroPaddedPacket(t *testing.T) {
	data := make([]byte, protocol.MaxPacketSize)
	binary.LittleEndian.PutUint32(data[12:], 7)
	var p pacoteTeste
	if err := Unmarshal(protocol.Packet{Data: data, Size: 16}, &p); err != nil {
		t.Fatal(err)
	}
	if p.Valor != 7 {
		t.Errorf("Valor = %d", p.Valor)
	}
}

func TestUnmarshalRejectsOversizedStruct(t *testing.T) {
	type enorme struct{ A [64]byte }
	if err := Unmarshal(protocol.Packet{Data: make([]byte, 16), Size: 16}, &enorme{}); err == nil {
		t.Error("struct maior que o espaco do pacote deveria dar erro")
	}
}

// Dois pacotes no mesmo segmento TCP tem de render dois handlers chamados: e
// o bug do servidor original, onde so o primeiro era tratado.
func TestServerHandlesTwoPacketsInOneWrite(t *testing.T) {
	var chamadas atomic.Int32
	srv := New(Config{
		Name: "teste", Addr: "127.0.0.1:0",
		IdleTimeout: 2 * time.Second, Log: quieto(),
	}, HandlerFunc(func(_ context.Context, _ *Conn, p protocol.Packet) error {
		chamadas.Add(1)
		return nil
	}))
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	c, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	a, _ := Marshal(0x301, pacoteTeste{})
	b, _ := Marshal(0x302, pacoteTeste{})
	if _, err := c.Write(bytes.Join([][]byte{a, b}, nil)); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200 && chamadas.Load() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if got := chamadas.Load(); got != 2 {
		t.Errorf("handler chamado %d vezes, queria 2", got)
	}
}

// Panico no handler fica contido na conexao: o servidor continua aceitando.
func TestServerContainsHandlerPanic(t *testing.T) {
	var ok atomic.Bool
	srv := New(Config{
		Name: "teste", Addr: "127.0.0.1:0",
		IdleTimeout: 2 * time.Second, Log: quieto(),
	}, HandlerFunc(func(_ context.Context, _ *Conn, p protocol.Packet) error {
		if p.Code == 0x666 {
			panic("explodiu de proposito")
		}
		ok.Store(true)
		return nil
	}))
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	// Conexao que faz o handler entrar em panico.
	c1, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	bomba, _ := Marshal(0x666, pacoteTeste{})
	c1.Write(bomba)
	c1.Close()

	// O servidor tem de continuar atendendo.
	c2, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("servidor caiu com o panico: %v", err)
	}
	defer c2.Close()
	normal, _ := Marshal(0x301, pacoteTeste{})
	c2.Write(normal)

	for i := 0; i < 200 && !ok.Load(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !ok.Load() {
		t.Error("o servidor parou de atender depois do panico")
	}
}

// Conexao parada e fechada pelo tempo de inatividade. No original a conexao
// que caia antes do login ficava com o slot preso e os 200 slots acabavam.
func TestIdleConnectionIsClosed(t *testing.T) {
	srv := New(Config{
		Name: "teste", Addr: "127.0.0.1:0",
		IdleTimeout: 150 * time.Millisecond, Log: quieto(),
	}, HandlerFunc(func(context.Context, *Conn, protocol.Packet) error { return nil }))
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	c, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Error("a conexao parada deveria ser fechada pelo servidor")
	}
	for i := 0; i < 200 && srv.Conns() != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.Conns(); got != 0 {
		t.Errorf("conexoes abertas = %d, queria 0: o slot ficou preso", got)
	}
}

// Close libera a porta e Serve termina sem erro.
func TestServerCloseIsGraceful(t *testing.T) {
	srv := New(Config{Name: "teste", Addr: "127.0.0.1:0", Log: quieto()}, nil)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	addr := srv.Addr().String()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	time.Sleep(50 * time.Millisecond)
	srv.Close()
	srv.Close() // idempotente

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve nao terminou")
	}
	if c, err := net.Dial("tcp", addr); err == nil {
		c.Close()
		t.Error("a porta deveria estar fechada")
	}
}

// Fluxo aleatorio de muitos clientes ao mesmo tempo nao pode derrubar nada.
func TestServerSurvivesConcurrentGarbage(t *testing.T) {
	srv := New(Config{
		Name: "teste", Addr: "127.0.0.1:0", MaxConns: 32,
		IdleTimeout: 300 * time.Millisecond, Log: quieto(),
	}, HandlerFunc(func(context.Context, *Conn, protocol.Packet) error { return nil }))
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	addr := srv.Addr().String()
	r := rand.New(rand.NewPCG(51, 53))
	for i := 0; i < 60; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			continue // pode bater no limite, o que e esperado
		}
		n := r.IntN(300)
		junk := make([]byte, n)
		for j := range junk {
			junk[j] = byte(r.UintN(256))
		}
		c.Write(junk)
		if r.IntN(2) == 0 {
			c.Close()
		} else {
			defer c.Close()
		}
	}

	// Depois de tudo isso o servidor continua de pe.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("servidor caiu: %v", err)
	}
	c.Close()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve nao terminou")
	}
}
