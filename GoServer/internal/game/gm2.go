package game

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos do fluxo de aprovacao do painel.
const (
	CodeGMRequestPending = 0x3229 // painel -> servidor
	CodeGMPendingRow     = 0x322B // servidor -> painel
	CodeGMRequestNames   = 0x322D
	CodeGMNameRow        = 0x322F
	CodeGMPanelMessage   = 0x3232
	CodeGMReprove        = 0x3234
	CodeGMApprove        = 0x3236
	CodeGMEffect         = 0x3238
	CodeGMCreateCoupon   = 0x323A
)

// GMCommandStore guarda os pedidos que esperam aprovacao.
type GMCommandStore interface {
	GMIDByUsername(ctx context.Context, username string) (uint32, bool, error)
	CreateCommand(ctx context.Context, c store.GMCommand) (uint32, error)
	PendingCommands(ctx context.Context) ([]store.GMCommand, error)
	ClaimCommand(ctx context.Context, id uint32, by, reason string, now time.Time) (store.GMCommand, bool, error)
	RefuseCommand(ctx context.Context, id uint32, by, reason string, now time.Time) (bool, error)
	OfflineCharacter(ctx context.Context, name string) (uint32, uint16, bool, error)
	SetLevelExp(ctx context.Context, charID uint32, level uint16, exp uint64) error
	GMUsernames(ctx context.Context) ([]string, error)
}

// delphiDate converte para TDateTime (dias desde 30/12/1899).
func delphiDate(t time.Time) float64 {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return t.UTC().Sub(base).Hours() / 24
}

// panelMessage e SendMessageToPainel ($3232).
func (h *Handler) panelMessage(c *netsrv.Conn, s *Session, msg string) {
	if c == nil {
		return
	}
	var pkt packets.MessageToPainelPacket
	pkt.Header.Index = s.ClientID
	pkt.Null, pkt.Type1 = 10, 16
	copy(pkt.Msg[:len(pkt.Msg)-1], msg)
	_ = c.SendStruct(CodeGMPanelMessage, pkt)
}

// gmRequestPending e o porte de GMRequestCommandsAutoriz ($3229): manda ao
// painel um $322B por pedido pendente. Corrigido: a origem nao conferia o
// privilegio (qualquer conexao via os pedidos) e lia uma coluna "username" que
// gm_commands nao tem; aqui so administrador e o nome vem de gm_accounts.
func (h *Handler) gmRequestPending(ctx context.Context, c *netsrv.Conn, s *Session, _ protocol.Packet) error {
	if !s.gmAtLeast(PrivAdmin) || h.GMCmds == nil {
		return nil
	}
	lista, err := h.GMCmds.PendingCommands(ctx)
	if err != nil {
		h.log().Error("comandos pendentes", "erro", err)
		return nil
	}
	if len(lista) == 0 {
		h.panelMessage(c, s, "Não foram encontrados registros para autorização.")
		return nil
	}
	for _, cmd := range lista {
		var pkt packets.CoupomReceive
		pkt.Header.Index = s.ClientID
		pkt.CommandID = int32(cmd.ID)
		pkt.GMID = cmd.OwnerID
		netsrv.PutCString(pkt.GMUsername[:], cmd.OwnerName)
		pkt.CommandType = uint16(cmd.Type)
		netsrv.PutCString(pkt.CommandSQL[:], cmd.Command)
		pkt.CreatedAt = delphiDate(cmd.CreatedAt)
		netsrv.PutCString(pkt.TargetName[:], cmd.Target)
		pkt.TargetItemID, pkt.TargetItemCnt = uint16(min(cmd.ItemID, 65535)), uint16(min(cmd.ItemCount, 65535))
		netsrv.PutCString(pkt.ReasonCreate[:], cmd.Reason)
		_ = c.SendStruct(CodeGMPendingRow, pkt)
	}
	return nil
}

// gmGiveLevels leva o alvo N niveis acima do nivel dele.
func (h *Handler) gmGiveLevels(ac *netsrv.Conn, alvo *Session, n int64) bool {
	if h.Exp == nil || n <= 0 {
		return false
	}
	exp, ok := h.Exp.ExpForLevel(uint16(min(int64(alvo.Avatar.Level)+n-1, 65535)))
	if !ok {
		return false
	}
	if falta := int64(exp) + 1 - alvo.Avatar.Exp; falta > 0 {
		h.addExp(ac, alvo, falta, ExpTypeNormal)
	}
	return true
}

