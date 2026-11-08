package game

import (
	"context"
	"sync"

	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos do comercio entre jogadores.
const (
	CodeTradeRequest  = 0x315 // os dois sentidos: convite
	CodeTradeResponse = 0x316 // os dois sentidos: aceitar ou recusar
	CodeTradeRefresh  = 0x317 // os dois sentidos: o que cada lado oferece
	CodeTradeCancel   = 0x318 // os dois sentidos: fechar a janela
)

// Respostas do convite, TRADE_* de GlobalDefs.pas.
const (
	tradeRefuse = 0
	tradeAccept = 1
)

// TradeSlots e quantos itens cada lado oferece (TTrade.Itens e Slots).
const TradeSlots = 10

// tradeEmpty marca slot sem item na oferta.
const tradeEmpty = 0xFF

// tradeOffer e o que um lado oferece. Os itens NAO vem do cliente: o cliente
// diz so de quais slots do inventario, e o servidor guarda uma fotografia do
// que havia ali (snap) para conferir na hora de executar.
type tradeOffer struct {
	slots   [TradeSlots]uint8
	snap    [TradeSlots]model.Item
	gold    int64
	ready   bool
	confirm bool
}

func emptyOffer() tradeOffer {
	var o tradeOffer
	for i := range o.slots {
		o.slots[i] = tradeEmpty
	}
	return o
}

// tradeState e a negociacao de uma sessao.
type tradeState struct {
	with    uint16 // clientID de quem negocia comigo, 0 se ninguem
	pending uint16 // clientID de quem me convidou e espera resposta
	offer   tradeOffer
}

// tradeMu serializa o comercio inteiro: uma troca mexe nas duas sessoes e, sem
// trava, duas goroutines (uma por jogador) leriam e gravariam os dois
// inventarios ao mesmo tempo. Trocas sao raras, entao uma trava so basta.
var tradeMu sync.Mutex

func (h *Handler) sessionOf(id uint16) (*Session, *netsrv.Conn, bool) {
	if h.Chan == nil || id == 0 {
		return nil, nil, false
	}
	s, c, ok := h.Chan.Lookup(id)
	if !ok || s.Status < Playing {
		return nil, nil, false
	}
	return s, c, true
}

// ---- convite

// tradeRequest e o porte de TPacketHandlers.TradeRequest ($315).
//
// A origem indexava Players[Data] com o valor do cliente e deixava convidar a
// si mesmo ou alguem que nao estava em jogo. O convite fica guardado no
// convidado, para o $316 so valer como resposta a ele.
func (h *Handler) tradeRequest(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	alvo, alvoConn, ok := h.sessionOf(uint16(pkt.Data))
	if pkt.Data > 0xFFFF || !ok || alvo == s {
		h.message(c, "Voce nao pode negociar com esse jogador.")
		return nil
	}
	if s.Trade.with != 0 || alvo.Trade.with != 0 {
		h.message(c, "Jogador ja esta em negociacao.")
		return nil
	}
	// Loja aberta e troca mexem nos mesmos itens: uma de cada vez.
	if s.Shop.isOpen() || alvo.Shop.isOpen() {
		h.message(c, "Voce nao pode negociar com esse jogador.")
		return nil
	}
	alvo.Trade.pending = s.ClientID

	conv := packets.SignalData{Data: uint32(s.ClientID)}
	conv.Header.Index = s.ClientID
	_ = alvoConn.SendStruct(CodeTradeRequest, conv)
	return nil
}

// tradeResponse e o porte de TPacketHandlers.TradeResponse ($316).
//
// A origem gravava TradingWith com o numero que o cliente mandou ANTES de
// conferir qualquer coisa: qualquer jogador podia mandar o $316 com o id de
// outro e abrir uma negociacao que ninguem pediu. Aqui so vale como resposta
// ao convite guardado.
func (h *Handler) tradeResponse(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.ResponseTradePacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	if pkt.OtherClient > 0xFFFF || s.Trade.pending == 0 || s.Trade.pending != uint16(pkt.OtherClient) {
		return nil
	}
	quem := s.Trade.pending
	s.Trade.pending = 0
	outro, outroConn, ok := h.sessionOf(quem)
	if !ok {
		h.message(c, "Jogador nao encontrado.")
		return nil
	}

	resp := pkt
	resp.OtherClient = uint32(s.ClientID)
	resp.Header.Index = s.ClientID

	switch pkt.Response {
	case tradeAccept:
		if s.Trade.with != 0 || outro.Trade.with != 0 {
			h.message(c, "Jogador ja esta em negociacao.")
			return nil
		}
		_ = outroConn.SendStruct(CodeTradeResponse, resp)
		s.Trade = tradeState{with: outro.ClientID, offer: emptyOffer()}
		outro.Trade = tradeState{with: s.ClientID, offer: emptyOffer()}
		h.sendTrade(c, s, s, true)
		h.sendTrade(outroConn, s, outro, false)
		h.sendTrade(outroConn, outro, outro, true)
		h.sendTrade(c, outro, s, false)
	case tradeRefuse:
		h.message(outroConn, "Pedido de troca recusado!")
		_ = outroConn.SendStruct(CodeTradeResponse, resp)
	}
	return nil
}

// sendTrade manda o $317 com a oferta de "dono" a "para". self diz se o pacote
// vai ao proprio dono (RefreshTrade) ou ao outro lado (RefreshTradeTo); muda so
// o campo OtherClientid, como na origem.
func (h *Handler) sendTrade(c *netsrv.Conn, dono, para *Session, self bool) {
	o := dono.Trade.offer
	var pkt packets.TradePacket
	pkt.Header.Index = dono.ClientID
	pkt.Trade.Slots = o.slots
	pkt.Trade.Gold = o.gold
	if o.ready {
		pkt.Trade.Ready = 1
	}
	if o.confirm {
		pkt.Trade.Confirm = 1
	}
	// Os itens saem do inventario do servidor, nunca do que o cliente disse.
	for i, slot := range o.slots {
		if slot == tradeEmpty {
			continue
		}
		pkt.Trade.Itens[i] = o.snap[i]
	}
	if self {
		pkt.Trade.OtherClientid = dono.Trade.with
	} else {
		pkt.Trade.OtherClientid = dono.ClientID
	}
	_ = c.SendStruct(CodeTradeRefresh, pkt)
}

// ---- oferta

// readOffer valida os slots e o ouro que o cliente mandou e monta a oferta.
// Devolve a mensagem de recusa quando algo nao serve; cancel diz que o item
// nao e trocavel e a negociacao inteira deve fechar (como na origem).
func (h *Handler) readOffer(s *Session, in model.Trade) (o tradeOffer, msg string, cancel bool) {
	t := h.data()
	o = emptyOffer()
	seen := map[uint8]bool{}
	for i, slot := range in.Slots {
		if slot == tradeEmpty {
			continue
		}
		it, ok := h.tradeSlotItem(s, slot)
		if !ok || seen[slot] {
			return o, "Itens da troca invalidos.", false
		}
		seen[slot] = true
		if itemfn.TypeTrade(t, it.Index) > 0 {
			return o, "Nao e possivel trocar itens que nao sao trocaveis.", true
		}
		o.slots[i], o.snap[i] = slot, it
	}
	if in.Gold < 0 || in.Gold > s.Avatar.Gold {
		return o, "Voce nao possui esse ouro.", false
	}
	o.gold = in.Gold
	return o, "", false
}

// tradeSlotItem devolve o item de um slot ofertavel do inventario (0 a 59,
// com a bolsa no lugar).
func (h *Handler) tradeSlotItem(s *Session, slot uint8) (model.Item, bool) {
	if int(slot) >= MovableInventorySlots || int(slot) >= len(s.Inventory.Slots) {
		return model.Item{}, false
	}
	if bolsa := inventoryBagSlot(int(slot)); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
		return model.Item{}, false
	}
	it := s.Inventory.Slots[slot]
	return it, !it.IsEmpty()
}

