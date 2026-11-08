package game

import (
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/store"
)

// Quests de evolucao da pran (NPCHandlers.pas, FinishQuest).
const (
	QuestPranEvolveLv5  = 406 // fada -> crianca, com a pran no nivel 5 (Level = 4)
	QuestPranEvolveLv20 = 407 // crianca -> adolescente, no nivel 20 (Level = 19)
	QuestPranEvolveLv50 = 408 // adolescente -> adulta: vazio na origem
)

// evolvePran e o bloco de evolucao da pran de TNPCHandlers.FinishQuest para as
// quests 406 e 407. Devolve true se a pran evoluiu.
//
// 406: a pran invocada tem de estar no nivel 4 e ser de classe 61, 71 ou 81;
// passa para a classe seguinte, ganha o equipamento inicial da forma nova
// (Equip[0] = 104, Equip[6] = 150) e um nivel na habilidade de transformacao
// (Skills[3]). 407: nivel 19 e classe 62, 72 ou 82; classe seguinte e
// Equip[0] = 105. Nos dois, o item de pran do personagem (Equip[10]) passa a
// ter o indice 104 / 105, o efeito do dono e zerado (so no 406), e a pran e
// reenviada ao cliente.
//
// Quest 408 (nivel 50) e vazia na origem.
//
// Quem chama e o fim da quest, que depende do sistema de NPCs e de quests
// (ver PENDENCIAS.md). A origem so avisava e fazia nada se as condicoes nao
// batiam; aqui o retorno diz o que aconteceu.
func (h *Handler) evolvePran(c *netsrv.Conn, s *Session, quest uint16) bool {
	var (
		nivel    uint8
		classes  [3]uint8
		equip0   uint16
		equip6   uint16
		transfor bool
		efeito0  bool
	)
	switch quest {
	case QuestPranEvolveLv5:
		nivel, classes, equip0, equip6, transfor, efeito0 = 4, [3]uint8{61, 71, 81}, 104, 150, true, true
	case QuestPranEvolveLv20:
		nivel, classes, equip0 = 19, [3]uint8{62, 72, 82}, 105
	default:
		return false
	}
	if s.Equip.Get(pranEquipSlot).Index == 0 {
		h.message(c, "Você deve equipar uma pran para este procedimento.")
		return false
	}
	slot, ok := activePran(s)
	if !ok {
		h.message(c, "Você deve equipar uma pran para este procedimento.")
		return false
	}
	p := s.Prans[slot]
	if p.Level != nivel {
		return false // a quest e abortada, sem aviso
	}
	if p.Class != classes[0] && p.Class != classes[1] && p.Class != classes[2] {
		h.message(c, "Essa pran não pode ser upada de classe.")
		return false
	}

	p.Class++
	p.Equip[0] = model.Item{Index: equip0, APP: equip0}
	if equip6 != 0 {
		p.Equip[6] = model.Item{Index: equip6, APP: equip6}
	}
	if transfor {
		p.Skills[3].Level++
	}
	h.removePranEffects(s)
	h.applyPranEffects(s, p) // o equipamento da forma nova traz efeitos novos

	ficha := &s.Equip.Slots[pranEquipSlot]
	ficha.Index, ficha.APP = equip0, equip0

	if efeito0 {
		h.sendEffect(s, 0)
	}
	h.sendPranToWorld(c, s, slot)
	h.sendPranSpawn(c, s, slot)
	h.sendRefreshItem(c, s, store.SlotTypeEquip, pranEquipSlot, *ficha)
	return true
}
