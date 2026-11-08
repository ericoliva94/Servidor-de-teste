package game

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

func prepMisc(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, _ := handlerComItens(t)
	e := novoEspiao(t)
	s := sessaoComInventario()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, s, e
}

func miscPacket(code uint16, v any) []byte {
	buf, err := netsrv.Marshal(code, v)
	if err != nil {
		panic(err)
	}
	return buf
}

func statusPoints(t *testing.T, h *Handler, s *Session, e *espiao, idx, qtd uint32) {
	t.Helper()
	if err := h.getStatusPoint(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeGetStatusPoint, packets.GetStatusPointPacket{StatusIndex: idx, StatusAmount: qtd}))); err != nil {
		t.Fatal(err)
	}
}

// ---- pontos de atributo

func TestDistribuirPontosSomaNoAtributoEDescontaDaReserva(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.StatusPoints = 10
	antes := [5]uint16{s.Avatar.Str, s.Avatar.Agility, s.Avatar.Int, s.Avatar.Cons, s.Avatar.Luck}
	campos := []*uint16{&s.Avatar.Str, &s.Avatar.Agility, &s.Avatar.Int, &s.Avatar.Cons, &s.Avatar.Luck}
	for i := range campos {
		statusPoints(t, h, s, e, uint32(i), 2)
		if *campos[i] != antes[i]+2 {
			t.Errorf("atributo %d = %d, queria %d", i, *campos[i], antes[i]+2)
		}
	}
	if s.Avatar.StatusPoints != 0 {
		t.Errorf("reserva = %d, queria 0", s.Avatar.StatusPoints)
	}
	proximoDe(t, e, CodeRefreshPoint)
}

// Defeito da origem: indice fora de 0..4 gastava os pontos sem dar atributo.
func TestDistribuirComIndiceInvalidoNaoGastaPontos(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.StatusPoints = 10
	statusPoints(t, h, s, e, 5, 3)
	statusPoints(t, h, s, e, 0xFFFFFFFF, 3)
	if s.Avatar.StatusPoints != 10 {
		t.Errorf("reserva = %d, queria 10", s.Avatar.StatusPoints)
	}
}

func TestDistribuirRecusas(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Avatar.StatusPoints = 5
	statusPoints(t, h, s, e, 0, 6)          // mais do que tem
	statusPoints(t, h, s, e, 0, 0)          // zero
	statusPoints(t, h, s, e, 0, 0xFFFFFFFF) // enorme
	s.Avatar.Str = 65534
	statusPoints(t, h, s, e, 0, 5) // estouraria o WORD
	if s.Avatar.Str != 65534 || s.Avatar.StatusPoints != 5 {
		t.Errorf("Str=%d reserva=%d: pedido recusado alterou algo", s.Avatar.Str, s.Avatar.StatusPoints)
	}
	e.nada(t, 20*time.Millisecond)
}

// ---- PvP e acao

func TestPKModeLigaDesligaERepassa(t *testing.T) {
	h, s, e := prepMisc(t)
	oe := novoEspiao(t)
	outro := sessaoEmJogo(101, 100)
	h.Chan.Join(oe.conn, outro)
	s.Base.VisiblePlayers.Add(outro.ClientID)

	pk := func(v uint32) {
		if err := h.pkMode(context.Background(), e.conn, s, asPacket(t,
			miscPacket(CodePKMode, packets.SignalData{Data: v}))); err != nil {
			t.Fatal(err)
		}
	}
	pk(1)
	if !s.Avatar.PlayerKill {
		t.Error("PvP deveria estar ligado")
	}
	if got := decodifica[packets.SignalData](t, proximoDe(t, oe, CodePKMode)); got.Data != 1 || got.Header.Index != s.ClientID {
		t.Errorf("repasse = %+v", got)
	}
	proximoDe(t, e, CodePKMode) // o proprio jogador tambem recebe
	pk(0)
	if s.Avatar.PlayerKill {
		t.Error("PvP deveria estar desligado")
	}
	pk(7) // qualquer valor que nao seja 1 desliga
	pk(1)
	pk(256) // acima de 255: ignorado
	if !s.Avatar.PlayerKill {
		t.Error("valor acima de 255 nao deveria mudar o estado")
	}
}

