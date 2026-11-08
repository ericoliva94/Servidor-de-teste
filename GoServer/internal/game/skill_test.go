package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/skillfn"
)

// tabelaDeSkills responde do mapa.
type tabelaDeSkills struct{ m map[int]gamedata.T_SkillData }

func novaTabelaDeSkills() *tabelaDeSkills {
	return &tabelaDeSkills{m: map[int]gamedata.T_SkillData{}}
}

func (t *tabelaDeSkills) Skill(i int) *gamedata.T_SkillData {
	d, ok := t.m[i]
	if !ok {
		return nil
	}
	return &d
}

func (t *tabelaDeSkills) poe(index int, d gamedata.T_SkillData) {
	d.Index = uint32(index)
	t.m[index] = d
}

// handlerComSkills devolve um handler com tabela de skill e relogio parado.
func handlerComSkills(t *testing.T) (*Handler, *tabelaDeSkills, *time.Time) {
	h, _, agora := handlerDeCombate(t)
	tab := novaTabelaDeSkills()
	h.SkillTable = tab
	h.Parties = NewPartyRegistry()
	return h, tab, agora
}

func learnSkillPacket(index uint32) []byte {
	var p packets.LearnSkillPacket
	p.SkillIndex = index
	buf, err := netsrv.Marshal(CodeLearnSkill, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func useSkillPacket(skill, alvo uint32) []byte {
	var p packets.SendSkillUse
	p.Skill = skill
	p.Index = alvo
	buf, err := netsrv.Marshal(CodeUseSkill, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- aprender

func TestAprenderSkill(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		MinLevel: 10, SkillPoints: 1, LearnCosts: 100, Classe: 1})

	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Avatar.Level = 50
	s.Avatar.SkillPoint = 5
	s.Avatar.Gold = 1000
	s.Skills.Others[0] = model.SkillFromList{Index: 300}
	h.Chan.Join(e.conn, s)

	if err := h.learnSkill(context.Background(), e.conn, s,
		asPacket(t, learnSkillPacket(300))); err != nil {
		t.Fatal(err)
	}
	if s.Skills.Others[0].Level != 1 {
		t.Errorf("nivel = %d, queria 1", s.Skills.Others[0].Level)
	}
	if s.Avatar.SkillPoint != 4 {
		t.Errorf("pontos = %d, queria 4", s.Avatar.SkillPoint)
	}
	if s.Avatar.Gold != 900 {
		t.Errorf("ouro = %d, queria 900", s.Avatar.Gold)
	}
}

// O bug mais grave de LearnSkill: "if (SkillPoint = 1) then
// CurrentScore.Status := 0" apagava a reserva de pontos de ATRIBUTO ao
// aprender uma skill com exatamente 1 ponto de habilidade sobrando.
func TestAprenderComUmPontoNaoApagaOsAtributos(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SkillPoints: 1, Classe: 1})

	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Avatar.SkillPoint = 1
	s.Avatar.StatusPoints = 42
	s.Skills.Others[0] = model.SkillFromList{Index: 300}
	h.Chan.Join(e.conn, s)

	if err := h.learnSkill(context.Background(), e.conn, s,
		asPacket(t, learnSkillPacket(300))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.StatusPoints != 42 {
		t.Errorf("pontos de atributo = %d, queria 42: a origem zerava isso",
			s.Avatar.StatusPoints)
	}
	if s.Avatar.SkillPoint != 0 {
		t.Errorf("pontos de habilidade = %d, queria 0", s.Avatar.SkillPoint)
	}
}

// Indice fora da tabela nao quebra nem aprende.
func TestAprenderIndiceInvalido(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.SkillPoint = 10
	h.Chan.Join(e.conn, s)

	for _, i := range []uint32{0, 11999, 50000, 4000000000} {
		if err := h.learnSkill(context.Background(), e.conn, s,
			asPacket(t, learnSkillPacket(i))); err != nil {
			t.Fatalf("indice %d: %v", i, err)
		}
	}
	if s.Avatar.SkillPoint != 10 {
		t.Errorf("pontos = %d, nao deveria ter gasto", s.Avatar.SkillPoint)
	}
}

func TestAprenderRecusas(t *testing.T) {
	casos := []struct {
		nome   string
		d      gamedata.T_SkillData
		nivel  uint16
		pontos uint16
		ouro   int64
		classe uint16
	}{
		{"nivel baixo", gamedata.T_SkillData{MinLevel: 99, Classe: 1}, 10, 5, 1000, 1},
		{"sem pontos", gamedata.T_SkillData{SkillPoints: 5, Classe: 1}, 50, 1, 1000, 1},
		{"sem ouro", gamedata.T_SkillData{LearnCosts: 5000, Classe: 1}, 50, 5, 100, 1},
		{"outra classe", gamedata.T_SkillData{Classe: 41}, 50, 5, 1000, 1},
	}
	for _, c := range casos {
		h, tab, _ := handlerComSkills(t)
		tab.poe(300, c.d)
		e := novoEspiao(t)
		s := lutador("Alfa", 1000, 1000)
		s.Avatar.ClassInfo = c.classe
		s.Avatar.Level = c.nivel
		s.Avatar.SkillPoint = c.pontos
		s.Avatar.Gold = c.ouro
		s.Skills.Others[0] = model.SkillFromList{Index: 300}
		h.Chan.Join(e.conn, s)

		if err := h.learnSkill(context.Background(), e.conn, s,
			asPacket(t, learnSkillPacket(300))); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if s.Skills.Others[0].Level != 0 {
			t.Errorf("%s: aprendeu e nao deveria", c.nome)
		}
		if got := e.espera(t); got != CodeClientMsg {
			t.Errorf("%s: recebeu %#x, queria a mensagem", c.nome, got)
		}
	}
}

// A skill 427 e bloqueada pelo servidor.
func TestSkillBloqueada(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(skillfn.BlockedSkillIndex, gamedata.T_SkillData{Classe: 1})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Avatar.SkillPoint = 5
	s.Skills.Others[0] = model.SkillFromList{Index: skillfn.BlockedSkillIndex}
	h.Chan.Join(e.conn, s)

	if err := h.learnSkill(context.Background(), e.conn, s,
		asPacket(t, learnSkillPacket(skillfn.BlockedSkillIndex))); err != nil {
		t.Fatal(err)
	}
	if s.Skills.Others[0].Level != 0 {
		t.Error("nao deveria ter aprendido a skill bloqueada")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- zerar

func TestZerarSkillsDevolveOsPontos(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.SkillPoint = 0
	s.Skills.Basics[0] = model.SkillFromList{Index: 100, Level: 3}
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	s.Base.Cooldowns.Use(300, time.Now())
	h.Chan.Join(e.conn, s)

	var p packets.SignalData
	raw, _ := netsrv.Marshal(CodeResetSkills, p)
	if err := h.resetSkills(context.Background(), e.conn, s, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.SkillPoint != 8 {
		t.Errorf("pontos = %d, queria 8", s.Avatar.SkillPoint)
	}
	if s.Skills.Basics[0].Level != 0 || s.Skills.Others[0].Level != 0 {
		t.Error("os niveis deveriam ter zerado")
	}
	// O indice fica: so o nivel zera.
	if s.Skills.Others[0].Index != 300 {
		t.Errorf("indice = %d, deveria ficar", s.Skills.Others[0].Index)
	}
	if s.Base.Cooldowns.Count() != 0 {
		t.Error("as recargas deveriam ter sido limpas")
	}
}

// ---- usar

// A origem repassava o $320 aos visiveis na PRIMEIRA linha, antes de qualquer
// validacao: skill inexistente, de outra classe, sem MP ou em recarga ainda
// aparecia na tela de todo mundo.
func TestSkillInvalidaNaoApareceNaTela(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	b := lutador("Beta", 1001, 1000)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	for _, sk := range []uint32{0, 11999, 50000} {
		if err := h.useSkill(context.Background(), ea.conn, a,
			asPacket(t, useSkillPacket(sk, uint32(b.ClientID)))); err != nil {
			t.Fatalf("skill %d: %v", sk, err)
		}
	}
	eb.nada(t, 150*time.Millisecond)
}

// A skill tem de estar aprendida: a origem nao conferia.
func TestUsarSkillNaoAprendida(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SuccessRate: 1, Classe: 1, MP: 10})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 0))); err != nil {
		t.Fatal(err)
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
	// E nao gastou MP.
	if s.Avatar.CurMP != s.Avatar.MaxMP {
		t.Errorf("MP = %d, nao deveria ter gasto", s.Avatar.CurMP)
	}
}

