package game

import (
	"fmt"
	"math"

	"aika/internal/mobdata"
	"aika/internal/netsrv"
)

// mobKillRange e a distancia, em celulas, entre o mob morto e o jogador para
// ele dividir a experiencia do grupo (InRange(.., 25) de TBaseMob.MobKilled).
const mobKillRange = 25

// mobExpForLevel porta o "case HelperX" de TBaseMob.MobKilled: a experiencia
// do mob depende da diferenca entre o nivel dele e o do jogador.
//
//	<= -8  (cinza)    1
//	-7..-3 (azul)     0,5x
//	-2..2  (amarelo)  1x
//	3..5   (laranja)  1,5x
//	>= 6   (roxo)     0,2x
//
// O Round do Delphi arredonda para o par em meio exato (banker's rounding),
// o que math.RoundToEven repete.
func mobExpForLevel(base uint32, mobLevel, charLevel uint16) int64 {
	d := int(mobLevel) - int(charLevel)
	b := float64(base)
	switch {
	case d <= -8:
		return 1
	case d <= -3:
		return int64(math.RoundToEven(b * 0.5))
	case d <= 2:
		return int64(base)
	case d <= 5:
		return int64(math.RoundToEven(b * 1.5))
	default:
		return int64(math.RoundToEven(b * 0.2))
	}
}

// relicExpBonus soma ao exp do mob o percentual da reliquia, como
// "MobExp + (MobExp div 100) * ReliqEffect[EF_RELIQUE_PER_EXP]".
func relicExpBonus(exp int64, perExp int) int64 {
	if perExp <= 0 {
		return exp
	}
	return exp + (exp/100)*int64(perExp)
}

// onMobDeath e o porte de TBaseMob.MobKilled: exp e drop do mob morto.
//
// Quem mata (s) e o ponto de partida. Sem grupo, leva a exp e o drop. Com
// grupo, cada membro perto do mob roda a mesma rotina como membro: a exp
// segue Party.ExpAlocate e o item Party.ItemAlocate. Se ninguem do grupo
// pegou exp, quem bateu primeiro (de fora do grupo) recebe como solo.
//
// Diferencas da origem, de proposito:
//   - O bonus da reliquia entra uma vez so. A origem somava em MobKilled e
//     de novo dentro de AddExp (EXP_TYPE_MOB), pagando o dobro.
//   - Raide (mob com mais de 999.999 de HP) nao esta portada: cai no ramo de
//     grupo comum.
//   - Quests de "matar mob" nao existem no Go ainda (ver PENDENCIAS.md).
func (h *Handler) onMobDeath(c *netsrv.Conn, s *Session, m *Mob, sp *mobdata.Species) {
	h.log().Debug("mob morto", "mob", sp.Name, "id", m.ClientID, "por", s.Avatar.Name)
	defer h.broadcastMobRemove(m)

	g := h.Parties.Of(s.ClientID)
	if g == nil {
		h.mobKilledFor(c, s, m, sp, nil, 0)
		return
	}

	dono := h.partyItemRecipient(g, m)
	deuExp := 0
	for _, id := range h.Parties.MembersOf(g) {
		mb, conn, ok := h.Chan.Lookup(id)
		if !ok || mb.Status < Playing || mb.Dead {
			continue
		}
		if h.mobKilledFor(conn, mb, m, sp, g, dono) {
			deuExp++
		}
	}
	if deuExp > 0 || m.FirstAttackerID == 0 || m.FirstAttackerID == s.ClientID {
		return
	}
	// Quem bateu primeiro nao esta no grupo de quem matou.
	if fa, conn, ok := h.Chan.Lookup(m.FirstAttackerID); ok && fa.Status >= Playing && !fa.Dead {
		h.mobKilledFor(conn, fa, m, sp, nil, 0)
	}
}

