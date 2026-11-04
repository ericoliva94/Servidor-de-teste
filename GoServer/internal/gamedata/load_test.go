package gamedata

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataDir e a pasta Data do pacote, quando presente. Os testes que dependem
// dela sao pulados quando o repositorio nao a tem.
func dataDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(d); err != nil {
		t.Skipf("pasta Data ausente: %v", err)
	}
	return d
}

func TestMissingFileIsReported(t *testing.T) {
	_, _, err := LoadSlice[ItemChanceReinforce](filepath.Join(t.TempDir(), "nao_existe.bin"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro=%v, queria ErrNotFound", err)
	}
}

// Arquivo vazio nao pode virar indexacao de vetor vazio — era o arr[0] do
// Delphi num SetLength(arr, 0).
func TestEmptyFileYieldsNoRecords(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vazio.bin")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, tab, err := LoadSlice[ItemChanceReinforce](p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 0 || tab.Records != 0 {
		t.Errorf("len=%d records=%d, queria 0", len(got), tab.Records)
	}
}

// Arquivo truncado no meio de um registro: le os completos e reporta o resto,
// em vez de ler alem do buffer como o ReadBuffer(f.Size) do Delphi.
func TestPartialRecordIsIgnoredNotOverflowed(t *testing.T) {
	rs := binary.Size(ItemChanceReinforce{})
	p := filepath.Join(t.TempDir(), "torto.bin")
	if err := os.WriteFile(p, make([]byte, rs*3+7), 0o644); err != nil {
		t.Fatal(err)
	}
	got, tab, err := LoadSlice[ItemChanceReinforce](p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("len=%d, queria 3", len(got))
	}
	if tab.Leftover != 7 {
		t.Errorf("Leftover=%d, queria 7", tab.Leftover)
	}
}

