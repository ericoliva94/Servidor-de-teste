package game

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// prepLoja poe vendedor e comprador em jogo, vendo um ao outro. O vendedor tem
// uma espada no slot 0, uma pocao (pilha de 5) no slot 1 e um item preso no 2.
func prepLoja(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	h, v, ve, b, be := prepTroca(t)
	v.Base.VisiblePlayers.Add(b.ClientID)
	b.Base.VisiblePlayers.Add(v.ClientID)
	v.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca, Min: 10, Max: 10}
	v.Inventory.Slots[1] = model.Item{Index: itemPocaoTroca, Refi: 5}
	v.Inventory.Slots[2] = model.Item{Index: itemPreso}
	ve.drena(10 * time.Millisecond)
	be.drena(10 * time.Millisecond)
	return h, v, ve, b, be
}

func vitrine(s *Session, nome string, prods ...model.PersonalShopItem) packets.PersonalShopPacket {
	var p packets.PersonalShopPacket
	p.Shop.Index = uint32(s.ClientID)
	netsrv.PutCString(p.Shop.Name[:], nome)
	for i := range p.Shop.Products {
		p.Shop.Products[i] = emptyProduct()
	}
	copy(p.Shop.Products[:], prods)
	return p
}

func prod(slot uint32, preco uint64) model.PersonalShopItem {
	// O Item que o cliente manda e propositalmente falso.
	return model.PersonalShopItem{Price: preco, Slot: slot, Item: model.Item{Index: itemFalso}}
}

func abreLoja(t *testing.T, h *Handler, s *Session, e *espiao, p packets.PersonalShopPacket) error {
	t.Helper()
	return h.createShop(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeShopCreate, p)))
}

func olhaLoja(t *testing.T, h *Handler, s *Session, e *espiao, dono uint32) {
	t.Helper()
	if err := h.openShop(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeShopOpen, packets.SignalData{Data: dono}))); err != nil {
		t.Fatal(err)
	}
}

func compra(t *testing.T, h *Handler, s *Session, e *espiao, dono, slot uint32, p model.PersonalShopItem) {
	t.Helper()
	if err := h.buyShopItem(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeShopBuy, packets.BuyPersonalShopItemPacket{Index: dono, Slot: slot, Product: p}))); err != nil {
		t.Fatal(err)
	}
}

// lojaAberta abre uma loja valida (espada a 300, pocao a 50) e a deixa visitada.
func lojaAberta(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	h, v, ve, b, be := prepLoja(t)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 300), prod(1, 50))); err != nil {
		t.Fatal(err)
	}
	olhaLoja(t, h, b, be, uint32(v.ClientID))
	ve.drena(10 * time.Millisecond)
	be.drena(10 * time.Millisecond)
	return h, v, ve, b, be
}

// ---- tamanhos

func TestLojaTamanhosDosPacotes(t *testing.T) {
	if n := len(miscPacket(CodeShopCreate, packets.PersonalShopPacket{})); n != 0x170 {
		t.Errorf("$319 = %d bytes, Delphi: $170", n)
	}
	if n := len(miscPacket(CodeShopBuy, packets.BuyPersonalShopItemPacket{})); n != 0x38 {
		t.Errorf("$31B = %d bytes, Delphi: $38", n)
	}
}

// ---- abrir a loja

