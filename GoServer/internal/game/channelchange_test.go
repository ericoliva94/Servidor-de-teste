package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
)

// doisCanais monta dois handlers de canais diferentes que dividem a fila de
// trocas, como o processo faz.
func doisCanais(t *testing.T) (h0, h1 *Handler, ho *Handoffs) {
	t.Helper()
	ho = NewHandoffs()
	estado := func(id uint8) ChannelState {
		switch id {
		case 0, 1:
			return ChannelUp
		case 2:
			return ChannelDown
		}
		return ChannelMissing
	}
	h0, h1 = testHandler(), testHandler()
	h1.Channel = 1
	h0.Chan, h1.Chan = NewChannel(0, 10), NewChannel(1, 10)
	for _, h := range []*Handler{h0, h1} {
		h.Handoffs = ho
		h.ChannelStatus = estado
		h.Parties = NewPartyRegistry()
	}
	return h0, h1, ho
}

// jogadorEm poe o jogador de teste em jogo no canal do handler.
func jogadorEm(t *testing.T, h *Handler) (*Session, *espiao) {
	t.Helper()
	e := novoEspiao(t)
	s := sessaoAutenticada()
	s.Status = Playing
	s.SelectedSlot = 0
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.conn.Session = s
	return s, e
}

func pedidoDeCanal(t *testing.T, alvo uint32) []byte {
	t.Helper()
	buf, err := netsrv.Marshal(CodeChangeChannel, packets.ChangeChannelPacket{Info1: alvo})
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func loginDeCanal(t *testing.T, serial uint32, usuario string) []byte {
	t.Helper()
	var p packets.ChannelSendInfoPacket
	p.Serial = serial
	netsrv.PutCString(p.Username[:], usuario)
	buf, err := netsrv.Marshal(CodeLoginIntoChannel, p)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestTrocaDeCanalRecusas(t *testing.T) {
	h0, _, _ := doisCanais(t)
	s, e := jogadorEm(t, h0)
	casos := []struct {
		alvo uint32
		msg  string
	}{
		{9, "Canal destino não válido. ERR_1."},
		{300, "Canal destino não válido. ERR_1."},
		{2, "Canal destino não válido. ERR_2."},
		{0, "Canal destino não válido. ERR_3."},
	}
	for _, k := range casos {
		if err := h0.changeChannel(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, k.alvo))); err != nil {
			t.Fatal(err)
		}
		if got := mensagemDe(t, e); got != k.msg {
			t.Errorf("alvo %d: mensagem = %q, queria %q", k.alvo, got, k.msg)
		}
	}
	if s.handoffAccount != 0 || e.conn.Closing() {
		t.Error("pedido recusado nao pode soltar a conexao")
	}
	// Sem registro de trocas, ou fora do jogo.
	h0.Handoffs = nil
	if err := h0.changeChannel(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, 1))); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, e); got != "Canal destino não válido. ERR_1." {
		t.Errorf("sem registro: %q", got)
	}
	s.Status = CharList
	if err := h0.changeChannel(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, 1))); err != ErrNotPlaying {
		t.Errorf("fora do jogo: erro = %v", err)
	}
}

