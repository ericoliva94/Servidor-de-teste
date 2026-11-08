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

// Codigos da loja pessoal. O $318 e o mesmo do comercio (CodeTradeCancel):
// a origem usava um so pacote para fechar janela de troca e de loja.
const (
	CodeShopCreate = 0x319 // cliente -> servidor: abrir a minha loja; servidor -> cliente: vitrine
	CodeShopOpen   = 0x31A // cliente -> servidor: ver a loja de alguem
	CodeShopBuy    = 0x31B // cliente -> servidor: comprar um produto
)

const (
	// ShopProducts e o numero de vitrines de uma loja (TPersonalShopData.Products).
	ShopProducts = 10
	// shopEmptySlot marca vitrine sem produto.
	shopEmptySlot = 0xFFFF
	// shopHeaderIndex e o Header.Index que a origem punha na vitrine ($7535).
	shopHeaderIndex = 0x7535
)

// shopState e a loja pessoal de uma sessao: a que ela mantem aberta e a que
// ela esta olhando. Protegido por tradeMu, que ja serializa o comercio inteiro
// (loja e troca mexem nos mesmos inventarios).
type shopState struct {
	// own e a minha loja; own.Index == 0 quando fechada.
	own model.PersonalShopData
	// visiting e o clientID da loja que estou olhando, zero se nenhuma.
	visiting uint16
}

func (st *shopState) isOpen() bool { return st.own.Index != 0 }

// active diz se uma vitrine tem produto.
func shopActive(p model.PersonalShopItem) bool {
	return p.Slot != shopEmptySlot && p.Item.Index != 0
}

// emptyProduct e a vitrine vazia que o cliente espera ver.
func emptyProduct() model.PersonalShopItem {
	return model.PersonalShopItem{Slot: shopEmptySlot}
}

// ---- abrir a minha loja

// createShop e o porte de TPacketHandlers.CreatePersonalShop ($319).
//
// Defeitos da origem corrigidos:
//   - os itens da vitrine vinham do cliente: ele podia anunciar um item e
//     entregar outro. Aqui so o slot e o preco contam, e o item sai do
//     inventario do servidor;
//   - o slot apontava para item inexistente (vazio) sem recusa, e o indice 0 a
//     63 incluia as quatro bolsas;
//   - abrir loja durante troca, ou preco absurdo, passavam;
//   - o jogador saia sem fechar a loja e os visitantes ficavam com a vitrine.
func (h *Handler) createShop(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.PersonalShopPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	in := pkt.Shop
	nome := netsrv.CString(in.Name[:])
	if in.Index != uint32(s.ClientID) || nome == "" {
		return nil
	}
	if s.Trade.with != 0 {
		h.message(c, "Termine a troca antes de abrir a loja.")
		return nil
	}
	if s.Dead {
		return nil
	}

	t := h.data()
	var loja model.PersonalShopData
	loja.Index = in.Index
	loja.Name = in.Name
	usados := map[uint32]bool{}
	n := 0
	for i, prod := range in.Products {
		loja.Products[i] = emptyProduct()
		if prod.Slot == shopEmptySlot || prod.Slot >= uint32(MovableInventorySlots) {
			continue
		}
		it, ok := h.tradeSlotItem(s, uint8(prod.Slot))
		if !ok || itemfn.TypeTrade(t, it.Index) > 0 {
			continue // item inexistente ou preso: a vitrine fica vazia
		}
		if usados[prod.Slot] {
			h.closeOwnShopLocked(s)
			h.message(c, "Voce nao pode vender o mesmo item varias vezes.")
			return nil
		}
		if prod.Price > uint64(MaxGold) {
			h.message(c, "Preco invalido.")
			return nil
		}
		usados[prod.Slot] = true
		loja.Products[i] = model.PersonalShopItem{Price: prod.Price, Slot: prod.Slot, Item: it}
		n++
	}
	if n == 0 {
		h.message(c, "Nenhum item valido para vender.")
		return nil
	}

	// Trocar de loja: os visitantes da anterior perdem a vitrine.
	h.closeOwnShopLocked(s)
	s.Shop.own = loja
	s.Avatar.ShopName = nome
	h.respawnToVisible(s)
	h.sendShop(c, s.Shop.own)
	return nil
}

// sendShop manda a vitrine ($319). Porte de TPlayer.SendPersonalShop.
func (h *Handler) sendShop(c *netsrv.Conn, loja model.PersonalShopData) {
	var pkt packets.PersonalShopPacket
	pkt.Header.Index = shopHeaderIndex
	pkt.Shop = loja
	if err := c.SendStruct(CodeShopCreate, pkt); err != nil {
		h.log().Debug("falha ao enviar vitrine", "erro", err)
	}
}

// ---- ver a loja de alguem

// openShop e o porte de TPacketHandlers.OpenPersonalShop ($31A).
//
// A origem indexava Players[Data] com o valor do cliente e deixava abrir a loja
// de quem estava do outro lado do mapa. Aqui o dono precisa estar em jogo, com
// loja aberta e visivel para quem olha.
func (h *Handler) openShop(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	if pkt.Data == 0 || pkt.Data > 0xFFFF || uint16(pkt.Data) == s.ClientID {
		return nil
	}
	dono, _, ok := h.sessionOf(uint16(pkt.Data))
	if !ok || !dono.Shop.isOpen() || !s.Base.VisiblePlayers.Contains(dono.ClientID) {
		return nil
	}
	if s.Trade.with != 0 {
		h.message(c, "Termine a troca antes.")
		return nil
	}
	h.sendShop(c, dono.Shop.own)
	s.Shop.visiting = dono.ClientID
	return nil
}

