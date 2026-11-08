package game

import (
	"context"
	"fmt"
	"math"

	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/stats"
	"aika/internal/store"
)

// Codigos de alteracao de item.
const (
	CodeChangeItemAttribute = 0x32D // cliente -> servidor
	CodeChangeItemResponse  = 0x32E // servidor -> cliente
)

// ChangeType do $32D, as constantes CHANGE_* de GlobalDefs.pas.
const (
	ChangeReinforce     = 0
	ChangeEnchants      = 1
	ChangeApp           = 2
	ChangeMountEnchants = 11
	ChangePranEnchants  = 13
)

const (
	noSlot               = 0xFFFFFFFF // "sem terceiro item"
	changeAppCost        = 500        // ouro da troca de aparencia
	athlonItem           = 4580       // material da troca de aparencia
	maxRefiBeforeBlocked = 175        // Refi a partir do qual o refino e recusado
	msgWrong             = "Algo de errado nao esta certo."
)

// Resultados do refino, os valores de GetResultRefineItem enviados ao cliente.
const (
	refineBreak   = 0 // o item quebra
	refineDown    = 1 // perde um nivel
	refineSuccess = 2
	refineBlocked = 3 // extrato enriquecido: nada muda ("Item esta refinando!")
)

// refineChances e ChancesOfRefinament de GlobalDefs.pas (+0 a +15).
var refineChances = [16]int{100, 100, 100, 70, 50, 40, 30, 20, 10, 5, 2, 1, 1, 1, 1, 0}

// reinforceCosts e o que a tabela de dados precisa dar para o custo do refino.
type reinforceCosts interface {
	ReinforceCost(weapon bool, useEffect int) (uint32, bool)
}

// changeItemAttribute e o porte de TPacketHandlers.ChangeItemAttribute ($32D).
//
// Portados: refino (CHANGE_REINFORCE) e troca de aparencia (CHANGE_APP).
// Encantamento, encantamento de montaria e de pran dependem de
// EnchantItem/EnchantMount, ainda nao portados: o pedido e recusado com aviso
// e nada e consumido.
func (h *Handler) changeItemAttribute(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.ChangeItemAttPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	switch pkt.ChangeType {
	case ChangeReinforce:
		h.reinforceItem(c, s, pkt)
	case ChangeApp:
		h.changeAppearance(c, s, pkt)
	case ChangeEnchants, ChangeMountEnchants, ChangePranEnchants:
		h.message(c, "Encantamento ainda nao esta disponivel neste servidor.")
	}
	return nil
}

func (h *Handler) sendChangeResponse(c *netsrv.Conn, s *Session, result, changeType uint32) {
	pkt := packets.ReinforceResponse{Unk1: changeType, ReinforceResult: result}
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(CodeChangeItemResponse, pkt)
}

