package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
)

// handlerComGrupos devolve um handler com canal e registro de grupos.
func handlerComGrupos(t *testing.T) *Handler {
	h := testHandler()
	h.Chan = NewChannel(0, 20)
	h.Parties = NewPartyRegistry()
	return h
}

// entra poe uma sessao em jogo no canal, com nome registrado.
func entra(t *testing.T, h *Handler, nome string) (*Session, *espiao) {
	t.Helper()
	e := novoEspiao(t)
	s := lutador(nome, 1000, 1000)
	id, err := h.Chan.Join(e.conn, s)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(id, nome)
	return s, e
}

func sendPartyPacket(index uint32, nome string) []byte {
	var p packets.SendPartyPacket
	p.PlayerIndex = index
	netsrv.PutCString(p.Name[:], nome)
	buf, err := netsrv.Marshal(CodeSendParty, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func acceptPartyPacket(tipo uint32) []byte {
	var p packets.AcceptPartyPacket
	p.AceptType = tipo
	buf, err := netsrv.Marshal(CodeAcceptParty, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func kickPartyPacket(index uint32) []byte {
	var p packets.KickPartyPacket
	p.PlayerIndex = index
	buf, err := netsrv.Marshal(CodeKickParty, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// forma monta um grupo com os jogadores dados, o primeiro como lider.
func forma(t *testing.T, h *Handler, lider *Session, liderE *espiao, outros ...*Session) *Party {
	t.Helper()
	ctx := context.Background()
	for _, o := range outros {
		if err := h.sendParty(ctx, liderE.conn, lider,
			asPacket(t, sendPartyPacket(uint32(o.ClientID), ""))); err != nil {
			t.Fatal(err)
		}
		_, oc, _ := h.Chan.Lookup(o.ClientID)
		if err := h.acceptParty(ctx, oc, o,
			asPacket(t, acceptPartyPacket(BoolAccept))); err != nil {
			t.Fatal(err)
		}
	}
	g := h.Parties.Of(lider.ClientID)
	if g == nil {
		t.Fatal("o grupo deveria existir")
	}
	return g
}

// ---- registro

func TestCriarGrupo(t *testing.T) {
	r := NewPartyRegistry()
	g, err := r.Create(5)
	if err != nil {
		t.Fatal(err)
	}
	if g.Index != 1 {
		t.Errorf("indice = %d, queria 1", g.Index)
	}
	if g.Leader != 5 || !g.Has(5) || g.Size() != 1 {
		t.Errorf("grupo = %+v", g)
	}
	// Os padroes de CreateParty.
	if g.ExpAllocate != AllocateDefault || g.ItemAllocate != AllocateDefault {
		t.Errorf("alocacao = %d/%d", g.ExpAllocate, g.ItemAllocate)
	}
	if r.Of(5) != g {
		t.Error("Of deveria achar o grupo")
	}
	// Criar duas vezes para o mesmo jogador e recusado.
	if _, err := r.Create(5); err != ErrAlreadyInParty {
		t.Errorf("erro = %v", err)
	}
}

func TestLimiteDeMembros(t *testing.T) {
	r := NewPartyRegistry()
	g, _ := r.Create(1)
	for i := uint16(2); i <= MaxPartyMembers; i++ {
		if err := r.Add(g, i); err != nil {
			t.Fatalf("membro %d: %v", i, err)
		}
	}
	if g.Size() != MaxPartyMembers {
		t.Fatalf("%d membros", g.Size())
	}
	if err := r.Add(g, 99); err != ErrPartyFull {
		t.Errorf("erro = %v, queria ErrPartyFull", err)
	}
}

// O limite de 30 grupos do vetor da origem agora e um limite explicito.
func TestLimiteDeGrupos(t *testing.T) {
	r := NewPartyRegistry()
	for i := uint16(1); i <= MaxParties; i++ {
		if _, err := r.Create(i); err != nil {
			t.Fatalf("grupo %d: %v", i, err)
		}
	}
	if _, err := r.Create(999); err != ErrNoFreeParty {
		t.Errorf("erro = %v, queria ErrNoFreeParty", err)
	}
	// Desfazer um libera o indice.
	g := r.Get(1)
	r.Destroy(g)
	if _, err := r.Create(999); err != nil {
		t.Errorf("deveria caber depois de desfazer: %v", err)
	}
}

// Jogador em dois grupos nao pode acontecer.
func TestMembroSoEntraEmUmGrupo(t *testing.T) {
	r := NewPartyRegistry()
	g1, _ := r.Create(1)
	g2, _ := r.Create(10)
	if err := r.Add(g1, 5); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(g2, 5); err != ErrAlreadyInParty {
		t.Errorf("erro = %v", err)
	}
}

// Grupo de dois vira nada quando um sai, como na origem.
func TestGrupoDeDoisSeDesfaz(t *testing.T) {
	r := NewPartyRegistry()
	g, _ := r.Create(1)
	r.Add(g, 2)

	restante, soltos := r.Remove(2)
	if restante != nil {
		t.Error("o grupo deveria ter sido desfeito")
	}
	if len(soltos) != 2 {
		t.Errorf("%d soltos, queria 2: %v", len(soltos), soltos)
	}
	if r.Of(1) != nil || r.Of(2) != nil {
		t.Error("ninguem deveria ter grupo")
	}
	if r.Count() != 0 {
		t.Errorf("%d grupos", r.Count())
	}
}

// Saindo de um grupo de tres, a lideranca passa ao primeiro da lista.
func TestLiderancaPassaAoPrimeiro(t *testing.T) {
	r := NewPartyRegistry()
	g, _ := r.Create(1)
	r.Add(g, 2)
	r.Add(g, 3)

	restante, soltos := r.Remove(1)
	if restante == nil {
		t.Fatal("o grupo deveria ter sobrado")
	}
	if restante.Leader != 2 {
		t.Errorf("lider = %d, queria 2", restante.Leader)
	}
	if len(soltos) != 1 || soltos[0] != 1 {
		t.Errorf("soltos = %v", soltos)
	}
	if restante.Has(1) {
		t.Error("quem saiu nao deveria estar no grupo")
	}
}

func TestRemoverQuemNaoEstaEmGrupo(t *testing.T) {
	r := NewPartyRegistry()
	restante, soltos := r.Remove(42)
	if restante != nil || soltos != nil {
		t.Error("nao deveria ter mexido em nada")
	}
}

func TestSetLeaderSoEntreMembros(t *testing.T) {
	r := NewPartyRegistry()
	g, _ := r.Create(1)
	r.Add(g, 2)
	if err := r.SetLeader(g, 2); err != nil {
		t.Fatal(err)
	}
	if g.Leader != 2 {
		t.Errorf("lider = %d", g.Leader)
	}
	if err := r.SetLeader(g, 99); err != ErrNoParty {
		t.Errorf("erro = %v", err)
	}
}

// Registro e grupo nil nao quebram.
func TestRegistroNilNaoQuebra(t *testing.T) {
	var r *PartyRegistry
	if r.Count() != 0 || r.Of(1) != nil || r.Get(1) != nil {
		t.Error("registro nil deveria responder vazio")
	}
	if _, err := r.Create(1); err == nil {
		t.Error("Create em registro nil deveria falhar")
	}
	if err := r.Add(nil, 1); err == nil {
		t.Error("Add em registro nil deveria falhar")
	}
	if r.Destroy(nil) != nil || r.MembersOf(nil) != nil {
		t.Error("Destroy/MembersOf nil deveriam ser vazios")
	}
	var g *Party
	if g.Has(1) || g.Size() != 0 {
		t.Error("grupo nil deveria responder vazio")
	}
}

// Entrar e sair muitas vezes nao vaza indice.
func TestIndicesNaoVazam(t *testing.T) {
	r := NewPartyRegistry()
	for range 200 {
		g, err := r.Create(1)
		if err != nil {
			t.Fatal(err)
		}
		r.Add(g, 2)
		r.Destroy(g)
	}
	if r.Count() != 0 {
		t.Errorf("%d grupos sobraram", r.Count())
	}
}

// Uso concorrente nao corrompe o registro.
func TestRegistroConcorrente(t *testing.T) {
	r := NewPartyRegistry()
	const n = 20
	pronto := make(chan struct{}, n)
	for i := range n {
		go func(id uint16) {
			for range 50 {
				if g, err := r.Create(id*10 + 1); err == nil {
					r.Add(g, id*10+2)
					r.MembersOf(g)
					r.Remove(id*10 + 2)
					r.Destroy(g)
				}
			}
			pronto <- struct{}{}
		}(uint16(i + 1))
	}
	for range n {
		<-pronto
	}
	if r.Count() != 0 {
		t.Errorf("%d grupos sobraram", r.Count())
	}
}

// ---- handlers

func TestConviteEAceiteFormamGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")

	if err := h.sendParty(context.Background(), ea.conn, a,
		asPacket(t, sendPartyPacket(uint32(b.ClientID), ""))); err != nil {
		t.Fatal(err)
	}
	// O convidado recebe o convite e guarda quem convidou.
	if got := eb.espera(t); got != CodeSendParty {
		t.Errorf("recebeu %#x, queria %#x", got, CodeSendParty)
	}
	if b.PartyRequester != a.ClientID {
		t.Errorf("PartyRequester = %d, queria %d", b.PartyRequester, a.ClientID)
	}

	if err := h.acceptParty(context.Background(), eb.conn, b,
		asPacket(t, acceptPartyPacket(BoolAccept))); err != nil {
		t.Fatal(err)
	}
	g := h.Parties.Of(a.ClientID)
	if g == nil {
		t.Fatal("o grupo deveria existir")
	}
	if !g.Has(a.ClientID) || !g.Has(b.ClientID) {
		t.Errorf("membros = %v", g.Members)
	}
	if g.Leader != a.ClientID {
		t.Errorf("lider = %d, queria quem convidou %d", g.Leader, a.ClientID)
	}
	// Os dois recebem o $326.
	for nome, e := range map[string]*espiao{"lider": ea, "membro": eb} {
		achou := false
		for range 4 {
			select {
			case p := <-e.pacotes:
				if p.code == CodeUpdateParty {
					achou = true
				}
			case <-time.After(300 * time.Millisecond):
			}
			if achou {
				break
			}
		}
		if !achou {
			t.Errorf("%s nao recebeu o estado do grupo", nome)
		}
	}
}

// Convite por nome tambem funciona.
func TestConvitePorNome(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")

	if err := h.sendParty(context.Background(), ea.conn, a,
		asPacket(t, sendPartyPacket(0, "beta"))); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeSendParty {
		t.Errorf("recebeu %#x", got)
	}
	if b.PartyRequester != a.ClientID {
		t.Error("o convite nao chegou")
	}
}

// Recusar avisa quem convidou e nao forma grupo.
func TestRecusarConvite(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")

	h.sendParty(context.Background(), ea.conn, a,
		asPacket(t, sendPartyPacket(uint32(b.ClientID), "")))
	eb.drena(100 * time.Millisecond)

	if err := h.acceptParty(context.Background(), eb.conn, b,
		asPacket(t, acceptPartyPacket(BoolRefuse))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Of(a.ClientID) != nil {
		t.Error("nao deveria ter formado grupo")
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("quem convidou recebeu %#x, queria a mensagem", got)
	}
}

// O convite vale uma vez: sem isso o mesmo $323 entrava de novo.
func TestConviteValeUmaVez(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")

	h.sendParty(context.Background(), ea.conn, a,
		asPacket(t, sendPartyPacket(uint32(b.ClientID), "")))
	h.acceptParty(context.Background(), eb.conn, b,
		asPacket(t, acceptPartyPacket(BoolAccept)))
	g := h.Parties.Of(a.ClientID)
	if g.Size() != 2 {
		t.Fatalf("%d membros", g.Size())
	}

	// Segundo aceite sem convite novo nao faz nada.
	if err := h.acceptParty(context.Background(), eb.conn, b,
		asPacket(t, acceptPartyPacket(BoolAccept))); err != nil {
		t.Fatal(err)
	}
	if g.Size() != 2 {
		t.Errorf("%d membros depois do segundo aceite", g.Size())
	}
}

// Aceitar sem convite nao faz nada.
func TestAceitarSemConvite(t *testing.T) {
	h := handlerComGrupos(t)
	_, eb := entra(t, h, "Beta")
	b := h.Parties
	_ = b
	s, _, _ := h.Chan.Lookup(1)
	if err := h.acceptParty(context.Background(), eb.conn, s,
		asPacket(t, acceptPartyPacket(BoolAccept))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Count() != 0 {
		t.Error("nao deveria ter criado grupo")
	}
}

// Valor de aceite fora de 0 e 1 nao faz nada, como o case sem ramo da origem.
func TestAceiteComValorEstranho(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	h.sendParty(context.Background(), ea.conn, a,
		asPacket(t, sendPartyPacket(uint32(b.ClientID), "")))
	eb.drena(100 * time.Millisecond)

	if err := h.acceptParty(context.Background(), eb.conn, b,
		asPacket(t, acceptPartyPacket(77))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Count() != 0 {
		t.Error("nao deveria ter formado grupo")
	}
}

// Convidar quem ja esta em grupo e recusado.
func TestConvidarQuemJaTemGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, ec := entra(t, h, "Gama")
	forma(t, h, a, ea, b)

	if err := h.sendParty(context.Background(), ec.conn, c,
		asPacket(t, sendPartyPacket(uint32(b.ClientID), ""))); err != nil {
		t.Fatal(err)
	}
	if got := ec.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// Indice de alvo fora da faixa nao quebra.
func TestConviteComIndiceInvalido(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")

	for _, idx := range []uint32{0, 21, 1000, 4000000000} {
		if err := h.sendParty(context.Background(), ea.conn, a,
			asPacket(t, sendPartyPacket(idx, ""))); err != nil {
			t.Fatalf("indice %d: %v", idx, err)
		}
	}
	if h.Parties.Count() != 0 {
		t.Error("nao deveria ter criado grupo")
	}
}

// So o lider convida.
func TestSoLiderConvida(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	forma(t, h, a, ea, b)
	eb.drena(100 * time.Millisecond)

	if err := h.sendParty(context.Background(), eb.conn, b,
		asPacket(t, sendPartyPacket(uint32(c.ClientID), ""))); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem de recusa", got)
	}
}

// ---- sair e expulsar

func TestSairDoGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, ec := entra(t, h, "Gama")
	g := forma(t, h, a, ea, b, c)
	if g.Size() != 3 {
		t.Fatalf("%d membros", g.Size())
	}
	ec.drena(100 * time.Millisecond)

	if err := h.kickParty(context.Background(), ec.conn, c,
		asPacket(t, kickPartyPacket(uint32(c.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Of(c.ClientID) != nil {
		t.Error("deveria ter saido")
	}
	if g.Size() != 2 {
		t.Errorf("%d membros", g.Size())
	}
}

func TestLiderExpulsa(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	forma(t, h, a, ea, b, c)
	ea.drena(100 * time.Millisecond)

	if err := h.kickParty(context.Background(), ea.conn, a,
		asPacket(t, kickPartyPacket(uint32(c.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Of(c.ClientID) != nil {
		t.Error("deveria ter sido expulso")
	}
}

// Membro comum nao expulsa ninguem.
func TestMembroNaoExpulsa(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	forma(t, h, a, ea, b, c)
	eb.drena(100 * time.Millisecond)

	if err := h.kickParty(context.Background(), eb.conn, b,
		asPacket(t, kickPartyPacket(uint32(c.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Of(c.ClientID) == nil {
		t.Error("nao deveria ter sido expulso")
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// Expulsar quem nao e do grupo nao faz nada.
func TestExpulsarDeFora(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	g := forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	if err := h.kickParty(context.Background(), ea.conn, a,
		asPacket(t, kickPartyPacket(uint32(c.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if g.Size() != 2 {
		t.Errorf("%d membros", g.Size())
	}
}

func TestDesfazerGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	forma(t, h, a, ea, b, c)

	var p packets.SignalData
	raw, err := netsrv.Marshal(CodeDestroyParty, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.destroyParty(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Session{a, b, c} {
		if h.Parties.Of(s.ClientID) != nil {
			t.Errorf("%s ainda tem grupo", s.Avatar.Name)
		}
	}
	if h.Parties.Count() != 0 {
		t.Errorf("%d grupos", h.Parties.Count())
	}
}

// Membro comum nao desfaz o grupo.
func TestMembroNaoDesfaz(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	forma(t, h, a, ea, b)
	eb.drena(100 * time.Millisecond)

	var p packets.SignalData
	raw, _ := netsrv.Marshal(CodeDestroyParty, p)
	if err := h.destroyParty(context.Background(), eb.conn, b, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if h.Parties.Of(a.ClientID) == nil {
		t.Error("o grupo deveria continuar")
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- lideranca

func TestPassarLideranca(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	g := forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	if err := h.giveLeaderParty(context.Background(), ea.conn, a,
		asPacket(t, kickPartyPacket(uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if g.Leader != b.ClientID {
		t.Errorf("lider = %d, queria %d", g.Leader, b.ClientID)
	}
}

func TestPassarLiderancaParaDeFora(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	g := forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	if err := h.giveLeaderParty(context.Background(), ea.conn, a,
		asPacket(t, kickPartyPacket(uint32(c.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if g.Leader != a.ClientID {
		t.Errorf("lider mudou para %d", g.Leader)
	}
}

// ---- pedido para entrar

func TestPedirParaEntrar(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, ec := entra(t, h, "Gama")
	forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	var p packets.SendPartyPacket
	p.PlayerIndex = uint32(b.ClientID) // pede ao membro, o pedido vai ao lider
	raw, err := netsrv.Marshal(CodeAddSelfParty, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.addSelfParty(context.Background(), ec.conn, c, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if got := ea.espera(t); got != CodeAddSelfParty {
		t.Errorf("o lider recebeu %#x, queria %#x", got, CodeAddSelfParty)
	}
	if a.PartyRequester != c.ClientID {
		t.Errorf("PartyRequester do lider = %d, queria %d", a.PartyRequester, c.ClientID)
	}
}

// Quem ja tem grupo nao pede para entrar em outro.
func TestPedirComGrupoERecusado(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	forma(t, h, a, ea, b)
	eb.drena(100 * time.Millisecond)

	var p packets.SendPartyPacket
	p.PlayerIndex = uint32(c.ClientID)
	raw, _ := netsrv.Marshal(CodeAddSelfParty, p)
	if err := h.addSelfParty(context.Background(), eb.conn, b, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- conversa de grupo

func TestConversaDeGrupoChegaAosMembros(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, eb := entra(t, h, "Beta")
	c, ec := entra(t, h, "Gama")
	forma(t, h, a, ea, b, c)
	ea.drena(100 * time.Millisecond)
	eb.drena(100 * time.Millisecond)
	ec.drena(100 * time.Millisecond)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatParty, "", "vamos la"))); err != nil {
		t.Fatal(err)
	}
	for nome, e := range map[string]*espiao{"a": ea, "b": eb, "c": ec} {
		vis := e.proximo(t)
		if vis.code != CodeClientSay {
			t.Errorf("%s recebeu %#x", nome, vis.code)
			continue
		}
		tipo, nick, fala := leFala(t, vis.corpo)
		if tipo != ChatParty || nick != "Alfa" || fala != "vamos la" {
			t.Errorf("%s: tipo %d, nick %q, fala %q", nome, tipo, nick, fala)
		}
	}
}

// Sem grupo a conversa de grupo avisa.
func TestConversaDeGrupoSemGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatParty, "", "alguem?"))); err != nil {
		t.Fatal(err)
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// ---- pacote de estado

func TestPacoteDeGrupoLevaOsMembros(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	a.Avatar.Level = 30
	g := forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	h.sendPartyUpdate(ea.conn, a, g)
	vis := ea.proximo(t)
	if vis.code != CodeUpdateParty {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.UpdatePartyPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if got := netsrv.CString(pkt.Name[0][:]); got != "Alfa" {
		t.Errorf("primeiro nome = %q", got)
	}
	if pkt.PlayerIndex[0] != a.ClientID {
		t.Errorf("primeiro indice = %d", pkt.PlayerIndex[0])
	}
	// "Level - 1" com piso em zero.
	if pkt.Level[0] != 29 {
		t.Errorf("nivel = %d, queria 29", pkt.Level[0])
	}
	if pkt.LeaderIndex != a.ClientID {
		t.Errorf("lider = %d", pkt.LeaderIndex)
	}
	if pkt.CurHP[0] == 0 || pkt.MaxHP[0] == 0 {
		t.Errorf("HP = %d/%d", pkt.CurHP[0], pkt.MaxHP[0])
	}
}

// Nivel zero nao vira 255 no pacote do grupo.
func TestNivelZeroNoPacoteDeGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	a.Avatar.Level = 0
	g := forma(t, h, a, ea, b)
	ea.drena(100 * time.Millisecond)

	h.sendPartyUpdate(ea.conn, a, g)
	var pkt packets.UpdatePartyPacket
	netsrv.Unmarshal(asPacote(t, ea.proximo(t).corpo), &pkt)
	if pkt.Level[0] != 0 {
		t.Errorf("nivel = %d, queria 0", pkt.Level[0])
	}
}

// ---- desconexao

// Sair do servidor tira o jogador do grupo: na origem isso dependia do
// caminho de desconexao chamar RemoveMember, e quando nao chamava o grupo
// ficava com um membro que nao estava mais no servidor.
func TestDesconexaoTiraDoGrupo(t *testing.T) {
	h := handlerComGrupos(t)
	a, ea := entra(t, h, "Alfa")
	b, _ := entra(t, h, "Beta")
	c, _ := entra(t, h, "Gama")
	g := forma(t, h, a, ea, b, c)
	if g.Size() != 3 {
		t.Fatalf("%d membros", g.Size())
	}

	h.OnDisconnect(ea.conn)
	if h.Parties.Of(a.ClientID) != nil {
		t.Error("quem saiu nao deveria ter grupo")
	}
	if g.Size() != 2 {
		t.Errorf("%d membros", g.Size())
	}
	// A lideranca passou.
	if g.Leader == a.ClientID {
		t.Error("a lideranca deveria ter passado")
	}
}
