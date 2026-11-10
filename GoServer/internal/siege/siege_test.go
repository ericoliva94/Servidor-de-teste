package siege

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)

func ally(names ...string) Alliance {
	var a Alliance
	f := []*string{&a.Marshal, &a.Tactician, &a.Judge, &a.Treasurer}
	for i, n := range names {
		*f[i] = n
	}
	return a
}

func TestRegisterAttackerRules(t *testing.T) {
	var w War
	w.SetCerco(Cerco{Defenders: ally("Defesa")})
	if _, err := w.RegisterAttacker(ally("Defesa")); !errors.Is(err, ErrIsDefender) {
		t.Errorf("defesa cadastrando: %v", err)
	}
	slot, err := w.RegisterAttacker(ally("Alfa", "Beta"))
	if err != nil {
		t.Fatal(err)
	}
	if slot != 3 {
		t.Errorf("slot = %d: a origem guarda o ULTIMO slot livre (3)", slot)
	}
	if _, err := w.RegisterAttacker(ally("alfa")); !errors.Is(err, ErrRegistered) {
		t.Errorf("repetida: %v", err)
	}
	for _, n := range []string{"B", "C", "D"} {
		if _, err := w.RegisterAttacker(ally(n)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	if _, err := w.RegisterAttacker(ally("E")); !errors.Is(err, ErrNoSlot) {
		t.Errorf("quinta alianca: %v", err)
	}
}

func TestRegisterDuringWarRefused(t *testing.T) {
	var w War
	w.Start(t0)
	if _, err := w.RegisterAttacker(ally("Alfa")); !errors.Is(err, ErrWarRunning) {
		t.Errorf("com guerra: %v", err)
	}
}

func TestEntryPointsByRole(t *testing.T) {
	var w War
	w.SetCerco(Cerco{
		Defenders: ally("Def", "Estr", "Juiz", "Tes"),
		Attackers: [MaxAttackers]Alliance{ally("Atk0", "Atk0b"), {}, ally("Atk2"), {}},
	})
	casos := []struct {
		guild string
		want  Entry
	}{
		{"Atk0", AttackEntry[0]}, {"Atk0b", AttackEntry[0]}, {"Atk2", AttackEntry[2]},
		{"Def", DefenseMarshal}, {"Estr", DefenseTactician}, {"Juiz", DefenseJudge}, {"Tes", DefenseTreasurer},
	}
	for _, c := range casos {
		got, err := w.Entry(c.guild)
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, queria %+v", c.guild, got, err, c.want)
		}
	}
	if _, err := w.Entry("Estranha"); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("sem cadastro: %v", err)
	}
	if w.Registered("") {
		t.Error("nome vazio nao e guilda cadastrada")
	}
}

func TestStatusFollowsOrbsAndSeal(t *testing.T) {
	var w War
	if !w.Start(t0) || w.Start(t0) {
		t.Fatal("Start deve abrir uma vez so")
	}
	if w.Status() != StatusRunning {
		t.Fatalf("status = %v", w.Status())
	}
	if w.BeginSeal(9, ally("A"), t0) {
		t.Error("selo exige as tres orbes")
	}
	for i := 0; i < Orbs; i++ {
		w.HoldOrb(i, uint16(10+i))
	}
	if w.Status() != StatusOrbsHeld || w.OrbsHeld() != 3 {
		t.Fatalf("status = %v orbes = %d", w.Status(), w.OrbsHeld())
	}
	if !w.BeginSeal(9, ally("A"), t0) || w.Status() != StatusSealing {
		t.Fatalf("selo nao comecou: %v", w.Status())
	}
	// Perder uma orbe derruba o selo.
	if got := w.ReleaseOrb(1); got != 11 {
		t.Errorf("saiu %d, queria 11", got)
	}
	if w.Status() != StatusRunning || w.SealHolder() != 0 {
		t.Errorf("status = %v selo = %d", w.Status(), w.SealHolder())
	}
}

func TestSealHeldLongEnoughWins(t *testing.T) {
	var w War
	w.SetCerco(Cerco{Defenders: ally("Def"), Attackers: [MaxAttackers]Alliance{ally("Atk", "Atk2")}})
	w.Start(t0)
	for i := 0; i < Orbs; i++ {
		w.HoldOrb(i, uint16(10+i))
	}
	w.BeginSeal(9, ally("Atk", "Atk2"), t0)
	if r := w.SealRemaining(t0.Add(30 * time.Second)); r != 90 {
		t.Errorf("restam %d, queria 90", r)
	}
	if o := w.Tick(t0.Add(119 * time.Second)); o.Kind != NoOutcome {
		t.Fatalf("antes de 120 s: %v", o.Kind)
	}
	o := w.Tick(t0.Add(120 * time.Second))
	if o.Kind != AttackersWin || !o.Winner.Has("Atk2") {
		t.Fatalf("resultado = %+v", o)
	}
	if w.InProgress() {
		t.Error("a guerra deveria estar fechada")
	}
	c := w.Cerco()
	if !c.Defenders.Has("Atk") || !c.Attackers[0].Empty() {
		t.Errorf("cerco depois da vitoria: %+v", c)
	}
}

func TestWarTimesOutWithoutWinner(t *testing.T) {
	var w War
	w.SetCerco(Cerco{Defenders: ally("Def"), Attackers: [MaxAttackers]Alliance{ally("Atk")}})
	w.Start(t0)
	if o := w.Tick(t0.Add(59 * time.Minute)); o.Kind != NoOutcome {
		t.Fatal("ainda dentro dos 60 min")
	}
	if o := w.Tick(t0.Add(60 * time.Minute)); o.Kind != NoWinner {
		t.Fatalf("resultado = %v", o.Kind)
	}
	c := w.Cerco()
	if !c.Defenders.Has("Def") || !c.Attackers[0].Empty() {
		t.Errorf("a defesa fica e os atacantes saem: %+v", c)
	}
	if o := w.Tick(t0.Add(2 * time.Hour)); o.Kind != NoOutcome {
		t.Error("guerra fechada nao acaba de novo")
	}
}
