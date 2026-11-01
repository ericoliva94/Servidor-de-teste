package entity

import (
	"testing"
	"time"
)

type fixedCooldowns struct{ ms int }

func (f fixedCooldowns) CooldownMillis(uint16) int { return f.ms }

func TestCooldownReadyWhenNeverUsed(t *testing.T) {
	c := NewCooldowns()
	if !c.Ready(10, time.Now(), fixedCooldowns{5000}) {
		t.Error("skill nunca usada deveria estar pronta")
	}
	if c.Remaining(10, time.Now(), fixedCooldowns{5000}) != 0 {
		t.Error("Remaining deveria ser zero")
	}
}

func TestCooldownBlocksThenReleases(t *testing.T) {
	c := NewCooldowns()
	cd := fixedCooldowns{1000}
	base := time.Now()
	c.Use(10, base)

	if c.Ready(10, base, cd) {
		t.Error("deveria bloquear no instante do uso")
	}
	if c.Ready(10, base.Add(999*time.Millisecond), cd) {
		t.Error("deveria bloquear antes do fim")
	}
	// Comparacao estrita, igual ao "not(EndTime < Now)" da origem.
	if c.Ready(10, base.Add(time.Second), cd) {
		t.Error("no instante exato do fim ainda nao esta pronta")
	}
	if !c.Ready(10, base.Add(time.Second+time.Nanosecond), cd) {
		t.Error("deveria liberar depois do fim")
	}
	if got := c.Remaining(10, base, cd); got != time.Second {
		t.Errorf("Remaining=%v, queria 1s", got)
	}
}

// Recarga zero (skill fora da tabela) nao pode travar a skill para sempre.
func TestCooldownZeroIsAlwaysReady(t *testing.T) {
	c := NewCooldowns()
	base := time.Now()
	c.Use(65535, base)
	if !c.Ready(65535, base.Add(time.Nanosecond), fixedCooldowns{0}) {
		t.Error("recarga zero deveria liberar de imediato")
	}
}

// A origem guardava TTime (hora do dia, sem data): na virada do dia a recarga
// era calculada contra um valor menor e liberava antes da hora.
func TestCooldownSurvivesMidnight(t *testing.T) {
	c := NewCooldowns()
	cd := fixedCooldowns{60_000} // um minuto
	use := time.Date(2026, 11, 1, 23, 59, 30, 0, time.UTC)
	c.Use(7, use)

	if c.Ready(7, use.Add(30*time.Second), cd) { // 00:00:00 do dia seguinte
		t.Error("liberou na virada do dia: o instante precisa ter data")
	}
	if !c.Ready(7, use.Add(61*time.Second), cd) {
		t.Error("deveria liberar um minuto depois")
	}
}

func TestCooldownClearAndCount(t *testing.T) {
	c := NewCooldowns()
	now := time.Now()
	c.Use(1, now)
	c.Use(2, now)
	if c.Count() != 2 {
		t.Errorf("Count=%d", c.Count())
	}
	if _, ok := c.LastUse(1); !ok {
		t.Error("LastUse deveria achar a skill 1")
	}
	c.Clear()
	if c.Count() != 0 {
		t.Errorf("Clear deixou %d", c.Count())
	}
}

func TestCooldownNilSafe(t *testing.T) {
	var c *Cooldowns
	cd := fixedCooldowns{1000}
	if !c.Ready(1, time.Now(), cd) {
		t.Error("*Cooldowns nil deveria responder pronto")
	}
	if c.Remaining(1, time.Now(), cd) != 0 || c.Count() != 0 {
		t.Error("operacoes em nil deveriam ser inofensivas")
	}
	if _, ok := c.LastUse(1); ok {
		t.Error("LastUse em nil deveria ser false")
	}
	c.Use(1, time.Now())
	c.Clear()
}

// Cooldowns com valor zero (sem NewCooldowns) tem que funcionar.
func TestCooldownZeroValueUsable(t *testing.T) {
	var c Cooldowns
	now := time.Now()
	c.Use(3, now)
	if c.Ready(3, now, fixedCooldowns{500}) {
		t.Error("deveria bloquear")
	}
}