func TestAbrirLojaUsaOItemDoServidorEMostraONome(t *testing.T) {
	h, v, ve, _, be := prepLoja(t)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 300), prod(1, 50))); err != nil {
		t.Fatal(err)
	}
	if !v.Shop.isOpen() || v.Avatar.ShopName != "Armas" {
		t.Fatalf("loja = %+v, nome = %q", v.Shop, v.Avatar.ShopName)
	}
	got := decodifica[packets.PersonalShopPacket](t, proximoDe(t, ve, CodeShopCreate))
	if got.Header.Index != shopHeaderIndex {
		t.Errorf("Header.Index = %#x, queria %#x", got.Header.Index, shopHeaderIndex)
	}
	// O item falso do cliente foi ignorado: vale o que esta no inventario.
	if got.Shop.Products[0].Item.Index != itemEspadaTroca || got.Shop.Products[0].Price != 300 {
		t.Errorf("produto 0 = %+v", got.Shop.Products[0])
	}
	if got.Shop.Products[1].Item.Index != itemPocaoTroca || got.Shop.Products[1].Item.Refi != 5 {
		t.Errorf("produto 1 = %+v", got.Shop.Products[1])
	}
	for i := 2; i < ShopProducts; i++ {
		if got.Shop.Products[i] != emptyProduct() {
			t.Errorf("vitrine %d deveria estar vazia: %+v", i, got.Shop.Products[i])
		}
	}
	// Quem ve o vendedor recebe o boneco de novo, agora com o nome da loja.
	cm := decodifica[packets.SendCreateMobPacket](t, proximoDe(t, be, CodeCreateMob))
	if netsrv.CString(cm.Title[:]) != "Armas" || cm.EffectType != EffectTypeShop {
		t.Errorf("spawn visto pelo comprador: titulo=%q efeito=%d", netsrv.CString(cm.Title[:]), cm.EffectType)
	}
}

func TestAbrirLojaRecusas(t *testing.T) {
	h, v, ve, _, _ := prepLoja(t)
	semNome := vitrine(v, "", prod(0, 1))
	outroIndice := vitrine(v, "X", prod(0, 1))
	outroIndice.Shop.Index = uint32(v.ClientID) + 1
	casos := map[string]packets.PersonalShopPacket{
		"sem nome":        semNome,
		"indice alheio":   outroIndice,
		"sem produto":     vitrine(v, "X"),
		"slot de bolsa":   vitrine(v, "X", prod(60, 1)),
		"slot 63":         vitrine(v, "X", prod(63, 1)),
		"slot gigante":    vitrine(v, "X", prod(0xFFFFFFF0, 1)),
		"slot vazio":      vitrine(v, "X", prod(10, 1)),
		"item preso":      vitrine(v, "X", prod(2, 1)),
		"preco absurdo":   vitrine(v, "X", prod(0, uint64(MaxGold)+1)),
		"preco uint64max": vitrine(v, "X", prod(0, ^uint64(0))),
	}
	for nome, p := range casos {
		if err := abreLoja(t, h, v, ve, p); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if v.Shop.isOpen() || v.Avatar.ShopName != "" {
			t.Errorf("%s: a loja abriu", nome)
		}
		ve.drena(5 * time.Millisecond)
	}
}

func TestAbrirLojaItemRepetidoFechaEAvisa(t *testing.T) {
	h, v, ve, _, _ := prepLoja(t)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 10))); err != nil {
		t.Fatal(err)
	}
	ve.drena(10 * time.Millisecond)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 10), prod(0, 20))); err != nil {
		t.Fatal(err)
	}
	if v.Shop.isOpen() {
		t.Error("a loja anterior deveria ter fechado")
	}
	if got := mensagemDe(t, ve); got != "Voce nao pode vender o mesmo item varias vezes." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestAbrirLojaDuranteTroca(t *testing.T) {
	h, v, ve, b, be := prepLoja(t)
	abre(t, h, v, ve, b, be)
	if err := abreLoja(t, h, v, ve, vitrine(v, "X", prod(0, 1))); err != nil {
		t.Fatal(err)
	}
	if v.Shop.isOpen() {
		t.Error("abriu loja no meio de uma troca")
	}
	if got := mensagemDe(t, ve); got != "Termine a troca antes de abrir a loja." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestTrocaRecusadaComLojaAberta(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	convida(t, h, b, be, v)
	if v.Trade.pending != 0 {
		t.Error("o convite chegou a quem tem loja aberta")
	}
	if got := mensagemDe(t, be); got != "Voce nao pode negociar com esse jogador." {
		t.Errorf("mensagem = %q", got)
	}
	_ = ve
}

// ---- ver a loja

func TestVerLojaMostraAVitrineEGuardaAVisita(t *testing.T) {
	h, v, ve, b, be := prepLoja(t)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 300))); err != nil {
		t.Fatal(err)
	}
	be.drena(10 * time.Millisecond)
	olhaLoja(t, h, b, be, uint32(v.ClientID))
	got := decodifica[packets.PersonalShopPacket](t, proximoDe(t, be, CodeShopCreate))
	if got.Shop.Index != uint32(v.ClientID) || got.Shop.Products[0].Price != 300 {
		t.Errorf("vitrine vista = %+v", got.Shop)
	}
	if b.Shop.visiting != v.ClientID {
		t.Errorf("visitando = %d", b.Shop.visiting)
	}
}

