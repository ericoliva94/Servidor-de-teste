package game

import (
	"context"
	"testing"

	"aika/internal/model"
	"aika/internal/store"
)

func TestBolsaInicialEhReparadaAoCarregar(t *testing.T) {
	novo := func(it model.Item) store.ItemSet {
		inv := store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}
		inv.Slots[60] = it
		return inv
	}
	// Criada so com o Index (personagens antigos): ganha APP e quantidade.
	inv := novo(model.Item{Index: BolsaInicial})
	garantirBolsaInicial(&inv)
	if b := inv.Slots[60]; b.APP != BolsaInicial || b.Refi != 1 {
		t.Errorf("bolsa = %+v", b)
	}
	// Vazia: a origem poe a bolsa.
	inv = novo(model.Item{})
	garantirBolsaInicial(&inv)
	if b := inv.Slots[60]; b.Index != BolsaInicial || b.APP != BolsaInicial || b.Refi != 1 {
		t.Errorf("bolsa vazia = %+v", b)
	}
	// Outro item no slot 60 nao e tocado.
	inv = novo(model.Item{Index: 8033, APP: 0, Refi: 0})
	garantirBolsaInicial(&inv)
	if b := inv.Slots[60]; b.Index != 8033 || b.APP != 0 {
		t.Errorf("outro item mexido: %+v", b)
	}
}

func TestBolsaInicialNaoPodeSerDescartada(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Inventory.Slots[60] = model.Item{Index: BolsaInicial, APP: BolsaInicial, Refi: 1}
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	if err := h.deleteItem(context.Background(), e.conn, s, asPacket(t, deleteItemPacket(store.SlotTypeInventory, 60))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(60).Index != BolsaInicial {
		t.Error("a bolsa inicial foi apagada")
	}
	if got := mensagemDe(t, e); got != "A bolsa inicial não pode ser descartada." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestBolsaInicialAusenteEhRefeitaAoMoverItem(t *testing.T) {
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Inventory.Slots[60] = model.Item{} // some, como no personagem que a descartou
	s.Inventory.Slots[0] = model.Item{Index: 7, Refi: 1}
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	if _, err := h.resolveSlot(e.conn, s, store.SlotTypeInventory, 0, true); err != nil {
		t.Fatalf("slot 0 deveria voltar a ser acessivel: %v", err)
	}
	if b := s.Inventory.Get(60); b.Index != BolsaInicial || b.APP != BolsaInicial || b.Refi != 1 {
		t.Errorf("bolsa = %+v", b)
	}
}
