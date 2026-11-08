package game

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/store"
)

const (
	itemEspadaTroca = 800 // nao empilha
	itemPocaoTroca  = 801 // empilha
	itemPreso       = 802 // TypeTrade 1: nao troca
	itemFalso       = 803 // o item "valioso" que o cliente tenta mostrar
)

func prepTroca(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(itemEspadaTroca, gamedata.ItemFromList{ItemType: 6})
	tab.poeItem(itemPocaoTroca, gamedata.ItemFromList{CanAgroup: 1})
	tab.poeItem(itemPreso, gamedata.ItemFromList{ItemType: 3, TypeTrade: 1})
	tab.poeItem(itemFalso, gamedata.ItemFromList{ItemType: 6})
	ae, be := novoEspiao(t), novoEspiao(t)
	a, b := sessaoComInventario(), sessaoComInventario()
	a.Avatar.Name, b.Avatar.Name = "Alfa", "Beta"
	a.Avatar.Gold, b.Avatar.Gold = 1000, 500
	if _, err := h.Chan.Join(ae.conn, a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Chan.Join(be.conn, b); err != nil {
		t.Fatal(err)
	}
	return h, a, ae, b, be
}

func tradeIn(t *testing.T, code uint16, v any) []byte {
	t.Helper()
	return miscPacket(code, v)
}

func convida(t *testing.T, h *Handler, de *Session, dee *espiao, para *Session) {
	t.Helper()
	if err := h.tradeRequest(context.Background(), dee.conn, de, asPacket(t,
		tradeIn(t, CodeTradeRequest, packets.SignalData{Data: uint32(para.ClientID)}))); err != nil {
		t.Fatal(err)
	}
}

func responde(t *testing.T, h *Handler, quem *Session, e *espiao, outro uint32, resp uint32) {
	t.Helper()
	if err := h.tradeResponse(context.Background(), e.conn, quem, asPacket(t,
		tradeIn(t, CodeTradeResponse, packets.ResponseTradePacket{OtherClient: outro, Response: resp}))); err != nil {
		t.Fatal(err)
	}
}

// abre leva os dois jogadores a uma negociacao aberta.
func abre(t *testing.T, h *Handler, a *Session, ae *espiao, b *Session, be *espiao) {
	t.Helper()
	convida(t, h, a, ae, b)
	responde(t, h, b, be, uint32(a.ClientID), tradeAccept)
	ae.drena(20 * time.Millisecond)
	be.drena(20 * time.Millisecond)
}

func oferece(t *testing.T, h *Handler, s *Session, e *espiao, slots []uint8, gold int64, ready, confirm bool) {
	t.Helper()
	var tr model.Trade
	for i := range tr.Slots {
		tr.Slots[i] = tradeEmpty
	}
	copy(tr.Slots[:], slots)
	tr.Gold = gold
	if ready {
		tr.Ready = 1
	}
	if confirm {
		tr.Confirm = 1
	}
	// O que o cliente diz sobre os itens e ignorado: poe um item falso.
	tr.Itens[0] = model.Item{Index: itemFalso}
	if err := h.tradeRefresh(context.Background(), e.conn, s, asPacket(t,
		tradeIn(t, CodeTradeRefresh, packets.TradePacket{Trade: tr}))); err != nil {
		t.Fatal(err)
	}
}

// ---- convite e resposta

func TestConviteChegaAoAlvoComOIdDeQuemConvidou(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	convida(t, h, a, ae, b)
	got := decodifica[packets.SignalData](t, proximoDe(t, be, CodeTradeRequest))
	if got.Data != uint32(a.ClientID) || got.Header.Index != a.ClientID {
		t.Errorf("convite = %+v", got)
	}
	if b.Trade.pending != a.ClientID {
		t.Error("o convite nao ficou guardado")
	}
}

func TestConviteRecusas(t *testing.T) {
	h, a, ae, b, _ := prepTroca(t)
	for _, alvo := range []uint32{uint32(a.ClientID), 0, 40, 0xFFFF, 0x10000, 0xFFFFFFFF} {
		if err := h.tradeRequest(context.Background(), ae.conn, a, asPacket(t,
			tradeIn(t, CodeTradeRequest, packets.SignalData{Data: alvo}))); err != nil {
			t.Fatal(err)
		}
		if got := mensagemDe(t, ae); got != "Voce nao pode negociar com esse jogador." {
			t.Errorf("alvo %d: mensagem = %q", alvo, got)
		}
	}
	b.Trade.with = 99
	convida(t, h, a, ae, b)
	if got := mensagemDe(t, ae); got != "Jogador ja esta em negociacao." {
		t.Errorf("ocupado: %q", got)
	}
}

func TestAceitarAbreANegociacaoNosDoisLados(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	convida(t, h, a, ae, b)
	be.drena(20 * time.Millisecond)

	responde(t, h, b, be, uint32(a.ClientID), tradeAccept)

	if a.Trade.with != b.ClientID || b.Trade.with != a.ClientID {
		t.Fatalf("with = %d/%d", a.Trade.with, b.Trade.with)
	}
	r := decodifica[packets.ResponseTradePacket](t, proximoDe(t, ae, CodeTradeResponse))
	if r.OtherClient != uint32(b.ClientID) || r.Response != tradeAccept {
		t.Errorf("resposta ao convidante = %+v", r)
	}
	for _, e := range []*espiao{ae, be} {
		tp := decodifica[packets.TradePacket](t, proximoDe(t, e, CodeTradeRefresh))
		if tp.Trade.Slots[0] != tradeEmpty {
			t.Errorf("oferta inicial nao esta vazia: %v", tp.Trade.Slots)
		}
	}
}

// Defeito da origem: o $316 gravava TradingWith com o numero do cliente antes
// de conferir qualquer coisa, abrindo negociacao que ninguem pediu.
func TestRespostaSemConviteNaoAbreNegociacao(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	responde(t, h, b, be, uint32(a.ClientID), tradeAccept) // a nunca convidou
	responde(t, h, b, be, 0xFFFFFFFF, tradeAccept)
	responde(t, h, b, be, 0, tradeAccept)
	if a.Trade.with != 0 || b.Trade.with != 0 {
		t.Fatal("negociacao aberta sem convite")
	}
	ae.nada(t, 20*time.Millisecond)

	convida(t, h, a, ae, b)
	responde(t, h, b, be, 999, tradeAccept) // convite de outro id
	if b.Trade.with != 0 {
		t.Error("aceitou convite que nao e dele")
	}
}

func TestConviteSoVale_UmaResposta(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	abre(t, h, a, ae, b, be)
	a.Trade, b.Trade = tradeState{}, tradeState{}
	responde(t, h, b, be, uint32(a.ClientID), tradeAccept)
	if b.Trade.with != 0 {
		t.Error("o mesmo convite serviu duas vezes")
	}
}

func TestRecusarAvisaEZeraOConvite(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	convida(t, h, a, ae, b)
	be.drena(20 * time.Millisecond)
	responde(t, h, b, be, uint32(a.ClientID), tradeRefuse)
	if got := mensagemDe(t, ae); got != "Pedido de troca recusado!" {
		t.Errorf("mensagem = %q", got)
	}
	if a.Trade.with != 0 || b.Trade.with != 0 || b.Trade.pending != 0 {
		t.Error("estado sobrou depois de recusar")
	}
}

// ---- oferta

// Defeito da origem: os itens vinham do pacote e eram repassados; o cliente
// podia mostrar um item e entregar outro.
func TestOutroLadoVeOItemRealENaoOQueOClienteDisse(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca, Refi: 32}
	abre(t, h, a, ae, b, be)

	oferece(t, h, a, ae, []uint8{0}, 0, false, false)

	tp := decodifica[packets.TradePacket](t, proximoDe(t, be, CodeTradeRefresh))
	if tp.Trade.Itens[0].Index != itemEspadaTroca || tp.Trade.Itens[0].Refi != 32 {
		t.Errorf("o outro lado viu %+v, queria a espada real", tp.Trade.Itens[0])
	}
	if tp.Header.Index != a.ClientID || tp.Trade.OtherClientid != a.ClientID {
		t.Errorf("cabecalho = %d outro=%d", tp.Header.Index, tp.Trade.OtherClientid)
	}
}

