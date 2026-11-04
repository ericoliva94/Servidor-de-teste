package game

import (
	"context"
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// ---- montagem

// catalogoFalso responde do mapa, indexado pela POSICAO, e procura pelo campo
// Index como a tabela real.
type catalogoFalso struct{ linhas []gamedata.PremiumItem }

func (c *catalogoFalso) PremiumAt(i int) *gamedata.PremiumItem {
	if i < 0 || i >= len(c.linhas) {
		return nil
	}
	return &c.linhas[i]
}

func (c *catalogoFalso) PremiumByIndex(index uint32) (*gamedata.PremiumItem, int) {
	if index == 0 {
		return nil, -1
	}
	for i := range c.linhas {
		if c.linhas[i].Index == index {
			return &c.linhas[i], i
		}
	}
	return nil, -1
}

func (c *catalogoFalso) poe(l gamedata.PremiumItem) { c.linhas = append(c.linhas, l) }

// contaCashFalsa guarda o que SaveCash recebeu.
type contaCashFalsa struct{ saldo map[uint32]uint32 }

func (f contaCashFalsa) SaveCash(_ context.Context, id uint32, cash uint32) error {
	if f.saldo != nil {
		f.saldo[id] = cash
	}
	return nil
}

// handlerComPremium devolve handler com catalogo, itens falsos e canal.
func handlerComPremium(t *testing.T) (*Handler, *tabelaDeItens, *catalogoFalso, *itensFalsos, contaCashFalsa) {
	t.Helper()
	h, tabItens, itens := handlerComArmazem(t)
	cat := &catalogoFalso{}
	h.Premium = cat
	contas := contaCashFalsa{saldo: map[uint32]uint32{}}
	h.AccountCash = contas
	h.Exp = tabelaExp{}
	h.Durations = duracoesFixas{}
	return h, tabItens, cat, itens, contas
}

// sessaoComPremium devolve uma sessao com os 24 slots premium prontos.
func sessaoComPremium(nome string, charID uint32, conta uint32) *Session {
	s := sessaoComArmazem(nome, charID)
	s.Account.ID = conta
	s.Cash = store.ItemSet{Slots: make([]model.Item, store.CashSlots)}
	return s
}

func buyCashPacket(indice uint32) []byte {
	var p packets.SignalData
	p.Data = indice
	buf, err := netsrv.Marshal(CodeBuyItemCash, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func giftPacket(target uint16, slot uint16, nick string) []byte {
	var p packets.SendGiftPacket
	p.Target = target
	p.Slot = slot
	netsrv.PutCString(p.Nick[:], nick)
	buf, err := netsrv.Marshal(CodeSendGift, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func unsealPacket(slot uint32) []byte {
	var p packets.SignalData
	p.Data = slot
	buf, err := netsrv.Marshal(CodeUnsealItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- usar item premium

// "Premium := @CashInventory.Items[Slot]" com Slot vindo de Packet.Slot, que o
// handler so conferia contra 63, num vetor ARRAY [0..23]. Logo depois vem
// ZeroMemory(Premium, sizeof(TItemCash)): oito bytes gravados fora do vetor.
func TestSlotPremiumForaDaFaixaNaoEscreve(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 10, ItemIndex: 500, Amount: 1, Show: 1})

	for _, slot := range []int{24, 25, 63, 1000, -1} {
		if err := h.usePremiumItem(context.Background(), e.conn, s, slot); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
	for i := range s.Inventory.Slots[:60] {
		if !s.Inventory.Get(i).IsEmpty() {
			t.Fatalf("slot %d do inventario recebeu item", i)
		}
	}
}

// Nao havia checagem de slot vazio: com Premium.Index = 0 a origem lia
// PremiumItems[0] e entregava o item daquela linha, de graca e quantas vezes
// o jogador clicasse.
func TestSlotPremiumVazioNaoDaItemDeGraca(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// A linha 0 do catalogo e a que a origem entregaria.
	cat.poe(gamedata.PremiumItem{Index: 1, ItemIndex: 999, Amount: 1, Show: 1})

	for range 3 {
		if err := h.usePremiumItem(context.Background(), e.conn, s, 0); err != nil {
			t.Fatal(err)
		}
	}
	for i := range s.Inventory.Slots[:60] {
		if !s.Inventory.Get(i).IsEmpty() {
			t.Fatalf("o slot premium vazio entregou o item %d",
				s.Inventory.Get(i).Index)
		}
	}
}

func TestUsarItemPremiumEntregaEGrava(t *testing.T) {
	h, tab, cat, itens, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 50, "Pocao")
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Amount: 5, Show: 1})
	// O slot guarda o INDICE DO CATALOGO, nao o do item.
	s.Cash.Slots[3] = model.Item{Index: 77}

	if err := h.usePremiumItem(context.Background(), e.conn, s, 3); err != nil {
		t.Fatal(err)
	}
	if !s.Cash.Get(3).IsEmpty() {
		t.Error("o slot premium nao foi esvaziado")
	}
	achou := false
	for i := range s.Inventory.Slots[:60] {
		if s.Inventory.Get(i).Index == 500 {
			achou = true
			if got := s.Inventory.Get(i).Refi; got != 5 {
				t.Errorf("quantidade = %d, queria 5 do catalogo", got)
			}
		}
	}
	if !achou {
		t.Error("o item nao chegou ao inventario")
	}
	if _, ok := itens.gravados["cash:7000"]; !ok {
		t.Errorf("o inventario premium nao foi gravado: %v", itens.gravados)
	}
}

// "PremiumItems[Premium.Index]" indexa o catalogo pelo VALOR guardado, nao
// pela posicao; FindPremiumIndex existe na origem e nao e usada em lugar
// nenhum.
func TestUsarItemPremiumComIndiceForaDoCatalogo(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 1, ItemIndex: 500, Amount: 1, Show: 1})
	s.Cash.Slots[0] = model.Item{Index: 4242} // nao existe no catalogo

	if err := h.usePremiumItem(context.Background(), e.conn, s, 0); err != nil {
		t.Fatal(err)
	}
	if s.Cash.Get(0).Index != 4242 {
		t.Error("o slot foi esvaziado sem entregar nada")
	}
}

func TestUsarItemPremiumComInventarioCheio(t *testing.T) {
	h, tab, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 50, "Pocao")
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Amount: 1, Show: 1})
	s.Cash.Slots[0] = model.Item{Index: 77}
	for i := range 60 {
		s.Inventory.Slots[i] = model.Item{Index: 1234, Refi: 1}
	}

	if err := h.usePremiumItem(context.Background(), e.conn, s, 0); err != nil {
		t.Fatal(err)
	}
	if s.Cash.Get(0).IsEmpty() {
		t.Error("o item premium sumiu com o inventario cheio")
	}
}

