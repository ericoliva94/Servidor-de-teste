package game

import (
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/skillfn"
)

func expiryHandler(t *testing.T) (*Handler, *Session, *espiao, *time.Time) {
	t.Helper()
	h, stab, agora := handlerComSkills(t)
	h.Durations = skillfn.BuffSeconds{T: stab}
	tab := h.Data.(*tabelaDeItens)
	tab.poeItem(2000, gamedata.ItemFromList{ItemType: 20, Expires: 1, Duration: 1, Name: [64]byte{'P', 'o', 'c', 'a', 'o'}})
	tab.poeItem(2001, gamedata.ItemFromList{ItemType: 9, Expires: 1})               // montaria
	tab.poeItem(2002, gamedata.ItemFromList{ItemType: 30, Classe: 101, Expires: 1}) // roupa de pran
	tab.poeItem(2003, gamedata.ItemFromList{ItemType: 20})                          // sem prazo
	tab.poeItem(2004, gamedata.ItemFromList{ItemType: itemTypeExpBuff, Expires: 1, UseEffect: 77})
	s := lutador("Alfa", 1000, 1000)
	e := novoEspiao(t)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.drena(10 * time.Millisecond)
	return h, s, e, agora
}

func comPrazo(h *Handler, idx uint16, quando time.Time) model.Item {
	it := model.Item{Index: idx, Min: 10, Max: 10}
	linha := h.data().Item(int(idx))
	it.SetExpire(linha.ItemType, baseDatetime, quando)
	return it
}

func TestExpiredItemIsRemovedWithMessage(t *testing.T) {
	h, s, e, agora := expiryHandler(t)
	s.Inventory.Slots[3] = comPrazo(h, 2000, agora.Add(-time.Hour))
	if n := h.ExpireItemsTick(); n != 1 {
		t.Fatalf("venceram %d, queria 1", n)
	}
	if !s.Inventory.Slots[3].IsEmpty() {
		t.Error("o item vencido deveria ser apagado")
	}
	if got := mensagemDe(t, e); got != "O item [Pocao] expirou." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestItemWithinDeadlineAndSealedAndNoExpiryStay(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	s.Inventory.Slots[3] = comPrazo(h, 2000, agora.Add(48*time.Hour)) // ainda vale
	s.Inventory.Slots[4] = model.Item{Index: 2000}                    // selado (Refi e Time zerados)
	s.Inventory.Slots[5] = comPrazo(h, 2003, agora.Add(-time.Hour))   // a tabela nao marca prazo
	if n := h.ExpireItemsTick(); n != 0 {
		t.Errorf("venceram %d, queria 0", n)
	}
	for _, i := range []int{3, 4, 5} {
		if s.Inventory.Slots[i].IsEmpty() {
			t.Errorf("slot %d nao deveria ser apagado", i)
		}
	}
}

func TestMountStaysAndIsMarkedExpired(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	s.Inventory.Slots[2] = comPrazo(h, 2001, agora.Add(-time.Hour))
	s.Inventory.Slots[2].Time = 1 // um dia desde a base: ja passou
	if n := h.ExpireItemsTick(); n != 1 {
		t.Fatalf("venceram %d, queria 1", n)
	}
	if s.Inventory.Slots[2].IsEmpty() || s.Inventory.Slots[2].Time != 0xFFFF {
		t.Errorf("montaria: %+v (deveria ficar com Time 0xFFFF)", s.Inventory.Slots[2])
	}
	// E nao vence de novo na passada seguinte.
	if n := h.ExpireItemsTick(); n != 0 {
		t.Errorf("segunda passada: %d, queria 0", n)
	}
}

func TestPranOutfitIsMarkedOnceOnly(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	s.Inventory.Slots[2] = comPrazo(h, 2002, agora.Add(-time.Hour))
	if n := h.ExpireItemsTick(); n != 1 {
		t.Fatalf("primeira passada: %d", n)
	}
	it := s.Inventory.Slots[2]
	if it.IsEmpty() || it.Min != 0xFF || it.Max != 0xFF {
		t.Errorf("roupa: %+v", it)
	}
	if n := h.ExpireItemsTick(); n != 0 {
		t.Errorf("a roupa ja tratada nao repete o aviso: %d", n)
	}
}

func TestExpiredEquipAndStorage(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	eq := comPrazo(h, 2000, agora.Add(-time.Hour))
	s.Equip.Slots[2] = eq
	s.Storage.Slots = make([]model.Item, 10)
	s.Storage.Slots[1] = comPrazo(h, 2000, agora.Add(-time.Hour))
	if n := h.ExpireItemsTick(); n != 2 {
		t.Fatalf("venceram %d, queria 2", n)
	}
	if !s.Equip.Slots[2].IsEmpty() || !s.Storage.Slots[1].IsEmpty() {
		t.Error("equipamento e armazem deveriam ser limpos")
	}
}

func TestExpiredExpBuffItemDropsTheBuff(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	h.SkillTable.(*tabelaDeSkills).poe(77, gamedata.T_SkillData{Duration: 600})
	h.addBuff(s, 77)
	s.Inventory.Slots[3] = comPrazo(h, 2004, agora.Add(-time.Hour))
	h.ExpireItemsTick()
	if s.Base.Buffs.Has(77) {
		t.Error("o buff da pocao deveria sair junto com o item")
	}
}

func TestSetItemDurationOnPutItem(t *testing.T) {
	h, s, _, agora := expiryHandler(t)
	if !h.putItem(h.connOf(s), s, model.Item{Index: 2000, Min: 10, Max: 10}) {
		t.Fatal("putItem recusou")
	}
	var got model.Item
	for _, it := range s.Inventory.Slots {
		if it.Index == 2000 {
			got = it
		}
	}
	if got.IsSealed() {
		t.Fatal("o item novo deveria ganhar prazo")
	}
	fim := got.Expire(20, baseDatetime)
	want := agora.Add(3 * time.Hour) // Duration 1 + 2 horas
	if d := fim.Sub(want); d < -5*time.Minute || d > 5*time.Minute {
		t.Errorf("fim = %v, queria perto de %v", fim, want)
	}
}
