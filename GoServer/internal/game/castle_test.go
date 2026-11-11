package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/devir"
	"aika/internal/model"
	"aika/internal/siege"
)

// A segunda-feira 9/11/2026 vale para o cadastro; o sabado 14/11 20h, para a guerra.
var (
	segunda = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	sabado  = time.Date(2026, 11, 14, 20, 30, 0, 0, time.UTC)
)

type cenarioCerco struct {
	*cenarioNacao
	agora *time.Time
	beta  *guildaDeTeste
}

// cercoPronto monta a Alfa (marechal e defesa) e a Beta (atacante, de outro lider).
func cercoPronto(t *testing.T) *cenarioCerco {
	t.Helper()
	c := novoCenarioNacao(t)
	h := c.h
	h.Devir = devir.New(1)
	h.Siege = &siege.War{}
	h.Siege.SetCerco(siege.Cerco{Defenders: siege.Alliance{Marshal: "Alfa"}})
	agora := segunda
	h.Now = func() time.Time { return agora }
	beta := c.novaGuilda(t, "Beta", 1, 21)
	return &cenarioCerco{cenarioNacao: c, agora: &agora, beta: beta}
}

func TestSignInCastleRules(t *testing.T) {
	c := cercoPronto(t)
	h := c.h
	if h.signInCastle(c.membro) {
		t.Error("membro comum nao cadastra")
	}
	*c.agora = sabado
	if h.signInCastle(c.beta.s) {
		t.Error("sabado nao cadastra")
	}
	*c.agora = segunda
	if h.signInCastle(c.lider) {
		t.Error("a defesa nao se cadastra como atacante")
	}
	if !h.signInCastle(c.beta.s) {
		t.Fatal("a Beta deveria conseguir")
	}
	if h.signInCastle(c.beta.s) {
		t.Error("cadastro repetido")
	}
	if got := h.Siege.Cerco().Attackers[3].Marshal; got != "Beta" {
		t.Errorf("a Beta ficou em %+v", h.Siege.Cerco().Attackers)
	}
}

func TestEnterCastleOnlyRegisteredAndOnlyOnSaturday(t *testing.T) {
	c := cercoPronto(t)
	h := c.h
	if h.enterCastle(c.beta.s) {
		t.Error("sem cadastro nao entra")
	}
	h.signInCastle(c.beta.s)
	if h.enterCastle(c.beta.s) {
		t.Error("segunda: a guerra nao pode comecar")
	}
	*c.agora = sabado
	if !h.enterCastle(c.beta.s) {
		t.Fatal("sabado 20h o lider da alianca deveria abrir a guerra e entrar")
	}
	want := siege.AttackEntry[3]
	if !h.Siege.InProgress() || !c.beta.s.Siege.InCastle || c.beta.s.Position.X != want.X || c.beta.s.Siege.CastleNation != want.Nation {
		t.Errorf("guerra=%v castelo=%v pos=%+v", h.Siege.InProgress(), c.beta.s.Siege.InCastle, c.beta.s.Position)
	}
	// A defesa entra pelo ponto do marechal; quem nao e lider nao abre guerra, mas entra com ela aberta.
	if !h.enterCastle(c.lider) || c.lider.Position.X != siege.DefenseMarshal.X {
		t.Errorf("defesa: %+v", c.lider.Position)
	}
}

func TestEnterCastleBeforeWarStarts(t *testing.T) {
	c := cercoPronto(t)
	c.h.SiegeAnyTime = true
	c.h.signInCastle(c.beta.s)
	if c.h.enterCastle(c.membro) { // membro da defesa, guerra fechada
		t.Error("membro nao abre guerra")
	}
	if !c.h.enterCastle(c.beta.s) {
		t.Error("com SiegeAnyTime o lider abre a guerra a qualquer hora")
	}
}

