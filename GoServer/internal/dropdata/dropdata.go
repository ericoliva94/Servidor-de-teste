// Package dropdata le as listas de drop de Data/Drops.
//
// Porte de TLoad.InitDropList (Load.pas:725). Na origem a funcao e o mesmo
// bloco de 60 linhas copiado treze vezes, uma por arquivo, cada copia com os
// seus quatro contadores e o seu "case" de quatro ramos — cerca de 800 linhas
// que aqui sao um laco. Cada copia tambem fazia "SetLength(..., Count + 1)"
// por linha lida, realocando o vetor a cada item.
package dropdata

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Raridades, as constantes DROP_*_ITEM de GlobalDefs.pas.
type Rarity uint8

const (
	Normal    Rarity = 1 // DROP_NORMAL_ITEM
	Superior  Rarity = 2 // DROP_SUPERIOR_ITEM
	Rare      Rarity = 3 // DROP_RARE_ITEM
	Legendary Rarity = 4 // DROP_LEGENDARY_ITEM
)

// RarityCount e quantas raridades existem.
const RarityCount = 4

var rarityNames = map[Rarity]string{
	Normal: "normal", Superior: "superior", Rare: "rara", Legendary: "lendaria",
}

// String da o nome da raridade.
func (r Rarity) String() string {
	if n, ok := rarityNames[r]; ok {
		return n
	}
	return "raridade(?)"
}

// Valid diz se a raridade e uma das quatro conhecidas.
func (r Rarity) Valid() bool { return r >= Normal && r <= Legendary }

// Categorias de monstro, as constantes MONSTERS_* de GlobalDefs.pas.
const (
	Monsters0a20       = 0
	Monsters21a40      = 1
	Monsters41a60      = 2
	Monsters61a80      = 3
	Monsters81a99      = 4
	MonstersPlanta     = 5
	MonstersCroshuAzul = 6
	MonstersCroshuVerm = 7
	MonstersButo       = 8
	MonstersPenza      = 9
	MonstersVerit      = 10
	MonstersAdicional1 = 11
	MonstersAdicional2 = 12

	// CategoryCount e quantas categorias existem.
	CategoryCount = 13
)

// arquivos liga cada categoria ao seu CSV, na ordem em que a origem os lia.
var arquivos = [CategoryCount]string{
	Monsters0a20:       "Monsters_0_20_DropList.csv",
	Monsters21a40:      "Monsters_21_40_DropList.csv",
	Monsters41a60:      "Monsters_41_60_DropList.csv",
	Monsters61a80:      "Monsters_61_80_DropList.csv",
	Monsters81a99:      "Monsters_81_99_DropList.csv",
	MonstersPlanta:     "Planta_DropList.csv",
	MonstersCroshuAzul: "CroshuAzul_DropList.csv",
	MonstersCroshuVerm: "CroshuVermelho_DropList.csv",
	MonstersButo:       "Buto_DropList.csv",
	MonstersPenza:      "Penza_DropList.csv",
	MonstersVerit:      "Verit_DropList.csv",
	MonstersAdicional1: "DropAdicional01_DropList.csv",
	MonstersAdicional2: "DropAdicional02_DropList.csv",
}

// Category e a lista de drop de uma categoria, por raridade.
type Category struct {
	// Items guarda os indices de item por raridade; o indice 0 nao e usado,
	// para a raridade poder indexar direto.
	Items [RarityCount + 1][]uint16
}

// Of devolve a lista de uma raridade.
func (c *Category) Of(r Rarity) []uint16 {
	if c == nil || !r.Valid() {
		return nil
	}
	return c.Items[r]
}

// Empty diz se a raridade nao tem item nenhum.
func (c *Category) Empty(r Rarity) bool { return len(c.Of(r)) == 0 }

// Total conta os itens de todas as raridades.
func (c *Category) Total() int {
	if c == nil {
		return 0
	}
	n := 0
	for r := Normal; r <= Legendary; r++ {
		n += len(c.Items[r])
	}
	return n
}

