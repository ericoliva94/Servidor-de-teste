package combat

import "testing"

// seq devolve os valores de IntN na ordem; IntN(n) usa v-ajuste para cair no
// valor pedido por RandomRange (que soma a base).
type seq struct {
	v []int
	i int
}

func (s *seq) IntN(n int) int {
	x := s.v[s.i%len(s.v)]
	s.i++
	return x % n
}

// rr devolve o IntN que faz randomRange(base..) dar o valor desejado.
func rr(want, base int) int { return want - base }

func TestRoll3MissThenGuaranteedHit(t *testing.T) {
	var r Roller
	a := Attacker{Acerto: 0}
	d := Defender{Esquiva: 10, IsPlayer: true}
	// diferenca -10: erra se sorteio <= 40. Sorteio 1 erra.
	for i := 1; i <= 2; i++ {
		if got := r.Roll3(a, d, 0, &seq{v: []int{rr(1, 1)}}); got != Miss {
			t.Fatalf("golpe %d: %v, queria Miss", i, got)
		}
	}
	// o terceiro nao pode errar e zera o contador.
	if got := r.Roll3(a, d, 0, &seq{v: []int{rr(1, 1)}}); got == Miss {
		t.Fatal("o terceiro golpe seguido nao pode errar")
	}
	if r.missCount != 0 {
		t.Errorf("missCount = %d, queria 0", r.missCount)
	}
}

func TestRoll3NoMissWithLowEsquiva(t *testing.T) {
	var r Roller
	d := Defender{Esquiva: 2, IsPlayer: true}
	if got := r.Roll3(Attacker{}, d, 0, &seq{v: []int{0}}); got == Miss {
		t.Error("Esquiva < 3 nunca faz errar")
	}
}

func TestRoll3CritThreshold(t *testing.T) {
	var r Roller
	a := Attacker{Acerto: 100, Critical: 50}
	d := Defender{IsPlayer: true} // resCrit = 14; dif 36 -> teto 25
	// ordem das chamadas: acerto, critico. Sorteio de critico 15 -> 15+25=40.
	if got := r.Roll3(a, d, 5, &seq{v: []int{rr(99, 1), rr(15, 1)}}); got != Critical {
		t.Errorf("sorteio 15: %v, queria Critical", got)
	}
	if got := r.Roll3(a, d, 5, &seq{v: []int{rr(99, 1), rr(14, 1)}}); got != Normal {
		t.Errorf("sorteio 14: %v, queria Normal", got)
	}
}

func TestRoll3ForceCrit(t *testing.T) {
	var r Roller
	a := Attacker{Acerto: 100, ForceCrit: true}
	if got := r.Roll3(a, Defender{IsPlayer: true}, 0, &seq{v: []int{rr(99, 1), rr(1, 1)}}); got != Critical {
		t.Errorf("com o buff: %v", got)
	}
}

func TestRoll3DoubleOnlyBasicAttack(t *testing.T) {
	var r Roller
	a := Attacker{Acerto: 100, DuploAtk: 40}
	d := Defender{IsPlayer: true} // resDuplo 7; dif 33 -> teto 10; sorteio >= 70
	rolls := []int{rr(99, 1), rr(1, 1), rr(70, 1)}
	if got := r.Roll3(a, d, 0, &seq{v: rolls}); got != Double {
		t.Errorf("ataque basico: %v, queria Double", got)
	}
	if got := r.Roll3(a, d, 7, &seq{v: rolls}); got != Normal {
		t.Errorf("com skill: %v, queria Normal (so o ataque basico duplica)", got)
	}
}

func TestRoll3MobGetsAccuracyBonus(t *testing.T) {
	var r Roller
	// Acerto 0 contra Esquiva 10: jogador erraria com sorteio baixo; contra
	// mob o acerto sobe 20..39, a diferenca vira >= 10 e o sorteio 1+10 <= 20
	// ainda erra (Esquiva >= 7), mas com sorteio 11 o golpe passa.
	d := Defender{Esquiva: 10, IsPlayer: false}
	if got := r.Roll3(Attacker{}, d, 5, &seq{v: []int{0, rr(11, 1)}}); got == Miss {
		t.Error("sorteio 11 + teto 10 > 20: deveria acertar")
	}
}
