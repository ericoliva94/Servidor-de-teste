package game

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

const (
	itemPocao  = 300 // empilha
	itemEspada = 400 // nao empilha
	itemTemp   = 500 // empilha e expira
)

func prepStack(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(itemPocao, gamedata.ItemFromList{CanAgroup: 1})
	tab.poeItem(itemEspada, gamedata.ItemFromList{})
	tab.poeItem(itemTemp, gamedata.ItemFromList{CanAgroup: 1, Expires: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, s, e
}

func agroupPacket(src, dest uint32) []byte {
	buf, err := netsrv.Marshal(CodeAgroupItem, packets.AgroupItemPacket{SrcSlot: src, DestSlot: dest})
	if err != nil {
		panic(err)
	}
	return buf
}

func ungroupPacket(slot, qtd, tipo uint32) []byte {
	buf, err := netsrv.Marshal(CodeUngroupItem, packets.UngroupItemPacket{Slot: slot, Quantidade: qtd, SlotType: tipo})
	if err != nil {
		panic(err)
	}
	return buf
}

func agrupa(t *testing.T, h *Handler, s *Session, e *espiao, src, dest uint32) {
	t.Helper()
	if err := h.agroupItem(context.Background(), e.conn, s, asPacket(t, agroupPacket(src, dest))); err != nil {
		t.Fatal(err)
	}
}

func separa(t *testing.T, h *Handler, s *Session, e *espiao, slot, qtd uint32) {
	t.Helper()
	if err := h.ungroupItem(context.Background(), e.conn, s,
		asPacket(t, ungroupPacket(slot, qtd, store.SlotTypeInventory))); err != nil {
		t.Fatal(err)
	}
}

func TestAgruparSomaEmDestinoEEsvaziaOrigem(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 30}
	s.Inventory.Slots[1] = model.Item{Index: itemPocao, Refi: 12}

	agrupa(t, h, s, e, 0, 1)

	if !s.Inventory.Slots[0].IsEmpty() {
		t.Error("origem nao foi esvaziada")
	}
	if got := s.Inventory.Slots[1].Refi; got != 42 {
		t.Errorf("destino = %d, queria 42", got)
	}
	// Dois $F0E: origem e destino.
	proximoDe(t, e, CodeRefreshItem)
	proximoDe(t, e, CodeRefreshItem)
}

// Defeito da origem: sem conferir CanAgroup, dois equipamentos iguais
// "juntavam" e o refino de um era somado ao do outro.
func TestAgruparEquipamentoNaoSomaRefino(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemEspada, Refi: 48}
	s.Inventory.Slots[1] = model.Item{Index: itemEspada, Refi: 48}

	agrupa(t, h, s, e, 0, 1)

	if s.Inventory.Slots[0].Refi != 48 || s.Inventory.Slots[1].Refi != 48 {
		t.Errorf("refino alterado: %d e %d", s.Inventory.Slots[0].Refi, s.Inventory.Slots[1].Refi)
	}
	e.nada(t, 30*time.Millisecond)
}

// Defeito da origem: src == dest dobrava e em seguida zerava o slot.
func TestAgruparMesmoSlotNaoApagaOItem(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[3] = model.Item{Index: itemPocao, Refi: 10}

	agrupa(t, h, s, e, 3, 3)

	if got := s.Inventory.Slots[3]; got.Index != itemPocao || got.Refi != 10 {
		t.Errorf("item = %+v, queria intacto", got)
	}
}

func TestAgruparItensDiferentesNaoFazNada(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 5}
	s.Inventory.Slots[1] = model.Item{Index: itemTemp, Refi: 5}

	agrupa(t, h, s, e, 0, 1)

	if s.Inventory.Slots[0].Refi != 5 || s.Inventory.Slots[1].Refi != 5 {
		t.Error("itens diferentes foram juntados")
	}
}

func TestAgruparAcimaDoTetoDeixaOResto(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 700}
	s.Inventory.Slots[1] = model.Item{Index: itemPocao, Refi: 600}

	agrupa(t, h, s, e, 0, 1)

	if got := s.Inventory.Slots[1].Refi; got != itemfn.MaxSlotAmount {
		t.Errorf("destino = %d, queria %d", got, itemfn.MaxSlotAmount)
	}
	if got := s.Inventory.Slots[0].Refi; got != 300 {
		t.Errorf("origem = %d, queria o resto 300 (nada se perde)", got)
	}
}

func TestAgruparSlotSemBolsaOuForaDaFaixaERecusado(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 5}
	s.Inventory.Slots[1] = model.Item{Index: itemPocao, Refi: 5}
	s.Inventory.Slots[61] = model.Item{} // tira a bolsa do segundo grupo
	s.Inventory.Slots[20] = model.Item{Index: itemPocao, Refi: 5}

	agrupa(t, h, s, e, 0, 20)  // destino sem bolsa
	agrupa(t, h, s, e, 0, 63)  // slot de bolsa
	agrupa(t, h, s, e, 0, 999) // fora da faixa
	agrupa(t, h, s, e, 0, 0xFFFFFFFF)

	if s.Inventory.Slots[0].Refi != 5 || s.Inventory.Slots[20].Refi != 5 {
		t.Error("pilha alterada por pedido invalido")
	}
}

