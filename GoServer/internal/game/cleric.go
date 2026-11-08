package game

import (
	"fmt"
	"slices"

	"aika/internal/gamedata"
	"aika/internal/skillfn"
)

// EF_* usados pela cura (GlobalDefs.pas).
const (
	efDamage6        = 7
	efDecure         = 154
	efAnticure       = 165
	efSkillDamage6   = 214
	efUpcure         = 234
	efPerCurePrepare = 361
)

// Indices de skill da Clériga e afins (GlobalDefs.pas e os literais de
// TargetBuffSkill).
const (
	skRemediar        = 26
	skFlecha          = 121
	skCura            = 122
	skCuraPreventiva  = 134
	skMaoDeCura       = 125
	skRessurreicao    = 126
	skLibertacaoMana  = 128
	skCuraMassaDef    = 131
	skRaioSolar       = 133
	skRetornoMagica   = 136
	skRecuperacao     = 138
	skSensorMagico    = 139
	skLimpar          = 140
	skAureaEsplendor  = 162
	skGloriaExcelis   = 167
	skTodosDebuffs    = 337
	skFloresGloria    = 458
	skCuraConcentrada = 459

	// buffAnticure e o buff 88, que acaba quando a contagem de anticura zera.
	buffAnticure = 88
	// Buffs de invisibilidade que o Sensor Magico revela.
	buffInvAtirador = 53
	buffInvDual     = 77
)

// regenState e a regeneracao que um buff mantem: HP e/ou MP por segundo, ate o
// buff acabar. Substitui HPRListener/HPRAction/HPRCycles da origem (uma thread
// varria todos os jogadores a cada segundo); preso ao buff, ele nao pode ficar
// ligado depois de o buff sair.
type regenState struct {
	hp, mp int
	// hpPctMax regenera esse percentual da vida maxima por segundo (Revitalizar).
	hpPctMax int
	// castSkill lanca essa skill de area em torno do autor a cada segundo (Lamina).
	castSkill uint32
	// anticure diz se a cura periodica passa pela regra de anticura (Mao de Cura).
	anticure bool
}

// cureValue e o miolo de CalcAndCure/CalcCure2: o valor curado.
//
// base entra antes dos bonus; skillDamage e o Damage da skill; prepare liga o
// EF_PER_CURE_PREPARE (so CalcAndCure o aplica).
func (h *Handler) cureValue(caster, alvo *Session, base, skillDamage uint32, prepare bool) uint32 {
	pos := func(v int) uint32 { return uint32(max(v, 0)) }
	cure := uint32(caster.Avatar.AtkMag)/2 + base + skillDamage
	cure += cure / 40 * pos(caster.Avatar.Ability.get(efDamage6))
	cure += pos(caster.Avatar.Ability.get(efSkillDamage6))
	if caster != alvo {
		cure += cure / 40 * pos(alvo.Avatar.Ability.get(efDamage6))
		cure += pos(alvo.Avatar.Ability.get(efSkillDamage6))
	}
	cure += cure / 100 * uint32(alvo.Avatar.CureTax)
	cure += cure / 100 * pos(alvo.Avatar.Ability.get(efUpcure))
	if prepare {
		cure += cure / 100 * pos(alvo.Avatar.Ability.get(efPerCurePrepare))
	}
	cure += uint32(h.rand().IntN(279)+20)/2 + 35 // RandomRange(20, 299) div 2 + 35
	cure -= min(cure, cure/100*pos(alvo.Avatar.Ability.get(efDecure)))
	return cure
}

// antiCure aplica a regra de EF_ANTICURE: em vez de curar, o alvo perde vida,
// por ate tres curas seguidas; ao zerar a contagem sai o buff 88. Devolve true
// quando a cura foi trocada por dano.
func (h *Handler) antiCure(alvo *Session, cure uint32) bool {
	ac := alvo.Avatar.Ability.get(efAnticure)
	if ac <= 0 {
		return false
	}
	if alvo.NegarCura == 0 {
		alvo.NegarCura = 3
	}
	h.loseHPKeepOne(alvo, cure/100*uint32(ac))
	alvo.NegarCura--
	if alvo.NegarCura == 0 {
		h.removeBuffAndNotify(alvo, buffAnticure)
	}
	return true
}

// loseHPKeepOne desconta vida sem matar (RemoveHP com StayOneHP).
func (h *Handler) loseHPKeepOne(s *Session, v uint32) {
	if v >= s.Avatar.CurHP {
		s.Avatar.CurHP = 1
	} else {
		s.Avatar.CurHP -= v
	}
	if c := h.connOf(s); c != nil {
		h.sendCurrentHPMP(c, s)
	}
}

