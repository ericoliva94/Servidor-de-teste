package store

import (
	"testing"
	"time"
)

// As colunas de atributo e nivel sao INT no banco mas WORD ou BYTE no
// pacote. Converter sem limitar estouraria com valor fora da faixa.
func TestClampToWord(t *testing.T) {
	cases := []struct {
		in   int64
		want uint16
	}{
		{0, 0}, {42, 42}, {65535, 65535},
		{65536, 65535}, {1 << 40, 65535},
		{-1, 0}, {-1 << 40, 0},
	}
	for _, c := range cases {
		if got := clampToWord(c.in); got != c.want {
			t.Errorf("clampToWord(%d) = %d, queria %d", c.in, got, c.want)
		}
	}
}

func TestClampToByte(t *testing.T) {
	cases := []struct {
		in   int64
		want uint8
	}{
		{0, 0}, {200, 200}, {255, 255}, {256, 255}, {-1, 0},
	}
	for _, c := range cases {
		if got := clampToByte(c.in); got != c.want {
			t.Errorf("clampToByte(%d) = %d, queria %d", c.in, got, c.want)
		}
	}
}

// delete_time e VARCHAR com a data em texto, como o Delphi grava. O
// StrToDateTime do original levantava excecao com texto invalido e derrubava
// o envio da lista de personagens.
func TestParseDelphiDateTime(t *testing.T) {
	validos := []string{
		"01/11/2026 15:30:00",
		"01/11/2026 15:30",
		"2026-11-01 15:30:00",
		"2026-11-01T15:30:00",
		"01/11/2026",
		"2026-11-01",
	}
	for _, s := range validos {
		got, err := parseDelphiDateTime(s)
		if err != nil {
			t.Errorf("%q: %v", s, err)
			continue
		}
		if got.Year() != 2026 || got.Month() != time.November || got.Day() != 1 {
			t.Errorf("%q virou %v", s, got)
		}
	}

	invalidos := []string{"", "ontem", "31/31/2026", "0", "   "}
	for _, s := range invalidos {
		if _, err := parseDelphiDateTime(s); err == nil {
			t.Errorf("%q deveria ser recusado", s)
		}
	}
}

// Data no formato brasileiro nao pode ser lida como mes/dia.
func TestParseDelphiDateTimeIsDayFirst(t *testing.T) {
	got, err := parseDelphiDateTime("25/12/2026 00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 25 || got.Month() != time.December {
		t.Errorf("25/12/2026 virou %v: dia e mes trocados", got)
	}
}
