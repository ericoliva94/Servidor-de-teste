package game

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

type fakeCmds struct {
	mu      sync.Mutex
	cmds    []*store.GMCommand
	chars   map[string][2]uint32 // nome -> id, nivel
	setados map[uint32][2]uint64
	proximo uint32
	nomes   []string
}

func (f *fakeCmds) GMIDByUsername(_ context.Context, u string) (uint32, bool, error) {
	return uint32(len(u)), u != "", nil
}

func (f *fakeCmds) CreateCommand(_ context.Context, c store.GMCommand) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proximo++
	c.ID = f.proximo
	f.cmds = append(f.cmds, &c)
	return c.ID, nil
}

func (f *fakeCmds) PendingCommands(context.Context) ([]store.GMCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.GMCommand
	for _, c := range f.cmds {
		if !c.Runned && !c.Refused {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeCmds) ClaimCommand(_ context.Context, id uint32, by, reason string, _ time.Time) (store.GMCommand, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if c.ID == id && !c.Runned && !c.Refused {
			c.Runned = true
			return *c, true, nil
		}
	}
	return store.GMCommand{}, false, nil
}

func (f *fakeCmds) RefuseCommand(_ context.Context, id uint32, by, reason string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if c.ID == id && !c.Runned && !c.Refused {
			c.Refused = true
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeCmds) OfflineCharacter(_ context.Context, nome string) (uint32, uint16, bool, error) {
	v, ok := f.chars[nome]
	return v[0], uint16(v[1]), ok, nil
}

func (f *fakeCmds) SetLevelExp(_ context.Context, id uint32, lv uint16, exp uint64) error {
	if f.setados == nil {
		f.setados = map[uint32][2]uint64{}
	}
	f.setados[id] = [2]uint64{uint64(lv), exp}
	return nil
}

func (f *fakeCmds) GMUsernames(context.Context) ([]string, error) { return f.nomes, nil }

func (c *cenarioGM) comTudo(t *testing.T) *fakeCmds {
	t.Helper()
	fc := &fakeCmds{chars: map[string][2]uint32{"Alvo": {55, 50}, "Fora": {77, 40}}, nomes: []string{"mod", "gm", "admin"}}
	c.h.GMCmds, c.h2.GMCmds = fc, fc
	c.h.Exp, c.h2.Exp = expLinear{}, expLinear{}
	return fc
}

func (c *cenarioGM) manda(t *testing.T, fn func(context.Context, *netsrv.Conn, *Session, protocol.Packet) error, code uint16, v any) {
	t.Helper()
	buf, err := netsrv.Marshal(code, v)
	if err != nil {
		t.Fatal(err)
	}
	if err := fn(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
}

func TestGameMasterPedeNivelEAdminSoAplica(t *testing.T) {
	c := novoCenarioGM(t)
	fc := c.comTudo(t)
	c.entra(t, "gm")
	c.nivel(t, 1, 0, 3)
	if c.alvo.Avatar.Level == 53 {
		t.Fatal("game master nao muda o nivel direto")
	}
	if len(fc.cmds) != 1 || fc.cmds[0].Type != store.GMCmdLevel || fc.cmds[0].Target != "Alvo" || fc.cmds[0].ItemCount != 3 {
		t.Fatalf("pedido = %+v", fc.cmds)
	}
	c.nivel(t, 0, 0, 3) // tirar niveis nao gera pedido
	c.nivel(t, 1, 0, 0)
	if len(fc.cmds) != 1 {
		t.Errorf("pedidos extras: %d", len(fc.cmds))
	}
}

func TestPedirCupom(t *testing.T) {
	c := novoCenarioGM(t)
	fc := c.comTudo(t)
	envia := func(p packets.SendCreateCoupomRequest) {
		buf, _ := netsrv.Marshal(CodeGMCreateCoupon, p)
		if err := c.h.gmCreateCoupon(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
			t.Fatal(err)
		}
	}
	envia(packets.SendCreateCoupomRequest{PinCodeType: 1})
	if len(fc.cmds) != 0 {
		t.Fatal("sem login de painel nao ha cupom")
	}
	c.entra(t, "mod")
	envia(packets.SendCreateCoupomRequest{PinCodeType: 2})
	envia(packets.SendCreateCoupomRequest{ItemID: 500})
	envia(packets.SendCreateCoupomRequest{ItemID: 501, ItemAmount: 7})
	envia(packets.SendCreateCoupomRequest{PinCodeType: 9})
	envia(packets.SendCreateCoupomRequest{ItemID: 70000})
	if len(fc.cmds) != 3 {
		t.Fatalf("cupons = %d, queria 3", len(fc.cmds))
	}
	pin, item1, item7 := fc.cmds[0], fc.cmds[1], fc.cmds[2]
	if pin.Type != store.GMCmdCoupon || pin.ItemID != 50 || item1.Type != store.GMCmdFounder ||
		item1.ItemID != 500 || item1.ItemCount != 1 || item7.ItemCount != 7 {
		t.Errorf("cupons = %+v %+v %+v", pin, item1, item7)
	}
	re := regexp.MustCompile(`^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`)
	if !re.MatchString(pin.Command) || pin.Command == item1.Command {
		t.Errorf("codigos = %q %q", pin.Command, item1.Command)
	}
	msg := decodifica[packets.MessageToPainelPacket](t, proximoDe(t, c.eg, CodeGMPanelMessage))
	if got := netsrv.CString(msg.Msg[:]); len(got) < 20 || got[:7] != "Cupom [" {
		t.Errorf("mensagem do painel = %q", got)
	}
}

func TestListaDePendentesSoParaAdministrador(t *testing.T) {
	c := novoCenarioGM(t)
	fc := c.comTudo(t)
	pede := func() {
		buf, _ := netsrv.Marshal(CodeGMRequestPending, packets.SendRequestGMUsernames{})
		if err := c.h.gmRequestPending(context.Background(), c.eg.conn, c.gm, asPacket(t, buf)); err != nil {
			t.Fatal(err)
		}
	}
	c.entra(t, "gm")
	fc.CreateCommand(context.Background(), store.GMCommand{Type: store.GMCmdLevel, Target: "Alvo", ItemCount: 2, Command: "x", CreatedAt: agora})
	pede()
	c.eg.nada(t, 30*time.Millisecond)
	c.entra(t, "admin")
	pede()
	linha := decodifica[packets.CoupomReceive](t, proximoDe(t, c.eg, 0x322B))
	if linha.CommandID != 1 || linha.CommandType != store.GMCmdLevel || netsrv.CString(linha.TargetName[:]) != "Alvo" || linha.TargetItemCnt != 2 {
		t.Errorf("linha = %+v", linha)
	}
	fc.cmds[0].Runned = true
	pede()
	msg := decodifica[packets.MessageToPainelPacket](t, proximoDe(t, c.eg, CodeGMPanelMessage))
	if netsrv.CString(msg.Msg[:]) != "Não foram encontrados registros para autorização." {
		t.Errorf("mensagem = %q", netsrv.CString(msg.Msg[:]))
	}
}

func TestAprovarComandos(t *testing.T) {
	c := novoCenarioGM(t)
	fc := c.comTudo(t)
	ctx := context.Background()
	fc.CreateCommand(ctx, store.GMCommand{Type: store.GMCmdLevel, Target: "Alvo", ItemCount: 2, Command: "x"})
	fc.CreateCommand(ctx, store.GMCommand{Type: store.GMCmdLevel, Target: "Fora", ItemCount: 5, Command: "x"})
	fc.CreateCommand(ctx, store.GMCommand{Type: store.GMCmdItem, Target: "Alvo", ItemID: 900, ItemCount: 3, Command: "x"})
	fc.CreateCommand(ctx, store.GMCommand{Type: store.GMCmdCoupon, Command: "AAAA-BBBB-CCCC-DDDD", ItemID: 50})
	fc.CreateCommand(ctx, store.GMCommand{Type: store.GMCmdLevel, Target: "Fantasma", ItemCount: 1, Command: "x"})
	c.alvo.Avatar.Level, c.alvo.Avatar.Exp = 50, 4900
	aprova := func(id uint32) {
		p := packets.AcceptCommandPacket{CommandID: id}
		netsrv.PutCString(p.Reason[:], "ok")
		c.manda(t, c.h.gmApprove, CodeGMApprove, p)
	}
	c.entra(t, "gm")
	aprova(1)
	if fc.cmds[0].Runned {
		t.Fatal("game master nao aprova")
	}
	c.entra(t, "admin")
	aprova(1) // nivel ao vivo
	if c.alvo.Avatar.Level != 52 || !fc.cmds[0].Runned {
		t.Errorf("nivel ao vivo: %d", c.alvo.Avatar.Level)
	}
	aprova(2) // nivel fora do jogo: 40 + 5
	if got := fc.setados[77]; got[0] != 45 || got[1] != 4401 {
		t.Errorf("nivel gravado = %v", got)
	}
	aprova(3) // item de evento a quem esta online
	if len(c.ev.fila) != 1 || c.ev.fila[0].ItemID != 900 || c.ev.fila[0].Amount != 3 {
		t.Errorf("fila de eventos = %+v", c.ev.fila)
	}
	aprova(4) // cupom: so e reservado
	if !fc.cmds[3].Runned {
		t.Error("cupom deveria ficar aprovado")
	}
	aprova(5) // alvo que nao existe: continua pendente
	if fc.cmds[4].Runned {
		t.Error("alvo inexistente nao pode ser marcado como feito")
	}
	aprova(1) // de novo: nao aplica duas vezes
	if c.alvo.Avatar.Level != 52 {
		t.Errorf("aplicou duas vezes: %d", c.alvo.Avatar.Level)
	}
	aprova(99)
}

func TestReprovarComando(t *testing.T) {
	c := novoCenarioGM(t)
	fc := c.comTudo(t)
	fc.CreateCommand(context.Background(), store.GMCommand{Type: store.GMCmdCoupon, Command: "X"})
	rep := func() {
		p := packets.RefuseCommandPacket{CommandID: 1}
		c.manda(t, c.h.gmReprove, CodeGMReprove, p)
	}
	c.entra(t, "gm")
	rep()
	if fc.cmds[0].Refused {
		t.Fatal("game master nao reprova")
	}
	c.entra(t, "admin")
	rep()
	if !fc.cmds[0].Refused {
		t.Error("administrador reprova")
	}
}

func TestNomesDePainelEEfeito(t *testing.T) {
	c := novoCenarioGM(t)
	c.comTudo(t)
	c.entra(t, "gm")
	c.manda(t, c.h.gmRequestNames, CodeGMRequestNames, packets.SendRequestGMUsernames{Data: 1})
	c.eg.nada(t, 30*time.Millisecond)
	c.entra(t, "admin")
	c.manda(t, c.h.gmRequestNames, CodeGMRequestNames, packets.SendRequestGMUsernames{Data: 0})
	c.eg.nada(t, 30*time.Millisecond)
	c.manda(t, c.h.gmRequestNames, CodeGMRequestNames, packets.SendRequestGMUsernames{Data: 1})
	for _, want := range []string{"mod", "gm", "admin"} {
		n := decodifica[packets.SendGMUsernameToClient](t, proximoDe(t, c.eg, CodeGMNameRow))
		if got := netsrv.CString(n.GMUsernameTo[:]); got != want {
			t.Errorf("nome = %q, queria %q", got, want)
		}
	}
	c.manda(t, c.h.gmEffect, CodeGMEffect, packets.SendAddEffectGMPacket{TargetID: uint32(c.alvo.ClientID), EffectID: 4})
	ef := decodifica[packets.SendClientIndexPacket](t, proximoDe(t, c.ea, CodeSendEffect))
	if ef.Effect != 4 {
		t.Errorf("efeito = %d", ef.Effect)
	}
}
