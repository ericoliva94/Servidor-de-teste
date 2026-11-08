package charcreate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func realDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data", "BaseAccs")
	if _, err := os.Stat(filepath.Join(dir, "Guerreiro.acc")); err != nil {
		t.Skip("modelos reais ausentes")
	}
	return dir
}

func TestModelosReaisSaoLidosComAsClassesEOsAtributosDoCodigo(t *testing.T) {
	tpls, warns := Load(realDir(t))
	if len(warns) != 0 {
		t.Fatalf("avisos: %v", warns)
	}
	want := [NumClasses]struct {
		class         uint8
		str, agi, con uint16
	}{{1, 15, 9, 16}, {11, 14, 10, 14}, {21, 8, 16, 12}, {31, 8, 14, 12}, {41, 7, 9, 8}, {51, 7, 10, 9}}
	for i, w := range want {
		g := tpls[i]
		if !g.Loaded || g.ClassInfo != w.class || g.Str != w.str || g.Agi != w.agi || g.Cons != w.con {
			t.Errorf("%s = %+v, queria classe %d str %d agi %d con %d", FileNames[i], g, w.class, w.str, w.agi, w.con)
		}
		if g.Level != 1 {
			t.Errorf("%s: nivel %d, queria 1", FileNames[i], g.Level)
		}
		if g.Sizes.Altura != 0x07 || g.Sizes.Tronco != 0x77 || g.Sizes.Perna != 0x77 {
			t.Errorf("%s: corpo %+v", FileNames[i], g.Sizes)
		}
	}
	// O modelo traz o equipamento inicial (slots 2 a 7) e itens no inventario.
	g := tpls[ClassWarrior]
	equipado := 0
	for i := 2; i <= 7; i++ {
		if g.Equip[i].Index != 0 {
			equipado++
		}
	}
	if equipado == 0 || g.Inventory[0].Index == 0 {
		t.Errorf("modelo sem equipamento (%d) ou sem inventario (%d)", equipado, g.Inventory[0].Index)
	}
}

func TestSkillIndexComoGetSkillIndex(t *testing.T) {
	casos := []struct{ classe, skill, level, want int }{
		{1, 1, 1, 1},   // guerreiro: nivel 1 nao soma
		{1, 2, 1, 17},  // + (2-1)*16
		{1, 1, 3, 3},   // + (3-1)
		{2, 1, 1, 961}, // (2-1)*960 + 1
		{2, 7, 1, 960 + 96 + 1},
		{6, 1, 1, 5*960 + 1},
		{6, 1, 4, 5*960 + 3},
	}
	for _, c := range casos {
		if got := SkillIndex(c.classe, c.skill, c.level); got != c.want {
			t.Errorf("SkillIndex(%d,%d,%d) = %d, queria %d", c.classe, c.skill, c.level, got, c.want)
		}
	}
}

func TestSkillsIniciais(t *testing.T) {
	l := Skills(ClassWarrior)
	for i := 0; i < 4; i++ {
		if l.Basics[i].Level != 1 {
			t.Errorf("basica %d nivel %d", i, l.Basics[i].Level)
		}
	}
	if l.Basics[4].Level != 0 || l.Basics[5].Level != 0 || l.Others[0].Level != 1 || l.Others[1].Level != 0 {
		t.Errorf("niveis = %+v / %+v", l.Basics, l.Others[:2])
	}
	if l.Basics[0].Index != 1 || l.Basics[3].Index != 49 || l.Others[0].Index != 97 || l.Others[39].Index != 1+(39+6)*16 {
		t.Errorf("indices = %v %v %v %v", l.Basics[0].Index, l.Basics[3].Index, l.Others[0].Index, l.Others[39].Index)
	}
	m := Skills(ClassMage)
	if m.Basics[0].Index != 4*960+1 {
		t.Errorf("feiticeiro basica 0 = %d", m.Basics[0].Index)
	}
}

func TestBarraInicialSoGuerreiroECleriga(t *testing.T) {
	for c := 0; c < NumClasses; c++ {
		bar := ItemBar(c, Skills(c))
		tem := bar[3] != 0
		if tem != (c == ClassWarrior || c == ClassCleric) {
			t.Errorf("classe %d: barra[3] = %d", c, bar[3])
		}
	}
	if got := ItemBar(ClassWarrior, Skills(ClassWarrior))[3]; got != 49*16+2 {
		t.Errorf("barra do guerreiro = %d", got)
	}
}

func TestNomeValido(t *testing.T) {
	casos := []struct {
		nome string
		err  error
	}{
		{"Alfa", nil}, {"Alfa123", nil}, {"12345678901234", nil},
		{"", ErrNameEmpty}, {"123456789012345", ErrNameLong},
		{"Al fa", ErrNameChars}, {"Alfa!", ErrNameChars}, {"Álfa", ErrNameChars}, {"a'b", ErrNameChars}, {"a;b", ErrNameChars},
	}
	for _, c := range casos {
		if err := ValidName(c.nome); !errors.Is(err, c.err) {
			t.Errorf("ValidName(%q) = %v, queria %v", c.nome, err, c.err)
		}
	}
}