// ---- compra

// "PremiumItems[Packet.Data].show" indexava um vetor dinamico com um DWORD do
// cliente, sem checagem, e o preco lido dali decidia a compra.
func TestCompraPremiumRecusaIndiceForaDoCatalogo(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 10, ItemIndex: 500, Price: 100, Show: 1})
	s.CashBalance = 100_000

	for _, idx := range []uint32{0, 1, 9999, 0xFFFFFFFF} {
		if err := h.buyItemCash(context.Background(), e.conn, s,
			asPacket(t, buyCashPacket(idx))); err != nil {
			t.Fatalf("indice %d: %v", idx, err)
		}
	}
	if s.CashBalance != 100_000 {
		t.Errorf("saldo = %d: uma compra invalida debitou", s.CashBalance)
	}
	if !s.Cash.Get(0).IsEmpty() {
		t.Error("um indice invalido entregou item")
	}
}

func TestCompraPremiumRespeitaOCampoShow(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 10, ItemIndex: 500, Price: 100, Show: 0})
	s.CashBalance = 1000

	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(10))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 1000 || !s.Cash.Get(0).IsEmpty() {
		t.Errorf("saldo=%d slot0=%d: item com Show 0 foi vendido",
			s.CashBalance, s.Cash.Get(0).Index)
	}
}