// mobKilledFor da a UM jogador a parte dele na morte do mob. g e o grupo
// quando ele recebe como membro (InParty), nil quando recebe como solo; dono
// e quem leva o item do mob (partyItemRecipient).
// Devolve true se o jogador ganhou experiencia.
func (h *Handler) mobKilledFor(c *netsrv.Conn, s *Session, m *Mob, sp *mobdata.Species, g *Party, dono uint16) bool {
	if _, ok := h.freeInventorySlot(s); !ok {
		h.message(c, "Seu inventário está cheio. Recompensas não serão recebidas.")
		return false
	}

	exp := mobExpForLevel(sp.Exp, sp.Level, s.Avatar.Level)
	if h.nationOwnsChannel(s) {
		exp = relicExpBonus(exp, s.Avatar.Relic.PerExp)
	}

	if g == nil {
		h.giveMobExp(c, s, exp)
		h.dropFromMob(c, s, m, sp.DropIndex)
		return true
	}

	// Em grupo, so divide quem esta perto do mob.
	if !m.Position.InRange(s.Position, mobKillRange) {
		return false
	}
	gotExp := false
	switch g.ExpAllocate {
	case ExpEqual:
		// Cada membro divide pelos que estao a vista DELE, como na origem.
		j := 0
		for _, id := range h.Parties.MembersOf(g) {
			if o, _, ok := h.Chan.Lookup(id); ok && s.Position.InRange(o.Position, h.watchDistance()) {
				j++
			}
		}
		j = max(j, 1)
		exp = max(exp/int64(j), 1)
		h.giveMobExp(c, s, exp)
		gotExp = true
	case ExpIndividual:
		if m.FirstAttackerID == 0 || m.FirstAttackerID == s.ClientID {
			h.giveMobExp(c, s, exp)
			gotExp = true
		}
	}

	if dono == s.ClientID {
		h.dropFromMob(c, s, m, sp.DropIndex)
	}
	return gotExp
}

// partyItemRecipient escolhe UM membro para levar o item do mob, segundo
// Party.ItemAlocate. Zero quando ninguem leva.
//
// A origem decidia dentro do laco de membros, e no modo "em ordem" cada
// membro relia e avancava o contador (LastSlotItemReceived): no mesmo abate o
// membro 0 levava o item, o contador ia a 1 e o membro 1 tambem levava, e
// assim por diante: todos ganhavam, quando a ordem existe para revezar. Aqui
// a decisao e uma por abate.
func (h *Handler) partyItemRecipient(g *Party, m *Mob) uint16 {
	membros := h.Parties.MembersOf(g)
	if len(membros) == 0 {
		return 0
	}
	switch g.ItemAllocate {
	case ItemInOrder:
		idx := int(g.LastItemSlot)
		if idx >= len(membros) {
			idx = 0
		}
		g.LastItemSlot = uint8(idx + 1)
		if idx >= len(membros)-1 {
			g.LastItemSlot = 0
		}
		return membros[idx]
	case ItemRandom:
		return h.randomMember(membros)
	case ItemIndividual:
		fa, _, ok := h.Chan.Lookup(m.FirstAttackerID)
		if m.FirstAttackerID != 0 && ok && fa.Status >= Playing && g.Has(m.FirstAttackerID) {
			return m.FirstAttackerID
		}
		return 0
	case ItemLeader:
		return g.Leader
	}
	return 0
}

func (h *Handler) randomMember(membros []uint16) uint16 {
	return membros[h.rand().IntN(len(membros))]
}

// nationOwnsChannel diz se a nacao do jogador e a dona do canal: so entao a
// reliquia dobra a exp (Servers[].NationID = Character.Nation).
func (h *Handler) nationOwnsChannel(s *Session) bool {
	return s.Avatar.Nation != 0 && s.Avatar.Relic.PerExp > 0
}

// giveMobExp soma a exp, passa uma parte para a pran e avisa o jogador.
func (h *Handler) giveMobExp(c *netsrv.Conn, s *Session, exp int64) {
	ganho := h.addExp(c, s, exp, ExpTypeMob)
	if ganho == 0 {
		return
	}
	pran := h.awardPranExp(c, s, ganho)
	if _, ok := activePran(s); ok {
		h.message(c, fmt.Sprintf("Adquiriu %d exp, Pran %d.", ganho, pran))
		return
	}
	h.message(c, fmt.Sprintf("Adquiriu %d exp.", ganho))
}
