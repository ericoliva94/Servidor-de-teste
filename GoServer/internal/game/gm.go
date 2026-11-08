package game

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos da ferramenta de painel (GM).
const (
	CodeGMLogin         = 0x3202
	CodeGMLoginReply    = 0x3203
	CodeGMMove          = 0x3204
	CodeGMChat          = 0x3205
	CodeGMGold          = 0x3206
	CodeGMCash          = 0x3207
	CodeGMDisconnect    = 0x3210
	CodeGMEventItem     = 0x3212
	CodeGMEventItemAll  = 0x3299
	gmMaxPasswordErrors = 5
)

// Niveis de privilegio de TMasterPrives.
const (
	PrivNone       = 0
	PrivModerator  = 3
	PrivGameMaster = 4
	PrivAdmin      = 5
)

// GMStore guarda as contas de painel e as acoes em massa.
type GMStore interface {
	ByUsername(ctx context.Context, username string) (store.GMAccount, bool, error)
	SetPasswordErrors(ctx context.Context, username string, n int) error
	Block(ctx context.Context, username string) error
	AddEventItemToAll(ctx context.Context, itemID, amount uint32) (int64, error)
	BanAccount(ctx context.Context, accountID uint32, days uint16, permanent bool) error
}

// gmSession e o estado de painel de uma conexao.
type gmSession struct {
	online   bool
	username string
	priv     int
}

func (s *Session) gmAtLeast(p int) bool { return s.gm.online && s.gm.priv >= p }

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// gmLogin e o porte de TPacketHandlers.CheckGMLogin ($3202 -> $3203).
//
// O cliente manda o MD5 da senha; a conta guarda a senha em texto (assim era na
// origem) e o servidor compara com o MD5 dela. Cinco erros seguidos bloqueiam a
// conta. Corrigido: a comparacao e de tempo constante, e a resposta de falha
// sai em todos os caminhos (a origem ficava calada quando o banco caia).
func (h *Handler) gmLogin(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.PacketLoginIntoServer
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	reply := func(r int32) error {
		out := packets.PacketLoginIntoServerResponse{Response: r}
		out.Header.Index = s.ClientID
		return c.SendStruct(CodeGMLoginReply, out)
	}
	user := strings.TrimSpace(netsrv.CString(pkt.Username[:]))
	if user == "" || h.GMDB == nil {
		return reply(-1)
	}
	acc, found, err := h.GMDB.ByUsername(ctx, user)
	if err != nil {
		h.log().Error("login de painel", "usuario", user, "erro", err)
		return reply(-1)
	}
	if !found {
		h.log().Warn("login de painel: usuario nao encontrado", "usuario", user, "ip", c.IP())
		return reply(-1)
	}
	if acc.Status == 0 {
		h.log().Warn("login de painel: conta bloqueada por erros de senha", "usuario", user, "ip", c.IP())
		return reply(-1)
	}
	enviada := strings.ToLower(strings.TrimSpace(netsrv.CString(pkt.Password[:])))
	if subtle.ConstantTimeCompare([]byte(enviada), []byte(md5Hex(acc.Password))) != 1 {
		if acc.PasswordErrors >= gmMaxPasswordErrors {
			_ = h.GMDB.Block(ctx, user)
		} else {
			_ = h.GMDB.SetPasswordErrors(ctx, user, acc.PasswordErrors+1)
		}
		h.log().Warn("login de painel: senha errada", "usuario", user, "ip", c.IP())
		return reply(-1)
	}
	_ = h.GMDB.SetPasswordErrors(ctx, user, 0)
	s.gm = gmSession{online: true, username: user, priv: acc.MasterPriv}
	h.log().Info("super usuario logado", "usuario", user, "privilegio", acc.MasterPriv, "ip", c.IP())
	return reply(1)
}

// gmTargets acha o alvo de uma acao de painel: o nome, se veio, em qualquer
// canal; senao o ClientID no canal do GM. Quem manda so atinge jogadores em jogo.
func (h *Handler) gmTarget(id uint16, nick []byte) (*Session, *netsrv.Conn, *Handler, bool) {
	nome := strings.TrimSpace(netsrv.CString(nick))
	pares := h.Channels
	if len(pares) == 0 {
		pares = []*Handler{h}
	}
	if nome == "" {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || s.Status < Playing {
			return nil, nil, nil, false
		}
		return s, c, h, true
	}
	for _, o := range pares {
		if o.Chan == nil {
			continue
		}
		cid, ok := o.Chan.FindByCharacter(nome)
		if !ok {
			continue
		}
		if s, c, ok := o.Chan.Lookup(cid); ok && s.Status >= Playing {
			return s, c, o, true
		}
	}
	return nil, nil, nil, false
}

