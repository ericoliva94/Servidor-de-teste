package game

import (
	"context"
	"fmt"

	"aika/internal/auction"
	"aika/internal/itemfn"
	"aika/internal/mail"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de leilao.
const (
	CodeAuctionRegister = 0x3F0B // nos dois sentidos: registrar / resultado
	CodeAuctionBuy      = 0x3F0C // nos dois sentidos: comprar / resultado
	CodeAuctionList     = 0x3F0D // nos dois sentidos: listar
	CodeAuctionCancel   = 0x3F10 // nos dois sentidos: cancelar / resultado
	CodeAuctionOwn      = 0x3F11 // nos dois sentidos: minhas ofertas
)

// Titulos das cartas que o leilao gera.
const (
	auctionBuyTitle    = "Leilao: item comprado"
	auctionSellTitle   = "Leilao: item vendido"
	auctionReturnTitle = "Leilao: item devolvido"
	auctionSender      = "Leilao"
)

// registerAuctionItem e o porte de
// TAuctionFunctions.RegisterAuctionItem ($3F0B).
//
// A origem fazia "RegisterItem := @Player.Base.Character.Inventory[Slot]" com
// Slot vindo do cliente como WORD e Inventory sendo ARRAY [0..63], SEM
// checagem de faixa: lia fora do vetor e, no fim, fazia
// "ZeroMemory(RegisterItem, sizeof(TItem))" — isto e, ESCREVIA 20 bytes fora
// do vetor, no meio do registro do personagem.
func (h *Handler) registerAuctionItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.AuctionDB == nil {
		h.message(c, "O leilao nao esta disponivel.")
		return nil
	}

	var pkt packets.AuctionRegisterItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	slot := int(pkt.Slot)
	if slot < 0 || slot >= store.InventorySlots || slot >= len(s.Inventory.Slots) {
		h.log().Warn("slot de leilao fora da faixa",
			"slot", pkt.Slot, "usuario", s.Username, "ip", c.IP())
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}
	it := s.Inventory.Get(slot)
	if it.IsEmpty() {
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}
	// Item marcado como nao comercializavel nao entra.
	if itemfn.TypeTrade(h.data(), it.Index) > 0 {
		h.message(c, "Item nao comercializavel.")
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}
	if pkt.Price == 0 || pkt.Time == 0 {
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}

	taxa := auction.Commission(pkt.Price, pkt.Time)
	if s.Avatar.Gold < int64(taxa) {
		h.message(c, "Ouro insuficiente para a taxa do leilao.")
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}

	indice, err := h.AuctionDB.NextIndex(ctx)
	if err != nil {
		h.log().Error("proximo indice de oferta", "erro", err)
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}
	oferta := auction.New(indice, s.CharID(), s.Avatar.Name, it,
		pkt.Price, pkt.Time, h.now())

	// A oferta e gravada antes de o item sair do inventario: uma falha aqui
	// nao pode tirar o item do jogador. Na origem o ZeroMemory vinha depois
	// das duas gravacoes, mas sem transacao entre elas — e o indice da oferta
	// saia de um "SELECT max(id)" feito FORA da transacao do insert, entao
	// dois registros ao mesmo tempo liam o mesmo id e a segunda oferta
	// apontava para o item da primeira.
	if err := h.AuctionDB.Register(ctx, oferta); err != nil {
		h.log().Error("registrar oferta",
			"usuario", s.Username, "item", it.Index, "erro", err)
		h.message(c, "Nao foi possivel registrar o item.")
		return h.sendAuctionResult(c, s, CodeAuctionRegister, BoolRefuse)
	}

	s.Inventory.Slots[slot] = model.Item{}
	s.Avatar.Gold = subGold(s.Avatar.Gold, int64(taxa))
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot),
		s.Inventory.Slots[slot])

	h.log().Info("item no leilao",
		"usuario", s.Username, "item", it.Index,
		"preco", pkt.Price, "horas", pkt.Time, "taxa", taxa, "oferta", indice)

	if err := h.sendAuctionResult(c, s, CodeAuctionRegister, BoolAccept); err != nil {
		return err
	}
	return h.sendOwnAuctionItems(ctx, c, s)
}