func TestCompraPremiumDebitaEGuardaOIndiceDoCatalogo(t *testing.T) {
	h, _, cat, _, contas := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Price: 250, Show: 1})
	s.CashBalance = 1000

	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(77))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 750 {
		t.Errorf("saldo = %d, queria 750", s.CashBalance)
	}
	if s.Cash.Get(0).Index != 77 {
		t.Errorf("slot 0 = %d, queria o indice do catalogo 77",
			s.Cash.Get(0).Index)
	}
	if got := contas.saldo[7000]; got != 750 {
		t.Errorf("gravado = %d, queria 750", got)
	}
}

func TestCompraPremiumSemCash(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Price: 250, Show: 1})
	s.CashBalance = 100

	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(77))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 100 || !s.Cash.Get(0).IsEmpty() {
		t.Errorf("saldo=%d slot0=%d", s.CashBalance, s.Cash.Get(0).Index)
	}
}

// DecCardinal calcula "Res: Integer := x - Value" com x sendo Cardinal: um
// saldo acima de 2.147.483.647 estourava o Integer, Res ficava negativo e a
// funcao zerava o cash inteiro.
func TestCompraPremiumComSaldoAltoNaoZeraOCash(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Price: 100, Show: 1})
	s.CashBalance = 3_000_000_000 // acima do maximo do Int32

	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(77))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 3_000_000_000-100 {
		t.Errorf("saldo = %d, queria %d", s.CashBalance, 3_000_000_000-100)
	}
}

func TestCompraPremiumSemEspacoNoInventarioCash(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Price: 100, Show: 1})
	s.CashBalance = 1000
	for i := range store.CashSlots {
		s.Cash.Slots[i] = model.Item{Index: 1}
	}

	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(77))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 1000 {
		t.Errorf("saldo = %d: debitou sem espaco", s.CashBalance)
	}
}

// ---- presente

// O destinatario saia de Packet.Target, um WORD do cliente usado como indice
// em Servers[].Players[Target] sem checagem, e o apelido digitado era
// ignorado.
func TestPresenteVaiPeloApelidoENaoPeloIndice(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 1})

	eA := novoEspiao(t)
	sA := sessaoComPremium("Alfa", 11, 7000)
	idA, err := h.Chan.Join(eA.conn, sA)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(idA, "Alfa")

	eB := novoEspiao(t)
	sB := sessaoComPremium("Beta", 22, 7001)
	idB, err := h.Chan.Join(eB.conn, sB)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(idB, "Beta")

	sA.Cash.Slots[2] = model.Item{Index: 77}

	// Target aponta para um indice absurdo; o apelido e que vale.
	if err := h.sendGift(context.Background(), eA.conn, sA,
		asPacket(t, giftPacket(65535, 2, "Beta"))); err != nil {
		t.Fatal(err)
	}
	if !sA.Cash.Get(2).IsEmpty() {
		t.Error("o presente nao saiu do remetente")
	}
	if sB.Cash.Get(0).Index != 77 {
		t.Errorf("destinatario slot 0 = %d, queria 77", sB.Cash.Get(0).Index)
	}
}

func TestPresenteParaApelidoInexistente(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 1})
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	id, err := h.Chan.Join(e.conn, s)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(id, "Alfa")
	s.Cash.Slots[0] = model.Item{Index: 77}

	if err := h.sendGift(context.Background(), e.conn, s,
		asPacket(t, giftPacket(0, 0, "NaoExiste"))); err != nil {
		t.Fatal(err)
	}
	if s.Cash.Get(0).Index != 77 {
		t.Error("o presente saiu sem destinatario")
	}
}

func TestPresenteParaSiMesmoEhRecusado(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 1})
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	id, err := h.Chan.Join(e.conn, s)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(id, "Alfa")
	s.Cash.Slots[0] = model.Item{Index: 77}

	if err := h.sendGift(context.Background(), e.conn, s,
		asPacket(t, giftPacket(0, 0, "Alfa"))); err != nil {
		t.Fatal(err)
	}
	if s.Cash.Get(0).Index != 77 {
		t.Error("o presente para si mesmo passou")
	}
}

