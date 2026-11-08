package game

import (
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// safeZone e o ponto que a origem protege de skill em area (2947, 1664), com
// raio 10.
var safeZone = model.Position{X: 2947, Y: 1664}

const safeZoneRadius = 10

// areaRadius e o raio do efeito em area: Trunc(Range * 1.5) na origem.
func areaRadius(d *gamedata.T_SkillData) uint16 {
	return uint16(min(int(float64(d.Range)*1.5), 65535))
}

// skillCenter e o centro da area. A origem confiava na posicao do pacote
// (LastPositionLongSkill); aqui ela so vale se estiver ao alcance de quem
// lanca, senao a area cai no proprio autor. Posicao zerada tambem.
func (h *Handler) skillCenter(s *Session, pos model.Position, d *gamedata.T_SkillData) model.Position {
	if (pos.X == 0 && pos.Y == 0) || !s.Position.InRange(pos, h.skillRange(d)+AttackRange) {
		return s.Position
	}
	return pos
}

// canHitPlayer aplica as regras de aliado de AreaSkill: zona segura, mesma
// guilda, mesmo grupo e mesma nacao (a menos que o autor esteja de PK). Duelo
// e alianca de guildas nao estao portados.
func (h *Handler) canHitPlayer(s, o *Session) bool {
	if o == s || o.Status < Playing || o.Dead {
		return false
	}
	if safeZone.InRange(o.Position, safeZoneRadius) {
		return false
	}
	if s.Avatar.GuildIndex > 0 && s.Avatar.GuildIndex == o.Avatar.GuildIndex && !s.Duel.active {
		return false // mesma guilda, a menos que estejam duelando
	}
	if h.Parties != nil {
		if g := h.Parties.Of(s.ClientID); g != nil && g == h.Parties.Of(o.ClientID) {
			return false
		}
	}
	if s.Avatar.Nation == o.Avatar.Nation && !s.Avatar.PlayerKill && !s.Duel.active {
		return false
	}
	// Duelando, so o oponente apanha da area, e so depois da espera.
	if s.Duel.active && (o.ClientID != s.Duel.with || h.duelWaiting(s)) {
		return false
	}
	return true
}

// skillArea e o porte de TBaseMob.AreaSkill para o que e generico: acerta
// jogadores e mobs ao redor do centro, um golpe por alvo, com o mesmo calculo
// da skill de alvo unico, e aplica o efeito de duracao nos jogadores atingidos.
//
// Nao portado (ver KNOWN_ISSUES.md): os efeitos proprios de cada skill das seis
// classes (WarriorAreaSkill ... ClericAreaSkill), dungeon, duelo, guardas e
// pedras de devir. Skills que dependem deles saem com o dano comum.
func (h *Handler) skillArea(c *netsrv.Conn, s *Session, pkt packets.SendSkillUse, d *gamedata.T_SkillData) {
	h.consumeBullets(c, s)
	centro := h.skillCenter(s, pkt.Pos, d)
	raio := areaRadius(d)
	limite := int(d.MaxTargets)
	if limite <= 0 {
		limite = 1 << 30
	}
	n := 0

	if h.Chan != nil {
		for _, id := range s.Base.VisiblePlayers.IDs() {
			if n >= limite {
				break
			}
			o, _, ok := h.Chan.Lookup(id)
			if !ok || !centro.InRange(o.Position, raio) || !h.canHitPlayer(s, o) {
				continue
			}
			n++
			h.skillHitPlayer(s, o, pkt.Skill, d)
		}
	}
	if h.Mobs != nil {
		for _, m := range h.Mobs.Near(centro, raio) {
			if n >= limite {
				break
			}
			sp := h.Mobs.SpeciesOf(m)
			if sp == nil || m.Dead {
				continue
			}
			n++
			h.skillHitMob(c, s, m, sp, d)
		}
	}
}

// skillAreaBuff e o porte de TBaseMob.AreaBuff: sem grupo, o efeito vai so no
// autor; com grupo, no autor e nos membros que estao a vista e dentro do raio.
func (h *Handler) skillAreaBuff(c *netsrv.Conn, s *Session, pkt packets.SendSkillUse, d *gamedata.T_SkillData) {
	skill := uint16(pkt.Skill)
	var grupo *Party
	if h.Parties != nil {
		grupo = h.Parties.Of(s.ClientID)
	}
	if grupo == nil || h.Chan == nil {
		// Sem grupo o efeito vai so no autor (SelfBuffSkill).
		h.targetBuffSkill(s, s, skill, d, true)
		return
	}
	h.targetBuffSkill(s, s, skill, d, false)
	raio := areaRadius(d)
	for _, id := range s.Base.VisiblePlayers.IDs() {
		o, _, ok := h.Chan.Lookup(id)
		if !ok || o.Status < Playing || o.Dead || h.Parties.Of(id) != grupo {
			continue
		}
		if !s.Position.InRange(o.Position, raio) {
			continue
		}
		h.targetBuffSkill(s, o, skill, d, false)
	}
}
