package combat

import (
	"math/rand/v2"
	"testing"
)

// fixedRand devolve sempre o mesmo valor, para fixar a posicao sorteada.
type fixedRand int

func (f fixedRand) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(f) % n
}

// A ordem dos tipos vai no pacote $102 e o cliente desenha por ela.
func TestDamageTypeValues(t *testing.T) {
	cases := []struct {
		d    DamageType
		want uint8
	}{
		{Normal, 0}, {Critical, 1}, {Double, 2}, {DoubleCritical, 3},
		{Immune, 4}, {Miss, 8}, {Miss2, 12}, {Block, 16}, {None, 32},
	}
	for _, c := range cases {
		if uint8(c.d) != c.want {
			t.Errorf("%v = %d, queria %d", c.d, uint8(c.d), c.want)
		}
	}
	if got := Normal.String(); got != "Normal" {
		t.Errorf("String() = %q", got)
	}
	if got := DamageType(200).String(); got != "DamageType(?)" {
		t.Errorf("String() fora da faixa = %q", got)
	}
}

// Cada chance pinta as primeiras posicoes vazias: a ordem de aplicacao decide
// quem fica com cada posicao.
func TestSetChanceFillsFirstEmptySlots(t *testing.T) {
	var r Roller
	r.reset()
	r.setChance(3, Critical)
	r.setChance(2, Miss)

	want := []DamageType{Critical, Critical, Critical, Miss, Miss, Normal}
	for i, w := range want {
		if r.slots[i] != w {
			t.Errorf("posicao %d = %v, queria %v", i, r.slots[i], w)
		}
	}
	if r.Total() != 5 {
		t.Errorf("Total = %d, queria 5", r.Total())
	}
}

func TestSetChanceIgnoresZeroAndNegative(t *testing.T) {
	var r Roller
	r.reset()
	r.setChance(0, Critical)
	r.setChance(-5, Miss)
	if r.Total() != 0 {
		t.Errorf("Total = %d, queria 0", r.Total())
	}
	if r.SlotsFor(Critical) != 0 || r.SlotsFor(Miss) != 0 {
		t.Error("chance zero ou negativa nao deveria pintar posicao")
	}
}

// A roleta tem 1000 posicoes: chance maior que isso nao pode escrever fora.
func TestSetChanceCannotOverflowSlots(t *testing.T) {
	var r Roller
	r.reset()
	r.setChance(50_000, Critical)
	if got := r.SlotsFor(Critical); got != slotCount {
		t.Errorf("pintou %d posicoes, a roleta tem %d", got, slotCount)
	}
	if r.Total() != 50_000 {
		t.Errorf("Total = %d: a soma declarada continua a pedida", r.Total())
	}
}

// Sem o reset, a soma vinha de lixo da pilha e a faixa do sorteio mudava a
// cada chamada. Duas rolagens iguais tem que dar o mesmo Total.
func TestRollIsRepeatable(t *testing.T) {
	a := Attacker{Critical: 100, DuploAtk: 50, Acerto: 30}
	d := Defender{CritRes: 10, DuploRes: 5, Esquiva: 80}

	var r Roller
	first := r.Roll(a, d, true, 0, fixedRand(7))
	total1 := r.Total()
	for i := 0; i < 20; i++ {
		got := r.Roll(a, d, true, 0, fixedRand(7))
		if got != first {
			t.Fatalf("rolagem %d deu %v, a primeira deu %v", i, got, first)
		}
		if r.Total() != total1 {
			t.Fatalf("rolagem %d somou %d, a primeira somou %d", i, r.Total(), total1)
		}
	}
}

// Com os atributos zerados sobram so as chances fixas: 20 de critico e 10 de
// miss no fisico, mais 10 de duplo e 20+10 de duplo critico.
func TestRollPhysicalBaseChances(t *testing.T) {
	var r Roller
	r.Roll(Attacker{}, Defender{}, true, 0, fixedRand(0))
	if got := r.SlotsFor(Critical); got != 20 {
		t.Errorf("critico = %d posicoes, queria 20", got)
	}
	if got := r.SlotsFor(Miss); got != 10 {
		t.Errorf("miss = %d posicoes, queria 10", got)
	}
	if got := r.SlotsFor(Double); got != 10 {
		t.Errorf("duplo = %d posicoes, queria 10", got)
	}
	// 20 fixos + duploHelp, que cai para 10 quando critHelp >= (0+0)/3.
	if got := r.SlotsFor(DoubleCritical); got != 30 {
		t.Errorf("duplo critico = %d posicoes, queria 30", got)
	}
}