// reinforceItem e TItemFunctions.ReinforceItem mais o fecho do handler.
//
// Defeitos da origem corrigidos:
//   - os tres slots vinham do cliente como DWORD e indexavam o inventario sem
//     faixa nem bolsa (leitura e escrita fora do vetor);
//   - nada exigia que o alvo fosse equipamento: um empilhavel (pocao) passava,
//     e "Inc(Refi, $10)" somava 16 unidades por refino com sucesso;
//   - o material podia ser o proprio item (slots repetidos);
//   - o custo vem de ReinforceCust, e linha ausente na tabela valia zero:
//     refino de graca. Aqui linha ausente recusa.
//
// Mantido: o material e o extrato sao consumidos antes do sorteio, e o
// sorteio e multiplicado pelo Rank do item.
func (h *Handler) reinforceItem(c *netsrv.Conn, s *Session, pkt packets.ChangeItemAttPacket) {
	t := h.data()
	fail := func() { h.message(c, msgWrong) }

	if pkt.ItemSlot == pkt.Item2 || pkt.ItemSlot == pkt.Item3 || pkt.Item2 == pkt.Item3 {
		fail()
		return
	}
	item, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.ItemSlot), true)
	if err != nil || item.IsEmpty() {
		fail()
		return
	}
	mat, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.Item2), true)
	if err != nil || mat.IsEmpty() {
		fail()
		return
	}
	extractSlot := -1
	var extractRank uint8
	var extractType uint16
	if pkt.Item3 != noSlot {
		ex, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.Item3), true)
		if err != nil || ex.IsEmpty() || ex.Refi == 0 || !itemfn.CanAgroup(t, ex.Index) {
			fail()
			return
		}
		einfo := t.Item(int(ex.Index))
		if einfo == nil {
			fail()
			return
		}
		extractSlot = int(pkt.Item3)
		extractRank, extractType = einfo.Rank, einfo.ItemType
	}

	info, minfo := t.Item(int(item.Index)), t.Item(int(mat.Index))
	if info == nil || minfo == nil || itemfn.CanAgroup(t, item.Index) || !itemfn.CanAgroup(t, mat.Index) {
		fail()
		return
	}
	equip := itemfn.EquipSlot(t, item.Index)
	if equip < 2 || equip > 7 {
		fail()
		return
	}
	if minfo.Rank < info.Rank || (extractSlot >= 0 && extractRank < info.Rank) {
		fail()
		return
	}

	var cust int64
	if info.UseEffect > 0 {
		rc, ok := t.(reinforceCosts)
		if !ok {
			fail()
			return
		}
		c32, ok := rc.ReinforceCost(equip == 6, int(info.UseEffect))
		if !ok {
			fail()
			return
		}
		cust = int64(c32)
	}
	if cust > s.Avatar.Gold {
		h.message(c, "Voce nao possui gold suficiente.")
		return
	}
	if item.Refi >= maxRefiBeforeBlocked {
		h.message(c, "Refinacao maxima permitida: +11")
		return
	}
	if info.Fortification != 0 {
		h.message(c, "Esse item nao pode ser refinado.")
		return
	}
	if mat.Refi == 0 {
		fail()
		return
	}

	// Tudo conferido: consome e sorteia.
	if extractSlot >= 0 {
		itemfn.DecreaseAmount(&s.Inventory.Slots[extractSlot], 1)
	}
	itemfn.DecreaseAmount(mat, 1)
	s.Avatar.Gold -= cust

	refine := int(item.Refi) / 16
	result := refineOutcome(h.rand(), int(info.Rank), extractType, extractSlot >= 0, refine)

	nome := itemfn.ItemName(t, item.Index)
	switch result {
	case refineBreak:
		*item = model.Item{}
	case refineDown:
		if item.Refi >= 16 {
			item.Refi -= 16
		}
	case refineSuccess:
		item.Refi += 16
	case refineBlocked:
		h.message(c, "Item esta refinando!")
	}

	if result == refineSuccess {
		if n := int(math.RoundToEven(float64(item.Refi) / 16)); n >= 9 {
			h.broadcastMessage(fmt.Sprintf("%s refinou com sucesso %s +%d", s.Avatar.Name, nome, n))
		}
	}

	h.sendRefreshMoney(c, s)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.ItemSlot), *item)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.Item2), *mat)
	if extractSlot >= 0 {
		h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(extractSlot), s.Inventory.Slots[extractSlot])
	}
	h.sendChangeResponse(c, s, uint32(result), ChangeReinforce)
}