// gmApprove e o porte de GMApproveCommand ($3236). So administrador.
//
// Os tipos sao fechados: nivel (1), item de evento (2) e cupom (3 e 4). O texto
// que a origem guardava em command NAO e executado — a aprovacao so aplica o
// tipo, o alvo e a quantidade. Corrigido: o pedido e reservado antes de aplicar
// (dois administradores nao o aplicam duas vezes), e um alvo que nao existe
// deixa o pedido pendente em vez de marca-lo como feito.
func (h *Handler) gmApprove(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.AcceptCommandPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) || h.GMCmds == nil {
		return nil
	}
	motivo := strings.TrimSpace(netsrv.CString(pkt.Reason[:]))
	pend, err := h.GMCmds.PendingCommands(ctx)
	if err != nil {
		h.log().Error("aprovar comando", "erro", err)
		return nil
	}
	var cmd *store.GMCommand
	for i := range pend {
		if pend[i].ID == pkt.CommandID {
			cmd = &pend[i]
		}
	}
	if cmd == nil {
		return nil
	}
	var charID uint32
	var nivel uint16
	switch cmd.Type {
	case store.GMCmdLevel, store.GMCmdItem:
		id, lv, found, err := h.GMCmds.OfflineCharacter(ctx, cmd.Target)
		if err != nil || !found {
			h.panelMessage(c, s, "Personagem do comando não encontrado.")
			return nil
		}
		charID, nivel = id, lv
	case store.GMCmdCoupon, store.GMCmdFounder:
	default:
		h.panelMessage(c, s, "Tipo de comando desconhecido.")
		return nil
	}
	got, ok, err := h.GMCmds.ClaimCommand(ctx, pkt.CommandID, s.gm.username, motivo, h.now())
	if err != nil {
		h.log().Error("reservar comando", "erro", err)
		return nil
	}
	if !ok {
		return nil // outro administrador chegou antes
	}
	switch got.Type {
	case store.GMCmdLevel:
		if alvo, ac, ah, on := h.gmTarget(0, []byte(cmd.Target)); on {
			ah.gmGiveLevels(ac, alvo, got.ItemCount)
		} else if h.Exp != nil {
			novo := min(int64(nivel)+got.ItemCount, int64(LevelCap))
			exp, _ := h.Exp.ExpForLevel(uint16(max(novo-1, 0)))
			if err := h.GMCmds.SetLevelExp(ctx, charID, uint16(novo), exp+1); err != nil {
				h.log().Error("nivel fora do jogo", "erro", err)
			}
		}
	case store.GMCmdItem:
		if h.EventDB != nil {
			if err := h.EventDB.Add(ctx, charID, uint16(min(got.ItemID, 65535)), uint16(min(got.ItemCount, 65535))); err != nil {
				h.log().Error("item de evento aprovado", "erro", err)
			}
		}
		if _, ac, ah, on := h.gmTarget(0, []byte(cmd.Target)); on {
			ah.message(ac, "Você recebeu um item de evento. Pressione T.")
		}
	}
	h.gmLog(s, fmt.Sprintf("aprovar comando %d (tipo %d)", got.ID, got.Type), got.Target)
	h.panelMessage(c, s, "Comando APROVADO com sucesso.")
	return nil
}

// gmReprove e o porte de GMReproveCommand ($3234). So administrador.
func (h *Handler) gmReprove(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.RefuseCommandPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) || h.GMCmds == nil {
		return nil
	}
	motivo := strings.TrimSpace(netsrv.CString(pkt.Reason[:]))
	ok, err := h.GMCmds.RefuseCommand(ctx, pkt.CommandID, s.gm.username, motivo, h.now())
	if err != nil {
		h.log().Error("recusar comando", "erro", err)
		return nil
	}
	if ok {
		h.gmLog(s, fmt.Sprintf("reprovar comando %d", pkt.CommandID), "")
		h.panelMessage(c, s, "Comando REPROVADO com sucesso.")
	}
	return nil
}

// couponCodeNew sorteia um codigo AAAA-BBBB-CCCC-DDDD. A origem usava o MD5 da
// hora atual, que qualquer um reproduz sabendo o minuto do pedido.
func couponCodeNew() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	h := strings.ToUpper(fmt.Sprintf("%x", b))
	return h[0:4] + "-" + h[4:8] + "-" + h[8:12] + "-" + h[12:16], nil
}

