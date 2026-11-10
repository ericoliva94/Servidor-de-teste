package game

import (
	"context"
	"testing"

	"aika/internal/gamedata"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/pran"
)

// tabela de teste: PranExpList[i] = i*100.
func tabelaPranExp() []uint32 {
	t := make([]uint32, 80)
	for i := range t {
		t[i] = uint32(i * 100)
	}
	return t
}

// pranAtiva monta um dono com a pran invocada (entrada completa).
func pranAtiva(t *testing.T, mod func(*pran.Pran)) (*Handler, *Session, *espiao, *pran.Pran) {
	t.Helper()
	p := pranDeTeste()
	p.Skills = [pran.SkillSlots]pran.Skill{}
	if mod != nil {
		mod(p)
	}
	h, s, e, _, _ := cenarioPranFx(t, p)
	h.PranExp = tabelaPranExp()
	h.enterPran(e.conn, s)
	e.drena(5 * 1e6)
	return h, s, e, s.Prans[0]
}

func levelExp(t *testing.T, e *espiao) packets.RefreshPranLevelExpPacket {
	t.Helper()
	return decodifica[packets.RefreshPranLevelExpPacket](t, proximoDe(t, e, CodePranLevelExp))
}

func TestExpDaPranUmTercoSemTeto(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp = 2, 0 })
	h.awardPranExp(e.conn, s, 300) // um terco = 100; nivel 2 precisa de 300 para o 3
	got := levelExp(t, e)
	if p.Exp != 100 || p.Level != 2 || got.Level != 3 || got.Exp != 100 {
		t.Errorf("pran = nivel %d exp %d, pacote %+v", p.Level, p.Exp, got)
	}
}

func TestExpDaPranSobeVariosNiveis(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp, p.Class = 10, 0, 63 })
	h.awardPranExp(e.conn, s, 3*1500) // ganha 1500: passa pelas linhas 1100,1200,1300,1400
	if p.Level != 14 || p.Exp != 1500 {
		t.Errorf("nivel %d exp %d, queria nivel 14 exp 1500", p.Level, p.Exp)
	}
	if p.MaxHP != 200+4*pran.HPPerLevel || p.CurHP != p.MaxHP {
		t.Errorf("HP = %d/%d", p.CurHP, p.MaxHP)
	}
	if got := mensagemDe(t, e); got != "Sua pran subiu de nível." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestExpDaPranTetoDaFaseFada(t *testing.T) {
	// Nivel 2, 450 de exp, ganho 100: passaria de PranExpList[5]=500, entao fica
	// em PranExpList[4]=400 e sobe os niveis 2 e 3 -> nivel 4.
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp, p.Class = 2, 450, 61 })
	h.awardPranExp(e.conn, s, 300)
	if p.Level != 4 || p.Exp != 400 {
		t.Errorf("nivel %d exp %d, queria nivel 4 exp 400", p.Level, p.Exp)
	}
}

func TestPranNaFronteiraPrecisaEvoluir(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp, p.Class = 4, 50, 61 })
	h.awardPranExp(e.conn, s, 300)
	if p.Exp != 50 {
		t.Errorf("fada no nivel 4 nao ganha exp: %d", p.Exp)
	}
	if got := mensagemDe(t, e); got != "A sua pran precisa evoluir para ganhar exp." {
		t.Errorf("mensagem = %q", got)
	}
	p.Class = 62 // ja evoluida
	h.awardPranExp(e.conn, s, 300)
	if p.Exp != 150 {
		t.Errorf("evoluida ganha: %d", p.Exp)
	}
}

func TestPranAdultaSemTeto(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp, p.Class = 50, 9000, 64 })
	h.awardPranExp(e.conn, s, 3000)
	if p.Exp != 10000 {
		t.Errorf("exp = %d, queria 10000", p.Exp)
	}
}

func TestSemPranInvocadaOuSemTabelaNaoGanha(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp = 2, 0 })
	s.SpawnedPran = -1
	h.awardPranExp(e.conn, s, 3000)
	if p.Exp != 0 {
		t.Error("sem pran invocada nao ganha")
	}
	s.SpawnedPran = 0
	h.PranExp = nil // sem tabela: nao le fora do vetor
	h.awardPranExp(e.conn, s, 3000)
}

