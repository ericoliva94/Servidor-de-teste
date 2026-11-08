package game

import "aika/internal/gamedata"

// Indices das skills em área de cada classe (GlobalDefs.pas). Os RAMOS de dano
// são os mesmos da skill de alvo único; o que cada uma acrescenta é o estado
// que causa e, em poucas, o fato de só testar o golpe.
const (
	skAreaImpacto      = 12
	skCancaoGuerra     = 13
	skInvestidaMortal  = 22
	skIncitarMultidao  = 28
	skEmissaoDivina    = 37
	skDetonacao        = 59
	skRajadaSonica     = 62
	skExplosaoRadiante = 87
	skEclater          = 108
	skEsplendorCaotico = 110
	skBruma            = 111
	skQuedaNegra       = 114
	skProeminencia     = 118
	skGritoMedo        = 145
	skPecadosMortais   = 159
	skTempestadeRaios  = 166
	skTempestade       = 436
	skFuracaoNegro     = 437
)

// fearType é FEAR_TYPE; efImFear, a imunidade a medo.
const fearType = 3

// areaDebuffKind devolve o estado que a skill em área causa, 0 se nenhum.
func areaDebuffKind(d *gamedata.T_SkillData) int {
	switch int(d.Index) {
	case skCancaoGuerra, skExplosaoRadiante, skBruma, skQuedaNegra,
		skProeminencia, skTempestade, skFuracaoNegro:
		return stunType
	case skAreaImpacto, skInvestidaMortal, skEsplendorCaotico,
		skPecadosMortais, skTempestadeRaios:
		return lentType
	case skEmissaoDivina, skDetonacao, skRajadaSonica:
		return silenceType
	case skGritoMedo:
		return fearType
	}
	return 0
}

// areaZeroDamage: skills em área que só testam o golpe (Dano := 0).
func areaZeroDamage(d *gamedata.T_SkillData) bool {
	switch int(d.Index) {
	case skGritoMedo, skIncitarMultidao, skBruma, skEclater:
		return true
	}
	return false
}

// areaOnHit: Incitar Multidão revela quem está invisível (buffs 53 e 77).
func (h *Handler) areaOnHit(alvo *Session, d *gamedata.T_SkillData) {
	if alvo != nil && int(d.Index) == skIncitarMultidao {
		h.removeBuffAndNotify(alvo, buffInvAtirador)
		h.removeBuffAndNotify(alvo, buffInvDual)
	}
}
