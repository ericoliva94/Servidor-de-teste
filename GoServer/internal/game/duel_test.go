package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// prepDuelo: dois jogadores da mesma nacao, nivel 50, perto, e um terceiro que assiste.
func prepDuelo(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao, *Session, *espiao, *time.Time) {
	t.Helper()
	h, a, ae, b, be := prepTroca(t)
	h.Parties = NewPartyRegistry()
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	a.Avatar.Nation, b.Avatar.Nation = 1, 1
	a.Avatar.Level, b.Avatar.Level = 50, 50
	a.Avatar.MaxHP, b.Avatar.MaxHP = 2000, 2000
	a.Avatar.CurHP, b.Avatar.CurHP = 2000, 2000
	a.Position, b.Position = model.Position{X: 1000, Y: 1000}, model.Position{X: 1002, Y: 1000}
	a.Base.VisiblePlayers.Add(b.ClientID)
	b.Base.VisiblePlayers.Add(a.ClientID)
	v := lutador("Plateia", 1003, 1000)
	ve := novoEspiao(t)
	h.Chan.Join(ve.conn, v)
	a.Base.VisiblePlayers.Add(v.ClientID)
	ae.drena(5 * time.Millisecond)
	be.drena(5 * time.Millisecond)
	return h, a, ae, b, be, v, ve, &agora
}

func desafia(t *testing.T, h *Handler, de *Session, e *espiao, id uint32, nick string) {
	t.Helper()
	var p packets.SendDuelResquest
	p.ClientID = id
	netsrv.PutCString(p.Nick[:], nick)
	if err := h.requestDuel(context.Background(), e.conn, de, asPacket(t, miscPacket(CodeDuelRequest, p))); err != nil {
		t.Fatal(err)
	}
}

func respondeDuelo(t *testing.T, h *Handler, s *Session, e *espiao, resp uint32) {
	t.Helper()
	if err := h.answerDuel(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeDuelResponse, packets.SendDuelResponse{Response: resp}))); err != nil {
		t.Fatal(err)
	}
}

func comecaDuelo(t *testing.T) (*Handler, *Session, *espiao, *Session, *espiao, *Session, *espiao, *time.Time) {
	t.Helper()
	h, a, ae, b, be, v, ve, agora := prepDuelo(t)
	desafia(t, h, a, ae, uint32(b.ClientID), "")
	respondeDuelo(t, h, b, be, 1)
	ae.drena(5 * time.Millisecond)
	be.drena(5 * time.Millisecond)
	ve.drena(5 * time.Millisecond)
	return h, a, ae, b, be, v, ve, agora
}

func TestDesafioChegaAoAlvoComONomeDoDesafiante(t *testing.T) {
	h, a, ae, b, be, _, _, _ := prepDuelo(t)
	desafia(t, h, a, ae, uint32(b.ClientID), "")
	got := decodifica[packets.SendDuelResquest](t, proximoDe(t, be, CodeDuelRequest))
	if got.ClientID != uint32(a.ClientID) || netsrv.CString(got.Nick[:]) != "Alfa" || got.Header.Index != a.ClientID {
		t.Errorf("desafio = %+v", got)
	}
	if b.Duel.requester != a.ClientID {
		t.Error("o desafio nao ficou guardado")
	}
}

func TestDesafioPeloNome(t *testing.T) {
	h, a, ae, b, be, _, _, _ := prepDuelo(t)
	h.Chan.RegisterCharacter(b.ClientID, "Beta")
	desafia(t, h, a, ae, 0, "Beta")
	proximoDe(t, be, CodeDuelRequest)
	if b.Duel.requester != a.ClientID {
		t.Error("desafio por nome nao chegou")
	}
}

