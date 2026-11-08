package game

import (
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/pran"
	"aika/internal/store"
)

// Limites dos slots da pran no MoveItem da origem.
const (
	pranEquipFirst   = 1  // Equip[0] e o corpo da forma, nao se mexe
	pranEquipLast    = 5  // slots de equipamento que o jogador troca
	pranBagSlots     = 40 // slots de item de 0 a 39; 40 e 41 sao as bolsas
	pranBagFirst     = 40 // bolsa dos slots 0..19
	pranBagSecond    = 41 // bolsa dos slots 20..39
	pranEquipItemOff = 18 // slot de equipamento da pran = ItemType - 18
)

// resolvePranSlot devolve o ponteiro para um slot da pran invocada.
//
// Origem (MoveItem): equipamento aceita os slots 1 a 5, a bolsa os slots 0 a 39
// com a bolsa que o cobre presente (pran.Inventory[40] ou [41]), e os dois
// exigem uma pran invocada (spawnedPran <> 255).
func (h *Handler) resolvePranSlot(s *Session, tipo, slot int) (*model.Item, error) {
	i, ok := activePran(s)
	if !ok {
		return nil, ErrSlotInvalido
	}
	p := s.Prans[i]
	switch tipo {
	case store.SlotTypePranEquip:
		if slot < pranEquipFirst || slot > pranEquipLast {
			return nil, ErrSlotInvalido
		}
		return &p.Equip[slot], nil
	case store.SlotTypePranInv:
		if slot < 0 || slot >= pranBagSlots {
			return nil, ErrSlotInvalido
		}
		bolsa := pranBagFirst
		if slot >= 20 {
			bolsa = pranBagSecond
		}
		if p.Inventory[bolsa].IsEmpty() {
			return nil, ErrBolsaAusente
		}
		return &p.Inventory[slot], nil
	}
	return nil, ErrTipoSlotInvalido
}

// pranClassStone e o porte de TPlayer.GetPranClassStoneItem: a "classe" que um
// item de equipamento da pran precisa ter para a forma atual.
func pranClassStone(class uint8) uint16 {
	switch class {
	case 61, 62, 71, 72, 81, 82:
		return 100
	case 63, 73, 83:
		return 101
	case 64, 74, 84:
		return 102
	}
	return 0
}

// pranEquipOk confere se o item entra no slot de equipamento da pran: o slot e
// ItemType - 18, o nivel do item no maximo o da pran + 1, e a classe do item
// zero ou igual a da pedra da forma da pran.
func (h *Handler) pranEquipOk(s *Session, it model.Item, slot int) bool {
	i, ok := activePran(s)
	if !ok {
		return false
	}
	p := s.Prans[i]
	t := h.data()
	if int(itemfn.ItemType(t, it.Index))-pranEquipItemOff != slot {
		return false
	}
	if int(itemfn.ItemLevel(t, it.Index)) > int(p.Level)+1 {
		return false
	}
	classe := itemfn.ItemClass(t, it.Index)
	if classe > 0 && classe != pranClassStone(p.Class) {
		return false
	}
	return true
}

// podeTrocarPran junta as regras de MoveItem que envolvem os slots da pran.
// Todas as recusas da origem eram "Exit" sem aviso.
//
// Diferencas conscientes:
//   - o equipamento da pran so troca com o inventario do dono. A origem deixava
//     o tipo 5 ir para o armazem ou o bau sem nenhuma conferencia;
//   - ao tirar uma peca do equipamento da pran a origem comparava o slot de
//     equipamento do item com o slot do INVENTARIO de destino, o que recusava
//     quase toda troca. Aqui compara com o slot da pran, que e a intencao.
func (h *Handler) podeTrocarPran(s *Session, pkt packets.MoveItemPacket, src, dest model.Item) bool {
	st, dt := int(pkt.SrcType), int(pkt.DestType)
	pranEq := store.SlotTypePranEquip
	pranInv := store.SlotTypePranInv
	if st != pranEq && dt != pranEq && st != pranInv && dt != pranInv {
		return true
	}
	t := h.data()

	if (st == pranEq && dt != store.SlotTypeInventory) || (dt == pranEq && st != store.SlotTypeInventory) {
		return false
	}
	// Item preso a conta nao vai para a bolsa da pran, nem volta dela por cima de
	// um item preso do inventario.
	if st == store.SlotTypeInventory && dt == pranInv &&
		!src.IsEmpty() && itemfn.TypeTrade(t, src.Index) == 1 {
		return false
	}
	if st == pranInv && dt == store.SlotTypeInventory &&
		!dest.IsEmpty() && itemfn.TypeTrade(t, dest.Index) == 1 {
		return false
	}
	if st == store.SlotTypeInventory && dt == pranEq {
		return h.pranEquipOk(s, src, int(pkt.DestSlot))
	}
	if st == pranEq && dt == store.SlotTypeInventory && !dest.IsEmpty() {
		return h.pranEquipOk(s, dest, int(pkt.SrcSlot))
	}
	return true
}

// pranEquipMoved diz se o movimento mexeu no equipamento da pran.
func pranEquipMoved(pkt packets.MoveItemPacket) bool {
	return int(pkt.SrcType) == store.SlotTypePranEquip || int(pkt.DestType) == store.SlotTypePranEquip
}

// afterPranEquipMove e o ramo PranRespawn do MoveItem: refaz os efeitos do
// equipamento no dono e reenvia a pran com a aparencia nova.
func (h *Handler) afterPranEquipMove(c *netsrv.Conn, s *Session) {
	i, ok := activePran(s)
	if !ok {
		return
	}
	h.applyPranEffects(s, s.Prans[i])
	h.sendPranSpawn(c, s, i)
}

// emptyPranSlot e o porte de TItemFunctions.GetEmptyPranSlot: o primeiro slot
// livre da bolsa da pran (0 a 19, e 20 a 39 se a segunda bolsa existe).
func emptyPranSlot(p *pran.Pran) int {
	max := 19
	if !p.Inventory[pranBagSecond].IsEmpty() {
		max = 39
	}
	for i := 0; i <= max; i++ {
		if p.Inventory[i].IsEmpty() {
			return i
		}
	}
	return -1
}
