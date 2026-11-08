package game

import (
	"context"
	"fmt"

	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeMakeItem e o $32B: forja, desmontagem e limpeza de item.
const CodeMakeItem = 0x32B

// Modos do $32B, o campo Null3 do pacote. Qualquer outro valor e fabricacao.
const (
	makeDisassemble     = 1 // desmontar equipamento
	makeClearExtrasA    = 3 // limpar adicionais
	makeClearExtrasB    = 4 // idem (o cliente usa os dois valores)
	makeClearAppearance = 6 // limpar aparencia
)

// ItemType dos Vaizan e da Pedra Magica da Restauracao.
const (
	itemTypeVaizanNormal   = 513 // Vaizan Cinza
	itemTypeVaizanSuperior = 514 // Vaizan Colorido
	itemTypeVaizanRare     = 515 // Vaizan Brilhante
	itemTypeRestoreStone   = 517 // Pedra Magica da Restauracao
)

// TypeItem do item (TYPE_ITEM_* de GlobalDefs.pas).
const (
	typeItemNormal        = 0
	typeItemRareSuperior  = 1
	typeItemRaro          = 3
	typeItemSuperior      = 5
	typeItemLegendary     = 6
	typeItemPremium       = 7
	craftInvFullMsg       = "Inventario cheio."
	craftNotFoundMsg      = "Item nao encontrado na forja."
	maxCraftAmount        = itemfn.MaxSlotAmount
	disassembleMaxLevel   = 99
	disassembleBigPileLvl = 81 // a partir daqui a desmontagem rende ate 7, nao 5
)

// destroyRanges sao os Level_N_Destroy_Range de GlobalDefs.pas: o que a
// desmontagem pode render, por faixa de nivel (indice = (nivel-1)/10, com
// 1..20 usando a primeira).
var destroyRanges = [9][6]uint16{
	{5100, 5080, 5081, 5082, 4630, 4631}, // 1..20
	{5100, 5080, 5081, 5082, 4632, 4633}, // 21..30
	{5100, 5080, 5081, 5082, 4634, 4635}, // 31..40
	{5100, 5080, 5081, 5082, 4636, 4638}, // 41..50
	{5100, 5080, 5081, 5082, 4642, 4644}, // 51..60
	{5100, 5080, 5081, 5082, 4644, 4636}, // 61..70
	{5100, 5080, 5081, 5082, 4642, 4645}, // 71..80
	{5100, 5080, 5081, 5082, 4649, 4647}, // 81..90
	{5100, 5080, 5081, 5082, 4649, 4647}, // 91..99
}

// recipeTable e o que a tabela de dados precisa dar para a fabricacao.
type recipeTable interface {
	RecipeByResult(result uint16) (*gamedata.MakeItemsData, bool)
	IngredientsOf(result uint16) []gamedata.MakeItemsIngredientsData
}

// makeItem e o porte de TPacketHandlers.MakeItem ($32B).
//
// O campo ItemID do pacote significa duas coisas: nos modos de desmontar e de
// limpar e o SLOT do inventario; na fabricacao e o item RESULTANTE da
// receita (a origem o procura em MakeItems[].ResultItemID).
//
// A origem abria o handler com "if Packet.ItemID > 63 then Exit", valendo
// para todos os modos. Como os itens resultantes das receitas dos arquivos
// de dados comecam em 1000, **a fabricacao nunca funcionava**: o pedido saia
// antes de chegar na receita. Aqui o limite de slot vale so nos modos que
// usam slot. E uma inferencia a partir do codigo, nao testada com o cliente;
// ver KNOWN_ISSUES.md.
func (h *Handler) makeItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.MakeItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	switch pkt.Null3 {
	case makeDisassemble:
		h.disassembleItem(c, s, p, pkt)
	case makeClearExtrasA, makeClearExtrasB:
		h.clearExtras(c, s, p, pkt)
	case makeClearAppearance:
		h.clearAppearance(c, s, p, pkt)
	default:
		h.craftItem(c, s, pkt)
	}
	return nil
}

func (h *Handler) echoMake(c *netsrv.Conn, s *Session, p protocol.Packet, pkt packets.MakeItemPacket) {
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(p.Code, pkt)
}