// gmCreateCoupon e o porte de GMRequestCreateCoupom ($323A): moderador ou
// acima pede um cupom (item de fundador ou PinCode de 10/20/50/100 de AikaCash)
// que so vale depois da aprovacao de um administrador. O pedido sai em nome da
// conta de painel logada, nao do nome que o pacote traz.
func (h *Handler) gmCreateCoupon(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendCreateCoupomRequest
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivModerator) || h.GMCmds == nil {
		return nil
	}
	var tipo int
	var produto, qtd int64
	switch {
	case pkt.ItemID > 0:
		tipo, produto, qtd = store.GMCmdFounder, int64(pkt.ItemID), int64(max(pkt.ItemAmount, 1))
		if pkt.ItemID > 65535 || qtd > 65535 {
			return nil
		}
	case pkt.PinCodeType <= 3:
		tipo, produto = store.GMCmdCoupon, []int64{10, 20, 50, 100}[pkt.PinCodeType]
	default:
		return nil
	}
	gmid, ok, err := h.GMCmds.GMIDByUsername(ctx, s.gm.username)
	if err != nil || !ok {
		return nil
	}
	codigo, err := couponCodeNew()
	if err != nil {
		return err
	}
	if _, err := h.GMCmds.CreateCommand(ctx, store.GMCommand{
		OwnerID: gmid, Type: tipo, Command: codigo, CreatedAt: h.now(),
		ItemID: produto, ItemCount: qtd, Reason: strings.TrimSpace(netsrv.CString(pkt.Reason[:])),
	}); err != nil {
		h.log().Error("criar cupom", "erro", err)
		return nil
	}
	h.gmLog(s, "pedir cupom "+codigo, fmt.Sprintf("tipo %d produto %d", tipo, produto))
	h.panelMessage(c, s, "Cupom ["+codigo+"] registrado com sucesso.\r\nAguardando aprovação do administrador para cupom ter validade.")
	return nil
}

// gmRequestNames e o porte de GMRequestGMUsernames ($322D): administrador pede
// a lista de contas de painel (um $322F por conta).
func (h *Handler) gmRequestNames(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendRequestGMUsernames
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) || pkt.Data != 1 || h.GMCmds == nil {
		return nil
	}
	nomes, err := h.GMCmds.GMUsernames(ctx)
	if err != nil {
		return nil
	}
	for _, n := range nomes {
		var out packets.SendGMUsernameToClient
		out.Header.Index = s.ClientID
		netsrv.PutCString(out.GMUsernameTo[:], n)
		_ = c.SendStruct(CodeGMNameRow, out)
	}
	return nil
}

// gmEffect e o porte de GMSendAddEffect ($3238): game master faz um efeito
// aparecer num jogador do canal (ele mesmo com TargetID 0).
func (h *Handler) gmEffect(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendAddEffectGMPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivGameMaster) {
		return nil
	}
	alvo := s
	if pkt.TargetID != 0 {
		o, _, ok := h.Chan.Lookup(uint16(min(pkt.TargetID, 65535)))
		if !ok || o.Status < Playing {
			return nil
		}
		alvo = o
	}
	h.sendEffect(alvo, pkt.EffectID)
	return nil
}

// gmRequestLevel e o ramo de GMLevelManagment para quem e game master mas nao
// administrador: em vez de mexer no nivel, deixa um pedido para um
// administrador aprovar (a origem montava um texto SQL; aqui o pedido e tipado:
// nome do alvo e numero de niveis). So o pedido de somar niveis vale.
func (h *Handler) gmRequestLevel(ctx context.Context, pkt packets.SendLevelAddRemove, s *Session) error {
	if h.GMCmds == nil || pkt.Add == 0 || pkt.Level == 0 {
		return nil
	}
	nome := ""
	if alvo, _, _, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:]); ok {
		nome = alvo.Avatar.Name
	} else {
		nome = strings.TrimSpace(netsrv.CString(pkt.Nickname[:]))
	}
	if nome == "" {
		return nil
	}
	gmid, ok, err := h.GMCmds.GMIDByUsername(ctx, s.gm.username)
	if err != nil || !ok {
		return nil
	}
	_, err = h.GMCmds.CreateCommand(ctx, store.GMCommand{
		OwnerID: gmid, Type: store.GMCmdLevel, CreatedAt: h.now(), Target: nome,
		ItemCount: int64(pkt.Level), Command: fmt.Sprintf("Somar %d nivel(is) a %s", pkt.Level, nome),
	})
	if err != nil {
		h.log().Error("pedido de nivel", "erro", err)
		return nil
	}
	h.gmLog(s, fmt.Sprintf("pedir +%d niveis", pkt.Level), nome)
	return nil
}
