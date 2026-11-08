package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMakeItemsDeArquivosReais(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(filepath.Join(dir, "MakeItems.csv")); err != nil {
		t.Skip("dados reais ausentes")
	}
	items, ings, missing, warnings := LoadMakeItems(dir)
	if len(missing) != 0 || len(warnings) != 0 {
		t.Fatalf("missing=%v warnings=%v", missing, warnings)
	}
	if len(items) != 425 || len(ings) != 2300 {
		t.Errorf("%d receitas e %d ingredientes, queria 425 e 2300", len(items), len(ings))
	}
	tab := &Tables{MakeItems: items, MakeIngredients: ings}
	r, ok := tab.RecipeByResult(1000)
	if !ok || r.ID != 6670 || r.Price != 100 || r.TaxSuccess != 1000 {
		t.Errorf("receita 1000 = %+v ok=%v", r, ok)
	}
	if got := tab.IngredientsOf(1000); len(got) == 0 || got[0].ItemID != 5100 || got[0].Amount != 5 {
		t.Errorf("ingredientes de 1000 = %+v", got)
	}
}

func TestLoadMakeItemsLinhaRuimNaoAbortaACarga(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "MakeItems.csv"),
		[]byte("1000, 6670, 0, 100, 1, 1000, 250, 0,\r\ncurta, 1\r\n1001, x, 0, 1, 1, 1000\r\n\r\n1002, 6672, 12, 676, 1, 1000,\r\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "MakeItemsIngredients.csv"),
		[]byte("1000, 5100, 5,\r\n1000, 70000, 1,\r\n1002, 4630, 13\r\n"), 0o644)
	items, ings, missing, warnings := LoadMakeItems(dir)
	if len(items) != 2 || items[1].ResultItemID != 1002 {
		t.Errorf("itens = %+v", items)
	}
	if len(ings) != 2 {
		t.Errorf("ingredientes = %+v", ings)
	}
	if len(missing) != 0 || len(warnings) != 3 {
		t.Errorf("missing=%v warnings=%v (queria 3 avisos)", missing, warnings)
	}
}

func TestLoadMakeItemsArquivoAusenteNaoEErro(t *testing.T) {
	items, ings, missing, _ := LoadMakeItems(t.TempDir())
	if len(items) != 0 || len(ings) != 0 || len(missing) != 2 {
		t.Errorf("items=%v ings=%v missing=%v", items, ings, missing)
	}
}
