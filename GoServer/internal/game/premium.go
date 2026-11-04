package game

import (
	"context"
	"fmt"
	"strings"

	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos do inventario premium e do selo.
const (
	// CodeBuyItemCash e o $209: cliente -> servidor, comprar do catalogo.
	CodeBuyItemCash = 0x209
	// CodeSendGift e o $356: cliente -> servidor, presentear um item premium.
	CodeSendGift = 0x356
	// CodeUnsealItem e o $224: cliente -> servidor, abrir o selo de um item.
	CodeUnsealItem = 0x224
	// CodeCashInventory e o $138: servidor -> cliente, os 24 slots premium.
	CodeCashInventory = 0x138
	// CodePlayerCash e o $139: servidor -> cliente, o saldo premium.
	CodePlayerCash = 0x139
)

// PremiumTable e a tabela do catalogo premium (PI.bin).
type PremiumTable interface {
	// PremiumAt devolve a linha pela posicao, nil fora da faixa.
	PremiumAt(i int) *gamedata.PremiumItem
	// PremiumByIndex acha a linha pelo campo Index e devolve a posicao.
	PremiumByIndex(index uint32) (*gamedata.PremiumItem, int)
}

// usePremiumItem e o porte de TItemFunctions.UsePremiumItem: tira um item do
// inventario premium e poe no inventario normal.
//
// Diferencas centrais em relacao a origem:
//
//   - "Premium := @CashInventory.Items[Slot]" com Slot vindo de Packet.Slot,
//     um DWORD que o handler so conferia contra 63, num vetor
//     ARRAY [0..23]. Um slot de 24 a 63 lia E ESCREVIA fora do vetor:
//     logo depois vem "ZeroMemory(Premium, sizeof(TItemCash))", oito bytes
//     gravados em cima do resto do registro da conta.
//
//   - Nao havia checagem de slot VAZIO. Com Premium.Index = 0 a origem lia
//     "PremiumItems[0]" e entregava o item daquela linha do catalogo — de
//     graca e quantas vezes o jogador clicasse num slot vazio.
//
//   - "PremiumItems[Premium.Index]" indexa o catalogo pelo VALOR guardado, e
//     nao pela posicao. A propria origem tem FindPremiumIndex
//     (ItemFunctions.pas:1976) escrita para fazer essa conversao, e nenhum
//     dos dois pontos que indexam a tabela a usa.
//
//   - "Result := (Premium.Index = 0)" era lido DEPOIS do ZeroMemory que zera
//     Premium, entao devolvia sempre True.
func (h *Handler) usePremiumItem(ctx context.Context, c *netsrv.Conn, s *Session, slot int) error {
	if h.Premium == nil {
		h.message(c, "Loja premium indisponivel.")
		return nil
	}
	if slot < 0 || slot >= store.CashSlots || slot >= len(s.Cash.Slots) {
		return nil
	}
	guardado := s.Cash.Slots[slot]
	if guardado.IsEmpty() {
		return nil
	}

	linha, _ := h.Premium.PremiumByIndex(uint32(guardado.Index))
	if linha == nil {
		h.log().Warn("slot premium aponta para indice fora do catalogo",
			"usuario", s.Username, "slot", slot, "indice", guardado.Index)
		h.message(c, "Este item nao existe mais no catalogo.")
		return nil
	}

	livre, ok := h.freeInventorySlot(s)
	if !ok {
		h.message(c, "Inventario cheio.")
		return nil
	}

	var it model.Item
	it.Index = linha.ItemIndex
	t := h.data()
	// A origem punha a quantidade e, para item que expira, a zerava em
	// seguida: o campo e o mesmo (Refi), entao um item com prazo entra com
	// quantidade zero e o prazo e quem manda.
	if itemfnExpires(t, linha.ItemIndex) {
		itemfn.SetAmount(t, &it, 0)
	} else {
		itemfn.SetAmount(t, &it, linha.Amount)
	}

	s.Inventory.Slots[livre] = it
	s.Cash.Slots[slot] = model.Item{}

	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(livre), it)
	h.sendRefreshItem(c, s, store.SlotTypeCash, uint16(slot), model.Item{})
	h.persistCashItems(ctx, s)
	h.log().Info("item premium retirado",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"slot_premium", slot, "item", linha.ItemIndex, "slot_inv", livre)
	return nil
}