func TestDesafioRecusas(t *testing.T) {
	casos := map[string]struct {
		prep func(h *Handler, a, b *Session)
		quer string
	}{
		"PvP ligado":  {func(h *Handler, a, b *Session) { a.Avatar.PlayerKill = true }, "Nao e possivel duelar com o PvP ligado."},
		"outra nacao": {func(h *Handler, a, b *Session) { b.Avatar.Nation = 2 }, "O alvo nao e da sua nacao."},
		"nivel baixo": {func(h *Handler, a, b *Session) { b.Avatar.Level = 9 }, "E necessario que os dois jogadores sejam nivel 10 para duelar."},
		"mesmo grupo": {func(h *Handler, a, b *Session) {
			g, _ := h.Parties.Create(a.ClientID)
			_ = h.Parties.Add(g, b.ClientID)
		}, "Jogador invalido. Pertence ao seu grupo."},
		"alvo duelando": {func(h *Handler, a, b *Session) { b.Duel.active = true }, "O jogador ja esta em um duelo."},
		"eu duelando":   {func(h *Handler, a, b *Session) { a.Duel.active = true }, "Voce ja esta em um duelo."},
	}
	for nome, c := range casos {
		t.Run(nome, func(t *testing.T) {
			h, a, ae, b, _, _, _, _ := prepDuelo(t)
			c.prep(h, a, b)
			desafia(t, h, a, ae, uint32(b.ClientID), "")
			if got := mensagemDe(t, ae); got != c.quer {
				t.Errorf("mensagem = %q, queria %q", got, c.quer)
			}
			if b.Duel.requester != 0 {
				t.Error("o desafio nao deveria ter sido enviado")
			}
		})
	}
	h, a, ae, _, _, _, _, _ := prepDuelo(t)
	for _, id := range []uint32{uint32(a.ClientID), 999, 0x10000} {
		desafia(t, h, a, ae, id, "")
	}
	ae.drena(20 * time.Millisecond)
}

func TestAceitarComecaODuelo(t *testing.T) {
	h, a, ae, b, be, _, ve, _ := prepDuelo(t)
	desafia(t, h, a, ae, uint32(b.ClientID), "")
	be.drena(5 * time.Millisecond)
	respondeDuelo(t, h, b, be, 1)
	if !a.Duel.active || !b.Duel.active || a.Duel.with != b.ClientID || b.Duel.with != a.ClientID || !a.Duel.host || b.Duel.host {
		t.Fatalf("duelos: %+v / %+v", a.Duel, b.Duel)
	}
	tm := decodifica[packets.SendDuelTime](t, proximoDe(t, ae, CodeDuelTime))
	if tm.SecondsCount != 15 {
		t.Errorf("contagem = %d", tm.SecondsCount)
	}
	proximoDe(t, be, CodeDuelTime)
	// Quem assiste ve a bandeira com o titulo do duelo.
	flag := decodifica[packets.SendCreateMobPacket](t, proximoDe(t, ve, CodeCreateMob))
	if netsrv.CString(flag.Title[:]) != "Alfa vs. Beta" {
		t.Errorf("titulo da bandeira = %q", netsrv.CString(flag.Title[:]))
	}
	if flag.Header.Index < duelFlagBaseID {
		t.Errorf("id da bandeira = %d", flag.Header.Index)
	}
}

func TestRecusarOuAceitarInvalido(t *testing.T) {
	h, a, ae, b, be, _, _, _ := prepDuelo(t)
	desafia(t, h, a, ae, uint32(b.ClientID), "")
	respondeDuelo(t, h, b, be, 0)
	if got := mensagemDe(t, ae); got != "O jogador desafiado recusou seu convite de duelo." {
		t.Errorf("mensagem = %q", got)
	}
	// Sem desafio pendente nada acontece.
	respondeDuelo(t, h, b, be, 1)
	if b.Duel.active {
		t.Error("duelo sem desafio")
	}
	// Longe, morto, PvP ligado.
	for nome, prep := range map[string]func(){
		"longe": func() { b.Position.X = 9000 },
		"morto": func() { a.Dead = true },
		"pvp":   func() { b.Avatar.PlayerKill = true },
	} {
		a.Dead, b.Avatar.PlayerKill = false, false
		b.Position = model.Position{X: 1002, Y: 1000}
		prep()
		b.Duel.requester = a.ClientID
		respondeDuelo(t, h, b, be, 1)
		if b.Duel.active {
			t.Errorf("%s: o duelo comecou", nome)
		}
	}
}

func TestNaoSeBateNaEsperaEDepoisSoNoOponente(t *testing.T) {
	h, a, _, b, _, v, _, agora := comecaDuelo(t)
	if _, err := h.resolveAttackTarget(a, b.ClientID); err == nil {
		t.Error("bateu durante a espera de 15 s")
	}
	*agora = agora.Add(16 * time.Second)
	if _, err := h.resolveAttackTarget(a, b.ClientID); err != nil {
		t.Errorf("depois da espera deveria bater: %v", err)
	}
	if _, err := h.resolveAttackTarget(a, v.ClientID); err == nil {
		t.Error("duelando, so o oponente pode ser atacado")
	}
}

