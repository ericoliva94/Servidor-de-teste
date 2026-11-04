package game

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aika/internal/auction"
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// leilaoFalso guarda as ofertas em memoria.
type leilaoFalso struct {
	mu        sync.Mutex
	ofertas   map[uint64]*auction.Offer
	proximo   uint64
	erroReg   error
	erroClose error
}

func novoLeilao() *leilaoFalso {
	return &leilaoFalso{ofertas: map[uint64]*auction.Offer{}}
}

func (l *leilaoFalso) NextIndex(context.Context) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.proximo++
	return l.proximo, nil
}

func (l *leilaoFalso) Register(_ context.Context, o auction.Offer) error {
	if l.erroReg != nil {
		return l.erroReg
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	copia := o
	l.ofertas[o.Index] = &copia
	return nil
}

func (l *leilaoFalso) Get(_ context.Context, index uint64) (*auction.Offer, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	o, ok := l.ofertas[index]
	if !ok {
		return nil, auction.ErrOfferNotFound
	}
	return o, nil
}

func (l *leilaoFalso) Active(_ context.Context, limite int) ([]auction.Offer, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []auction.Offer
	for _, o := range l.ofertas {
		if o.Active && len(out) < limite {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (l *leilaoFalso) ByOwner(_ context.Context, char uint32, limite int) ([]auction.Offer, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []auction.Offer
	for _, o := range l.ofertas {
		if o.Active && o.SellerCharIndex == char && len(out) < limite {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (l *leilaoFalso) Close(_ context.Context, index uint64, comprador uint32) (bool, error) {
	if l.erroClose != nil {
		return false, l.erroClose
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	o, ok := l.ofertas[index]
	if !ok || !o.Active {
		return false, nil
	}
	o.Active = false
	o.SoldTo = comprador
	return true, nil
}

func (l *leilaoFalso) Expired(_ context.Context, agora time.Time) ([]auction.Offer, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []auction.Offer
	for _, o := range l.ofertas {
		if o.Active && o.Expired(agora) {
			out = append(out, *o)
		}
	}
	return out, nil
}

// handlerComLeilao devolve um handler com leilao e correio em memoria.
func handlerComLeilao(t *testing.T) (*Handler, *leilaoFalso, *correioFalso, *time.Time) {
	h, correio := handlerComCorreio(t)
	l := novoLeilao()
	h.AuctionDB = l
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	itens := novaTabelaDeItens()
	itens.poeItem(100, gamedata.ItemFromList{})
	itens.poeItem(200, gamedata.ItemFromList{TypeTrade: 1}) // nao comercializavel
	itens.poeItem(9000, gamedata.ItemFromList{})
	h.Data = itens
	return h, l, correio, &agora
}

func registerAuctionPacket(preco uint32, slot, horas uint16) []byte {
	var p packets.AuctionRegisterItemPacket
	p.Price = preco
	p.Slot = slot
	p.Time = horas
	buf, err := netsrv.Marshal(CodeAuctionRegister, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func auctionOfferPacket(code uint16, oferta uint64) []byte {
	var p packets.AuctionCancelOfferPacket
	p.AuctionOfferId = oferta
	buf, err := netsrv.Marshal(code, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- registrar

func TestRegistrarItemNoLeilao(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 100, Refi: 3}
	ouroAntes := a.Avatar.Gold
	ea.drena(100 * time.Millisecond)

	if err := h.registerAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, registerAuctionPacket(10000, 0, 6))); err != nil {
		t.Fatal(err)
	}
	o, err := l.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("a oferta deveria existir: %v", err)
	}
	if o.Item.Index != 100 || o.Price != 10000 || o.Hours != 6 {
		t.Errorf("= %+v", o)
	}
	if o.SellerCharIndex != 100 || o.SellerName != "Alfa" {
		t.Errorf("vendedor = %d/%q", o.SellerCharIndex, o.SellerName)
	}
	// O item saiu do inventario.
	if !a.Inventory.Get(0).IsEmpty() {
		t.Error("o item deveria ter saido do inventario")
	}
	// A taxa foi cobrada: 10000/1000 * (6/3) = 20.
	if a.Avatar.Gold != ouroAntes-20 {
		t.Errorf("ouro = %d, queria %d", a.Avatar.Gold, ouroAntes-20)
	}
}

// O bug grave: "Inventory[Slot]" com Slot vindo do cliente como WORD e o
// vetor ARRAY [0..63], sem checagem. O ZeroMemory no fim ESCREVIA 20 bytes
// fora do vetor.
func TestRegistrarComSlotForaDaFaixa(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	ea.drena(100 * time.Millisecond)

	for _, slot := range []uint16{64, 100, 1000, 65535} {
		if err := h.registerAuctionItem(context.Background(), ea.conn, a,
			asPacket(t, registerAuctionPacket(10000, slot, 6))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
	if len(l.ofertas) != 0 {
		t.Errorf("%d ofertas, nao deveria ter registrado nada", len(l.ofertas))
	}
}

func TestRegistrarSlotVazio(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	ea.drena(100 * time.Millisecond)

	if err := h.registerAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, registerAuctionPacket(10000, 0, 6))); err != nil {
		t.Fatal(err)
	}
	if len(l.ofertas) != 0 {
		t.Error("nao deveria registrar slot vazio")
	}
}

// Item marcado como nao comercializavel nao entra.
func TestRegistrarItemNaoComercializavel(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 200}
	ea.drena(100 * time.Millisecond)

	if err := h.registerAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, registerAuctionPacket(10000, 0, 6))); err != nil {
		t.Fatal(err)
	}
	if len(l.ofertas) != 0 {
		t.Error("nao deveria registrar item nao comercializavel")
	}
	if a.Inventory.Get(0).Index != 200 {
		t.Error("o item deveria ter ficado no inventario")
	}
}

// Falha ao gravar a oferta nao pode tirar o item do jogador.
func TestFalhaAoRegistrarNaoPerdeOItem(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	l.erroReg = errors.New("falha de teste")
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 100}
	ouroAntes := a.Avatar.Gold
	ea.drena(100 * time.Millisecond)

	if err := h.registerAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, registerAuctionPacket(10000, 0, 6))); err != nil {
		t.Fatal(err)
	}
	if a.Inventory.Get(0).Index != 100 {
		t.Error("o item deveria ter ficado no inventario")
	}
	if a.Avatar.Gold != ouroAntes {
		t.Errorf("ouro = %d, a taxa nao deveria ter sido cobrada", a.Avatar.Gold)
	}
}

func TestRegistrarSemOuroParaTaxa(t *testing.T) {
	h, l, _, _ := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 100}
	a.Avatar.Gold = 5 // taxa de 1000000/1000*(12/3) = 4000
	ea.drena(100 * time.Millisecond)

	if err := h.registerAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, registerAuctionPacket(1000000, 0, 12))); err != nil {
		t.Fatal(err)
	}
	if len(l.ofertas) != 0 {
		t.Error("nao deveria registrar sem ouro para a taxa")
	}
}