func TestAcaoGuardaSoAs40e65ERepassaAosOutros(t *testing.T) {
	h, s, e := prepMisc(t)
	oe := novoEspiao(t)
	outro := sessaoEmJogo(101, 100)
	h.Chan.Join(oe.conn, outro)
	s.Base.VisiblePlayers.Add(outro.ClientID)
	acao := func(i uint32) {
		if err := h.updateAction(context.Background(), e.conn, s, asPacket(t,
			miscPacket(CodeUpdateAction, packets.SendActionPacket{Index: i, InLoop: 1}))); err != nil {
			t.Fatal(err)
		}
	}
	acao(40)
	if s.Action != 40 {
		t.Errorf("Action = %d, queria 40", s.Action)
	}
	acao(12)
	if s.Action != 40 {
		t.Errorf("Action = %d: acao fora de 40/65 nao deve ser guardada", s.Action)
	}
	acao(65)
	if s.Action != 65 {
		t.Errorf("Action = %d, queria 65", s.Action)
	}
	if got := decodifica[packets.SendActionPacket](t, proximoDe(t, oe, CodeUpdateAction)); got.Header.Index != s.ClientID {
		t.Errorf("repasse com Index %d", got.Header.Index)
	}
	e.nada(t, 20*time.Millisecond) // o autor nao recebe o proprio pacote
}

// ---- buffs

func TestCancelarBuffTiraDaListaEReenvia(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Base.Buffs.Set(100, h.now())
	s.Base.Buffs.Set(200, h.now())
	rem := func(i uint32) {
		if err := h.removeBuff(context.Background(), e.conn, s, asPacket(t,
			miscPacket(CodeRemoveBuff, packets.RemoveBuffPacket{BuffIndex: i}))); err != nil {
			t.Fatal(err)
		}
	}
	rem(100)
	if s.Base.Buffs.Has(100) || !s.Base.Buffs.Has(200) {
		t.Error("buffs apos o cancelamento errados")
	}
	lista := decodifica[packets.SendBuffsPacket](t, proximoDe(t, e, CodeRefreshBuffs))
	if lista.Buffs[0] != 200 || lista.Buffs[1] != 0 {
		t.Errorf("lista = %v", lista.Buffs[:3])
	}
	rem(0x10000) // acima do WORD: ignorado
	rem(999)     // sem esse buff: nada acontece
	if !s.Base.Buffs.Has(200) {
		t.Error("buff 200 sumiu")
	}
}

// ---- ping, cash, voltar

func TestPingMostraOsMilissegundosDesdeOComando(t *testing.T) {
	h, s, e := prepMisc(t)
	s.LastPing = h.now().Add(-250 * time.Millisecond)
	if err := h.requestPing(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeRequestPing, packets.SignalData{}))); err != nil {
		t.Fatal(err)
	}
	if got := mensagemDe(t, e); got != "250 ms." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestPingSemComandoPreviaNaoInventaUmTempoEnorme(t *testing.T) {
	h, s, e := prepMisc(t)
	_ = h.requestPing(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeRequestPing, packets.SignalData{})))
	if got := mensagemDe(t, e); got != "0 ms." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestVoltarParaAListaEstaDesativado(t *testing.T) {
	h, s, e := prepMisc(t)
	_ = h.backToCharList(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeBackToCharList, packets.SignalData{})))
	if got := mensagemDe(t, e); got != "Essa funcao foi temporariamente desativada." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Status != Playing {
		t.Error("o status mudou")
	}
}

func TestPedirSaldoPremiumResponde(t *testing.T) {
	h, s, e := prepMisc(t)
	s.CashBalance = 1234
	_ = h.requestCash(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeRequestCash, packets.SignalData{})))
	proximoDe(t, e, CodePlayerCash)
}

func TestPedidosDiversosForaDoJogo(t *testing.T) {
	h, s, e := prepMisc(t)
	s.Status = CharList
	fs := map[string]func(context.Context, *netsrv.Conn, *Session, protocol.Packet) error{
		"cash": h.requestCash, "voltar": h.backToCharList, "pontos": h.getStatusPoint,
		"acao": h.updateAction, "pk": h.pkMode, "buff": h.removeBuff, "ping": h.requestPing,
	}
	for nome, f := range fs {
		err := f(context.Background(), e.conn, s, asPacket(t, miscPacket(CodePKMode, packets.SignalData{})))
		if !errors.Is(err, ErrNotPlaying) {
			t.Errorf("%s: erro = %v", nome, err)
		}
	}
}

// ---- gravacao do progresso

