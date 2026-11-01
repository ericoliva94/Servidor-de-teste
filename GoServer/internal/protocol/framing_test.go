package protocol

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"testing"
)

// makePacket monta um pacote cifrado valido com um corpo reconhecivel.
func makePacket(t *testing.T, size int, code uint16, fill byte) []byte {
	t.Helper()
	if size < HeaderSize {
		t.Fatalf("size %d menor que o cabecalho", size)
	}
	p := make([]byte, size)
	for i := HeaderSize; i < size; i++ {
		p[i] = fill
	}
	binary.LittleEndian.PutUint16(p[6:], code)
	if err := encryptWith(p, size, 7, 0); err != nil {
		t.Fatal(err)
	}
	return p
}

func drain(t *testing.T, f *Framer) []Packet {
	t.Helper()
	var out []Packet
	for {
		pkt, ok, err := f.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			return out
		}
		out = append(out, Packet{
			Size:       pkt.Size,
			Code:       pkt.Code,
			Data:       append([]byte(nil), pkt.Data[:pkt.Size]...),
			ChecksumOK: pkt.ChecksumOK,
		})
	}
}

// O bug central do Delphi: tres pacotes no mesmo segmento TCP rendiam uma
// resposta so. Aqui tem que render tres.
func TestThreePacketsInOneSegment(t *testing.T) {
	a := makePacket(t, 20, 0x301, 0xAA)
	b := makePacket(t, 32, 0x22C, 0xBB)
	c := makePacket(t, 16, 0x102, 0xCC)

	f := NewFramer()
	if err := f.Feed(bytes.Join([][]byte{a, b, c}, nil)); err != nil {
		t.Fatal(err)
	}

	got := drain(t, f)
	if len(got) != 3 {
		t.Fatalf("recebi %d pacotes, queria 3", len(got))
	}
	wantSizes := []int{20, 32, 16}
	wantCodes := []uint16{0x301, 0x22C, 0x102}
	for i, p := range got {
		if p.Size != wantSizes[i] || p.Code != wantCodes[i] {
			t.Errorf("pacote %d: size=%d code=%#x, queria size=%d code=%#x",
				i, p.Size, p.Code, wantSizes[i], wantCodes[i])
		}
		if !p.ChecksumOK {
			t.Errorf("pacote %d: checksum nao fechou", i)
		}
	}
}

// O outro bug: pacote partido entre dois segmentos era processado pela metade.
func TestPacketSplitAcrossSegments(t *testing.T) {
	p := makePacket(t, 64, 0x301, 0x5A)

	for cut := 1; cut < len(p); cut++ {
		f := NewFramer()
		if err := f.Feed(p[:cut]); err != nil {
			t.Fatal(err)
		}
		if got := drain(t, f); len(got) != 0 {
			t.Fatalf("cut=%d: entregou pacote incompleto", cut)
		}
		if err := f.Feed(p[cut:]); err != nil {
			t.Fatal(err)
		}
		got := drain(t, f)
		if len(got) != 1 {
			t.Fatalf("cut=%d: recebi %d pacotes, queria 1", cut, len(got))
		}
		if got[0].Size != 64 || got[0].Code != 0x301 {
			t.Errorf("cut=%d: size=%d code=%#x", cut, got[0].Size, got[0].Code)
		}
	}
}

// Byte a byte, o pior caso de fragmentacao.
func TestOneByteAtATime(t *testing.T) {
	a := makePacket(t, 24, 0x111, 0x11)
	b := makePacket(t, 28, 0x222, 0x22)
	stream := append(append([]byte(nil), a...), b...)

	f := NewFramer()
	var got []Packet
	for i := range stream {
		if err := f.Feed(stream[i : i+1]); err != nil {
			t.Fatal(err)
		}
		got = append(got, drain(t, f)...)
	}
	if len(got) != 2 {
		t.Fatalf("recebi %d pacotes, queria 2", len(got))
	}
	if got[0].Code != 0x111 || got[1].Code != 0x222 {
		t.Errorf("codigos: %#x %#x", got[0].Code, got[1].Code)
	}
}

