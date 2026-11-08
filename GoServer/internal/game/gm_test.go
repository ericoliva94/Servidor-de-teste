package game

import (
	"context"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

type fakeGM struct {
	mu      sync.Mutex
	contas  map[string]*store.GMAccount
	todos   [][2]uint32
	bloqueo []string
	dias    []uint16
	banidos []uint32
}

func (f *fakeGM) BanAccount(_ context.Context, id uint32, dias uint16, perm bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.banidos = append(f.banidos, id)
	f.dias = append(f.dias, dias)
	return nil
}

func (f *fakeGM) ByUsername(_ context.Context, u string) (store.GMAccount, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.contas[u]
	if !ok {
		return store.GMAccount{}, false, nil
	}
	return *a, true, nil
}

func (f *fakeGM) SetPasswordErrors(_ context.Context, u string, n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contas[u].PasswordErrors = n
	return nil
}

func (f *fakeGM) Block(_ context.Context, u string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contas[u].Status = 0
	f.bloqueo = append(f.bloqueo, u)
	return nil
}

func (f *fakeGM) AddEventItemToAll(_ context.Context, item, qtd uint32) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.todos = append(f.todos, [2]uint32{item, qtd})
	return 3, nil
}

type cenarioGM struct {
	h     *Handler
	h2    *Handler
	db    *fakeGM
	gm    *Session
	eg    *espiao
	alvo  *Session
	ea    *espiao
	ev    *fakeEvents
	outro *Session
	eo    *espiao
}

func novoCenarioGM(t *testing.T) *cenarioGM {
	t.Helper()
	h, h2, _ := doisCanais(t)
	c := &cenarioGM{h: h, h2: h2}
	c.db = &fakeGM{contas: map[string]*store.GMAccount{
		"mod":   {ID: 1, Username: "mod", Password: "senha", Status: 1, MasterPriv: PrivModerator},
		"gm":    {ID: 2, Username: "gm", Password: "senha", Status: 1, MasterPriv: PrivGameMaster},
		"admin": {ID: 3, Username: "admin", Password: "senha", Status: 1, MasterPriv: PrivAdmin},
		"preso": {ID: 4, Username: "preso", Password: "senha", Status: 0, MasterPriv: PrivAdmin},
	}}
	c.ev = &fakeEvents{roubado: map[uint32]bool{}}
	for _, x := range []*Handler{h, h2} {
		x.GMDB = c.db
		x.EventDB = c.ev
		x.Channels = []*Handler{h, h2}
	}
	c.gm, c.eg = jogadorEm(t, h)
	c.gm.Avatar.Name = "Mestre"
	c.alvo, c.ea = membroEm(t, h, 1, 1)
	c.alvo.Avatar.Name = "Alvo"
	c.alvo.CashBalance = 100
	c.alvo.Avatar.Gold = 1000
	h.Chan.RegisterCharacter(c.alvo.ClientID, "Alvo")
	c.outro, c.eo = membroEm(t, h2, 2, 2)
	c.outro.Avatar.Name = "Distante"
	h2.Chan.RegisterCharacter(c.outro.ClientID, "Distante")
	return c
}