func TestNinguemMorreNoDueloEOQueChegaA10Perde(t *testing.T) {
	h, a, ae, b, be, v, ve, agora := comecaDuelo(t)
	*agora = agora.Add(16 * time.Second)
	b.Avatar.CurHP = 500
	h.applyDamage(b, 100000)
	if b.Dead || b.Avatar.CurHP != duelEndHP {
		t.Fatalf("dead=%v HP=%d, queria vivo com %d", b.Dead, b.Avatar.CurHP, duelEndHP)
	}
	h.duelTick()
	if a.Duel.active || b.Duel.active {
		t.Fatal("o duelo deveria ter acabado")
	}
	ganhou := decodifica[packets.MessageEndDuel](t, proximoDe(t, ae, CodeDuelEnd))
	perdeu := decodifica[packets.MessageEndDuel](t, proximoDe(t, be, CodeDuelEnd))
	if ganhou.WonLose != 1 || perdeu.WonLose != 0 {
		t.Errorf("ganhou=%d perdeu=%d", ganhou.WonLose, perdeu.WonLose)
	}
	if got := mensagemDe(t, ve); got != "Alfa venceu Beta no duelo." {
		t.Errorf("plateia viu %q", got)
	}
	proximoDe(t, ve, CodeRemoveMob) // bandeira some da tela de quem assistia
	_ = v
}

func TestDueloEmpatePorTempoESaidaDaArea(t *testing.T) {
	h, a, ae, _, be, _, _, agora := comecaDuelo(t)
	*agora = agora.Add(16 * time.Second)
	h.duelTick()
	if !a.Duel.active {
		t.Fatal("acabou cedo")
	}
	*agora = agora.Add(duelMax)
	h.duelTick()
	if a.Duel.active {
		t.Fatal("o limite de 5 minutos deveria encerrar")
	}
	for _, e := range []*espiao{ae, be} {
		if r := decodifica[packets.MessageEndDuel](t, proximoDe(t, e, CodeDuelEnd)); r.WonLose != 0 {
			t.Errorf("empate deveria mandar 0 aos dois, veio %d", r.WonLose)
		}
	}

	// Sair da area por 5 segundos perde.
	h2, a2, ae2, b2, be2, _, _, agora2 := comecaDuelo(t)
	*agora2 = agora2.Add(16 * time.Second)
	b2.Position.X = 9000
	for i := 0; i < duelOutLimit-1; i++ {
		h2.duelTick()
	}
	if !a2.Duel.active {
		t.Fatal("ainda da tempo de voltar")
	}
	h2.duelTick()
	if a2.Duel.active {
		t.Fatal("5 segundos fora da area deveriam encerrar")
	}
	if r := decodifica[packets.MessageEndDuel](t, proximoDe(t, ae2, CodeDuelEnd)); r.WonLose != 1 {
		t.Errorf("quem ficou na area deveria ganhar: %d", r.WonLose)
	}
	_ = be2
}

func TestQuemSaiDoDueloPerde(t *testing.T) {
	h, a, ae, b, _, _, _, _ := comecaDuelo(t)
	_, bc, _ := h.Chan.Lookup(b.ClientID)
	h.OnDisconnect(bc)
	if a.Duel.active {
		t.Error("o duelo de quem ficou deveria acabar")
	}
	if r := decodifica[packets.MessageEndDuel](t, proximoDe(t, ae, CodeDuelEnd)); r.WonLose != 1 {
		t.Errorf("quem ficou deveria ganhar: %d", r.WonLose)
	}
}

func TestAreaEmDueloAcertaSoOOponente(t *testing.T) {
	h, a, _, b, _, v, _, agora := comecaDuelo(t)
	v.Avatar.Nation = 1
	*agora = agora.Add(16 * time.Second)
	if !h.canHitPlayer(a, b) {
		t.Error("o oponente do duelo deveria ser atingido (mesma nacao, sem PK)")
	}
	if h.canHitPlayer(a, v) {
		t.Error("a plateia nao pode ser atingida")
	}
	*agora = agora.Add(-10 * time.Second) // volta para dentro da espera
	if h.canHitPlayer(a, b) {
		t.Error("durante a espera ninguem apanha da area")
	}
}