// ---- comprar

// A correcao central: na origem o ouro do comprador so era conferido DEPOIS
// de as cartas terem sido criadas e o item ja estar no anexo do comprador.
func TestComprarSemOuroNaoEntregaOItem(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	b.Avatar.Gold = 100
	correio.poeNome("Beta", 200)
	eb.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100}, 50000, 6, *agora)
	l.Register(context.Background(), o)

	if err := h.buyAuctionItem(context.Background(), eb.conn, b,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1))); err != nil {
		t.Fatal(err)
	}
	// Nenhuma carta foi criada.
	caixa, _ := correio.Box(context.Background(), 200)
	if caixa.Count() != 0 {
		t.Errorf("%d cartas: o item nao deveria ter sido entregue", caixa.Count())
	}
	// A oferta continua aberta.
	guardada, _ := l.Get(context.Background(), 1)
	if !guardada.Active {
		t.Error("a oferta deveria ter ficado aberta")
	}
	if b.Avatar.Gold != 100 {
		t.Errorf("ouro = %d", b.Avatar.Gold)
	}
}

func TestComprarItem(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	correio.poeNome("Beta", 200)
	ouroAntes := b.Avatar.Gold
	eb.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100, Refi: 5}, 50000, 6, *agora)
	l.Register(context.Background(), o)

	if err := h.buyAuctionItem(context.Background(), eb.conn, b,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1))); err != nil {
		t.Fatal(err)
	}
	// O comprador recebeu o item por carta.
	caixaComprador, _ := correio.Box(context.Background(), 200)
	if caixaComprador.Count() != 1 {
		t.Fatalf("%d cartas na caixa do comprador", caixaComprador.Count())
	}
	cartaItem := caixaComprador.Mails[0]
	if cartaItem.Items[0].Index != 100 || cartaItem.Items[0].Refi != 5 {
		t.Errorf("anexo = %+v", cartaItem.Items[0])
	}
	if !cartaItem.Auction {
		t.Error("a carta deveria estar marcada como de leilao")
	}
	// O vendedor recebeu o ouro por carta.
	caixaVendedor, _ := correio.Box(context.Background(), 100)
	if caixaVendedor.Count() != 1 {
		t.Fatalf("%d cartas na caixa do vendedor", caixaVendedor.Count())
	}
	if caixaVendedor.Mails[0].Gold != 50000 {
		t.Errorf("ouro na carta = %d", caixaVendedor.Mails[0].Gold)
	}
	// O comprador pagou.
	if b.Avatar.Gold != ouroAntes-50000 {
		t.Errorf("ouro = %d, queria %d", b.Avatar.Gold, ouroAntes-50000)
	}
	// A oferta fechou.
	guardada, _ := l.Get(context.Background(), 1)
	if guardada.Active || guardada.SoldTo != 200 {
		t.Errorf("oferta = %+v", guardada)
	}
}