func TestSepararCriaSegundaPilha(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 10}

	separa(t, h, s, e, 0, 4)

	if s.Inventory.Slots[0].Refi != 6 {
		t.Errorf("origem = %d, queria 6", s.Inventory.Slots[0].Refi)
	}
	if got := s.Inventory.Slots[1]; got.Index != itemPocao || got.Refi != 4 {
		t.Errorf("nova pilha = %+v, queria 4 no primeiro slot livre", got)
	}
}

// Defeito da origem: Expires era o unico filtro, entao um equipamento com
// Refi 48 separado em 16 gerava uma COPIA do equipamento.
func TestSepararEquipamentoNaoDuplica(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemEspada, Refi: 48, Identific: 77}

	separa(t, h, s, e, 0, 16)

	for i := 1; i < MovableInventorySlots; i++ {
		if !s.Inventory.Slots[i].IsEmpty() {
			t.Fatalf("slot %d recebeu uma copia: %+v", i, s.Inventory.Slots[i])
		}
	}
	if s.Inventory.Slots[0].Refi != 48 {
		t.Errorf("refino do original = %d, queria 48", s.Inventory.Slots[0].Refi)
	}
}

func TestSepararQuantidadesInvalidas(t *testing.T) {
	for _, q := range []uint32{0, 10, 11, 1 << 31, 0xFFFFFFFF} {
		h, s, e := prepStack(t)
		s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 10}
		separa(t, h, s, e, 0, q)
		if s.Inventory.Slots[0].Refi != 10 || !s.Inventory.Slots[1].IsEmpty() {
			t.Errorf("quantidade %d alterou o inventario", q)
		}
	}
}

func TestSepararItemQueExpiraERecusado(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemTemp, Refi: 10}
	separa(t, h, s, e, 0, 3)
	if s.Inventory.Slots[0].Refi != 10 || !s.Inventory.Slots[1].IsEmpty() {
		t.Error("item com prazo foi separado")
	}
}

func TestSepararSemEspacoAvisa(t *testing.T) {
	h, s, e := prepStack(t)
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: itemEspada}
	}
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 10}

	separa(t, h, s, e, 0, 4)

	if got := mensagemDe(t, e); got != "Inventario cheio." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[0].Refi != 10 {
		t.Error("pilha mexida sem espaco")
	}
}

func TestSepararNaoUsaSlotSemBolsa(t *testing.T) {
	h, s, e := prepStack(t)
	// Enche o primeiro grupo; o segundo fica sem bolsa.
	s.Inventory.Slots[61] = model.Item{}
	for i := 0; i < 15; i++ {
		s.Inventory.Slots[i] = model.Item{Index: itemEspada}
	}
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 10}

	separa(t, h, s, e, 0, 4)

	if !s.Inventory.Slots[15].IsEmpty() {
		t.Error("usou slot cuja bolsa nao esta no inventario")
	}
}

func TestSepararOutrosTiposDeSlot(t *testing.T) {
	h, s, e := prepStack(t)
	s.Inventory.Slots[0] = model.Item{Index: itemPocao, Refi: 10}
	for _, tipo := range []uint32{store.SlotTypeEquip, store.SlotTypeStorage, 99} {
		if err := h.ungroupItem(context.Background(), e.conn, s,
			asPacket(t, ungroupPacket(0, 4, tipo))); err != nil {
			t.Fatal(err)
		}
	}
	if s.Inventory.Slots[0].Refi != 10 {
		t.Error("tipo sem suporte mexeu no inventario")
	}
	// Bolsa da pran sem pran invocada: nao ha de onde separar.
	if err := h.ungroupItem(context.Background(), e.conn, s,
		asPacket(t, ungroupPacket(0, 4, store.SlotTypePranInv))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Slots[0].Refi != 10 {
		t.Error("bolsa da pran sem pran mexeu no inventario")
	}
}

func TestPilhaForaDoJogoERecusada(t *testing.T) {
	h, s, e := prepStack(t)
	s.Status = CharList
	if err := h.agroupItem(context.Background(), e.conn, s, asPacket(t, agroupPacket(0, 1))); err != ErrNotPlaying {
		t.Errorf("agroup erro = %v", err)
	}
	if err := h.ungroupItem(context.Background(), e.conn, s,
		asPacket(t, ungroupPacket(0, 1, 1))); err != ErrNotPlaying {
		t.Errorf("ungroup erro = %v", err)
	}
}

func TestTamanhosDosPacotesDePilha(t *testing.T) {
	if got := binary.Size(packets.AgroupItemPacket{}); got != 20 {
		t.Errorf("AgroupItemPacket = %d, o Delphi tem 20", got)
	}
	if got := binary.Size(packets.UngroupItemPacket{}); got != 24 {
		t.Errorf("UngroupItemPacket = %d, o Delphi tem 24", got)
	}
}