func TestSubirDeNivelRefazAsPassivas(t *testing.T) {
	// Nivel 3 -> 4 (mostra 5, marco): a habilidade de ataque (familia 193) vai
	// do nivel 3 ao 4, e o bonus do dono de 11 para 13.
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) {
		p.Level = 3
		p.Skills[0] = pran.Skill{Index: 500, Level: 3}
	})
	sk := h.SkillTable.(*tabelaDeSkills)
	sk.m[502] = gamedata.T_SkillData{Index: 193}
	sk.m[503] = gamedata.T_SkillData{Index: 193}
	h.applyPranEffects(s, p) // reaplica com a tabela agora preenchida
	if got := s.Avatar.Ability.get(efPranDamage1); got != 11 {
		t.Fatalf("dano antes = %d, queria 11", got)
	}
	h.addPranLevel(e.conn, s, 0)
	if got := s.Avatar.Ability.get(efPranDamage1); got != 13 {
		t.Errorf("dano depois = %d, queria 13 (nivel 4 da habilidade)", got)
	}
}

func TestSubirDeNivelAvisaABarra(t *testing.T) {
	h, s, e, _ := pranAtiva(t, func(p *pran.Pran) {
		p.Level = 3
		p.Skills[0] = pran.Skill{Index: 500, Level: 1}
		p.ItemBar[1] = 500
	})
	h.addPranLevel(e.conn, s, 0)
	got := decodifica[packets.ChangeItemBarPacket](t, proximoDe(t, e, CodeChangeItemBar))
	if got.DestSlot != 1 || got.SrcType != 3 || got.SrcIndex != 501 {
		t.Errorf("barra = %+v", got)
	}
}

func comidaNoInventario(t *testing.T, h *Handler, tab *tabelaDeItens, s *Session, item uint16, tipo uint16) {
	t.Helper()
	tab.poeItem(int(item), gamedata.ItemFromList{ItemType: tipo})
	s.Inventory.Slots[0] = model.Item{Index: item, Refi: 2}
}

func TestComidaDePranGastaOItemEMudaPersonalidade(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Food, p.Devotion, p.Cute, p.Sexy = 10, 5, 3, 8 })
	tab := h.data().(*tabelaDeItens)
	comidaNoInventario(t, h, tab, s, 8106, ItemTypePranFood)
	if err := h.useInventoryItem(context.Background(), e.conn, s, 0, 0); err != nil {
		t.Fatal(err)
	}
	if p.Food != 25 || p.Devotion != 6 || p.Sexy != 10 || p.Cute != 0 {
		t.Errorf("pran = comida %d devocao %d sexy %d cute %d", p.Food, p.Devotion, p.Sexy, p.Cute)
	}
	if s.Inventory.Slots[0].Refi != 1 {
		t.Errorf("item deveria gastar uma unidade: %d", s.Inventory.Slots[0].Refi)
	}
	proximoDe(t, e, CodePranToWorld)
}

func TestComidaCheiaOuSemPranNaoGasta(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Food = 121 })
	tab := h.data().(*tabelaDeItens)
	comidaNoInventario(t, h, tab, s, 8106, ItemTypePranFood)
	_ = h.useInventoryItem(context.Background(), e.conn, s, 0, 0)
	if got := mensagemDe(t, e); got != "Sua pran não consegue comer mais." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[0].Refi != 2 {
		t.Error("pran cheia nao gasta o item")
	}
	p.Food = 10
	s.SpawnedPran = -1
	_ = h.useInventoryItem(context.Background(), e.conn, s, 0, 0)
	if s.Inventory.Slots[0].Refi != 2 || p.Food != 10 {
		t.Error("sem pran invocada nao gasta nem alimenta")
	}
}

func TestDigestivo(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Food = 80 })
	tab := h.data().(*tabelaDeItens)
	comidaNoInventario(t, h, tab, s, 8200, ItemTypePranDigest)
	_ = h.useInventoryItem(context.Background(), e.conn, s, 0, 0)
	if p.Food != 40 {
		t.Errorf("comida = %d, queria 40", p.Food)
	}
	p.Food = 13
	s.Inventory.Slots[0] = model.Item{Index: 8200, Refi: 2}
	_ = h.useInventoryItem(context.Background(), e.conn, s, 0, 0)
	if got := mensagemDe(t, e); got != "Sua pran está com muita fome para usar o Digestivo." {
		t.Errorf("mensagem = %q", got)
	}
}

// O abate de mob entrega a experiencia ao dono e um terco a pran invocada.
func TestAbateDeMobDaExpParaAPran(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Level, p.Exp, p.Class = 20, 0, 63 })
	h.ExpMultiplier = 1
	h.onMobDeath(e.conn, s, &Mob{ClientID: 100}, &mobdata.Species{Exp: 900, Level: s.Avatar.Level})
	if p.Exp != 300 {
		t.Errorf("exp da pran = %d, queria 300 (um terco de 900)", p.Exp)
	}
}
