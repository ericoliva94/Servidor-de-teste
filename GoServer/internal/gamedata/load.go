// Package gamedata le as tabelas estaticas do jogo (os .bin de Server/Bin/Data).
//
// Porte de Server/Src/Functions/Load.pas.
package gamedata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrNotFound indica arquivo ausente. O Delphi registrava um aviso e seguia em
// frente nesse caso, entao o chamador decide se e fatal.
var ErrNotFound = errors.New("gamedata: arquivo nao encontrado")

// Table descreve o que foi lido de um arquivo.
type Table struct {
	Path      string
	RecSize   int
	Records   int
	Leftover  int // bytes finais que nao completam um registro
	Truncated int // registros no arquivo que nao couberam no destino
}

func (t Table) String() string {
	s := fmt.Sprintf("%s: %d registros de %d bytes", filepath.Base(t.Path), t.Records, t.RecSize)
	if t.Leftover > 0 {
		s += fmt.Sprintf(", %d bytes finais ignorados", t.Leftover)
	}
	if t.Truncated > 0 {
		s += fmt.Sprintf(", %d registros alem da capacidade da tabela", t.Truncated)
	}
	return s
}

// recSize devolve o tamanho empacotado de T.
func recSize[T any]() (int, error) {
	var zero T
	n := binary.Size(zero)
	if n <= 0 {
		return 0, fmt.Errorf("gamedata: %T nao tem tamanho binario fixo", zero)
	}
	return n, nil
}

// LoadSlice le o arquivo inteiro como um vetor de T.
//
// Alguns .bin do pacote trazem bytes a mais no fim que nao completam um
// registro — ItemList.bin, PI.bin e ExpList.bin tem 4 bytes de sobra, que
// pelos offsets reais dos campos sao um rodape, nao um cabecalho. Esses bytes
// sao ignorados e contados em Table.Leftover.
//
// O Delphi dimensionava o vetor por "tamanho div registro" mas mandava
// ReadBuffer ler f.Size bytes, estourando o bloco alocado justamente nesses
// arquivos; e indexava arr[0] sem checar se o arquivo tinha ao menos um
// registro. Aqui le exatamente Records*RecSize.
func LoadSlice[T any](path string) ([]T, Table, error) {
	rs, err := recSize[T]()
	if err != nil {
		return nil, Table{}, err
	}
	raw, err := readFile(path)
	if err != nil {
		return nil, Table{Path: path, RecSize: rs}, err
	}
	n := len(raw) / rs
	tab := Table{Path: path, RecSize: rs, Records: n, Leftover: len(raw) - n*rs}
	if n == 0 {
		return nil, tab, nil
	}
	out := make([]T, n)
	if err := binary.Read(bytes.NewReader(raw[:n*rs]), binary.LittleEndian, out); err != nil {
		return nil, tab, fmt.Errorf("gamedata: %s: %w", path, err)
	}
	return out, tab, nil
}

// LoadArray preenche dst com o que couber do arquivo.
//
// E o caso das tabelas que no Delphi sao array de tamanho fixo lido por
// `Read(f, Tabela)` de arquivo tipado: o arquivo pode ter mais registros que a
// tabela (ItemList.bin tem 26000 registros, TItemList cabe 20478), e o
// excedente simplesmente nao e lido. Table.Truncated informa quantos ficaram
// de fora, para isso aparecer no log em vez de passar em silencio.
func LoadArray[T any](path string, dst []T) (Table, error) {
	rs, err := recSize[T]()
	if err != nil {
		return Table{}, err
	}
	raw, err := readFile(path)
	if err != nil {
		return Table{Path: path, RecSize: rs}, err
	}
	avail := len(raw) / rs
	n := min(avail, len(dst))
	tab := Table{
		Path: path, RecSize: rs, Records: n,
		Leftover:  len(raw) - avail*rs,
		Truncated: avail - n,
	}
	if n == 0 {
		return tab, nil
	}
	if err := binary.Read(bytes.NewReader(raw[:n*rs]), binary.LittleEndian, dst[:n]); err != nil {
		return tab, fmt.Errorf("gamedata: %s: %w", path, err)
	}
	return tab, nil
}

// LoadRecord le um unico registro do inicio do arquivo.
func LoadRecord[T any](path string, dst *T) (Table, error) {
	rs, err := recSize[T]()
	if err != nil {
		return Table{}, err
	}
	raw, err := readFile(path)
	if err != nil {
		return Table{Path: path, RecSize: rs}, err
	}
	if len(raw) < rs {
		return Table{Path: path, RecSize: rs},
			fmt.Errorf("gamedata: %s: tem %d bytes, o registro precisa de %d",
				path, len(raw), rs)
	}
	if err := binary.Read(bytes.NewReader(raw[:rs]), binary.LittleEndian, dst); err != nil {
		return Table{Path: path, RecSize: rs}, fmt.Errorf("gamedata: %s: %w", path, err)
	}
	return Table{Path: path, RecSize: rs, Records: 1, Leftover: len(raw) - rs}, nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("gamedata: %s: %w", path, err)
	}
	return raw, nil
}
