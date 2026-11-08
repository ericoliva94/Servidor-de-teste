package game

import (
	"context"

	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de pilha de itens.
const (
	CodeAgroupItem  = 0x332 // cliente -> servidor: juntar duas pilhas
	CodeUngroupItem = 0x333 // cliente -> servidor: separar parte de uma pilha
)

// agroupItem e o porte de TPacketHandlers.AgroupItem ($332).
//
// A origem juntava dois slots do inventario quando o Index era igual, sem
// olhar mais nada:
//
//   - nao conferia se o item empilha. Dois equipamentos iguais "juntavam": o
//     Refi (o refino, no equipamento) de um era somado ao do outro e o
//     segundo sumia. Refino de graca;
//   - nao conferia src == dest. O mesmo slot nos dois campos dobrava a
//     quantidade e em seguida zerava o slot — o item inteiro desaparecia;
//   - nao conferia a bolsa do slot (so o limite 63), entao dava para mexer em
//     slot sem bolsa;
//   - a soma passava de MAX_SLOT_AMOUNT sem teto.
//
// A pilha junta fica em DEST, como na origem (diferente do arrasto do $70F,
// que deixa em src). Se a soma passa do teto, dest enche e o resto fica em src
// em vez de se perder.
func (h *Handler) agroupItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.AgroupItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.SrcSlot == pkt.DestSlot {
		return nil
	}
	src, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.SrcSlot), true)
	if err != nil {
		return nil
	}
	dest, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.DestSlot), true)
	if err != nil {
		return nil
	}
	if src.IsEmpty() || src.Index != dest.Index || !itemfn.CanAgroup(h.data(), src.Index) {
		return nil
	}

	total := int(src.Refi) + int(dest.Refi)
	if total <= itemfn.MaxSlotAmount {
		dest.Refi = uint16(total)
		*src = model.Item{}
	} else {
		dest.Refi = itemfn.MaxSlotAmount
		src.Refi = uint16(total - itemfn.MaxSlotAmount)
	}

	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.SrcSlot), *src)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.DestSlot), *dest)
	return nil
}

// ungroupItem e o porte de TPacketHandlers.UngroupItem ($333).
//
// So o inventario esta portado: na origem equipamento e armazem eram um
// "Result := true; Exit" (nada acontece) e a pran depende de um sistema que
// ainda nao existe.
//
// A origem recusava pelo campo Expires da tabela, nao por empilhar. Um
// equipamento com Refi 48 e Quantidade 16 gerava uma COPIA do equipamento
// (Move do registro inteiro, mesmo Identific) com Refi 16 e deixava o
// original com 32: duplicacao. Quantidade zero criava um slot com item de
// quantidade 0. Aqui o item precisa empilhar e a quantidade ficar entre 1 e
// Refi-1.
func (h *Handler) ungroupItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.UngroupItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tipo := int(pkt.SlotType)
	switch tipo {
	case store.SlotTypeInventory, store.SlotTypePranInv:
	default:
		return nil
	}

	src, err := h.resolveSlot(c, s, tipo, int(pkt.Slot), true)
	if err != nil || src.IsEmpty() {
		return nil
	}
	t := h.data()
	if !itemfn.CanAgroup(t, src.Index) {
		return nil
	}
	if info := t.Item(int(src.Index)); info == nil || info.Expires != 0 {
		return nil
	}
	if pkt.Quantidade == 0 || pkt.Quantidade >= uint32(src.Refi) {
		return nil
	}

	// O pedaco separado vai para o mesmo lugar de onde saiu: a bolsa da pran
	// (GetEmptyPranSlot) ou o inventario (GetEmptySlot).
	var slot int
	var dest *model.Item
	if tipo == store.SlotTypePranInv {
		i, _ := activePran(s) // resolveSlot ja garantiu a pran
		slot = emptyPranSlot(s.Prans[i])
		if slot >= 0 {
			dest = &s.Prans[i].Inventory[slot]
		}
	} else {
		slot = h.emptyInventorySlot(s)
		if slot >= 0 {
			dest = &s.Inventory.Slots[slot]
		}
	}
	if slot < 0 {
		h.message(c, "Inventario cheio.")
		return nil
	}
	*dest = *src
	dest.Refi = uint16(pkt.Quantidade)
	src.Refi -= uint16(pkt.Quantidade)

	h.sendRefreshItem(c, s, uint8(tipo), uint16(pkt.Slot), *src)
	h.sendRefreshItem(c, s, uint8(tipo), uint16(slot), *dest)
	return nil
}

// emptyInventorySlot e TItemFunctions.GetEmptySlot: o primeiro slot livre de
// 0 a 59 cuja bolsa esta no inventario. Devolve -1 quando nao ha.
func (h *Handler) emptyInventorySlot(s *Session) int {
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if !s.Inventory.Slots[i].IsEmpty() {
			continue
		}
		bolsa := inventoryBagSlot(i)
		if bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		return i
	}
	return -1
}
