package titledata

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// poe grava um nivel de titulo no buffer do arquivo, no formato da origem.
func poe(b []byte, index, level int, l Level) {
	off := (index*Levels + level - 1) * levelBytes
	r := b[off : off+levelBytes]
	if l.Hide {
		r[0] = 1
	}
	r[1] = l.Type
	binary.LittleEndian.PutUint16(r[2:], l.Goal)
	for k := 0; k < 3; k++ {
		binary.LittleEndian.PutUint16(r[4+2*k:], l.EF[k])
		binary.LittleEndian.PutUint16(r[10+2*k:], l.EFV[k])
	}
	copy(r[16:48], l.Name)
	r[48], r[49] = l.Category, l.Index
	binary.LittleEndian.PutUint32(r[52:], l.Color)
}

func TestParseLeOsCamposDoRegistro(t *testing.T) {
	b := make([]byte, TableBytes)
	poe(b, 3, 2, Level{Hide: true, Type: 8, Goal: 300, EF: [3]uint16{92, 5, 0}, EFV: [3]uint16{2, 7, 0},
		Name: "Protetor", Category: 1, Index: 4, Color: 0xFFA2FB76})
	tb, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := tb.Level(3, 2)
	if !ok {
		t.Fatal("titulo 3 nivel 2 deveria existir")
	}
	if !l.Hide || l.Type != 8 || l.Goal != 300 || l.EF != [3]uint16{92, 5, 0} || l.EFV != [3]uint16{2, 7, 0} ||
		l.Name != "Protetor" || l.Category != 1 || l.Index != 4 || l.Color != 0xFFA2FB76 {
		t.Errorf("nivel lido = %+v", l)
	}
}

func TestConsultaForaDaTabelaNaoQuebra(t *testing.T) {
	tb, _ := Parse(make([]byte, TableBytes))
	for _, c := range [][2]int{{0, 1}, {1, 0}, {1, 5}, {255, 1}, {-1, 1}} {
		if _, ok := tb.Level(c[0], c[1]); ok {
			t.Errorf("Level(%d,%d) nao deveria existir", c[0], c[1])
		}
	}
	var vazia *Table
	if _, ok := vazia.Level(1, 1); ok {
		t.Error("tabela nula nao tem titulos")
	}
	if _, err := Parse(make([]byte, TableBytes-1)); err == nil {
		t.Error("arquivo curto deveria falhar")
	}
}

func TestLoadSemArquivoDevolveTabelaVazia(t *testing.T) {
	tb, err := Load(t.TempDir())
	if err == nil || tb == nil {
		t.Fatalf("tb=%v err=%v", tb, err)
	}
	if l, ok := tb.Level(1, 1); !ok || l.EF != [3]uint16{} || l.Name != "" {
		t.Errorf("tabela vazia deveria devolver nivel zerado: %+v", l)
	}
}

// O Title.bin do repositorio e o dado real: o titulo 1 e "Protetor da Natureza".
func TestTitleBinDoRepositorio(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Skip("Title.bin ausente")
	}
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := tb.Level(1, 1)
	if !ok || l.Name != "Protetor da Natureza" || l.EF[0] != 92 || l.EFV[0] != 2 || l.Goal != 300 {
		t.Errorf("titulo 1 nivel 1 = %+v", l)
	}
}
