package itemfn

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// tabelasFalsas responde do mapa, para o teste nao depender dos .bin.
type tabelasFalsas struct {
	itens      map[int]gamedata.ItemFromList
	reinforce3 []gamedata.ArmorAttributeReinforce
}

func (t *tabelasFalsas) Item(i int) *gamedata.ItemFromList {
	it, ok := t.itens[i]
	if !ok {
		return nil
	}
	return &it
}

func (t *tabelasFalsas) Reinforce3At(i int) *gamedata.ArmorAttributeReinforce {
	if i < 0 || i >= len(t.reinforce3) {
		return nil
	}
	return &t.reinforce3[i]
}

func novasTabelas() *tabelasFalsas {
	return &tabelasFalsas{
		itens:      map[int]gamedata.ItemFromList{},
		reinforce3: make([]gamedata.ArmorAttributeReinforce, 600),
	}
}

func (t *tabelasFalsas) poe(index int, itemType, useEffect uint16) {
	t.itens[index] = gamedata.ItemFromList{ItemType: itemType, UseEffect: useEffect}
}

func TestEquipSlot(t *testing.T) {
	tb := novasTabelas()
	tb.poe(1, 2, 0)    // elmo
	tb.poe(2, 15, 0)   // ultimo slot da faixa 1..15
	tb.poe(3, 1005, 0) // anel
	tb.poe(4, 50, 0)
	tb.poe(5, 52, 0)
	tb.poe(6, 0, 0)
	tb.poe(7, 999, 0)

	casos := []struct {
		index uint16
		quer  int
	}{
		{1, 2}, {2, 15}, {3, SlotRing}, {4, 15}, {5, 15}, {6, 0}, {7, 0},
		{999, 0}, // item que nao existe na tabela
	}
	for _, c := range casos {
		if got := EquipSlot(tb, c.index); got != c.quer {
			t.Errorf("EquipSlot(%d) = %d, queria %d", c.index, got, c.quer)
		}
	}
}

// A origem usava Round(Refi / 16), com arredondamento bancario, e nao
// divisao inteira. Trocar mudaria o refino de metade dos itens.
func TestReinforceFromItemUsaArredondamentoBancario(t *testing.T) {
	casos := []struct {
		refi uint16
		quer int
	}{
		{0, 0},  // sem refino
		{8, 0},  // 0.5 -> empate vai para o par
		{16, 1}, // 1.0
		{24, 2}, // 1.5 -> empate vai para o par
		{25, 2}, // 1.5625
		{32, 2}, // 2.0
		{40, 2}, // 2.5 -> empate vai para o par
		{56, 4}, // 3.5 -> empate vai para o par
		{160, 10},
	}
	for _, c := range casos {
		got := ReinforceFromItem(model.Item{Refi: c.refi})
		if got != c.quer {
			t.Errorf("ReinforceFromItem(Refi=%d) = %d, queria %d",
				c.refi, got, c.quer)
		}
	}
}

func TestReinforce3IndexFaixas(t *testing.T) {
	tb := novasTabelas()
	// Elmo (slot 2 -> area 0) com UseEffect em cada faixa.
	tb.poe(10, 2, 0)
	tb.poe(11, 2, 35)
	tb.poe(12, 2, 36)
	tb.poe(13, 2, 70)
	tb.poe(14, 2, 71)
	tb.poe(15, 2, 105)

	casos := []struct {
		index uint16
		quer  int
	}{
		{10, 0 + 0},
		{11, 0 + 35},
		{12, 150 - 35 + 36},
		{13, 150 - 35 + 70},
		{14, 300 - 70 + 71},
		{15, 300 - 70 + 105},
	}
	for _, c := range casos {
		if got := Reinforce3Index(tb, c.index); got != c.quer {
			t.Errorf("Reinforce3Index(%d) = %d, queria %d", c.index, got, c.quer)
		}
	}
}

// UseEffect fora das tres faixas que a origem conhece nao gera indice: ali o
// resultado era o proprio UseEffect e indexava a tabela fora do lugar.
func TestReinforce3IndexForaDasFaixas(t *testing.T) {
	tb := novasTabelas()
	tb.poe(20, 2, 106)
	tb.poe(21, 2, 65535)
	for _, idx := range []uint16{20, 21} {
		if got := Reinforce3Index(tb, idx); got != -1 {
			t.Errorf("Reinforce3Index(%d) = %d, queria -1", idx, got)
		}
	}
	if got := Reinforce3Index(tb, 999); got != -1 {
		t.Errorf("item inexistente deu %d, queria -1", got)
	}
}

