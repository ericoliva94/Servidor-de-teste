package game

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// grupoDeTeste cria um grupo de dois (lider + membro), perto de x.
type grupoDeTeste struct {
	lider, membro *Session
	le, me        *espiao
	party         *Party
}

func novoGrupoDeTeste(t *testing.T, h *Handler, nome string, x float32) grupoDeTeste {
	t.Helper()
	g := grupoDeTeste{le: novoEspiao(t), me: novoEspiao(t)}
	g.lider, g.membro = lutador(nome+"1", x, 1000), lutador(nome+"2", x+1, 1000)
	g.lider.Avatar.Nation, g.membro.Avatar.Nation = 1, 1
	if _, err := h.Chan.Join(g.le.conn, g.lider); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Chan.Join(g.me.conn, g.membro); err != nil {
		t.Fatal(err)
	}
	p, err := h.Parties.Create(g.lider.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Parties.Add(p, g.membro.ClientID); err != nil {
		t.Fatal(err)
	}
	g.party = p
	g.le.drena(5 * time.Millisecond)
	g.me.drena(5 * time.Millisecond)
	return g
}

func prepRaid(t *testing.T, n int) (*Handler, []grupoDeTeste) {
	t.Helper()
	h, _, _ := handlerComSkills(t)
	h.Chan = NewChannel(0, 60)
	gs := make([]grupoDeTeste, n)
	for i := range gs {
		gs[i] = novoGrupoDeTeste(t, h, string(rune('A'+i)), 1000+float32(i)*3)
	}
	return h, gs
}

func convidaRaid(h *Handler, t *testing.T, de grupoDeTeste, para *Session) {
	t.Helper()
	if err := h.sendRaid(context.Background(), de.le.conn, de.lider, asPacket(t,
		miscPacket(CodeSendRaid, packets.SendInviteToRaid{SendTo: uint32(para.ClientID)}))); err != nil {
		t.Fatal(err)
	}
}

func respondeRaid(h *Handler, t *testing.T, quem grupoDeTeste, aceita int32) {
	t.Helper()
	if err := h.acceptRaid(context.Background(), quem.le.conn, quem.lider, asPacket(t,
		miscPacket(CodeAcceptRaid, packets.InviteToRaidResponse{Accept: aceita}))); err != nil {
		t.Fatal(err)
	}
}

// formaRaid junta os grupos gs[0] (lider) e os demais numa legiao.
func formaRaid(h *Handler, t *testing.T, gs []grupoDeTeste) {
	t.Helper()
	for _, g := range gs[1:] {
		convidaRaid(h, t, gs[0], g.lider)
		respondeRaid(h, t, g, BoolAccept)
	}
}

func TestConviteDeRaidChegaAoLiderDoOutroGrupo(t *testing.T) {
	h, gs := prepRaid(t, 2)
	convidaRaid(h, t, gs[0], gs[1].lider)
	got := decodifica[packets.SendInviteToRaid](t, proximoDe(t, gs[1].le, CodeSendRaid))
	if got.SendTo != uint32(gs[0].lider.ClientID) || got.Header.Index != gs[1].lider.ClientID {
		t.Errorf("convite = %+v", got)
	}
	if gs[1].lider.RaidRequester != gs[0].lider.ClientID {
		t.Error("o convite nao ficou guardado")
	}
}

func TestConviteDeRaidRecusas(t *testing.T) {
	casos := map[string]struct {
		prep func(h *Handler, gs []grupoDeTeste)
		quer string // mensagem esperada ao convidador ("" = silencio)
	}{
		"longe": {func(h *Handler, gs []grupoDeTeste) {
			gs[1].lider.Position.X = 5000
		}, "O alvo esta muito longe."},
		"alvo nao e lider": {nil, "O alvo nao e lider do outro grupo."},
		"eu nao sou lider": {func(h *Handler, gs []grupoDeTeste) {
			gs[0].party.Leader = gs[0].membro.ClientID
		}, "Voce nao e lider do seu grupo."},
		"outra nacao": {func(h *Handler, gs []grupoDeTeste) {
			gs[1].lider.Avatar.Nation = 2
		}, "Nao e possivel convidar jogadores de outras nacoes para a raid."},
	}
	for nome, c := range casos {
		t.Run(nome, func(t *testing.T) {
			h, gs := prepRaid(t, 2)
			alvo := gs[1].lider
			if nome == "alvo nao e lider" {
				alvo = gs[1].membro
			}
			if c.prep != nil {
				c.prep(h, gs)
			}
			convidaRaid(h, t, gs[0], alvo)
			if alvo.RaidRequester != 0 {
				t.Error("o convite nao deveria ter sido enviado")
			}
			if got := mensagemDe(t, gs[0].le); got != c.quer {
				t.Errorf("mensagem = %q, queria %q", got, c.quer)
			}
		})
	}
}

func TestConviteSemGrupoOuAlvoInvalidoFicaEmSilencio(t *testing.T) {
	h, gs := prepRaid(t, 2)
	solo := lutador("Solo", 1000, 1000)
	se := novoEspiao(t)
	h.Chan.Join(se.conn, solo)
	// quem convida sem grupo
	if err := h.sendRaid(context.Background(), se.conn, solo, asPacket(t,
		miscPacket(CodeSendRaid, packets.SendInviteToRaid{SendTo: uint32(gs[0].lider.ClientID)}))); err != nil {
		t.Fatal(err)
	}
	se.nada(t, 30*time.Millisecond)
	// alvo fora da faixa ou inexistente
	for _, alvo := range []uint32{0, 0x10000, 999} {
		_ = h.sendRaid(context.Background(), gs[0].le.conn, gs[0].lider, asPacket(t,
			miscPacket(CodeSendRaid, packets.SendInviteToRaid{SendTo: alvo})))
	}
}

func TestAceitarCriaARaidEMostraOsSlots(t *testing.T) {
	h, gs := prepRaid(t, 2)
	gs[0].party.ExpAllocate = 3
	convidaRaid(h, t, gs[0], gs[1].lider)
	respondeRaid(h, t, gs[1], BoolAccept)

	rd := h.Parties.RaidOf(gs[0].party.Index)
	if rd == nil || rd.Leader != gs[0].party.Index || len(rd.Parties) != 2 {
		t.Fatalf("raid = %+v", rd)
	}
	if gs[1].party.ExpAllocate != 3 {
		t.Errorf("o grupo que entrou deveria adotar a divisao de experiencia: %d", gs[1].party.ExpAllocate)
	}
	// O lider da raid recebe o proprio grupo (lider da raid = 1) e o slot 1 com o aliado.
	proprio := decodifica[packets.UpdatePartyPacket](t, proximoDe(t, gs[0].le, CodeUpdateParty))
	if proprio.PartyInRaidIndex != 0 || proprio.IsPartyLeaderOfRaid != 1 {
		t.Errorf("proprio grupo = %+v", proprio)
	}
	slot1 := decodifica[packets.UpdatePartyPacket](t, proximoDe(t, gs[0].le, CodeUpdateParty))
	if slot1.PartyInRaidIndex != 1 || slot1.PlayerIndex[0] != gs[1].lider.ClientID || slot1.IsPartyLeaderOfRaid != 0 {
		t.Errorf("slot 1 = idx %d jogador %d lider %d", slot1.PartyInRaidIndex, slot1.PlayerIndex[0], slot1.IsPartyLeaderOfRaid)
	}
	// O grupo aliado ve o lider da raid no slot 1, marcado como lider.
	_ = decodifica[packets.UpdatePartyPacket](t, proximoDe(t, gs[1].le, CodeUpdateParty))
	aliado := decodifica[packets.UpdatePartyPacket](t, proximoDe(t, gs[1].le, CodeUpdateParty))
	if aliado.PartyInRaidIndex != 1 || aliado.PlayerIndex[0] != gs[0].lider.ClientID || aliado.IsPartyLeaderOfRaid != 1 {
		t.Errorf("slot 1 do aliado = %+v", aliado)
	}
	// Membros de grupo tambem recebem.
	proximoDe(t, gs[1].me, CodeUpdateParty)
}

func TestRecusarAvisaOConvidador(t *testing.T) {
	h, gs := prepRaid(t, 2)
	convidaRaid(h, t, gs[0], gs[1].lider)
	respondeRaid(h, t, gs[1], 0)
	if got := mensagemDe(t, gs[0].le); got != "B1 recusou entrar na legiao." {
		t.Errorf("mensagem = %q", got)
	}
	if h.Parties.RaidOf(gs[0].party.Index) != nil {
		t.Error("criou raid depois de recusa")
	}
	if gs[1].lider.RaidRequester != 0 {
		t.Error("o convite deveria ser apagado")
	}
}

func TestAceitarSemConviteOuAposMudancaNaoCria(t *testing.T) {
	h, gs := prepRaid(t, 2)
	respondeRaid(h, t, gs[1], BoolAccept) // sem convite
	if h.Parties.RaidOf(gs[0].party.Index) != nil {
		t.Fatal("criou raid sem convite")
	}
	convidaRaid(h, t, gs[0], gs[1].lider)
	gs[1].party.Leader = gs[1].membro.ClientID // quem aceita nao lidera mais
	respondeRaid(h, t, gs[1], BoolAccept)
	if h.Parties.RaidOf(gs[0].party.Index) != nil {
		t.Error("criou raid com quem nao e lider")
	}
}

func TestRaidAteQuatroGrupos(t *testing.T) {
	h, gs := prepRaid(t, 5)
	formaRaid(h, t, gs[:4])
	rd := h.Parties.RaidOf(gs[0].party.Index)
	if rd == nil || len(rd.Parties) != 4 {
		t.Fatalf("raid = %+v", rd)
	}
	convidaRaid(h, t, gs[0], gs[4].lider)
	if got := mensagemDe(t, gs[0].le); got != "Sua legiao ja esta completa, com 4 grupos." {
		t.Errorf("mensagem = %q", got)
	}
	// Quem nao e lider da legiao nao convida.
	convidaRaid(h, t, gs[1], gs[4].lider)
	if got := mensagemDe(t, gs[1].le); got != "Seu grupo nao pode convidar outros grupos. Peca ao lider da legiao." {
		t.Errorf("mensagem = %q", got)
	}
	// Cada grupo ve os tres outros nos slots.
	if o := rd.Others(gs[1].party.Index); len(o) != 3 {
		t.Errorf("aliados = %v", o)
	}
}

func TestSairDaRaid(t *testing.T) {
	h, gs := prepRaid(t, 3)
	formaRaid(h, t, gs)
	sai := func(g grupoDeTeste) {
		if err := h.exitRaid(context.Background(), g.le.conn, g.lider, asPacket(t,
			miscPacket(CodeExitRaid, packets.SignalData{}))); err != nil {
			t.Fatal(err)
		}
	}
	// Membro que nao lidera nao tira o grupo.
	if err := h.exitRaid(context.Background(), gs[1].me.conn, gs[1].membro, asPacket(t,
		miscPacket(CodeExitRaid, packets.SignalData{}))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.RaidOf(gs[1].party.Index) == nil {
		t.Fatal("um membro comum tirou o grupo da raid")
	}
	// O lider da legiao sai: a lideranca passa ao primeiro que ficou.
	sai(gs[0])
	rd := h.Parties.RaidOf(gs[1].party.Index)
	if rd == nil || rd.Leader != gs[1].party.Index || len(rd.Parties) != 2 {
		t.Fatalf("depois da saida do lider: %+v", rd)
	}
	if h.Parties.RaidOf(gs[0].party.Index) != nil {
		t.Error("quem saiu continua na raid")
	}
	// Sobrando um grupo so, a raid se desfaz.
	sai(gs[2])
	if h.Parties.RaidOf(gs[1].party.Index) != nil {
		t.Error("raid de um grupo so deveria se desfazer")
	}
}

func TestPassarALiderancaDaRaid(t *testing.T) {
	h, gs := prepRaid(t, 3)
	formaRaid(h, t, gs)
	da := func(de grupoDeTeste, para uint16) {
		if err := h.giveLeaderRaid(context.Background(), de.le.conn, de.lider, asPacket(t,
			miscPacket(CodeGiveLeaderRaid, packets.GiveRaidLeader{GiveTo: para}))); err != nil {
			t.Fatal(err)
		}
	}
	// Alvo que nao lidera o grupo dele.
	da(gs[0], gs[1].membro.ClientID)
	if got := mensagemDe(t, gs[0].le); got != "O alvo nao e o lider do grupo." {
		t.Errorf("mensagem = %q", got)
	}
	// Quem nao lidera a legiao.
	da(gs[1], gs[2].lider.ClientID)
	if got := mensagemDe(t, gs[1].le); got != "Voce nao e o lider da raid." {
		t.Errorf("mensagem = %q", got)
	}
	// Alvo fora da raid.
	fora := novoGrupoDeTeste(t, h, "Z", 1020)
	da(gs[0], fora.lider.ClientID)
	if got := mensagemDe(t, gs[0].le); got != "O alvo esta em um grupo que nao pertence a sua raid." {
		t.Errorf("mensagem = %q", got)
	}
	// Caso valido.
	da(gs[0], gs[2].lider.ClientID)
	if rd := h.Parties.RaidOf(gs[0].party.Index); rd.Leader != gs[2].party.Index {
		t.Errorf("lider da raid = %d, queria %d", rd.Leader, gs[2].party.Index)
	}
}

func TestDesfazerOGrupoTiraDaRaid(t *testing.T) {
	h, gs := prepRaid(t, 3)
	formaRaid(h, t, gs)
	h.Parties.Destroy(gs[2].party)
	rd := h.Parties.RaidOf(gs[0].party.Index)
	if rd == nil || len(rd.Parties) != 2 {
		t.Fatalf("raid depois de desfazer um grupo = %+v", rd)
	}
}

func TestRaidComEntradaAleatoriaNaoQuebra(t *testing.T) {
	h, gs := prepRaid(t, 3)
	formaRaid(h, t, gs[:2])
	for _, g := range gs {
		g := g
		go func() {
			for range g.le.pacotes {
			}
		}()
		go func() {
			for range g.me.pacotes {
			}
		}()
	}
	r := rand.New(rand.NewSource(11))
	hs := []struct {
		code uint16
		fn   func(context.Context, *netsrv.Conn, *Session, protocol.Packet) error
	}{
		{CodeSendRaid, h.sendRaid}, {CodeAcceptRaid, h.acceptRaid},
		{CodeExitRaid, h.exitRaid}, {CodeGiveLeaderRaid, h.giveLeaderRaid},
	}
	for i := 0; i < 3000; i++ {
		x := hs[r.Intn(len(hs))]
		corpo := make([]byte, r.Intn(60))
		r.Read(corpo)
		g := gs[r.Intn(len(gs))]
		quem, e := g.lider, g.le
		if r.Intn(2) == 0 {
			quem, e = g.membro, g.me
		}
		_ = x.fn(context.Background(), e.conn, quem, protocol.Packet{Code: x.code, Size: len(corpo), Data: corpo})
	}
}