func (h *Handler) gmLog(s *Session, acao, alvo string) {
	h.log().Info("painel", "conta", s.gm.username, "personagem", s.Avatar.Name, "acao", acao, "alvo", alvo)
}

// gmMove e o porte de GMPlayerMove ($3204): moderador ou acima teleporta um
// jogador. O alvo e sempre pelo nome.
func (h *Handler) gmMove(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.MovePacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivModerator) || !ValidPosition(pkt.Position) {
		return nil
	}
	nome := strings.TrimSpace(netsrv.CString(pkt.NickName[:]))
	if nome == "" {
		return nil
	}
	alvo, ac, ah, ok := h.gmTarget(0, pkt.NickName[:])
	if !ok {
		return nil
	}
	ah.teleportTo(alvo, pkt.Position)
	if alvo != s {
		ah.message(ac, "Você foi teleportado pelo <"+s.Avatar.Name+"> com atributos de Administrador.")
	}
	h.gmLog(s, fmt.Sprintf("mover para %v,%v", pkt.Position.X, pkt.Position.Y), alvo.Avatar.Name)
	return nil
}

// gmChat e o porte de GMSendChat ($3205): mensagem de servidor ao canal, ou a
// todos os canais com Type2 = 1. O texto vai no formato de string curta do
// Delphi (o primeiro byte e o tamanho) e e cortado em 90.
func (h *Handler) gmChat(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.ClientMessagePacket2
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivModerator) {
		return nil
	}
	n := int(pkt.Msg[0])
	if n > len(pkt.Msg)-1 {
		n = len(pkt.Msg) - 1
	}
	texto := string(pkt.Msg[1 : 1+n])
	if len(texto) > 90 {
		texto = texto[:90]
	}
	msg := "<[GameMaster] " + s.Avatar.Name + "> " + texto
	alvos := []*Handler{h}
	if pkt.Type2 == 1 && len(h.Channels) > 0 {
		alvos = h.Channels
	}
	for _, o := range alvos {
		o.broadcastServerMessage(msg, pkt.Type1)
	}
	h.gmLog(s, "gritar", texto)
	return nil
}

// broadcastServerMessage manda uma mensagem a todo mundo em jogo no canal.
func (h *Handler) broadcastServerMessage(msg string, tipo uint8) {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		if _, c, ok := h.Chan.Lookup(id); ok {
			h.messageType(c, msg, tipo)
		}
	}
}

// gmGold e o porte de GMGoldManagment ($3206). So administrador. Soma, tira ou
// zera o ouro do alvo, com o teto de ouro do porte.
func (h *Handler) gmGold(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendGoldAddRemove
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) {
		return nil
	}
	alvo, ac, ah, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:])
	if !ok {
		return nil
	}
	quantia := int64(min(pkt.Gold, uint64(MaxGold)))
	switch {
	case pkt.Add != 0:
		if quantia > MaxGold-alvo.Avatar.Gold {
			alvo.Avatar.Gold = MaxGold
		} else {
			alvo.Avatar.Gold += quantia
		}
		h.gmLog(s, fmt.Sprintf("criar ouro %d", quantia), alvo.Avatar.Name)
	case pkt.RemoveAllGold != 0:
		alvo.Avatar.Gold = 0
		h.gmLog(s, "zerar ouro", alvo.Avatar.Name)
	default:
		alvo.Avatar.Gold = max(alvo.Avatar.Gold-quantia, 0)
		h.gmLog(s, fmt.Sprintf("remover ouro %d", quantia), alvo.Avatar.Name)
	}
	ah.sendRefreshMoney(ac, alvo)
	return nil
}

// gmCash e o porte de GMCashManagment ($3207). So administrador.
func (h *Handler) gmCash(ctx context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendCashAddRemove
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) {
		return nil
	}
	alvo, ac, ah, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:])
	if !ok {
		return nil
	}
	const teto = uint64(^uint32(0))
	quantia := min(pkt.Cash, teto)
	saldo := uint64(alvo.CashBalance)
	switch {
	case pkt.Add != 0:
		saldo = min(saldo+quantia, teto)
		h.gmLog(s, fmt.Sprintf("criar cash %d", quantia), alvo.Avatar.Name)
	case pkt.RemoveAllCash != 0:
		saldo = 0
		h.gmLog(s, "zerar cash", alvo.Avatar.Name)
	default:
		if quantia >= saldo {
			saldo = 0
		} else {
			saldo -= quantia
		}
		h.gmLog(s, fmt.Sprintf("remover cash %d", quantia), alvo.Avatar.Name)
	}
	alvo.CashBalance = uint32(saldo)
	ah.persistCash(ctx, alvo)
	ah.sendPlayerCash(ac, alvo)
	return nil
}