func TestVerLojaRecusas(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	outro, outroE := entraJogadorSimples(t, h)
	_ = outroE
	b.Shop.visiting = 0 // lojaAberta deixa o comprador visitando
	// alvo sem loja
	olhaLoja(t, h, b, be, uint32(outro.ClientID))
	// o proprio, zero, fora da faixa, inexistente
	for _, alvo := range []uint32{uint32(b.ClientID), 0, 0x10000, 0xFFFFFFFF, 999} {
		olhaLoja(t, h, b, be, alvo)
	}
	if b.Shop.visiting != 0 {
		t.Errorf("visitando = %d, queria 0", b.Shop.visiting)
	}
	// loja de quem nao esta na minha tela
	b.Base.VisiblePlayers.Remove(v.ClientID)
	olhaLoja(t, h, b, be, uint32(v.ClientID))
	if b.Shop.visiting != 0 {
		t.Error("abriu loja de quem esta fora da tela")
	}
	_ = ve
}

func entraJogadorSimples(t *testing.T, h *Handler) (*Session, *espiao) {
	t.Helper()
	e := novoEspiao(t)
	s := sessaoComInventario()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return s, e
}

// ---- comprar

func TestCompraTransfereItemEOuro(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	loja := v.Shop.own
	compra(t, h, b, be, uint32(v.ClientID), 0, loja.Products[0])

	if b.Avatar.Gold != 500-300 || v.Avatar.Gold != 1000+300 {
		t.Errorf("ouro comprador=%d vendedor=%d", b.Avatar.Gold, v.Avatar.Gold)
	}
	if !v.Inventory.Slots[0].IsEmpty() {
		t.Error("o item continua com o vendedor")
	}
	achou := false
	for _, it := range b.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == itemEspadaTroca && it.Min == 10 {
			achou = true
		}
	}
	if !achou {
		t.Error("o comprador nao recebeu a espada")
	}
	if shopActive(v.Shop.own.Products[0]) {
		t.Error("o produto continua na vitrine")
	}
	if !shopActive(v.Shop.own.Products[1]) {
		t.Error("o outro produto sumiu")
	}
	if got := mensagemDe(t, be); got != "Item comprado com sucesso." {
		t.Errorf("mensagem = %q", got)
	}
	// O vendedor ve o slot esvaziado, o ouro novo e a vitrine atualizada.
	proximoDe(t, ve, CodeRefreshItem)
	proximoDe(t, ve, CodeShopCreate)
}

func TestCompraEmPilhaEntregaAPilhaInteira(t *testing.T) {
	h, v, _, b, be := lojaAberta(t)
	compra(t, h, b, be, uint32(v.ClientID), 1, v.Shop.own.Products[1])
	if b.Avatar.Gold != 450 {
		t.Errorf("ouro = %d", b.Avatar.Gold)
	}
	var total uint16
	for _, it := range b.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == itemPocaoTroca {
			total += it.Refi
		}
	}
	if total != 5 {
		t.Errorf("pocoes recebidas = %d, queria 5", total)
	}
}