func TestOfertaInvalidaNaoEGuardada(t *testing.T) {
	casos := []struct {
		nome  string
		slots []uint8
		gold  int64
		msg   string
	}{
		{"slot vazio", []uint8{5}, 0, "Itens da troca invalidos."},
		{"slot fora", []uint8{200}, 0, "Itens da troca invalidos."},
		{"slot de bolsa", []uint8{61}, 0, "Itens da troca invalidos."},
		{"slot repetido", []uint8{0, 0}, 0, "Itens da troca invalidos."},
		{"ouro demais", []uint8{0}, 1001, "Voce nao possui esse ouro."},
		{"ouro negativo", nil, -5, "Voce nao possui esse ouro."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, a, ae, b, be := prepTroca(t)
			a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
			abre(t, h, a, ae, b, be)
			oferece(t, h, a, ae, c.slots, c.gold, false, false)
			if got := mensagemDe(t, ae); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if a.Trade.offer.count() != 0 || a.Trade.offer.gold != 0 {
				t.Error("oferta invalida foi guardada")
			}
			be.nada(t, 20*time.Millisecond)
		})
	}
}

func TestItemNaoTrocavelFechaANegociacaoDosDois(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemPreso}
	abre(t, h, a, ae, b, be)
	oferece(t, h, a, ae, []uint8{0}, 0, false, false)
	proximoDe(t, ae, CodeTradeCancel)
	proximoDe(t, be, CodeTradeCancel)
	if a.Trade.with != 0 || b.Trade.with != 0 {
		t.Error("a negociacao continuou aberta")
	}
}

