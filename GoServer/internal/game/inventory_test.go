package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// tabelaDeItens responde do mapa, como em itemfn.
type tabelaDeItens struct {
	itens map[int]gamedata.ItemFromList
	ref3  []gamedata.ArmorAttributeReinforce
}

func novaTabelaDeItens() *tabelaDeItens {
	return &tabelaDeItens{
		itens: map[int]gamedata.ItemFromList{},
		ref3:  make([]gamedata.ArmorAttributeReinforce, 600),
	}
}

func (t *tabelaDeItens) Item(i int) *gamedata.ItemFromList {
	it, ok := t.itens[i]
	if !ok {
		return nil
	}
	return &it
}

func (t *tabelaDeItens) Reinforce3At(i int) *gamedata.ArmorAttributeReinforce {
	if i < 0 || i >= len(t.ref3) {
		return nil
	}
	return &t.ref3[i]
}

// poeItem registra um item na tabela.
func (t *tabelaDeItens) poeItem(index int, it gamedata.ItemFromList) {
	t.itens[index] = it
}

// sessaoComInventario devolve uma sessao em jogo com as quatro bolsas postas.
func sessaoComInventario() *Session {
	s := sessaoEmJogo(100, 100)
	s.Avatar.Name = "Alfa"
	s.Avatar.ClassInfo = 1 // guerreiro
	s.Avatar.Level = 50
	s.Equip = store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}
	s.Inventory = store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}
	// As bolsas dos slots 60..63 habilitam 0..59.
	for _, b := range []int{60, 61, 62, 63} {
		s.Inventory.Slots[b] = model.Item{Index: 9000}
	}
	return s
}

// movePacket monta um $70F.
func moveItemPacket(srcType, srcSlot, destType, destSlot uint16) []byte {
	var p packets.MoveItemPacket
	p.SrcType = srcType
	p.SrcSlot = srcSlot
	p.DestType = destType
	p.DestSlot = destSlot
	buf, err := netsrv.Marshal(CodeMoveItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func deleteItemPacket(tipo, slot uint32) []byte {
	var p packets.DeleteItemPacket
	p.TypeSlot = tipo
	p.Slot = slot
	buf, err := netsrv.Marshal(CodeDeleteItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// handlerComItens devolve um handler com tabela e canal prontos.
func handlerComItens(t *testing.T) (*Handler, *tabelaDeItens) {
	tab := novaTabelaDeItens()
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	h.Data = tab
	return h, tab
}

// ---- slots que vem do cliente

// O bug central do MoveItem: srcItem e destItem comecavam em nil e so eram
// preenchidos dentro dos ramos do case. Um SrcType ou DestType fora da lista
// deixava o ponteiro nil e poucas linhas abaixo
// "ItemList[destItem.Index].ItemType" o dereferenciava.
func TestMoveItemTipoDeSlotDesconhecidoNaoQuebra(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 100}

	// Tipos que a origem nao trata: o case caia no fim sem atribuir.
	for _, tipo := range []uint16{4, 7, 8, 11, 20, 999, 65535} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(tipo, 0, store.SlotTypeInventory, 1))); err != nil {
			t.Fatalf("tipo %d na origem: %v", tipo, err)
		}
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeInventory, 0, tipo, 1))); err != nil {
			t.Fatalf("tipo %d no destino: %v", tipo, err)
		}
	}
	// Nada se moveu.
	if s.Inventory.Get(0).Index != 100 {
		t.Errorf("slot 0 = %d, queria 100", s.Inventory.Get(0).Index)
	}
}

// Slot fora da faixa e recusado nos dois lados.
func TestMoveItemSlotForaDaFaixa(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 100}

	for _, slot := range []uint16{60, 63, 64, 1000, 65535} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
				store.SlotTypeInventory, slot))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
	if s.Inventory.Get(0).Index != 100 {
		t.Errorf("slot 0 = %d, queria 100", s.Inventory.Get(0).Index)
	}
}

