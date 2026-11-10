package game

import (
	"context"
	"fmt"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/questdata"
	"aika/internal/store"
)

// Codigos de quest.
const (
	CodeAbandonQuest = 0x32F // cliente -> servidor: abandonar (e eco da resposta)
	CodeQuestInfo    = 0x331 // servidor -> cliente: progresso de uma quest
)

// QuestStore guarda as quests do personagem. Nil deixa as quests so em memoria.
type QuestStore interface {
	Load(ctx context.Context, charID uint32) ([]model.QuestProgress, error)
	Save(ctx context.Context, charID uint32, list []model.QuestProgress) error
}

// loadQuests le as quests do personagem que entra no mundo.
func (h *Handler) loadQuests(ctx context.Context, s *Session, charID uint32) {
	s.Quests = nil
	if h.QuestDB == nil || charID == 0 {
		return
	}
	list, err := h.QuestDB.Load(ctx, charID)
	if err != nil {
		h.log().Error("carregar quests", "usuario", s.Username, "personagem", charID, "erro", err)
		return
	}
	s.Quests = list
}

// saveQuests grava as quests do personagem em jogo.
func (h *Handler) saveQuests(s *Session) {
	if h.QuestDB == nil || s.SelectedSlot < 0 || s.SelectedSlot >= store.MaxCharacterSlots {
		return
	}
	charID := s.Characters[s.SelectedSlot].ID
	if charID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := h.QuestDB.Save(ctx, charID, s.Quests); err != nil {
		h.log().Error("gravar quests", "usuario", s.Username, "personagem", charID, "erro", err)
	}
}

// questSlot acha a quest na lista do jogador, ou -1 (QuestExists).
func questSlot(s *Session, id uint32) int {
	for i, q := range s.Quests {
		if q.QuestID == id {
			return i
		}
	}
	return -1
}

// startQuest poe uma quest na lista do jogador. Falha se o id nao existe na
// tabela, se o jogador ja a tem ou se a lista (50) esta cheia. A origem
// (SearchEmptyQuestIndex) devolvia 255 com a lista cheia e o chamador seguia
// escrevendo; aqui a recusa e explicita. Ainda sem caminho por pacote: a quest
// e aceita num dialogo de NPC.
func (h *Handler) startQuest(c *netsrv.Conn, s *Session, id uint32) bool {
	def := h.QuestTable.Get(id)
	if def == nil || questSlot(s, id) >= 0 || len(s.Quests) >= questdata.MaxPlayerQuests {
		return false
	}
	agora := h.now().Unix()
	s.Quests = append(s.Quests, model.QuestProgress{QuestID: id, UpdatedAt: agora, CreatedAt: agora})
	h.sendQuestInfo(c, s, id)
	return true
}

// sendQuestInfo e TPlayer.UpdateQuest: manda o $331 com o progresso. Sem a
// parte de redesenhar o NPC da quest, que ainda nao existe.
func (h *Handler) sendQuestInfo(c *netsrv.Conn, s *Session, id uint32) {
	i := questSlot(s, id)
	def := h.QuestTable.Get(id)
	if c == nil || i < 0 || def == nil {
		return
	}
	var pkt packets.SendQuestInfo
	pkt.Header.Index = s.ClientID
	pkt.QuestID = uint16(min(id, 65535))
	for j := 0; j < 5; j++ {
		pkt.RequirimentType[j] = def.RequirementsType[j]
		pkt.RequirimentComplete[j] = s.Quests[i].Complete[j]
		pkt.RequirimentAmount[j] = uint8(min(def.RequirementsAmount[j], 255))
		pkt.RequirimentItem[j] = uint16(min(def.Requirements[j], 65535))
	}
	if def.Requirements[0] == 0 {
		pkt.IsCompleted = 1
	}
	if err := c.SendStruct(CodeQuestInfo, pkt); err != nil {
		h.log().Debug("enviar quest", "usuario", s.Username, "erro", err)
	}
}

// questOnKill porta o bloco de quests de TBaseMob.MobKilled: cada quest em
// andamento com requisito de "matar mob" (tipo 1) daquele mob soma 1 ao
// progresso; ao bater a quantidade, o jogador e avisado.
//
// A origem incrementava um BYTE sem teto e so o travava na quantidade ao
// atingi-la; como a quest continua na lista, matar mais um mob a incrementava
// de novo e depois de 255 o contador dava a volta. Aqui uma quest que ja bateu
// a quantidade nao conta mais.
func (h *Handler) questOnKill(c *netsrv.Conn, s *Session, mobIntName uint16) {
	if h.QuestTable == nil {
		return
	}
	for i := range s.Quests {
		q := &s.Quests[i]
		if q.QuestID == 0 || q.Done {
			continue
		}
		def := h.QuestTable.Get(q.QuestID)
		if def == nil {
			continue
		}
		mudou := false
		for j := 0; j < 5; j++ {
			if def.RequirementsType[j] != questdata.ReqKillMob || def.Requirements[j] != uint32(mobIntName) {
				continue
			}
			amount := uint8(min(def.RequirementsAmount[j], 255))
			if q.Complete[j] >= amount {
				continue
			}
			q.Complete[j]++
			mudou = true
			if q.Complete[j] >= amount {
				h.message(c, fmt.Sprintf("Você completou a quest [%s]", h.questTitle(q.QuestID)))
			}
		}
		if mudou {
			q.UpdatedAt = h.now().Unix()
			h.sendQuestInfo(c, s, q.QuestID)
		}
	}
}

// questTitle e o titulo da quest em Quest.bin (Quests[QuestID].Titulo).
func (h *Handler) questTitle(id uint32) string {
	if int(id) >= len(h.QuestList) {
		return ""
	}
	return netsrv.CString(h.QuestList[id].Titulo[:])
}

// abandonQuest porta TPacketHandlers.AbandonQuest ($32F). Quest com marca 11 e
// dada como concluida; as demais, se nao concluidas, saem da lista. O pedido e
// ecoado ao cliente nos dois casos. A origem tambem redesenhava o NPC da quest,
// sem NPCs no Go isto fica de fora (e com ele a checagem de distancia, que so
// decidia se o NPC era redesenhado).
func (h *Handler) abandonQuest(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.AbandonQuestPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	i := questSlot(s, pkt.QuestID)
	if i < 0 {
		return nil
	}
	q := &s.Quests[i]
	if def := h.QuestTable.Get(q.QuestID); def != nil && def.QuestMark == questdata.MarkAutoDone {
		q.Done = true
		q.UpdatedAt = h.now().Unix()
		return c.SendStruct(CodeAbandonQuest, pkt)
	}
	if q.Done {
		return nil
	}
	s.Quests = append(s.Quests[:i], s.Quests[i+1:]...)
	return c.SendStruct(CodeAbandonQuest, pkt)
}
