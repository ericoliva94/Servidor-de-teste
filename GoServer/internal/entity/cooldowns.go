package entity

import "time"

// CooldownMillis da o tempo de recarga de uma skill em milissegundos. Na
// origem era SkillData[Index].Cooldown, com SkillData sendo um array global
// ARRAY [0..11998]; aqui a tabela e injetada e responde 0 para indice
// invalido, em vez de ler memoria fora do array.
type CooldownMillis interface {
	CooldownMillis(skill uint16) int
}

// Cooldowns guarda quando cada skill foi usada por ultimo.
//
// Equivale ao TDictionary<WORD, TTime> _cooldown do TBaseMob. A origem
// guardava TTime (so a hora do dia, sem data): uma skill usada as 23h59 ficava
// com a recarga calculada contra um valor menor na virada do dia, liberando o
// uso antes da hora. Aqui o instante e completo.
type Cooldowns struct {
	lastUse map[uint16]time.Time
}

// NewCooldowns cria a tabela.
func NewCooldowns() *Cooldowns {
	return &Cooldowns{lastUse: make(map[uint16]time.Time, 8)}
}

// Use marca a skill como usada agora.
func (c *Cooldowns) Use(skill uint16, now time.Time) {
	if c == nil {
		return
	}
	if c.lastUse == nil {
		c.lastUse = make(map[uint16]time.Time, 8)
	}
	c.lastUse[skill] = now
}

// Ready diz se a skill pode ser usada em now.
//
// Skill nunca usada esta pronta. A comparacao e estrita, como o
// "if not(EndTime < Now) then Exit" da origem: no instante exato do fim ainda
// nao esta pronta.
func (c *Cooldowns) Ready(skill uint16, now time.Time, t CooldownMillis) bool {
	if c == nil || c.lastUse == nil {
		return true
	}
	last, ok := c.lastUse[skill]
	if !ok {
		return true
	}
	end := last.Add(time.Duration(t.CooldownMillis(skill)) * time.Millisecond)
	return end.Before(now)
}

// Remaining devolve quanto falta para a skill ficar pronta, ou zero.
func (c *Cooldowns) Remaining(skill uint16, now time.Time, t CooldownMillis) time.Duration {
	if c == nil || c.lastUse == nil {
		return 0
	}
	last, ok := c.lastUse[skill]
	if !ok {
		return 0
	}
	end := last.Add(time.Duration(t.CooldownMillis(skill)) * time.Millisecond)
	if !end.After(now) {
		return 0
	}
	return end.Sub(now)
}

// LastUse devolve quando a skill foi usada por ultimo.
func (c *Cooldowns) LastUse(skill uint16) (time.Time, bool) {
	if c == nil || c.lastUse == nil {
		return time.Time{}, false
	}
	t, ok := c.lastUse[skill]
	return t, ok
}

// Clear esvazia a tabela. A origem faz isso ao trocar de forma (Polimorfed).
func (c *Cooldowns) Clear() {
	if c != nil && c.lastUse != nil {
		clear(c.lastUse)
	}
}

// Count conta as skills com recarga registrada.
func (c *Cooldowns) Count() int {
	if c == nil {
		return 0
	}
	return len(c.lastUse)
}
