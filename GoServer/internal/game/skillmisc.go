package game

import (
	"time"

	"aika/internal/combat"
	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/skillfn"
)

// Skills e buffs que faltavam (SelfBuffSkill/TargetBuffSkill/AttackParse).
const (
	skResoluto      = 8
	skTeleporte     = 72
	skDisparoRapido = 89
	skTeleMassa     = 113
	skLamina        = 150
	skFluxoMana     = 201
	skEfeito5       = 208

	buffEstripador   = 90
	buffPedraDaAlma  = 460
	buffResolutoMark = 6879
	buffEstripadorEf = 6279

	efAddDamage1  = 27
	efSkillATime6 = 122

	// resolutoWindow e quanto o alvo marcado pelo Resoluto continua marcado.
	resolutoWindow = 8 * time.Second
	// teleportDest e o ponto do Teleporte (skill 72).
	teleportDestX, teleportDestY = 3450, 690
)

// addBuffExtra é addBuff com tempo a mais no fim do buff (EF_SKILL_ATIME6 *
// 60 s nas skills 124, 127, 137 e 160, passado a AddBuff como TimeAditional).
func (h *Handler) addBuffExtra(alvo *Session, skill uint16, extra time.Duration) bool {
	if !h.addBuff(alvo, skill) || extra <= 0 {
		return false
	}
	if ini, ok := alvo.Base.Buffs.StartedAt(skill); ok {
		alvo.Base.Buffs.Set(skill, ini.Add(extra))
		h.notifyBuff(alvo, skill)
	}
	return true
}

// hasRelicItem diz se o inventário tem relíquia (item de tipo 40).
func (h *Handler) hasRelicItem(s *Session) bool {
	t := h.data()
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if it := s.Inventory.Slots[i]; !it.IsEmpty() && itemfn.ItemType(t, it.Index) == itemTypeRelic {
			return true
		}
	}
	return false
}

// dropRelics larga no chão as relíquias do inventário (a invisibilidade não
// as carrega). A origem criava um objeto de relíquia por item; aqui sai um
// objeto de item comum na posição do jogador.
func (h *Handler) dropRelics(s *Session) {
	if h.MapObjs == nil {
		return
	}
	t := h.data()
	c := h.connOf(s)
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		it := s.Inventory.Slots[i]
		if it.IsEmpty() || itemfn.ItemType(t, it.Index) != itemTypeRelic {
			continue
		}
		obj := &MapObject{
			Position: s.Position, ContentType: ObjectBoxItem, ItemID: it.Index, Amount: 1,
			CollectTime: dropCollectTime, Face: dropFace, NameID: dropNameID, CreatedAt: h.now(),
		}
		if _, ok := h.MapObjs.Add(obj); !ok {
			continue
		}
		h.broadcastMapObject(obj)
		s.Inventory.Slots[i] = model.Item{}
		h.sendRefreshItem(c, s, 1, uint16(i), model.Item{})
	}
}

// canTeleport são as recusas comuns do Teleporte e do Teleporte em Massa.
func (h *Handler) canTeleport(s *Session, nationRule bool) bool {
	c := h.connOf(s)
	if h.hasRelicItem(s) {
		h.message(c, "Impossivel usar com reliquia.")
		return false
	}
	if nationRule && s.Avatar.Nation > 0 && h.Devir != nil && s.Avatar.Nation != uint16(h.Devir.Nation) {
		h.message(c, "Impossivel usar em outros paises.")
		return false
	}
	return true
}

// selfMiscBuff trata os ramos "em si mesmo" que não eram de uma classe só.
// Devolve true quando tratou.
func (h *Handler) selfMiscBuff(s *Session, skill uint16, d *gamedata.T_SkillData) bool {
	switch int(d.Index) {
	case 124, 127, 137, 160:
		h.addBuffExtra(s, skill, time.Duration(s.Avatar.Ability.get(efSkillATime6)*60)*time.Second)
		if !s.Base.Buffs.Has(skill) {
			h.addBuff(s, skill)
		}
	case skFluxoMana:
		// MaxMP/2 mais 0..N% da mana, N conforme a Inteligência.
		var teto int
		switch i := s.Avatar.Int; {
		case i <= 20:
			teto = 10
		case i <= 40:
			teto = 20
		case i <= 60:
			teto = 30
		case i <= 80:
			teto = 40
		default:
			teto = 50
		}
		mp := s.Avatar.MaxMP/2 + s.Avatar.MaxMP/100*uint32(h.rand().IntN(teto))
		h.healPlayer(h.connOf(s), s, 0, mp)
	case skEfeito5:
		// Parte dos efeitos de 5 peças: Damage 200 devolve 15% do MP (e dá o
		// buff), 300 devolve 15% do HP; outro valor só dá o buff.
		switch d.Damage {
		case 200:
			h.healPlayer(h.connOf(s), s, 0, s.Avatar.MaxMP/100*15)
			h.addBuff(s, skill)
		case 300:
			h.healPlayer(h.connOf(s), s, s.Avatar.MaxHP/100*15, 0)
		default:
			h.addBuff(s, skill)
		}
	case skDisparoRapido:
		s.Base.Cooldowns.Clear()
		h.addBuff(s, skill)
	case skTeleporte:
		if h.canTeleport(s, true) {
			h.teleportTo(s, model.Position{X: teleportDestX, Y: teleportDestY})
		}
	case skTeleMassa:
		if h.canTeleport(s, true) {
			h.teleportTo(s, s.LongSkillPos)
		}
	case skLamina:
		if h.addBuff(s, skill) {
			h.startRegen(s, skill, regenState{castSkill: uint32(max(d.EFV[0], 0))})
		}
	case buffInvAtirador, buffInvDual:
		h.dropRelics(s)
		h.addBuff(s, skill)
	default:
		return false
	}
	return true
}