// gmDisconnect e o porte de GMDisconnect ($3210): game master ou acima derruba
// o jogador. A origem, pelo nome, derrubava todo jogador de mesmo nome em todos
// os canais; aqui e o primeiro em jogo.
func (h *Handler) gmDisconnect(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendDisconnect
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivGameMaster) {
		return nil
	}
	alvo, ac, _, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:])
	if !ok {
		return nil
	}
	ac.Close()
	h.gmLog(s, "desconectar", alvo.Avatar.Name)
	return nil
}

// gmEventItem e o porte de GMEventItem ($3212): administrador poe um item na
// fila de eventos de um jogador em jogo.
func (h *Handler) gmEventItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendItemEventOne
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) {
		return nil
	}
	if pkt.ItemID == 0 || pkt.ItemID > 65535 || pkt.Amount == 0 || pkt.Amount > 65535 {
		h.messageType(c, "Item ou quantidade inválidos.", 16)
		return nil
	}
	alvo, _, ah, ok := h.gmTarget(uint16(min(pkt.TargetID, 65535)), pkt.Nickname[:])
	if !ok {
		h.messageType(c, "Jogador não encontrado.", 16)
		return nil
	}
	if ah.EventDB == nil {
		return nil
	}
	if err := ah.EventDB.Add(ctx, ah.selectedCharID(alvo), uint16(pkt.ItemID), uint16(pkt.Amount)); err != nil {
		h.log().Error("item de evento do painel", "erro", err)
		return nil
	}
	h.messageType(c, `Item de Evento "T" enviado com sucesso.`, 32)
	h.gmLog(s, fmt.Sprintf("item de evento %d x%d", pkt.ItemID, pkt.Amount), alvo.Avatar.Name)
	return nil
}

// gmEventItemAll e o porte de GMEventItemForAll ($3299): administrador poe um
// item na fila de eventos de todos os personagens.
func (h *Handler) gmEventItemAll(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendItemEventForAll
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) || h.GMDB == nil {
		return nil
	}
	if pkt.ItemID == 0 || pkt.ItemID > 65535 || pkt.Amount == 0 || pkt.Amount > 65535 {
		h.messageType(c, "Item ou quantidade inválidos.", 16)
		return nil
	}
	n, err := h.GMDB.AddEventItemToAll(ctx, pkt.ItemID, pkt.Amount)
	if err != nil {
		h.log().Error("item de evento para todos", "erro", err)
		return nil
	}
	h.messageType(c, `Item de Evento "T" enviado PARA TODOS com sucesso.`, 32)
	h.gmLog(s, fmt.Sprintf("item de evento para todos %d x%d", pkt.ItemID, pkt.Amount), fmt.Sprintf("%d personagens", n))
	return nil
}

// Codigos da fatia 2 do painel.
const (
	CodeGMLevel = 0x3208
	CodeGMBuffs = 0x3209
	CodeGMBan   = 0x3211
	// gmBanPermanent e o numero de dias a partir do qual o banimento e
	// permanente (Packet.Days >= 1000 na origem).
	gmBanPermanent = 1000
)

