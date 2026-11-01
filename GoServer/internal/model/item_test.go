package model

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// O layout tem que bater com o sizeof() do Delphi, senao os pacotes e as
// linhas do banco saem tortos. binary.Size e o tamanho empacotado, que e o
// que "packed record" significa.
func TestBinaryLayoutMatchesDelphi(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want int
	}{
		{"TPosition", Position{}, 8},
		{"TItemEffect", ItemEffect{}, 6},
		{"TItem", Item{}, ItemSize},
	}
	for _, c := range cases {
		if got := binary.Size(c.v); got != c.want {
			t.Errorf("%s: %d bytes, o Delphi tem %d", c.name, got, c.want)
		}
	}
}

// TITEM_SIZE = 20 em GlobalDefs.pas; o codigo usa essa constante em Move().
func TestItemSizeConstant(t *testing.T) {
	if binary.Size(Item{}) != ItemSize {
		t.Fatalf("ItemSize=%d nao bate com a struct", ItemSize)
	}
}

func TestItemFieldOffsets(t *testing.T) {
	it := Item{
		Index:     0x1122,
		APP:       0x3344,
		Identific: 0x55667788,
		Effects:   ItemEffect{Index: [3]byte{1, 2, 3}, Value: [3]byte{4, 5, 6}},
		Min:       0x9A,
		Max:       0xBC,
		Refi:      0xDEF0,
		Time:      0x1357,
	}
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, it); err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x22, 0x11, // Index
		0x44, 0x33, // APP
		0x88, 0x77, 0x66, 0x55, // Identific
		1, 2, 3, 4, 5, 6, // Effects
		0x9A,       // Min
		0xBC,       // Max
		0xF0, 0xDE, // Refi
		0x57, 0x13, // Time
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("layout:\n got %x\nwant %x", buf.Bytes(), want)
	}
}

// Ida e volta da expiracao: a escrita guarda 3 bytes (passo de 256 s), entao a
// leitura tem que devolver o valor truncado nesse passo, sem lixo.
func TestExpireRoundTrip(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	for _, unix := range []int64{0, 256, 1000, 1762000000, 1 << 24} {
		var it Item
		want := time.Unix(unix, 0).UTC()
		it.SetExpire(1, base, want)
		got := it.Expire(1, base)

		// O valor guardado despreza o byte baixo do unixtime.
		wantTrunc := unix & ^int64(0xFF)
		if unix >= 1<<32 {
			t.Fatalf("caso invalido")
		}
		if got.Unix() != wantTrunc {
			t.Errorf("unix=%d: voltou %d, queria %d", unix, got.Unix(), wantTrunc)
		}
	}
}

// Ler duas vezes tem que dar o mesmo valor. No Delphi nao dava: o byte baixo
// era lixo de pilha a cada chamada.
func TestExpireIsStable(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	var it Item
	it.SetExpire(1, base, time.Unix(1762000000, 0).UTC())
	first := it.Expire(1, base)
	for i := 0; i < 50; i++ {
		if got := it.Expire(1, base); !got.Equal(first) {
			t.Fatalf("leitura %d deu %v, a primeira deu %v", i, got, first)
		}
	}
}

// SetExpire preserva o byte baixo de Refi, que nao faz parte da data.
func TestSetExpireKeepsLowRefiByte(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	it := Item{Refi: 0x00A5}
	it.SetExpire(1, base, time.Unix(1762000000, 0).UTC())
	if got := it.Refi & 0x00FF; got != 0xA5 {
		t.Errorf("byte baixo de Refi virou %#x", got)
	}
}

// ItemType 9 conta em dias, nao em unixtime.
func TestExpireItemType9CountsDays(t *testing.T) {
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	var it Item
	it.SetExpire(9, base, base.AddDate(0, 0, 37))
	if it.Time != 37 {
		t.Fatalf("Time=%d, queria 37", it.Time)
	}
	got := it.Expire(9, base)
	if want := base.AddDate(0, 0, 37); !got.Equal(want) {
		t.Errorf("voltou %v, queria %v", got, want)
	}
}

func TestSealed(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	now := time.Unix(1762000000, 0).UTC()

	it := Item{Refi: 1234, Time: 5678}
	if it.IsSealed() {
		t.Error("item com Refi/Time preenchidos nao esta selado")
	}
	it.SetSealed(true, 1, base, 0, now)
	if !it.IsSealed() {
		t.Error("SetSealed(true) nao selou")
	}
	it.SetSealed(false, 1, base, 24, now)
	if it.IsSealed() {
		t.Error("SetSealed(false) deixou selado")
	}
}

// GetEquipSellPrice dividia MIN/MAX sem checar MAX: um item zerado levantava
// divisao por zero no Delphi.
func TestEquipSellPriceHandlesZeroMax(t *testing.T) {
	it := Item{Min: 0, Max: 0}
	if got := it.EquipSellPrice(1000); got != 0 {
		t.Errorf("preco=%d, queria 0", got)
	}
}

func TestEquipSellPriceProportional(t *testing.T) {
	cases := []struct {
		min, max uint8
		price    uint32
		want     uint32
	}{
		{100, 100, 1000, 1000},
		{50, 100, 1000, 500},
		{0, 100, 1000, 0},
		{33, 100, 999, 330},
	}
	for _, c := range cases {
		it := Item{Min: c.min, Max: c.max}
		if got := it.EquipSellPrice(c.price); got != c.want {
			t.Errorf("min=%d max=%d price=%d: %d, queria %d",
				c.min, c.max, c.price, got, c.want)
		}
	}
}

func TestPositionDistance(t *testing.T) {
	cases := []struct {
		a, b Position
		want uint16
	}{
		{Position{0, 0}, Position{0, 0}, 0},
		{Position{0, 0}, Position{3, 4}, 4},
		{Position{10, 10}, Position{7, 2}, 8},
		{Position{-5, 0}, Position{5, 0}, 10},
	}
	for _, c := range cases {
		if got := c.a.Distance(c.b); got != c.want {
			t.Errorf("%v->%v: %d, queria %d", c.a, c.b, got, c.want)
		}
	}
	if !(Position{0, 0}).InRange(Position{3, 3}, 3) {
		t.Error("InRange deveria aceitar distancia igual ao limite")
	}
	if (Position{0, 0}).InRange(Position{4, 0}, 3) {
		t.Error("InRange aceitou fora do limite")
	}
}