// slotFromID resolve o ItemID do pacote como slot do inventario.
func (h *Handler) slotFromID(c *netsrv.Conn, s *Session, id uint16) *model.Item {
	it, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(id), true)
	if err != nil {
		return nil
	}
	return it
}

// consumeOne gasta uma unidade do slot e atualiza o cliente.
func (h *Handler) consumeOne(c *netsrv.Conn, s *Session, slot int) {
	itemfn.DecreaseAmount(&s.Inventory.Slots[slot], 1)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot), s.Inventory.Slots[slot])
}

// findByItemType acha o primeiro slot (0 a 59, com bolsa) cujo item tem o
// ItemType pedido. Porte de GetItemSlotByItemType. A origem varria ate o 63
// e comparava tambem os slots vazios (ItemType do item 0).
func (h *Handler) findByItemType(s *Session, itemType uint16) int {
	t := h.data()
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		it := s.Inventory.Slots[i]
		if it.IsEmpty() {
			continue
		}
		if bolsa := inventoryBagSlot(i); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		if itemfn.ItemType(t, it.Index) == itemType {
			return i
		}
	}
	return -1
}

// ---- desmontar

// disassembleItem desmonta equipamento (Null3 = 1): troca o item por materiais
// sorteados da faixa do nivel dele.
func (h *Handler) disassembleItem(c *netsrv.Conn, s *Session, p protocol.Packet, pkt packets.MakeItemPacket) {
	t := h.data()
	it := h.slotFromID(c, s, pkt.ItemID)
	const cant = "Este item nao pode ser desmontado."
	if it == nil || it.IsEmpty() {
		h.message(c, cant)
		return
	}
	switch es := itemfn.EquipSlot(t, it.Index); {
	case es >= 2 && es <= 7, es >= 11 && es <= 14:
	default:
		h.message(c, cant)
		return
	}
	info := t.Item(int(it.Index))
	if info == nil || info.Level < 1 || info.Level > disassembleMaxLevel {
		h.message(c, cant)
		return
	}

	faixa := (int(info.Level) - 1) / 10
	if faixa > 0 {
		faixa-- // 1..20 dividem a primeira faixa
	}
	if info.Level <= 20 {
		faixa = 0
	}
	r := h.rand()
	material := destroyRanges[faixa][r.IntN(len(destroyRanges[faixa]))]
	qtd := 1 + r.IntN(5) // RandomRange(1, 6)
	if info.Level >= disassembleBigPileLvl {
		qtd = 1 + r.IntN(7) // RandomRange(1, 8)
	}

	slot := int(pkt.ItemID)
	removido := *it
	*it = model.Item{}
	if !h.putItem(c, s, h.newStackItem(material, uint16(qtd))) {
		// Sem espaco para o material: devolve o item em vez de perder.
		s.Inventory.Slots[slot] = removido
		h.message(c, craftInvFullMsg)
		return
	}
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot), s.Inventory.Slots[slot])
	h.message(c, "Item desmontado com sucesso.")
	pkt.ItemID, pkt.Null, pkt.Amount, pkt.Null2, pkt.Null3, pkt.Null4 = 0, 0, 0, 0, 0, 0
	h.echoMake(c, s, p, pkt)
}

// ---- limpar adicionais

// vaizanFor devolve o ItemType do Vaizan que serve ao TypeItem do item e as
// mensagens de falta (para "limpar tudo" e para "selecionar"); ok false quando
// o TypeItem nao tem Vaizan (a origem saia sem dizer nada).
func vaizanFor(typeItem uint8, all bool) (itemType uint16, falta string, ok bool, premium bool) {
	var cor, rotulo string
	switch typeItem {
	case typeItemNormal:
		itemType, cor, rotulo = itemTypeVaizanNormal, "Cinza", "Normal"
	case typeItemRareSuperior, typeItemSuperior:
		itemType, cor, rotulo = itemTypeVaizanSuperior, "Colorido", "Superior"
	case typeItemRaro, typeItemLegendary:
		itemType, cor, rotulo = itemTypeVaizanRare, "Brilhante", "Raro"
	case typeItemPremium:
		return 0, "", false, true
	default:
		return 0, "", false, false
	}
	if !all {
		rotulo = "Selecione"
	}
	return itemType, fmt.Sprintf("Voce deve possuir o item Vaizan %s [%s].", cor, rotulo), true, false
}

