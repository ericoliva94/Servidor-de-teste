package game

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// prepClasse monta uma skill em area (SuccessRate 1, Range>0) de uma classe.
func prepClasse(t *testing.T, idx, classe int, d gamedata.T_SkillData) (*Handler, *Session, *espiao, *Session) {
	t.Helper()
	d.SuccessRate, d.Range, d.DamageRange, d.Classe = 1, 5, 20, uint32(classe)
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(idx, d)
	a.Avatar.ClassInfo = uint16(classe)
	a.Equip.Slots[BulletSlot] = model.Item{Index: 1000, Refi: 500} // Atirador e Dual precisam de bala
	a.Skills.Others[0] = model.SkillFromList{Index: uint16(idx), Level: 5}
	return h, a, ae, b
}

func TestSkillsEmAreaCausamOEstadoDaClasse(t *testing.T) {
	casos := []struct {
		nome   string
		skill  int
		classe int
		ef     int
	}{
		{"Cancao de Guerra (stun)", skCancaoGuerra, 1, efImSkillStun},
		{"Area de Impacto (lentidao)", skAreaImpacto, 1, efImRunSpeed},
		{"Emissao Divina (silencio)", skEmissaoDivina, 11, efImSilence},
		{"Detonacao (silencio)", skDetonacao, 21, efImSilence},
		{"Explosao Radiante (stun)", skExplosaoRadiante, 31, efImSkillStun},
		{"Queda Negra (stun)", skQuedaNegra, 41, efImSkillStun},
		{"Tempestade de Raios (lentidao)", skTempestadeRaios, 41, efImRunSpeed},
		{"Grito de Medo (medo)", skGritoMedo, 1, efImFear},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d := gamedata.T_SkillData{Damage: 10, Duration: 5}
			h, a, ae, b := prepClasse(t, c.skill, c.classe, d)
			lanca(t, h, a, ae, uint32(c.skill), 0, a.Position)
			if !b.Base.Buffs.Has(uint16(c.skill)) {
				t.Fatal("o estado deveria entrar em quem estava na area")
			}
			h2, a2, ae2, b2 := prepClasse(t, c.skill, c.classe, d)
			b2.Avatar.Ability[c.ef] = 300
			lanca(t, h2, a2, ae2, uint32(c.skill), 0, a2.Position)
			if b2.Base.Buffs.Has(uint16(c.skill)) {
				t.Error("estado resistido entrou")
			}
		})
	}
}

func TestSkillsEmAreaSoDeTesteNaoCausamDano(t *testing.T) {
	for _, c := range []struct{ skill, classe int }{
		{skGritoMedo, 1}, {skIncitarMultidao, 11}, {skBruma, 41}, {skEclater, 41},
	} {
		h, a, ae, b := prepClasse(t, c.skill, c.classe, gamedata.T_SkillData{Damage: 900, Duration: 5})
		antes := b.Avatar.CurHP
		lanca(t, h, a, ae, uint32(c.skill), 0, a.Position)
		if b.Avatar.CurHP != antes {
			t.Errorf("skill %d causou dano: %d -> %d", c.skill, antes, b.Avatar.CurHP)
		}
	}
}

func TestIncitarMultidaoRevelaInvisiveis(t *testing.T) {
	h, a, ae, b := prepClasse(t, skIncitarMultidao, 11, gamedata.T_SkillData{Duration: 5})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(buffInvAtirador, gamedata.T_SkillData{Duration: 60})
	tab.poe(buffInvDual, gamedata.T_SkillData{Duration: 60})
	h.addBuff(b, buffInvAtirador)
	h.addBuff(b, buffInvDual)
	lanca(t, h, a, ae, skIncitarMultidao, 0, a.Position)
	if b.Base.Buffs.Has(buffInvAtirador) || b.Base.Buffs.Has(buffInvDual) {
		t.Error("a invisibilidade deveria ter sido revelada")
	}
}

func TestPecadosMortaisTemDanoContinuoFixo(t *testing.T) {
	h, a, ae, b := prepClasse(t, skPecadosMortais, 41, gamedata.T_SkillData{Damage: 10, Duration: 8, EFV: [4]int32{37}})
	lanca(t, h, a, ae, skPecadosMortais, 0, a.Position)
	if st, ok := b.dots[skPecadosMortais]; !ok || st.dmg != 37 {
		t.Errorf("dot = %+v ok=%v, queria dano fixo 37 (EFV[0])", st, ok)
	}
}

func TestSkillEmAreaDeMagoUsaDanoMagico(t *testing.T) {
	h, a, ae, b := prepClasse(t, skTempestade, 41, gamedata.T_SkillData{Damage: 0})
	a.Avatar.AtkFis, a.Avatar.AtkMag = 10, 3000
	b.Avatar.DefFis, b.Avatar.DefMag = 8000, 0
	b.Avatar.CurHP = b.Avatar.MaxHP
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skTempestade, 0, a.Position)
	if antes-b.Avatar.CurHP < 2000 {
		t.Errorf("dano em area do mago = %d, esperado perto de 3000", antes-b.Avatar.CurHP)
	}
}
