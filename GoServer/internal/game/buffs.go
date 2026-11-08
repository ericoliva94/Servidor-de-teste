package game

import (
	"time"

	"aika/internal/packets"
	"aika/internal/skillfn"
	"aika/internal/stats"
)

// AbilitySize e o tamanho do acumulador de efeitos (MOB_EF da origem).
const AbilitySize = 400

// Indices EF_* de GlobalDefs.pas que entram nas contas de atributo.
const (
	efDamage1       = 2
	efDamage2       = 3
	efResistance1   = 8
	efResistance2   = 9
	efHP            = 13
	efMP            = 14
	efStr           = 15
	efDex           = 16
	efInt           = 17
	efCon           = 18
	efSpi           = 19
	efResistance6   = 20
	efResistance7   = 21
	efCriticalPower = 31
	efRunSpeed      = 46
	efCooltime      = 48
	efDouble        = 49
	efCritical      = 50
	efParry         = 51
	efHit           = 53
	efPerDamage1    = 54
	efPerDamage2    = 55
	efPerResist1    = 59
	efPerResist2    = 60
	efSkillDamage   = 66
	efStateResist   = 80
	efPiercing1     = 86
	efPiercing2     = 87
	efCritDefence   = 89
	efMarshalPerHP  = 252
	efMarshalPerMP  = 253
	efPranDamage1   = 182
	efPranDamage2   = 183
	efPranResist1   = 186
	efPranResist2   = 187
	efPerHP         = 268
	efPerMP         = 269
)

// Ability e o acumulador de efeitos de buff (MOB_EF). Equipamento e set entram
// por outro caminho (itemfn); aqui so o que vem de buff.
type Ability [AbilitySize]int

func (a *Ability) get(ef int) int {
	if ef < 0 || ef >= AbilitySize {
		return 0
	}
	return a[ef]
}

// increase e IncreasseMobAbility.
func (a *Ability) increase(ef, v int) {
	if ef <= 0 || ef >= AbilitySize {
		return
	}
	a[ef] += v
}

// decrease e DecreasseMobAbility: valor negativo soma o oposto (desfaz um
// debuff); positivo desconta com piso em zero (decInt).
func (a *Ability) decrease(ef, v int) {
	if ef <= 0 || ef >= AbilitySize {
		return
	}
	if v < 0 {
		a[ef] += -v
		return
	}
	a[ef] = max(a[ef]-v, 0)
}

// addTo soma ao bonus de atributos o que os buffs dao. Porte das chamadas
// GetMobAbility(EF_*) de TBaseMob.GetCurrentScore.
func (a *Ability) addTo(b *stats.Bonus) {
	b.Str += a.get(efStr)
	b.Agility += a.get(efDex)
	b.Int += a.get(efInt)
	b.Cons += a.get(efCon)
	b.Luck += a.get(efSpi)

	b.PranAtkFis += a.get(efPranDamage1)
	b.PranAtkMag += a.get(efPranDamage2)
	b.DefFisFlat += a.get(efPranResist1)
	b.DefMagFlat += a.get(efPranResist2)
	b.AtkFisFlat += a.get(efDamage1)
	b.AtkMagFlat += a.get(efDamage2)
	b.AtkFisPercent += a.get(efPerDamage1)
	b.AtkMagPercent += a.get(efPerDamage2)
	b.DefFisFlat += a.get(efResistance1)
	b.DefMagFlat += a.get(efResistance2)
	b.DefFisPercent += a.get(efPerResist1)
	b.DefMagPercent += a.get(efPerResist2)

	b.SpeedMove += a.get(efRunSpeed)
	b.Double += a.get(efDouble)
	b.Critical += a.get(efCritical)
	b.CriticalPower += a.get(efCriticalPower)
	b.FisPenetration += a.get(efPiercing1)
	b.MagPenetration += a.get(efPiercing2)
	b.SkillDamage += a.get(efSkillDamage)
	b.CritRes += a.get(efResistance6)
	b.CritDefence += a.get(efCritDefence)
	b.DuploRes += a.get(efResistance7)
	b.Acerto += a.get(efHit)
	b.Esquiva += a.get(efParry)
	b.Resistence += a.get(efStateResist)
	b.Cooldown += a.get(efCooltime)
}

// percentHP e percentMP somam os percentuais de buff e de marechal, que a
// origem aplicava em passos seguidos sobre o total corrente.
func (a *Ability) percentHP() int { return a.get(efMarshalPerHP) + a.get(efPerHP) }
func (a *Ability) percentMP() int { return a.get(efMarshalPerMP) + a.get(efPerMP) }

// noBuff: indices que AddBuff recusa na origem.
func noBuff(skill uint16) bool { return skill == 7257 || skill == 9133 }

// CodeUpdateBuff e o $16F, o aviso de buff novo.
const CodeUpdateBuff = 0x16F

// buffEnd devolve o instante em que o buff termina.
func (h *Handler) buffEnd(alvo *Session, skill uint16) time.Time {
	inicio, _ := alvo.Base.Buffs.StartedAt(skill)
	return inicio.Add(time.Duration(h.durations().BuffSeconds(skill)) * time.Second)
}

