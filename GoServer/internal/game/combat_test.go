package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/combat"
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/stats"
)

// randFixo devolve sempre o mesmo valor, para o teste nao depender de sorteio.
type randFixo struct{ v int }

func (r randFixo) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	if r.v >= n {
		return n - 1
	}
	return r.v
}

// atkPacket monta um $302.
func atkPacket(alvo, skill uint16) []byte {
	var p packets.SendAtkPacket
	p.Index = alvo
	p.Skill = skill
	buf, err := netsrv.Marshal(CodeAttack, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// lutador devolve uma sessao armada e com atributos.
func lutador(nome string, x, y float32) *Session {
	s := sessaoComInventario()
	s.Avatar.Name = nome
	s.Position = model.Position{X: x, Y: y}
	s.AnchorPosition = s.Position
	s.Avatar.Str = 100
	s.Avatar.Agility = 100
	s.Avatar.Cons = 100
	s.Avatar.Luck = 50
	s.Avatar.Int = 50
	s.Avatar.Level = 50
	s.Avatar.Recalc(stats.Bonus{})
	s.Avatar.CurHP = s.Avatar.MaxHP
	s.Avatar.CurMP = s.Avatar.MaxMP
	// Arma no slot 6, como a origem exige.
	s.Equip.Slots[WeaponSlot] = model.Item{Index: 1000, Min: 100, Max: 100, Refi: 16}
	return s
}

// handlerDeCombate devolve um handler com tabela, canal e sorteio fixo.
func handlerDeCombate(t *testing.T) (*Handler, *tabelaDeItens, *time.Time) {
	tab := novaTabelaDeItens()
	tab.poeItem(1000, gamedata.ItemFromList{ItemType: 6})
	agora := time.Now()
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	h.Data = tab
	h.Rand = randFixo{v: 0}
	h.Now = func() time.Time { return agora }
	return h, tab, &agora
}

// ---- atributos derivados

// O dano nao pode sair zero: sem GetCurrentScore o DNFis ficava em zero e
// todo golpe batia 1.
func TestAtributosDerivadosSaemDosPontos(t *testing.T) {
	var a Avatar
	a.ClassInfo = 1
	a.Level = 50
	a.Str = 100
	a.Agility = 100
	a.Cons = 100
	a.Luck = 100
	a.Int = 100
	a.Recalc(stats.Bonus{})

	if a.AtkFis == 0 {
		t.Error("ataque fisico zerado")
	}
	if a.AtkMag == 0 {
		t.Error("ataque magico zerado")
	}
	if a.Critical == 0 || a.DuploAtk == 0 {
		t.Errorf("critico/duplo = %d/%d", a.Critical, a.DuploAtk)
	}
	if a.Acerto == 0 {
		t.Error("acerto zerado")
	}
	if a.MaxHP <= 1 {
		t.Errorf("MaxHP = %d", a.MaxHP)
	}
}

// Os valores seguem as constantes da origem: 2.6 por forca e por agilidade no
// dano fisico, 3.2 por inteligencia no magico.
func TestConstantesDoDanoSaoAsDaOrigem(t *testing.T) {
	d := stats.Compute(stats.Score{Str: 100, Agility: 100, Int: 100}, stats.Bonus{})
	if quer := uint16(int(100*2.6) + int(100*2.6)); d.AtkFis != quer {
		t.Errorf("AtkFis = %d, queria %d", d.AtkFis, quer)
	}
	if quer := uint16(int(100 * 3.2)); d.AtkMag != quer {
		t.Errorf("AtkMag = %d, queria %d", d.AtkMag, quer)
	}
}

// A velocidade gravada no banco nao e apagada pelo recalculo.
func TestRecalculoNaoApagaVelocidadeGravada(t *testing.T) {
	var a Avatar
	a.ClassInfo = 1
	a.SpeedMove = 33
	a.Recalc(stats.Bonus{})
	if a.SpeedMove != 33 {
		t.Errorf("SpeedMove = %d, queria 33", a.SpeedMove)
	}
	// Zero cede ao valor calculado.
	a.SpeedMove = 0
	a.Recalc(stats.Bonus{})
	if a.SpeedMove == 0 {
		t.Error("SpeedMove deveria ter vindo da conta")
	}
}

// Os limites de Util.pas valem: nenhum campo passa do teto.
func TestLimitesDosAtributos(t *testing.T) {
	d := stats.Compute(stats.Score{
		Str: 65535, Agility: 65535, Int: 65535, Cons: 65535, Luck: 65535,
	}, stats.Bonus{
		Critical: 100000, Double: 100000, Acerto: 100000, Esquiva: 100000,
		Cooldown: 100000, SpeedMove: 100000,
	})
	if d.Critical > 255 || d.DuploAtk > 255 {
		t.Errorf("critico/duplo passaram de 255: %d/%d", d.Critical, d.DuploAtk)
	}
	if d.ReduceCooldown > 70 {
		t.Errorf("cooldown = %d, teto 70", d.ReduceCooldown)
	}
	if d.SpeedMove > 70 {
		t.Errorf("velocidade = %d, teto 70", d.SpeedMove)
	}
	if d.Acerto == 0 || d.Esquiva == 0 {
		t.Error("acerto e esquiva nao deveriam ser zero")
	}
}

// EF_UNARMOR zera as duas defesas, como na origem.
func TestUnarmorZeraDefesas(t *testing.T) {
	d := stats.Compute(stats.Score{Cons: 100}, stats.Bonus{
		DefFis: 500, DefMag: 400, Unarmor: true})
	if d.DefFis != 0 || d.DefMag != 0 {
		t.Errorf("defesas = %d/%d, queria 0/0", d.DefFis, d.DefMag)
	}
}

// ---- ataque

func TestAtaqueBasicoTiraVida(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 102, 100)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), ea.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if b.Avatar.CurHP >= antes {
		t.Errorf("vida do alvo = %d, antes %d", b.Avatar.CurHP, antes)
	}

	// Os dois recebem o $102.
	for nome, e := range map[string]*espiao{"autor": ea, "alvo": eb} {
		if got := e.espera(t); got != CodeRecvDamage {
			t.Errorf("%s recebeu %#x, queria %#x", nome, got, CodeRecvDamage)
		}
	}
}