func TestLoadArrayTruncatesAndReports(t *testing.T) {
	rs := binary.Size(ItemChanceReinforce{})
	p := filepath.Join(t.TempDir(), "grande.bin")
	if err := os.WriteFile(p, make([]byte, rs*10), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := make([]ItemChanceReinforce, 4)
	tab, err := LoadArray(p, dst)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Records != 4 || tab.Truncated != 6 {
		t.Errorf("Records=%d Truncated=%d, queria 4 e 6", tab.Records, tab.Truncated)
	}
}

// Os .bin reais do pacote: o tamanho de cada registro calculado a partir do
// Delphi tem que dividir o arquivo, e o resto tem que ser o esperado.
func TestRealFilesDivideByRecordSize(t *testing.T) {
	d := dataDir(t)
	cases := []struct {
		file         string
		recSize      int
		wantRecords  int
		wantLeftover int
	}{
		{"ItemList.bin", 464, 26000, 4},
		{"PI.bin", 376, 6000, 4},
		{"Quest.bin", 2332, 7168, 0},
		{"Recipes.bin", 56, 3000, 0},
		{"Reinforce2.bin", 128, 2880, 0},
		{"Reinforce3.bin", 128, 451, 0},
		{"ReinforceA01.bin", 40, 105, 0},
		{"ReinforceW01.bin", 40, 105, 0},
		{"ScrollPos.bin", 144, 100, 0},
		{"PranExpList.bin", 4, 150, 0},
		{"ExpList.bin", 8, 152, 4},
	}
	for _, c := range cases {
		p := filepath.Join(d, c.file)
		st, err := os.Stat(p)
		if err != nil {
			t.Logf("%s ausente, pulando", c.file)
			continue
		}
		recs := int(st.Size()) / c.recSize
		left := int(st.Size()) - recs*c.recSize
		if recs != c.wantRecords || left != c.wantLeftover {
			t.Errorf("%s: %d registros resto %d, queria %d e %d",
				c.file, recs, left, c.wantRecords, c.wantLeftover)
		}
	}
}

// Verificacao de ponta a ponta contra dado real: as tabelas de refino sao
// lidas e o tamanho do registro que o Delphi usa ($28 = 40) confere.
func TestLoadReinforceFromRealFile(t *testing.T) {
	d := dataDir(t)
	p := filepath.Join(d, "ReinforceA01.bin")
	if _, err := os.Stat(p); err != nil {
		t.Skip("ReinforceA01.bin ausente")
	}
	got, tab, err := LoadSlice[ItemChanceReinforce](p)
	if err != nil {
		t.Fatal(err)
	}
	if tab.RecSize != 40 {
		t.Errorf("RecSize=%d, o Delphi usa 40", tab.RecSize)
	}
	if tab.Records != 105 || len(got) != 105 {
		t.Fatalf("Records=%d len=%d, queria 105", tab.Records, len(got))
	}
	// A tabela de chance tem 16 niveis; alguma entrada tem que ser nao nula,
	// senao o arquivo nao foi interpretado.
	nonZero := 0
	for _, r := range got {
		for _, c := range r.Chance {
			if c != 0 {
				nonZero++
			}
		}
	}
	if nonZero == 0 {
		t.Error("nenhuma chance de refino diferente de zero: layout provavelmente errado")
	}
}

// O nome do item tem que cair no inicio do registro: e o que prova que o
// arquivo nao tem cabecalho e que o registro realmente tem 464 bytes.
func TestItemListNamesLandOnRecordBoundary(t *testing.T) {
	d := dataDir(t)
	p := filepath.Join(d, "ItemList.bin")
	if _, err := os.Stat(p); err != nil {
		t.Skip("ItemList.bin ausente")
	}
	items, tab, err := LoadSlice[ItemFromList](p)
	if err != nil {
		t.Fatal(err)
	}
	if tab.RecSize != 464 {
		t.Fatalf("RecSize=%d, queria 464", tab.RecSize)
	}
	// O item 10 e WarriorFace1 no pacote atual (byte 4640 = 10*464).
	name := cstr(items[10].Name[:])
	if name != "WarriorFace1" {
		t.Errorf("item 10 = %q, queria WarriorFace1 (deslocamento no arquivo?)", name)
	}
	if en := cstr(items[10].NameEnglish[:]); en != "WarriorFace1" {
		t.Errorf("NameEnglish do item 10 = %q", en)
	}
}

func cstr(b []byte) string {
	if i := indexZero(b); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func indexZero(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

// Carga completa contra a pasta Data real do pacote.
func TestLoadAllRealTables(t *testing.T) {
	d := dataDir(t)
	tabs, err := Load(d)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tabs.Loaded) == 0 {
		t.Fatal("nenhuma tabela carregada")
	}
	for _, tb := range tabs.Loaded {
		t.Log(tb.String())
	}
	if len(tabs.Missing) > 0 {
		t.Logf("ausentes (nao fatal, igual ao Delphi): %v", tabs.Missing)
	}
	// Acesso fora da faixa tem que devolver nil, nao entrar em panico.
	if tabs.Item(-1) != nil || tabs.Item(1<<30) != nil {
		t.Error("Item fora da faixa deveria devolver nil")
	}
	if tabs.Skill(1<<30) != nil {
		t.Error("Skill fora da faixa deveria devolver nil")
	}
	if tabs.ExpFor(1<<30) != 0 {
		t.Error("ExpFor fora da faixa deveria devolver 0")
	}
}

// Tabelas nao carregadas nao podem fazer os acessores entrarem em panico.
func TestAccessorsOnEmptyTables(t *testing.T) {
	var tabs Tables
	if tabs.Item(0) != nil || tabs.Skill(0) != nil || tabs.ExpFor(0) != 0 {
		t.Error("acessores em tabelas vazias deveriam devolver nil/0")
	}
	var nilp *Tables
	if nilp.Item(0) != nil || nilp.Skill(0) != nil || nilp.ExpFor(0) != 0 {
		t.Error("acessores em *Tables nil deveriam devolver nil/0")
	}
}