// listAuctionItems e o porte de TAuctionFunctions.GetAuctionItems ($3F0D).
func (h *Handler) listAuctionItems(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.AuctionDB == nil {
		return nil
	}
	ofertas, err := h.AuctionDB.Active(ctx, auction.MaxListItems)
	if err != nil {
		h.log().Error("listar leilao", "erro", err)
		return nil
	}

	var pkt packets.SendAuctionItemsPacket
	pkt.Header.Index = s.ClientID
	n := 0
	for i := range ofertas {
		if n >= len(pkt.Items) {
			break
		}
		pkt.Items[n] = h.offerToPacket(&ofertas[i])
		n++
	}
	pkt.ItemCount = uint32(n)
	return c.SendStruct(CodeAuctionList, pkt)
}

// sendOwnAuctionItems manda o $3F11 com as ofertas do proprio jogador.
func (h *Handler) sendOwnAuctionItems(ctx context.Context, c *netsrv.Conn, s *Session) error {
	if h.AuctionDB == nil {
		return nil
	}
	ofertas, err := h.AuctionDB.ByOwner(ctx, s.CharID(), auction.MaxOwnItems)
	if err != nil {
		h.log().Error("listar ofertas proprias", "erro", err)
		return nil
	}

	var pkt packets.CadastredItemsPacket
	pkt.Header.Index = s.ClientID
	n := 0
	for i := range ofertas {
		if n >= len(pkt.Items) {
			// A origem recusava a lista inteira com
			// "if (Length(Items) > 12) then Result := false": o jogador com
			// mais de doze ofertas nao via nenhuma.
			break
		}
		pkt.Items[n] = h.offerToPacket(&ofertas[i])
		n++
	}
	pkt.ItemCount = uint32(n)
	return c.SendStruct(CodeAuctionOwn, pkt)
}

// ownAuctionItems e o porte de TPacketHandlers.RequestOwnAuctionItems ($3F11).
func (h *Handler) ownAuctionItems(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	return h.sendOwnAuctionItems(ctx, c, s)
}

