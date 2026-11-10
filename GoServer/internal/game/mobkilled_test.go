package game

import (
	"testing"

	"aika/internal/mobdata"
)

func TestMobExpForLevelBands(t *testing.T) {
	casos := []struct {
		mob, jog uint16
		want     int64
	}{
		{10, 20, 1},    // -10: cinza
		{12, 20, 1},    // -8: cinza
		{13, 20, 500},  // -7: azul
		{17, 20, 500},  // -3: azul
		{18, 20, 1000}, // -2: amarelo
		{22, 20, 1000}, // +2: amarelo
		{23, 20, 1500}, // +3: laranja
		{25, 20, 1500}, // +5: laranja
		{26, 20, 200},  // +6: roxo
		{90, 20, 200},
	}
	for _, c := range casos {
		if got := mobExpForLevel(1000, c.mob, c.jog); got != c.want {
			t.Errorf("mob %d x jogador %d: %d, queria %d", c.mob, c.jog, got, c.want)
		}
	}
}

func TestMobExpRoundsHalfToEven(t *testing.T) {
	// Round do Delphi: 5 * 0,5 = 2,5 -> 2 (par); 7 * 0,5 = 3,5 -> 4.
	if got := mobExpForLevel(5, 15, 20); got != 2 {
		t.Errorf("5 * 0,5 = %d, queria 2", got)
	}
	if got := mobExpForLevel(7, 15, 20); got != 4 {
		t.Errorf("7 * 0,5 = %d, queria 4", got)
	}
}

func TestRelicExpBonus(t *testing.T) {
	if got := relicExpBonus(1000, 10); got != 1100 {
		t.Errorf("1000 +10%% = %d", got)
	}
	// div 100 antes de multiplicar, como na origem: 150 -> 1 * 10 = 10.
	if got := relicExpBonus(150, 10); got != 160 {
		t.Errorf("150 +10%% = %d, queria 160", got)
	}
	if got := relicExpBonus(1000, 0); got != 1000 {
		t.Errorf("sem relíquia = %d", got)
	}
}

func partyOnMob(t *testing.T, n int, exp, item uint8) (*Handler, []*Session, *Party, *Mob, *mobdata.Species) {
	t.Helper()
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 1, 1, 1000, 1000)))
	h.Parties = NewPartyRegistry()
	var ss []*Session
	for i := 0; i < n; i++ {
		e := novoEspiao(t)
		s := lutador(string(rune('A'+i)), 1000, 1000)
		s.Avatar.Level = 50
		h.Chan.Join(e.conn, s)
		ss = append(ss, s)
	}
	g, err := h.Parties.Create(ss[0].ClientID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ss[1:] {
		if err := h.Parties.Add(g, s.ClientID); err != nil {
			t.Fatal(err)
		}
	}
	g.ExpAllocate, g.ItemAllocate = exp, item
	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	return h, ss, g, m, &mobdata.Species{Exp: 1000, Level: 50}
}

func TestPartyExpEqualSplit(t *testing.T) {
	h, ss, _, m, sp := partyOnMob(t, 2, ExpEqual, ItemLeader)
	m.Position = ss[0].Position
	h.onMobDeath(nil, ss[0], m, sp)
	for _, s := range ss {
		if s.Avatar.Exp != 500 {
			t.Errorf("%s: exp %d, queria 500", s.Avatar.Name, s.Avatar.Exp)
		}
	}
}

func TestPartyExpIndividualOnlyFirstAttacker(t *testing.T) {
	h, ss, _, m, sp := partyOnMob(t, 2, ExpIndividual, ItemLeader)
	m.Position = ss[0].Position
	m.FirstAttackerID = ss[1].ClientID
	h.onMobDeath(nil, ss[0], m, sp)
	if ss[0].Avatar.Exp != 0 || ss[1].Avatar.Exp != 1000 {
		t.Errorf("exp: %d e %d, queria 0 e 1000", ss[0].Avatar.Exp, ss[1].Avatar.Exp)
	}
}

func TestPartyMemberFarAwayGetsNothing(t *testing.T) {
	h, ss, _, m, sp := partyOnMob(t, 2, ExpEqual, ItemLeader)
	m.Position = ss[0].Position
	ss[1].Position.X += 200
	h.onMobDeath(nil, ss[0], m, sp)
	if ss[1].Avatar.Exp != 0 {
		t.Errorf("membro longe levou %d", ss[1].Avatar.Exp)
	}
}

func TestPartyItemInOrderRotates(t *testing.T) {
	h, ss, g, _, _ := partyOnMob(t, 3, ExpEqual, ItemInOrder)
	m := &Mob{}
	for i, want := range []int{0, 1, 2, 0} {
		if got := h.partyItemRecipient(g, m); got != ss[want].ClientID {
			t.Fatalf("morte %d: recebeu %d, queria membro %d", i, got, want)
		}
	}
}

func TestPartyItemLeaderAndIndividual(t *testing.T) {
	h, ss, g, _, _ := partyOnMob(t, 2, ExpEqual, ItemLeader)
	if got := h.partyItemRecipient(g, &Mob{}); got != ss[0].ClientID {
		t.Errorf("modo líder: %d", got)
	}
	g.ItemAllocate = ItemIndividual
	if got := h.partyItemRecipient(g, &Mob{FirstAttackerID: ss[1].ClientID}); got != ss[1].ClientID {
		t.Errorf("modo individual: %d", got)
	}
	if got := h.partyItemRecipient(g, &Mob{}); got != 0 {
		t.Errorf("sem primeiro atacante ninguém leva: %d", got)
	}
}