// Defeito da origem: mudar a oferta nao desfazia a confirmacao do parceiro.
func TestMudarAOfertaDesfazAConfirmacaoDoOutroLado(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	a.Inventory.Slots[1] = model.Item{Index: itemPocaoTroca, Refi: 5}
	abre(t, h, a, ae, b, be)
	oferece(t, h, a, ae, []uint8{0}, 0, true, false)
	oferece(t, h, b, be, nil, 0, true, true) // b confirma
	if !b.Trade.offer.confirm {
		t.Fatal("b deveria estar confirmado")
	}

	oferece(t, h, a, ae, []uint8{1}, 0, true, false) // a troca a oferta

	if b.Trade.offer.confirm || b.Trade.offer.ready {
		t.Error("a confirmacao de b deveria ter sido desfeita")
	}
	tp := decodifica[packets.TradePacket](t, proximoDe(t, ae, CodeTradeRefresh))
	if tp.Trade.Confirm != 0 {
		t.Error("a nao foi avisado de que b deixou de confirmar")
	}
}

func TestSemEspacoNoDestinoDesfazOPronto(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	for i := 0; i < MovableInventorySlots; i++ {
		b.Inventory.Slots[i] = model.Item{Index: itemFalso}
	}
	abre(t, h, a, ae, b, be)
	oferece(t, h, a, ae, []uint8{0}, 0, true, false)
	if got := mensagemDe(t, be); got != "Libere mais espaco no inventario antes." {
		t.Errorf("b: %q", got)
	}
	if a.Trade.offer.ready {
		t.Error("a continuou pronto sem espaco no destino")
	}
}

// ---- execucao

