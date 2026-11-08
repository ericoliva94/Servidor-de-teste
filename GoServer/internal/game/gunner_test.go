package game

import (
	"testing"

	"aika/internal/combat"
	"aika/internal/gamedata"
)

func TestContraGolpeViraCriticoESomaAdicional(t *testing.T) {
	h, _, _, _, _ := prepWar(t, skContraGolpe, gamedata.T_SkillData{})
	d := &gamedata.T_SkillData{Index: skContraGolpe, Adicional: 50}
	dano, tipo := h.gunnerAdjust(d, 1000, combat.Normal, 0)
	if dano != 1500 || tipo != combat.Critical {
		t.Errorf("dano=%d tipo=%v, queria 1500/Critical", dano, tipo)
	}
}

func TestSentencaDobraPelaChanceDaTabela(t *testing.T) {
	h, _, _, _, _ := prepWar(t, skSentenca, gamedata.T_SkillData{})
	// randFixo(50): dobra quando 50 <= DamageRange-20.
	sim := &gamedata.T_SkillData{Index: skSentenca, DamageRange: 90}
	nao := &gamedata.T_SkillData{Index: skSentenca, DamageRange: 40}
	if d, _ := h.gunnerAdjust(sim, 1000, combat.Normal, 0); d != 2000 {
		t.Errorf("com chance alta = %d, queria 2000", d)
	}
	if d, _ := h.gunnerAdjust(nao, 1000, combat.Normal, 0); d != 1000 {
		t.Errorf("com chance baixa = %d, queria 1000", d)
	}
}

func TestDestinoEMorteDecididaSomamBonus(t *testing.T) {
	h, _, _, _, _ := prepWar(t, skDestino, gamedata.T_SkillData{})
	if d, _ := h.gunnerAdjust(&gamedata.T_SkillData{Index: skDestino}, 100, combat.Normal, 800); d != 100+100 {
		t.Errorf("Destino = %d, queria 200 (defesa 800 >> 3)", d)
	}
	// randFixo(50): fator = max(50,30) = 50; (Damage+1000)/100*50
	if d, _ := h.gunnerAdjust(&gamedata.T_SkillData{Index: skMorteDecidida, Damage: 200}, 100, combat.Normal, 0); d != 100+12*50 {
		t.Errorf("Morte Decidida = %d, queria 700", d)
	}
}

func TestVenenoDeManaRoubaMana(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skVenenoMana, gamedata.T_SkillData{Damage: 10, Adicional: 40})
	a.Avatar.CurMP, b.Avatar.CurMP = 0, 100
	lanca(t, h, a, ae, skVenenoMana, uint32(b.ClientID), a.Position)
	if b.Avatar.CurMP != 60 || a.Avatar.CurMP != 40 {
		t.Errorf("MP alvo=%d autor=%d, queria 60 e 40", b.Avatar.CurMP, a.Avatar.CurMP)
	}
}

func TestInspirarMatancaCuraOAutor(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skInspirarMatanca, gamedata.T_SkillData{Damage: 10, Adicional: 20})
	a.Avatar.CurHP = 100
	lanca(t, h, a, ae, skInspirarMatanca, uint32(b.ClientID), a.Position)
	if a.Avatar.CurHP <= 100 {
		t.Errorf("HP do autor = %d, deveria ter subido", a.Avatar.CurHP)
	}
}

func TestEliminacaoTiraUmBuffSemDano(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skEliminacao, gamedata.T_SkillData{Damage: 900})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(602, gamedata.T_SkillData{Duration: 60, BuffDebuff: 1})
	tab.poe(603, gamedata.T_SkillData{Duration: 60, BuffDebuff: 1})
	h.addBuff(b, 602)
	h.addBuff(b, 603)
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skEliminacao, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != antes {
		t.Errorf("Eliminacao causou dano: %d -> %d", antes, b.Avatar.CurHP)
	}
	if b.Base.Buffs.Count() != 1 {
		t.Errorf("buffs = %d, queria 1", b.Base.Buffs.Count())
	}
}

func TestPosturaFantasmaLancaASkillDeAdicional(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skPosturaFantasma, gamedata.T_SkillData{Damage: 10, Adicional: 700})
	h.SkillTable.(*tabelaDeSkills).poe(700, gamedata.T_SkillData{Duration: 30, EF: [4]int32{efHit}, EFV: [4]int32{9}})
	lanca(t, h, a, ae, skPosturaFantasma, uint32(b.ClientID), a.Position)
	if !a.Base.Buffs.Has(700) {
		t.Error("o autor deveria receber a skill de Adicional")
	}
}

func TestEstadosNovosPassamPelaResistencia(t *testing.T) {
	casos := []struct {
		nome  string
		skill int
		ef    int
	}{
		{"lentidao", skTiroNaPerna, efImRunSpeed},
		{"choque", skChoqueHidra, efImShock},
		{"paralisia", skEspinhoVenenoso, efImImmovable},
		{"stun", skPontoVital, efImSkillStun},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, a, ae, b, _ := prepWar(t, c.skill, gamedata.T_SkillData{Damage: 10, Duration: 5})
			lanca(t, h, a, ae, uint32(c.skill), uint32(b.ClientID), a.Position)
			if !b.Base.Buffs.Has(uint16(c.skill)) {
				t.Fatal("sem imunidade o estado deveria entrar")
			}
			h2, a2, ae2, b2, _ := prepWar(t, c.skill, gamedata.T_SkillData{Damage: 10, Duration: 5})
			b2.Avatar.Ability[c.ef] = 300
			lanca(t, h2, a2, ae2, uint32(c.skill), uint32(b2.ClientID), a2.Position)
			if b2.Base.Buffs.Has(uint16(c.skill)) {
				t.Error("estado resistido entrou")
			}
		})
	}
}

func TestSkillsDeSoTesteNaoCausamDano(t *testing.T) {
	for _, sk := range []int{skPerseguidor, skMarcaPerseguidor, skRequiem, skNegarCura, skChoqueSubito} {
		h, a, ae, b, _ := prepWar(t, sk, gamedata.T_SkillData{Damage: 900})
		antes := b.Avatar.CurHP
		lanca(t, h, a, ae, uint32(sk), uint32(b.ClientID), a.Position)
		if b.Avatar.CurHP != antes {
			t.Errorf("skill %d causou dano: %d -> %d", sk, antes, b.Avatar.CurHP)
		}
	}
}
