package game

import (
	"fmt"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// Indices de skill do Guerreiro e do Templário (GlobalDefs.pas e literais).
const (
	skAtaquePoderoso  = 1
	skAvancoPoderoso  = 2
	skQuebrarArmadura = 3
	skIncitar         = 4
	skEstocada        = 15
	skFeridaMortal    = 17
	skProficiencia    = 25
	skNemesis         = 27
	skTravarAlvo      = 31
	skProtecao        = 32
	skUniaoDivina     = 35
	skBolha           = 36
	skElTycia         = 39
	skRevitalizar     = 120
	skAtracaoDivina   = 148
	skStigma          = 300
	skPancada         = 336
	skCargaDivina     = 362

	buffProtecao = skProtecao
	buffUniao    = skUniaoDivina
	buffBolha    = skBolha
)

// EF_* da resistência a estados e da União Divina.
const (
	efTransfer      = 136
	efTransferLimit = 137
	efImSilence     = 246
	efImFear        = 247
	efImSkillStun   = 248
)

// Tipos de estado que um golpe pode causar (STUN_TYPE etc.).
const (
	stunType    = 1
	silenceType = 2
)

// dashRange é a distância máxima do avanço (WalkAvanced: "Distance(Pos) > 18").
const dashRange = 18

// debuffKind diz de que tipo é o estado que a skill causa, 0 quando não há
// teste de resistência. Porte dos ValidAttack(DmgType, STUN_TYPE/SILENCE_TYPE).
func debuffKind(d *gamedata.T_SkillData) int {
	switch int(d.Index) {
	case skAvancoPoderoso, skProficiencia, skAtracaoDivina:
		return stunType
	case skFeridaMortal:
		return silenceType
	}
	return gunnerDebuffKind(d)
}

// statusResisted é o teste de resistência de ValidAttack: com taxa
// Resistence/5 + EF_STATE_RESISTANCE (os dois do ATACANTE, como na origem) mais
// a imunidade do alvo, o estado falha se o sorteio de 1 a 254 ficar na taxa.
func (h *Handler) statusResisted(s, alvo *Session, kind int) bool {
	k, ok := statusKinds[kind]
	if !ok {
		return false
	}
	nome := k.nome
	rate := int(s.Avatar.Resistence)/5 + s.Avatar.Ability.get(efStateResist) + alvo.Avatar.Ability.get(k.ef)
	if h.rand().IntN(254)+1 > rate {
		return false
	}
	h.message(h.connOf(s), fmt.Sprintf("[%s] resistiu a sua habilidade de %s.", alvo.Avatar.Name, nome))
	h.message(h.connOf(alvo), fmt.Sprintf("Voce resistiu a habilidade de %s de [%s].", nome, s.Avatar.Name))
	return true
}

// bubbleBlocks é o buff 36 (Bolha): o golpe de skill que acerta é anulado e
// gasta pontos (Retorno de Magia gasta Damage pontos). Ao zerar o buff sai.
// Porte do trecho de AttackParse; vale só para skill, não para golpe básico.
func (h *Handler) bubbleBlocks(s, alvo *Session, d *gamedata.T_SkillData) bool {
	if !alvo.Base.Buffs.Has(buffBolha) {
		return false
	}
	gasto := uint8(1)
	if int(d.Index) == skRetornoMagica {
		gasto = uint8(min(d.Damage, 255))
	}
	if alvo.BolhaPoints > gasto {
		alvo.BolhaPoints -= gasto
	} else {
		alvo.BolhaPoints = 0
	}
	h.message(h.connOf(s), "["+alvo.Avatar.Name+"] resistiu a sua habilidade de ataque.")
	if alvo.BolhaPoints == 0 {
		h.removeBuffAndNotify(alvo, buffBolha)
		h.message(h.connOf(alvo), "Voce resistiu ao ataque de ["+s.Avatar.Name+"]. Protecao desativada.")
	} else {
		h.message(h.connOf(alvo), fmt.Sprintf("Voce resistiu ao ataque de [%s], restam %d ticks.",
			s.Avatar.Name, alvo.BolhaPoints))
	}
	return true
}

// warriorAdjust é o ajuste de dano de WarriorSkill/TemplarSkill: a Pancada soma
// Adicional% da vida atual do autor. dano 0 e o tipo do golpe ficam com o
// chamador (Estocada).
func warriorAdjust(s *Session, d *gamedata.T_SkillData, dano int) int {
	if int(d.Index) == skPancada {
		return dano + int(s.Avatar.CurHP)/100*int(d.Adicional)
	}
	return dano
}

// zeroDamage: Estocada só testa o golpe (Dano := 0).
func zeroDamage(d *gamedata.T_SkillData) bool {
	return noDamageSkill(d) || int(d.Index) == skEstocada || gunnerZeroDamage(d)
}

// isDash: skills em que o autor avança sobre o alvo.
func isDash(d *gamedata.T_SkillData) bool {
	return int(d.Index) == skAvancoPoderoso || int(d.Index) == skCargaDivina
}

// dashTo é o porte de TBaseMob.WalkAvanced: o autor vai até a posição do alvo,
// se ela estiver a 18 ou menos, e a tela de todos é avisada.
func (h *Handler) dashTo(s *Session, pos model.Position, skill uint32) {
	if !ValidPosition(pos) || s.Position.Distance(pos) > dashRange {
		return
	}
	s.Position = pos
	s.LastMoved = h.now()
	var pkt packets.MovementPacket
	pkt.Header.Index = s.ClientID
	pkt.Destination.X, pkt.Destination.Y = pos.X, pos.Y
	pkt.Speed = 125
	pkt.Unk = skill
	if raw, err := netsrv.Marshal(CodeMovement, pkt); err == nil && h.Chan != nil {
		_ = h.Chan.SendTo(s.ClientID, raw)
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
	s.AnchorPosition = s.Position
	h.refreshVisible(s)
	h.refreshVisibleMobs(s)
}

// divineUnion é o buff 35: parte do dano recebido (EF_TRANSFER %) vai para
// quem lançou, até o limite de EF_TRANSFER_LIMIT. Devolve o dano que sobra.
// Se o lançador saiu, morreu ou não está mais no jogo, o buff cai.
func (h *Handler) divineUnion(alvo *Session, dano int) int {
	if dano <= 0 || !alvo.Base.Buffs.Has(buffUniao) || alvo.UniaoDivina == "" {
		return dano
	}
	parte := dano / 100 * alvo.Avatar.Ability.get(efTransfer)
	parte = max(min(parte, dano), 0)
	dano -= parte

	var outro *Session
	if h.Chan != nil {
		if id, ok := h.Chan.FindByCharacter(alvo.UniaoDivina); ok {
			if o, _, ok := h.Chan.Lookup(id); ok && o.Status >= Playing && !o.Dead {
				outro = o
			}
		}
	}
	if outro == nil {
		alvo.UniaoDivina = ""
		h.removeBuffAndNotify(alvo, buffUniao)
		return dano
	}
	h.loseHPKeepOne(outro, uint32(parte))
	h.message(h.connOf(outro), fmt.Sprintf(
		"Seu HP foi consumido em %d pontos pelo buff [Uniao Divina] no membro <%s>.", parte, alvo.Avatar.Name))
	alvo.Avatar.Ability.decrease(efTransferLimit, parte)
	if alvo.Avatar.Ability.get(efTransferLimit) == 0 {
		alvo.UniaoDivina = ""
		h.removeBuffAndNotify(alvo, buffUniao)
	}
	return dano
}

// selfWarriorBuff trata os buffs "em si mesmo" do Guerreiro e do Templário.
// Devolve true quando tratou. Proteção e Bolha guardam os pontos ANTES do buff,
// como a origem; Revitalizar e o buff 42 regeneram vida enquanto durarem.
func (h *Handler) selfWarriorBuff(s *Session, skill uint16, d *gamedata.T_SkillData) bool {
	switch int(d.Index) {
	case skProtecao:
		s.DefesaPoints = 3
		h.addBuff(s, skill)
	case skBolha:
		s.BolhaPoints = uint8(max(min(d.EFV[0], 255), 0))
		h.addBuff(s, skill)
	case 42: // Égide do Templário: HPRAction 2, EFV[2]/2 de vida por segundo
		if h.addBuff(s, skill) {
			h.startRegen(s, skill, regenState{hp: int(d.EFV[2]) / 2})
		}
	case skRevitalizar:
		if h.addBuff(s, skill) {
			h.startRegen(s, skill, regenState{hpPctMax: 1})
		}
	default:
		return false
	}
	return true
}

// elTycia (39): o alvo recupera toda a vida e o autor perde toda a mana.
func (h *Handler) elTycia(caster, alvo *Session) {
	if alvo.Dead {
		return
	}
	caster.Avatar.CurMP = 0
	alvo.Avatar.CurHP = alvo.Avatar.MaxHP
	if c := h.connOf(caster); c != nil {
		h.sendCurrentHPMP(c, caster)
	}
	if c := h.connOf(alvo); c != nil && alvo != caster {
		h.sendCurrentHPMP(c, alvo)
	}
}
