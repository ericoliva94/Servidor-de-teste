package skillfn

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// tabela responde do mapa.
type tabela struct{ m map[int]gamedata.T_SkillData }

func (t tabela) Skill(i int) *gamedata.T_SkillData {
	d, ok := t.m[i]
	if !ok {
		return nil
	}
	return &d
}

func nova() tabela { return tabela{m: map[int]gamedata.T_SkillData{}} }

// O bug central: "SkillData[Packet.SkillIndex]" com o indice vindo do cliente
// como DWORD num ARRAY [0..11998], lido cinco vezes antes de qualquer
// validacao.
func TestDataRecusaIndiceForaDaTabela(t *testing.T) {
	tb := nova()
	tb.m[5] = gamedata.T_SkillData{Index: 5}

	if d := Data(tb, 5); d == nil || d.Index != 5 {
		t.Error("indice valido deveria ser encontrado")
	}
	for _, i := range []uint32{11999, 50000, 1 << 20, 4000000000} {
		if d := Data(tb, i); d != nil {
			t.Errorf("indice %d deveria ser recusado", i)
		}
	}
	if Data(nil, 5) != nil {
		t.Error("tabela nil deveria recusar tudo")
	}
	// Indice dentro da faixa mas ausente da tabela tambem.
	if Data(tb, 7) != nil {
		t.Error("indice ausente deveria ser nil")
	}
}

func TestGroupBase(t *testing.T) {
	var sk model.SkillsList
	sk.Basics[0] = model.SkillFromList{Index: 100, Level: 3}
	sk.Others[2] = model.SkillFromList{Index: 500, Level: 7}

	casos := []struct {
		index  uint32
		base   uint16
		nivel  uint16
		basico bool
		ok     bool
	}{
		{100, 100, 3, true, true},
		{115, 100, 3, true, true}, // ultimo do grupo
		{116, 0, 0, false, false}, // fora do grupo
		{500, 500, 7, false, true},
		{515, 500, 7, false, true},
		{99, 0, 0, false, false},
		{9999, 0, 0, false, false},
	}
	for _, c := range casos {
		base, nivel, _, basico, ok := GroupBase(sk, c.index)
		if ok != c.ok || base != c.base || nivel != c.nivel || (ok && basico != c.basico) {
			t.Errorf("GroupBase(%d) = %d/%d/%v/%v, queria %d/%d/%v/%v",
				c.index, base, nivel, basico, ok, c.base, c.nivel, c.basico, c.ok)
		}
	}
}

// Slot vazio (indice 0) nao casa com nada: sem isso toda skill de 0 a 15
// cairia no primeiro slot vazio.
func TestGroupBaseIgnoraSlotVazio(t *testing.T) {
	var sk model.SkillsList
	for _, i := range []uint32{0, 1, 10, 15} {
		if _, _, _, _, ok := GroupBase(sk, i); ok {
			t.Errorf("indice %d nao deveria casar com slot vazio", i)
		}
	}
}

// A origem nao conferia se a skill estava aprendida em UseSkill: bastava
// mandar o indice no $320.
func TestKnows(t *testing.T) {
	var sk model.SkillsList
	sk.Others[0] = model.SkillFromList{Index: 200, Level: 2}

	// Niveis liberados: 200, 201, 202.
	for _, i := range []uint32{200, 201, 202} {
		if !Knows(sk, i) {
			t.Errorf("deveria conhecer %d", i)
		}
	}
	// Acima do nivel aprendido, nao.
	for _, i := range []uint32{203, 210, 215} {
		if Knows(sk, i) {
			t.Errorf("nao deveria conhecer %d", i)
		}
	}
	// Fora do grupo, nao.
	if Knows(sk, 300) {
		t.Error("nao deveria conhecer skill de outro grupo")
	}
}

// Grupo com nivel 0 nao libera nem a base: a origem deixava usar.
func TestKnowsComNivelZero(t *testing.T) {
	var sk model.SkillsList
	sk.Others[0] = model.SkillFromList{Index: 200, Level: 0}
	if !Knows(sk, 200) {
		t.Error("o nivel 0 libera a propria base")
	}
	if Knows(sk, 201) {
		t.Error("o nivel 0 nao libera o segundo nivel")
	}
}

func TestIncLevel(t *testing.T) {
	var sk model.SkillsList
	sk.Others[3] = model.SkillFromList{Index: 300, Level: 0}

	base, ok := IncLevel(&sk, 300)
	if !ok || base != 300 {
		t.Fatalf("= %d/%v", base, ok)
	}
	if sk.Others[3].Level != 1 {
		t.Errorf("nivel = %d, queria 1", sk.Others[3].Level)
	}
	// Skill de outro grupo nao sobe nada.
	if _, ok := IncLevel(&sk, 999); ok {
		t.Error("nao deveria ter subido")
	}
	if IncLevel(nil, 300); false {
		t.Error("nunca")
	}
}