// ---- comprar

// buyShopItem e o porte de TPacketHandlers.BuyPersonalShopItem ($31B).
//
// Defeitos da origem corrigidos:
//   - Packet.Slot indexava Products sem faixa;
//   - o produto era conferido contra o que o cliente mandou, nao contra o item
//     que o dono TEM agora: se ele moveu, vendeu ou apagou o item depois de abrir
//     a loja, o comprador pagava por um slot vazio ou por outro item;
//   - o ouro do dono somava sem teto;
//   - "inventario cheio" era conferido por GetEmptySlot, mas PutItem podia falhar
//     depois de o ouro ja ter sido cobrado.
func (h *Handler) buyShopItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.BuyPersonalShopItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	if pkt.Index == 0 || pkt.Index > 0xFFFF || uint16(pkt.Index) == s.ClientID ||
		s.Shop.visiting == 0 || s.Shop.visiting != uint16(pkt.Index) {
		return nil
	}
	dono, donoConn, ok := h.sessionOf(uint16(pkt.Index))
	if !ok || !dono.Shop.isOpen() {
		return nil
	}
	if pkt.Slot >= ShopProducts {
		h.message(c, "Item invalido.")
		return nil
	}
	prod := dono.Shop.own.Products[pkt.Slot]
	if !shopActive(prod) || prod != pkt.Product {
		h.message(c, "Item invalido.")
		return nil
	}

	// O item ainda esta no inventario do dono? Senao a vitrine esta velha.
	atual, ok := h.tradeSlotItem(dono, uint8(prod.Slot))
	if !ok || atual != prod.Item {
		dono.Shop.own.Products[pkt.Slot] = model.PersonalShopItem{}
		h.sendShop(c, dono.Shop.own)
		h.sendShop(donoConn, dono.Shop.own)
		h.message(c, "O item nao esta mais disponivel.")
		return nil
	}

	preco := int64(prod.Price) // ja limitado a MaxGold ao abrir a loja
	if preco > s.Avatar.Gold {
		h.message(c, "Gold insuficiente.")
		return nil
	}
	if dono.Avatar.Gold > MaxGold-preco {
		h.message(c, "O vendedor nao pode receber esse valor.")
		return nil
	}
	if !h.putItem(c, s, atual) {
		h.message(c, "Inventario cheio.")
		return nil
	}

	s.Avatar.Gold -= preco
	dono.Avatar.Gold += preco
	dono.Inventory.Slots[prod.Slot] = model.Item{}
	h.sendRefreshItem(donoConn, dono, store.SlotTypeInventory, uint16(prod.Slot), model.Item{})
	h.sendRefreshMoney(c, s)
	h.sendRefreshMoney(donoConn, dono)
	dono.Shop.own.Products[pkt.Slot] = model.PersonalShopItem{}

	h.message(c, "Item comprado com sucesso.")
	h.sendShop(c, dono.Shop.own)
	h.sendShop(donoConn, dono.Shop.own)
	h.log().Info("compra em loja pessoal",
		"comprador", s.Avatar.Name, "vendedor", dono.Avatar.Name,
		"item", atual.Index, "preco", preco)
	return nil
}

// ---- fechar

// closeShopOrTrade e o ramo de loja do TradeCancel ($318). Devolve true quando
// o pacote era sobre loja (aberta ou visitada) e portanto nao e de comercio.
//
// A origem testava primeiro "estou olhando outra loja": nesse caso so zerava a
// visita, sem eco ao cliente. Senao ecoava o $318 e, se o Header.Index fosse o
// da minha loja, a apagava.
func (h *Handler) closeShopOrTrade(c *netsrv.Conn, s *Session) bool {
	if s.Shop.visiting != 0 {
		s.Shop.visiting = 0
		return true
	}
	if !s.Shop.isOpen() {
		return false
	}
	h.closeOwnShopLocked(s)
	h.sendTradeClose(c, s)
	return true
}

// closeOwnShopLocked apaga a minha loja, tira o nome da tela e fecha a janela de
// quem a estava olhando. Precisa de tradeMu.
func (h *Handler) closeOwnShopLocked(s *Session) {
	if !s.Shop.isOpen() {
		return
	}
	s.Shop.own = model.PersonalShopData{}
	s.Avatar.ShopName = ""
	h.respawnToVisible(s)
	if h.Chan == nil {
		return
	}
	// A origem deixava os visitantes com a vitrine na tela.
	for _, id := range s.Base.VisiblePlayers.IDs() {
		v, vc, ok := h.sessionOf(id)
		if ok && v.Shop.visiting == s.ClientID {
			v.Shop.visiting = 0
			h.sendTradeClose(vc, v)
		}
	}
}

// shopOnDisconnect fecha a loja de quem saiu.
func (h *Handler) shopOnDisconnect(s *Session) {
	tradeMu.Lock()
	defer tradeMu.Unlock()
	h.closeOwnShopLocked(s)
	s.Shop.visiting = 0
}
