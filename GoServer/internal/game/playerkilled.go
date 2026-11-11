package game

import (
	"fmt"

	"aika/internal/model"
	"aika/internal/netsrv"
)

// Constantes de PvP de GlobalDefs.pas e de PlayerKilled.
const (
	honorPerKill     = 120   // HONOR_PER_KILL
	skullMultiplier  = 1     // SKULL_MULTIPLIER
	skullItem        = 11285 // caveira dada a quem abate
	pvpMinLevel      = 25    // so ha PvP/honra entre jogadores de nivel 25 ou mais
	pvpTitleID       = 27    // titulo de abates
	buffLasting      = 126   // "Efeito Duradouro": sem PvP/honra
	buffAfterDeath   = 6471  // buff que a vitima ganha ao morrer
	relicFace        = 320
	relicNameID      = 914
	relicCollectTime = 10
)

// playerKilled porta TBaseMob.PlayerKilled: o que acontece depois que um
// jogador mata outro. autor e quem matou; morto, a vitima.
//
//  1. A vitima larga todas as reliquias no chao, perto de quem matou.
//  2. Com "Efeito Duradouro" na vitima ou no autor, nao ha PvP nem honra.
//  3. A vitima ganha o buff 6471.
//  4. Abaixo do nivel 25 (qualquer dos dois) nao ha PvP.
//  5. Quem matou (e o grupo dele, ate DISTANCE_TO_WATCH) ganha ponto de PvP,
//     honra, uma caveira e progresso no titulo 27.
//
// Diferencas da origem:
//   - O progresso do titulo dos OUTROS membros do grupo usava o nivel do
//     titulo de quem matou (Players[Self].Titles[j].Level); aqui cada um usa o
//     seu.
//   - A copia da origem aplicava a mensagem de honra por membro; mantida.
func (h *Handler) playerKilled(autor, morto *Session) {
	if autor == nil || morto == nil || autor == morto {
		return
	}
	h.dropAllRelics(autor, morto)

	if h.hasBuffWithSkillIndex(morto, buffLasting) {
		h.message(h.connOf(autor), "Alvo está sob Efeito Duradouro. Impossível receber PvP/Honra.")
		return
	}
	if !morto.Siege.InCastle { // no castelo a vitima nao ganha o buff (InClastleVerus)
		h.addBuff(morto, buffAfterDeath)
	}

	if h.hasBuffWithSkillIndex(autor, buffLasting) {
		h.message(h.connOf(autor), "Você está sob Efeito Duradouro. Impossível receber PvP/Honra.")
		return
	}
	if morto.Avatar.Level < pvpMinLevel || autor.Avatar.Level < pvpMinLevel {
		h.message(h.connOf(autor), "Você só pode receber PvP de alvos acima do Nv 25.")
		return
	}

	recebem := []*Session{autor}
	if g := h.Parties.Of(autor.ClientID); g != nil {
		recebem = recebem[:0]
		for _, id := range h.Parties.MembersOf(g) {
			mb, _, ok := h.Chan.Lookup(id)
			if !ok {
				continue
			}
			if id != autor.ClientID && !mb.Position.InRange(autor.Position, h.watchDistance()) {
				continue
			}
			recebem = append(recebem, mb)
		}
	}
	for _, s := range recebem {
		h.rewardKill(s)
	}
}

// rewardKill da a UM jogador o premio do abate.
func (h *Handler) rewardKill(s *Session) {
	c := h.connOf(s)
	s.Avatar.KillPoint++
	h.message(c, "Seus pontos de PvP foram incrementados.")
	h.giveHonor(c, s, honorPerKill) // soma, avisa e manda o $12A com os abates
	h.putItem(c, s, h.newStackItem(skullItem, skullMultiplier))
	h.killTitleProgress(c, s)
}

// killTitleProgress soma 1 ao titulo 27 e sobe o nivel ao bater a meta; sem o
// titulo, ele nasce no nivel 0.
func (h *Handler) killTitleProgress(c *netsrv.Conn, s *Session) {
	slot := titleSlot(s, pvpTitleID)
	if slot < 0 {
		h.addTitle(c, s, pvpTitleID, 0, false)
		return
	}
	t := &s.Titles[slot]
	t.Progress++
	lv, ok := h.TitleTable.Level(pvpTitleID, int(t.Level)+1)
	if ok && t.Progress >= lv.Goal && int(t.Level) < 4 {
		h.updateTitleLevel(c, s, pvpTitleID, int(t.Level)+1, true)
		h.message(c, fmt.Sprintf("Seu título [%s] foi atualizado.", lv.Name))
		return
	}
	h.updateTitleLevel(c, s, pvpTitleID, int(t.Level), false)
}

// dropAllRelics larga no chao, perto de quem matou, toda reliquia (ItemType 40)
// que a vitima carrega, e depois manda o efeito 0 (a vitima deixa de brilhar).
func (h *Handler) dropAllRelics(autor, morto *Session) {
	c := h.connOf(morto)
	soltou := false
	for {
		slot := h.findByItemType(morto, itemTypeRelic)
		if slot < 0 {
			break
		}
		it := morto.Inventory.Slots[slot]
		if h.MapObjs != nil {
			obj := &MapObject{
				Position:    h.neighborOf(autor.Position),
				ContentType: ObjectReliquare,
				ItemID:      it.Index,
				Amount:      1,
				CollectTime: relicCollectTime,
				Face:        relicFace,
				NameID:      relicNameID,
				CreatedAt:   h.now(),
			}
			if _, ok := h.MapObjs.Add(obj); ok {
				h.broadcastMapObject(obj)
			} else {
				h.message(h.connOf(autor), "Erro ao criar o objeto no mapa. Abra um ticket no suporte.")
			}
		}
		morto.Inventory.Slots[slot] = model.Item{}
		h.sendRefreshItem(c, morto, 1, uint16(slot), model.Item{})
		soltou = true
	}
	if soltou {
		h.sendEffect(morto, 0)
	}
}

// neighborOf escolhe uma das oito celulas ao redor (RandomRange(1, 7) em
// Neighbors da origem).
func (h *Handler) neighborOf(p model.Position) model.Position {
	dx := h.rand().IntN(3) - 1
	dy := h.rand().IntN(3) - 1
	if dx == 0 && dy == 0 {
		dx = 1
	}
	p.X += float32(dx)
	p.Y += float32(dy)
	return p
}