// Skill de outra classe tambem e recusada no uso, nao so ao aprender.
func TestUsarSkillDeOutraClasse(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SuccessRate: 1, Classe: 41})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 0))); err != nil {
		t.Fatal(err)
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

func TestSkillDeDanoNumJogador(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		SuccessRate: 1, Classe: 1, MP: 20, Damage: 500, DamageRange: 10})
	h.Rand = randFixo{v: 50}

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	b := lutador("Beta", 1002, 1000)
	a.Avatar.ClassInfo = 1
	a.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	mpAntes := a.Avatar.CurMP
	hpAntes := b.Avatar.CurHP
	if err := h.useSkill(context.Background(), ea.conn, a,
		asPacket(t, useSkillPacket(300, uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if a.Avatar.CurMP != mpAntes-20 {
		t.Errorf("MP = %d, queria %d", a.Avatar.CurMP, mpAntes-20)
	}
	if b.Avatar.CurHP >= hpAntes {
		t.Errorf("vida do alvo = %d, antes %d", b.Avatar.CurHP, hpAntes)
	}
	// O alvo recebe o uso da skill e o dano.
	codigos := map[uint16]bool{}
	for range 6 {
		select {
		case p := <-eb.pacotes:
			codigos[p.code] = true
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !codigos[CodeUseSkill] {
		t.Error("o alvo deveria ver o uso da skill")
	}
	if !codigos[CodeRecvDamage] {
		t.Error("o alvo deveria receber o dano")
	}
}

// Sem MP a skill nao sai e nada e descontado.
func TestSkillSemMP(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SuccessRate: 1, Classe: 1, MP: 1000})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Avatar.CurMP = 10
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 0))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.CurMP != 10 {
		t.Errorf("MP = %d, queria 10", s.Avatar.CurMP)
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
	// A recarga nao foi marcada: a skill nao saiu.
	if s.Base.Cooldowns.Count() != 0 {
		t.Error("a recarga nao deveria ter sido marcada")
	}
}

// A recarga vale, e vem da tabela.
func TestSkillRespeitaARecarga(t *testing.T) {
	h, tab, agora := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		SuccessRate: 1, Classe: 1, Cooldown: 5000, DamageRange: 10})
	h.Rand = randFixo{v: 50}

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	b := lutador("Beta", 1002, 1000)
	a.Avatar.ClassInfo = 1
	a.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	lanca := func() uint32 {
		antes := b.Avatar.CurHP
		if err := h.useSkill(context.Background(), ea.conn, a,
			asPacket(t, useSkillPacket(300, uint32(b.ClientID)))); err != nil {
			t.Fatal(err)
		}
		return antes - b.Avatar.CurHP
	}

	if lanca() == 0 {
		t.Fatal("a primeira skill deveria ter acertado")
	}
	if got := lanca(); got != 0 {
		t.Errorf("a segunda tirou %d e deveria estar em recarga", got)
	}
	*agora = agora.Add(6 * time.Second)
	if lanca() == 0 {
		t.Error("passada a recarga deveria acertar")
	}
}