// buyAuctionItem e o porte de TAuctionFunctions.RequestBuyItem ($3F0C).
//
// A ordem aqui e a correcao central: na origem o ouro do comprador so era
// lido e conferido DEPOIS de RegisterAquisitionMail e
// RegisterSellerAquisitionMail terem criado as duas cartas e o item ja ter
// sido inserido no anexo do comprador. Um comprador sem ouro recebia o item e
// saia sem pagar — a funcao conferia o ouro, dava Exit, e as cartas ja
// estavam gravadas.
func (h *Handler) buyAuctionItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.AuctionDB == nil || h.MailDB == nil {
		h.message(c, "O leilao nao esta disponivel.")
		return nil
	}

	var pkt packets.AuctionCancelOfferPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.AuctionOfferId == 0 {
		return nil
	}

	oferta, err := h.AuctionDB.Get(ctx, pkt.AuctionOfferId)
	if err != nil {
		h.log().Error("buscar oferta", "oferta", pkt.AuctionOfferId, "erro", err)
		return h.sendAuctionResult(c, s, CodeAuctionBuy, BoolRefuse)
	}
	if err := auction.CanBuy(oferta, s.CharID(), s.Avatar.Gold, h.now()); err != nil {
		switch err {
		case auction.ErrNotEnoughGold:
			h.message(c, "Ouro insuficiente.")
		case auction.ErrOwnOffer:
			h.message(c, "Voce nao pode comprar a sua propria oferta.")
		default:
			h.message(c, "Esta oferta nao esta mais disponivel.")
		}
		return h.sendAuctionResult(c, s, CodeAuctionBuy, BoolRefuse)
	}

	// Fecha a oferta primeiro: isso e o que impede dois compradores de levar
	// o mesmo item. A origem conferia a oferta com um SELECT e so depois
	// gravava, sem nada entre as duas coisas.
	fechou, err := h.AuctionDB.Close(ctx, oferta.Index, s.CharID())
	if err != nil {
		h.log().Error("fechar oferta", "oferta", oferta.Index, "erro", err)
		return h.sendAuctionResult(c, s, CodeAuctionBuy, BoolRefuse)
	}
	if !fechou {
		// Outro comprador chegou primeiro.
		h.message(c, "Esta oferta nao esta mais disponivel.")
		return h.sendAuctionResult(c, s, CodeAuctionBuy, BoolRefuse)
	}

	// Oferta fechada: o item vai ao comprador e o ouro ao vendedor, os dois
	// por carta, como na origem.
	cartaItem := h.auctionMail(ctx, auctionBuyTitle,
		fmt.Sprintf("Item comprado no leilao de %s.", oferta.SellerName))
	cartaItem.Items[0] = oferta.Item
	cartaItem.CheckItem = true
	cartaItem.Return = true
	cartaItem.Auction = true
	if err := h.MailDB.Add(ctx, s.Avatar.Name, cartaItem); err != nil {
		// A oferta ja esta fechada; reabrir seria pior, porque o ouro ainda
		// nao saiu. Registra alto para o item nao se perder em silencio.
		h.log().Error("entregar item do leilao",
			"oferta", oferta.Index, "comprador", s.Avatar.Name, "erro", err)
		h.message(c, "Falha ao entregar o item. Procure o suporte.")
		return h.sendAuctionResult(c, s, CodeAuctionBuy, BoolRefuse)
	}

	cartaOuro := h.auctionMail(ctx, auctionSellTitle,
		fmt.Sprintf("Item vendido no leilao para %s.", s.Avatar.Name))
	cartaOuro.Gold = oferta.Price
	cartaOuro.Return = true
	cartaOuro.Auction = true
	if err := h.MailDB.AddTo(ctx, oferta.SellerCharIndex, cartaOuro); err != nil {
		h.log().Error("entregar ouro do leilao",
			"oferta", oferta.Index, "vendedor", oferta.SellerCharIndex, "erro", err)
	}

	// Por ultimo o ouro do comprador sai.
	s.Avatar.Gold = subGold(s.Avatar.Gold, int64(oferta.Price))

	h.log().Info("item comprado no leilao",
		"oferta", oferta.Index, "comprador", s.Avatar.Name,
		"vendedor", oferta.SellerName, "preco", oferta.Price)

	if err := h.sendAuctionResult(c, s, CodeAuctionBuy, BoolAccept); err != nil {
		return err
	}
	_ = h.sendMailList(ctx, c, s)
	return h.listAuctionItems(ctx, c, s, p)
}

// cancelAuctionOffer e o porte de TAuctionFunctions.CancelItemOffer ($3F10).
func (h *Handler) cancelAuctionOffer(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.AuctionDB == nil || h.MailDB == nil {
		return nil
	}

	var pkt packets.AuctionCancelOfferPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.AuctionOfferId == 0 {
		return nil
	}

	oferta, err := h.AuctionDB.Get(ctx, pkt.AuctionOfferId)
	if err != nil {
		h.log().Error("buscar oferta", "oferta", pkt.AuctionOfferId, "erro", err)
		return h.sendAuctionResult(c, s, CodeAuctionCancel, BoolRefuse)
	}
	// A origem nao conferia o dono da oferta no cancelamento: qualquer
	// jogador podia cancelar a oferta de qualquer outro mandando o id.
	if err := auction.CanCancel(oferta, s.CharID()); err != nil {
		if err == auction.ErrNotOwner {
			h.log().Warn("cancelamento de oferta de outro personagem",
				"oferta", pkt.AuctionOfferId, "usuario", s.Username, "ip", c.IP())
		}
		return h.sendAuctionResult(c, s, CodeAuctionCancel, BoolRefuse)
	}

	fechou, err := h.AuctionDB.Close(ctx, oferta.Index, 0)
	if err != nil {
		h.log().Error("fechar oferta", "oferta", oferta.Index, "erro", err)
		return h.sendAuctionResult(c, s, CodeAuctionCancel, BoolRefuse)
	}
	if !fechou {
		return h.sendAuctionResult(c, s, CodeAuctionCancel, BoolRefuse)
	}

	// O item volta por carta, nao direto ao inventario: o inventario pode
	// estar cheio, e a origem perdia o item nesse caso.
	carta := h.auctionMail(ctx, auctionReturnTitle,
		"Oferta cancelada: item devolvido.")
	carta.Items[0] = oferta.Item
	carta.CheckItem = true
	carta.Return = true
	carta.Auction = true
	if err := h.MailDB.AddTo(ctx, s.CharID(), carta); err != nil {
		h.log().Error("devolver item do leilao",
			"oferta", oferta.Index, "erro", err)
		h.message(c, "Falha ao devolver o item. Procure o suporte.")
		return h.sendAuctionResult(c, s, CodeAuctionCancel, BoolRefuse)
	}

	h.log().Info("oferta cancelada",
		"oferta", oferta.Index, "usuario", s.Username)
	if err := h.sendAuctionResult(c, s, CodeAuctionCancel, BoolAccept); err != nil {
		return err
	}
	_ = h.sendMailList(ctx, c, s)
	return h.sendOwnAuctionItems(ctx, c, s)
}