// Dois compradores ao mesmo tempo: so um leva.
func TestDoisCompradoresSoUmLeva(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	c, ec := entraComPersonagem(t, h, "Gama", 300)
	correio.poeNome("Beta", 200)
	correio.poeNome("Gama", 300)
	eb.drena(100 * time.Millisecond)
	ec.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100}, 1000, 6, *agora)
	l.Register(context.Background(), o)

	h.buyAuctionItem(context.Background(), eb.conn, b,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1)))
	h.buyAuctionItem(context.Background(), ec.conn, c,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1)))

	cB, _ := correio.Box(context.Background(), 200)
	cC, _ := correio.Box(context.Background(), 300)
	total := cB.Count() + cC.Count()
	if total != 1 {
		t.Errorf("%d cartas entregues no total, queria 1", total)
	}
}

func TestNaoCompraAPropriaOferta(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	correio.poeNome("Alfa", 100)
	ea.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100}, 1000, 6, *agora)
	l.Register(context.Background(), o)

	if err := h.buyAuctionItem(context.Background(), ea.conn, a,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1))); err != nil {
		t.Fatal(err)
	}
	guardada, _ := l.Get(context.Background(), 1)
	if !guardada.Active {
		t.Error("a oferta deveria ter ficado aberta")
	}
}

func TestComprarOfertaInexistente(t *testing.T) {
	h, _, correio, _ := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	correio.poeNome("Beta", 200)
	eb.drena(100 * time.Millisecond)

	for _, idx := range []uint64{0, 1, 999, 1 << 40} {
		if err := h.buyAuctionItem(context.Background(), eb.conn, b,
			asPacket(t, auctionOfferPacket(CodeAuctionBuy, idx))); err != nil {
			t.Fatalf("oferta %d: %v", idx, err)
		}
	}
}

// Oferta vencida nao e comprada.
func TestComprarOfertaVencida(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	correio.poeNome("Beta", 200)
	eb.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100}, 1000, 6,
		agora.Add(-10*time.Hour))
	l.Register(context.Background(), o)

	if err := h.buyAuctionItem(context.Background(), eb.conn, b,
		asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := correio.Box(context.Background(), 200)
	if caixa.Count() != 0 {
		t.Error("oferta vencida nao deveria ser comprada")
	}
}