func (c *cenarioGM) envia(t *testing.T, code uint16, v any) {
	t.Helper()
	buf, err := netsrv.Marshal(code, v)
	if err != nil {
		t.Fatal(err)
	}
	p := asPacket(t, buf)
	var e error
	switch code {
	case CodeGMLogin:
		e = c.h.gmLogin(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMMove:
		e = c.h.gmMove(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMChat:
		e = c.h.gmChat(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMGold:
		e = c.h.gmGold(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMCash:
		e = c.h.gmCash(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMDisconnect:
		e = c.h.gmDisconnect(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMEventItem:
		e = c.h.gmEventItem(context.Background(), c.eg.conn, c.gm, p)
	case CodeGMEventItemAll:
		e = c.h.gmEventItemAll(context.Background(), c.eg.conn, c.gm, p)
	}
	if e != nil {
		t.Fatal(e)
	}
}

func (c *cenarioGM) entra(t *testing.T, user string) int32 {
	t.Helper()
	var p packets.PacketLoginIntoServer
	netsrv.PutCString(p.Username[:], user)
	netsrv.PutCString(p.Password[:], md5Hex("senha"))
	c.envia(t, CodeGMLogin, p)
	r := decodifica[packets.PacketLoginIntoServerResponse](t, proximoDe(t, c.eg, CodeGMLoginReply))
	return r.Response
}

func nick(dst []byte, s string) { netsrv.PutCString(dst, s) }

func TestLoginDePainel(t *testing.T) {
	c := novoCenarioGM(t)
	if got := c.entra(t, "admin"); got != 1 || !c.gm.gmAtLeast(PrivAdmin) || c.gm.gm.username != "admin" {
		t.Errorf("login valido: resposta=%d gm=%+v", got, c.gm.gm)
	}
	c2 := novoCenarioGM(t)
	for _, u := range []string{"naoexiste", "preso", "  "} {
		if got := c2.entra(t, u); got != -1 || c2.gm.gm.online {
			t.Errorf("%q: resposta=%d online=%v", u, got, c2.gm.gm.online)
		}
	}
}

func TestSenhaErradaContaEBloqueia(t *testing.T) {
	c := novoCenarioGM(t)
	var p packets.PacketLoginIntoServer
	netsrv.PutCString(p.Username[:], "admin")
	netsrv.PutCString(p.Password[:], "errada")
	for i := 1; i <= gmMaxPasswordErrors; i++ {
		c.envia(t, CodeGMLogin, p)
		if r := decodifica[packets.PacketLoginIntoServerResponse](t, proximoDe(t, c.eg, CodeGMLoginReply)); r.Response != -1 {
			t.Fatalf("tentativa %d: resposta %d", i, r.Response)
		}
		if c.db.contas["admin"].PasswordErrors != i {
			t.Fatalf("contador = %d, queria %d", c.db.contas["admin"].PasswordErrors, i)
		}
	}
	if len(c.db.bloqueo) != 0 {
		t.Fatal("ainda nao devia ter bloqueado")
	}
	c.envia(t, CodeGMLogin, p) // a sexta errada bloqueia
	proximoDe(t, c.eg, CodeGMLoginReply)
	if len(c.db.bloqueo) != 1 {
		t.Error("a conta deveria ter sido bloqueada")
	}
	// Bloqueada: nem a senha certa entra.
	if got := c.entra(t, "admin"); got != -1 {
		t.Errorf("conta bloqueada entrou: %d", got)
	}
	// Acerto zera o contador.
	c.db.contas["gm"].PasswordErrors = 3
	c.entra(t, "gm")
	if c.db.contas["gm"].PasswordErrors != 0 {
		t.Error("acerto deveria zerar os erros")
	}
}

func TestSemLoginNadaFunciona(t *testing.T) {
	c := novoCenarioGM(t)
	var g packets.SendGoldAddRemove
	g.Gold, g.Add = 500, 1
	nick(g.Nickname[:], "Alvo")
	c.envia(t, CodeGMGold, g)
	var d packets.SendDisconnect
	nick(d.Nickname[:], "Alvo")
	c.envia(t, CodeGMDisconnect, d)
	if c.alvo.Avatar.Gold != 1000 || c.ea.conn.Closing() {
		t.Error("conexao sem login de painel nao pode agir")
	}
}

func TestPrivilegiosDoPainel(t *testing.T) {
	casos := []struct {
		user                              string
		move, chat, gold, cash, dc, event bool
	}{
		{"mod", true, true, false, false, false, false},
		{"gm", true, true, false, false, true, false},
		{"admin", true, true, true, true, true, true},
	}
	for _, k := range casos {
		c := novoCenarioGM(t)
		c.entra(t, k.user)
		var mv packets.MovePacket
		nick(mv.NickName[:], "Alvo")
		mv.Position = model.Position{X: 50, Y: 60}
		c.envia(t, CodeGMMove, mv)
		if got := c.alvo.Position.X == 50; got != k.move {
			t.Errorf("%s mover = %v", k.user, got)
		}
		var g packets.SendGoldAddRemove
		g.Gold, g.Add = 500, 1
		nick(g.Nickname[:], "Alvo")
		c.envia(t, CodeGMGold, g)
		if got := c.alvo.Avatar.Gold == 1500; got != k.gold {
			t.Errorf("%s ouro = %v", k.user, got)
		}
		var ca packets.SendCashAddRemove
		ca.Cash, ca.Add = 50, 1
		nick(ca.Nickname[:], "Alvo")
		c.envia(t, CodeGMCash, ca)
		if got := c.alvo.CashBalance == 150; got != k.cash {
			t.Errorf("%s cash = %v", k.user, got)
		}
		var d packets.SendDisconnect
		nick(d.Nickname[:], "Alvo")
		c.envia(t, CodeGMDisconnect, d)
		if got := c.ea.conn.Closing(); got != k.dc {
			t.Errorf("%s desconectar = %v", k.user, got)
		}
		var ev packets.SendItemEventOne
		ev.ItemID, ev.Amount = 500, 2
		nick(ev.Nickname[:], "Alvo")
		c.envia(t, CodeGMEventItem, ev)
		if got := len(c.ev.fila) == 1; got != k.event {
			t.Errorf("%s item de evento = %v", k.user, got)
		}
		c.envia(t, CodeGMEventItemAll, packets.SendItemEventForAll{ItemID: 500, Amount: 1})
		if got := len(c.db.todos) == 1; got != k.event {
			t.Errorf("%s item para todos = %v", k.user, got)
		}
	}
}

func TestOuroECashDoPainel(t *testing.T) {
	c := novoCenarioGM(t)
	c.entra(t, "admin")
	g := func(add, zero uint8, v uint64) {
		var p packets.SendGoldAddRemove
		p.Gold, p.Add, p.RemoveAllGold = v, add, zero
		nick(p.Nickname[:], "Alvo")
		c.envia(t, CodeGMGold, p)
	}
	g(0, 0, 300)
	if c.alvo.Avatar.Gold != 700 {
		t.Errorf("remover: %d", c.alvo.Avatar.Gold)
	}
	g(0, 0, 99999)
	if c.alvo.Avatar.Gold != 0 {
		t.Errorf("remover alem do saldo: %d", c.alvo.Avatar.Gold)
	}
	g(1, 0, 500)
	g(1, 0, 1<<62)
	if c.alvo.Avatar.Gold != MaxGold {
		t.Errorf("teto: %d", c.alvo.Avatar.Gold)
	}
	g(0, 1, 0)
	if c.alvo.Avatar.Gold != 0 {
		t.Errorf("zerar: %d", c.alvo.Avatar.Gold)
	}
	proximoDe(t, c.ea, CodeRefreshMoney)

	k := func(add, zero uint8, v uint64) {
		var p packets.SendCashAddRemove
		p.Cash, p.Add, p.RemoveAllCash = v, add, zero
		nick(p.Nickname[:], "Alvo")
		c.envia(t, CodeGMCash, p)
	}
	k(0, 0, 40)
	if c.alvo.CashBalance != 60 {
		t.Errorf("cash remover: %d", c.alvo.CashBalance)
	}
	k(0, 0, 4000)
	if c.alvo.CashBalance != 0 {
		t.Errorf("cash alem do saldo: %d", c.alvo.CashBalance)
	}
	k(1, 0, 7)
	k(1, 0, 1<<40)
	if c.alvo.CashBalance != ^uint32(0) {
		t.Errorf("cash teto: %d", c.alvo.CashBalance)
	}
	k(0, 1, 0)
	if c.alvo.CashBalance != 0 {
		t.Errorf("cash zerar: %d", c.alvo.CashBalance)
	}
	proximoDe(t, c.ea, CodePlayerCash)
}

func TestPainelAlcancaOutroCanalEPorId(t *testing.T) {
	c := novoCenarioGM(t)
	c.entra(t, "admin")
	var g packets.SendGoldAddRemove
	g.Gold, g.Add = 10, 1
	nick(g.Nickname[:], "Distante")
	antes := c.outro.Avatar.Gold
	c.envia(t, CodeGMGold, g)
	if c.outro.Avatar.Gold != antes+10 {
		t.Errorf("jogador de outro canal: ouro %d", c.outro.Avatar.Gold)
	}
	// Por id, sem nome, so no canal do painel.
	var p packets.SendGoldAddRemove
	p.Gold, p.Add, p.TargetID = 5, 1, c.alvo.ClientID
	before := c.alvo.Avatar.Gold
	c.envia(t, CodeGMGold, p)
	if c.alvo.Avatar.Gold != before+5 {
		t.Errorf("por id: %d", c.alvo.Avatar.Gold)
	}
	// Alvo inexistente nao quebra.
	var n packets.SendGoldAddRemove
	n.Gold, n.Add = 5, 1
	nick(n.Nickname[:], "Ninguem")
	c.envia(t, CodeGMGold, n)
}

func TestChatDePainelNoCanalEEmTodos(t *testing.T) {
	c := novoCenarioGM(t)
	c.entra(t, "mod")
	msg := func(todos uint8, texto string) packets.ClientMessagePacket2 {
		var m packets.ClientMessagePacket2
		m.Type1, m.Type2 = 16, todos
		m.Msg[0] = byte(len(texto))
		copy(m.Msg[1:], texto)
		return m
	}
	c.envia(t, CodeGMChat, msg(0, "ola canal"))
	if got := mensagemDe(t, c.ea); got != "<[GameMaster] Mestre> ola canal" {
		t.Errorf("mensagem local = %q", got)
	}
	c.eo.nada(t, 50*time.Millisecond)
	c.envia(t, CodeGMChat, msg(1, "ola todos"))
	if got := mensagemDe(t, c.eo); got != "<[GameMaster] Mestre> ola todos" {
		t.Errorf("mensagem global = %q", got)
	}
	long := make([]byte, 120)
	for i := range long {
		long[i] = 'x'
	}
	c.envia(t, CodeGMChat, msg(0, string(long)))
	mensagemDe(t, c.ea) // a anterior ("ola todos")
	if got := mensagemDe(t, c.ea); len(got) > len("<[GameMaster] Mestre> ")+90 {
		t.Errorf("texto nao foi cortado: %d bytes", len(got))
	}
}

func TestMoverParaPosicaoInvalidaNaoFaz(t *testing.T) {
	c := novoCenarioGM(t)
	c.entra(t, "mod")
	var mv packets.MovePacket
	nick(mv.NickName[:], "Alvo")
	mv.Position = model.Position{X: 999999, Y: -5}
	c.envia(t, CodeGMMove, mv)
	if c.alvo.Position.X != 1 || c.alvo.Position.Y != 1 {
		t.Errorf("posicao invalida moveu: %+v", c.alvo.Position)
	}
}

type expLinear struct{}

func (expLinear) ExpForLevel(l uint16) (uint64, bool) {
	if l > 200 {
		return 0, false
	}
	return uint64(l) * 100, true
}

func (c *cenarioGM) nivel(t *testing.T, add, zero uint8, n uint16) {
	t.Helper()
	p := packets.SendLevelAddRemove{Level: n, Add: add, RemoveAllLevel: zero}
	nick(p.Nickname[:], "Alvo")
	buf, _ := netsrv.Marshal(CodeGMLevel, p)
	if err := c.h.gmLevel(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
}

func TestNivelDoPainel(t *testing.T) {
	c := novoCenarioGM(t)
	c.h.Exp, c.h2.Exp = expLinear{}, expLinear{}
	c.alvo.Avatar.Level, c.alvo.Avatar.Exp = 50, 4900
	c.entra(t, "mod")
	c.nivel(t, 1, 0, 3)
	if c.alvo.Avatar.Level != 50 {
		t.Fatal("moderador nao mexe em nivel")
	}
	c.entra(t, "admin")
	c.nivel(t, 1, 0, 3)
	if c.alvo.Avatar.Level != 53 {
		t.Errorf("somar 3: nivel %d, queria 53 (o do ALVO, nao o de quem mandou)", c.alvo.Avatar.Level)
	}
	c.nivel(t, 0, 0, 5)
	if c.alvo.Avatar.Level != 48 || c.alvo.Avatar.Exp != 4800 {
		t.Errorf("tirar 5: nivel %d exp %d", c.alvo.Avatar.Level, c.alvo.Avatar.Exp)
	}
	c.nivel(t, 0, 0, 500)
	if c.alvo.Avatar.Level != 1 || c.alvo.Avatar.Exp != 1 {
		t.Errorf("tirar demais: nivel %d exp %d", c.alvo.Avatar.Level, c.alvo.Avatar.Exp)
	}
	c.alvo.Avatar.Level = 30
	c.nivel(t, 0, 1, 0)
	if c.alvo.Avatar.Level != 1 {
		t.Errorf("zerar: nivel %d", c.alvo.Avatar.Level)
	}
	c.nivel(t, 1, 0, 0) // somar zero niveis nao faz nada
	if c.alvo.Avatar.Level != 1 {
		t.Errorf("somar zero: nivel %d", c.alvo.Avatar.Level)
	}
	c.nivel(t, 1, 0, 65000) // fora da tabela
	if c.alvo.Avatar.Level != 1 {
		t.Errorf("fora da tabela: nivel %d", c.alvo.Avatar.Level)
	}
}

func TestBanimentoDoPainel(t *testing.T) {
	c := novoCenarioGM(t)
	ban := func(dias uint16, motivo string) {
		p := packets.SendBan{Days: dias}
		nick(p.Nickname[:], "Alvo")
		netsrv.PutCString(p.Reason[:], motivo)
		buf, _ := netsrv.Marshal(CodeGMBan, p)
		if err := c.h.gmBan(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
			t.Fatal(err)
		}
	}
	c.entra(t, "mod")
	ban(3, "x")
	if len(c.db.banidos) != 0 || c.ea.conn.Closing() {
		t.Fatal("moderador nao bane")
	}
	c.entra(t, "gm")
	ban(0, "x")
	if len(c.db.banidos) != 0 {
		t.Error("zero dias nao bane")
	}
	ban(3, "trapaca")
	if len(c.db.banidos) != 1 || c.db.banidos[0] != c.alvo.Account.ID || c.db.dias[0] != 3 || !c.ea.conn.Closing() {
		t.Fatalf("banidos=%v dias=%v fechando=%v", c.db.banidos, c.db.dias, c.ea.conn.Closing())
	}
	// A mensagem chega a todos os canais, com o motivo.
	if got := mensagemDe(t, c.eo); got != "O jogador <Alvo> foi banido por 3 dias. Motivo do ban: " {
		t.Errorf("aviso = %q", got)
	}
	if got := mensagemDe(t, c.eo); got != "trapaca" {
		t.Errorf("motivo = %q", got)
	}
	// Permanente e sem motivo.
	c2 := novoCenarioGM(t)
	c2.entra(t, "gm")
	p := packets.SendBan{Days: 1000}
	nick(p.Nickname[:], "Alvo")
	buf, _ := netsrv.Marshal(CodeGMBan, p)
	if err := c2.h.gmBan(context.Background(), c2.eg.conn, c2.gm, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, c2.eo); got != "O jogador <Alvo> foi banido permanentemente. Motivo do ban não apresentado." {
		t.Errorf("aviso permanente = %q", got)
	}
	// Ninguem bane a si mesmo.
	c3 := novoCenarioGM(t)
	c3.entra(t, "gm")
	c3.h.Chan.RegisterCharacter(c3.gm.ClientID, "Mestre")
	q := packets.SendBan{Days: 5}
	nick(q.Nickname[:], "Mestre")
	buf, _ = netsrv.Marshal(CodeGMBan, q)
	_ = c3.h.gmBan(context.Background(), c3.eg.conn, c3.gm, asPacket(t, buf))
	if len(c3.db.banidos) != 0 || c3.eg.conn.Closing() {
		t.Error("auto-banimento")
	}
}

func TestBuffsDoPainelExigemAdministrador(t *testing.T) {
	c := novoCenarioGM(t)
	p := packets.SendBuffAddRemove{BuffID: 100, TargetID: c.alvo.ClientID, RemoveAllBuff: 1}
	buf, _ := netsrv.Marshal(CodeGMBuffs, p)
	c.entra(t, "gm")
	if err := c.h.gmBuffs(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
	c.entra(t, "admin")
	if err := c.h.gmBuffs(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
		t.Fatal(err) // sem tabela de skills nada quebra
	}
}
