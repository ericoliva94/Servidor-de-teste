package stats

import "testing"

func TestMobClass(t *testing.T) {
	casos := []struct {
		classInfo uint16
		quer      int
	}{
		{1, ClassWarrior}, {9, ClassWarrior},
		{11, ClassTemplar}, {19, ClassTemplar},
		{21, ClassArcher}, {29, ClassArcher},
		{31, ClassDual}, {39, ClassDual},
		{41, ClassMage}, {49, ClassMage},
		{51, ClassCleric}, {59, ClassCleric},
		// Valores que a origem deixava cair em guerreiro.
		{0, ClassWarrior}, {10, ClassWarrior}, {60, ClassWarrior},
		{65535, ClassWarrior},
	}
	for _, c := range casos {
		if got := MobClass(c.classInfo); got != c.quer {
			t.Errorf("MobClass(%d) = %d, quer %d", c.classInfo, got, c.quer)
		}
	}
}

func TestMaxHPGuerreiroNu(t *testing.T) {
	// War nivel 10, CONS 20: Round(150*0.3)=45 por nivel.
	in := Input{ClassInfo: 1, Level: 10, Cons: 20}
	quer := uint32(45*10 + 20*27)
	if got := MaxHP(in); got != quer {
		t.Fatalf("MaxHP = %d, quer %d", got, quer)
	}
}

func TestMaxMPUsaLuck(t *testing.T) {
	// Mago nivel 10, Luck 20: Round(330*0.3)=99 por nivel.
	in := Input{ClassInfo: 41, Level: 10, Luck: 20}
	quer := uint32(99*10 + 20*27)
	if got := MaxMP(in); got != quer {
		t.Fatalf("MaxMP = %d, quer %d", got, quer)
	}
}

func TestPercentualSomaSobreOTotal(t *testing.T) {
	base := Input{ClassInfo: 1, Level: 10, Cons: 20}
	semPerc := MaxHP(base)
	base.PercentHP = 50
	comPerc := MaxHP(base)
	quer := semPerc + 50*(semPerc/100)
	if comPerc != quer {
		t.Fatalf("MaxHP com 50%% = %d, quer %d", comPerc, quer)
	}
}

// TestBonusNegativoNaoDaVolta pinna a correcao: na origem a conta era em
// DWORD e um bonus negativo virava quase 4 bilhoes de HP.
func TestBonusNegativoNaoDaVolta(t *testing.T) {
	in := Input{ClassInfo: 1, Level: 1, Cons: 0, FlatHP: -1000}
	got := MaxHP(in)
	if got != 1 {
		t.Fatalf("MaxHP com bonus negativo = %d, quer 1", got)
	}
	in2 := Input{ClassInfo: 1, Level: 1, Luck: 0, FlatMP: -1000}
	if got := MaxMP(in2); got != 1 {
		t.Fatalf("MaxMP com bonus negativo = %d, quer 1", got)
	}
}

func TestNivelZeroNaoEstoura(t *testing.T) {
	// Slot vazio chega com nivel 0; o piso de 1 tem de valer.
	if got := MaxHP(Input{ClassInfo: 0, Level: 0}); got != 1 {
		t.Fatalf("MaxHP nivel 0 = %d, quer 1", got)
	}
	if got := MaxMP(Input{ClassInfo: 0, Level: 0}); got != 1 {
		t.Fatalf("MaxMP nivel 0 = %d, quer 1", got)
	}
}

func TestTodasAsClassesTemTabela(t *testing.T) {
	for _, ci := range []uint16{1, 11, 21, 31, 41, 51} {
		if got := MaxHP(Input{ClassInfo: ci, Level: 1, Cons: 1}); got == 0 {
			t.Errorf("classe %d deu HP zero", ci)
		}
		if got := MaxMP(Input{ClassInfo: ci, Level: 1, Luck: 1}); got == 0 {
			t.Errorf("classe %d deu MP zero", ci)
		}
	}
}