// calcAndCure e o porte de TBaseMob.CalcAndCure: cura o alvo, avisa quem foi
// curado. Alvo morto nao e curado (so a Ressurreicao o levanta).
func (h *Handler) calcAndCure(caster, alvo *Session, d *gamedata.T_SkillData) {
	if alvo.Dead {
		return
	}
	cure := h.cureValue(caster, alvo, 0, d.Damage, true)
	if h.antiCure(alvo, cure) {
		return
	}
	c := h.connOf(alvo)
	h.healPlayer(c, alvo, cure, 0)
	if caster == alvo {
		h.message(c, fmt.Sprintf("Seu HP foi restaurado em %d", cure))
	} else {
		h.message(c, fmt.Sprintf("Seu HP foi restaurado em %d por [%s].", cure, caster.Avatar.Name))
	}
}

// removeDebuffs tira ate quant debuffs (BuffDebuff 3 ou 4), todos se quant <= 0.
// Porte de RemoveDebuffs/RemoveAllDebuffs; a origem percorria o dicionario sem
// ordem, aqui a ordem e a do indice para o resultado ser repetivel.
func (h *Handler) removeDebuffs(alvo *Session, quant int) int {
	return h.removeByKind(alvo, quant, func(k uint32) bool { return k == 3 || k == 4 })
}

// removeBuffs tira ate quant buffs (BuffDebuff 1). Porte de RemoveBuffs.
func (h *Handler) removeBuffs(alvo *Session, quant int) int {
	if quant <= 0 {
		return 0
	}
	return h.removeByKind(alvo, quant, func(k uint32) bool { return k == 1 })
}

func (h *Handler) removeByKind(alvo *Session, quant int, ok func(uint32) bool) int {
	if alvo == nil || alvo.Base.Buffs == nil {
		return 0
	}
	ids := alvo.Base.Buffs.Indexes()
	slices.Sort(ids)
	n := 0
	for _, id := range ids {
		if quant > 0 && n >= quant {
			break
		}
		d := skillfn.Data(h.SkillTable, uint32(id))
		if d == nil || !ok(d.BuffDebuff) {
			continue
		}
		if h.dropBuff(alvo, id) {
			n++
		}
	}
	if n > 0 {
		h.refreshBuffState(alvo)
		if c := h.connOf(alvo); c != nil {
			h.sendCurrentHPMP(c, alvo)
		}
	}
	return n
}

// startRegen liga a regeneracao presa ao buff skill.
func (h *Handler) startRegen(alvo *Session, skill uint16, r regenState) {
	if alvo.regen == nil {
		alvo.regen = map[uint16]regenState{}
	}
	alvo.regen[skill] = r
}

// regenTick aplica um segundo de regeneracao a um jogador.
func (h *Handler) regenTick(s *Session) {
	if s.Dead || len(s.regen) == 0 {
		return
	}
	c := h.connOf(s)
	for _, r := range s.regen {
		if r.castSkill != 0 {
			h.castPeriodic(s, r.castSkill)
		}
		hp := uint32(max(r.hp, 0)) + s.Avatar.MaxHP/100*uint32(max(r.hpPctMax, 0))
		if r.anticure && hp > 0 && h.antiCure(s, hp) {
			continue
		}
		h.healPlayer(c, s, hp, uint32(max(r.mp, 0)))
	}
}

// resurrect e o ramo RESSUREICAO: levanta um membro morto do mesmo grupo com
// Damage% da vida. Raid (grupos aliados) nao esta portada.
func (h *Handler) resurrect(caster, alvo *Session, d *gamedata.T_SkillData) {
	if !alvo.Dead || h.Parties == nil {
		return
	}
	g := h.Parties.Of(caster.ClientID)
	if g == nil || g != h.Parties.Of(alvo.ClientID) {
		return
	}
	alvo.Dead = false
	alvo.Avatar.CurHP = max(alvo.Avatar.MaxHP/100*d.Damage, 1)
	h.sendEffect(alvo, 1)
	h.respawnToVisible(alvo)
	if c := h.connOf(alvo); c != nil {
		h.sendCurrentHPMP(c, alvo)
		h.message(c, "Voce foi ressuscitado pelo jogador "+caster.Avatar.Name+".")
	}
}

