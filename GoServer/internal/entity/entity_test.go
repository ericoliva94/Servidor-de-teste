package entity

import (
	"math/rand/v2"
	"testing"
	"time"
)

// fixedDurations devolve a mesma duracao para todo buff, exceto os listados.
type fixedDurations struct {
	def  int
	spec map[uint16]int
}

func (f fixedDurations) BuffSeconds(i uint16) int {
	if d, ok := f.spec[i]; ok {
		return d
	}
	return f.def
}

func TestBuffsSetHasRemove(t *testing.T) {
	b := NewBuffs()
	now := time.Now()

	if b.Has(10) {
		t.Error("buff nao deveria existir")
	}
	if !b.Set(10, now) {
		t.Fatal("Set falhou")
	}
	if !b.Has(10) || b.Count() != 1 {
		t.Errorf("Has=%v Count=%d", b.Has(10), b.Count())
	}
	if got, ok := b.StartedAt(10); !ok || !got.Equal(now) {
		t.Errorf("StartedAt=%v ok=%v", got, ok)
	}
	if !b.Remove(10) {
		t.Error("Remove deveria devolver true")
	}
	if b.Remove(10) {
		t.Error("Remove de buff ausente deveria devolver false")
	}
	if b.Count() != 0 {
		t.Errorf("Count=%d", b.Count())
	}
}

// Substituir um buff existente nao conta como novo e nao bate no limite.
func TestBuffsLimit(t *testing.T) {
	b := NewBuffs()
	now := time.Now()
	for i := range MaxBuffs {
		if !b.Set(uint16(i+1), now) {
			t.Fatalf("Set do buff %d falhou antes do limite", i+1)
		}
	}
	if !b.Full() {
		t.Error("deveria estar cheia")
	}
	if b.Set(9999, now) {
		t.Error("Set de buff novo deveria falhar com a lista cheia")
	}
	if !b.Set(1, now.Add(time.Minute)) {
		t.Error("substituir buff existente deveria funcionar com a lista cheia")
	}
	if b.Count() != MaxBuffs {
		t.Errorf("Count=%d, queria %d", b.Count(), MaxBuffs)
	}
}

// O caso que a origem errava: remover dentro da iteracao do dicionario podia
// pular buffs vencidos. Com muitos buffs vencidos de uma vez, todos tem que
// sair numa chamada.
func TestBuffsRemoveExpiredRemovesAllAtOnce(t *testing.T) {
	b := NewBuffs()
	d := fixedDurations{def: 10}
	base := time.Now()

	const n = 50
	for i := range n {
		b.Set(uint16(i+1), base)
	}
	expired := b.RemoveExpired(base.Add(11*time.Second), d)
	if len(expired) != n {
		t.Fatalf("expirou %d de %d numa chamada", len(expired), n)
	}
	if b.Count() != 0 {
		t.Errorf("sobraram %d buffs vencidos", b.Count())
	}
}

func TestBuffsExpiryBoundary(t *testing.T) {
	b := NewBuffs()
	d := fixedDurations{def: 10}
	base := time.Now()
	b.Set(1, base)

	// Exatamente no fim ainda nao expirou: a origem usava EndTime < Now.
	if got := b.Expired(base.Add(10*time.Second), d); len(got) != 0 {
		t.Errorf("expirou no limite exato: %v", got)
	}
	if got := b.Expired(base.Add(10*time.Second+time.Nanosecond), d); len(got) != 1 {
		t.Errorf("deveria expirar um pouco depois do limite: %v", got)
	}
}

// Duracao zero (indice de buff que nao existe na tabela) expira de imediato,
// em vez de ler memoria fora do array como SkillData[Index] fazia.
func TestBuffsUnknownIndexHasZeroDuration(t *testing.T) {
	b := NewBuffs()
	d := fixedDurations{def: 0}
	base := time.Now()
	b.Set(65535, base)
	if got := b.RemoveExpired(base.Add(time.Nanosecond), d); len(got) != 1 {
		t.Errorf("buff com duracao desconhecida deveria expirar: %v", got)
	}
}