// gmLevel e o porte de GMLevelManagment ($3208) para o administrador.
//
// Somar niveis leva o alvo ao nivel (nivel DELE + N), como o comando /addlevel;
// a origem lia o nivel de quem mandava, indexava ExpList sem conferir e podia
// dar a volta no UInt64. Tirar niveis baixa o nivel e poe a experiencia na
// linha do nivel novo; zerar volta ao nivel 1 com experiencia 1.
// Nao portado: o ramo de quem nao e administrador, que escrevia SQL em texto em
// gm_commands para aprovacao (ver KNOWN_ISSUES.md).
func (h *Handler) gmLevel(ctx context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendLevelAddRemove
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if s.gmAtLeast(PrivGameMaster) && !s.gmAtLeast(PrivAdmin) {
		return h.gmRequestLevel(ctx, pkt, s)
	}
	if !s.gmAtLeast(PrivAdmin) || h.Exp == nil {
		return nil
	}
	alvo, ac, ah, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:])
	if !ok {
		return nil
	}
	n := int64(pkt.Level)
	switch {
	case pkt.Add != 0:
		if n <= 0 {
			return nil
		}
		// ExpList[nivel + N - 1] + 1 da o nivel + N (o valor logo acima da linha).
		destino := int64(alvo.Avatar.Level) + n - 1
		exp, found := ah.Exp.ExpForLevel(uint16(min(destino, 65535)))
		if !found {
			return nil
		}
		if falta := int64(exp) + 1 - alvo.Avatar.Exp; falta > 0 {
			ah.addExp(ac, alvo, falta, ExpTypeNormal)
		}
		h.gmLog(s, fmt.Sprintf("criar nivel +%d", n), alvo.Avatar.Name)
	default:
		novo := int64(1)
		if pkt.RemoveAllLevel == 0 && int64(alvo.Avatar.Level)-n > 0 {
			novo = int64(alvo.Avatar.Level) - n
		}
		alvo.Avatar.Level = uint16(novo)
		alvo.Avatar.Exp = 1
		if novo > 1 {
			if exp, found := ah.Exp.ExpForLevel(uint16(novo)); found {
				alvo.Avatar.Exp = int64(exp)
			}
		}
		alvo.Avatar.ApplyEquipBonus(ah.data(), alvo.Equip)
		alvo.Avatar.CurHP = min(alvo.Avatar.CurHP, alvo.Avatar.MaxHP)
		alvo.Avatar.CurMP = min(alvo.Avatar.CurMP, alvo.Avatar.MaxMP)
		ah.sendStatus(ac, alvo)
		ah.sendRefreshLevel(ac, alvo)
		h.gmLog(s, fmt.Sprintf("tirar nivel (agora %d)", novo), alvo.Avatar.Name)
	}
	return nil
}

// gmBuffs e o porte de GMBuffsManagment ($3209): o administrador poe, tira ou
// zera buffs de um jogador do canal, por ClientID.
func (h *Handler) gmBuffs(_ context.Context, _ *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendBuffAddRemove
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) {
		return nil
	}
	alvo, ac, _, ok := h.gmTarget(pkt.TargetID, nil)
	if !ok {
		return nil
	}
	switch {
	case pkt.Add == 1:
		h.addBuff(alvo, pkt.BuffID)
		h.gmLog(s, fmt.Sprintf("buffar %d", pkt.BuffID), alvo.Avatar.Name)
	case pkt.RemoveAllBuff != 1:
		h.removeBuffAndNotify(alvo, pkt.BuffID)
		h.gmLog(s, fmt.Sprintf("tirar buff %d", pkt.BuffID), alvo.Avatar.Name)
	default:
		h.clearBuffs(alvo)
		alvo.Avatar.ApplyEquipBonus(h.data(), alvo.Equip)
		h.sendStatus(ac, alvo)
		h.sendRefreshPoint(ac, alvo)
		h.gmLog(s, "zerar buffs", alvo.Avatar.Name)
	}
	return nil
}

// gmBan e o porte de GMBan ($3211): game master ou acima bane a conta do
// jogador (permanente a partir de 1000 dias), avisa todos os canais e derruba a
// conexao.
//
// Corrigido: a origem so mudava o AccountStatus em memoria, e o banco so ficava
// sabendo se algum outro caminho gravasse a conta; aqui o banimento e gravado na
// hora, e sem gravar nao se derruba nem se avisa.
func (h *Handler) gmBan(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendBan
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivGameMaster) || h.GMDB == nil {
		return nil
	}
	alvo, ac, _, ok := h.gmTarget(pkt.TargetID, pkt.Nickname[:])
	if !ok || alvo.Account.ID == 0 {
		return nil
	}
	if alvo == s {
		h.messageType(c, "Você não pode banir a si mesmo.", 16)
		return nil
	}
	permanente := pkt.Days >= gmBanPermanent
	if pkt.Days == 0 {
		return nil
	}
	if err := h.GMDB.BanAccount(ctx, alvo.Account.ID, pkt.Days, permanente); err != nil {
		h.log().Error("banir conta", "conta", alvo.Account.ID, "erro", err)
		return nil
	}
	motivo := strings.TrimSpace(netsrv.CString(pkt.Reason[:]))
	duracao := fmt.Sprintf("por %d dias", pkt.Days)
	if permanente {
		duracao = "permanentemente"
	}
	aviso := "O jogador <" + alvo.Avatar.Name + "> foi banido " + duracao + "."
	todos := h.Channels
	if len(todos) == 0 {
		todos = []*Handler{h}
	}
	for _, o := range todos {
		if motivo == "" {
			o.broadcastServerMessage(aviso+" Motivo do ban não apresentado.", 16)
			continue
		}
		o.broadcastServerMessage(aviso+" Motivo do ban: ", 16)
		o.broadcastServerMessage(motivo, 16)
	}
	ac.Close()
	h.gmLog(s, fmt.Sprintf("banir %s motivo %q", duracao, motivo), alvo.Avatar.Name)
	return nil
}