// auctionMail monta a carta base que o leilao usa.
func (h *Handler) auctionMail(ctx context.Context, titulo, texto string) mail.Mail {
	indice, err := h.MailDB.NextIndex(ctx)
	if err != nil {
		h.log().Error("proximo indice de carta do leilao", "erro", err)
	}
	return mail.Mail{
		Index:    indice,
		Nick:     auctionSender,
		Title:    titulo,
		Text:     texto,
		SentAt:   h.now(),
		ReturnAt: h.now().AddDate(0, 0, mail.ReturnDays),
	}
}

// offerToPacket converte a oferta para o formato do pacote.
func (h *Handler) offerToPacket(o *auction.Offer) packets.AuctionItemData {
	var out packets.AuctionItemData
	out.SellerCharacterIndex = o.SellerCharIndex
	out.AuctionIndex = o.Index
	out.ItemPrice = o.Price
	netsrv.PutCString(out.SellerCharacterName[:], o.SellerName)
	if !o.ExpiresAt.IsZero() {
		netsrv.PutCString(out.ExpireDate[:], o.ExpiresAt.Format("2006-01-02"))
	}
	out.Item = o.Item
	return out
}

// sendAuctionResult manda o resultado de uma operacao de leilao.
func (h *Handler) sendAuctionResult(c *netsrv.Conn, s *Session, code uint16, resultado uint32) error {
	var pkt packets.SignalData
	pkt.Header.Index = s.ClientID
	pkt.Data = resultado
	return c.SendStruct(code, pkt)
}

// AuctionExpireTick devolve ao vendedor, por carta, os itens das ofertas que
// passaram do prazo.
//
// A origem tinha TAuctionFunctions.CheckAuctionItems, cujo corpo e so
// "Result := True" — isto e, a oferta vencida ficava no banco para sempre e o
// item nunca voltava ao vendedor.
func (h *Handler) AuctionExpireTick(ctx context.Context) int {
	if h.AuctionDB == nil || h.MailDB == nil {
		return 0
	}
	vencidas, err := h.AuctionDB.Expired(ctx, h.now())
	if err != nil {
		h.log().Error("buscar ofertas vencidas", "erro", err)
		return 0
	}
	n := 0
	for i := range vencidas {
		o := &vencidas[i]
		fechou, err := h.AuctionDB.Close(ctx, o.Index, 0)
		if err != nil || !fechou {
			continue
		}
		carta := h.auctionMail(ctx, auctionReturnTitle,
			"A oferta expirou: item devolvido.")
		carta.Items[0] = o.Item
		carta.CheckItem = true
		carta.Return = true
		carta.Auction = true
		if err := h.MailDB.AddTo(ctx, o.SellerCharIndex, carta); err != nil {
			h.log().Error("devolver item de oferta vencida",
				"oferta", o.Index, "erro", err)
			continue
		}
		n++
	}
	return n
}
