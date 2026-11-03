package dropdata

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// escreveDrops cria uma pasta Drops com os arquivos dados.
func escreveDrops(t *testing.T, arquivos map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	drops := filepath.Join(dir, "Drops")
	if err := os.MkdirAll(drops, 0o700); err != nil {
		t.Fatal(err)
	}
	for nome, conteudo := range arquivos {
		if err := os.WriteFile(filepath.Join(drops, nome), []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// linhas monta um CSV de drop a partir de pares item/raridade.
func linhas(pares ...[2]int) string {
	var b strings.Builder
	for i, p := range pares {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(",")
		b.WriteString(strconv.Itoa(p[0]))
		b.WriteString(",")
		b.WriteString(strconv.Itoa(p[1]))
		b.WriteString("\n")
	}
	return b.String()
}

// randFixo devolve sempre o mesmo indice.
type randFixo struct{ v int }

func (r randFixo) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	if r.v >= n {
		return n - 1
	}
	return r.v
}

func TestCarregaUmArquivo(t *testing.T) {
	dir := escreveDrops(t, map[string]string{
		"Monsters_0_20_DropList.csv": linhas(
			[2]int{100, int(Normal)},
			[2]int{200, int(Superior)},
			[2]int{300, int(Rare)},
			[2]int{400, int(Legendary)},
			[2]int{101, int(Normal)},
		),
	})
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cat := tb.Category(Monsters0a20)
	if cat == nil {
		t.Fatal("categoria deveria existir")
	}
	if got := cat.Of(Normal); len(got) != 2 || got[0] != 100 || got[1] != 101 {
		t.Errorf("normais = %v", got)
	}
	if got := cat.Of(Superior); len(got) != 1 || got[0] != 200 {
		t.Errorf("superiores = %v", got)
	}
	if got := cat.Of(Legendary); len(got) != 1 || got[0] != 400 {
		t.Errorf("lendarias = %v", got)
	}
	if cat.Total() != 5 {
		t.Errorf("total = %d, queria 5", cat.Total())
	}
	// Os doze arquivos ausentes entram em Missing, nao em erro.
	if len(tb.Missing) != CategoryCount-1 {
		t.Errorf("%d ausentes, queria %d", len(tb.Missing), CategoryCount-1)
	}
}

// Linha curta nao derruba a carga: a origem chamava FileStrings[2] sem checar
// e a excecao abortava o resto dos arquivos.
func TestLinhaCurtaNaoDerruba(t *testing.T) {
	dir := escreveDrops(t, map[string]string{
		"Monsters_0_20_DropList.csv": "1,100,1\n2,200\n\n3,300,1\nlixo\n",
	})
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cat := tb.Category(Monsters0a20)
	if len(cat.Of(Normal)) != 2 {
		t.Errorf("normais = %v", cat.Of(Normal))
	}
	achou := 0
	for _, a := range tb.Avisos {
		if strings.Contains(a, "colunas") {
			achou++
		}
	}
	if achou != 2 {
		t.Errorf("%d avisos de coluna, queria 2: %v", achou, tb.Avisos)
	}
}

// Raridade fora de 1..4 era descartada em silencio pelo case sem else.
func TestRaridadeForaDaFaixaEReportada(t *testing.T) {
	dir := escreveDrops(t, map[string]string{
		"Monsters_0_20_DropList.csv": linhas(
			[2]int{100, 1}, [2]int{200, 0}, [2]int{300, 5}, [2]int{400, 99},
		),
	})
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := tb.Category(Monsters0a20).Total(); n != 1 {
		t.Errorf("%d itens, queria 1", n)
	}
	achou := 0
	for _, a := range tb.Avisos {
		if strings.Contains(a, "fora de 1..4") {
			achou++
		}
	}
	if achou != 3 {
		t.Errorf("%d avisos de raridade, queria 3: %v", achou, tb.Avisos)
	}
}

func TestItemInvalidoEReportado(t *testing.T) {
	dir := escreveDrops(t, map[string]string{
		"Monsters_0_20_DropList.csv": "1,abc,1\n2,0,1\n3,-5,1\n4,99999,1\n5,100,1\n",
	})
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := tb.Category(Monsters0a20).Total(); n != 1 {
		t.Errorf("%d itens, queria 1", n)
	}
	if len(tb.Avisos) < 4 {
		t.Errorf("avisos = %v", tb.Avisos)
	}
}

// Categoria fora da faixa devolve nil: a origem indexava Drops[DropIndex] com
// o indice vindo do CSV de mob, sem checar.
func TestCategoriaForaDaFaixa(t *testing.T) {
	tb, err := Load(escreveDrops(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []int{-1, CategoryCount, 1000} {
		if tb.Category(c) != nil {
			t.Errorf("categoria %d deveria ser nil", c)
		}
	}
	var nulo *Table
	if nulo.Category(0) != nil || nulo.Total() != 0 {
		t.Error("tabela nil deveria responder vazio")
	}
	var nc *Category
	if nc.Of(Normal) != nil || !nc.Empty(Normal) || nc.Total() != 0 {
		t.Error("categoria nil deveria responder vazio")
	}
}

// ---- sorteio

func TestRarityForRoll(t *testing.T) {
	casos := map[int]Rarity{
		1: Legendary,
		2: Rare, 13: Rare,
		14: Superior, 33: Superior,
		34: Normal, 100: Normal, 255: Normal,
	}
	for roll, quer := range casos {
		if got := RarityForRoll(roll); got != quer {
			t.Errorf("RarityForRoll(%d) = %v, queria %v", roll, got, quer)
		}
	}
}

func TestRollPegaDaRaridadeSorteada(t *testing.T) {
	var c Category
	c.Items[Normal] = []uint16{10, 11}
	c.Items[Superior] = []uint16{20}
	c.Items[Rare] = []uint16{30}
	c.Items[Legendary] = []uint16{40}

	casos := map[int]uint16{1: 40, 5: 30, 20: 20, 50: 10}
	for roll, quer := range casos {
		item, _, ok := c.Roll(roll, 0, randFixo{v: 0})
		if !ok || item != quer {
			t.Errorf("roll %d deu %d (ok=%v), queria %d", roll, item, ok, quer)
		}
	}
}

// O bug central: na origem a raridade vazia caia na MAIS RARA
// ("ItemTypeFrom + 1"). Com as categorias que so tem lendaria, dois tercos
// das mortes entregavam um item lendario.
func TestRaridadeVaziaDesceEmVezDeSubir(t *testing.T) {
	var c Category
	// So tem normal e lendaria.
	c.Items[Normal] = []uint16{10}
	c.Items[Legendary] = []uint16{40}

	// Roll de superior (14..33): a origem subiria para rara e dali para
	// lendaria; aqui desce para normal.
	item, r, ok := c.Roll(20, 0, randFixo{v: 0})
	if !ok {
		t.Fatal("deveria ter sorteado algo")
	}
	if item != 10 || r != Normal {
		t.Errorf("sorteou %d (%v), queria o item normal 10", item, r)
	}
	// Roll de rara tambem desce.
	item, r, _ = c.Roll(5, 0, randFixo{v: 0})
	if item != 10 || r != Normal {
		t.Errorf("sorteou %d (%v), queria o item normal 10", item, r)
	}
	// Roll de lendaria pega a lendaria mesmo.
	item, r, _ = c.Roll(1, 0, randFixo{v: 0})
	if item != 40 || r != Legendary {
		t.Errorf("sorteou %d (%v), queria o lendario 40", item, r)
	}
}

// Categoria que so tem lendaria: o sorteio normal sobe, porque nao ha nada
// abaixo, mas isso e o ultimo recurso e nao o caminho comum.
func TestSoLendariaAindaDropa(t *testing.T) {
	var c Category
	c.Items[Legendary] = []uint16{40, 41}

	item, r, ok := c.Roll(50, 0, randFixo{v: 1})
	if !ok || r != Legendary || item != 41 {
		t.Errorf("sorteou %d (%v, ok=%v)", item, r, ok)
	}
}

// Categoria vazia nao dropa nada, e nao gira para sempre: na origem o ramo de
// chefe fazia "ItemTypeFrom := 1; goto ReCase", voltando a primeira raridade,
// e com as quatro vazias o laco nao terminava.
func TestCategoriaVaziaNaoGiraParaSempre(t *testing.T) {
	var c Category
	pronto := make(chan struct{})
	go func() {
		for _, roll := range []int{1, 5, 20, 50, 100} {
			if _, _, ok := c.Roll(roll, 0, randFixo{v: 0}); ok {
				t.Errorf("roll %d nao deveria dropar nada", roll)
			}
		}
		close(pronto)
	}()
	select {
	case <-pronto:
	case <-timeoutRoll():
		t.Fatal("o sorteio travou numa categoria vazia")
	}
}

// O bonus de drop empurra o sorteio para as raridades maiores.
func TestBonusDeDropEmpurraParaORaro(t *testing.T) {
	var c Category
	c.Items[Normal] = []uint16{10}
	c.Items[Rare] = []uint16{30}

	// Sem bonus, roll 40 cai em normal.
	if item, _, _ := c.Roll(40, 0, randFixo{v: 0}); item != 10 {
		t.Errorf("sem bonus = %d, queria 10", item)
	}
	// Com bonus de 30, o roll vira 10 e cai em rara.
	if item, r, _ := c.Roll(40, 30, randFixo{v: 0}); item != 30 || r != Rare {
		t.Errorf("com bonus = %d (%v), queria o raro 30", item, r)
	}
}

// Bonus maior que o sorteio nao deixa o numero negativo: a origem corrigia o
// zero para 2, e DecInt tem piso em zero.
func TestBonusMaiorQueOSorteio(t *testing.T) {
	var c Category
	c.Items[Rare] = []uint16{30}
	item, r, ok := c.Roll(5, 1000, randFixo{v: 0})
	if !ok || r != Rare || item != 30 {
		t.Errorf("= %d (%v, ok=%v), queria o raro", item, r, ok)
	}
}

func TestRollSemRand(t *testing.T) {
	var c Category
	c.Items[Normal] = []uint16{10}
	if _, _, ok := c.Roll(50, 0, nil); ok {
		t.Error("sem gerador nao deveria sortear")
	}
}

func TestRarityString(t *testing.T) {
	if Normal.String() != "normal" || Legendary.String() != "lendaria" {
		t.Error("nomes de raridade errados")
	}
	if Rarity(99).String() != "raridade(?)" {
		t.Error("raridade desconhecida deveria ter nome generico")
	}
	for _, r := range []Rarity{0, 5, 99} {
		if r.Valid() {
			t.Errorf("raridade %d nao deveria valer", r)
		}
	}
}

// ---- dados reais

// Confere o carregador contra os treze CSV do servidor.
func TestDadosReais(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(filepath.Join(dir, "Drops")); err != nil {
		t.Skip("pasta de drops nao esta neste checkout")
	}
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("itens=%d ausentes=%d avisos=%d", tb.Total(), len(tb.Missing), len(tb.Avisos))
	for _, a := range tb.Avisos {
		t.Logf("  aviso: %s", a)
	}

	if len(tb.Missing) != 0 {
		t.Errorf("arquivos ausentes: %v", tb.Missing)
	}
	if tb.Total() == 0 {
		t.Fatal("nao carregou nada")
	}

	// As categorias de Planta, Croshu Azul e Croshu Vermelho tem item APENAS
	// na raridade lendaria. E por causa disso que a direcao do encadeamento
	// importa: com o "+1" da origem, dois tercos das mortes desses mobs
	// entregavam um lendario.
	soLendaria := []int{MonstersPlanta, MonstersCroshuAzul, MonstersCroshuVerm}
	for _, cat := range soLendaria {
		c := tb.Category(cat)
		if c.Empty(Legendary) {
			t.Errorf("categoria %d deveria ter lendarias", cat)
		}
		for _, r := range []Rarity{Normal, Superior, Rare} {
			if !c.Empty(r) {
				t.Errorf("categoria %d tem item em %v: o teste precisa ser revisto",
					cat, r)
			}
		}
		// Com o porte, o roll normal nessas categorias sobe para lendaria
		// porque nao ha nada abaixo — e o unico caso em que isso acontece.
		item, r, ok := c.Roll(50, 0, randFixo{v: 0})
		if !ok || r != Legendary || item == 0 {
			t.Errorf("categoria %d: roll normal deu %d (%v)", cat, item, r)
		}
	}

	// Monsters_61_80 nao tem lendaria: o roll de lendaria desce para rara.
	c := tb.Category(Monsters61a80)
	if !c.Empty(Legendary) {
		t.Error("Monsters_61_80 deveria estar sem lendarias")
	}
	if _, r, ok := c.Roll(1, 0, randFixo{v: 0}); !ok || r != Rare {
		t.Errorf("roll de lendaria deu %v, queria descer para rara", r)
	}

	// Nenhum item com indice zero entrou.
	for cat := range tb.Categories {
		for r := Normal; r <= Legendary; r++ {
			for _, item := range tb.Categories[cat].Of(r) {
				if item == 0 {
					t.Errorf("categoria %d raridade %v com item zero", cat, r)
				}
			}
		}
	}
}

// timeoutRoll limita a espera do teste de laco infinito.
func timeoutRoll() <-chan time.Time { return time.After(2 * time.Second) }