// itemfnExpires diz se o item tem prazo de validade (ItemList[].Expires).
func itemfnExpires(t itemfn.Tabelas, index uint16) bool {
	it := t.Item(int(index))
	return it != nil && it.Expires != 0
}

// buyItemCash e o porte de TPacketHandlers.BuyItemCash ($209).
//
// Diferencas centrais em relacao a origem:
//
//   - "PremiumItems[Packet.Data].show" indexava um vetor DINAMICO com um
//     DWORD vindo direto do cliente, sem checagem nenhuma
//     (PacketHandlers.pas:4578), e em seguida fazia o mesmo com .Price. Um
//     indice qualquer lia memoria fora da tabela, e o preco que saisse dali
//     decidia a compra: com um preco lido como zero o item saia de graca.
//     Aqui o indice e procurado no catalogo pelo campo Index.
//
//   - A compra nao conferia o campo Status nem o Trade, so o Show.
//
//   - "DecCardinal(Cash, Price)" calcula "Res: Integer := x - Value" com x
//     sendo Cardinal: um saldo acima de 2.147.483.647 estourava o Integer,
//     Res ficava negativo e a funcao zerava o cash inteiro. Quem tinha mais
//     de 2,1 bilhoes de cash perdia tudo na primeira compra.
func (h *Handler) buyItemCash(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Premium == nil {
		h.message(c, "Loja premium indisponivel.")
		return nil
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Data == 0 {
		h.message(c, "Item invalido.")
		return nil
	}

	linha, _ := h.Premium.PremiumByIndex(pkt.Data)
	if linha == nil {
		h.message(c, "Item invalido.")
		h.log().Debug("compra premium com indice fora do catalogo",
			"usuario", s.Username, "indice", pkt.Data)
		return nil
	}
	if linha.Show == 0 {
		h.message(c, "Item nao pode ser comprado.")
		return nil
	}
	if s.CashBalance < linha.Price {
		h.message(c, "Voce nao tem cash suficiente.")
		return nil
	}

	slot, ok := h.freeCashSlot(s)
	if !ok {
		h.message(c, "Voce nao tem espaco suficiente na loja cash.")
		return nil
	}
	// O catalogo guarda o INDICE do catalogo no slot, nao o do item. O campo
	// e WORD na origem (TItemCash.Index), entao um indice acima de 65535 era
	// truncado em silencio; aqui ele e recusado.
	if linha.Index > 0xFFFF {
		h.log().Error("indice de catalogo acima do WORD",
			"indice", linha.Index)
		h.message(c, "Item invalido.")
		return nil
	}

	s.CashBalance -= linha.Price
	s.Cash.Slots[slot] = model.Item{Index: uint16(linha.Index)}

	h.sendRefreshItem(c, s, store.SlotTypeCash, uint16(slot), s.Cash.Slots[slot])
	h.message(c, "Item comprado com sucesso.")
	h.sendPlayerCash(c, s)
	h.persistCash(ctx, s)
	h.log().Info("item premium comprado",
		"usuario", s.Username, "conta", s.Account.ID,
		"catalogo", linha.Index, "preco", linha.Price,
		"saldo", s.CashBalance, "slot", slot)
	return nil
}

// freeCashSlot acha o primeiro slot premium vazio.
//
// Porte de TCashInventory.AddItem, que ja parava no primeiro vazio. O que
// faltava era a guarda de faixa em TCashInventory.IsEmpyt, que indexa
// "Items[Slot]" com Slot sendo byte num vetor ARRAY [0..23]: um slot de 24 a
// 255 lia fora.
func (h *Handler) freeCashSlot(s *Session) (int, bool) {
	for i := range min(store.CashSlots, len(s.Cash.Slots)) {
		if s.Cash.Slots[i].IsEmpty() {
			return i, true
		}
	}
	return 0, false
}

// sendGift e o porte de TPacketHandlers.SendGift ($356).
//
// Diferencas centrais em relacao a origem:
//
//   - O destinatario saia de "Packet.Target", um WORD do cliente usado como
//     indice em "Servers[].Players[Packet.Target]" sem nenhuma checagem — e
//     dali ela lia e ESCREVIA o inventario premium daquele slot. O pacote
//     tambem traz o apelido digitado, que a origem simplesmente ignorava.
//     Aqui o destinatario e resolvido pelo APELIDO, como no correio.
//
//   - "SelfPremium.IsEmpyt(Packet.Slot)" indexa ARRAY [0..23] com um byte.
//
//   - Nao havia checagem de presentear a si mesmo, nem de o item ser
//     presenteavel (o campo Trade do catalogo).
//
//   - O item era retirado do remetente com ZeroMemory DEPOIS de ter sido
//     posto no destinatario, sem transacao: um erro no meio duplicava.
func (h *Handler) sendGift(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Premium == nil {
		return nil
	}
	var pkt packets.SendGiftPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	slot := int(pkt.Slot)
	if slot < 0 || slot >= store.CashSlots || slot >= len(s.Cash.Slots) {
		return nil
	}
	guardado := s.Cash.Slots[slot]
	if guardado.IsEmpty() {
		return nil
	}

	nome := strings.TrimSpace(netsrv.CString(pkt.Nick[:]))
	if nome == "" {
		h.message(c, "Diga para quem e o presente.")
		return nil
	}
	if strings.EqualFold(nome, s.Avatar.Name) {
		h.message(c, "Voce nao pode presentear a si mesmo.")
		return nil
	}
	id, ok := h.Chan.FindByCharacter(nome)
	if !ok {
		h.message(c, "Jogador nao encontrado.")
		return nil
	}
	alvo, alvoConn, ok := h.Chan.Lookup(id)
	if !ok || alvo.Status < Playing {
		h.message(c, "Jogador nao encontrado.")
		return nil
	}
	if alvo.Account.ID == s.Account.ID {
		h.message(c, "Voce nao pode presentear a si mesmo.")
		return nil
	}

	linha, _ := h.Premium.PremiumByIndex(uint32(guardado.Index))
	if linha == nil {
		h.message(c, "Este item nao existe mais no catalogo.")
		return nil
	}
	if linha.Trade == 0 {
		h.message(c, "Este item nao pode ser presenteado.")
		return nil
	}

	destino, ok := h.freeCashSlot(alvo)
	if !ok {
		h.message(c, "Inventario cash do destinatario ja esta cheio.")
		return nil
	}

	// Tira primeiro, poe depois: assim uma falha deixa o item com o
	// remetente em vez de criar uma copia.
	s.Cash.Slots[slot] = model.Item{}
	alvo.Cash.Slots[destino] = guardado

	h.sendRefreshItem(c, s, store.SlotTypeCash, uint16(slot), model.Item{})
	h.sendRefreshItem(alvoConn, alvo, store.SlotTypeCash, uint16(destino), guardado)
	h.sendCashInventory(c, s)
	h.sendCashInventory(alvoConn, alvo)
	h.message(alvoConn, fmt.Sprintf("Recebeu presente de %s.", s.Avatar.Name))
	h.message(c, "Presente enviado.")

	h.persistCashItems(ctx, s)
	h.persistCashItems(ctx, alvo)
	h.log().Info("presente premium enviado",
		"de", s.Avatar.Name, "para", alvo.Avatar.Name,
		"catalogo", guardado.Index)
	return nil
}

// unsealItem e o porte de TPacketHandlers.UnsealItem ($224).
//
// Diferencas centrais em relacao a origem:
//
//   - Ela chamava "TItemFunctions.UseItem(Player, Packet.Data)" logo depois
//     de desselar, e INCONDICIONALMENTE — mesmo quando o item nao estava
//     selado. Abrir o selo de uma pocao bebia a pocao; abrir o selo de uma
//     bolsa gastava a bolsa. E como o caminho nao conferia IsDead, o $224
//     virava um "usar item" sem as guardas do $31D.
//
//   - "if (Item.IsSealed) then Item.IsSealed := False" sem else: um item NAO
//     selado passava direto para o UseItem.
//
//   - GetIsSealed le Refi e Time como um DWORD unico e considera selado
//     quando os quatro bytes sao zero. Isso e verdade tambem para um item
//     comum de refino 0 sem prazo — um equipamento recem-criado, por exemplo.
//     Na origem, abrir o selo de uma arma de refino 0 gravava nela um prazo
//     de validade (Now + Duration + 2h) e em seguida a "usava". Aqui o item
//     so e desselado quando a tabela diz que ele e selavel, isto e quando
//     tem Duration configurada.
func (h *Handler) unsealItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if s.Dead {
		return nil
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Data >= store.InventorySlots {
		return nil
	}
	slot := int(pkt.Data)
	if slot >= len(s.Inventory.Slots) {
		return nil
	}
	it := &s.Inventory.Slots[slot]
	if it.IsEmpty() {
		return nil
	}
	if !it.IsSealed() {
		return nil
	}
	t := h.data()
	linha := t.Item(int(it.Index))
	if linha == nil {
		return nil
	}
	// Item sem Duration na tabela nao e selavel: o "selado" dele e so refino
	// 0 sem prazo, que GetIsSealed nao distingue.
	if linha.Duration == 0 {
		h.log().Debug("pedido de selo em item sem duracao na tabela",
			"usuario", s.Username, "slot", slot, "item", it.Index)
		return nil
	}

	it.SetSealed(false, linha.ItemType, h.now(), int(linha.Duration), h.now())
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot), *it)
	h.log().Info("selo aberto",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"slot", slot, "item", it.Index, "horas", linha.Duration+2)
	return nil
}