func TestHelloAloneThenPacket(t *testing.T) {
	hello := []byte{0x11, 0xF3, 0x11, 0x1F}
	p := makePacket(t, 20, 0x301, 0x77)

	f := NewFramer()
	if err := f.Feed(hello); err != nil {
		t.Fatal(err)
	}
	if got := drain(t, f); len(got) != 0 {
		t.Fatalf("hello virou pacote")
	}
	if err := f.Feed(p); err != nil {
		t.Fatal(err)
	}
	got := drain(t, f)
	if len(got) != 1 || got[0].Code != 0x301 {
		t.Fatalf("depois do hello: %+v", got)
	}
}

func TestHelloGluedToFirstPacket(t *testing.T) {
	hello := []byte{0x11, 0xF3, 0x11, 0x1F}
	p := makePacket(t, 20, 0x301, 0x77)

	f := NewFramer()
	if err := f.Feed(append(append([]byte(nil), hello...), p...)); err != nil {
		t.Fatal(err)
	}
	got := drain(t, f)
	if len(got) != 1 || got[0].Code != 0x301 {
		t.Fatalf("hello colado: %+v", got)
	}
}

// Sem hello: o primeiro pacote tem que passar inteiro.
func TestNoHelloFirstPacketKept(t *testing.T) {
	p := makePacket(t, 40, 0x455, 0x33)
	f := NewFramer()
	if err := f.Feed(p); err != nil {
		t.Fatal(err)
	}
	got := drain(t, f)
	if len(got) != 1 {
		t.Fatalf("recebi %d, queria 1", len(got))
	}
	if got[0].Size != 40 || got[0].Code != 0x455 {
		t.Errorf("size=%d code=%#x", got[0].Size, got[0].Code)
	}
}

func TestInvalidSizeDiscardsBuffer(t *testing.T) {
	bad := make([]byte, 40)
	binary.LittleEndian.PutUint16(bad[offSize:], 3) // menor que o cabecalho

	f := NewFramer()
	f.helloChecked = true
	if err := f.Feed(bad); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.Next(); err != ErrInvalidPacketSize {
		t.Fatalf("erro=%v, queria ErrInvalidPacketSize", err)
	}
	if len(f.pending) != 0 {
		t.Errorf("buffer nao foi descartado: %d bytes", len(f.pending))
	}
}

func TestPendingOverflow(t *testing.T) {
	f := NewFramer()
	f.helloChecked = true
	big := make([]byte, MaxPacketSize)
	binary.LittleEndian.PutUint16(big[offSize:], MaxPacketSize)
	if err := f.Feed(big); err != nil {
		t.Fatal(err)
	}
	if err := f.Feed(big); err != nil {
		t.Fatal(err)
	}
	if err := f.Feed(big); err != ErrPendingOverflow {
		t.Fatalf("erro=%v, queria ErrPendingOverflow", err)
	}
}

// A cauda do buffer tem que vir zerada: e isso que permite um handler ler uma
// struct de tamanho fixo de um pacote curto sem estourar o indice.
func TestShortPacketIsZeroPadded(t *testing.T) {
	p := makePacket(t, HeaderSize, 0x999, 0)
	f := NewFramer()
	f.helloChecked = true
	if err := f.Feed(p); err != nil {
		t.Fatal(err)
	}
	pkt, ok, err := f.Next()
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(pkt.Data) != MaxPacketSize {
		t.Fatalf("Data tem %d bytes, queria %d", len(pkt.Data), MaxPacketSize)
	}
	for i := pkt.Size; i < len(pkt.Data); i++ {
		if pkt.Data[i] != 0 {
			t.Fatalf("byte %d da cauda nao esta zerado", i)
		}
	}
}

// Lixo aleatorio na conexao nao pode derrubar o servidor.
func TestFramerNeverPanics(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	for i := 0; i < 4000; i++ {
		f := NewFramer()
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("iteracao %d: panico: %v", i, p)
				}
			}()
			for seg := 0; seg < 6; seg++ {
				n := r.IntN(256)
				junk := make([]byte, n)
				for j := range junk {
					junk[j] = byte(r.UintN(256))
				}
				_ = f.Feed(junk)
				for {
					_, ok, err := f.Next()
					if err != nil || !ok {
						break
					}
				}
			}
		}()
	}
}
