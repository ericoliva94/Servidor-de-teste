package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/store"
)

const (
	itemPranArma   = 7100 // ItemType 20 -> slot 2 da pran, nivel 7, classe 100
	itemPranAlto   = 7101 // ItemType 20, nivel 8: alto demais para a pran nivel 6
	itemPranClasse = 7102 // ItemType 20, classe 101: nao e da forma 62
	itemPranSlot3  = 7103 // ItemType 21 -> slot 3
	itemPranPreso  = 7104 // TypeTrade 1
	itemPranPilha  = 7105 // empilha
	itemPranBolsa  = 7106 // bolsa da pran (so precisa existir no slot 40/41)
)

// cenarioItensPran deixa a pran 1 invocada, com a primeira bolsa, e o inventario
// do dono com os itens de teste.
func cenarioItensPran(t *testing.T) *cenarioInvocar {
	t.Helper()
	c := novoCenarioInvocar(t)
	tab := c.h.Data.(*tabelaDeItens)
	tab.poeItem(itemPranArma, gamedata.ItemFromList{ItemType: 20, Level: 7, Classe: 100,
		EF: [3]uint16{182}, EFV: [3]uint16{7}})
	tab.poeItem(itemPranAlto, gamedata.ItemFromList{ItemType: 20, Level: 8, Classe: 100})
	tab.poeItem(itemPranClasse, gamedata.ItemFromList{ItemType: 20, Level: 1, Classe: 101})
	tab.poeItem(itemPranSlot3, gamedata.ItemFromList{ItemType: 21, Level: 1})
	tab.poeItem(itemPranPreso, gamedata.ItemFromList{ItemType: 3, TypeTrade: 1})
	tab.poeItem(itemPranPilha, gamedata.ItemFromList{ItemType: 3, CanAgroup: 1})
	tab.poeItem(itemPranBolsa, gamedata.ItemFromList{ItemType: 3})
	c.s.Inventory.Slots[2] = model.Item{Index: itemPranArma}
	c.s.Inventory.Slots[3] = model.Item{Index: itemPranAlto}
	c.s.Inventory.Slots[4] = model.Item{Index: itemPranClasse}
	c.s.Inventory.Slots[5] = model.Item{Index: itemPranSlot3}
	c.s.Inventory.Slots[6] = model.Item{Index: itemPranPreso}
	c.s.Inventory.Slots[7] = model.Item{Index: itemPranPilha, Refi: 10}
	c.p1.Inventory[40] = model.Item{Index: itemPranBolsa}
	return c
}

// invoca deixa a pran 1 no mundo e limpa o que ja chegou aos espioes.
func (c *cenarioInvocar) invoca(t *testing.T) {
	t.Helper()
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	c.e.drena(20 * time.Millisecond)
	c.eViz.drena(20 * time.Millisecond)
}

func TestEquiparPecaNaPranSomaEfeitoEReenviaAPran(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, 2)
	if c.p1.Equip[2].Index != itemPranArma || !c.s.Inventory.Get(2).IsEmpty() {
		t.Fatalf("equip[2]=%+v inv[2]=%+v", c.p1.Equip[2], c.s.Inventory.Get(2))
	}
	if got := c.s.Avatar.Ability.get(efPranDamage1); got != 7 {
		t.Errorf("efeito da peca = %d, queria 7", got)
	}
	proximoDe(t, c.e, CodePranSpawn) // a pran e reenviada com a aparencia nova
	proximoDe(t, c.eViz, CodePranSpawn)
}

func TestTirarAPecaDaPranDevolveOItemELimpaOEfeito(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, 2)
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypeInventory, 9)
	if !c.p1.Equip[2].IsEmpty() || c.s.Inventory.Get(9).Index != itemPranArma {
		t.Fatalf("equip[2]=%+v inv[9]=%+v", c.p1.Equip[2], c.s.Inventory.Get(9))
	}
	if got := c.s.Avatar.Ability.get(efPranDamage1); got != 0 {
		t.Errorf("sobrou %d do efeito", got)
	}
}