// O dano nao sai 1: e a prova de que os derivados chegaram ao calculo.
func TestDanoNaoSaiMinimo(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	b.Avatar.DefFis = 0
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	dano := antes - b.Avatar.CurHP
	if dano < 10 {
		t.Errorf("dano = %d, esperava bem mais que o minimo", dano)
	}
}

// Sem arma no slot 6 nao ha golpe, como na origem.
func TestAtaqueExigeArma(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	a.Equip.Slots[WeaponSlot] = model.Item{}
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if b.Avatar.CurHP != antes {
		t.Error("nao deveria ter atacado sem arma")
	}
	e.nada(t, 150*time.Millisecond)
}

// Alvo longe nao pode ser atingido: a origem aceitava qualquer ClientID de
// qualquer ponto do mapa.
func TestAtaqueExigeAlvoPerto(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 5000, 5000)
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if b.Avatar.CurHP != antes {
		t.Errorf("alvo longe levou dano: %d -> %d", antes, b.Avatar.CurHP)
	}
}

// Indice de alvo fora da faixa nao quebra nem acerta ninguem.
func TestAtaqueComIndiceInvalido(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, a)

	for _, idx := range []uint16{0, 11, 1000, 65535} {
		if err := h.attack(context.Background(), e.conn, a,
			asPacket(t, atkPacket(idx, 0))); err != nil {
			t.Fatalf("indice %d: %v", idx, err)
		}
	}
	e.nada(t, 150*time.Millisecond)
}

// Atacar a si mesmo nao se aplica no ataque basico.
func TestAtaqueASiMesmo(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, a)

	antes := a.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(a.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if a.Avatar.CurHP != antes {
		t.Error("nao deveria ter batido em si mesmo")
	}
}

// O ritmo minimo entre golpes vale.
func TestRitmoMinimoEntreGolpes(t *testing.T) {
	h, _, agora := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	golpe := func() uint32 {
		antes := b.Avatar.CurHP
		if err := h.attack(context.Background(), e.conn, a,
			asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
			t.Fatal(err)
		}
		return antes - b.Avatar.CurHP
	}

	if golpe() == 0 {
		t.Fatal("o primeiro golpe deveria ter acertado")
	}
	if got := golpe(); got != 0 {
		t.Errorf("o segundo golpe imediato tirou %d e deveria ser descartado", got)
	}
	*agora = agora.Add(MinDelayAttack + time.Millisecond)
	if golpe() == 0 {
		t.Error("passado o intervalo o golpe deveria acertar")
	}
}

