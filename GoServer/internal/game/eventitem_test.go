package game

import (
	"context"
	"sync"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// fakeEvents e a fila de itens de evento em memoria.
type fakeEvents struct {
	mu      sync.Mutex
	fila    []store.EventItem
	proximo uint32
	diario  bool
	// roubado marca ids que "outro pedido" ja levou: Delete devolve false.
	roubado map[uint32]bool
}

func (f *fakeEvents) List(_ context.Context, _ uint32) ([]store.EventItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.EventItem(nil), f.fila...), nil
}

func (f *fakeEvents) Delete(_ context.Context, _ uint32, id uint32) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.roubado[id] {
		return false, nil
	}
	for i, ev := range f.fila {
		if ev.ID == id {
			f.fila = append(f.fila[:i], f.fila[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeEvents) Add(_ context.Context, _ uint32, item, qtd uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proximo++
	f.fila = append(f.fila, store.EventItem{ID: 1000 + f.proximo, ItemID: item, Amount: qtd})
	return nil
}

func (f *fakeEvents) DiaryDue(_ context.Context, _ uint32, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.diario
	f.diario = false // como o banco: libera uma vez
	return d, nil
}

const (
	itemEvento    = 500 // nao empilha
	itemEventoPil = 501 // empilha
)

func cenarioEvento(t *testing.T) (*Handler, *Session, *espiao, *fakeEvents) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(itemEvento, gamedata.ItemFromList{})
	tab.poeItem(itemEventoPil, gamedata.ItemFromList{CanAgroup: 1})
	tab.poeItem(diaryItemID, gamedata.ItemFromList{})
	db := &fakeEvents{roubado: map[uint32]bool{}}
	h.EventDB = db
	s, e := entraJogadorSimples(t, h)
	s.Status = Playing
	s.SelectedSlot = 0
	s.Characters[0].ID = 5
	return h, s, e, db
}

func (c *fakeEvents) poe(id uint32, item, qtd uint16) {
	c.fila = append(c.fila, store.EventItem{ID: id, ItemID: item, Amount: qtd})
}

func pedeEvento(t *testing.T, h *Handler, s *Session, e *espiao) error {
	t.Helper()
	buf, err := netsrv.Marshal(CodeReceiveEventItem, packets.SignalData{})
	if err != nil {
		t.Fatal(err)
	}
	return h.receiveEventItem(context.Background(), e.conn, s, asPacket(t, buf))
}

func contaNoInventario(s *Session, item uint16) int {
	n := 0
	for _, it := range s.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == item {
			n++
		}
	}
	return n
}

func TestEventoEntregaOsItensEAvisa(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	db.poe(1, itemEvento, 1)
	db.poe(2, itemEventoPil, 7)
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if contaNoInventario(s, itemEvento) != 1 || contaNoInventario(s, itemEventoPil) != 1 {
		t.Errorf("inventario sem os itens do evento")
	}
	if len(db.fila) != 0 {
		t.Errorf("fila deveria esvaziar: %+v", db.fila)
	}
	if got := mensagemDe(t, e); got != "Não existem mais itens de evento para receber." {
		t.Errorf("mensagem = %q", got)
	}
	proximoDe(t, e, CodeReceiveEventItem)
}

func TestEventoDuasLinhasDoMesmoItemChegamAsDuas(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	db.poe(1, itemEvento, 1)
	db.poe(2, itemEvento, 1)
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	// A origem apagava pelo item_id: a segunda caixa sumia sem ser entregue.
	if got := contaNoInventario(s, itemEvento); got != 2 {
		t.Errorf("chegaram %d, queria 2", got)
	}
}

func TestEventoSoEntregaOQueCabe(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	for i := 1; i < MovableInventorySlots; i++ { // deixa so o slot 0 livre
		s.Inventory.Slots[i] = model.Item{Index: 300}
	}
	for i := uint32(1); i <= 3; i++ {
		db.poe(i, itemEvento, 1)
	}
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if len(db.fila) == 2 && (db.fila[0].ID != 2 || db.fila[1].ID != 3) {
		t.Errorf("as linhas que sobraram devem ser as originais, sem apagar e recriar: %+v", db.fila)
	}
	if contaNoInventario(s, itemEvento) != 1 || len(db.fila) != 2 {
		t.Errorf("entregues=%d, restam %d na fila (queria 1 e 2)", contaNoInventario(s, itemEvento), len(db.fila))
	}
}

func TestEventoComInventarioCheioNaoTocaNaFila(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: 300}
	}
	db.poe(1, itemEvento, 1)
	db.diario = true
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if len(db.fila) != 1 || !db.diario {
		t.Errorf("fila=%+v diario=%v: nada deve mudar sem espaco", db.fila, db.diario)
	}
	if got := mensagemDe(t, e); got != "Inventário cheio." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestEventoSemItensAvisaEMandaOSinal(t *testing.T) {
	h, s, e, _ := cenarioEvento(t)
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, e); got != "Não existem itens de evento para receber." {
		t.Errorf("mensagem = %q", got)
	}
	proximoDe(t, e, CodeReceiveEventItem)
}

func TestEventoPremioDiarioEntraNaFilaEChega(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	db.diario = true
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if contaNoInventario(s, diaryItemID) != 1 {
		t.Error("o premio diario deveria chegar")
	}
	// Segundo pedido: o banco ja registrou o dia.
	e.drena(20 * time.Millisecond)
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if contaNoInventario(s, diaryItemID) != 1 {
		t.Error("o premio diario nao pode chegar duas vezes")
	}
}

func TestEventoLinhaLevadaPorOutroPedidoNaoEntrega(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	db.poe(1, itemEvento, 1)
	db.roubado[1] = true
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if contaNoInventario(s, itemEvento) != 0 {
		t.Error("linha ja levada nao pode ser entregue de novo")
	}
}

func TestEventoForaDoJogoESemBanco(t *testing.T) {
	h, s, e, _ := cenarioEvento(t)
	s.Status = CharList
	if err := pedeEvento(t, h, s, e); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
	s.Status = Playing
	h.EventDB = nil
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, e); got != "Não existem itens de evento para receber." {
		t.Errorf("mensagem = %q", got)
	}
	proximoDe(t, e, CodeReceiveEventItem)
}

func TestEventoQueNaoCabeVoltaParaAFila(t *testing.T) {
	h, s, e, db := cenarioEvento(t)
	for i := 1; i < MovableInventorySlots; i++ { // so o slot 0 livre
		s.Inventory.Slots[i] = model.Item{Index: 300}
	}
	// 2500 unidades pedem 3 pilhas de 1000, mas so ha um slot livre.
	db.poe(1, itemEventoPil, 2500)
	if err := pedeEvento(t, h, s, e); err != nil {
		t.Fatal(err)
	}
	if contaNoInventario(s, itemEventoPil) != 0 {
		t.Error("nada deveria entrar no inventario")
	}
	if len(db.fila) != 1 || db.fila[0].ItemID != itemEventoPil || db.fila[0].Amount != 2500 {
		t.Errorf("a linha deveria voltar inteira para a fila: %+v", db.fila)
	}
}
