package game

import (
	"context"
	"errors"

	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/stats"
	"aika/internal/store"
)

// Codigos do inventario em jogo.
const (
	CodeMoveItem    = 0x70F // cliente -> servidor: arrastar item
	CodeDeleteItem  = 0x32C // cliente -> servidor: jogar item fora
	CodeRefreshItem = 0xF0E // servidor -> cliente: slot mudou
)

// Faixas de slot, iguais as da origem.
const (
	// MovableInventorySlots e o "if (Packet.srcSlot < 60)" da origem: os
	// slots 60..63 guardam as bolsas e nao podem ser arrastados.
	MovableInventorySlots = 60
	// MinMovableEquipSlot e o "(srcSlot < 16) and (srcSlot > 1)": os slots 0
	// e 1 nao saem do equipamento.
	MinMovableEquipSlot = 2
)

// Erros do inventario.
var (
	ErrSlotInvalido     = errors.New("game: slot invalido")
	ErrTipoSlotInvalido = errors.New("game: tipo de slot nao suportado")
	ErrBolsaAusente     = errors.New("game: bolsa do slot nao esta no inventario")
)

// inventoryBagSlot devolve o slot da bolsa que habilita um slot do
// inventario. Porte do "case Packet.srcSlot of 0..14: SrcBag := 60; ..." da
// origem: cada grupo de 15 slots depende de uma bolsa nos slots 60 a 63.
func inventoryBagSlot(slot int) int {
	switch {
	case slot < 15:
		return 60
	case slot < 30:
		return 61
	case slot < 45:
		return 62
	case slot < 60:
		return 63
	}
	return -1
}

// moveItem e o porte da parte de TPacketHandlers.MoveItem ($70F) que trata
// inventario e equipamento.
//
// Armazem, bau de guilda e pran dependem de sistemas que ainda nao estao
// portados e sao recusados aqui; ver GoServer/PORTE.md.
//
// A origem iniciava srcItem e destItem em nil e so os preenchia dentro dos
// ramos do case. Um SrcType ou DestType fora da lista deixava o ponteiro nil
// e, poucas linhas abaixo, "ItemList[destItem.Index].ItemType" o
// dereferenciava: o cliente derrubava a thread do jogador mandando um tipo
// qualquer. Aqui os dois lados sao resolvidos antes e tipo invalido e
// recusado.
func (h *Handler) moveItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.MoveItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	// Equipamento so troca com inventario, nos dois sentidos.
	if pkt.SrcType == store.SlotTypeEquip && pkt.DestType != store.SlotTypeInventory {
		return nil
	}
	if pkt.DestType == store.SlotTypeEquip && pkt.SrcType != store.SlotTypeInventory {
		return nil
	}

	src, err := h.resolveSlot(c, s, int(pkt.SrcType), int(pkt.SrcSlot), true)
	if err != nil {
		h.log().Debug("movimento de item recusado na origem",
			"usuario", s.Username, "tipo", pkt.SrcType,
			"slot", pkt.SrcSlot, "motivo", err)
		return nil
	}
	dest, err := h.resolveSlot(c, s, int(pkt.DestType), int(pkt.DestSlot), false)
	if err != nil {
		h.log().Debug("movimento de item recusado no destino",
			"usuario", s.Username, "tipo", pkt.DestType,
			"slot", pkt.DestSlot, "motivo", err)
		return nil
	}
	if src == dest {
		// Arrastar o slot sobre ele mesmo: a origem seguia e trocava o item
		// consigo, o que com a pilha de itens dobrava a quantidade.
		return nil
	}

	// Duas pecas de equipamento nao se trocam direto, e o slot de origem
	// precisa ter item.
	if src.IsEmpty() {
		return nil
	}

	if !h.podeTrocar(c, s, pkt, *src, *dest) {
		return nil
	}

	equipando := pkt.DestType == store.SlotTypeEquip
	desequipando := pkt.SrcType == store.SlotTypeEquip

	// Pilhas iguais e empilhaveis se juntam em vez de trocar de lugar.
	if src.Index == dest.Index && itemfn.CanAgroup(h.data(), dest.Index) {
		itemfn.Agroup(h.data(), src, dest)
	} else {
		*src, *dest = *dest, *src
	}

	h.sendRefreshItem(c, s, uint8(pkt.DestType), pkt.DestSlot, *dest)
	h.sendRefreshItem(c, s, uint8(pkt.SrcType), pkt.SrcSlot, *src)

	// Trocar de equipamento muda os maximos e a aparencia.
	if equipando || desequipando {
		s.Avatar.SetEquip(s.Equip)
		s.Avatar.ApplyEquipBonus(h.Data, s.Equip)
		h.sendStatus(c, s)
		if slotMudaAparencia(int(pkt.DestSlot)) || slotMudaAparencia(int(pkt.SrcSlot)) {
			h.respawnToVisible(s)
		}
	}
	return nil
}

