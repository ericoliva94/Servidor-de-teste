package combat

import (
	"math/rand/v2"
	"testing"
)

// fakeTarget implementa Target guardando o que aconteceu.
type fakeTarget struct {
	buffs     map[uint16]bool
	abilities map[int]int
	msgs      []string
	hp        int
	isPlayer  bool
}

func newTarget() *fakeTarget {
	return &fakeTarget{
		buffs:     map[uint16]bool{},
		abilities: map[int]int{},
		isPlayer:  true,
	}
}

func (f *fakeTarget) HasBuff(i uint16) bool { return f.buffs[i] }
func (f *fakeTarget) RemoveBuff(i uint16)   { delete(f.buffs, i) }
func (f *fakeTarget) Ability(e int) int     { return f.abilities[e] }
func (f *fakeTarget) DecAbility(e, v int) {
	f.abilities[e] -= v
	if f.abilities[e] < 0 {
		f.abilities[e] = 0
	}
}
func (f *fakeTarget) Notify(m string) {
	// Um mob nao recebe mensagem. Na origem a entrega era
	// Players[mob.ClientID].SendClientMessage, que indexava fora do vetor
	// justamente quando o alvo era mob.
	if f.isPlayer {
		f.msgs = append(f.msgs, m)
	}
}
func (f *fakeTarget) AddHP(v int) { f.hp += v }

func TestAmplify(t *testing.T) {
	tg := newTarget()
	if got := Amplify(1000, tg); got != 1000 {
		t.Errorf("sem efeito = %d, queria 1000", got)
	}
	tg.abilities[EFAmpPhysical] = 25
	if got := Amplify(1000, tg); got != 1250 {
		t.Errorf("com 25%% = %d, queria 1250", got)
	}
	tg2 := newTarget()
	tg2.abilities[EFType45] = 1
	if got := Amplify(1000, tg2); got != 1100 {
		t.Errorf("raio solar = %d, queria 1100", got)
	}
	// Os dois somam, com a divisao inteira da origem ("Dano div 100"):
	// 1000 + 25% = 1250, depois (1250 div 100)*10 = 120, logo 1370 e nao 1375.
	tg3 := newTarget()
	tg3.abilities[EFAmpPhysical] = 25
	tg3.abilities[EFType45] = 1
	if got := Amplify(1000, tg3); got != 1370 {
		t.Errorf("os dois = %d, queria 1370", got)
	}
}

// Escudo maior que o golpe: dano vai a zero, tipo vira None, escudo desconta.
func TestAbsorbShieldCoversHit(t *testing.T) {
	tg := newTarget()
	tg.buffs[BuffAbsorb432] = true
	tg.abilities[EFSkillAbsorb1] = 500

	dano, dt := Absorb(200, Critical, tg)
	if dano != 0 {
		t.Errorf("dano = %d, queria 0", dano)
	}
	if dt != None {
		t.Errorf("tipo = %v, queria None", dt)
	}
	if tg.abilities[EFSkillAbsorb1] != 300 {
		t.Errorf("escudo = %d, queria 300", tg.abilities[EFSkillAbsorb1])
	}
	if !tg.buffs[BuffAbsorb432] {
		t.Error("o buff deveria continuar: o escudo cobriu o golpe")
	}
	if len(tg.msgs) != 1 {
		t.Errorf("mensagens = %d, queria 1", len(tg.msgs))
	}
}

// Escudo menor: consome, reduz o dano e o buff cai.
func TestAbsorbShieldBreaks(t *testing.T) {
	tg := newTarget()
	tg.buffs[BuffAbsorb432] = true
	tg.abilities[EFSkillAbsorb1] = 50

	dano, dt := Absorb(200, Critical, tg)
	if dano != 150 {
		t.Errorf("dano = %d, queria 150", dano)
	}
	if dt != Critical {
		t.Errorf("tipo = %v, queria Critical", dt)
	}
	if tg.buffs[BuffAbsorb432] {
		t.Error("o buff deveria cair quando o escudo nao cobre")
	}
}

// Os quatro buffs de absorcao aplicam em sequencia, na ordem da origem.
func TestAbsorbAppliesAllRulesInOrder(t *testing.T) {
	tg := newTarget()
	for _, b := range []uint16{BuffAbsorb432, BuffAbsorb123, BuffAbsorb131} {
		tg.buffs[b] = true
	}
	tg.abilities[EFSkillAbsorb1] = 30

	// Peculiaridade da origem, mantida: no caminho em que o escudo NAO cobre
	// o golpe, o Delphi desconta o escudo do dano mas nao chama
	// DecreasseMobAbility, ou seja nao consome o escudo guardado. Com tres
	// buffs de absorcao ativos o mesmo valor e descontado tres vezes:
	// 300 - 30 - 30 - 30 = 210.
	dano, _ := Absorb(300, Normal, tg)
	if dano != 210 {
		t.Errorf("dano = %d, queria 210", dano)
	}
	if tg.abilities[EFSkillAbsorb1] != 30 {
		t.Errorf("escudo = %d: o caminho de quebra nao consome o escudo na origem",
			tg.abilities[EFSkillAbsorb1])
	}
	for _, b := range []uint16{BuffAbsorb432, BuffAbsorb123, BuffAbsorb131} {
		if tg.buffs[b] {
			t.Errorf("buff %d deveria ter caido", b)
		}
	}
}