type fakeCharSave struct {
	mu    sync.Mutex
	feito []store.Progress
	conta []uint32
	char  []uint32
}

func (f *fakeCharSave) SaveProgress(_ context.Context, acc, id uint32, p store.Progress) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.feito = append(f.feito, p)
	f.conta = append(f.conta, acc)
	f.char = append(f.char, id)
	return nil
}

func TestSairGravaOProgressoDoPersonagem(t *testing.T) {
	h, s, e := prepMisc(t)
	f := &fakeCharSave{}
	h.CharSave = f
	s.Account.ID = 7
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: 42, Name: "Alfa"}
	s.Avatar.Level = 51
	s.Avatar.Exp = 123456
	s.Avatar.Gold = 777
	s.Avatar.Str, s.Avatar.StatusPoints, s.Avatar.PlayerKill = 120, 9, true
	s.Position.X, s.Position.Y = 1001.5, 2002.5

	h.OnDisconnect(e.conn)

	if len(f.feito) != 1 || f.conta[0] != 7 || f.char[0] != 42 {
		t.Fatalf("gravacoes = %d conta=%v char=%v", len(f.feito), f.conta, f.char)
	}
	p := f.feito[0]
	if p.Level != 51 || p.Exp != 123456 || p.Gold != 777 || p.Strength != 120 || p.Status != 9 ||
		!p.PlayerKill || p.PosX != 1001.5 || p.PosY != 2002.5 {
		t.Errorf("progresso = %+v", p)
	}
}

func TestSairSemPersonagemOuSemBancoNaoGrava(t *testing.T) {
	h, s, e := prepMisc(t)
	f := &fakeCharSave{}
	h.CharSave = f
	s.SelectedSlot = -1
	h.OnDisconnect(e.conn)
	if len(f.feito) != 0 {
		t.Error("gravou sem personagem escolhido")
	}
	h2, s2, e2 := prepMisc(t) // sem CharSave
	s2.SelectedSlot = 0
	h2.OnDisconnect(e2.conn) // nao pode dar panic
}

// ---- gravacao periodica

func TestGravacaoPeriodicaGravaQuemEstaEmJogo(t *testing.T) {
	h, s, _ := prepMisc(t)
	f := &fakeCharSave{}
	h.CharSave = f
	s.Account.ID = 7
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: 42, Name: "Alfa"}
	s.Avatar.Gold = 555

	// Quem ainda esta na lista de personagens nao e gravado.
	fora := sessaoComInventario()
	fora.Status = CharList
	fora.Characters[0] = store.Character{ID: 43}
	fora.SelectedSlot = 0
	fora.Account.ID = 8
	if _, err := h.Chan.Join(novoEspiao(t).conn, fora); err != nil {
		t.Fatal(err)
	}

	if n := h.SaveAllTick(); n != 1 {
		t.Fatalf("gravados = %d, queria 1", n)
	}
	if len(f.feito) != 1 || f.char[0] != 42 || f.feito[0].Gold != 555 {
		t.Errorf("gravacoes = %+v char=%v", f.feito, f.char)
	}
	// Uma segunda volta grava o estado novo, sem depender da saida.
	s.Avatar.Gold = 999
	h.SaveAllTick()
	if len(f.feito) != 2 || f.feito[1].Gold != 999 {
		t.Errorf("segunda volta = %+v", f.feito)
	}
}

func TestGravacaoPeriodicaSemCanalOuSemBancoNaoQuebra(t *testing.T) {
	h := testHandler()
	if n := h.SaveAllTick(); n != 0 {
		t.Errorf("sem canal gravou %d", n)
	}
	h2, _, _ := prepMisc(t) // canal com um jogador, sem nenhum banco ligado
	if n := h2.SaveAllTick(); n != 1 {
		t.Errorf("sem banco: n = %d", n)
	}
}

// Gravar enquanto outros handlers trabalham e uma troca/compra mexe nos
// inventarios nao pode travar nem quebrar.
func TestGravacaoPeriodicaConvivecomLoja(t *testing.T) {
	h, v, ve, b, be := lojaAberta(t)
	h.CharSave = &fakeCharSave{}
	go func() {
		for range ve.pacotes {
		}
	}()
	go func() {
		for range be.pacotes {
		}
	}()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			h.SaveAllTick()
		}
		close(done)
	}()
	compra(t, h, b, be, uint32(v.ClientID), 0, v.Shop.own.Products[0])
	<-done
}