// Golpe com skill nao cai no ataque basico: a skill sairia de graca.
func TestAtaqueComSkillERecusado(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 123))); err != nil {
		t.Fatal(err)
	}
	if b.Avatar.CurHP != antes {
		t.Error("skill nao deveria ter batido")
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// Skill com indice absurdo tambem e recusada: a origem fazia
// "DataSkill := @SkillData[Packet.Skill]" num ARRAY [0..11998].
func TestSkillComIndiceAbsurdoNaoQuebra(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	for _, sk := range []uint16{11999, 50000, 65535} {
		if err := h.attack(context.Background(), e.conn, a,
			asPacket(t, atkPacket(b.ClientID, sk))); err != nil {
			t.Fatalf("skill %d: %v", sk, err)
		}
	}
}

// Morto nao ataca, e o servidor reenvia o HP zerado.
func TestMortoNaoAtaca(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	a.Dead = true
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	antes := b.Avatar.CurHP
	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if b.Avatar.CurHP != antes {
		t.Error("morto nao deveria bater")
	}
	if a.Avatar.CurHP != 0 {
		t.Errorf("HP do morto = %d, queria 0", a.Avatar.CurHP)
	}
	if got := e.espera(t); got != CodeCurrentHPMP {
		t.Errorf("recebeu %#x, queria %#x", got, CodeCurrentHPMP)
	}
}

// Alvo morto nao e atacavel.
func TestAlvoMortoNaoEAtacavel(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	b.Dead = true
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), b)

	if err := h.attack(context.Background(), e.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	e.nada(t, 150*time.Millisecond)
}