// slotMudaAparencia diz se o slot aparece no boneco. Na origem eram os slots
// 2 a 7, que marcavam ReSpawn := True.
func slotMudaAparencia(slot int) bool {
	return slot >= 2 && slot <= 7
}

// resolveSlot devolve o ponteiro para o item de um slot, validando tudo que
// vem do cliente.
//
// origem indica se o slot e o de partida: ali a origem exigia a bolsa e
// recusava os slots 0 e 1 do equipamento.
func (h *Handler) resolveSlot(c *netsrv.Conn, s *Session, tipo, slot int, origem bool) (*model.Item, error) {
	switch tipo {
	case store.SlotTypeInventory:
		if slot < 0 || slot >= MovableInventorySlots {
			return nil, ErrSlotInvalido
		}
		// A bolsa que cobre o slot tem de estar no inventario, como na
		// origem ("if Inventory[SrcBag].Index <= 0 then Exit").
		bolsa := inventoryBagSlot(slot)
		if bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			return nil, ErrBolsaAusente
		}
		if slot >= len(s.Inventory.Slots) {
			return nil, ErrSlotInvalido
		}
		return &s.Inventory.Slots[slot], nil

	case store.SlotTypeEquip:
		if slot < MinMovableEquipSlot || slot >= store.EquipSlots {
			return nil, ErrSlotInvalido
		}
		if slot >= len(s.Equip.Slots) {
			return nil, ErrSlotInvalido
		}
		return &s.Equip.Slots[slot], nil

	default:
		return nil, ErrTipoSlotInvalido
	}
}

// podeTrocar aplica as regras de equipar da origem.
func (h *Handler) podeTrocar(c *netsrv.Conn, s *Session, pkt packets.MoveItemPacket, src, dest model.Item) bool {
	t := h.data()

	// Pran e mascote dependem de sistemas que ainda nao estao portados: a
	// origem spawnava o bicho aqui dentro. Recusar e melhor que equipar um
	// item que nao vai produzir nada.
	for _, it := range []model.Item{src, dest} {
		if it.IsEmpty() {
			continue
		}
		switch itemfn.ItemType(t, it.Index) {
		case itemfn.ItemTypePran:
			h.message(c, "Pran ainda nao esta disponivel neste servidor.")
			return false
		case itemfn.ItemTypePet:
			h.message(c, "Mascote ainda nao esta disponivel neste servidor.")
			return false
		}
	}

	// Do inventario para o equipamento.
	if pkt.SrcType == store.SlotTypeInventory && pkt.DestType == store.SlotTypeEquip {
		if !h.podeVestir(c, s, src, int(pkt.DestSlot)) {
			return false
		}
	}
	// Do equipamento para o inventario: a origem checava o item que esta
	// indo PARA o equipamento (o que estava no inventario), nao o que sai.
	if pkt.SrcType == store.SlotTypeEquip && pkt.DestType == store.SlotTypeInventory {
		if !dest.IsEmpty() && !h.podeVestir(c, s, dest, int(pkt.SrcSlot)) {
			return false
		}
	}
	return true
}

// podeVestir diz se o item pode ocupar o slot de equipamento.
func (h *Handler) podeVestir(c *netsrv.Conn, s *Session, it model.Item, slot int) bool {
	t := h.data()
	if it.IsEmpty() {
		return true
	}
	if itemfn.EquipSlot(t, it.Index) != slot {
		return false
	}
	if s.Avatar.Level < itemfn.ItemLevel(t, it.Index) {
		h.message(c, "Seu nivel e baixo para este item.")
		return false
	}
	// A origem comparava GetMobClass() com GetMobClass(ItemList[].Classe):
	// item de classe 0 cai na classe do guerreiro, e nao em "sem restricao".
	// Mantido, porque e o que decide o que cada classe veste hoje.
	classeItem := itemfn.ItemClass(t, it.Index)
	if stats.MobClass(s.Avatar.ClassInfo) != stats.MobClass(classeItem) {
		h.message(c, "Sua classe nao pode usar este item.")
		return false
	}
	return true
}