func TestTrocaDeCanalCompleta(t *testing.T) {
	h0, h1, ho := doisCanais(t)
	s, e := jogadorEm(t, h0)
	if err := h0.changeChannel(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, 1))); err != nil {
		t.Fatal(err)
	}
	ordem := decodifica[packets.ChangeChannelPacket](t, proximoDe(t, e, CodeChangeChannel))
	if ordem.Info1 != 7 || ordem.TypeChanel != 1 {
		t.Errorf("ordem = %+v, queria conta 7 e canal 1", ordem)
	}
	if !e.conn.Closing() || s.handoffAccount != 7 {
		t.Errorf("fechando=%v conta=%d", e.conn.Closing(), s.handoffAccount)
	}

	// Enquanto o canal velho nao gravou, o novo espera.
	curto, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := ho.Claim(curto, 7, 1, "teste", e.conn.IP(), agora); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("antes da gravacao: %v", err)
	}

	h0.OnDisconnect(e.conn) // grava e libera
	if s.handoffAccount != 0 {
		t.Error("a ordem deveria ter sido liberada ao soltar a conexao")
	}

	// O jogador reaparece no canal 1.
	e2 := novoEspiao(t)
	s2 := NewSession(1)
	e2.conn.Session = s2
	if err := h1.loginIntoChannel(context.Background(), e2.conn, s2,
		asPacket(t, loginDeCanal(t, 7, "teste"))); err != nil {
		t.Fatalf("login no canal novo: %v", err)
	}
	if s2.Status != Playing || s2.SelectedSlot != 0 || !s2.Authenticated || s2.Account.ID != 7 {
		t.Errorf("sessao nova: status=%v slot=%d conta=%d", s2.Status, s2.SelectedSlot, s2.Account.ID)
	}
	if _, _, ok := h1.Chan.Lookup(s2.ClientID); !ok {
		t.Error("o jogador deveria estar no canal 1")
	}
	proximoDe(t, e2, CodeSendToWorld)

	// A ordem vale uma vez so.
	e3 := novoEspiao(t)
	err := h1.loginIntoChannel(context.Background(), e3.conn, NewSession(1),
		asPacket(t, loginDeCanal(t, 7, "teste")))
	if !errors.Is(err, ErrNoHandoff) {
		t.Errorf("repetir a ordem: %v", err)
	}
}

func TestLoginNoCanalNovoConfereOrdemUsuarioEnderecoECanal(t *testing.T) {
	h0, h1, ho := doisCanais(t)
	_, e := jogadorEm(t, h0)
	ip := e.conn.IP()
	pronta := func(para uint8, ipOrdem string) {
		ho.Cancel(7)
		if !ho.Begin(7, "teste", 0, 0, para, ipOrdem, agora) {
			t.Fatal("Begin recusado")
		}
		ho.Saved(7)
	}
	tenta := func(h *Handler, serial uint32, usuario string) error {
		return h.loginIntoChannel(context.Background(), novoEspiao(t).conn, NewSession(h.Channel),
			asPacket(t, loginDeCanal(t, serial, usuario)))
	}

	if err := tenta(h1, 7, "teste"); !errors.Is(err, ErrNoHandoff) {
		t.Errorf("sem ordem: %v", err)
	}
	pronta(1, ip)
	if err := tenta(h1, 7, "outro"); !errors.Is(err, ErrHandoffMismatch) {
		t.Errorf("usuario errado: %v", err)
	}
	if err := tenta(h0, 7, "teste"); !errors.Is(err, ErrHandoffMismatch) {
		t.Errorf("canal errado: %v", err)
	}
	pronta(1, "10.9.9.9")
	if err := tenta(h1, 7, "teste"); !errors.Is(err, ErrHandoffMismatch) {
		t.Errorf("endereco errado: %v", err)
	}
	pronta(1, ip)
	if err := tenta(h1, 8, "teste"); !errors.Is(err, ErrNoHandoff) {
		t.Errorf("conta errada: %v", err)
	}
	if err := tenta(h1, 7, "  "); err == nil {
		t.Error("usuario vazio deveria falhar")
	}
	h1.Now = func() time.Time { return agora.Add(handoffTTL + time.Second) }
	if err := tenta(h1, 7, "teste"); !errors.Is(err, ErrHandoffExpired) {
		t.Errorf("ordem vencida: %v", err)
	}
}

func TestOrdensDeTrocaNaoSeSobrepoe(t *testing.T) {
	ho := NewHandoffs()
	if !ho.Begin(1, "a", 0, 0, 1, "ip", agora) {
		t.Fatal("primeira ordem")
	}
	if ho.Begin(1, "a", 0, 0, 1, "ip", agora.Add(time.Second)) {
		t.Error("ordem viva nao pode ser substituida")
	}
	if !ho.Begin(1, "a", 0, 0, 1, "ip", agora.Add(handoffTTL+time.Second)) {
		t.Error("ordem vencida pode ser substituida")
	}
}

