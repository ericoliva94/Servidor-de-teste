package gamedata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MakeItems e MakeItemsIngredients vem de CSV, nao de .bin: TLoad.InitMakeItems
// le MakeItems.csv e MakeItemsIngredients.csv.
//
// Colunas de MakeItems.csv: 0 = item resultante, 1 = ID da linha, 2 = nivel
// minimo, 3 = preco, 4 = quantidade, 5 = taxa de sucesso (1000 = 100%); as
// demais colunas nao sao lidas pela origem. MakeItemsIngredients.csv:
// 0 = item resultante, 1 = ingrediente, 2 = quantidade.

// LoadMakeItems le os dois CSV de fabricacao de dir.
//
// A origem lia cada coluna com FileStrings[n].ToInteger() sem conferir o
// numero de colunas nem o valor: uma linha curta ou com lixo levantava
// excecao no meio da carga e o resto da tabela ficava sem ler. Aqui a linha
// ruim e devolvida em warnings e a carga segue. Arquivo ausente nao e erro.
func LoadMakeItems(dir string) (items []MakeItemsData, ings []MakeItemsIngredientsData, missing, warnings []string) {
	rows, miss, w := readCSV(filepath.Join(dir, "MakeItems.csv"), 6)
	if miss {
		missing = append(missing, "MakeItems.csv")
	}
	warnings = append(warnings, w...)
	for _, r := range rows {
		v, ok := parseUints(r.f[:6], []int{16, 16, 8, 32, 8, 16})
		if !ok {
			warnings = append(warnings, fmt.Sprintf("MakeItems.csv:%d: valor invalido ou fora da faixa", r.line))
			continue
		}
		items = append(items, MakeItemsData{
			ResultItemID: uint16(v[0]), ID: uint16(v[1]), LevelMin: uint8(v[2]),
			Price: uint32(v[3]), ResultAmount: uint8(v[4]), TaxSuccess: uint16(v[5]),
		})
	}

	rows, miss, w = readCSV(filepath.Join(dir, "MakeItemsIngredients.csv"), 3)
	if miss {
		missing = append(missing, "MakeItemsIngredients.csv")
	}
	warnings = append(warnings, w...)
	for _, r := range rows {
		v, ok := parseUints(r.f[:3], []int{16, 16, 16})
		if !ok {
			warnings = append(warnings, fmt.Sprintf("MakeItemsIngredients.csv:%d: valor invalido ou fora da faixa", r.line))
			continue
		}
		ings = append(ings, MakeItemsIngredientsData{ID: uint16(v[0]), ItemID: uint16(v[1]), Amount: uint16(v[2])})
	}
	return items, ings, missing, warnings
}

type csvRow struct {
	line int
	f    []string
}

// readCSV separa as linhas por virgula, tirando espacos e campos vazios, como
// ExtractStrings([','], [' '], ...). Linha com menos de minCols campos vira
// aviso e e descartada.
func readCSV(path string, minCols int) (rows []csvRow, missing bool, warnings []string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, os.IsNotExist(err), []string{fmt.Sprintf("%s: %v", filepath.Base(path), err)}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields []string
		for _, p := range strings.Split(line, ",") {
			if p = strings.TrimSpace(p); p != "" {
				fields = append(fields, p)
			}
		}
		if len(fields) < minCols {
			warnings = append(warnings, fmt.Sprintf("%s:%d: %d colunas, preciso de %d",
				filepath.Base(path), n, len(fields), minCols))
			continue
		}
		rows = append(rows, csvRow{line: n, f: fields})
	}
	if err := sc.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("%s: %v", filepath.Base(path), err))
	}
	return rows, false, warnings
}

func parseUints(fs []string, bits []int) ([]uint64, bool) {
	out := make([]uint64, len(fs))
	for i, s := range fs {
		v, err := strconv.ParseUint(s, 10, bits[i])
		if err != nil {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// RecipeByResult acha a receita pelo item resultante, a primeira que casar
// (TFunctions.SearchMakeItemIDByRewardID).
func (t *Tables) RecipeByResult(result uint16) (*MakeItemsData, bool) {
	if t == nil {
		return nil, false
	}
	for i := range t.MakeItems {
		if t.MakeItems[i].ResultItemID == result {
			return &t.MakeItems[i], true
		}
	}
	return nil, false
}

// IngredientsOf lista os ingredientes da receita do item resultante, na
// ordem do arquivo.
func (t *Tables) IngredientsOf(result uint16) []MakeItemsIngredientsData {
	if t == nil {
		return nil
	}
	var out []MakeItemsIngredientsData
	for _, g := range t.MakeIngredients {
		if g.ID == result {
			out = append(out, g)
		}
	}
	return out
}
