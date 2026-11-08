package game

import (
	"aika/internal/combat"
	"aika/internal/gamedata"
	"aika/internal/skillfn"
)

// Indices de skill do Atirador e do Dual Gunner (GlobalDefs.pas).
const (
	skTiroFatal        = 50
	skTiroNaPerna      = 52
	skPerseguidor      = 55
	skPrimeiroEncontro = 60
	skPontoVital       = 63
	skMarcaPerseguidor = 64
	skContraGolpe      = 66
	skEliminacao       = 67
	skEspinhoVenenoso  = 74
	skRequiem          = 80
	skVenenoMana       = 84
	skChoqueSubito     = 86
	skNegarCura        = 88
	skChoqueHidra      = 94
	skAtaqueAtordoante = 151
	skVenenoHidra      = 154
	skDestino          = 344
	skInspirarMatanca  = 384
	skSentenca         = 385
	skPosturaFantasma  = 386
	skMorteDecidida    = 409
)

// Tipos de estado extras (GlobalDefs.pas) e as imunidades correspondentes.
const (
	lentType      = 4
	chockType     = 5
	paralisysType = 6

	efImRunSpeed  = 242
	efImImmovable = 243
	efImShock     = 249
)

// statusKinds descreve cada tipo de estado: a imunidade do alvo que soma na
// taxa de resistência e o nome nas mensagens.
var statusKinds = map[int]struct {
	ef   int
	nome string
}{
	stunType:      {efImSkillStun, "stun"},
	silenceType:   {efImSilence, "silencio"},
	lentType:      {efImRunSpeed, "lentidao"},
	chockType:     {efImShock, "choque"},
	paralisysType: {efImImmovable, "paralisia"},
	fearType:      {efImFear, "medo"},
}

// gunnerDebuffKind devolve o estado que a skill de Atirador/Dual causa.
func gunnerDebuffKind(d *gamedata.T_SkillData) int {
	switch int(d.Index) {
	case skPrimeiroEncontro, skPontoVital, skAtaqueAtordoante, skVenenoHidra:
		return stunType
	case skTiroNaPerna, skVenenoLentid:
		return lentType
	case skChoqueSubito, skChoqueHidra:
		return chockType
	case skEspinhoVenenoso:
		return paralisysType
	}
	return mageDebuffKind(d)
}

// gunnerZeroDamage: skills que só testam o golpe (Dano := 0).
func gunnerZeroDamage(d *gamedata.T_SkillData) bool {
	if mageZeroDamage(d) {
		return true
	}
	switch int(d.Index) {
	case skEliminacao, skPerseguidor, skMarcaPerseguidor, skEspinhoVenenoso,
		skRequiem, skChoqueSubito, skNegarCura:
		return true
	}
	return false
}

// gunnerAdjust é o ajuste de dano (e às vezes do tipo) de RiflemanSkill e
// DualGunnerSkill, depois do cálculo comum. defFis é a defesa física do alvo.
func (h *Handler) gunnerAdjust(d *gamedata.T_SkillData, dano int, tipo combat.DamageType, defFis int) (int, combat.DamageType) {
	rnd := h.rand()
	switch int(d.Index) {
	case skContraGolpe:
		// Acerto vira crítico e soma Adicional%.
		return dano + dano/100*int(d.Adicional), combat.Critical
	case skSentenca:
		// Chance de (DamageRange - 20)% de dobrar.
		if rnd.IntN(100) <= int(d.DamageRange)-20 {
			return dano * 2, tipo
		}
	case skDestino:
		return dano + (defFis >> 3), tipo
	case skVenenoMana:
		return dano + int(d.Adicional), tipo
	case skMorteDecidida:
		fator := max(rnd.IntN(100), 30)
		return dano + (int(d.Damage)+1000)/100*fator, tipo
	}
	return dano, tipo
}

// gunnerOnHit são os efeitos do golpe que acertou, em quem atira e em quem
// apanhou. alvo é nil quando o alvo é um mob.
func (h *Handler) gunnerOnHit(s, alvo *Session, d *gamedata.T_SkillData, dano int, matou bool) {
	switch int(d.Index) {
	case skEliminacao:
		if alvo != nil {
			h.removeBuffs(alvo, 1)
		}
	case skPosturaFantasma, skDestino:
		// Lança em si mesmo a skill indicada em Adicional.
		if sub := skillByIndex(h, uint32(d.Adicional)); sub != nil {
			h.targetBuffSkill(s, s, uint16(d.Adicional), sub, true)
		}
	case skVenenoMana:
		mana := uint32(d.Adicional)
		if alvo != nil {
			alvo.Avatar.CurMP -= min(mana, alvo.Avatar.CurMP)
			if c := h.connOf(alvo); c != nil {
				h.sendCurrentHPMP(c, alvo)
			}
		}
		h.healPlayer(h.connOf(s), s, 0, mana)
	case skInspirarMatanca:
		cura := uint32(dano / 100 * int(d.Adicional))
		if matou && alvo != nil {
			cura += s.Avatar.MaxHP / 100 * 25
		}
		h.healPlayer(h.connOf(s), s, cura, 0)
	}
}

// skillByIndex devolve a linha da tabela de skill, ou nil.
func skillByIndex(h *Handler, i uint32) *gamedata.T_SkillData {
	return skillfn.Data(h.SkillTable, i)
}
