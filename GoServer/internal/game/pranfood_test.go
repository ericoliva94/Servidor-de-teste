package game

import (
	"testing"
	"time"

	"aika/internal/pran"
)

func TestPranFoodDecaysAndWarns(t *testing.T) {
	h, s, e, p := pranAtiva(t, func(p *pran.Pran) { p.Food = 30 })
	_ = s
	e.drena(10 * time.Millisecond)
	if n := h.PranFoodTick(); n != 1 {
		t.Fatalf("prans afetadas = %d", n)
	}
	if p.Food != 27 {
		t.Errorf("comida = %d, queria 27", p.Food)
	}
	// 27 ainda nao e fome.
	e.drena(10 * time.Millisecond)
	h.PranFoodTick() // 24
	if p.Food != 24 {
		t.Fatalf("comida = %d, queria 24", p.Food)
	}
	if got := mensagemDe(t, e); got != "Pran está com fome." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestPranFoodFloorsAtZeroAndSkipsEmpty(t *testing.T) {
	h, _, _, p := pranAtiva(t, func(p *pran.Pran) { p.Food = 2 })
	h.PranFoodTick()
	if p.Food != 0 {
		t.Errorf("comida = %d, queria 0 (piso)", p.Food)
	}
	if n := h.PranFoodTick(); n != 0 {
		t.Errorf("pran sem comida nao conta: %d", n)
	}
}
