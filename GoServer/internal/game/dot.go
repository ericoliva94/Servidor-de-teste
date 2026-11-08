package game

import (
	"sync"
	"time"

	"aika/internal/combat"
	"aika/internal/gamedata"
)

// Skills de dano contínuo da origem (SKDListener em TargetBuffSkill).
const (
	skSacrificio    = 15 // "estocada wr"
	skVeneno55      = 55
	skVenenoLentid  = 79
	skEspinhoVen    = 74
	skDotMagico250  = 250
	efAcceleration1 = 285

	// Buffs que ferem o dono quando acabam (RemoveBuffEffect).
	buffContagem = 49
	buffMjolnir  = 73
	buffEclater  = 108
)

// dotState é o dano contínuo que um buff mantém num jogador: dmg a cada
// segundo, até o buff acabar. Substitui SKDListener/SKDAction/SKDCycles da
// origem, que guardava UM único dano contínuo por AUTOR (lançar o segundo
// apagava o primeiro) e o fazia correr numa thread à parte do buff.
type dotState struct {
	caster uint16
	dmg    int
}

type mobDotKey struct{ mob, skill uint16 }

type mobDot struct {
	caster uint16
	dmg    int
	until  time.Time
}

// dotRegistry guarda o dano contínuo em mobs, que não têm lista de buffs.
type dotRegistry struct {
	mu   sync.Mutex
	mobs map[mobDotKey]mobDot
}

// isDot diz se a skill é de dano contínuo.
func isDot(d *gamedata.T_SkillData) bool {
	switch int(d.Index) {
	case skSacrificio, skVeneno55, skVenenoLentid, skEspinhoVen, skDotMagico250, skRaioSolar, skPecadosMortais:
		return true
	}
	return false
}

// dotPerTick calcula o dano por segundo. Porte do trecho comum dos ramos
// 15/55/79/74/250/133 de TargetBuffSkill: o jogador alvo sorteia numa faixa que
// depende de ele estar acima de 80% da vida; o mob sorteia entre metade e todo o
// ataque do autor (físico ou mágico, conforme a skill).
func (h *Handler) dotPerTick(caster *Session, d *gamedata.T_SkillData, alvoPlayer *Session) int {
	rnd := h.rand()
	between := func(lo, hi int) int { return lo + rnd.IntN(max(hi-lo, 1)) }
	idx := int(d.Index)

	var h2 int
	if alvoPlayer != nil {
		acima := alvoPlayer.Avatar.CurHP >= alvoPlayer.Avatar.MaxHP/100*80
		switch idx {
		case skSacrificio:
			h2 = pick(acima, between(30, 60), between(15, 30))
		case skRaioSolar:
			h2 = pick(acima, between(30, 59), between(10, 29))
		default: // 55, 79, 74, 250
			h2 = pick(acima, between(60, 120), between(30, 59))
		}
	} else {
		atk := int(caster.Avatar.AtkFis)
		if idx == skDotMagico250 || idx == skRaioSolar {
			atk = int(caster.Avatar.AtkMag)
		}
		h2 = between(atk/2, atk+1)
	}

	efv := int(d.EFV[0])
	if idx == skPecadosMortais {
		return efv // SKDSkillEtc1 := EFV[0], sem sorteio
	}
	switch idx {
	case skVeneno55, skVenenoLentid, skEspinhoVen:
		if d.Duration > 0 {
			return (efv + h2) / int(d.Duration)
		}
	case skRaioSolar:
		return efv/2 + h2
	}
	return efv + h2
}

func pick(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}

// applyHitEffect é o que o golpe que acertou faz depois do dano: dano contínuo
// nas skills que o têm, ou o buff/debuff comum da tabela.
func (h *Handler) applyHitEffect(caster, alvo *Session, skill uint16, d *gamedata.T_SkillData) {
	if !isDot(d) {
		h.addBuff(alvo, skill)
		return
	}
	if !h.addBuff(alvo, skill) {
		return
	}
	if alvo.dots == nil {
		alvo.dots = map[uint16]dotState{}
	}
	alvo.dots[skill] = dotState{caster: caster.ClientID, dmg: h.dotPerTick(caster, d, alvo)}
}

// applyMobDot liga o dano contínuo num mob por Duration segundos. Recastar a
// mesma skill no mesmo mob renova (não acumula).
func (h *Handler) applyMobDot(caster *Session, m *Mob, d *gamedata.T_SkillData) {
	dmg := h.dotPerTick(caster, d, nil)
	if dmg <= 0 || d.Duration == 0 {
		return
	}
	h.dots.mu.Lock()
	defer h.dots.mu.Unlock()
	if h.dots.mobs == nil {
		h.dots.mobs = map[mobDotKey]mobDot{}
	}
	h.dots.mobs[mobDotKey{m.ClientID, uint16(d.Index)}] = mobDot{
		caster: caster.ClientID, dmg: dmg,
		until: h.now().Add(time.Duration(d.Duration) * time.Second),
	}
}

// dotTick aplica um segundo de dano contínuo num jogador. O dano nunca mata
// (RemoveHP com StayOneHP, como na origem).
func (h *Handler) dotTick(s *Session) {
	if s.Dead || len(s.dots) == 0 {
		return
	}
	for _, st := range s.dots {
		h.loseHPKeepOne(s, uint32(max(st.dmg, 0)))
	}
}

// mobDotTick aplica um segundo de dano contínuo em todos os mobs afetados.
func (h *Handler) mobDotTick() {
	if h.Mobs == nil {
		return
	}
	agora := h.now()
	h.dots.mu.Lock()
	var lote []struct {
		k mobDotKey
		v mobDot
	}
	for k, v := range h.dots.mobs {
		if !agora.Before(v.until) {
			delete(h.dots.mobs, k)
			continue
		}
		lote = append(lote, struct {
			k mobDotKey
			v mobDot
		}{k, v})
	}
	h.dots.mu.Unlock()

	for _, e := range lote {
		m, ok := h.Mobs.Get(e.k.mob)
		if !ok || m.Dead {
			h.dots.mu.Lock()
			delete(h.dots.mobs, e.k)
			h.dots.mu.Unlock()
			continue
		}
		autor, _, ok := h.Chan.Lookup(e.v.caster)
		if !ok {
			continue
		}
		if h.Mobs.HitKeepOne(m, e.v.dmg, e.v.caster, agora) {
			h.broadcastMobDamage(autor, m, combat.Normal, e.v.dmg)
		}
	}
}

// expiryDamage é o ferimento de quem tem um destes buffs quando ele acaba
// (contagem regressiva, Mjolnir, Eclater). Não mata.
func (h *Handler) expiryDamage(alvo *Session, skill uint16, d *gamedata.T_SkillData) {
	if alvo.Dead || d == nil {
		return
	}
	var base int
	switch skill {
	case buffContagem, buffMjolnir:
		base = 15 + h.rand().IntN(75) + int(d.EFV[0])
	case buffEclater:
		base = 15 + h.rand().IntN(75) + int(d.EFV[0])
		if alvo.Avatar.Ability.get(efAcceleration1) > 0 {
			base += int(d.Damage)
		}
	default:
		return
	}
	h.loseHPKeepOne(alvo, uint32(base))
}