// Sem a bolsa o slot nao pode ser usado, como na origem.
func TestMoveItemExigeBolsa(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 100}
	// Tira a bolsa do segundo grupo (slots 15..29).
	s.Inventory.Slots[61] = model.Item{}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeInventory, 20))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 100 || !s.Inventory.Get(20).IsEmpty() {
		t.Error("o movimento deveria ter sido recusado sem a bolsa")
	}
}

func TestInventoryBagSlot(t *testing.T) {
	casos := map[int]int{
		0: 60, 14: 60, 15: 61, 29: 61, 30: 62, 44: 62, 45: 63, 59: 63,
		60: -1, 64: -1, 1000: -1,
	}
	for slot, quer := range casos {
		if got := inventoryBagSlot(slot); got != quer {
			t.Errorf("inventoryBagSlot(%d) = %d, queria %d", slot, got, quer)
		}
	}
}

// Equipamento so troca com inventario.
func TestEquipamentoSoTrocaComInventario(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[2] = model.Item{Index: 200}

	for _, tipo := range []uint16{store.SlotTypeStorage, store.SlotTypeEquip} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeEquip, 2, tipo, 3))); err != nil {
			t.Fatal(err)
		}
	}
	if s.Equip.Get(2).Index != 200 {
		t.Errorf("equipamento mudou: %d", s.Equip.Get(2).Index)
	}
}

// Os slots 0 e 1 do equipamento nao saem, como na origem
// ("(srcSlot < 16) and (srcSlot > 1)").
func TestSlotsZeroEUmNaoSaemDoEquipamento(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[0] = model.Item{Index: 10}
	s.Equip.Slots[1] = model.Item{Index: 11}

	for _, slot := range []uint16{0, 1} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeEquip, slot,
				store.SlotTypeInventory, 0))); err != nil {
			t.Fatal(err)
		}
	}
	if s.Equip.Get(0).Index != 10 || s.Equip.Get(1).Index != 11 {
		t.Error("slots 0 e 1 nao deveriam ter saido")
	}
}

// Arrastar o slot sobre ele mesmo nao duplica a pilha.
func TestMoverSobreOMesmoSlotNaoDuplica(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(300, gamedata.ItemFromList{CanAgroup: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 300, Refi: 10}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeInventory, 0))); err != nil {
		t.Fatal(err)
	}
	if got := s.Inventory.Get(0).Refi; got != 10 {
		t.Errorf("quantidade = %d, queria 10", got)
	}
}

// ---- movimento que funciona

func TestMoverNoInventarioTrocaDeLugar(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(100, gamedata.ItemFromList{})
	tab.poeItem(200, gamedata.ItemFromList{})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 100}
	s.Inventory.Slots[5] = model.Item{Index: 200}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeInventory, 5))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 200 || s.Inventory.Get(5).Index != 100 {
		t.Errorf("trocou errado: %d / %d",
			s.Inventory.Get(0).Index, s.Inventory.Get(5).Index)
	}
	// Dois pacotes de atualizacao, um por slot.
	for i := range 2 {
		if got := e.espera(t); got != CodeRefreshItem {
			t.Errorf("pacote %d = %#x, queria %#x", i, got, CodeRefreshItem)
		}
	}
}

// Pilhas iguais se juntam em vez de trocar de lugar.
func TestPilhasIguaisSeJuntam(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(300, gamedata.ItemFromList{CanAgroup: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 300, Refi: 30}
	s.Inventory.Slots[1] = model.Item{Index: 300, Refi: 20}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeInventory, 1))); err != nil {
		t.Fatal(err)
	}
	// A origem junta na origem e esvazia o destino.
	if got := s.Inventory.Get(0).Refi; got != 50 {
		t.Errorf("pilha junta = %d, queria 50", got)
	}
	if !s.Inventory.Get(1).IsEmpty() {
		t.Errorf("destino deveria estar vazio: %+v", s.Inventory.Get(1))
	}
}