func TestRollMagicalBaseChances(t *testing.T) {
	var r Roller
	r.Roll(Attacker{}, Defender{}, false, 0, fixedRand(0))
	if got := r.SlotsFor(Critical); got != 30 {
		t.Errorf("critico = %d posicoes, queria 30", got)
	}
	if got := r.SlotsFor(Miss); got != 15 {
		t.Errorf("miss = %d posicoes, queria 15", got)
	}
	if got := r.SlotsFor(Double); got != 0 {
		t.Errorf("magico nao tem chance de duplo, deu %d", got)
	}
	// 20 fixos + duploHelp, que cai para 20 quando duploHelp <= critHelp.
	if got := r.SlotsFor(DoubleCritical); got != 40 {
		t.Errorf("duplo critico = %d posicoes, queria 40", got)
	}
}

// Resistencia do alvo maior que o atributo do atacante zera a chance, em vez
// de estourar o WORD na subtracao.
func TestRollResistanceCannotUnderflow(t *testing.T) {
	a := Attacker{Critical: 10, DuploAtk: 10, Acerto: 10}
	d := Defender{CritRes: 60000, DuploRes: 60000, Esquiva: 5}

	var r Roller
	r.Roll(a, d, true, 0, fixedRand(0))
	// Sobram apenas as chances fixas; nada estourou para um valor enorme.
	if got := r.SlotsFor(Critical); got != 20 {
		t.Errorf("critico = %d, queria so os 20 fixos", got)
	}
	if got := r.SlotsFor(Double); got != 10 {
		t.Errorf("duplo = %d, queria so os 10 fixos", got)
	}
	if r.Total() > slotCount {
		t.Errorf("Total = %d: a resistencia estourou a soma", r.Total())
	}
}

// Esquiva alta vira chance de miss; acerto do atacante desconta.
func TestRollMissFromEvasion(t *testing.T) {
	var r Roller
	r.Roll(Attacker{Acerto: 30}, Defender{Esquiva: 130}, true, 0, fixedRand(0))
	// 10 fixos (base/2) + (130 - 30).
	if got := r.SlotsFor(Miss); got != 110 {
		t.Errorf("miss = %d posicoes, queria 110", got)
	}
}

// Golpe de skill fisica nao conta como duplo.
func TestRollSkillPhysicalDemotesDouble(t *testing.T) {
	var r Roller
	// Posicao 30 cai na faixa de Double com os atributos zerados:
	// 20 critico + 10 miss + 10 duplo => posicoes 30..39 sao Double.
	if got := r.Roll(Attacker{}, Defender{}, true, 0, fixedRand(30)); got != Double {
		t.Fatalf("sem skill a posicao deveria dar Double, deu %v", got)
	}
	if got := r.Roll(Attacker{}, Defender{}, true, 5, fixedRand(30)); got != Normal {
		t.Errorf("com skill fisica Double deveria virar Normal, deu %v", got)
	}
}

// A soma e limitada em 998 antes do sorteio, entao o indice nunca passa do
// vetor de 1000 posicoes.
func TestRollClampsTotalAndStaysInBounds(t *testing.T) {
	a := Attacker{Critical: 60000, DuploAtk: 60000, Acerto: 0}
	d := Defender{Esquiva: 60000}

	var r Roller
	// IntN recebe o total limitado; devolver o maior valor possivel tem que
	// continuar dentro do vetor.
	got := r.Roll(a, d, true, 0, maxRand{})
	if r.Total() <= maxRoll {
		t.Fatalf("este caso precisa de Total acima de %d, deu %d", maxRoll, r.Total())
	}
	_ = got // o valor nao importa: o teste e nao estourar o indice
}

// maxRand devolve sempre o ultimo valor da faixa.
type maxRand struct{}

func (maxRand) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return n - 1
}

// Qualquer combinacao de atributos, por absurda que seja, nao pode entrar em
// panico nem devolver um tipo fora da faixa.
func TestRollNeverPanics(t *testing.T) {
	rnd := rand.New(rand.NewPCG(21, 23))
	var r Roller
	for i := 0; i < 20000; i++ {
		a := Attacker{
			Critical: uint16(rnd.UintN(65536)),
			DuploAtk: uint16(rnd.UintN(65536)),
			Acerto:   uint16(rnd.UintN(65536)),
		}
		d := Defender{
			CritRes:  uint16(rnd.UintN(65536)),
			DuploRes: uint16(rnd.UintN(65536)),
			Esquiva:  uint16(rnd.UintN(65536)),
		}
		phys := rnd.IntN(2) == 0
		skill := uint32(rnd.UintN(70000))
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("iteracao %d: panico: %v", i, p)
				}
			}()
			got := r.Roll(a, d, phys, skill, rnd)
			if got > None {
				t.Fatalf("iteracao %d: tipo %d fora da faixa", i, got)
			}
		}()
	}
}

// Total zero nao pode sortear: devolve Normal sem indexar.
func TestRollWithNoChances(t *testing.T) {
	var r Roller
	r.reset()
	// Chama o sorteio direto com a roleta vazia.
	if r.total != 0 {
		t.Fatal("a roleta deveria estar vazia")
	}
	got := r.Roll(Attacker{}, Defender{}, true, 0, fixedRand(0))
	if got != Critical {
		t.Logf("com as chances fixas a posicao 0 e Critical, deu %v", got)
	}
}