func TestTrocaConfirmadaPorOsDoisTrocaItensEOuro(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca, Refi: 32}
	b.Inventory.Slots[3] = model.Item{Index: itemPocaoTroca, Refi: 7}
	abre(t, h, a, ae, b, be)

	oferece(t, h, a, ae, []uint8{0}, 200, true, false)
	oferece(t, h, b, be, []uint8{3}, 50, true, false)
	oferece(t, h, a, ae, []uint8{0}, 200, true, true)
	oferece(t, h, b, be, []uint8{3}, 50, true, true)

	if !a.Inventory.Slots[0].IsEmpty() && a.Inventory.Slots[0].Index == itemEspadaTroca {
		t.Error("a ainda tem a espada")
	}
	if contaItem(a, itemPocaoTroca) != 7 || slotsCom(b, itemEspadaTroca) != 1 {
		t.Errorf("a tem %d pocoes, b tem %d espadas", contaItem(a, itemPocaoTroca), slotsCom(b, itemEspadaTroca))
	}
	for _, it := range b.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == itemEspadaTroca && it.Refi != 32 {
			t.Errorf("a espada chegou com refino %d", it.Refi)
		}
	}
	if a.Avatar.Gold != 1000-200+50 || b.Avatar.Gold != 500+200-50 {
		t.Errorf("ouro a=%d b=%d", a.Avatar.Gold, b.Avatar.Gold)
	}
	if a.Trade.with != 0 || b.Trade.with != 0 {
		t.Error("negociacao nao foi encerrada")
	}
	for _, e := range []*espiao{ae, be} {
		proximoDe(t, e, CodeTradeCancel)
	}
	if got := mensagemDe(t, ae); got != "Troca realizada com sucesso." {
		t.Errorf("mensagem = %q", got)
	}
}

// Defeito da origem: o segundo laco de ExecuteTrade testava os itens do
// proprio jogador, entao quem oferecia menos itens recebia menos.
func TestTrocaDeQuantidadesDiferentesMoveTudoDosDoisLados(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	b.Inventory.Slots[0] = model.Item{Index: itemPocaoTroca, Refi: 3}
	b.Inventory.Slots[1] = model.Item{Index: itemEspadaTroca, Refi: 16}
	b.Inventory.Slots[2] = model.Item{Index: itemFalso}
	abre(t, h, a, ae, b, be)

	oferece(t, h, a, ae, []uint8{0}, 0, true, false)
	oferece(t, h, b, be, []uint8{0, 1, 2}, 0, true, false)
	oferece(t, h, a, ae, []uint8{0}, 0, true, true)
	oferece(t, h, b, be, []uint8{0, 1, 2}, 0, true, true)

	if contaItem(a, itemPocaoTroca) != 3 || slotsCom(a, itemFalso) != 1 || slotsCom(a, itemEspadaTroca) != 1 {
		t.Errorf("a recebeu pocoes=%d falso=%d espadas=%d", contaItem(a, itemPocaoTroca), slotsCom(a, itemFalso), slotsCom(a, itemEspadaTroca))
	}
	if slotsCom(b, itemEspadaTroca) != 1 {
		t.Errorf("b recebeu %d espadas", slotsCom(b, itemEspadaTroca))
	}
}

func TestOfertaQueMudouDepoisCancelaSemMoverNada(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	abre(t, h, a, ae, b, be)
	oferece(t, h, a, ae, []uint8{0}, 0, true, true)
	// a tira a espada do slot por outro caminho (outra goroutine na pratica).
	a.Inventory.Slots[0] = model.Item{Index: itemFalso}

	oferece(t, h, b, be, nil, 0, true, true)

	if a.Inventory.Slots[0].Index != itemFalso || slotsCom(b, itemFalso) != 0 || slotsCom(b, itemEspadaTroca) != 0 {
		t.Error("a troca moveu itens apesar de a oferta ter mudado")
	}
	if a.Trade.with != 0 {
		t.Error("a negociacao deveria ter sido cancelada")
	}
}

func TestTrocaSemOuroSuficienteCancelaSemMoverNada(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	abre(t, h, a, ae, b, be)
	oferece(t, h, a, ae, []uint8{0}, 900, true, true)
	a.Avatar.Gold = 100 // gastou o ouro enquanto negociava
	oferece(t, h, b, be, nil, 0, true, true)
	if a.Avatar.Gold != 100 || slotsCom(b, itemEspadaTroca) != 0 || a.Inventory.Slots[0].Index != itemEspadaTroca {
		t.Error("a troca andou sem o ouro")
	}
}