func TestReinforce3IndexSomaAArea(t *testing.T) {
	tb := novasTabelas()
	tb.poe(30, 2, 5) // elmo  -> +0
	tb.poe(31, 3, 5) // armadura -> +30
	tb.poe(32, 4, 5) // luva -> +60
	tb.poe(33, 5, 5) // bota -> +90
	tb.poe(34, 7, 5) // escudo -> +0 (mesma area do elmo)

	quer := map[uint16]int{30: 5, 31: 35, 32: 65, 33: 95, 34: 5}
	for idx, q := range quer {
		if got := Reinforce3Index(tb, idx); got != q {
			t.Errorf("Reinforce3Index(%d) = %d, queria %d", idx, got, q)
		}
	}
}

// O bug grave: "Refine - 1" com Refine podendo chegar a 4095 indexava um
// vetor de 16 posicoes muito alem do fim.
func TestReinforceHPMPIncRecusaNivelForaDaFaixa(t *testing.T) {
	tb := novasTabelas()
	tb.poe(40, 2, 0)
	tb.reinforce3[0].HealthIncrementPoints[0] = 111
	tb.reinforce3[0].HealthIncrementPoints[15] = 999

	if got := ReinforceHPMPInc(tb, 40, 0); got != 111 {
		t.Errorf("nivel 0 = %d, queria 111", got)
	}
	if got := ReinforceHPMPInc(tb, 40, 15); got != 999 {
		t.Errorf("nivel 15 = %d, queria 999", got)
	}
	for _, nivel := range []int{-1, 16, 4094, 1 << 20} {
		if got := ReinforceHPMPInc(tb, 40, nivel); got != 0 {
			t.Errorf("nivel %d = %d, queria 0", nivel, got)
		}
	}
}

func TestEquippedHPMPIncSomaOsSlotsCertos(t *testing.T) {
	tb := novasTabelas()
	// Um item por slot, todos com UseEffect 0.
	for slot, tipo := range map[int]uint16{2: 2, 3: 3, 4: 4, 5: 5, 6: 1005, 7: 7} {
		tb.poe(100+slot, tipo, 0)
	}
	// Bonus 10 no nivel 0 em cada area.
	for _, i := range []int{0, 30, 60, 90} {
		tb.reinforce3[i].HealthIncrementPoints[0] = 10
	}

	equip := make([]model.Item, 16)
	for slot := 2; slot <= 7; slot++ {
		// Refi 16 -> refino 1 -> indice 0 em HealthIncrementPoints.
		equip[slot] = model.Item{Index: uint16(100 + slot), Refi: 16}
	}

	// Slots 2,3,4,5,7 contam; o 6 e pulado. O escudo (7) divide area com o
	// elmo (2), entao sao 5 itens somando 10 cada.
	if got := EquippedHPMPInc(tb, equip); got != 50 {
		t.Errorf("soma = %d, queria 50", got)
	}
}

// Item com prazo (Time > 0) nao conta, como na origem.
func TestEquippedHPMPIncIgnoraItemComPrazo(t *testing.T) {
	tb := novasTabelas()
	tb.poe(200, 2, 0)
	tb.reinforce3[0].HealthIncrementPoints[0] = 10

	equip := make([]model.Item, 16)
	equip[2] = model.Item{Index: 200, Refi: 16, Time: 1}
	if got := EquippedHPMPInc(tb, equip); got != 0 {
		t.Errorf("soma = %d, queria 0", got)
	}
	equip[2].Time = 0
	if got := EquippedHPMPInc(tb, equip); got != 10 {
		t.Errorf("soma = %d, queria 10", got)
	}
}

// Item sem refino nao conta, e slot vazio nao entra em panico.
func TestEquippedHPMPIncSemRefinoNemItem(t *testing.T) {
	tb := novasTabelas()
	tb.poe(300, 2, 0)
	tb.reinforce3[0].HealthIncrementPoints[0] = 10

	equip := make([]model.Item, 16)
	equip[2] = model.Item{Index: 300, Refi: 0}
	if got := EquippedHPMPInc(tb, equip); got != 0 {
		t.Errorf("sem refino = %d, queria 0", got)
	}
	if got := EquippedHPMPInc(tb, nil); got != 0 {
		t.Errorf("sem equipamento = %d, queria 0", got)
	}
	if got := EquippedHPMPInc(tb, make([]model.Item, 3)); got != 0 {
		t.Errorf("vetor curto = %d, queria 0", got)
	}
}

// Tabela nao carregada responde zero em vez de quebrar.
func TestTabelaVaziaNaoQuebra(t *testing.T) {
	var t0 *gamedata.Tables
	if got := EquippedHPMPInc(t0, make([]model.Item, 16)); got != 0 {
		t.Errorf("soma = %d, queria 0", got)
	}
}
