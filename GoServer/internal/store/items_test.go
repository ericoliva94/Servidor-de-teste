package store

import (
	"strings"
	"testing"

	"aika/internal/model"
)

func linha(slot, itemID int64) itemRow {
	return itemRow{Slot: slot, ItemID: itemID, Min: 10, Max: 20}
}

func novoSet(n int) ItemSet { return ItemSet{Slots: make([]model.Item, n)} }

// O bug central do carregamento de itens: o slot vinha do banco e indexava
// Equip[ItemSlot] e Inventory[ItemSlot] sem checagem de faixa, escrevendo no
// meio do registro do personagem.
func TestSlotForaDaFaixaEIgnorado(t *testing.T) {
	out := novoSet(EquipSlots)
	vistos := map[int]bool{}

	for _, slot := range []int64{-1, EquipSlots, EquipSlots + 100, 1 << 40} {
		applyItemRow(&out, 7, SlotTypeEquip, linha(slot, 1234), vistos)
	}
	if len(out.Avisos) != 4 {
		t.Errorf("%d avisos, queria 4: %v", len(out.Avisos), out.Avisos)
	}
	for i, it := range out.Slots {
		if !it.IsEmpty() {
			t.Errorf("slot %d recebeu item e nao deveria", i)
		}
	}
}

func TestSlotValidoEntra(t *testing.T) {
	out := novoSet(EquipSlots)
	vistos := map[int]bool{}
	applyItemRow(&out, 7, SlotTypeEquip, itemRow{
		Slot: 3, ItemID: 1234, APP: 5678, Identific: -9,
		E1Index: 1, E2Index: 2, E3Index: 3,
		E1Value: 10, E2Value: 20, E3Value: 30,
		Min: 40, Max: 50, Refine: 7, Time: 99,
	}, vistos)

	it := out.Get(3)
	if it.Index != 1234 || it.APP != 5678 {
		t.Errorf("indice/aparencia = %d/%d", it.Index, it.APP)
	}
	if it.Identific != -9 {
		t.Errorf("identific = %d", it.Identific)
	}
	if it.Effects.Index != [3]byte{1, 2, 3} || it.Effects.Value != [3]byte{10, 20, 30} {
		t.Errorf("efeitos = %v", it.Effects)
	}
	if it.Min != 40 || it.Max != 50 || it.Refi != 7 || it.Time != 99 {
		t.Errorf("durabilidade/refino/tempo = %d/%d/%d/%d",
			it.Min, it.Max, it.Refi, it.Time)
	}
	if len(out.Avisos) != 0 {
		t.Errorf("avisos inesperados: %v", out.Avisos)
	}
}

// item_id zero e slot vazio, nao item.
func TestItemZeroNaoEntra(t *testing.T) {
	out := novoSet(InventorySlots)
	vistos := map[int]bool{}
	applyItemRow(&out, 7, SlotTypeInventory, linha(0, 0), vistos)
	if !out.Get(0).IsEmpty() {
		t.Error("slot 0 deveria ter ficado vazio")
	}
	// E o slot continua livre para uma linha boa.
	applyItemRow(&out, 7, SlotTypeInventory, linha(0, 55), vistos)
	if out.Get(0).Index != 55 {
		t.Errorf("slot 0 = %d, queria 55", out.Get(0).Index)
	}
}

// Slot repetido: o primeiro vence e o segundo e reportado, em vez de
// desaparecer em silencio.
func TestSlotRepetidoMantemOPrimeiro(t *testing.T) {
	out := novoSet(InventorySlots)
	vistos := map[int]bool{}
	applyItemRow(&out, 7, SlotTypeInventory, linha(5, 111), vistos)
	applyItemRow(&out, 7, SlotTypeInventory, linha(5, 222), vistos)

	if out.Get(5).Index != 111 {
		t.Errorf("slot 5 = %d, queria 111", out.Get(5).Index)
	}
	if len(out.Avisos) != 1 || !strings.Contains(out.Avisos[0], "repetido") {
		t.Errorf("avisos = %v", out.Avisos)
	}
}

// As colunas sao INT e os campos tem larguras menores: nada pode dar a volta.
func TestColunasForaDaFaixaSaoLimitadas(t *testing.T) {
	out := novoSet(EquipSlots)
	vistos := map[int]bool{}
	applyItemRow(&out, 7, SlotTypeEquip, itemRow{
		Slot: 0, ItemID: 1 << 40, APP: -5,
		E1Index: 300, E1Value: -1,
		Min: 999, Max: -999, Refine: 1 << 30, Time: -1,
	}, vistos)

	it := out.Get(0)
	if it.Index != 65535 {
		t.Errorf("Index = %d, queria 65535", it.Index)
	}
	if it.APP != 0 {
		t.Errorf("APP = %d, queria 0", it.APP)
	}
	if it.Effects.Index[0] != 255 || it.Effects.Value[0] != 0 {
		t.Errorf("efeito = %d/%d", it.Effects.Index[0], it.Effects.Value[0])
	}
	if it.Min != 255 || it.Max != 0 {
		t.Errorf("durabilidade = %d/%d", it.Min, it.Max)
	}
	if it.Refi != 65535 || it.Time != 0 {
		t.Errorf("refino/tempo = %d/%d", it.Refi, it.Time)
	}
}

// Get fora da faixa devolve item vazio em vez de entrar em panico.
func TestGetForaDaFaixa(t *testing.T) {
	s := novoSet(4)
	for _, i := range []int{-1, 4, 1000} {
		if !s.Get(i).IsEmpty() {
			t.Errorf("Get(%d) deveria ser vazio", i)
		}
	}
}

func TestClampToInt32(t *testing.T) {
	casos := []struct {
		in   int64
		quer int32
	}{
		{0, 0}, {-1, -1}, {2147483647, 2147483647},
		{2147483648, 2147483647},
		{-2147483648, -2147483648},
		{-2147483649, -2147483648},
	}
	for _, c := range casos {
		if got := clampToInt32(c.in); got != c.quer {
			t.Errorf("clampToInt32(%d) = %d, queria %d", c.in, got, c.quer)
		}
	}
}

// Os tamanhos tem de casar com os vetores do registro do personagem.
func TestTamanhosDosVetores(t *testing.T) {
	if EquipSlots != 16 {
		t.Errorf("Equip tem %d slots, TCharacter diz 16", EquipSlots)
	}
	if InventorySlots != 64 {
		t.Errorf("Inventory tem %d slots, TCharacter diz 64", InventorySlots)
	}
}

// ---- skills

func TestClampSkillLevel(t *testing.T) {
	casos := map[int64]uint16{
		0: 0, 5: 5, 15: 15,
		16: MaxSkillLevelDB, 100: MaxSkillLevelDB, 1 << 40: MaxSkillLevelDB,
		-1: 0, -100: 0,
	}
	for in, quer := range casos {
		if got := clampSkillLevel(in); got != quer {
			t.Errorf("clampSkillLevel(%d) = %d, queria %d", in, got, quer)
		}
	}
}

// Os tamanhos tem de casar com os vetores de PlayerData.pas.
func TestTamanhosDeSkill(t *testing.T) {
	if BasicSkillSlots != 6 {
		t.Errorf("Basics tem %d slots, PlayerData.pas diz 6", BasicSkillSlots)
	}
	if OtherSkillSlots != 40 {
		t.Errorf("Others tem %d slots, PlayerData.pas diz 40", OtherSkillSlots)
	}
}