// Metodos em *Buffs nil nao podem entrar em panico.
func TestBuffsNilSafe(t *testing.T) {
	var b *Buffs
	if b.Count() != 0 || b.Has(1) || b.Remove(1) || b.Set(1, time.Now()) {
		t.Error("operacoes em *Buffs nil deveriam ser inofensivas")
	}
	if !b.Full() {
		t.Error("*Buffs nil deveria se declarar cheia")
	}
	if b.Indexes() != nil || b.Expired(time.Now(), fixedDurations{}) != nil {
		t.Error("listagens em nil deveriam ser nil")
	}
	b.Clear()
}

// O defeito central da lista de alvos da origem: o vetor crescia a cada alvo
// novo mesmo havendo buraco livre.
func TestTargetListReusesFreedSlots(t *testing.T) {
	l := NewTargetList()
	for i := 1; i <= 10; i++ {
		if _, ok := l.Add(Target{ClientID: uint16(i)}); !ok {
			t.Fatalf("Add %d falhou", i)
		}
	}
	if l.Cap() != 10 || l.Len() != 10 {
		t.Fatalf("Cap=%d Len=%d", l.Cap(), l.Len())
	}

	// Entra e sai mil vezes: a capacidade nao pode crescer.
	for i := 0; i < 1000; i++ {
		id := uint16(100 + i)
		l.Remove(5)
		if _, ok := l.Add(Target{ClientID: id}); !ok {
			t.Fatalf("Add %d falhou", id)
		}
		l.Remove(id)
		l.Add(Target{ClientID: 5})
	}
	if l.Cap() != 10 {
		t.Errorf("Cap=%d: a lista cresceu reaproveitando slot", l.Cap())
	}
	if l.Len() != 10 {
		t.Errorf("Len=%d, queria 10", l.Len())
	}
}

// Len tem que contar os ocupados, nao o tamanho do vetor: na origem o contador
// divergia porque era atribuido na inclusao e decrementado na remocao.
func TestTargetListLenMatchesOccupied(t *testing.T) {
	l := NewTargetList()
	for i := 1; i <= 5; i++ {
		l.Add(Target{ClientID: uint16(i)})
	}
	l.Remove(2)
	l.Remove(4)
	if l.Len() != 3 {
		t.Errorf("Len=%d, queria 3", l.Len())
	}
	if l.Cap() != 5 {
		t.Errorf("Cap=%d, queria 5", l.Cap())
	}
	if len(l.All()) != 3 {
		t.Errorf("All devolveu %d", len(l.All()))
	}
}

func TestTargetListRejectsZeroID(t *testing.T) {
	l := NewTargetList()
	if _, ok := l.Add(Target{ClientID: 0}); ok {
		t.Error("ClientID zero e a marca de slot livre: nao pode ser alvo")
	}
	if l.Has(0) {
		t.Error("Has(0) deveria ser false")
	}
}

func TestTargetListRespectsMax(t *testing.T) {
	l := &TargetList{max: 4}
	for i := 1; i <= 4; i++ {
		if _, ok := l.Add(Target{ClientID: uint16(i)}); !ok {
			t.Fatalf("Add %d falhou", i)
		}
	}
	if _, ok := l.Add(Target{ClientID: 99}); ok {
		t.Error("deveria recusar acima do limite")
	}
}

func TestTargetListAtOutOfRange(t *testing.T) {
	l := NewTargetList()
	l.Add(Target{ClientID: 7})
	if l.At(-1) != nil || l.At(99) != nil {
		t.Error("At fora da faixa deveria devolver nil")
	}
	if got := l.At(0); got == nil || got.ClientID != 7 {
		t.Errorf("At(0) = %+v", got)
	}
}

func TestTargetListCompact(t *testing.T) {
	l := NewTargetList()
	for i := 1; i <= 6; i++ {
		l.Add(Target{ClientID: uint16(i)})
	}
	for i := 3; i <= 6; i++ {
		l.Remove(uint16(i))
	}
	l.Compact()
	if l.Cap() != 2 {
		t.Errorf("Cap=%d depois de Compact, queria 2", l.Cap())
	}
	if l.Len() != 2 {
		t.Errorf("Len=%d", l.Len())
	}
}

func TestTargetListNilSafe(t *testing.T) {
	var l *TargetList
	if l.Len() != 0 || l.Cap() != 0 || l.Has(1) || l.Remove(1) || l.At(0) != nil {
		t.Error("operacoes em *TargetList nil deveriam ser inofensivas")
	}
	if _, ok := l.Add(Target{ClientID: 1}); ok {
		t.Error("Add em nil deveria falhar")
	}
	l.Clear()
	l.Compact()
}