// clearExtras limpa os adicionais (efeitos) do item (Null3 = 3 ou 4).
//
// Amount 0 limpa os tres. 1, 2 e 3 limpam um so, escolhido como a origem: 1 o
// primeiro adicional existente, 2 o segundo (ou o terceiro quando nao ha
// segundo), 3 o ultimo existente.
func (h *Handler) clearExtras(c *netsrv.Conn, s *Session, p protocol.Packet, pkt packets.MakeItemPacket) {
	t := h.data()
	it := h.slotFromID(c, s, pkt.ItemID)
	if it == nil || it.IsEmpty() {
		return
	}
	if itemfn.CanAgroup(t, it.Index) {
		h.message(c, "Esse item nao pode ser limpo.")
		return
	}
	info := t.Item(int(it.Index))
	if info == nil || pkt.Amount > 3 {
		return
	}
	vType, falta, ok, premium := vaizanFor(info.TypeItem, pkt.Amount == 0)
	if premium {
		h.message(c, "Itens de cash nao podem ser limpos.")
		return
	}
	if !ok {
		return
	}
	vaizan := h.findByItemType(s, vType)
	if vaizan < 0 {
		h.message(c, falta)
		return
	}

	e := &it.Effects
	clear1 := func(i int) { e.Index[i], e.Value[i] = 0, 0 }
	switch pkt.Amount {
	case 0:
		*e = model.ItemEffect{}
	case 1:
		switch {
		case e.Index[0] != 0:
			clear1(0)
		case e.Index[1] != 0:
			clear1(1)
		default:
			clear1(2)
		}
	case 2:
		if e.Index[1] == 0 {
			clear1(2)
		} else {
			clear1(1)
		}
	case 3:
		switch {
		case e.Index[2] != 0:
			clear1(2)
		case e.Index[1] != 0:
			clear1(1)
		default:
			clear1(0)
		}
	}
	h.consumeOne(c, s, vaizan)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, pkt.ItemID, *it)
	h.message(c, "Adicionais do item limpos com sucesso.")
	h.echoMake(c, s, p, pkt)
}

// ---- limpar aparencia

func (h *Handler) clearAppearance(c *netsrv.Conn, s *Session, p protocol.Packet, pkt packets.MakeItemPacket) {
	t := h.data()
	it := h.slotFromID(c, s, pkt.ItemID)
	if it == nil || it.IsEmpty() {
		return
	}
	switch {
	case itemfn.CanAgroup(t, it.Index):
		h.message(c, "Esse item nao pode ser limpo.")
		return
	case it.APP == 0:
		h.message(c, "Esse item nao possui aparencia.")
		return
	case it.APP == it.Index:
		h.message(c, "Esse item ja esta limpo.")
		return
	}
	stone := h.findByItemType(s, itemTypeRestoreStone)
	if stone < 0 {
		h.message(c, "Voce deve possuir o item Pedra Magica da Restauracao.")
		return
	}
	it.APP = it.Index
	h.consumeOne(c, s, stone)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, pkt.ItemID, *it)
	h.message(c, "Aparencia do item removida.")
	h.echoMake(c, s, p, pkt)
}

// ---- fabricar