// deleteItem e o porte de TPacketHandlers.DeleteItem ($32C).
//
// As guardas de slot da origem (> 15 no equipamento, > 63 no inventario) ja
// estavam ali; o que faltava era o ramo "else Exit" cobrir Item sem
// atribuicao, e o acesso a ItemList sem checar o indice.
func (h *Handler) deleteItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.DeleteItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	var it *model.Item
	switch int(pkt.TypeSlot) {
	case store.SlotTypeEquip:
		if pkt.Slot >= store.EquipSlots || int(pkt.Slot) >= len(s.Equip.Slots) {
			return nil
		}
		it = &s.Equip.Slots[pkt.Slot]
	case store.SlotTypeInventory:
		if pkt.Slot >= store.InventorySlots || int(pkt.Slot) >= len(s.Inventory.Slots) {
			return nil
		}
		it = &s.Inventory.Slots[pkt.Slot]
	default:
		return nil
	}
	if it.IsEmpty() {
		return nil
	}

	// Reliquia e um objeto do mundo: apagar solta no chao e avisa o servidor.
	// Isso depende de CreateMapObject, que ainda nao esta portado, entao o
	// pedido e recusado em vez de a reliquia desaparecer.
	if itemfn.ItemType(h.data(), it.Index) == itemTypeReliquia {
		h.message(c, "Esta reliquia nao pode ser descartada aqui.")
		return nil
	}

	descartado := *it
	*it = model.Item{}
	h.sendRefreshItem(c, s, uint8(pkt.TypeSlot), uint16(pkt.Slot), *it)

	if pkt.TypeSlot == store.SlotTypeEquip {
		s.Avatar.SetEquip(s.Equip)
		s.Avatar.ApplyEquipBonus(h.Data, s.Equip)
		h.sendStatus(c, s)
		if slotMudaAparencia(int(pkt.Slot)) {
			h.respawnToVisible(s)
		}
	}

	h.log().Info("item descartado",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"tipo_slot", pkt.TypeSlot, "slot", pkt.Slot,
		"item", descartado.Index, "quantidade", descartado.Refi)
	return nil
}

// itemTypeReliquia e o ItemType 40 da origem.
const itemTypeReliquia = 40

// sendRefreshItem manda o $F0E de um slot.
//
// A origem tinha tres formatos de pacote para o mesmo codigo, escolhidos pelo
// slot de equipamento do item: montaria (9), pran (10) e o normal. Montaria e
// pran ainda nao estao portados, entao aqui vale o formato normal.
func (h *Handler) sendRefreshItem(c *netsrv.Conn, s *Session, tipoSlot uint8, slot uint16, it model.Item) {
	var pkt packets.RefreshItemPacket
	pkt.Header.Index = WorldIndex
	pkt.TypeSlot = tipoSlot
	pkt.Slot = slot
	pkt.Item = it
	if err := c.SendStruct(CodeRefreshItem, pkt); err != nil {
		h.log().Debug("falha ao atualizar slot",
			"usuario", s.Username, "slot", slot, "erro", err)
	}
}

// respawnToVisible manda o pacote de criacao de novo a quem ja ve o jogador,
// para o boneco aparecer com o equipamento novo. Porte do
// "Player.Base.SendCreateMob(SPAWN_NORMAL)" do ramo ReSpawn.
func (h *Handler) respawnToVisible(s *Session) {
	if h.Chan == nil {
		return
	}
	pkt := h.buildCreateMob(s, SpawnNormal)
	raw, err := netsrv.Marshal(CodeCreateMob, pkt)
	if err != nil {
		h.log().Error("montar respawn", "cliente", s.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(s.ClientID, raw)
	h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
}

// data devolve as tabelas, ou uma vazia.
func (h *Handler) data() itemfn.Tabelas {
	if h.Data != nil {
		return h.Data
	}
	return tabelasVazias{}
}

// tabelasVazias responde que nada existe. E o que acontecia no original
// quando ItemList nao havia sido carregado: o registro vinha zerado.
type tabelasVazias struct{}

func (tabelasVazias) Item(int) *gamedata.ItemFromList { return nil }

func (tabelasVazias) Reinforce3At(int) *gamedata.ArmorAttributeReinforce { return nil }
