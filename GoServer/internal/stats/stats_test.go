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

// ---- Derived

// O Round do Delphi arredonda para o par no empate; Trunc corta para zero.
func TestRoundEhBancario(t *testing.T) {
	casos := []struct {
		in   float64
		quer int
	}{
		{0.5, 0}, {1.5, 2}, {2.5, 2}, {3.5, 4},
		{1.4, 1}, {1.6, 2},
		{-0.5, 0}, {-1.5, -2}, {-2.5, -2},
	}
	for _, c := range casos {
		if got := round(c.in); got != c.quer {
			t.Errorf("round(%v) = %d, queria %d", c.in, got, c.quer)
		}
	}
}

func TestTruncCortaParaZero(t *testing.T) {
	if got := trunc(1.9); got != 1 {
		t.Errorf("trunc(1.9) = %d", got)
	}
	if got := trunc(-1.9); got != -1 {
		t.Errorf("trunc(-1.9) = %d", got)
	}
}

// Personagem sem pontos nao tem ataque, mas tem velocidade base.
func TestComputeSemPontos(t *testing.T) {
	d := Compute(Score{}, Bonus{})
	if d.AtkFis != 0 || d.AtkMag != 0 {
		t.Errorf("ataque = %d/%d, queria 0/0", d.AtkFis, d.AtkMag)
	}
	if d.SpeedMove != baseSpeedMove {
		t.Errorf("velocidade = %d, queria %d", d.SpeedMove, baseSpeedMove)
	}
}

// Os pontos efetivos somam o bonus, saturando como IncCritical.
func TestBonusDeAtributoSoma(t *testing.T) {
	semBonus := Compute(Score{Str: 50}, Bonus{})
	comBonus := Compute(Score{Str: 50}, Bonus{Str: 50})
	if comBonus.AtkFis <= semBonus.AtkFis {
		t.Errorf("bonus de forca nao somou: %d vs %d",
			comBonus.AtkFis, semBonus.AtkFis)
	}
}

// O percentual de dano incide sobre o total ja somado, como na origem.
func TestPercentualDeDanoIncideSobreOTotal(t *testing.T) {
	base := Compute(Score{Str: 100, Agility: 100}, Bonus{})
	com := Compute(Score{Str: 100, Agility: 100}, Bonus{AtkFisPercent: 50})
	quer := base.AtkFis + uint16(50*(int(base.AtkFis)/100))
	if com.AtkFis != quer {
		t.Errorf("AtkFis = %d, queria %d", com.AtkFis, quer)
	}
}

// A arma entra antes dos pontos: o percentual pega os dois.
func TestArmaEntraAntesDosPontos(t *testing.T) {
	d := Compute(Score{Str: 100}, Bonus{AtkFis: 1000})
	if d.AtkFis <= 1000 {
		t.Errorf("AtkFis = %d, deveria somar arma e pontos", d.AtkFis)
	}
}

// A defesa do equipamento recebe o percentual.
func TestDefesaPercentual(t *testing.T) {
	d := Compute(Score{}, Bonus{DefFis: 200, DefFisPercent: 50})
	quer := uint16(200 + 50*(200/100))
	if d.DefFis != quer {
		t.Errorf("DefFis = %d, queria %d", d.DefFis, quer)
	}
}

// Resistence usa Round, nao Trunc. Com Luck 7 a conta da 0.7: Round devolve 1
// e Trunc devolveria 0 — e e por isso que a origem usa Round so neste campo.
func TestResistenceUsaRound(t *testing.T) {
	if d := Compute(Score{Luck: 7}, Bonus{}); d.Resistence != 1 {
		t.Errorf("Resistence com Luck 7 = %d, queria 1", d.Resistence)
	}
	// Com Luck 5 a conta da exatamente 0.5, e o empate vai para o par: zero.
	if d := Compute(Score{Luck: 5}, Bonus{}); d.Resistence != 0 {
		t.Errorf("Resistence com Luck 5 = %d, queria 0 (empate vai para o par)",
			d.Resistence)
	}
}

// A reliquia de dano e um SEGUNDO passo percentual, sobre o valor que ja
// recebeu o primeiro (BaseMob.pas, "{ Reliquia }" depois de EF_PER_DAMAGE1).
func TestRelicDanoComporComOPercentualNormal(t *testing.T) {
	sc := Score{Str: 100}
	base := Compute(sc, Bonus{}).AtkFis // 260
	so := Compute(sc, Bonus{AtkFisPercent: 10}).AtkFis
	ambos := Compute(sc, Bonus{AtkFisPercent: 10, RelicAtkFisPercent: 10}).AtkFis
	if base != 260 || so != 280 {
		t.Fatalf("base=%d so=%d", base, so)
	}
	if want := uint16(280 + 10*(280/100)); ambos != want {
		t.Errorf("com reliquia = %d, queria %d", ambos, want)
	}
	if m := Compute(Score{Int: 100}, Bonus{RelicAtkMagPercent: 10}).AtkMag; m != 320+10*3 {
		t.Errorf("magico = %d", m)
	}
}
