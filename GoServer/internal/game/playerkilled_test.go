package game

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

func pvpPair(t *testing.T) (*Handler, *Session, *Session) {
	t.Helper()
	h, a, _, b, _ := prepArea(t, gamedata.T_SkillData{})
	h.MapObjs = NewMapObjects()
	h.Parties = NewPartyRegistry()
	a.Avatar.Level, b.Avatar.Level = 30, 30
	return h, a, b
}

func TestPlayerKilledGivesPvPAndHonor(t *testing.T) {
	h, a, b := pvpPair(t)
	h.playerKilled(a, b)
	if a.Avatar.KillPoint != 1 || a.Avatar.Honor != honorPerKill {
		t.Errorf("PvP=%d honra=%d, queria 1 e %d", a.Avatar.KillPoint, a.Avatar.Honor, honorPerKill)
	}
	if titleSlot(a, pvpTitleID) < 0 {
		t.Error("o titulo 27 deveria nascer no primeiro abate")
	}
	if b.Avatar.KillPoint != 0 {
		t.Error("a vitima nao ganha PvP")
	}
}

func TestPlayerKilledNeedsLevel25(t *testing.T) {
	h, a, b := pvpPair(t)
	b.Avatar.Level = 24
	h.playerKilled(a, b)
	if a.Avatar.KillPoint != 0 || a.Avatar.Honor != 0 {
		t.Error("alvo abaixo do nivel 25 nao da PvP")
	}
	h, a, b = pvpPair(t)
	a.Avatar.Level = 24
	h.playerKilled(a, b)
	if a.Avatar.KillPoint != 0 {
		t.Error("autor abaixo do nivel 25 nao recebe PvP")
	}
}

func TestPlayerKilledSharesWithNearbyPartyOnly(t *testing.T) {
	h, a, b := pvpPair(t)
	c := lutador("Gama", 1002, 1000)
	d := lutador("Delta", 1500, 1500) // longe
	for _, s := range []*Session{c, d} {
		s.Avatar.Level = 30
		h.Chan.Join(novoEspiao(t).conn, s)
	}
	g, err := h.Parties.Create(a.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	_ = h.Parties.Add(g, c.ClientID)
	_ = h.Parties.Add(g, d.ClientID)
	h.playerKilled(a, b)
	if a.Avatar.KillPoint != 1 || c.Avatar.KillPoint != 1 {
		t.Errorf("autor=%d perto=%d, queria 1 e 1", a.Avatar.KillPoint, c.Avatar.KillPoint)
	}
	if d.Avatar.KillPoint != 0 {
		t.Error("membro longe nao recebe")
	}
}

func TestPlayerKilledDropsEveryRelicNearKiller(t *testing.T) {
	h, a, b := pvpPair(t)
	h.Data.(*tabelaDeItens).poeItem(900, gamedata.ItemFromList{ItemType: 40})
	b.Inventory.Slots[3] = model.Item{Index: 900}
	b.Inventory.Slots[4] = model.Item{Index: 900}
	h.playerKilled(a, b)
	if !b.Inventory.Slots[3].IsEmpty() || !b.Inventory.Slots[4].IsEmpty() {
		t.Error("a vitima deveria soltar as duas reliquias")
	}
	if n := h.MapObjs.Count(); n != 2 {
		t.Fatalf("objetos no chao = %d, queria 2", n)
	}
	for _, o := range h.MapObjs.Near(a.Position, 3) {
		if o.ContentType != ObjectReliquare || o.Face != relicFace || o.NameID != relicNameID {
			t.Errorf("objeto errado: %+v", o)
		}
		if o.Position == a.Position {
			t.Error("a reliquia deveria cair numa celula vizinha, nao em cima do autor")
		}
	}
}

func TestPlayerKilledLastingEffectBlocksPvP(t *testing.T) {
	h, a, b := pvpPair(t)
	h.SkillTable.(*tabelaDeSkills).poe(buffLasting, gamedata.T_SkillData{Index: buffLasting, Duration: 60})
	h.addBuff(b, buffLasting)
	h.playerKilled(a, b)
	if a.Avatar.KillPoint != 0 {
		t.Error("vitima com Efeito Duradouro nao da PvP")
	}
}