// Table reune as treze categorias.
type Table struct {
	Categories [CategoryCount]Category
	// Avisos descreve o que foi recusado ou chamou atencao.
	Avisos []string
	// Missing lista os arquivos ausentes. Como em Load.pas, isso e aviso e
	// nao erro: o servidor sobe sem aquela lista.
	Missing []string
}

// ErrNotFound indica arquivo ausente.
var ErrNotFound = errors.New("dropdata: arquivo nao encontrado")

// Load le as treze listas da pasta Data/Drops.
func Load(dataDir string) (*Table, error) {
	t := &Table{}
	dir := filepath.Join(dataDir, "Drops")

	for cat := range arquivos {
		nome := arquivos[cat]
		if nome == "" {
			continue
		}
		p := filepath.Join(dir, nome)
		avisos, err := t.loadFile(cat, p)
		t.Avisos = append(t.Avisos, avisos...)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				t.Missing = append(t.Missing, nome)
				continue
			}
			return t, err
		}
	}

	// Categoria com uma raridade vazia muda o resultado do sorteio: o
	// chamador precisa saber. Ver DropRoll.
	for cat := range t.Categories {
		var vazias []string
		for r := Normal; r <= Legendary; r++ {
			if t.Categories[cat].Empty(r) {
				vazias = append(vazias, r.String())
			}
		}
		if len(vazias) == RarityCount {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"categoria %d nao tem item em nenhuma raridade: mob dela nao dropa nada", cat))
			continue
		}
		if len(vazias) > 0 {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"categoria %d sem item nas raridades %s: o sorteio cai na mais proxima",
				cat, strings.Join(vazias, ", ")))
		}
	}
	return t, nil
}

// loadFile le um CSV de drop. O formato tem a ordem na coluna 0, o indice do
// item na 1 e a raridade na 2.
func (t *Table) loadFile(cat int, path string) ([]string, error) {
	var avisos []string

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("dropdata: abrir %s: %w", path, err)
	}
	defer f.Close()

	base := filepath.Base(path)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	linha := 0
	for sc.Scan() {
		linha++
		texto := strings.TrimSpace(sc.Text())
		if texto == "" {
			continue
		}
		col := strings.Split(texto, ",")
		if len(col) < 3 {
			// A origem chamava FileStrings[2] sem checar: uma linha curta
			// levantava excecao e abortava a carga, deixando os arquivos
			// seguintes sem ler.
			avisos = append(avisos, fmt.Sprintf(
				"%s:%d: %d colunas, precisa de 3", base, linha, len(col)))
			continue
		}

		item, err := strconv.Atoi(strings.TrimSpace(col[1]))
		if err != nil || item <= 0 || item > 65535 {
			avisos = append(avisos, fmt.Sprintf(
				"%s:%d: indice de item %q invalido", base, linha, col[1]))
			continue
		}
		raridade, err := strconv.Atoi(strings.TrimSpace(col[2]))
		if err != nil {
			avisos = append(avisos, fmt.Sprintf(
				"%s:%d: raridade %q invalida", base, linha, col[2]))
			continue
		}
		r := Rarity(raridade)
		if !r.Valid() {
			// A origem tinha um "case" de quatro ramos sem else: linha com
			// raridade fora de 1..4 era descartada em silencio.
			avisos = append(avisos, fmt.Sprintf(
				"%s:%d: raridade %d fora de 1..4, item %d ignorado",
				base, linha, raridade, item))
			continue
		}

		t.Categories[cat].Items[r] = append(t.Categories[cat].Items[r], uint16(item))
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return avisos, fmt.Errorf("dropdata: ler %s: %w", path, err)
	}
	return avisos, nil
}

// Total conta todos os itens carregados.
func (t *Table) Total() int {
	if t == nil {
		return 0
	}
	n := 0
	for i := range t.Categories {
		n += t.Categories[i].Total()
	}
	return n
}

// Category devolve a categoria de um indice, ou nil fora da faixa.
//
// A origem indexava "Drops[MobT^.DropIndex]" com DropIndex vindo do CSV de
// mob, sem checar a faixa do vetor.
func (t *Table) Category(cat int) *Category {
	if t == nil || cat < 0 || cat >= CategoryCount {
		return nil
	}
	return &t.Categories[cat]
}