// Skill de pran ainda avisa em vez de sair (a de area foi portada: ver skillarea_test.go).
func TestSkillDePranAvisa(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(400, gamedata.T_SkillData{SuccessRate: 1, Classe: 61})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Skills.Others[1] = model.SkillFromList{Index: 400, Level: 5}
	h.Chan.Join(e.conn, s)

	for _, sk := range []uint32{400} {
		if err := h.useSkill(context.Background(), e.conn, s,
			asPacket(t, useSkillPacket(sk, 0))); err != nil {
			t.Fatalf("skill %d: %v", sk, err)
		}
		if got := e.espera(t); got != CodeClientMsg {
			t.Errorf("skill %d: recebeu %#x, queria a mensagem", sk, got)
		}
	}
}

// Skill de buff em si mesmo.
func TestSkillDeBuffEmSiMesmo(t *testing.T) {
	h, tab, agora := handlerComSkills(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, Classe: 1, MP: 10})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Skills.Others[0] = model.SkillFromList{Index: 500, Level: 5}
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(500, 0))); err != nil {
		t.Fatal(err)
	}
	if !s.Base.Buffs.Has(500) {
		t.Error("o buff deveria estar ativo")
	}
	inicio, ok := s.Base.Buffs.StartedAt(500)
	if !ok || !inicio.Equal(*agora) {
		t.Errorf("inicio do buff = %v, queria %v", inicio, *agora)
	}
}