// targetMiscBuff: os ramos de TargetBuffSkill que usam o tempo extra do autor
// e o Teleporte em Massa (o alvo vai para a posição da skill).
func (h *Handler) targetMiscBuff(caster, alvo *Session, skill uint16, d *gamedata.T_SkillData) bool {
	switch int(d.Index) {
	case 124, 127, 137, 160:
		extra := time.Duration(caster.Avatar.Ability.get(efSkillATime6)*60) * time.Second
		if !h.addBuffExtra(alvo, skill, extra) {
			h.addBuff(alvo, skill)
		}
	case skTeleMassa:
		if h.canTeleport(alvo, true) {
			h.teleportTo(alvo, caster.LongSkillPos)
		}
	default:
		return false
	}
	return true
}

// castPeriodic lança a skill de área de Lâmina em torno do próprio autor.
func (h *Handler) castPeriodic(s *Session, skill uint32) {
	d := skillfn.Data(h.SkillTable, skill)
	if d == nil || s.Dead {
		return
	}
	var pkt packets.SendSkillUse
	pkt.Skill, pkt.Pos = skill, s.Position
	h.skillArea(h.connOf(s), s, pkt, d)
}

// reactiveHit são os buffs do ALVO que reagem a um golpe, na ordem de
// AttackParse: Pedra da Alma (revive uma vez), Requiem (dano a mais),
// Estripador (crítico dispara um buff) e a marca do Resoluto. Devolve o dano.
func (h *Handler) reactiveHit(s, alvo *Session, dano int, tipo combat.DamageType, efeito bool) int {
	if dano > 0 && alvo.Base.Buffs.Has(buffPedraDaAlma) && uint32(dano) > alvo.Avatar.CurHP {
		h.removeBuffAndNotify(alvo, buffPedraDaAlma)
		h.removeDebuffs(alvo, 0)
		alvo.ResolutoPoints = 0
		alvo.Avatar.CurHP = alvo.Avatar.MaxHP / 100 * 30
		alvo.Avatar.CurMP = alvo.Avatar.MaxMP / 100 * 25
		if c := h.connOf(alvo); c != nil {
			h.sendCurrentHPMP(c, alvo)
			h.message(c, "Voce foi revivido gracas ao buff [Pedra da Alma].")
		}
		if s != nil {
			h.message(h.connOf(s), "O seu alvo foi revivido gracas ao buff [Pedra da Alma].")
		}
	}
	if ab := alvo.Avatar.Ability.get(efAddDamage1); ab > 0 {
		dano += ab * 2
	}
	if alvo.Base.Buffs.Has(buffEstripador) && (tipo == combat.Critical || tipo == combat.DoubleCritical) {
		h.addBuff(alvo, buffEstripadorEf)
	}
	if alvo.ResolutoPoints > 0 {
		if h.now().Sub(alvo.ResolutoTime) >= resolutoWindow {
			alvo.ResolutoPoints = 0
		} else if efeito {
			alvo.ResolutoPoints--
			h.addBuff(alvo, buffResolutoMark)
		}
	}
	return dano
}

// markResoluto é o efeito do Resoluto no golpe que acerta: o alvo fica marcado
// por 8 s, com Damage cargas.
func (h *Handler) markResoluto(alvo *Session, d *gamedata.T_SkillData) {
	if int(d.Index) != skResoluto {
		return
	}
	alvo.ResolutoPoints = uint8(min(d.Damage, 255))
	alvo.ResolutoTime = h.now()
	h.addBuff(alvo, buffResolutoMark)
}