// targetBuffSkill e o porte de TBaseMob.TargetBuffSkill/SelfBuffSkill para o que
// a Clériga usa: cura, limpeza, regeneracao e ressurreicao. O resto cai no
// buff generico (EF/EFV da tabela). self escolhe a variante "em si mesmo"
// (SkillData.TargetType = 1), que na origem e um procedimento a parte e so
// trata alguns indices.
func (h *Handler) targetBuffSkill(caster, alvo *Session, skill uint16, d *gamedata.T_SkillData, self bool) {
	idx := int(d.Index)
	if self {
		if h.selfWarriorBuff(caster, skill, d) {
			return
		}
		if h.selfMiscBuff(caster, skill, d) {
			return
		}
		switch idx {
		case skTodosDebuffs:
			h.removeDebuffs(caster, 0)
		case skCuraMassaDef:
			h.calcAndCure(caster, caster, d)
			h.addBuff(caster, skill)
		case 457: // Agua Benta: cria o item de Damage no inventario
			if !h.putItem(h.connOf(caster), caster, h.newStackItem(uint16(min(d.Damage, 65535)), 1)) {
				h.message(h.connOf(caster), "Inventario cheio.")
			}
		case skLibertacaoMana:
			if h.addBuff(caster, skill) {
				h.startRegen(caster, skill, regenState{mp: int(caster.Avatar.AtkMag>>3) + int(d.EFV[0])})
			}
		default:
			h.addBuff(alvo, skill)
		}
		return
	}
	if h.targetMiscBuff(caster, alvo, skill, d) {
		return
	}
	switch idx {
	case skCura, skFloresGloria, skRemediar:
		h.calcAndCure(caster, alvo, d)
	case skCuraMassaDef, skAureaEsplendor:
		h.calcAndCure(caster, alvo, d)
		h.addBuff(alvo, skill)
	case skRecuperacao:
		h.calcAndCure(caster, alvo, d)
		h.removeDebuffs(alvo, 0)
	case skGloriaExcelis:
		h.removeDebuffs(alvo, 1)
		h.calcAndCure(caster, alvo, d)
		h.addBuff(alvo, skill)
	case skCuraConcentrada:
		if !alvo.Dead {
			// CalcCure2: sem EF_PER_CURE_PREPARE; o Damage entra como base.
			cure := h.cureValue(caster, alvo, d.Damage, 0, false)
			if !h.antiCure(alvo, cure) {
				h.healPlayer(h.connOf(alvo), alvo, cure, 0)
			}
		}
	case skLimpar:
		h.removeDebuffs(alvo, 1)
	case skTodosDebuffs:
		h.removeDebuffs(caster, 0)
	case skElTycia:
		h.elTycia(caster, alvo)
	case skUniaoDivina:
		alvo.UniaoDivina = caster.Avatar.Name
		h.addBuff(alvo, skill)
	case skRessurreicao:
		h.resurrect(caster, alvo, d)
	case skMaoDeCura:
		if h.addBuff(alvo, skill) {
			per := int(h.cureValue(caster, alvo, uint32(max(d.EFV[0], 0)), d.Damage, false))
			if dur := int(d.Duration); dur > 0 {
				per = (per + dur/2) / dur // Round(cura / Duration)
			}
			h.startRegen(alvo, skill, regenState{hp: per, anticure: true})
		}
	case skLibertacaoMana:
		if h.addBuff(alvo, skill) {
			mp := int(h.cureValue(caster, alvo, uint32(max(d.EFV[0], 0)), d.Damage, false)) + int(d.EFV[0])
			h.startRegen(alvo, skill, regenState{mp: mp})
		}
	default:
		h.applyHitEffect(caster, alvo, skill, d)
	}
}

// clericAdjust e o ajuste de dano de ClericSkill/ClericAreaSkill depois do
// calculo comum. Flecha Sagrada soma Adicional por cento; Retorno de Magia soma
// Adicional.
func clericAdjust(d *gamedata.T_SkillData, dano int) int {
	switch int(d.Index) {
	case skFlecha:
		return dano + dano/100*int(d.Adicional)
	case skRetornoMagica:
		return dano + int(d.Adicional)
	}
	return dano
}

// noDamageSkill: Sensor Magico e Raio Solar nao causam dano (Dano := 0); o que
// importa e o tipo do golpe, que decide se o efeito entra.
func noDamageSkill(d *gamedata.T_SkillData) bool {
	return int(d.Index) == skSensorMagico || int(d.Index) == skRaioSolar
}

// clericOnHit e o efeito em quem foi atingido: Retorno de Magia tira buffs do
// alvo; Sensor Magico revela quem esta invisivel.
func (h *Handler) clericOnHit(alvo *Session, d *gamedata.T_SkillData) {
	switch int(d.Index) {
	case skRetornoMagica:
		h.removeBuffs(alvo, int(d.Damage))
	case skSensorMagico:
		h.removeBuffAndNotify(alvo, buffInvAtirador)
		h.removeBuffAndNotify(alvo, buffInvDual)
	}
}