// ---- cancelar

// A origem nao conferia o dono: qualquer jogador cancelava a oferta de
// qualquer outro mandando o id.
func TestCancelarOfertaDeOutroERecusado(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	correio.poeNome("Beta", 200)
	eb.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100}, 1000, 6, *agora)
	l.Register(context.Background(), o)

	if err := h.cancelAuctionOffer(context.Background(), eb.conn, b,
		asPacket(t, auctionOfferPacket(CodeAuctionCancel, 1))); err != nil {
		t.Fatal(err)
	}
	guardada, _ := l.Get(context.Background(), 1)
	if !guardada.Active {
		t.Error("a oferta de outro nao deveria ter sido cancelada")
	}
	caixa, _ := correio.Box(context.Background(), 200)
	if caixa.Count() != 0 {
		t.Error("nao deveria ter recebido o item de outro")
	}
}

// O item volta por carta, nao direto ao inventario: a origem perdia o item
// com o inventario cheio.
func TestCancelarDevolvePorCarta(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	// Inventario cheio: a devolucao por carta nao se importa.
	for i := 0; i < MovableInventorySlots; i++ {
		a.Inventory.Slots[i] = model.Item{Index: 9000}
	}
	correio.poeNome("Alfa", 100)
	ea.drena(100 * time.Millisecond)

	o := auction.New(1, 100, "Alfa", model.Item{Index: 100, Refi: 7}, 1000, 6, *agora)
	l.Register(context.Background(), o)

	if err := h.cancelAuctionOffer(context.Background(), ea.conn, a,
		asPacket(t, auctionOfferPacket(CodeAuctionCancel, 1))); err != nil {
		t.Fatal(err)
	}
	guardada, _ := l.Get(context.Background(), 1)
	if guardada.Active {
		t.Error("a oferta deveria ter fechado")
	}
	caixa, _ := correio.Box(context.Background(), 100)
	if caixa.Count() != 1 {
		t.Fatalf("%d cartas", caixa.Count())
	}
	if caixa.Mails[0].Items[0].Index != 100 || caixa.Mails[0].Items[0].Refi != 7 {
		t.Errorf("anexo = %+v", caixa.Mails[0].Items[0])
	}
}

// ---- listas