func TestPresenteDeItemNaoTrocavel(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 0})

	eA := novoEspiao(t)
	sA := sessaoComPremium("Alfa", 11, 7000)
	idA, _ := h.Chan.Join(eA.conn, sA)
	h.Chan.RegisterCharacter(idA, "Alfa")
	eB := novoEspiao(t)
	sB := sessaoComPremium("Beta", 22, 7001)
	idB, _ := h.Chan.Join(eB.conn, sB)
	h.Chan.RegisterCharacter(idB, "Beta")
	sA.Cash.Slots[0] = model.Item{Index: 77}

	if err := h.sendGift(context.Background(), eA.conn, sA,
		asPacket(t, giftPacket(0, 0, "Beta"))); err != nil {
		t.Fatal(err)
	}
	if sA.Cash.Get(0).Index != 77 || !sB.Cash.Get(0).IsEmpty() {
		t.Errorf("item nao trocavel foi presenteado: A=%d B=%d",
			sA.Cash.Get(0).Index, sB.Cash.Get(0).Index)
	}
}

func TestPresenteComDestinatarioCheio(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 1})

	eA := novoEspiao(t)
	sA := sessaoComPremium("Alfa", 11, 7000)
	idA, _ := h.Chan.Join(eA.conn, sA)
	h.Chan.RegisterCharacter(idA, "Alfa")
	eB := novoEspiao(t)
	sB := sessaoComPremium("Beta", 22, 7001)
	idB, _ := h.Chan.Join(eB.conn, sB)
	h.Chan.RegisterCharacter(idB, "Beta")

	sA.Cash.Slots[0] = model.Item{Index: 77}
	for i := range store.CashSlots {
		sB.Cash.Slots[i] = model.Item{Index: 1}
	}

	if err := h.sendGift(context.Background(), eA.conn, sA,
		asPacket(t, giftPacket(0, 0, "Beta"))); err != nil {
		t.Fatal(err)
	}
	if sA.Cash.Get(0).Index != 77 {
		t.Error("o presente saiu com o destinatario cheio")
	}
}