// Atomicidade: se um dos lados nao tem lugar, os dois inventarios voltam ao
// que eram.
func TestTrocaSemLugarNaExecucaoDesfazTudo(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	a.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca}
	a.Inventory.Slots[1] = model.Item{Index: itemFalso}
	for i := 2; i < MovableInventorySlots; i++ {
		a.Inventory.Slots[i] = model.Item{Index: itemFalso}
	}
	for i := 0; i < MovableInventorySlots; i++ {
		b.Inventory.Slots[i] = model.Item{Index: itemFalso}
	}
	b.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca, Refi: 16}
	abre(t, h, a, ae, b, be)
	// Monta as ofertas direto, sem passar pela conferencia de "pronto".
	a.Trade.offer = emptyOffer()
	b.Trade.offer = emptyOffer()
	a.Trade.offer.slots[0], a.Trade.offer.snap[0] = 0, a.Inventory.Slots[0]
	a.Trade.offer.slots[1], a.Trade.offer.snap[1] = 1, a.Inventory.Slots[1]
	b.Trade.offer.slots[0], b.Trade.offer.snap[0] = 0, b.Inventory.Slots[0]
	antesA := append([]model.Item(nil), a.Inventory.Slots...)
	antesB := append([]model.Item(nil), b.Inventory.Slots...)

	tradeMu.Lock()
	h.executeTrade(ae.conn, a, be.conn, b)
	tradeMu.Unlock()

	// a oferece 2 itens e recebe 1: sobra lugar. b oferece 1 e recebe 2: faltaria.
	for i := range antesA {
		if a.Inventory.Slots[i] != antesA[i] || b.Inventory.Slots[i] != antesB[i] {
			t.Fatalf("slot %d mudou apesar da falha", i)
		}
	}
	if got := mensagemDe(t, ae); got != "Inventario cheio. Troca cancelada." {
		t.Errorf("mensagem = %q", got)
	}
	if a.Trade.with != 0 || b.Trade.with != 0 {
		t.Error("negociacao continuou aberta")
	}
}

// ---- cancelar e sair

func TestCancelarFechaAJanelaDoOutroLado(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	abre(t, h, a, ae, b, be)
	if err := h.tradeCancel(context.Background(), ae.conn, a, asPacket(t,
		tradeIn(t, CodeTradeCancel, packets.SignalData{}))); err != nil {
		t.Fatal(err)
	}
	proximoDe(t, be, CodeTradeCancel)
	if a.Trade.with != 0 || b.Trade.with != 0 {
		t.Error("estado sobrou")
	}
	ae.nada(t, 20*time.Millisecond) // quem cancelou ja fechou a propria janela
}

// Defeito da origem: quem saia deixava o parceiro com a janela aberta.
func TestSairDoJogoFechaANegociacaoDoParceiro(t *testing.T) {
	h, a, ae, b, be := prepTroca(t)
	abre(t, h, a, ae, b, be)
	h.OnDisconnect(ae.conn)
	proximoDe(t, be, CodeTradeCancel)
	if b.Trade.with != 0 {
		t.Error("o parceiro ficou preso numa negociacao morta")
	}
}

func TestComercioForaDoJogoETamanhoDosPacotes(t *testing.T) {
	h, a, ae, _, _ := prepTroca(t)
	a.Status = CharList
	if err := h.tradeRequest(context.Background(), ae.conn, a, asPacket(t,
		tradeIn(t, CodeTradeRequest, packets.SignalData{}))); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
	if n := binary.Size(packets.TradePacket{}); n != 236 {
		t.Errorf("TradePacket = %d, o Delphi tem 12 + 224 = 236", n)
	}
	if n := binary.Size(packets.ResponseTradePacket{}); n != 20 {
		t.Errorf("ResponseTradePacket = %d, o Delphi tem 20", n)
	}
	_ = store.MaxFriends
}

// slotsCom conta quantos slots do inventario tem o item (sem somar o Refi).
func slotsCom(s *Session, index uint16) int {
	n := 0
	for _, it := range s.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == index {
			n++
		}
	}
	return n
}