func TestListarLeilao(t *testing.T) {
	h, l, _, agora := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	ea.drena(100 * time.Millisecond)

	for i := uint64(1); i <= 3; i++ {
		o := auction.New(i, 200, "Beta", model.Item{Index: uint16(100 + i)},
			uint32(1000*i), 6, *agora)
		l.Register(context.Background(), o)
	}

	if err := h.listAuctionItems(context.Background(), ea.conn, a,
		asPacket(t, auctionOfferPacket(CodeAuctionList, 0))); err != nil {
		t.Fatal(err)
	}
	vis := ea.proximo(t)
	if vis.code != CodeAuctionList {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.SendAuctionItemsPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.ItemCount != 3 {
		t.Errorf("total = %d, queria 3", pkt.ItemCount)
	}
}

func TestListarMinhasOfertas(t *testing.T) {
	h, l, _, agora := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	ea.drena(100 * time.Millisecond)

	// Duas minhas e uma de outro.
	l.Register(context.Background(), auction.New(1, 100, "Alfa",
		model.Item{Index: 100}, 1000, 6, *agora))
	l.Register(context.Background(), auction.New(2, 100, "Alfa",
		model.Item{Index: 101}, 2000, 6, *agora))
	l.Register(context.Background(), auction.New(3, 200, "Beta",
		model.Item{Index: 102}, 3000, 6, *agora))

	if err := h.ownAuctionItems(context.Background(), ea.conn, a,
		asPacket(t, auctionOfferPacket(CodeAuctionOwn, 0))); err != nil {
		t.Fatal(err)
	}
	vis := ea.proximo(t)
	if vis.code != CodeAuctionOwn {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.CadastredItemsPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.ItemCount != 2 {
		t.Errorf("total = %d, queria 2", pkt.ItemCount)
	}
}

// Mais de doze ofertas: a origem recusava a lista inteira e o jogador nao
// via nenhuma.
func TestMaisDeDozeOfertasMostraOTeto(t *testing.T) {
	h, l, _, agora := handlerComLeilao(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	ea.drena(100 * time.Millisecond)

	for i := uint64(1); i <= 20; i++ {
		l.Register(context.Background(), auction.New(i, 100, "Alfa",
			model.Item{Index: 100}, uint32(i*100), 6, *agora))
	}
	if err := h.ownAuctionItems(context.Background(), ea.conn, a,
		asPacket(t, auctionOfferPacket(CodeAuctionOwn, 0))); err != nil {
		t.Fatal(err)
	}
	var pkt packets.CadastredItemsPacket
	netsrv.Unmarshal(asPacote(t, ea.proximo(t).corpo), &pkt)
	if pkt.ItemCount != auction.MaxOwnItems {
		t.Errorf("total = %d, queria o teto de %d", pkt.ItemCount, auction.MaxOwnItems)
	}
}

// ---- prazo

// A origem tinha CheckAuctionItems com o corpo esvaziado ("Result := True"):
// a oferta vencida ficava no banco para sempre e o item nunca voltava.
func TestOfertaVencidaVoltaAoVendedor(t *testing.T) {
	h, l, correio, agora := handlerComLeilao(t)
	correio.poeNome("Alfa", 100)

	l.Register(context.Background(), auction.New(1, 100, "Alfa",
		model.Item{Index: 100}, 1000, 6, agora.Add(-10*time.Hour)))
	l.Register(context.Background(), auction.New(2, 100, "Alfa",
		model.Item{Index: 101}, 1000, 6, *agora))

	if n := h.AuctionExpireTick(context.Background()); n != 1 {
		t.Fatalf("%d devolvidas, queria 1", n)
	}
	caixa, _ := correio.Box(context.Background(), 100)
	if caixa.Count() != 1 {
		t.Fatalf("%d cartas", caixa.Count())
	}
	if caixa.Mails[0].Items[0].Index != 100 {
		t.Errorf("anexo = %d", caixa.Mails[0].Items[0].Index)
	}
	// A que nao venceu continua aberta.
	o2, _ := l.Get(context.Background(), 2)
	if !o2.Active {
		t.Error("a oferta dentro do prazo deveria ter ficado aberta")
	}
}

func TestAuctionExpireTickSemLeilao(t *testing.T) {
	h := testHandler()
	if n := h.AuctionExpireTick(context.Background()); n != 0 {
		t.Errorf("= %d", n)
	}
}

// Fora de jogo os handlers recusam.
func TestLeilaoExigeEstarEmJogo(t *testing.T) {
	h, _, _, _ := handlerComLeilao(t)
	s := sessaoAutenticada()
	c := connDescartavel(t)
	ctx := context.Background()
	casos := map[string]func() error{
		"registrar": func() error {
			return h.registerAuctionItem(ctx, c, s, asPacket(t, registerAuctionPacket(1, 0, 6)))
		},
		"listar": func() error {
			return h.listAuctionItems(ctx, c, s, asPacket(t, auctionOfferPacket(CodeAuctionList, 0)))
		},
		"minhas": func() error {
			return h.ownAuctionItems(ctx, c, s, asPacket(t, auctionOfferPacket(CodeAuctionOwn, 0)))
		},
		"comprar": func() error {
			return h.buyAuctionItem(ctx, c, s, asPacket(t, auctionOfferPacket(CodeAuctionBuy, 1)))
		},
		"cancelar": func() error {
			return h.cancelAuctionOffer(ctx, c, s, asPacket(t, auctionOfferPacket(CodeAuctionCancel, 1)))
		},
	}
	for nome, f := range casos {
		if err := f(); err != ErrNotPlaying {
			t.Errorf("%s: erro = %v, queria ErrNotPlaying", nome, err)
		}
	}
}