// refineOutcome e TItemFunctions.GetResultRefineItem.
//
// A origem tinha Randomize dentro da funcao (reiniciava o gerador a cada
// refino); aqui vale o gerador da biblioteca. rank multiplica o sorteio:
// quanto maior o Rank, menor a chance. O resultado 1 e o padrao quando o
// extrato nao e de um dos tipos 63 a 66.
func refineOutcome(r interface{ IntN(int) int }, rank int, extractType uint16, hasExtract bool, refine int) uint8 {
	var chance int
	switch {
	case refine <= 0:
		chance = refineChances[0]
	case refine-1 < len(refineChances):
		chance = refineChances[refine-1]
	default:
		chance = refineChances[len(refineChances)-1]
	}
	roll := 1 + r.IntN(100)
	if rank > 0 {
		roll *= rank
	}
	if roll <= chance {
		return refineSuccess
	}
	switch {
	case refine <= 3:
		return refineSuccess // 100%: impossivel voltar
	case refine <= 5:
		if !hasExtract {
			return refineDown
		}
	case refine <= 12:
		if !hasExtract {
			return refineBreak
		}
	default:
		return refineDown
	}
	switch extractType {
	case 64, 66: // extrato enriquecido
		return refineBlocked
	}
	return refineDown // 63, 65 (normal) e qualquer outro tipo
}

// changeAppearance e ChangeApp mais o fecho do handler (CHANGE_APP).
//
// Defeito da origem corrigido: ChangeApp ja tinha aplicado a aparencia,
// zerado o item de aparencia e gasto o Athlon quando o handler conferia os
// 500 de ouro; sem ouro o jogador perdia os dois itens e ainda ficava com a
// aparencia. O ouro agora e conferido antes de qualquer mudanca. Os slots
// tambem eram usados sem faixa, e o mesmo slot em dois campos apagava o item.
func (h *Handler) changeAppearance(c *netsrv.Conn, s *Session, pkt packets.ChangeItemAttPacket) {
	t := h.data()
	if pkt.ItemSlot == pkt.Item2 {
		h.message(c, msgWrong)
		return
	}
	item, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.ItemSlot), true)
	if err != nil || item.IsEmpty() {
		h.message(c, msgWrong)
		return
	}
	novo, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.Item2), true)
	if err != nil || novo.IsEmpty() {
		h.message(c, msgWrong)
		return
	}

	athlonSlot := -1
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if s.Inventory.Slots[i].Index != athlonItem {
			continue
		}
		if bolsa := inventoryBagSlot(i); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		athlonSlot = i
		break
	}
	if athlonSlot < 0 {
		h.message(c, "Voce nao possui Athlon!")
		return
	}
	if athlonSlot == int(pkt.ItemSlot) || athlonSlot == int(pkt.Item2) {
		h.message(c, msgWrong)
		return
	}

	info, ninfo := t.Item(int(item.Index)), t.Item(int(novo.Index))
	if info == nil || ninfo == nil {
		h.message(c, msgWrong)
		return
	}
	if appearanceClass(s, ninfo.Classe) != appearanceClass(s, info.Classe) ||
		itemfn.CanAgroup(t, item.Index) || itemfn.CanAgroup(t, novo.Index) {
		h.message(c, "Este item nao pode ser colocado como aparencia!")
		return
	}
	// Changeable: sem aparencia ainda, ou aparencia igual ao proprio item.
	if item.APP != 0 && item.APP != item.Index {
		h.message(c, msgWrong)
		return
	}
	if s.Avatar.Gold < changeAppCost {
		h.message(c, "Voce nao tem o gold necessario!")
		return
	}

	item.APP = novo.Index
	*novo = model.Item{}
	itemfn.DecreaseAmount(&s.Inventory.Slots[athlonSlot], 1)
	s.Avatar.Gold -= changeAppCost

	h.sendRefreshMoney(c, s)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.ItemSlot), *item)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.Item2), *novo)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(athlonSlot), s.Inventory.Slots[athlonSlot])
	h.sendChangeResponse(c, s, 2, ChangeApp)
}

// appearanceClass e GetMobClass(ItemList[].Classe): classe zero (item de
// qualquer classe) vira a do proprio jogador.
func appearanceClass(s *Session, classe uint16) int {
	if classe == 0 {
		classe = s.Avatar.ClassInfo
	}
	return stats.MobClass(classe)
}

// broadcastMessage manda uma mensagem a todos em jogo (SendServerMsg, tipo 16).
func (h *Handler) broadcastMessage(msg string) {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		if _, conn, ok := h.Chan.Lookup(id); ok {
			h.message(conn, msg)
		}
	}
}