// freeSlots conta os slots livres do inventario (com bolsa).
func (h *Handler) freeSlots(s *Session) int {
	n := 0
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if bolsa := inventoryBagSlot(i); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		if s.Inventory.Slots[i].IsEmpty() {
			n++
		}
	}
	return n
}

func (o tradeOffer) count() int {
	n := 0
	for _, s := range o.slots {
		if s != tradeEmpty {
			n++
		}
	}
	return n
}

// tradeRefresh e o porte de TPacketHandlers.TradeRefresh ($317).
//
// Defeitos da origem corrigidos:
//   - os ITENS vinham do pacote do cliente e eram repassados ao outro lado e
//     gravados como oferta: o cliente podia mostrar um item valioso e entregar
//     outro. Aqui so os slots contam, e os itens saem do inventario do servidor;
//   - mudar a oferta nao desfazia a confirmacao do outro lado: dava para
//     esperar o parceiro confirmar e trocar o item. Agora qualquer mudanca
//     desfaz "pronto" e "confirmado" dos dois;
//   - ExecuteTrade tinha um erro de copia (o segundo `if` testava os itens do
//     PROPRIO jogador), movia itens antes de conferir o ouro e saia no meio se
//     faltasse ouro: itens trocados sem pagamento;
//   - slot do cliente indexava o inventario (64) sem faixa.
func (h *Handler) tradeRefresh(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.TradePacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()

	outro, outroConn, ok := h.partner(s)
	if !ok {
		return nil
	}

	nova, msg, cancel := h.readOffer(s, pkt.Trade)
	if cancel {
		h.message(c, msg)
		h.message(outroConn, msg)
		h.closeBoth(c, s, outroConn, outro)
		return nil
	}
	if msg != "" {
		h.message(c, msg)
		return nil
	}

	mudou := nova.slots != s.Trade.offer.slots || nova.gold != s.Trade.offer.gold
	nova.ready, nova.confirm = pkt.Trade.Ready != 0, pkt.Trade.Confirm != 0
	if mudou {
		// Oferta nova: o outro lado precisa conferir e confirmar de novo.
		outro.Trade.offer.ready, outro.Trade.offer.confirm = false, false
	}
	s.Trade.offer = nova

	// Quem esta pronto precisa que o outro lado tenha lugar para o que recebe;
	// os slots que o outro esta oferecendo tambem ficam livres.
	if nova.ready && h.freeSlots(outro)+outro.Trade.offer.count() < nova.count() {
		h.message(outroConn, "Libere mais espaco no inventario antes.")
		h.message(c, "O inventario do alvo nao possui espaco suficiente.")
		s.Trade.offer.ready, outro.Trade.offer.ready = false, false
		h.sendTrade(outroConn, s, outro, false)
		return nil
	}

	if s.Trade.offer.confirm && outro.Trade.offer.confirm {
		h.executeTrade(c, s, outroConn, outro)
		return nil
	}

	h.sendTrade(outroConn, s, outro, false)
	if mudou {
		h.sendTrade(c, outro, s, false) // o parceiro voltou a "nao confirmado"
	}
	return nil
}

