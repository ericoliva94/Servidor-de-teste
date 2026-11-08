package game

import (
	"aika/internal/devir"
	"aika/internal/stats"
)

// Indices EF_RELIQUE_* de GlobalDefs.pas.
const (
	efRelicPerHP    = 146
	efRelicPerMP    = 148
	efRelicPerDmg1  = 149
	efRelicPerDmg2  = 150
	efRelicCritical = 198
	efRelicHit      = 199
	efRelicParry    = 200
	efRelicDouble   = 201
	efRelicStateRes = 202
	efRelicCooltime = 212
	efRelicRunSpeed = 213
)

// RelicEffects e o que as reliquias do templo dao a UM jogador.
type RelicEffects struct {
	PerHP, PerMP, PerDmg1, PerDmg2 int
	Critical, Hit, Parry           int
	StateRes, Cooltime, RunSpeed   int
	Double                         int
}

// relicEffectsFor monta os efeitos para um jogador de certa nacao.
//
// Na origem quase todo efeito so vale se Servers[].NationID = Character.Nation
// (a reliquia e da nacao dona do canal); a unica excecao e o EF_RELIQUE_DOUBLE,
// somado a todos sem conferir a nacao (BaseMob.pas, "Duplo Atk"). Mantido.
// EF_RELIQUE_PER_RESISTANCE1/2 nao entram: na origem o valor e lido e a soma
// esta comentada, entao nao tem efeito.
func relicEffectsFor(reg *devir.Registry, nation uint16) RelicEffects {
	if reg == nil {
		return RelicEffects{}
	}
	fx := RelicEffects{Double: reg.EffectValue(efRelicDouble)}
	if nation == 0 || nation != uint16(reg.Nation) {
		return fx
	}
	fx.PerHP = reg.EffectValue(efRelicPerHP)
	// A origem somava o MP com o efeito de HP (EF_RELIQUE_PER_HP): erro de
	// digitacao. Aqui vale o efeito de MP, como o proprio stats.MaxMP previa.
	fx.PerMP = reg.EffectValue(efRelicPerMP)
	fx.PerDmg1 = reg.EffectValue(efRelicPerDmg1)
	fx.PerDmg2 = reg.EffectValue(efRelicPerDmg2)
	fx.Critical = reg.EffectValue(efRelicCritical)
	fx.Hit = reg.EffectValue(efRelicHit)
	fx.Parry = reg.EffectValue(efRelicParry)
	fx.StateRes = reg.EffectValue(efRelicStateRes)
	fx.Cooltime = reg.EffectValue(efRelicCooltime)
	fx.RunSpeed = reg.EffectValue(efRelicRunSpeed)
	return fx
}

// addTo soma os efeitos ao bonus do calculo de atributos.
func (fx RelicEffects) addTo(b *stats.Bonus) {
	b.RelicAtkFisPercent += fx.PerDmg1
	b.RelicAtkMagPercent += fx.PerDmg2
	b.Critical += fx.Critical
	b.Acerto += fx.Hit
	b.Esquiva += fx.Parry
	b.Resistence += fx.StateRes
	b.Cooldown += fx.Cooltime
	b.SpeedMove += fx.RunSpeed
	b.Double += fx.Double
}

// refreshRelics refaz os atributos de um jogador com as reliquias de agora e
// mantem a vida dentro do novo maximo.
func (h *Handler) refreshRelics(s *Session) {
	s.Avatar.Relic = relicEffectsFor(h.Devir, s.Avatar.Nation)
	s.Avatar.ApplyEquipBonus(h.data(), s.Equip)
}