func TestCompraRecusas(t *testing.T) {
	type caso struct {
		nome string
		pre  func(h *Handler, v, b *Session)
		pega func(v *Session) (uint32, model.PersonalShopItem)
		msg  string // "" quando a recusa e silenciosa
	}
	casos := []caso{
		{"ouro insuficiente", func(h *Handler, v, b *Session) { b.Avatar.Gold = 299 },
			func(v *Session) (uint32, model.PersonalShopItem) { return 0, v.Shop.own.Products[0] }, "Gold insuficiente."},
		{"produto forjado (preco menor)", nil,
			func(v *Session) (uint32, model.PersonalShopItem) {
				p := v.Shop.own.Products[0]
				p.Price = 1
				return 0, p
			}, "Item invalido."},
		{"produto forjado (outro item)", nil,
			func(v *Session) (uint32, model.PersonalShopItem) {
				p := v.Shop.own.Products[0]
				p.Item.Index = itemFalso
				return 0, p
			}, "Item invalido."},
		{"vitrine vazia", nil,
			func(v *Session) (uint32, model.PersonalShopItem) { return 5, v.Shop.own.Products[5] }, "Item invalido."},
		{"slot da vitrine fora da faixa (10)", nil,
			func(v *Session) (uint32, model.PersonalShopItem) { return 10, v.Shop.own.Products[0] }, "Item invalido."},
		{"slot da vitrine gigante", nil,
			func(v *Session) (uint32, model.PersonalShopItem) { return 0xFFFFFFFF, v.Shop.own.Products[0] }, "Item invalido."},
		{"inventario cheio", func(h *Handler, v, b *Session) {
			for i := 0; i < MovableInventorySlots; i++ {
				b.Inventory.Slots[i] = model.Item{Index: itemEspadaTroca}
			}
		}, func(v *Session) (uint32, model.PersonalShopItem) { return 0, v.Shop.own.Products[0] }, "Inventario cheio."},
		{"vendedor no teto de ouro", func(h *Handler, v, b *Session) { v.Avatar.Gold = MaxGold - 100 },
			func(v *Session) (uint32, model.PersonalShopItem) { return 0, v.Shop.own.Products[0] },
			"O vendedor nao pode receber esse valor."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, v, _, b, be := lojaAberta(t)
			if c.pre != nil {
				c.pre(h, v, b)
			}
			ouroB, ouroV := b.Avatar.Gold, v.Avatar.Gold
			slot, p := c.pega(v)
			compra(t, h, b, be, uint32(v.ClientID), slot, p)
			if b.Avatar.Gold != ouroB || v.Avatar.Gold != ouroV {
				t.Errorf("o ouro mudou: comprador %d->%d, vendedor %d->%d", ouroB, b.Avatar.Gold, ouroV, v.Avatar.Gold)
			}
			if v.Inventory.Slots[0].IsEmpty() {
				t.Error("o vendedor perdeu o item sem receber")
			}
			if c.msg != "" {
				if got := mensagemDe(t, be); got != c.msg {
					t.Errorf("mensagem = %q, queria %q", got, c.msg)
				}
			}
		})
	}
}

func TestCompraSemTerAbertoALoja(t *testing.T) {
	h, v, _, b, be := lojaAberta(t)
	b.Shop.visiting = 0
	compra(t, h, b, be, uint32(v.ClientID), 0, v.Shop.own.Products[0])
	if b.Avatar.Gold != 500 || v.Inventory.Slots[0].IsEmpty() {
		t.Error("comprou sem estar vendo a loja")
	}
	// Comprar da loja de outro: so vale o id que eu abri.
	b.Shop.visiting = v.ClientID
	compra(t, h, b, be, uint32(b.ClientID), 0, v.Shop.own.Products[0])
	compra(t, h, b, be, 0x10000, 0, v.Shop.own.Products[0])
	if b.Avatar.Gold != 500 {
		t.Error("comprou de id que nao abriu")
	}
}