// sendCashInventory e o porte de TPlayer.SendCashInventory ($138).
func (h *Handler) sendCashInventory(c *netsrv.Conn, s *Session) {
	if c == nil || s == nil {
		return
	}
	var pkt packets.UpdateCashInventory
	pkt.Header.Index = s.ClientID
	for i := range pkt.Items {
		if i >= len(s.Cash.Slots) {
			break
		}
		it := s.Cash.Slots[i]
		pkt.Items[i] = model.ItemCash{
			Index:     it.Index,
			APP:       it.APP,
			Identific: it.Identific,
		}
	}
	if err := c.SendStruct(CodeCashInventory, pkt); err != nil {
		h.log().Debug("enviar inventario premium",
			"usuario", s.Username, "erro", err)
	}
}

// sendPlayerCash e o porte de TPlayer.SendPlayerCash ($139).
//
// A origem punha zero no Index do cabecalho deste pacote, ao contrario dos
// vizinhos; mantido.
func (h *Handler) sendPlayerCash(c *netsrv.Conn, s *Session) {
	if c == nil || s == nil {
		return
	}
	if err := h.sendSignalData(c, 0, CodePlayerCash, s.CashBalance); err != nil {
		h.log().Debug("enviar saldo premium",
			"usuario", s.Username, "erro", err)
	}
}