// O bug: "inc(Skills.Basics[i].Level)" sem teto. Passando de 15,
// "Index + Level" entrava no grupo de skill seguinte.
func TestIncLevelTemTeto(t *testing.T) {
	var sk model.SkillsList
	sk.Others[0] = model.SkillFromList{Index: 400, Level: MaxSkillLevel}

	base, ok := IncLevel(&sk, 400)
	if ok {
		t.Error("nao deveria subir acima do teto")
	}
	if base != 400 {
		t.Errorf("base = %d, queria 400 para a mensagem de teto", base)
	}
	if sk.Others[0].Level != MaxSkillLevel {
		t.Errorf("nivel = %d, queria %d", sk.Others[0].Level, MaxSkillLevel)
	}
}

// Subir ate o teto a partir do zero da exatamente 15 passos.
func TestIncLevelDoZeroAoTeto(t *testing.T) {
	var sk model.SkillsList
	sk.Others[0] = model.SkillFromList{Index: 400}
	passos := 0
	for {
		if _, ok := IncLevel(&sk, 400); !ok {
			break
		}
		passos++
		if passos > 100 {
			t.Fatal("laco infinito")
		}
	}
	if passos != MaxSkillLevel {
		t.Errorf("%d passos, queria %d", passos, MaxSkillLevel)
	}
}

func TestMatchClass(t *testing.T) {
	// Guerreiro (1) aprende skill de classe 1 e 2.
	if !MatchClass(1, 1) || !MatchClass(1, 2) {
		t.Error("guerreiro deveria aprender skill de guerreiro")
	}
	// E nao skill de mago.
	if MatchClass(1, 41) {
		t.Error("guerreiro nao deveria aprender skill de mago")
	}
	// Mago com skill de mago.
	if !MatchClass(41, 42) {
		t.Error("mago deveria aprender skill de mago")
	}
}

func TestIsPranSkill(t *testing.T) {
	for _, c := range []uint32{61, 70, 84} {
		if !IsPranSkill(c) {
			t.Errorf("classe %d deveria ser de pran", c)
		}
	}
	for _, c := range []uint32{0, 1, 60, 85, 100} {
		if IsPranSkill(c) {
			t.Errorf("classe %d nao deveria ser de pran", c)
		}
	}
}

func TestClassificacaoDeSkill(t *testing.T) {
	area := &gamedata.T_SkillData{SuccessRate: 1, Range: 5}
	single := &gamedata.T_SkillData{SuccessRate: 1, Range: 0}
	buff := &gamedata.T_SkillData{Duration: 30}

	if !IsAreaSkill(area) || IsAreaSkill(single) || IsAreaSkill(nil) {
		t.Error("IsAreaSkill errado")
	}
	if !IsSingleTargetDamage(single) || IsSingleTargetDamage(area) ||
		IsSingleTargetDamage(nil) {
		t.Error("IsSingleTargetDamage errado")
	}
	if !IsBuff(buff) || IsBuff(single) || IsBuff(nil) {
		t.Error("IsBuff errado")
	}
}

// A mascara de niveis e "2^(nivel+1) - 2": os bits 1 a nivel ligados.
func TestLevelMask(t *testing.T) {
	casos := map[uint16]uint16{
		0:  0,
		1:  0b10,
		2:  0b110,
		3:  0b1110,
		15: 0xFFFE,
	}
	for nivel, quer := range casos {
		if got := LevelMask(nivel); got != quer {
			t.Errorf("LevelMask(%d) = %#b, queria %#b", nivel, got, quer)
		}
	}
}

// Nivel acima do teto nao estoura os 16 bits do campo: na origem o valor
// passava de 65535 e o Move de 4 bytes escrevia por cima do slot vizinho.
func TestLevelMaskNaoEstoura(t *testing.T) {
	for _, nivel := range []uint16{16, 20, 100, 65535} {
		got := LevelMask(nivel)
		if got != 0xFFFE {
			t.Errorf("LevelMask(%d) = %#x, queria 0xFFFE", nivel, got)
		}
	}
}

func TestCooldownEBuffSeconds(t *testing.T) {
	tb := nova()
	tb.m[7] = gamedata.T_SkillData{Index: 7, Cooldown: 3000, Duration: 60}

	cd := CooldownMillis{T: tb}
	if got := cd.CooldownMillis(7); got != 3000 {
		t.Errorf("recarga = %d, queria 3000", got)
	}
	// Indice invalido responde zero em vez de ler fora do vetor.
	if got := cd.CooldownMillis(9999); got != 0 {
		t.Errorf("recarga de skill inexistente = %d", got)
	}

	bs := BuffSeconds{T: tb}
	if got := bs.BuffSeconds(7); got != 60 {
		t.Errorf("duracao = %d, queria 60", got)
	}
	if got := bs.BuffSeconds(9999); got != 0 {
		t.Errorf("duracao de skill inexistente = %d", got)
	}
}

func TestSkillDamageBonus(t *testing.T) {
	d := &gamedata.T_SkillData{Damage: 200}
	// "(Damage + HabAtk) div 2" da origem.
	if got := SkillDamageBonus(d, 100); got != 150 {
		t.Errorf("= %d, queria 150", got)
	}
	if got := SkillDamageBonus(nil, 100); got != 0 {
		t.Errorf("= %d, queria 0", got)
	}
}