// Golpe suficiente mata e o estado fica coerente.
func TestGolpeFatalMarcaAMorte(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	ea, eb := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 100, 100)
	b := lutador("Beta", 101, 100)
	b.Avatar.CurHP = 1
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	if err := h.attack(context.Background(), ea.conn, a,
		asPacket(t, atkPacket(b.ClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if !b.Dead {
		t.Error("o alvo deveria estar morto")
	}
	if b.Avatar.CurHP != 0 {
		t.Errorf("HP = %d, queria 0", b.Avatar.CurHP)
	}
}

// Atacar antes de entrar no mundo e recusado.
func TestAtacarExigeEstarEmJogo(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	s := sessaoAutenticada()
	if err := h.attack(context.Background(), connDescartavel(t), s,
		asPacket(t, atkPacket(1, 0))); err != ErrNotPlaying {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

// ---- desgaste da arma

// O bug: "Dec(Equip[6].MIN, 1)" num BYTE sem piso. Arma com durabilidade zero
// dava a volta para 255 e voltava a funcionar sozinha.
func TestDesgasteDaArmaNaoDaVolta(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[WeaponSlot].Min = 0
	s.Equip.Slots[WeaponSlot].Refi = 200 // perda de 3 por ciclo

	for range 3 {
		s.AttacksAccumulated = AttacksPerDurabilityLoss - 1
		h.wearWeapon(e.conn, s)
		if got := s.Equip.Get(WeaponSlot).Min; got != 0 {
			t.Fatalf("durabilidade = %d, queria 0", got)
		}
	}
}

// A perda cresce com o refino, nas faixas da origem.
func TestPerdaDeDurabilidadePorFaixaDeRefino(t *testing.T) {
	casos := []struct {
		refi uint16
		quer uint8
	}{
		{0, 0}, // sem refino nao desgasta
		{1, 1}, {80, 1},
		{81, 2}, {160, 2},
		{161, 3}, {240, 3},
	}
	for _, c := range casos {
		h, _, _ := handlerDeCombate(t)
		e := novoEspiao(t)
		s := lutador("Alfa", 100, 100)
		h.Chan.Join(e.conn, s)
		s.Equip.Slots[WeaponSlot].Min = 100
		s.Equip.Slots[WeaponSlot].Refi = c.refi
		s.AttacksAccumulated = AttacksPerDurabilityLoss - 1

		h.wearWeapon(e.conn, s)
		perdeu := 100 - s.Equip.Get(WeaponSlot).Min
		if perdeu != c.quer {
			t.Errorf("refino %d perdeu %d, queria %d", c.refi, perdeu, c.quer)
		}
	}
}

// O contador so dispara no 48o golpe.
func TestDesgasteSoAcadaQuarentaEOito(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, s)
	s.Equip.Slots[WeaponSlot].Min = 100

	for i := range AttacksPerDurabilityLoss - 1 {
		h.wearWeapon(e.conn, s)
		if got := s.Equip.Get(WeaponSlot).Min; got != 100 {
			t.Fatalf("golpe %d: durabilidade %d, queria 100", i, got)
		}
	}
	h.wearWeapon(e.conn, s)
	if got := s.Equip.Get(WeaponSlot).Min; got != 99 {
		t.Errorf("durabilidade = %d, queria 99", got)
	}
	if s.AttacksAccumulated != 0 {
		t.Errorf("contador = %d, queria 0", s.AttacksAccumulated)
	}
}

// ---- reviver

func TestReviverDevolveAVida(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, s)
	s.Dead = true
	s.Avatar.CurHP = 0
	s.Avatar.CurMP = 0

	var p packets.SignalData
	raw, err := netsrv.Marshal(CodeRevive, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.revive(context.Background(), e.conn, s, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if s.Dead {
		t.Error("deveria ter revivido")
	}
	if s.Avatar.CurHP != s.Avatar.MaxHP {
		t.Errorf("HP = %d, queria %d", s.Avatar.CurHP, s.Avatar.MaxHP)
	}
	if s.Position != StartPosition {
		t.Errorf("posicao = %v, queria %v", s.Position, StartPosition)
	}
}

// Reviver quem nao morreu nao enche o HP de graca.
func TestReviverVivoNaoFazNada(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, s)
	s.Avatar.CurHP = 10

	var p packets.SignalData
	raw, err := netsrv.Marshal(CodeRevive, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.revive(context.Background(), e.conn, s, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.CurHP != 10 {
		t.Errorf("HP = %d, queria 10", s.Avatar.CurHP)
	}
	e.nada(t, 150*time.Millisecond)
}

// ---- sessionTarget

// A mensagem de absorcao sai pela conexao do jogador, nao por um indice de
// vetor: na origem era Players[mob.ClientID], com mob podendo ser um mob.
func TestSessionTargetEntregaMensagem(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	e := novoEspiao(t)
	s := lutador("Alfa", 100, 100)
	h.Chan.Join(e.conn, s)

	alvo := &sessionTarget{s: s, h: h}
	alvo.Notify("teste")
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

func TestSessionTargetHPEMP(t *testing.T) {
	h, _, _ := handlerDeCombate(t)
	s := lutador("Alfa", 100, 100)
	alvo := &sessionTarget{s: s, h: h}

	s.Avatar.CurHP = 100
	alvo.AddHP(50)
	if s.Avatar.CurHP != 150 {
		t.Errorf("HP = %d, queria 150", s.Avatar.CurHP)
	}
	// Nao passa do maximo.
	alvo.AddHP(int(s.Avatar.MaxHP) * 2)
	if s.Avatar.CurHP != s.Avatar.MaxHP {
		t.Errorf("HP = %d, queria o maximo %d", s.Avatar.CurHP, s.Avatar.MaxHP)
	}
	// Valor negativo tira vida.
	alvo.AddHP(-int(s.Avatar.MaxHP))
	if s.Avatar.CurHP != 0 || !s.Dead {
		t.Errorf("HP = %d, morto = %v", s.Avatar.CurHP, s.Dead)
	}

	s.Avatar.CurMP = 100
	alvo.RemoveMP(30)
	if s.Avatar.CurMP != 70 {
		t.Errorf("MP = %d, queria 70", s.Avatar.CurMP)
	}
	// Nao da a volta.
	alvo.RemoveMP(1000)
	if s.Avatar.CurMP != 0 {
		t.Errorf("MP = %d, queria 0", s.Avatar.CurMP)
	}
}

func TestSessionTargetEhJogador(t *testing.T) {
	alvo := &sessionTarget{s: lutador("Alfa", 0, 0)}
	if !alvo.IsPlayer() {
		t.Error("a casca envolve sessao de jogador")
	}
	if alvo.Ability(combat.EFAmpPhysical) != 0 {
		t.Error("habilidade deveria ser zero nesta fase")
	}
}