// craftItem fabrica o item pelo ItemID da receita.
//
// Mantido da origem:
//   - a quantidade resultante e a pedida (Amount), nao a coluna ResultAmount
//     do CSV (que a origem nem le no handler); equipamento sai sempre 1;
//   - cada ingrediente tem de estar inteiro no PRIMEIRO slot em que aparece
//     ("Separe a quantidade correta em apenas UM slot");
//   - falha na fabricacao nao cobra ouro nem consome ingrediente;
//   - o sorteio de sucesso e RandomRange(1, TaxSuccess div 10 + 1) <=
//     TaxSuccess div 10: **sempre verdadeiro** quando TaxSuccess >= 10 (e
//     sempre falso abaixo disso). Das 425 receitas, 415 tem 1000 e 10 tem 500,
//     que a origem fabrica 100% das vezes. Ver KNOWN_ISSUES.md.
//
// Corrigido: Amount 0 passava na conferencia de ouro (preco x 0) e depois
// cobrava preco x 1 sem conferir (ouro negativo); a quantidade dos
// ingredientes era multiplicada num WORD (65535 x 2 dava a volta e pedia
// quase nada); o saldo de ingrediente era lido num BYTE (uma pilha de 300
// parecia ter 44); duas linhas da receita com o mesmo item eram conferidas
// separadamente.
func (h *Handler) craftItem(c *netsrv.Conn, s *Session, pkt packets.MakeItemPacket) {
	t := h.data()
	rt, ok := t.(recipeTable)
	if !ok {
		h.message(c, craftNotFoundMsg)
		return
	}
	recipe, ok := rt.RecipeByResult(pkt.ItemID)
	if !ok {
		h.message(c, craftNotFoundMsg)
		return
	}
	if int(recipe.LevelMin)+1 > int(s.Avatar.Level) {
		h.message(c, "Voce nao possui o nivel necessario.")
		return
	}

	qtd := int(pkt.Amount)
	if qtd < 1 {
		qtd = 1
	}
	if qtd > maxCraftAmount {
		qtd = maxCraftAmount
	}
	if es := itemfn.EquipSlot(t, recipe.ResultItemID); (es >= 2 && es <= 7) || (es >= 11 && es <= 14) {
		qtd = 1
	}

	preco := int64(recipe.Price) * int64(qtd)
	if preco > s.Avatar.Gold {
		h.message(c, fmt.Sprintf("Voce nao possui o gold necessario. %d necessarios.", preco))
		return
	}
	if _, ok := h.freeInventorySlot(s); !ok {
		h.message(c, craftInvFullMsg)
		return
	}

	type req struct{ slot, amount int }
	var reqs []req
	necessario := map[int]int{} // slot -> total pedido
	for _, ing := range rt.IngredientsOf(recipe.ResultItemID) {
		slot := h.firstSlotWith(s, ing.ItemID)
		nome := itemfn.ItemName(t, ing.ItemID)
		if slot < 0 {
			h.message(c, fmt.Sprintf("Voce precisa ter o item [%s]. Separe a quantidade correta em apenas UM slot.", nome))
			return
		}
		precisa := int(ing.Amount) * qtd
		necessario[slot] += precisa
		tem := 1 // equipamento: conta como uma unidade
		if itemfn.CanAgroup(t, ing.ItemID) {
			tem = int(s.Inventory.Slots[slot].Refi)
		}
		if tem < necessario[slot] {
			h.message(c, fmt.Sprintf("Voce precisa de (%d) do item [%s].", precisa, nome))
			return
		}
		reqs = append(reqs, req{slot: slot, amount: precisa})
	}

	// Sorteio de sucesso, como a origem (ver o comentario da funcao).
	tax := int(recipe.TaxSuccess) / 10
	if tax < 1 || 1+h.rand().IntN(tax) > tax {
		h.message(c, "A criacao do item falhou.")
		return
	}

	if !h.putItem(c, s, h.newStackItem(recipe.ResultItemID, uint16(qtd))) {
		h.message(c, craftInvFullMsg)
		return
	}
	s.Avatar.Gold -= preco
	h.sendRefreshMoney(c, s)
	for _, r := range reqs {
		it := &s.Inventory.Slots[r.slot]
		if es := itemfn.EquipSlot(t, it.Index); es >= 2 && es <= 14 {
			*it = model.Item{}
		} else {
			itemfn.DecreaseAmount(it, uint16(min(r.amount, 65535)))
		}
		h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(r.slot), *it)
	}
	h.message(c, "A criacao do item foi bem sucedida.")
}

// firstSlotWith acha o primeiro slot (0 a 59, com bolsa) com o item.
func (h *Handler) firstSlotWith(s *Session, index uint16) int {
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if s.Inventory.Slots[i].Index != index {
			continue
		}
		if bolsa := inventoryBagSlot(i); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		return i
	}
	return -1
}