func TestClaimEsperaAGravacaoEDepoisLibera(t *testing.T) {
	ho := NewHandoffs()
	ho.Begin(1, "a", 3, 0, 1, "ip", agora)
	feito := make(chan error, 1)
	go func() {
		hd, err := ho.Claim(context.Background(), 1, 1, "A", "ip", agora)
		if err == nil && hd.Slot != 3 {
			err = errors.New("slot errado")
		}
		feito <- err
	}()
	select {
	case err := <-feito:
		t.Fatalf("nao devia terminar antes da gravacao: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	ho.Saved(1)
	select {
	case err := <-feito:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Claim nao terminou depois da gravacao")
	}
}

func TestF0BSoReconhece(t *testing.T) {
	h0, _, _ := doisCanais(t)
	s, e := jogadorEm(t, h0)
	if err := h0.sendToWorldSends(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, 0))); err != nil {
		t.Errorf("em jogo: %v", err)
	}
	s.Status = CharList
	if err := h0.sendToWorldSends(context.Background(), e.conn, s, asPacket(t, pedidoDeCanal(t, 0))); err != ErrNotPlaying {
		t.Errorf("fora do jogo: %v", err)
	}
}

func TestLoginNoCanalNovoRecusaSessaoJaAutenticadaEContaDiferente(t *testing.T) {
	_, h1, ho := doisCanais(t)
	e := novoEspiao(t)
	ip := e.conn.IP()

	// Sessao que ja fez login normal nao pode trocar de conta por este caminho.
	ho.Begin(7, "teste", 0, 0, 1, ip, agora)
	ho.Saved(7)
	logada := sessaoAutenticada()
	if err := h1.loginIntoChannel(context.Background(), e.conn, logada,
		asPacket(t, loginDeCanal(t, 7, "teste"))); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("sessao ja autenticada: %v", err)
	}

	// Ordem da conta 8 com o usuario "teste", que no banco e a conta 7.
	ho.Cancel(7)
	ho.Begin(8, "teste", 0, 0, 1, ip, agora)
	ho.Saved(8)
	s := NewSession(1)
	err := h1.loginIntoChannel(context.Background(), e.conn, s, asPacket(t, loginDeCanal(t, 8, "teste")))
	if err == nil || s.Authenticated {
		t.Errorf("conta diferente da ordem: err=%v autenticada=%v", err, s.Authenticated)
	}
}

// membroEm poe um segundo jogador (conta 8) em jogo no canal do handler.
func membroEm(t *testing.T, h *Handler, x, y float32) (*Session, *espiao) {
	t.Helper()
	e := novoEspiao(t)
	s := sessaoAutenticada()
	s.Username = "membro"
	s.Account.ID = 8
	s.Account.Username = "membro"
	s.Characters = personagens()[8]
	s.Status = Playing
	s.SelectedSlot = 0
	s.Position.X, s.Position.Y = x, y
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.conn.Session = s
	return s, e
}

func contaDoMembro(h *Handler) {
	acc := contaValida()
	acc.ID, acc.Username = 8, "membro"
	h.Accounts = memAccounts{"teste": contaValida(), "membro": acc}
	chars := personagens()
	ar := chars[7]
	ar[0].ID = 200
	ar[0].Name = "Membro"
	chars[8] = ar
	h.Chars = memChars{chars: chars}
}