// tomaOsPontos poe o atacante nas tres orbes e na bandeira.
func (c *cenarioCerco) tomaOsPontos(t *testing.T) {
	t.Helper()
	h := c.h
	h.SiegeAnyTime = true
	h.signInCastle(c.beta.s)
	if !h.enterCastle(c.beta.s) {
		t.Fatal("nao entrou")
	}
	holders := []*Session{c.outro, c.lider, c.membro}
	for i, s := range holders {
		s.Siege.InCastle = true
		p := siege.OrbPos[i]
		s.Position = model.Position{X: p[0], Y: p[1]}
		h.castleCollect(h.connOf(s), s, uint32(castleOrbWater+i))
	}
	if h.Siege.OrbsHeld() != 3 {
		t.Fatalf("orbes seguras = %d", h.Siege.OrbsHeld())
	}
	c.beta.s.Position = model.Position{X: siege.SealPos[0], Y: siege.SealPos[1]}
	h.castleCollect(c.beta.e.conn, c.beta.s, castleSealFlag)
	if h.Siege.Status() != siege.StatusSealing {
		t.Fatalf("status = %v", h.Siege.Status())
	}
}

func TestSealHeldWinsAndCrownsTheAlliance(t *testing.T) {
	c := cercoPronto(t)
	c.tomaOsPontos(t)
	h := c.h
	// Quem entra tem de poder voltar: guarda a posicao de origem.
	c.membro.Siege.SavedPos, c.membro.Siege.HasSavedPos = model.Position{X: 1111, Y: 1111}, true
	// O selo fica seguro o tempo todo? A orbe 1 e 2 tem de continuar com os donos:
	// a posicao deles esta nas orbes, entao o tick nao os solta.
	*c.agora = c.agora.Add(siege.SealDuration + time.Second)
	if !h.SiegeTick() {
		t.Fatal("a guerra deveria acabar com o selo mantido")
	}
	st, _ := h.Nations.Get(1)
	if st.MarshalGuild != c.beta.g.Index {
		t.Errorf("marechal = %d, queria a Beta (%d)", st.MarshalGuild, c.beta.g.Index)
	}
	if h.Siege.InProgress() || !h.Siege.Cerco().Defenders.Has("Beta") {
		t.Errorf("cerco = %+v", h.Siege.Cerco())
	}
	if c.membro.Siege.InCastle || c.membro.Position.X != 1111 {
		t.Errorf("quem estava no castelo volta de onde veio: %+v", c.membro.Position)
	}
	if c.lider.Position.X != castleExitDefault.X {
		t.Errorf("sem posicao salva vai para (3450, 690): %+v", c.lider.Position)
	}
}

func TestWarTimeoutKeepsDefense(t *testing.T) {
	c := cercoPronto(t)
	c.h.SiegeAnyTime = true
	c.h.signInCastle(c.beta.s)
	c.h.enterCastle(c.beta.s)
	*c.agora = c.agora.Add(siege.WarDuration)
	if !c.h.SiegeTick() {
		t.Fatal("a guerra deveria acabar")
	}
	st, _ := c.h.Nations.Get(1)
	if st.MarshalGuild != c.alfa.Index {
		t.Errorf("marechal = %d, a defesa devia ficar", st.MarshalGuild)
	}
	if c.beta.s.Siege.InCastle {
		t.Error("o atacante deveria ter sido tirado do castelo")
	}
}

func TestHolderLeavingDropsTheOrb(t *testing.T) {
	c := cercoPronto(t)
	c.tomaOsPontos(t)
	c.membro.Position.X += 50 // saiu da orbe do fogo
	c.h.SiegeTick()
	if c.h.Siege.OrbHolder(2) != 0 || c.h.Siege.Status() != siege.StatusRunning || c.h.Siege.SealHolder() != 0 {
		t.Errorf("status=%v orbe2=%d selo=%d", c.h.Siege.Status(), c.h.Siege.OrbHolder(2), c.h.Siege.SealHolder())
	}
}

func TestSealNeedsOrbsAndAllianceLeader(t *testing.T) {
	c := cercoPronto(t)
	h := c.h
	h.SiegeAnyTime = true
	h.signInCastle(c.beta.s)
	h.enterCastle(c.beta.s)
	c.beta.s.Position = model.Position{X: siege.SealPos[0], Y: siege.SealPos[1]}
	h.castleCollect(c.beta.e.conn, c.beta.s, castleSealFlag)
	if h.Siege.SealHolder() != 0 {
		t.Error("sem as orbes nao ha selo")
	}
}

func TestSiegeBlocksAllianceChanges(t *testing.T) {
	c := cercoPronto(t)
	// A Alfa e a defesa: lider de alianca cadastrada, nao desfaz.
	if c.h.destroyAlliance(context.Background(), c.lider) {
		t.Error("lider cadastrado na guerra nao desfaz a alianca")
	}
}
