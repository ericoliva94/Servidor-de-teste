package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/pran"
	"aika/internal/store"
)

// cenarioInvocar monta um dono com duas prans (itens 900 e 901) no inventario e
// um vizinho que o ve.
type cenarioInvocar struct {
	h      *Handler
	s      *Session
	e      *espiao
	viz    *Session
	eViz   *espiao
	p1, p2 *pran.Pran
}

func novoCenarioInvocar(t *testing.T) *cenarioInvocar {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(7000, gamedata.ItemFromList{ItemType: 10})
	tab.poeItem(7001, gamedata.ItemFromList{ItemType: 10})
	tab.poeItem(5, gamedata.ItemFromList{ItemType: 6})
	h.SkillTable = novaTabelaDeSkills()
	h.PranIDs = pran.NewIDs()
	c := &cenarioInvocar{h: h}
	c.s, c.e = entraJogadorSimples(t, h)
	c.s.Avatar.Name = "Dono"
	c.s.Equip.Slots = make([]model.Item, store.EquipSlots)
	c.s.Status = Playing
	c.p1 = pranDeTeste() // item 900, classe 62
	c.p1.Level = 6
	c.p2 = pranDeTeste()
	c.p2.DBID, c.p2.ItemID, c.p2.Name, c.p2.Level = 6, 901, "Lulu", 8
	c.s.Prans = [2]*pran.Pran{c.p1, c.p2}
	c.s.SpawnedPran = -1
	c.s.Inventory.Slots[0] = model.Item{Index: 7000, Identific: 900}
	c.s.Inventory.Slots[1] = model.Item{Index: 7001, Identific: 901}
	c.viz, c.eViz = entraJogadorSimples(t, h)
	c.viz.Status = Playing
	c.s.Base.VisiblePlayers.Add(c.viz.ClientID)
	c.viz.Base.VisiblePlayers.Add(c.s.ClientID)
	return c
}

func (c *cenarioInvocar) move(t *testing.T, st, ss, dt, ds uint16) {
	t.Helper()
	if err := c.h.moveItem(context.Background(), c.e.conn, c.s,
		asPacket(t, moveItemPacket(st, ss, dt, ds))); err != nil {
		t.Fatal(err)
	}
}

func TestEquiparOItemDaPranInvocaEMostraAosVizinhos(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	if c.s.Equip.Get(10).Identific != 900 || c.s.SpawnedPran != 0 || !c.p1.Spawned {
		t.Fatalf("equip=%+v spawned=%d", c.s.Equip.Get(10), c.s.SpawnedPran)
	}
	spawn := decodifica[packets.SendCreatePranPacket](t, proximoDe(t, c.e, CodePranSpawn))
	if spawn.PranClientID != pran.FirstClientID || netsrv.CString(spawn.Name[:]) != "Mimi" {
		t.Errorf("spawn do dono = %+v", spawn)
	}
	proximoDe(t, c.e, CodePranToWorld)
	viz := decodifica[packets.SendCreatePranPacket](t, proximoDe(t, c.eViz, CodePranSpawn))
	if viz.PranClientID != spawn.PranClientID || viz.Position != spawn.Position {
		t.Errorf("o vizinho viu %+v, o dono %+v", viz.Position, spawn.Position)
	}
}

func TestGuardarAPranApagaDaTelaELiberaOId(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	c.e.drena(10 * time.Millisecond)
	c.eViz.drena(10 * time.Millisecond)
	c.move(t, store.SlotTypeEquip, 10, store.SlotTypeInventory, 0)
	if c.s.SpawnedPran != -1 || c.p1.Spawned || c.s.PranClientID != 0 {
		t.Errorf("spawned=%d id=%d", c.s.SpawnedPran, c.s.PranClientID)
	}
	rem := decodifica[packets.SendRemoveMobPacket](t, proximoDe(t, c.eViz, CodeRemoveMob))
	if rem.Index != pran.FirstClientID {
		t.Errorf("remocao do id %d", rem.Index)
	}
	if _, ok := c.h.PranIDs.Owner(pran.FirstClientID); ok {
		t.Error("o id deveria voltar ao alocador")
	}
	if !c.s.Equip.Get(10).IsEmpty() || c.s.Inventory.Get(0).Identific != 900 {
		t.Error("o item deveria voltar ao inventario")
	}
}

func TestTrocarUmaPranPelaOutra(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	c.move(t, store.SlotTypeInventory, 1, store.SlotTypeEquip, 10)
	if c.s.SpawnedPran != 1 || c.p1.Spawned || !c.p2.Spawned {
		t.Errorf("spawned=%d p1=%v p2=%v", c.s.SpawnedPran, c.p1.Spawned, c.p2.Spawned)
	}
	if c.s.Inventory.Get(1).Identific != 900 {
		t.Error("a primeira pran deveria ter ido para o inventario")
	}
	c.move(t, store.SlotTypeEquip, 10, store.SlotTypeInventory, 1)
	if c.s.SpawnedPran != 0 || !c.p1.Spawned || c.p2.Spawned {
		t.Errorf("spawned=%d p1=%v p2=%v", c.s.SpawnedPran, c.p1.Spawned, c.p2.Spawned)
	}
}

