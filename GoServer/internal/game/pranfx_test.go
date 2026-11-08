package game

import (
	"context"
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/pran"
	"aika/internal/stats"
	"aika/internal/store"
)

func TestEfeitosDasPassivas(t *testing.T) {
	casos := []struct {
		fam, lv int
		want    []efv
	}{
		{193, 3, []efv{{efPranDamage1, 11}, {efPranDamage2, 11}}},
		{217, 3, []efv{{efPranDamage1, 9}, {efPranDamage2, 9}}},
		{241, 3, []efv{{efPranDamage1, 10}, {efPranDamage2, 10}}},
		{194, 4, []efv{{efPranResist1, 8}, {efPranResist2, 8}}},
		{218, 4, []efv{{efPranResist1, 8}, {efPranResist2, 8}}},
		{242, 4, []efv{{efPranResist1, 8}, {efPranResist2, 8}}},
		{197, 2, []efv{{efHP, 23 + 15*2}}},
		{199, 4, []efv{{efCritical, 4}}},
		{221, 2, []efv{{efMP, 70}}},
		{223, 5, []efv{{efResistance6, 5}}},
		{245, 1, []efv{{efHP, 24}, {efMP, 33}}},
		{247, 2, []efv{{efHP, 15 + 10*2}, {efMP, 21 + 13*2}}},
	}
	for _, k := range casos {
		got := passiveEffects(k.fam, k.lv)
		if len(got) != len(k.want) {
			t.Errorf("familia %d: %v, queria %v", k.fam, got, k.want)
			continue
		}
		for i := range got {
			if got[i] != k.want[i] {
				t.Errorf("familia %d: %v, queria %v", k.fam, got, k.want)
			}
		}
	}
	if passiveEffects(999, 3) != nil {
		t.Error("familia sem efeito deveria dar nil")
	}
}

// cenarioPranFx monta um dono com a pran equipada e as tabelas de teste.
func cenarioPranFx(t *testing.T, p *pran.Pran) (*Handler, *Session, *espiao, *tabelaDeItens, *tabelaDeSkills) {
	t.Helper()
	h, tab := handlerComItens(t)
	sk := novaTabelaDeSkills()
	h.SkillTable = sk
	e := novoEspiao(t)
	s := sessaoComInventario()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	h.PranDB = &fakePrans{carga: []*pran.Pran{p}}
	h.PranIDs = pran.NewIDs()
	s.Account.ID = 77
	s.Equip.Slots = make([]model.Item, store.EquipSlots)
	s.Equip.Slots[10] = model.Item{Index: 7000, Identific: int32(p.ItemID)}
	s.Avatar.Recalc(stats.Bonus{})
	h.loadPrans(context.Background(), s)
	return h, s, e, tab, sk
}

func TestPassivasEEquipamentoDaPranEntramNoDono(t *testing.T) {
	p := pranDeTeste()
	p.Skills = [pran.SkillSlots]pran.Skill{}
	p.Skills[0] = pran.Skill{Index: 500, Level: 3} // 502 -> familia 193
	p.Skills[1] = pran.Skill{Index: 600, Level: 2} // 601 -> familia 194
	p.Equip = [pran.EquipSlots]model.Item{}
	p.Equip[0] = model.Item{Index: 3000, Effects: model.ItemEffect{Index: [3]byte{efHP}, Value: [3]byte{10}}}
	p.Equip[1] = model.Item{Index: 3001} // roupa de ItemType 10: nao conta
	h, s, e, tab, sk := cenarioPranFx(t, p)
	sk.m[502] = gamedata.T_SkillData{Index: 193}
	sk.m[601] = gamedata.T_SkillData{Index: 194}
	tab.poeItem(3000, gamedata.ItemFromList{ItemType: 5, EF: [3]uint16{efCritical}, EFV: [3]uint16{7}})
	tab.poeItem(3001, gamedata.ItemFromList{ItemType: 10, EF: [3]uint16{efCritical}, EFV: [3]uint16{99}})

	atkAntes, defAntes := s.Avatar.AtkFis, s.Avatar.DefFis
	h.enterPran(e.conn, s)

	ab := &s.Avatar.Ability
	if ab.get(efPranDamage1) != 11 || ab.get(efPranDamage2) != 11 {
		t.Errorf("dano da pran = %d/%d, queria 11", ab.get(efPranDamage1), ab.get(efPranDamage2))
	}
	if ab.get(efPranResist1) != 4 || ab.get(efPranResist2) != 4 {
		t.Errorf("defesa da pran = %d/%d, queria 4", ab.get(efPranResist1), ab.get(efPranResist2))
	}
	if ab.get(efHP) != 20 { // efeito gravado no item vale o dobro
		t.Errorf("HP = %d, queria 20", ab.get(efHP))
	}
	if ab.get(efCritical) != 7 { // so o item comum; o de ItemType 10 e ignorado
		t.Errorf("critico = %d, queria 7", ab.get(efCritical))
	}
	if s.Avatar.AtkFis <= atkAntes || s.Avatar.DefFis != defAntes+4 {
		t.Errorf("atk %d->%d def %d->%d", atkAntes, s.Avatar.AtkFis, defAntes, s.Avatar.DefFis)
	}

	// Desligar devolve tudo.
	h.removePranEffects(s)
	if *ab != (Ability{}) {
		t.Errorf("sobrou efeito: %v", nonZero(ab))
	}
	if s.Avatar.AtkFis != atkAntes || s.Avatar.DefFis != defAntes {
		t.Errorf("atributos nao voltaram: atk %d def %d", s.Avatar.AtkFis, s.Avatar.DefFis)
	}
}

func nonZero(a *Ability) map[int]int {
	m := map[int]int{}
	for i, v := range a {
		if v != 0 {
			m[i] = v
		}
	}
	return m
}

// Ligar de novo nao soma em dobro, e subir o nivel da habilidade antes de
// desligar nao deixa resto (a origem recalculava na desativacao).
func TestReligarNaoDobraEDesligarDesfazOQueSomou(t *testing.T) {
	p := pranDeTeste()
	p.Skills = [pran.SkillSlots]pran.Skill{{Index: 500, Level: 3}}
	h, s, e, _, sk := cenarioPranFx(t, p)
	sk.m[502] = gamedata.T_SkillData{Index: 193}
	sk.m[503] = gamedata.T_SkillData{Index: 193}
	h.enterPran(e.conn, s)
	h.applyPranEffects(s, s.Prans[0])
	if got := s.Avatar.Ability.get(efPranDamage1); got != 11 {
		t.Fatalf("dano = %d, queria 11 (ligar duas vezes nao soma em dobro)", got)
	}
	s.Prans[0].Skills[0].Level = 4 // sobe de nivel com a pran ligada
	h.removePranEffects(s)
	if got := s.Avatar.Ability.get(efPranDamage1); got != 0 {
		t.Errorf("sobrou %d depois de desligar", got)
	}
}

func TestPranSemSkillTableOuFadaNaoQuebra(t *testing.T) {
	p := pranDeTeste()
	p.Skills[0] = pran.Skill{Index: 500, Level: 3}
	h, s, e, _, _ := cenarioPranFx(t, p)
	h.SkillTable = nil
	h.enterPran(e.conn, s) // sem tabela: so o equipamento conta, sem panico
}