// Buff num companheiro perto.
func TestSkillDeBuffNoAlvo(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, Classe: 1, DamageRange: 10})
	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	b := lutador("Beta", 1002, 1000)
	a.Avatar.ClassInfo = 1
	a.Skills.Others[0] = model.SkillFromList{Index: 500, Level: 5}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	if err := h.useSkill(context.Background(), ea.conn, a,
		asPacket(t, useSkillPacket(500, uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if !b.Base.Buffs.Has(500) {
		t.Error("o alvo deveria ter o buff")
	}
	if a.Base.Buffs.Has(500) {
		t.Error("o autor nao deveria ter o buff")
	}
}

// Alvo longe nao recebe o buff: o autor fica com ele.
func TestBuffEmAlvoLongeVoltaAoAutor(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, Classe: 1, DamageRange: 5})
	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	b := lutador("Beta", 5000, 5000)
	a.Avatar.ClassInfo = 1
	a.Skills.Others[0] = model.SkillFromList{Index: 500, Level: 5}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	if err := h.useSkill(context.Background(), ea.conn, a,
		asPacket(t, useSkillPacket(500, uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if b.Base.Buffs.Has(500) {
		t.Error("o alvo longe nao deveria ter o buff")
	}
	if !a.Base.Buffs.Has(500) {
		t.Error("o autor deveria ficar com o buff")
	}
}

// Skill de dano num mob.
func TestSkillDeDanoNumMob(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		SuccessRate: 1, Classe: 1, Damage: 500, DamageRange: 10})
	h.Rand = randFixo{v: 50}
	h.Mobs = NewMobWorld(tabelaDeMobs(especie("Bicho", 100000, 1, 1000, 1000)))
	h.Exp = expFixa{}

	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(3048)

	m, ok := h.Mobs.Get(3048)
	if !ok {
		t.Fatal("o mob deveria existir")
	}
	antes := m.CurHP
	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 3048))); err != nil {
		t.Fatal(err)
	}
	if m.CurHP >= antes {
		t.Errorf("vida do mob = %d, antes %d", m.CurHP, antes)
	}
}

// Morto nao usa skill.
func TestMortoNaoUsaSkill(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SuccessRate: 1, Classe: 1})
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Dead = true
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 0))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.CurHP != 0 {
		t.Errorf("HP = %d, queria 0", s.Avatar.CurHP)
	}
}