// O dono mexe no inventario depois de abrir a loja: a origem entregava o que
// houvesse no slot (nada ou outro item) pelo preco combinado.
func TestCompraItemQueOVendedorMoveuOuVendeu(t *testing.T) {
	for nome, mexe := range map[string]func(v *Session){
		"apagou":  func(v *Session) { v.Inventory.Slots[0] = model.Item{} },
		"trocou":  func(v *Session) { v.Inventory.Slots[0] = model.Item{Index: itemFalso} },
		"gastou":  func(v *Session) { v.Inventory.Slots[0].Min = 1 },
		"pilha-1": func(v *Session) { v.Inventory.Slots[0] = model.Item{Index: itemEspadaTroca, Min: 9, Max: 10} },
	} {
		t.Run(nome, func(t *testing.T) {
			h, v, ve, b, be := lojaAberta(t)
			mexe(v)
			antes := v.Inventory.Slots[0]
			compra(t, h, b, be, uint32(v.ClientID), 0, v.Shop.own.Products[0])
			if b.Avatar.Gold != 500 || v.Avatar.Gold != 1000 {
				t.Errorf("ouro mudou: %d / %d", b.Avatar.Gold, v.Avatar.Gold)
			}
			if v.Inventory.Slots[0] != antes {
				t.Error("o inventario do vendedor mudou")
			}
			if shopActive(v.Shop.own.Products[0]) {
				t.Error("o produto velho continuou na vitrine")
			}
			if got := mensagemDe(t, be); got != "O item nao esta mais disponivel." {
				t.Errorf("mensagem = %q", got)
			}
			_ = ve
		})
	}
}

// Quatro compradores no mesmo produto: so um leva.
func TestCompraConcorrenteSoUmLeva(t *testing.T) {
	h, v, ve, _, _ := prepLoja(t)
	if err := abreLoja(t, h, v, ve, vitrine(v, "Armas", prod(0, 100))); err != nil {
		t.Fatal(err)
	}
	prod0 := v.Shop.own.Products[0]
	const n = 4
	var compradores [n]*Session
	var espioes [n]*espiao
	for i := range compradores {
		s, e := entraJogadorSimples(t, h)
		s.Avatar.Gold = 1000
		v.Base.VisiblePlayers.Add(s.ClientID)
		s.Base.VisiblePlayers.Add(v.ClientID)
		s.Shop.visiting = v.ClientID
		compradores[i], espioes[i] = s, e
		go func() { // drena para o pipe nunca travar quem escreve
			for range e.pacotes {
			}
		}()
	}
	var wg sync.WaitGroup
	for i := range compradores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.buyShopItem(context.Background(), espioes[i].conn, compradores[i], asPacket(t,
				miscPacket(CodeShopBuy, packets.BuyPersonalShopItemPacket{
					Index: uint32(v.ClientID), Slot: 0, Product: prod0})))
		}()
	}
	wg.Wait()
	levaram := 0
	for _, s := range compradores {
		for _, it := range s.Inventory.Slots[:MovableInventorySlots] {
			if it.Index == itemEspadaTroca {
				levaram++
			}
		}
		if s.Avatar.Gold != 1000 && s.Avatar.Gold != 900 {
			t.Errorf("ouro de comprador = %d", s.Avatar.Gold)
		}
	}
	if levaram != 1 {
		t.Errorf("%d compradores levaram a espada, queria 1", levaram)
	}
	if v.Avatar.Gold != 1100 {
		t.Errorf("vendedor = %d, queria 1100", v.Avatar.Gold)
	}
}

// ---- fechar

func TestFecharLojaTiraONomeEFechaAJanelaDosVisitantes(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	if err := h.tradeCancel(context.Background(), ve.conn, v, asPacket(t,
		miscPacket(CodeTradeCancel, packets.SignalData{}))); err != nil {
		t.Fatal(err)
	}
	if v.Shop.isOpen() || v.Avatar.ShopName != "" {
		t.Error("a loja continua aberta")
	}
	proximoDe(t, ve, CodeTradeCancel) // eco ao dono
	// proximoDe descarta o que pula, entao a ordem de chegada importa: primeiro o
	// boneco sem nome de loja, depois a janela fechada.
	cm := decodifica[packets.SendCreateMobPacket](t, proximoDe(t, be, CodeCreateMob))
	if netsrv.CString(cm.Title[:]) == "Armas" {
		t.Error("o comprador ainda ve o nome da loja")
	}
	proximoDe(t, be, CodeTradeCancel) // janela do visitante fechada
	if b.Shop.visiting != 0 {
		t.Error("o visitante continua preso a loja")
	}
}