func TestGrupoViajaJuntoParaOCanalNovo(t *testing.T) {
	h0, h1, _ := doisCanais(t)
	contaDoMembro(h0)
	contaDoMembro(h1)
	h0.WatchDistance = 50
	lider, el := jogadorEm(t, h0)
	perto, ep := membroEm(t, h0, 3, 3)
	g, err := h0.Parties.Create(lider.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	g.ExpAllocate, g.ItemAllocate = 2, 3
	if err := h0.Parties.Add(g, perto.ClientID); err != nil {
		t.Fatal(err)
	}
	if err := h0.changeChannel(context.Background(), el.conn, lider, asPacket(t, pedidoDeCanal(t, 1))); err != nil {
		t.Fatal(err)
	}
	if !el.conn.Closing() || !ep.conn.Closing() {
		t.Fatalf("lider fechando=%v membro fechando=%v", el.conn.Closing(), ep.conn.Closing())
	}
	ordem := decodifica[packets.ChangeChannelPacket](t, proximoDe(t, ep, CodeChangeChannel))
	if ordem.Info1 != 8 || ordem.TypeChanel != 1 {
		t.Errorf("ordem do membro = %+v", ordem)
	}
	h0.OnDisconnect(ep.conn)
	h0.OnDisconnect(el.conn)

	// O membro chega primeiro; o lider depois assume a lideranca.
	login := func(serial uint32, nome string) (*Session, *espiao) {
		e := novoEspiao(t)
		s := NewSession(1)
		e.conn.Session = s
		if err := h1.loginIntoChannel(context.Background(), e.conn, s, asPacket(t, loginDeCanal(t, serial, nome))); err != nil {
			t.Fatalf("login de %s: %v", nome, err)
		}
		return s, e
	}
	m1, _ := login(8, "membro")
	l1, _ := login(7, "teste")
	novo := h1.Parties.Of(l1.ClientID)
	if novo == nil || novo.Size() != 2 || !novo.Has(m1.ClientID) {
		t.Fatalf("grupo no canal novo = %+v", novo)
	}
	if novo.Leader != l1.ClientID || novo.ExpAllocate != 2 || novo.ItemAllocate != 3 {
		t.Errorf("lider=%d (queria %d) exp=%d item=%d", novo.Leader, l1.ClientID, novo.ExpAllocate, novo.ItemAllocate)
	}
	if h0.Parties.Count() != 0 {
		t.Error("o grupo do canal velho deveria ter acabado")
	}
}

func TestMembroLongeFicaForaDaViagem(t *testing.T) {
	h0, _, ho := doisCanais(t)
	contaDoMembro(h0)
	h0.WatchDistance = 50
	lider, el := jogadorEm(t, h0)
	longe, ep := membroEm(t, h0, 500, 500)
	g, _ := h0.Parties.Create(lider.ClientID)
	if err := h0.Parties.Add(g, longe.ClientID); err != nil {
		t.Fatal(err)
	}
	if err := h0.changeChannel(context.Background(), el.conn, lider, asPacket(t, pedidoDeCanal(t, 1))); err != nil {
		t.Fatal(err)
	}
	if ep.conn.Closing() {
		t.Error("quem esta longe nao viaja")
	}
	if h0.Parties.Of(longe.ClientID) != nil {
		t.Error("quem ficou longe sai do grupo")
	}
	if _, err := ho.Claim(context.Background(), 8, 1, "membro", ep.conn.IP(), agora); !errors.Is(err, ErrNoHandoff) {
		t.Errorf("sem ordem para quem ficou: %v", err)
	}
}

func TestLiderDeLegiaoNaoTrocaDeCanal(t *testing.T) {
	h0, _, _ := doisCanais(t)
	contaDoMembro(h0)
	lider, el := jogadorEm(t, h0)
	outro, _ := membroEm(t, h0, 1, 1)
	g, _ := h0.Parties.Create(lider.ClientID)
	_ = h0.Parties.Add(g, outro.ClientID)
	g2, _ := h0.Parties.Create(outro.ClientID + 100)
	if err := h0.Parties.CreateRaid(g.Index, g2.Index); err != nil {
		t.Skipf("raide nao montada: %v", err)
	}
	if err := h0.changeChannel(context.Background(), el.conn, lider, asPacket(t, pedidoDeCanal(t, 1))); err != nil {
		t.Fatal(err)
	}
	if el.conn.Closing() || mensagemDe(t, el) == "" {
		t.Error("lider de legiao deveria ser avisado e ficar")
	}
}

func TestDoisGruposQueViajamNaoSeMisturam(t *testing.T) {
	_, h1, ho := doisCanais(t)
	a, b := &PartyMove{Key: ho.NextGroupKey(), LeaderAcc: 1}, &PartyMove{Key: ho.NextGroupKey(), LeaderAcc: 3}
	if a.Key == b.Key {
		t.Fatal("chaves iguais")
	}
	novo := func(acc uint32) *Session {
		s := NewSession(1)
		s.Account.ID = acc
		if _, err := h1.Chan.Join(novoEspiao(t).conn, s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	a1, b1, a2, b2 := novo(1), novo(3), novo(2), novo(4)
	h1.rejoinParty(a1, a)
	h1.rejoinParty(b1, b)
	h1.rejoinParty(a2, a)
	h1.rejoinParty(b2, b)
	ga, gb := h1.Parties.Of(a1.ClientID), h1.Parties.Of(b1.ClientID)
	if ga == nil || gb == nil || ga.Index == gb.Index || !ga.Has(a2.ClientID) || !gb.Has(b2.ClientID) {
		t.Errorf("grupos misturados: %+v %+v", ga, gb)
	}
}