// addBuff e o porte de TBaseMob.AddBuff para jogadores: poe o buff, soma os
// efeitos da tabela (EF/EFV 0..3), refaz os atributos e avisa o cliente
// ($16F do buff novo, $16E com a lista, pontos e status).
//
// Diferencas da origem:
//   - o buff nunca perdia o efeito: ninguem removia os vencidos (ver BuffTick);
//   - recastar um buff ativo refaz o efeito do zero, como a origem (remove e
//     adiciona), em vez de acumula-lo.
func (h *Handler) addBuff(alvo *Session, skill uint16) bool {
	if alvo == nil || alvo.Base.Buffs == nil || noBuff(skill) {
		return false
	}
	// Sem linha na tabela o buff entra sem efeito (so o prazo, que vem de Durations).
	d := skillfn.Data(h.SkillTable, uint32(skill))
	if alvo.Base.Buffs.Has(skill) {
		h.dropBuff(alvo, skill)
	} else if alvo.Base.Buffs.Full() {
		h.message(h.connOf(alvo), "Nao foi possivel adicionar novos buffs. Limite: 60 Buffs.")
		return false
	}

	inicio := h.now()
	// Reliquia da nacao: soma tempo proporcional a duracao (EF_RELIQUE_SKILL_ATIME0).
	if h.Devir != nil && alvo.Avatar.Nation != 0 && alvo.Avatar.Nation == uint16(h.Devir.Nation) {
		extra := h.Devir.EffectValue(175) * h.durations().BuffSeconds(skill) / 100
		inicio = inicio.Add(time.Duration(extra) * time.Second)
	}
	if !alvo.Base.Buffs.Set(skill, inicio) {
		return false
	}
	if d != nil {
		for i := 0; i < 4; i++ {
			alvo.Avatar.Ability.increase(int(d.EF[i]), int(d.EFV[i]))
		}
	}
	alvo.Avatar.ApplyEquipBonus(h.data(), alvo.Equip)
	h.notifyBuff(alvo, skill)
	return true
}

// dropBuff tira o buff e desfaz os efeitos, sem avisar o cliente. Devolve false
// se o buff nao existia.
func (h *Handler) dropBuff(alvo *Session, skill uint16) bool {
	if alvo == nil || !alvo.Base.Buffs.Remove(skill) {
		return false
	}
	delete(alvo.regen, skill)
	delete(alvo.dots, skill)
	if d := skillfn.Data(h.SkillTable, uint32(skill)); d != nil {
		for i := 0; i < 4; i++ {
			alvo.Avatar.Ability.decrease(int(d.EF[i]), int(d.EFV[i]))
		}
	}
	alvo.Avatar.ApplyEquipBonus(h.data(), alvo.Equip)
	if skill == buffContagem || skill == buffMjolnir || skill == buffEclater {
		h.expiryDamage(alvo, skill, skillfn.Data(h.SkillTable, uint32(skill)))
	}
	// Cura Preventiva (134) cura quem a recebeu quando o buff acaba.
	if skill == skCuraPreventiva && !alvo.Dead {
		if d := skillfn.Data(h.SkillTable, uint32(skill)); d != nil {
			h.calcAndCure(alvo, alvo, d)
		}
	}
	return true
}

// removeBuffAndNotify tira o buff, refaz os atributos e avisa o cliente.
func (h *Handler) removeBuffAndNotify(alvo *Session, skill uint16) bool {
	if !h.dropBuff(alvo, skill) {
		return false
	}
	h.refreshBuffState(alvo)
	return true
}

// notifyBuff manda o $16F do buff novo e atualiza lista, pontos e status.
func (h *Handler) notifyBuff(alvo *Session, skill uint16) {
	if c := h.connOf(alvo); c != nil {
		pkt := packets.UpdateBuffPacket{Buff: uint32(skill), EndTime: uint32(max(h.buffEnd(alvo, skill).Unix(), 0))}
		_ = c.SendStruct(CodeUpdateBuff, pkt)
	}
	h.refreshBuffState(alvo)
}

// refreshBuffState reenvia $16E, pontos e status (TBaseMob.SendAddBuff).
func (h *Handler) refreshBuffState(alvo *Session) {
	h.sendRefreshBuffs(alvo)
	if c := h.connOf(alvo); c != nil {
		h.sendRefreshPoint(c, alvo)
		h.sendStatus(c, alvo)
	}
}

// BuffTick remove os buffs vencidos de todo jogador em jogo e devolve quantos
// foram. Porte de TBaseMob.RefreshBuffs, que a origem chamava de uma thread;
// aqui o laco do main chama a cada segundo. Sem isto, buff nao expirava nunca.
func (h *Handler) BuffTick() int {
	if h.Chan == nil {
		return 0
	}
	h.mobDotTick()
	h.duelTick()
	agora := h.now()
	n := 0
	for _, id := range h.Chan.PlayingIDs() {
		s, _, ok := h.Chan.Lookup(id)
		if !ok || s.Base.Buffs == nil {
			continue
		}
		h.regenTick(s)
		h.dotTick(s)
		venceu := s.Base.Buffs.Expired(agora, h.durations())
		if len(venceu) == 0 {
			continue
		}
		for _, skill := range venceu {
			if h.dropBuff(s, skill) {
				n++
			}
		}
		h.refreshBuffState(s)
	}
	return n
}

// clearBuffs tira todos os buffs desfazendo os efeitos (sem avisar o cliente).
// Buffs.Clear sozinho deixaria o acumulador de efeitos com valores de buffs que
// ja nao existem.
func (h *Handler) clearBuffs(s *Session) {
	if s.Base.Buffs == nil {
		return
	}
	for _, id := range s.Base.Buffs.Indexes() {
		h.dropBuff(s, id)
	}
	s.regen = nil
	s.dots = nil
	s.Avatar.Ability = Ability{}
	s.NegarCura = 0
}