// persistCash grava o saldo premium.
func (h *Handler) persistCash(ctx context.Context, s *Session) {
	if h.AccountCash == nil || s.Account.ID == 0 {
		return
	}
	if err := h.AccountCash.SaveCash(ctx, s.Account.ID, s.CashBalance); err != nil {
		h.log().Error("gravar cash",
			"usuario", s.Username, "conta", s.Account.ID, "erro", err)
	}
}

// persistCashItems grava os slots do inventario premium.
func (h *Handler) persistCashItems(ctx context.Context, s *Session) {
	if h.Items == nil || s.Account.ID == 0 || len(s.Cash.Slots) == 0 {
		return
	}
	if err := h.Items.SaveCash(ctx, s.Account.ID, s.Cash); err != nil {
		h.log().Error("gravar inventario premium",
			"usuario", s.Username, "conta", s.Account.ID, "erro", err)
	}
}

// loadCash le o inventario premium e o saldo da conta.
func (h *Handler) loadCash(ctx context.Context, s *Session) store.ItemSet {
	vazio := store.ItemSet{Slots: make([]model.Item, store.CashSlots)}
	if h.Items == nil || s.Account.ID == 0 {
		return vazio
	}
	set, err := h.Items.Cash(ctx, s.Account.ID)
	if err != nil {
		h.log().Error("carregar inventario premium",
			"conta", s.Account.ID, "usuario", s.Username, "erro", err)
		return vazio
	}
	for _, a := range set.Avisos {
		h.log().Warn("inventario premium", "conta", s.Account.ID, "aviso", a)
	}
	if len(set.Slots) < store.CashSlots {
		completo := make([]model.Item, store.CashSlots)
		copy(completo, set.Slots)
		set.Slots = completo
	}
	return set
}