func TestPranEntraNosAtributosAoInvocarESaiAoGuardar(t *testing.T) {
	c := novoCenarioInvocar(t)
	sk := c.h.SkillTable.(*tabelaDeSkills)
	sk.m[502] = gamedata.T_SkillData{Index: 193}
	c.p1.Skills = [pran.SkillSlots]pran.Skill{{Index: 500, Level: 3}}
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	if got := c.s.Avatar.Ability.get(efPranDamage1); got != 11 {
		t.Fatalf("dano da pran = %d, queria 11", got)
	}
	c.move(t, store.SlotTypeEquip, 10, store.SlotTypeInventory, 0)
	if got := c.s.Avatar.Ability.get(efPranDamage1); got != 0 {
		t.Errorf("sobrou %d depois de guardar", got)
	}
}

func TestPranFadaApareceComoEfeitoNaoComoCriatura(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.p1.Level, c.p1.Class = 2, 61
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	if c.s.PranClientID != 0 {
		t.Error("fada nao ganha id")
	}
	ef := decodifica[packets.SendClientIndexPacket](t, proximoDe(t, c.e, CodeSendEffect))
	if ef.Effect != 2 {
		t.Errorf("efeito = %d, queria 2 (fogo)", ef.Effect)
	}
	c.e.drena(10 * time.Millisecond)
	c.move(t, store.SlotTypeEquip, 10, store.SlotTypeInventory, 0)
	ef = decodifica[packets.SendClientIndexPacket](t, proximoDe(t, c.e, CodeSendEffect))
	if ef.Effect != 0 {
		t.Errorf("ao guardar o efeito deveria zerar: %d", ef.Effect)
	}
}

func TestPranNaoPrecisaDaClasseDoDono(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.s.Avatar.ClassInfo = 41 // mago: a classe do item (0) cai em guerreiro
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	if c.s.Equip.Get(10).Identific != 900 {
		t.Error("a pran nao tem classe; qualquer dono a equipa")
	}
}

func TestItemDePranDeOutraContaNaoInvocaNada(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.s.Inventory.Slots[2] = model.Item{Index: 7000, Identific: 999}
	c.move(t, store.SlotTypeInventory, 2, store.SlotTypeEquip, 10)
	if c.s.SpawnedPran != -1 || c.p1.Spawned || c.p2.Spawned {
		t.Error("item sem pran da conta nao invoca nada")
	}
}

func TestVizinhoQueChegaDepoisVeAPranEAoSairElaSome(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	c.eViz.drena(10 * time.Millisecond)
	c.h.despawnPair(c.s, c.viz)
	proximoDe(t, c.eViz, CodeRemoveMob)
	c.eViz.drena(10 * time.Millisecond)
	c.h.spawnPair(c.s, c.viz)
	proximoDe(t, c.eViz, CodeCreateMob) // o proprio dono (mesmo codigo $349)
	got := decodifica[packets.SendCreatePranPacket](t, proximoDe(t, c.eViz, CodePranSpawn))
	if got.PranClientID != pran.FirstClientID {
		t.Errorf("pran vista = %d", got.PranClientID)
	}
	c.eViz.drena(10 * time.Millisecond)
	c.h.removeFromEveryone(c.s)
	ids := map[uint32]bool{}
	for i := 0; i < 2; i++ {
		p := proximoDe(t, c.eViz, CodeRemoveMob)
		ids[decodifica[packets.SendRemoveMobPacket](t, p).Index] = true
	}
	if !ids[uint32(c.s.ClientID)] || !ids[pran.FirstClientID] {
		t.Errorf("remocoes = %v, queria o dono e a pran", ids)
	}
}

func TestCentralDePranSoAceitaPran(t *testing.T) {
	c := novoCenarioInvocar(t)
	c.s.OpenedOption = NPCOptionStorage
	c.s.OpenedNPC = 1
	c.s.Storage.Slots = make([]model.Item, store.StorageSlots)
	c.s.Inventory.Slots[3] = model.Item{Index: 5}
	c.move(t, store.SlotTypeInventory, 3, store.SlotTypeStorage, 84)
	if !c.s.Storage.Get(84).IsEmpty() {
		t.Error("item comum nao entra na central de pran")
	}
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeStorage, 84)
	if c.s.Storage.Get(84).Identific != 900 {
		t.Error("a pran entra na central")
	}
	c.move(t, store.SlotTypeStorage, 84, store.SlotTypeInventory, 0)
	if c.s.Inventory.Get(0).Identific != 900 {
		t.Error("a pran volta da central")
	}
}