func TestTrocarPecasDaPranConfereAPecaQueEntra(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, 2)
	// Troca a peca do slot 2 por outra boa para o slot 2.
	c.s.Inventory.Slots[8] = model.Item{Index: itemPranArma, Refi: 1}
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypeInventory, 8)
	if c.p1.Equip[2].Index != itemPranArma || c.s.Inventory.Get(8).Index != itemPranArma {
		t.Errorf("a troca valida deveria passar: equip=%+v inv=%+v", c.p1.Equip[2], c.s.Inventory.Get(8))
	}
	// A que entra e de outro slot: a origem comparava com o slot do inventario
	// e recusava quase tudo; aqui compara com o slot da pran.
	c.s.Inventory.Slots[8] = model.Item{Index: itemPranSlot3}
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypeInventory, 8)
	if c.p1.Equip[2].Index != itemPranArma || c.s.Inventory.Get(8).Index != itemPranSlot3 {
		t.Errorf("peca de outro slot nao pode entrar: equip=%+v", c.p1.Equip[2])
	}
}

func TestPecaDaPranRecusadaPorSlotNivelEClasse(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	casos := []struct {
		nome       string
		invSlot    uint16
		pranSlot   uint16
		deveEntrar bool
	}{
		{"slot errado", 5, 2, false},
		{"nivel alto", 3, 2, false},
		{"classe de outra forma", 4, 2, false},
		{"peca certa", 2, 2, true},
	}
	for _, k := range casos {
		c2 := cenarioItensPran(t)
		c2.invoca(t)
		c2.move(t, store.SlotTypeInventory, k.invSlot, store.SlotTypePranEquip, k.pranSlot)
		entrou := !c2.p1.Equip[k.pranSlot].IsEmpty()
		if entrou != k.deveEntrar {
			t.Errorf("%s: entrou=%v, queria %v", k.nome, entrou, k.deveEntrar)
		}
	}
	// Peca com o nivel exatamente igual ao da pran + 1 passa (nivel 7, pran 6).
	if c.p1.Level != 6 {
		t.Fatal("o cenario assume a pran no nivel 6")
	}
}

func TestEquipamentoDaPranExigePranInvocada(t *testing.T) {
	c := cenarioItensPran(t) // nada invocado
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, 2)
	if !c.p1.Equip[2].IsEmpty() || c.s.Inventory.Get(2).Index != itemPranArma {
		t.Error("sem pran invocada nada se move")
	}
	c.move(t, store.SlotTypeInventory, 7, store.SlotTypePranInv, 0)
	if !c.p1.Inventory[0].IsEmpty() {
		t.Error("a bolsa da pran tambem exige a pran invocada")
	}
}

func TestSlotsDeEquipamentoDaPranSao1a5(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.s.Inventory.Slots[2] = model.Item{Index: itemPranArma}
	for _, slot := range []uint16{0, 6, 15} {
		c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, slot)
	}
	if c.s.Inventory.Get(2).Index != itemPranArma {
		t.Error("slot fora de 1..5 nao aceita peca")
	}
}

func TestBolsaDaPranExigeABolsaQueCobreOSlot(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 7, store.SlotTypePranInv, 3)
	if c.p1.Inventory[3].Index != itemPranPilha {
		t.Fatalf("slot 3 tem bolsa 40: %+v", c.p1.Inventory[3])
	}
	c.s.Inventory.Slots[10] = model.Item{Index: itemPranPilha, Refi: 2}
	c.move(t, store.SlotTypeInventory, 10, store.SlotTypePranInv, 25)
	if !c.p1.Inventory[25].IsEmpty() {
		t.Error("slot 25 e da bolsa 41, que nao existe")
	}
	c.p1.Inventory[41] = model.Item{Index: itemPranBolsa}
	c.move(t, store.SlotTypeInventory, 10, store.SlotTypePranInv, 25)
	if c.p1.Inventory[25].Index != itemPranPilha {
		t.Error("com a segunda bolsa o slot 25 aceita")
	}
	c.s.Inventory.Slots[10] = model.Item{Index: itemPranPilha, Refi: 2}
	c.move(t, store.SlotTypeInventory, 10, store.SlotTypePranInv, 40)
	if c.p1.Inventory[40].Index != itemPranBolsa || c.s.Inventory.Get(10).Refi != 2 {
		t.Error("os slots 40 e 41 sao as bolsas e nao se mexem")
	}
}