// partner devolve a sessao com quem s negocia, conferindo os dois lados.
func (h *Handler) partner(s *Session) (*Session, *netsrv.Conn, bool) {
	if s.Trade.with == 0 {
		return nil, nil, false
	}
	outro, outroConn, ok := h.sessionOf(s.Trade.with)
	if !ok || outro.Trade.with != s.ClientID {
		s.Trade = tradeState{}
		return nil, nil, false
	}
	return outro, outroConn, true
}

// closeBoth fecha a janela dos dois e zera a negociacao.
func (h *Handler) closeBoth(c *netsrv.Conn, s *Session, oc *netsrv.Conn, o *Session) {
	s.Trade, o.Trade = tradeState{}, tradeState{}
	h.sendTradeClose(c, s)
	h.sendTradeClose(oc, o)
}

func (h *Handler) sendTradeClose(c *netsrv.Conn, s *Session) {
	pkt := packets.SignalData{}
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(CodeTradeCancel, pkt)
}

// executeTrade conclui a troca. Tudo ou nada: confere de novo itens, ouro e
// lugar nos dois inventarios e, se qualquer coisa falhar, devolve os dois
// inventarios ao que eram e cancela.
func (h *Handler) executeTrade(c *netsrv.Conn, s *Session, oc *netsrv.Conn, o *Session) {
	fail := func(msg string) {
		h.message(c, msg)
		h.message(oc, msg)
		h.closeBoth(c, s, oc, o)
	}
	for _, side := range []*Session{s, o} {
		off := side.Trade.offer
		for i, slot := range off.slots {
			if slot == tradeEmpty {
				continue
			}
			it, ok := h.tradeSlotItem(side, slot)
			if !ok || it != off.snap[i] {
				fail("A oferta mudou. Troca cancelada.")
				return
			}
		}
		if off.gold < 0 || off.gold > side.Avatar.Gold {
			fail("Ouro insuficiente. Troca cancelada.")
			return
		}
	}
	if s.Avatar.Gold-s.Trade.offer.gold+o.Trade.offer.gold > MaxGold ||
		o.Avatar.Gold-o.Trade.offer.gold+s.Trade.offer.gold > MaxGold {
		fail("Limite de ouro excedido. Troca cancelada.")
		return
	}

	antesS := append([]model.Item(nil), s.Inventory.Slots...)
	antesO := append([]model.Item(nil), o.Inventory.Slots...)
	restore := func() {
		copy(s.Inventory.Slots, antesS)
		copy(o.Inventory.Slots, antesO)
	}

	for _, par := range [2]struct{ de, para *Session }{{s, o}, {o, s}} {
		for _, slot := range par.de.Trade.offer.slots {
			if slot != tradeEmpty {
				par.de.Inventory.Slots[slot] = model.Item{}
			}
		}
	}
	for _, par := range [2]struct {
		de, para *Session
		pc       *netsrv.Conn
	}{{s, o, oc}, {o, s, c}} {
		for i, slot := range par.de.Trade.offer.slots {
			if slot == tradeEmpty {
				continue
			}
			if !h.putItem(par.pc, par.para, par.de.Trade.offer.snap[i]) {
				restore()
				fail("Inventario cheio. Troca cancelada.")
				return
			}
		}
	}

	gs, gOut := s.Trade.offer.gold, o.Trade.offer.gold
	s.Avatar.Gold += gOut - gs
	o.Avatar.Gold += gs - gOut

	// Atualiza no cliente tudo que mudou (slots esvaziados e destinos).
	for _, par := range [2]struct {
		sess  *Session
		conn  *netsrv.Conn
		antes []model.Item
	}{{s, c, antesS}, {o, oc, antesO}} {
		for i := 0; i < MovableInventorySlots && i < len(par.antes); i++ {
			if par.sess.Inventory.Slots[i] != par.antes[i] {
				h.sendRefreshItem(par.conn, par.sess, store.SlotTypeInventory, uint16(i), par.sess.Inventory.Slots[i])
			}
		}
		h.sendRefreshMoney(par.conn, par.sess)
	}

	h.closeBoth(c, s, oc, o)
	h.message(c, "Troca realizada com sucesso.")
	h.message(oc, "Troca realizada com sucesso.")
	h.log().Info("troca concluida",
		"a", s.Avatar.Name, "b", o.Avatar.Name, "ouro_a", gs, "ouro_b", gOut)
}