func TestVisitanteFechaSoASuaJanela(t *testing.T) {
	h, v, _, b, be := lojaAberta(t)
	if err := h.tradeCancel(context.Background(), be.conn, b, asPacket(t,
		miscPacket(CodeTradeCancel, packets.SignalData{}))); err != nil {
		t.Fatal(err)
	}
	if b.Shop.visiting != 0 {
		t.Error("continua visitando")
	}
	if !v.Shop.isOpen() {
		t.Error("a loja do vendedor fechou")
	}
}

func TestSairDoJogoFechaALoja(t *testing.T) {
	h, v, _, b, be := lojaAberta(t)
	_, vc, _ := h.Chan.Lookup(v.ClientID)
	h.OnDisconnect(vc)
	if b.Shop.visiting != 0 {
		t.Error("o visitante continua preso a uma loja que saiu")
	}
	proximoDe(t, be, CodeTradeCancel)
}

// ---- $338

func prepGrupo(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	h, a, ae, b, be := prepTroca(t)
	h.Parties = NewPartyRegistry()
	g, err := h.Parties.Create(a.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Parties.Add(g, b.ClientID); err != nil {
		t.Fatal(err)
	}
	a.Position = model.Position{X: 100, Y: 200}
	b.Position = model.Position{X: 1234, Y: 567}
	ae.drena(10 * time.Millisecond)
	be.drena(10 * time.Millisecond)
	return h, a, ae, b, be
}

func pedePosicao(t *testing.T, h *Handler, s *Session, e *espiao, alvo uint32) {
	t.Helper()
	if err := h.updateMemberPosition(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeUpdateMemberPosition, packets.PartyMemberPositionPacket{PlayerIndex: alvo}))); err != nil {
		t.Fatal(err)
	}
}

func TestPosicaoDeMembroEnviaOCoordDoOutro(t *testing.T) {
	h, a, ae, b, _ := prepGrupo(t)
	pedePosicao(t, h, a, ae, uint32(b.ClientID))
	got := decodifica[packets.PartyMemberCoordPacket](t, proximoDe(t, ae, CodePartyCoord))
	if got.Index != uint32(b.ClientID) || got.PosX != 1234 || got.PosY != 567 {
		t.Errorf("coord = %+v", got)
	}
}

func TestPosicaoDeMembroRecusas(t *testing.T) {
	h, a, ae, b, _ := prepGrupo(t)
	estranho, _ := entraJogadorSimples(t, h)
	for _, alvo := range []uint32{uint32(a.ClientID), 0, 0x10000, 0xFFFFFFFF, uint32(estranho.ClientID), 999} {
		pedePosicao(t, h, a, ae, alvo)
	}
	// Sem grupo.
	h.Parties.Remove(a.ClientID)
	pedePosicao(t, h, a, ae, uint32(b.ClientID))
	// Sem registro de grupos.
	h.Parties = nil
	pedePosicao(t, h, a, ae, uint32(b.ClientID))
	ae.nada(t, 50*time.Millisecond)
}

// ---- entrada aleatoria

func TestLojaEPosicaoComEntradaAleatoriaNaoQuebram(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	h.Parties = NewPartyRegistry()
	go func() {
		for range ve.pacotes {
		}
	}()
	go func() {
		for range be.pacotes {
		}
	}()
	r := rand.New(rand.NewSource(7))
	handlers := []struct {
		code uint16
		fn   func(context.Context, *netsrv.Conn, *Session, protocol.Packet) error
	}{
		{CodeShopCreate, h.createShop},
		{CodeShopOpen, h.openShop},
		{CodeShopBuy, h.buyShopItem},
		{CodeUpdateMemberPosition, h.updateMemberPosition},
		{CodeTradeCancel, h.tradeCancel},
	}
	for i := 0; i < 3000; i++ {
		hd := handlers[r.Intn(len(handlers))]
		corpo := make([]byte, r.Intn(400))
		r.Read(corpo)
		quem, e := v, ve
		if r.Intn(2) == 0 {
			quem, e = b, be
		}
		_ = hd.fn(context.Background(), e.conn, quem, protocol.Packet{Code: hd.code, Size: len(corpo), Data: corpo})
	}
}