// Classes 2 e 3 precisam de bala, e ela e gasta.
func TestClasseComBalaGastaMunicao(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		SuccessRate: 1, Classe: 21, Damage: 100, DamageRange: 10})
	itens := novaTabelaDeItens()
	itens.poeItem(7000, gamedata.ItemFromList{ItemType: 50, CanAgroup: 1})
	h.Data = itens
	h.Rand = randFixo{v: 50}

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Arqueiro", 1000, 1000)
	b := lutador("Beta", 1002, 1000)
	a.Avatar.ClassInfo = 21
	a.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	a.Equip.Slots[BulletSlot] = model.Item{Index: 7000, Refi: 50}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	if err := h.useSkill(context.Background(), ea.conn, a,
		asPacket(t, useSkillPacket(300, uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if got := a.Equip.Get(BulletSlot).Refi; got != 49 {
		t.Errorf("balas = %d, queria 49", got)
	}
}

// Sem bala a skill nao sai.
func TestClasseComBalaSemMunicao(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{SuccessRate: 1, Classe: 21})
	e := novoEspiao(t)
	s := lutador("Arqueiro", 1000, 1000)
	s.Avatar.ClassInfo = 21
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	s.Equip.Slots[BulletSlot] = model.Item{}
	h.Chan.Join(e.conn, s)

	if err := h.useSkill(context.Background(), e.conn, s,
		asPacket(t, useSkillPacket(300, 0))); err != nil {
		t.Fatal(err)
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// Bala do tipo infinito nao se gasta.
func TestBalaInfinitaNaoGasta(t *testing.T) {
	h, tab, _ := handlerComSkills(t)
	tab.poe(300, gamedata.T_SkillData{
		SuccessRate: 1, Classe: 21, DamageRange: 10})
	itens := novaTabelaDeItens()
	itens.poeItem(7001, gamedata.ItemFromList{ItemType: ItemTypeInfiniteBullet})
	h.Data = itens
	h.Rand = randFixo{v: 50}

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Arqueiro", 1000, 1000)
	b := lutador("Beta", 1002, 1000)
	a.Avatar.ClassInfo = 21
	a.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	a.Equip.Slots[BulletSlot] = model.Item{Index: 7001, Refi: 1}
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	if err := h.useSkill(context.Background(), ea.conn, a,
		asPacket(t, useSkillPacket(300, uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if got := a.Equip.Get(BulletSlot).Refi; got != 1 {
		t.Errorf("balas = %d, a infinita nao deveria gastar", got)
	}
}

// ---- cancelar

func TestCancelarLancamento(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.UsingSkill = 300
	h.Chan.Join(e.conn, s)

	var p packets.SignalData
	raw, _ := netsrv.Marshal(CodeCancelSkill, p)
	if err := h.cancelSkill(context.Background(), e.conn, s, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if s.UsingSkill != 0 {
		t.Errorf("UsingSkill = %d, queria 0", s.UsingSkill)
	}
}

// ---- pacotes de lista

// O $107 ia com a mascara ZERADA na origem: o SendPacket vinha antes do Move
// que preenchia o vetor, e o Move escrevia num registro local ja enviado.
func TestPacoteDeNiveisLevaAMascara(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.SkillPoint = 7
	s.Skills.Basics[0] = model.SkillFromList{Index: 100, Level: 2}
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 3}
	h.Chan.Join(e.conn, s)

	h.sendSkillLevels(e.conn, s)
	vis := e.proximo(t)
	if vis.code != CodeSkillLevels {
		t.Fatalf("codigo %#x, queria %#x", vis.code, CodeSkillLevels)
	}
	var pkt packets.SendSkillsLevelPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.SkillPoints != 7 {
		t.Errorf("pontos = %d, queria 7", pkt.SkillPoints)
	}
	// A basica leva o valor fixo 2 e a "Others" leva a mascara.
	if pkt.Skills[0] != 2 {
		t.Errorf("basica = %d, queria 2", pkt.Skills[0])
	}
	if quer := skillfn.LevelMask(3); pkt.Skills[6] != quer {
		t.Errorf("mascara = %#x, queria %#x", pkt.Skills[6], quer)
	}
}

// Slot com nivel zero nao entra na mascara.
func TestMascaraIgnoraNivelZero(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	s := lutador("Alfa", 1000, 1000)
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 0}

	out := make([]uint16, 60)
	h.fillSkillMask(s, out)
	for i, v := range out {
		if v != 0 {
			t.Errorf("slot %d = %d, tudo deveria estar zerado", i, v)
		}
	}
}

// A mascara nao escreve fora do vetor que recebe.
func TestMascaraNaoEstouraOVetor(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	s := lutador("Alfa", 1000, 1000)
	for i := range s.Skills.Others {
		s.Skills.Others[i] = model.SkillFromList{Index: uint16(300 + i*16), Level: 5}
	}
	// Vetor curto: nao pode entrar em panico.
	h.fillSkillMask(s, make([]uint16, 3))
	h.fillSkillMask(s, nil)
}

func TestPacoteDeSkillsLevaOsIndices(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 1}
	s.Skills.Others[5] = model.SkillFromList{Index: 800, Level: 2}
	h.Chan.Join(e.conn, s)

	h.sendPlayerSkills(e.conn, s, 0)
	vis := e.proximo(t)
	if vis.code != CodePlayerSkills {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.SendSkillsPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.Skills[0] != 300 || pkt.Skills[5] != 800 {
		t.Errorf("indices = %d/%d", pkt.Skills[0], pkt.Skills[5])
	}
	// Com NPC o SendType vira $B, como na origem.
	h.sendPlayerSkills(e.conn, s, 42)
	netsrv.Unmarshal(asPacote(t, e.proximo(t).corpo), &pkt)
	if pkt.NPCIndex != 42 || pkt.SendType != 0xB {
		t.Errorf("NPC = %d, SendType = %#x", pkt.NPCIndex, pkt.SendType)
	}
}

// Usar skill sem estar em jogo e recusado.
func TestSkillExigeEstarEmJogo(t *testing.T) {
	h, _, _ := handlerComSkills(t)
	s := sessaoAutenticada()
	for _, f := range []func() error{
		func() error {
			return h.useSkill(context.Background(), connDescartavel(t), s,
				asPacket(t, useSkillPacket(300, 0)))
		},
		func() error {
			return h.learnSkill(context.Background(), connDescartavel(t), s,
				asPacket(t, learnSkillPacket(300)))
		},
	} {
		if err := f(); err != ErrNotPlaying {
			t.Errorf("erro = %v, queria ErrNotPlaying", err)
		}
	}
}