func TestAbsorbSkippedWhenNoDamage(t *testing.T) {
	tg := newTarget()
	tg.buffs[BuffAbsorb432] = true
	tg.abilities[EFSkillAbsorb1] = 500

	dano, dt := Absorb(0, Miss, tg)
	if dano != 0 || dt != Miss {
		t.Errorf("dano=%d tipo=%v: sem dano nada deve mudar", dano, dt)
	}
	if tg.abilities[EFSkillAbsorb1] != 500 {
		t.Error("o escudo nao deveria ser consumido sem dano")
	}
}

// O caso que estourava o vetor na origem: alvo que nao e jogador.
func TestAbsorbOnNonPlayerTargetDoesNotNotify(t *testing.T) {
	tg := newTarget()
	tg.isPlayer = false
	tg.buffs[BuffAbsorb142] = true
	tg.abilities[EFSkillAbsorb2] = 10

	dano, _ := Absorb(100, Normal, tg)
	if dano != 90 {
		t.Errorf("dano = %d, queria 90", dano)
	}
	if len(tg.msgs) != 0 {
		t.Errorf("mob nao deveria receber mensagem, recebeu %d", len(tg.msgs))
	}
}

func TestPointDefence(t *testing.T) {
	tg := newTarget()
	pts := uint8(3)
	if got := PointDefence(500, tg, &pts); got != 500 {
		t.Errorf("sem buff = %d", got)
	}
	if pts != 3 {
		t.Errorf("sem buff nao deveria gastar ponto, sobrou %d", pts)
	}

	tg.buffs[BuffPointDefence] = true
	tg.abilities[EFPointDefence] = 20
	if got := PointDefence(500, tg, &pts); got != 400 {
		t.Errorf("com 20%% = %d, queria 400", got)
	}
	if pts != 2 {
		t.Errorf("pontos = %d, queria 2", pts)
	}
}

// Pontos de defesa nao podem estourar abaixo de zero.
func TestPointDefenceCannotUnderflowPoints(t *testing.T) {
	tg := newTarget()
	tg.buffs[BuffPointDefence] = true
	tg.abilities[EFPointDefence] = 10
	pts := uint8(0)
	PointDefence(100, tg, &pts)
	if pts != 0 {
		t.Errorf("pontos = %d, queria 0", pts)
	}
	// Ponteiro nil tambem nao pode entrar em panico.
	PointDefence(100, tg, nil)
}

// O bug de digitacao: o reflexo tem que usar a porcentagem lida, nao um valor
// de outra conta.
func TestReflectUsesTheAbilityItRead(t *testing.T) {
	alvo := newTarget()
	alvo.abilities[EFReflection1] = 10
	atacante := newTarget()

	refletido := Reflect(1000, alvo, atacante)
	if refletido != 100 {
		t.Errorf("refletido = %d, queria 100 (10%% de 1000)", refletido)
	}
	if atacante.hp != -100 {
		t.Errorf("vida do atacante = %d, queria -100", atacante.hp)
	}
}

func TestReflectNoopCases(t *testing.T) {
	alvo := newTarget()
	atacante := newTarget()

	if got := Reflect(0, alvo, atacante); got != 0 {
		t.Errorf("sem dano = %d", got)
	}
	if got := Reflect(100, alvo, atacante); got != 0 {
		t.Errorf("sem o efeito = %d", got)
	}
	alvo.abilities[EFReflection1] = 10
	if got := Reflect(100, alvo, nil); got != 0 {
		t.Errorf("sem atacante = %d", got)
	}
	if atacante.hp != 0 {
		t.Errorf("vida do atacante mudou: %d", atacante.hp)
	}
}

// Acima do teto o dano vira 1, nao o teto. E assim na origem.
func TestClampDamage(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0},
		{1, 1},
		{500, 500},
		{MaxDamage, MaxDamage},
		{MaxDamage + 1, 1},
		{1 << 30, 1},
		{-1, 1},
		{-999999, 1},
	}
	for _, c := range cases {
		if got := ClampDamage(c.in); got != c.want {
			t.Errorf("ClampDamage(%d) = %d, queria %d", c.in, got, c.want)
		}
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", 999999: "999999", -5: "-5"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, queria %q", in, got, want)
		}
	}
}

// Nenhuma combinacao de dano e efeito pode entrar em panico, e o dano final
// nunca pode ser negativo.
func TestPipelineNeverPanics(t *testing.T) {
	r := rand.New(rand.NewPCG(31, 37))
	for i := 0; i < 20000; i++ {
		tg := newTarget()
		atacante := newTarget()
		for _, b := range []uint16{BuffAbsorb432, BuffAbsorb123, BuffAbsorb131,
			BuffAbsorb142, BuffPointDefence} {
			if r.IntN(2) == 0 {
				tg.buffs[b] = true
			}
		}
		for _, e := range []int{EFAmpPhysical, EFType45, EFSkillAbsorb1,
			EFSkillAbsorb2, EFPointDefence, EFReflection1} {
			tg.abilities[e] = r.IntN(200) - 50 // inclui negativo
		}
		dano := r.IntN(2_000_000) - 500_000 // inclui negativo
		pts := uint8(r.UintN(256))

		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("iteracao %d: panico: %v", i, p)
				}
			}()
			d := Amplify(dano, tg)
			d, dt := Absorb(d, Normal, tg)
			d = PointDefence(d, tg, &pts)
			Reflect(d, tg, atacante)
			d = ClampDamage(d)
			if d < 0 {
				t.Fatalf("iteracao %d: dano final negativo: %d", i, d)
			}
			if d > MaxDamage {
				t.Fatalf("iteracao %d: dano final acima do teto: %d", i, d)
			}
			_ = dt
		}()
	}
}