// Equipar: do inventario para o slot certo.
func TestEquiparItemDoSlotCerto(t *testing.T) {
	h, tab := handlerComItens(t)
	// ItemType 2 -> slot 2 (elmo), classe 1 (guerreiro), nivel 10.
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 1, Level: 10})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 500}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeEquip, 2))); err != nil {
		t.Fatal(err)
	}
	if s.Equip.Get(2).Index != 500 {
		t.Errorf("equipamento = %d, queria 500", s.Equip.Get(2).Index)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("o slot de origem deveria estar vazio")
	}
	// O avatar e os maximos acompanham.
	if s.Avatar.Equip[2] != 500 {
		t.Errorf("avatar nao acompanhou: %d", s.Avatar.Equip[2])
	}
}

// Slot errado e recusado.
func TestEquiparNoSlotErradoERecusado(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 500}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeEquip, 5))); err != nil {
		t.Fatal(err)
	}
	if !s.Equip.Get(5).IsEmpty() {
		t.Error("nao deveria ter equipado no slot errado")
	}
}

// Nivel baixo e recusado com mensagem.
func TestEquiparComNivelBaixo(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 1, Level: 99})
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Level = 10
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 500}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeEquip, 2))); err != nil {
		t.Fatal(err)
	}
	if !s.Equip.Get(2).IsEmpty() {
		t.Error("nao deveria ter equipado")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// Classe errada e recusada.
func TestEquiparComClasseErrada(t *testing.T) {
	h, tab := handlerComItens(t)
	// Classe 41 e mago; o personagem e guerreiro.
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 41})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 500}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeEquip, 2))); err != nil {
		t.Fatal(err)
	}
	if !s.Equip.Get(2).IsEmpty() {
		t.Error("nao deveria ter equipado item de outra classe")
	}
}

// Desequipar devolve ao inventario.
func TestDesequiparVoltaAoInventario(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[2] = model.Item{Index: 500}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeEquip, 2,
			store.SlotTypeInventory, 3))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(3).Index != 500 {
		t.Errorf("inventario = %d, queria 500", s.Inventory.Get(3).Index)
	}
	if !s.Equip.Get(2).IsEmpty() {
		t.Error("o equipamento deveria estar vazio")
	}
}

// Pran e mascote avisam em vez de equipar algo que nao funciona.
func TestPranEMascoteSaoRecusadosComAviso(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(600, gamedata.ItemFromList{ItemType: 10}) // pran
	tab.poeItem(601, gamedata.ItemFromList{ItemType: 8})  // mascote
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)

	for i, idx := range []uint16{600, 601} {
		s.Inventory.Slots[0] = model.Item{Index: idx}
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
				store.SlotTypeEquip, 2))); err != nil {
			t.Fatal(err)
		}
		if !s.Equip.Get(2).IsEmpty() {
			t.Errorf("caso %d: nao deveria ter equipado", i)
		}
		if got := e.espera(t); got != CodeClientMsg {
			t.Errorf("caso %d: recebeu %#x, queria a mensagem", i, got)
		}
	}
}

// Mover item sem estar em jogo e recusado.
func TestMoverExigeEstarEmJogo(t *testing.T) {
	h, _ := handlerComItens(t)
	s := sessaoAutenticada()
	err := h.moveItem(context.Background(), connDescartavel(t), s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeInventory, 1)))
	if err != ErrNotPlaying {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

// ---- apagar item

func TestApagarItemDoInventario(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(100, gamedata.ItemFromList{})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[7] = model.Item{Index: 100, Refi: 5}

	if err := h.deleteItem(context.Background(), e.conn, s,
		asPacket(t, deleteItemPacket(store.SlotTypeInventory, 7))); err != nil {
		t.Fatal(err)
	}
	if !s.Inventory.Get(7).IsEmpty() {
		t.Error("o slot deveria estar vazio")
	}
	if got := e.espera(t); got != CodeRefreshItem {
		t.Errorf("recebeu %#x, queria %#x", got, CodeRefreshItem)
	}
}

func TestApagarItemSlotForaDaFaixa(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 100}

	casos := []struct{ tipo, slot uint32 }{
		{store.SlotTypeInventory, 64},
		{store.SlotTypeInventory, 1000},
		{store.SlotTypeInventory, 4000000000},
		{store.SlotTypeEquip, 16},
		{store.SlotTypeEquip, 65535},
		{99, 0},
		{store.SlotTypePranInv, 0},
	}
	for _, c := range casos {
		if err := h.deleteItem(context.Background(), e.conn, s,
			asPacket(t, deleteItemPacket(c.tipo, c.slot))); err != nil {
			t.Fatalf("tipo %d slot %d: %v", c.tipo, c.slot, err)
		}
	}
	if s.Inventory.Get(0).Index != 100 {
		t.Error("nada deveria ter sido apagado")
	}
	e.nada(t, 150*time.Millisecond)
}