func TestItemPresoNaoVaiParaABolsaDaPranNemVoltaPorCima(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 6, store.SlotTypePranInv, 0)
	if !c.p1.Inventory[0].IsEmpty() {
		t.Error("item preso nao entra na bolsa da pran")
	}
	// Volta: o item da bolsa vai trocar com um preso do inventario.
	c.p1.Inventory[1] = model.Item{Index: itemPranPilha, Refi: 1}
	c.move(t, store.SlotTypePranInv, 1, store.SlotTypeInventory, 6)
	if c.s.Inventory.Get(6).Index != itemPranPreso {
		t.Error("o item preso do inventario nao pode ser trocado para a bolsa")
	}
}

func TestEquipamentoDaPranSoTrocaComOInventario(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypePranEquip, 2)
	c.p1.Inventory[0] = model.Item{Index: itemPranArma}
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypePranInv, 0)
	c.move(t, store.SlotTypePranInv, 0, store.SlotTypePranEquip, 2)
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypePranEquip, 3)
	if c.p1.Equip[2].Index != itemPranArma || c.p1.Inventory[0].Index != itemPranArma || !c.p1.Equip[3].IsEmpty() {
		t.Errorf("equip2=%+v inv0=%+v equip3=%+v", c.p1.Equip[2], c.p1.Inventory[0], c.p1.Equip[3])
	}
	c.s.OpenedOption = NPCOptionStorage
	c.s.OpenedNPC = 1
	c.s.Storage.Slots = make([]model.Item, store.StorageSlots)
	c.move(t, store.SlotTypePranEquip, 2, store.SlotTypeStorage, 84)
	if c.p1.Equip[2].Index != itemPranArma {
		t.Error("peca da pran nao vai para o armazem")
	}
}

func TestReorganizarABolsaDaPranEntreSlots(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.p1.Inventory[0] = model.Item{Index: itemPranSlot3}
	c.move(t, store.SlotTypePranInv, 0, store.SlotTypePranInv, 5)
	if !c.p1.Inventory[0].IsEmpty() || c.p1.Inventory[5].Index != itemPranSlot3 {
		t.Errorf("inv0=%+v inv5=%+v", c.p1.Inventory[0], c.p1.Inventory[5])
	}
}

func TestApagarItemDaBolsaDaPran(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.p1.Inventory[4] = model.Item{Index: itemPranSlot3}
	if err := c.h.deleteItem(context.Background(), c.e.conn, c.s,
		asPacket(t, deleteItemPacket(store.SlotTypePranInv, 4))); err != nil {
		t.Fatal(err)
	}
	if !c.p1.Inventory[4].IsEmpty() {
		t.Error("item deveria ter sido apagado")
	}
	// Slot fora da bolsa (42) e ignorado.
	if err := c.h.deleteItem(context.Background(), c.e.conn, c.s,
		asPacket(t, deleteItemPacket(store.SlotTypePranInv, 42))); err != nil {
		t.Fatal(err)
	}
}

func TestSepararPilhaNaBolsaDaPran(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	c.p1.Inventory[2] = model.Item{Index: itemPranPilha, Refi: 10}
	if err := c.h.ungroupItem(context.Background(), c.e.conn, c.s,
		asPacket(t, ungroupPacket(2, 4, store.SlotTypePranInv))); err != nil {
		t.Fatal(err)
	}
	if c.p1.Inventory[2].Refi != 6 || c.p1.Inventory[0].Refi != 4 {
		t.Errorf("origem=%d novo=%d, queria 6 e 4", c.p1.Inventory[2].Refi, c.p1.Inventory[0].Refi)
	}
	if c.s.Inventory.Get(7).Refi != 10 {
		t.Error("a pilha do inventario do dono nao deve mudar")
	}
}

func TestSepararPilhaDaPranSemEspacoAvisa(t *testing.T) {
	c := cenarioItensPran(t)
	c.invoca(t)
	for i := 0; i < 20; i++ {
		c.p1.Inventory[i] = model.Item{Index: itemPranPilha, Refi: 10}
	}
	c.e.drena(20 * time.Millisecond)
	if err := c.h.ungroupItem(context.Background(), c.e.conn, c.s,
		asPacket(t, ungroupPacket(2, 4, store.SlotTypePranInv))); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, c.e); got != "Inventario cheio." {
		t.Errorf("mensagem = %q", got)
	}
	if c.p1.Inventory[2].Refi != 10 {
		t.Error("nada deve ser separado quando nao ha espaco")
	}
}