// ---- cancelar

// tradeCancel e o porte da parte de comercio de TradeCancel ($318): fecha a
// janela do outro lado e zera a minha negociacao. (A origem tambem fechava
// loja pessoal aqui; ela ainda nao esta portada.)
func (h *Handler) tradeCancel(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	tradeMu.Lock()
	defer tradeMu.Unlock()
	// A origem tratava loja pessoal aqui primeiro: o mesmo $318 fecha as duas.
	if h.closeShopOrTrade(c, s) {
		return nil
	}
	h.cancelTradeLocked(s)
	return nil
}

func (h *Handler) cancelTradeLocked(s *Session) {
	with := s.Trade.with
	s.Trade = tradeState{}
	if with == 0 {
		return
	}
	if outro, oc, ok := h.sessionOf(with); ok && outro.Trade.with == s.ClientID {
		outro.Trade = tradeState{}
		h.sendTradeClose(oc, outro)
	}
}

// tradeOnDisconnect desfaz a negociacao de quem saiu. Na origem o parceiro
// ficava com a janela aberta e TradingWith apontando para uma conexao morta.
func (h *Handler) tradeOnDisconnect(s *Session) {
	tradeMu.Lock()
	defer tradeMu.Unlock()
	h.cancelTradeLocked(s)
	s.Trade.pending = 0
}