func TestVisibleSet(t *testing.T) {
	var s VisibleSet
	if !s.Add(5) || !s.Add(9) || !s.Add(1) {
		t.Fatal("Add falhou")
	}
	if s.Add(5) {
		t.Error("duplicata deveria ser recusada")
	}
	if s.Add(0) {
		t.Error("id zero deveria ser recusado")
	}
	if got := s.IDs(); len(got) != 3 || got[0] != 5 || got[1] != 9 || got[2] != 1 {
		t.Errorf("ordem de inclusao nao preservada: %v", got)
	}
	if !s.Remove(9) || s.Remove(9) {
		t.Error("Remove inconsistente")
	}
	if s.Len() != 2 || s.Contains(9) {
		t.Errorf("Len=%d Contains(9)=%v", s.Len(), s.Contains(9))
	}
	s.Clear()
	if s.Len() != 0 {
		t.Errorf("Clear deixou %d", s.Len())
	}
}

func TestVisibleSetNilSafe(t *testing.T) {
	var s *VisibleSet
	if s.Add(1) || s.Remove(1) || s.Contains(1) || s.Len() != 0 || s.IDs() != nil {
		t.Error("operacoes em *VisibleSet nil deveriam ser inofensivas")
	}
	s.Clear()
}

// Players e ARRAY [1..1000]: o id 0 e os ids de mob nao podem passar.
func TestIsPlayerID(t *testing.T) {
	cases := []struct {
		id   uint16
		max  uint16
		want bool
	}{
		{0, 1000, false}, // o Players[0] que o original aceitava
		{1, 1000, true},
		{1000, 1000, true},
		{1001, 1000, false},
		{500, 100, false}, // acima do MAX_CONNECTIONS em uso
		{100, 100, true},
		{1000, 5000, true}, // .ini acima do vetor: limita em 1000
		{1001, 5000, false},
	}
	for _, c := range cases {
		if got := IsPlayerID(c.id, c.max); got != c.want {
			t.Errorf("IsPlayerID(%d, %d) = %v, queria %v", c.id, c.max, got, c.want)
		}
	}
}

func TestIsMobID(t *testing.T) {
	for _, id := range []uint16{1001, 3339, 3370, 9147} {
		if !IsMobID(id) {
			t.Errorf("%d deveria ser mob", id)
		}
	}
	for _, id := range []uint16{0, 1, 1000, 3340, 3369, 9148} {
		if IsMobID(id) {
			t.Errorf("%d nao deveria ser mob", id)
		}
	}
	if !IsDungeonMobID(9148) || IsDungeonMobID(9147) {
		t.Error("faixa de mob de dungeon errada")
	}
}

func TestClampMaxConnections(t *testing.T) {
	cases := []struct{ in, want int }{
		{-5, 0}, {0, 0}, {200, 200}, {1000, 1000}, {5000, 1000},
	}
	for _, c := range cases {
		if got := ClampMaxConnections(c.in); int(got) != c.want {
			t.Errorf("ClampMaxConnections(%d) = %d, queria %d", c.in, got, c.want)
		}
	}
}

// Sequencia aleatoria de operacoes: nada pode entrar em panico e os
// invariantes tem que se manter.
func TestEntityCoreNeverPanics(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 13))
	d := fixedDurations{def: 1}
	for i := 0; i < 5000; i++ {
		b := NewBuffs()
		l := NewTargetList()
		var s VisibleSet
		now := time.Now()
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("iteracao %d: panico: %v", i, p)
				}
			}()
			for op := 0; op < 40; op++ {
				id := uint16(r.UintN(70000))
				switch r.IntN(9) {
				case 0:
					b.Set(id, now)
				case 1:
					b.Remove(id)
				case 2:
					b.RemoveExpired(now.Add(time.Duration(r.IntN(5))*time.Second), d)
				case 3:
					l.Add(Target{ClientID: id})
				case 4:
					l.Remove(id)
				case 5:
					l.At(r.IntN(20) - 5)
				case 6:
					s.Add(id)
				case 7:
					s.Remove(id)
				case 8:
					l.Compact()
				}
				if l.Len() > l.Cap() {
					t.Fatalf("Len %d maior que Cap %d", l.Len(), l.Cap())
				}
				if b.Count() > MaxBuffs {
					t.Fatalf("Count %d passou do limite", b.Count())
				}
			}
		}()
	}
}
