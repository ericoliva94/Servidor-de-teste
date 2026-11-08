package game

import (
	"aika/internal/gamedata"
	"aika/internal/mobdata"
)

// Indices de skill do Mago (GlobalDefs.pas).
const (
	skChamaCaotica    = 97
	skPolimorfo       = 99
	skOndaChoque      = 100
	skInfernoCaotico  = 102
	skImpedimento     = 103
	skCorroer         = 104
	skVinculo         = 157
	skLancaRaio       = 225
	skMaoEscuridao    = 226
	skCristalizarMana = 435
)

// skillPhysical diz se o golpe de skill do autor usa o ataque físico. Porte de
// GetDamage: classes 0 a 3 (Guerreiro, Templário, Atirador, Dual) são físicas;
// Mago e Clériga usam o ataque e a defesa MÁGICOS quando lançam skill. Antes
// toda skill saía com o ataque físico, e o Mago e a Clériga batiam fraco.
func skillPhysical(s *Session) bool { return stats_MobClassOf(s.Avatar.ClassInfo) <= 3 }

// skillBaseDamage é o dano base de uma skill em jogador: o ataque do autor
// menos a defesa do alvo dividida por 8, com a penetração do tipo certo.
func (h *Handler) skillBaseDamage(s, alvo *Session) int {
	if skillPhysical(s) {
		return h.baseDamage(s, alvo)
	}
	def := int(alvo.Avatar.DefMag)
	if pen := int(s.Avatar.MagPenetration); pen > 0 {
		def -= (def / 100) * pen
	}
	return max(int(s.Avatar.AtkMag)-(def>>3), 1)
}

// skillMobBaseDamage é o mesmo para um mob alvo.
func (h *Handler) skillMobBaseDamage(s *Session, sp *mobdata.Species) int {
	atk, def, pen := int(s.Avatar.AtkFis), int(sp.DefFis), int(s.Avatar.FisPenetration)
	if !skillPhysical(s) {
		atk, def, pen = int(s.Avatar.AtkMag), int(sp.DefMag), int(s.Avatar.MagPenetration)
	}
	if pen > 0 {
		def -= (def / 100) * pen
	}
	return max(atk-(def>>3), 1)
}

// mageDebuffKind devolve o estado que a skill do Mago causa.
func mageDebuffKind(d *gamedata.T_SkillData) int {
	switch int(d.Index) {
	case skChamaCaotica, skInfernoCaotico:
		return stunType
	case skImpedimento:
		return silenceType
	}
	return areaDebuffKind(d)
}

// mageZeroDamage: Sofrimento, Polimorfo e Mão da Escuridão só testam o golpe.
func mageZeroDamage(d *gamedata.T_SkillData) bool {
	if areaZeroDamage(d) {
		return true
	}
	switch int(d.Index) {
	case skDotMagico250, skPolimorfo, skMaoEscuridao:
		return true
	}
	return false
}

// mageAdjust soma ao dano o que a skill do Mago pede. alvoMP é a mana atual do
// alvo jogador (0 para mob): Cristalizar Mana soma Adicional% dela.
func mageAdjust(d *gamedata.T_SkillData, dano int, alvoMP uint32) int {
	if int(d.Index) == skCristalizarMana {
		return dano + int(alvoMP/100)*int(d.Adicional)
	}
	return dano
}

// mageOnHit: Corroer cura o autor em Adicional% do dano; Vínculo FERE o autor
// em Adicional% do dano (RemoveHP sem o piso de 1, como a origem).
func (h *Handler) mageOnHit(s *Session, d *gamedata.T_SkillData, dano int) {
	pct := dano / 100 * int(d.Adicional)
	switch int(d.Index) {
	case skCorroer:
		h.healPlayer(h.connOf(s), s, uint32(max(pct, 0)), 0)
	case skVinculo:
		if pct > 0 && !s.Dead {
			h.applyDamage(s, pct)
			if c := h.connOf(s); c != nil {
				h.sendCurrentHPMP(c, s)
			}
		}
	}
}
