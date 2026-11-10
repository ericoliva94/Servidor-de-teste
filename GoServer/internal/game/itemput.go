package game

import (
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/store"
)

// putItem poe um item no inventario, empilhando quando da. Porte de
// TItemFunctions.PutItem (a versao com TItem).
//
// Item que empilha completa as pilhas do mesmo Index antes de abrir um slot
// novo, e uma quantidade acima do teto do slot (1000) vira varias pilhas. Item
// que nao empilha ocupa um slot livre (so contam os slots cuja bolsa esta no
// inventario). Devolve false, sem mexer em nada, quando nao ha espaco: a
// origem gravava mesmo assim em Inventory[255] quando GetEmptySlot nao
// achava slot (o caminho "255" do PutItem nao conferia o resultado).
//
// SetItemDuration vale para o item que chega sem data (Refi e Time zerados).
//
// Nao portado: o IsSealed para itens com CanSealed e o tratamento
// especial do item 5300 e do tesouro sagrado (ItemType 40).
func (h *Handler) putItem(c *netsrv.Conn, s *Session, it model.Item) bool {
	t := h.data()
	snapshot := append([]model.Item(nil), s.Inventory.Slots...)
	var tocados []int
	toca := func(i int) {
		for _, x := range tocados {
			if x == i {
				return
			}
		}
		tocados = append(tocados, i)
	}

	if !itemfn.CanAgroup(t, it.Index) {
		// Item novo com prazo na tabela ganha a data de fim (SetItemDuration).
		// So o que chega sem data: um item que ja tem data nao a perde.
		if it.IsSealed() {
			h.setItemDuration(&it)
		}
		slot, ok := h.freeInventorySlot(s)
		if !ok {
			return false
		}
		s.Inventory.Slots[slot] = it
		toca(slot)
	} else {
		resta := int(it.Refi)
		if resta == 0 {
			resta = 1
		}
		for slot := 0; slot < MovableInventorySlots && slot < len(s.Inventory.Slots) && resta > 0; slot++ {
			cur := &s.Inventory.Slots[slot]
			if bolsa := inventoryBagSlot(slot); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
				continue
			}
			if cur.Index != it.Index || int(cur.Refi) >= itemfn.MaxSlotAmount {
				continue
			}
			cabe := itemfn.MaxSlotAmount - int(cur.Refi)
			if cabe > resta {
				cabe = resta
			}
			cur.Refi += uint16(cabe)
			resta -= cabe
			toca(slot)
		}
		for resta > 0 {
			slot, ok := h.freeInventorySlot(s)
			if !ok {
				copy(s.Inventory.Slots, snapshot)
				return false
			}
			q := resta
			if q > itemfn.MaxSlotAmount {
				q = itemfn.MaxSlotAmount
			}
			novo := it
			novo.Refi = uint16(q)
			s.Inventory.Slots[slot] = novo
			resta -= q
			toca(slot)
		}
	}

	for _, i := range tocados {
		h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(i), s.Inventory.Slots[i])
	}
	return true
}

// newStackItem monta o item que PutItem(Index, quant) criava: APP igual ao
// Index, durabilidade cheia da tabela e quantidade em Refi (so se empilha).
func (h *Handler) newStackItem(index, quant uint16) model.Item {
	t := h.data()
	it := model.Item{Index: index, APP: index}
	if itemfn.CanAgroup(t, index) {
		it.Refi = quant
	}
	if info := t.Item(int(index)); info != nil {
		it.Min, it.Max = info.Durabilidade, info.Durabilidade
	}
	return it
}
