package game

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/mobdata"
)

func TestMagoUsaAtaqueEDefesaMagicos(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skLancaRaio, gamedata.T_SkillData{Damage: 0, Classe: 41})
	a.Avatar.ClassInfo = 41 // mago
	a.Avatar.AtkFis, a.Avatar.AtkMag = 10, 3000
	b.Avatar.DefFis, b.Avatar.DefMag = 8000, 0
	b.Avatar.CurHP = b.Avatar.MaxHP
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skLancaRaio, uint32(b.ClientID), a.Position)
	magico := antes - b.Avatar.CurHP

	h2, a2, ae2, b2, _ := prepWar(t, skLancaRaio, gamedata.T_SkillData{Damage: 0})
	a2.Avatar.ClassInfo = 1 // guerreiro: fisico
	a2.Avatar.AtkFis, a2.Avatar.AtkMag = 10, 3000
	b2.Avatar.DefFis, b2.Avatar.DefMag = 8000, 0
	b2.Avatar.CurHP = b2.Avatar.MaxHP
	antes2 := b2.Avatar.CurHP
	lanca(t, h2, a2, ae2, skLancaRaio, uint32(b2.ClientID), a2.Position)
	fisico := antes2 - b2.Avatar.CurHP

	if magico <= fisico {
		t.Errorf("mago causou %d, guerreiro %d: o mago deveria usar o ataque magico", magico, fisico)
	}
	if magico < 2000 {
		t.Errorf("dano do mago = %d, esperado perto de 3000 menos a defesa magica", magico)
	}
}

func TestSkillMobBaseDamageClasse(t *testing.T) {
	h, a, _, _, _ := prepWar(t, skLancaRaio, gamedata.T_SkillData{})
	sp := &mobdata.Species{DefFis: 8000, DefMag: 0}
	a.Avatar.AtkFis, a.Avatar.AtkMag = 100, 2000
	a.Avatar.ClassInfo = 41
	if got := h.skillMobBaseDamage(a, sp); got != 2000 {
		t.Errorf("mago em mob = %d, queria 2000", got)
	}
	a.Avatar.ClassInfo = 1
	if got := h.skillMobBaseDamage(a, sp); got != 1 {
		t.Errorf("guerreiro em mob = %d, queria 1 (100 - 1000 -> piso)", got)
	}
}

func TestCorroerCuraEVinculoFereOAutor(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skCorroer, gamedata.T_SkillData{Adicional: 50, Classe: 41})
	a.Avatar.ClassInfo = 41
	a.Avatar.CurHP = 100
	lanca(t, h, a, ae, skCorroer, uint32(b.ClientID), a.Position)
	if a.Avatar.CurHP <= 100 {
		t.Errorf("Corroer nao curou: %d", a.Avatar.CurHP)
	}

	h2, a2, ae2, b2, _ := prepWar(t, skVinculo, gamedata.T_SkillData{Adicional: 5, Classe: 41})
	a2.Avatar.ClassInfo = 41
	a2.Avatar.CurHP = a2.Avatar.MaxHP
	lanca(t, h2, a2, ae2, skVinculo, uint32(b2.ClientID), a2.Position)
	if a2.Avatar.CurHP >= a2.Avatar.MaxHP {
		t.Error("Vinculo deveria ferir o autor")
	}
}

func TestCristalizarManaSomaPorcentoDaManaDoAlvo(t *testing.T) {
	d := &gamedata.T_SkillData{Index: skCristalizarMana, Adicional: 10}
	if got := mageAdjust(d, 100, 1000); got != 100+100 {
		t.Errorf("dano = %d, queria 200", got)
	}
	if got := mageAdjust(d, 100, 0); got != 100 {
		t.Errorf("mob (mana 0) = %d, queria 100", got)
	}
	if got := mageAdjust(&gamedata.T_SkillData{Index: 1}, 100, 1000); got != 100 {
		t.Errorf("skill comum = %d", got)
	}
}

func TestMagoStunSilencioESkillsSemDano(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skChamaCaotica, gamedata.T_SkillData{Damage: 10, Duration: 5, Classe: 41})
	a.Avatar.ClassInfo = 41
	b.Avatar.Ability[efImSkillStun] = 300
	lanca(t, h, a, ae, skChamaCaotica, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(skChamaCaotica) {
		t.Error("stun resistido entrou")
	}
	h2, a2, ae2, b2, _ := prepWar(t, skImpedimento, gamedata.T_SkillData{Damage: 10, Duration: 5, Classe: 41})
	a2.Avatar.ClassInfo = 41
	b2.Avatar.Ability[efImSilence] = 300
	lanca(t, h2, a2, ae2, skImpedimento, uint32(b2.ClientID), a2.Position)
	if b2.Base.Buffs.Has(skImpedimento) {
		t.Error("silencio resistido entrou")
	}
	for _, sk := range []int{skPolimorfo, skMaoEscuridao} {
		h3, a3, ae3, b3, _ := prepWar(t, sk, gamedata.T_SkillData{Damage: 900, Duration: 5, Classe: 41})
		a3.Avatar.ClassInfo = 41
		antes := b3.Avatar.CurHP
		lanca(t, h3, a3, ae3, uint32(sk), uint32(b3.ClientID), a3.Position)
		if b3.Avatar.CurHP != antes {
			t.Errorf("skill %d causou dano", sk)
		}
	}
}