// Reliquia nao e descartada aqui: na origem isso soltava um objeto no mapa, o
// que ainda nao esta portado.
func TestApagarReliquiaERecusado(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(700, gamedata.ItemFromList{ItemType: itemTypeReliquia})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Inventory.Slots[0] = model.Item{Index: 700}

	if err := h.deleteItem(context.Background(), e.conn, s,
		asPacket(t, deleteItemPacket(store.SlotTypeInventory, 0))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 700 {
		t.Error("a reliquia nao deveria ter sido apagada")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// Apagar equipamento refaz a aparencia e os maximos.
func TestApagarEquipamentoRefazOAvatar(t *testing.T) {
	h, tab := handlerComItens(t)
	tab.poeItem(500, gamedata.ItemFromList{ItemType: 2, Classe: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[2] = model.Item{Index: 500}
	s.Avatar.SetEquip(s.Equip)

	if err := h.deleteItem(context.Background(), e.conn, s,
		asPacket(t, deleteItemPacket(store.SlotTypeEquip, 2))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Equip[2] != 0 {
		t.Errorf("avatar ainda com o item: %d", s.Avatar.Equip[2])
	}
}

// Apagar slot vazio nao gera pacote.
func TestApagarSlotVazio(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	h.Chan.Join(e.conn, s)

	if err := h.deleteItem(context.Background(), e.conn, s,
		asPacket(t, deleteItemPacket(store.SlotTypeInventory, 0))); err != nil {
		t.Fatal(err)
	}
	e.nada(t, 150*time.Millisecond)
}

// ---- gravacao na saida

func TestSaidaGravaItens(t *testing.T) {
	h, _ := handlerComItens(t)
	gravados := map[string]store.ItemSet{}
	h.Items = itensFalsos{gravados: gravados}

	e := novoEspiao(t)
	s := sessaoComInventario()
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: 4242}
	s.Inventory.Slots[0] = model.Item{Index: 123}
	s.Equip.Slots[2] = model.Item{Index: 456}
	h.Chan.Join(e.conn, s)

	h.OnDisconnect(e.conn)

	if set, ok := gravados["equip:4242"]; !ok {
		t.Error("equipamento nao foi gravado")
	} else if set.Get(2).Index != 456 {
		t.Errorf("equipamento gravado = %d", set.Get(2).Index)
	}
	if set, ok := gravados["inv:4242"]; !ok {
		t.Error("inventario nao foi gravado")
	} else if set.Get(0).Index != 123 {
		t.Errorf("inventario gravado = %d", set.Get(0).Index)
	}
}

// Sem personagem escolhido nao ha o que gravar, e nao pode quebrar.
func TestSaidaSemPersonagemNaoGrava(t *testing.T) {
	h, _ := handlerComItens(t)
	gravados := map[string]store.ItemSet{}
	h.Items = itensFalsos{gravados: gravados}

	e := novoEspiao(t)
	s := sessaoComInventario()
	s.SelectedSlot = -1
	h.Chan.Join(e.conn, s)
	h.OnDisconnect(e.conn)

	if len(gravados) != 0 {
		t.Errorf("gravou %d conjuntos e nao deveria", len(gravados))
	}
}
