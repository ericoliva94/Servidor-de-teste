// Package titledata le o Title.bin: a tabela de titulos com os quatro niveis de
// cada um (TitleList de FilesData.pas).
package titledata

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"aika/internal/netsrv"
)

// Tamanhos do arquivo, iguais aos da origem: 255 titulos de 4 niveis de 56 bytes.
const (
	MaxTitles  = 255
	Levels     = 4
	levelBytes = 56
	TableBytes = MaxTitles * Levels * levelBytes
	// FileName e o nome do arquivo dentro da pasta de dados.
	FileName = "Title.bin"
)

// Level e TitleLevel de FilesData.pas (packed record, 56 bytes).
type Level struct {
	Hide     bool
	Type     uint8
	Goal     uint16
	EF, EFV  [3]uint16
	Name     string
	Category uint8
	Index    uint8
	Color    uint32
}

// Table e a tabela de titulos. O zero value e uma tabela vazia, que responde
// "sem titulo" a qualquer consulta.
type Table struct {
	titles [MaxTitles][Levels]Level
}

// Parse monta a tabela dos primeiros TableBytes do arquivo. A origem lia um
// unico registro TitleList e ignorava o resto do arquivo; o Title.bin atual tem
// mais bytes que isso e o excedente continua ignorado.
func Parse(b []byte) (*Table, error) {
	if len(b) < TableBytes {
		return nil, fmt.Errorf("titledata: arquivo com %d bytes, queria pelo menos %d", len(b), TableBytes)
	}
	t := &Table{}
	for i := 0; i < MaxTitles; i++ {
		for l := 0; l < Levels; l++ {
			off := (i*Levels + l) * levelBytes
			r := b[off : off+levelBytes]
			lv := &t.titles[i][l]
			lv.Hide = r[0] != 0
			lv.Type = r[1]
			lv.Goal = binary.LittleEndian.Uint16(r[2:])
			for k := 0; k < 3; k++ {
				lv.EF[k] = binary.LittleEndian.Uint16(r[4+2*k:])
				lv.EFV[k] = binary.LittleEndian.Uint16(r[10+2*k:])
			}
			lv.Name = netsrv.CString(r[16:48])
			lv.Category = r[48]
			lv.Index = r[49]
			lv.Color = binary.LittleEndian.Uint32(r[52:])
		}
	}
	return t, nil
}

// Load le Title.bin da pasta de dados.
func Load(dir string) (*Table, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return &Table{}, fmt.Errorf("titledata: %w", err)
	}
	t, err := Parse(b)
	if err != nil {
		return &Table{}, err
	}
	return t, nil
}

// Level devolve o nivel (1 a 4) do titulo, ou false se nao existe.
func (t *Table) Level(index, level int) (*Level, bool) {
	if t == nil || index <= 0 || index >= MaxTitles || level < 1 || level > Levels {
		return nil, false
	}
	return &t.titles[index][level-1], true
}