func TestPresenteComSlotForaDaFaixa(t *testing.T) {
	h, _, cat, _, _ := handlerComPremium(t)
	cat.poe(gamedata.PremiumItem{Index: 77, ItemIndex: 500, Show: 1, Trade: 1})
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	id, _ := h.Chan.Join(e.conn, s)
	h.Chan.RegisterCharacter(id, "Alfa")

	for _, slot := range []uint16{24, 100, 65535} {
		if err := h.sendGift(context.Background(), e.conn, s,
			asPacket(t, giftPacket(0, slot, "Beta"))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
}

// ---- selo

// A origem chamava TItemFunctions.UseItem logo depois de desselar, e
// incondicionalmente: abrir o selo de uma pocao bebia a pocao.
func TestAbrirSeloNaoUsaOItem(t *testing.T) {
	h, tab, _, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// Pocao selavel: tem Duration na tabela.
	it := gamedata.ItemFromList{
		ItemType: ItemTypeHPPotion, UseEffect: 500, Duration: 24,
	}
	copy(it.Name[:], "Pocao selada")
	tab.poeItem(500, it)
	s.Avatar.MaxHP, s.Avatar.CurHP = 1000, 100
	s.Inventory.Slots[0] = model.Item{Index: 500} // Refi 0, Time 0 = selado

	if err := h.unsealItem(context.Background(), e.conn, s,
		asPacket(t, unsealPacket(0))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.CurHP != 100 {
		t.Errorf("HP = %d: abrir o selo bebeu a pocao", s.Avatar.CurHP)
	}
	if s.Inventory.Get(0).IsEmpty() {
		t.Error("o item foi consumido ao abrir o selo")
	}
	if s.Inventory.Get(0).IsSealed() {
		t.Error("o item continuou selado")
	}
}

// GetIsSealed le Refi+Time como um DWORD e considera selado quando sao zero —
// o que vale tambem para um equipamento comum de refino 0 sem prazo. Na
// origem, abrir o selo dele gravava um prazo de validade e em seguida o
// "usava".
func TestAbrirSeloNaoAtingeItemComumDeRefinoZero(t *testing.T) {
	h, tab, _, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// Equipamento comum: sem Duration, portanto nao e selavel.
	it := gamedata.ItemFromList{ItemType: 6}
	copy(it.Name[:], "Espada comum")
	tab.poeItem(1000, it)
	s.Inventory.Slots[0] = model.Item{Index: 1000, Min: 50, Max: 100}

	if err := h.unsealItem(context.Background(), e.conn, s,
		asPacket(t, unsealPacket(0))); err != nil {
		t.Fatal(err)
	}
	got := s.Inventory.Get(0)
	if got.Refi != 0 || got.Time != 0 {
		t.Errorf("o item comum ganhou prazo: Refi=%d Time=%d", got.Refi, got.Time)
	}
	if got.Min != 50 || got.Max != 100 {
		t.Errorf("a durabilidade mudou: %d/%d", got.Min, got.Max)
	}
}

func TestAbrirSeloDeSlotVazioOuForaDaFaixa(t *testing.T) {
	h, _, _, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []uint32{0, 10, 64, 1000, 0xFFFFFFFF} {
		if err := h.unsealItem(context.Background(), e.conn, s,
			asPacket(t, unsealPacket(slot))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
}

func TestAbrirSeloMortoEhRecusado(t *testing.T) {
	h, tab, _, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	it := gamedata.ItemFromList{ItemType: ItemTypeHPPotion, Duration: 24}
	copy(it.Name[:], "Pocao selada")
	tab.poeItem(500, it)
	s.Inventory.Slots[0] = model.Item{Index: 500}
	s.Dead = true

	if err := h.unsealItem(context.Background(), e.conn, s,
		asPacket(t, unsealPacket(0))); err != nil {
		t.Fatal(err)
	}
	if !s.Inventory.Get(0).IsSealed() {
		t.Error("o selo foi aberto com o personagem morto")
	}
}

// ---- envio dos pacotes

func TestEnvioDoInventarioPremiumEDoSaldo(t *testing.T) {
	h, _, _, _, _ := handlerComPremium(t)
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Cash.Slots[1] = model.Item{Index: 55, APP: 2, Identific: 7}
	s.CashBalance = 4321

	h.sendCashInventory(e.conn, s)
	if p := e.proximo(t); p.code != CodeCashInventory {
		t.Fatalf("codigo %#x, queria %#x", p.code, CodeCashInventory)
	}
	h.sendPlayerCash(e.conn, s)
	if p := e.proximo(t); p.code != CodePlayerCash {
		t.Fatalf("codigo %#x, queria %#x", p.code, CodePlayerCash)
	}
}

func TestCashCurtoDoBancoEhCompletado(t *testing.T) {
	h, _, _, itens, _ := handlerComPremium(t)
	itens.cash = map[uint32]store.ItemSet{
		7000: {Slots: make([]model.Item, 5)},
	}
	s := sessaoComPremium("Alfa", 11, 7000)
	got := h.loadCash(context.Background(), s)
	if len(got.Slots) != store.CashSlots {
		t.Errorf("slots = %d, queria %d", len(got.Slots), store.CashSlots)
	}
}

// Sem catalogo carregado a loja e recusada, em vez de ler fora do vetor.
func TestLojaPremiumSemCatalogo(t *testing.T) {
	h, _, _, _, _ := handlerComPremium(t)
	h.Premium = nil
	e := novoEspiao(t)
	s := sessaoComPremium("Alfa", 11, 7000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.CashBalance = 1000
	if err := h.buyItemCash(context.Background(), e.conn, s,
		asPacket(t, buyCashPacket(77))); err != nil {
		t.Fatal(err)
	}
	if s.CashBalance != 1000 {
		t.Errorf("saldo = %d", s.CashBalance)
	}
}