func TestClasseECabelo(t *testing.T) {
	for idx, want := range map[uint16]int{10: 0, 19: 0, 20: 1, 35: 2, 49: 3, 50: 4, 69: 5} {
		if got, err := ClassGroup(idx); err != nil || got != want {
			t.Errorf("ClassGroup(%d) = %d, %v", idx, got, err)
		}
	}
	for _, idx := range []uint16{0, 9, 70, 65535} {
		if _, err := ClassGroup(idx); !errors.Is(err, ErrBadClass) {
			t.Errorf("ClassGroup(%d) deveria falhar", idx)
		}
	}
	for _, h := range []uint16{7699, 7732, 0, 65535} {
		if ValidHair(h) == nil {
			t.Errorf("cabelo %d deveria falhar", h)
		}
	}
	if ValidHair(7700) != nil || ValidHair(7731) != nil {
		t.Error("os extremos validos foram recusados")
	}
}

func TestPosicaoDePartida(t *testing.T) {
	if x, y := StartPosition(0); x != 3432 || y != 672 {
		t.Errorf("local 0 = %v,%v", x, y)
	}
	if x, y := StartPosition(1); x != 3443 || y != 912 {
		t.Errorf("local 1 = %v,%v", x, y)
	}
	if x, y := StartPosition(7); x != 0 || y != 0 {
		t.Errorf("local 7 = %v,%v", x, y)
	}
}

func TestBuildMontaPersonagemComFaceCabeloBolsaEMunicao(t *testing.T) {
	tpls, _ := Load(realDir(t))
	n, err := Build(tpls[ClassSniper], 30, 7710, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n.Equip[EquipFace].Index != 30 || n.Equip[EquipHair].Index != 7710 {
		t.Errorf("face/cabelo = %d/%d", n.Equip[0].Index, n.Equip[1].Index)
	}
	if n.Inventory[StarterBagSlot].Index != ItemStarterBag {
		t.Errorf("bolsa = %d", n.Inventory[StarterBagSlot].Index)
	}
	for _, it := range []struct {
		nome string
		got  uint16
		refi uint16
	}{{"Equip[15]", n.Equip[15].Index, n.Equip[15].Refi}, {"Inv[5]", n.Inventory[5].Index, n.Inventory[5].Refi}, {"Inv[6]", n.Inventory[6].Index, n.Inventory[6].Refi}} {
		if it.got != ItemBulletSniper || it.refi != 1000 {
			t.Errorf("%s = %d x%d, queria bala %d x1000", it.nome, it.got, it.refi, ItemBulletSniper)
		}
	}
	if n.PosX != 3443 || n.PosY != 912 {
		t.Errorf("posicao = %v,%v", n.PosX, n.PosY)
	}
	if n.Skills.Basics[0].Index == 0 {
		t.Error("sem skills iniciais")
	}

	// Classe sem municao nao ganha.
	g, _ := Build(tpls[ClassWarrior], 10, 7700, 0)
	if g.Equip[EquipBullet].Index != 0 || g.Inventory[InventoryBullet1].Index == ItemBulletSniper {
		t.Error("guerreiro nao leva municao")
	}
	// Pistoleira usa outra bala.
	p, _ := Build(tpls[ClassGunslinger], 40, 7700, 0)
	if p.Equip[EquipBullet].Index != ItemBulletGunner {
		t.Errorf("bala da pistoleira = %d", p.Equip[EquipBullet].Index)
	}
}

func TestBuildRecusas(t *testing.T) {
	var vazio Template
	if _, err := Build(vazio, 10, 7700, 0); !errors.Is(err, ErrNoTemplate) {
		t.Errorf("sem modelo: %v", err)
	}
	t0 := Template{Loaded: true, ClassInfo: 1, Level: 1}
	if _, err := Build(t0, 9, 7700, 0); !errors.Is(err, ErrBadClass) {
		t.Errorf("classe: %v", err)
	}
	if _, err := Build(t0, 10, 7000, 0); !errors.Is(err, ErrBadHair) {
		t.Errorf("cabelo: %v", err)
	}
}

func TestParseLixoNaoQuebra(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Error("arquivo vazio deveria falhar")
	}
	if _, err := Parse(make([]byte, 100)); err == nil {
		t.Error("arquivo curto deveria falhar")
	}
	if _, err := Parse(make([]byte, 8000)); err == nil {
		t.Error("modelo zerado (sem classe) deveria falhar")
	}
	_, w := Load(t.TempDir())
	if len(w) != NumClasses {
		t.Errorf("%d avisos, queria %d", len(w), NumClasses)
	}
}

func TestBolsaInicialNasceVisivel(t *testing.T) {
	tpls, _ := Load(realDir(t))
	n, err := Build(tpls[ClassSniper], 30, 7710, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := n.Inventory[StarterBagSlot]
	if b.Index != ItemStarterBag || b.APP != ItemStarterBag || b.Refi != 1 {
		t.Errorf("bolsa = %+v", b)
	}
}
