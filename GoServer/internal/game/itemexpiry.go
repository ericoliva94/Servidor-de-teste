package game

import (
	"fmt"

	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/store"
)

// itemTypeExpBuff e o ItemType 716: pocao de exp / quest em dobro, cujo buff
// sai junto com o item.
const itemTypeExpBuff = 716

// ExpireItemsTick porta TTimeItensThread.CheckItens: confere o prazo dos itens
// de todo jogador em jogo (inventario, equipamento e armazem) e trata os que
// venceram. Devolve quantos itens venceram. O chamador (main) fixa o intervalo.
//
// Um item vence quando a tabela o marca com Expires, ele nao esta selado e
// agora passou da data gravada nele. O que acontece depois depende do item:
//   - montaria (ItemType 9): fica no lugar com Time 0xFFFF (so "expirada");
//   - roupa de pran (Classe 100 a 104): fica com Min e Max 0xFF;
//   - o resto e apagado.
//
// Diferencas da origem:
//   - A roupa de pran vencida entrava de novo a cada passada, repetindo o
//     aviso para sempre; aqui o par Min/Max 0xFF ja marca que foi tratada.
//   - So inventario, equipamento e armazem estao portados. A origem tambem
//     conferia os itens da pran invocada; os itens da pran ainda nao tem
//     prazo no Go (ver PORTE.md), entao ficam de fora.
func (h *Handler) ExpireItemsTick() int {
	if h.Chan == nil {
		return 0
	}
	total := 0
	for _, id := range h.Chan.PlayingIDs() {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || s.Status < Playing || c == nil {
			continue
		}
		tradeMu.Lock()
		mudouEquip := false
		total += h.expireSet(s, s.Inventory.Slots, store.SlotTypeInventory, false, nil)
		n := h.expireSet(s, s.Equip.Slots, store.SlotTypeEquip, true, &mudouEquip)
		total += n
		total += h.expireSet(s, s.Storage.Slots, store.SlotTypeStorage, false, nil)
		tradeMu.Unlock()
		if mudouEquip {
			h.recalcAfterPran(s)
		}
	}
	return total
}

// itemExpired diz se o item venceu, e devolve a linha da tabela dele.
func (h *Handler) itemExpired(it model.Item) bool {
	if it.IsEmpty() || it.IsSealed() {
		return false
	}
	linha := h.data().Item(int(it.Index))
	if linha == nil || linha.Expires == 0 {
		return false
	}
	return h.now().After(it.Expire(linha.ItemType, baseDatetime))
}

// expireSet trata um conjunto de slots. Devolve quantos itens venceram.
func (h *Handler) expireSet(s *Session, slots []model.Item, tipo uint8, equip bool, mudou *bool) int {
	c := h.connOf(s)
	t := h.data()
	n := 0
	for j := range slots {
		it := &slots[j]
		// No equipamento a origem pulava Time = 0.
		if equip && it.Time == 0 {
			continue
		}
		if !h.itemExpired(*it) {
			continue
		}
		linha := t.Item(int(it.Index))
		nome := itemfn.ItemName(t, it.Index)

		switch {
		case itemfn.EquipSlot(t, it.Index) == itemfn.ItemTypeMount:
			it.Time = 0xFFFF
		case linha.Classe >= pranOutfitClassMin && linha.Classe <= pranOutfitClassMax:
			if it.Min == 0xFF && it.Max == 0xFF {
				continue
			}
			it.Min, it.Max = 0xFF, 0xFF
		default:
			if linha.ItemType == itemTypeExpBuff && tipo == store.SlotTypeInventory {
				h.removeBuffAndNotify(s, uint16(linha.UseEffect))
			}
			*it = model.Item{}
		}
		if mudou != nil {
			*mudou = true
		}
		h.sendRefreshItem(c, s, tipo, uint16(j), *it)
		h.message(c, fmt.Sprintf("O item [%s] expirou.", nome))
		n++
	}
	return n
}

// setItemDuration porta TItemFunctions.SetItemDuration: item novo com Expires
// na tabela ganha a data de fim em agora + Duration + 2 horas. Devolve se o
// item tem prazo.
func (h *Handler) setItemDuration(it *model.Item) bool {
	linha := h.data().Item(int(it.Index))
	if linha == nil || linha.Expires == 0 {
		return false
	}
	it.SetSealed(false, linha.ItemType, baseDatetime, int(linha.Duration), h.now())
	return true
}
