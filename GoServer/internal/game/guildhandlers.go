package game

import (
	"context"
	"strings"

	"aika/internal/guild"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de guilda.
const (
	CodeCreateGuild       = 0x341 // cliente -> servidor: criar
	CodeInviteGuild       = 0xF7B // cliente -> servidor: convidar
	CodeInviteGuildAccept = 0x67D // cliente -> servidor: aceitar
	CodeInviteGuildDeny   = 0xF7E // cliente -> servidor: recusar
	CodeKickGuild         = 0xF2D // cliente -> servidor: expulsar
	CodeExitGuild         = 0xF1C // cliente -> servidor: sair
	CodeGuildRank         = 0xF1D // cliente -> servidor: mudar rank
	CodeGuildNotices      = 0xF20 // nos dois sentidos: avisos
	CodeGuildSite         = 0xF21 // nos dois sentidos: site
	CodeGuildRanksConfig  = 0xF22 // nos dois sentidos: permissoes
	CodeGuildMaster       = 0x619 // cliente -> servidor: passar mestre

	CodeGuildInfo      = 0x965 // servidor -> cliente: dados da guilda
	CodeGuildPlayers   = 0x97F // servidor -> cliente: lista de membros
	CodeAddGuildPlayer = 0x125 // servidor -> cliente: membro novo
	CodeGuildMemberOut = 0x96A // servidor -> cliente: membro saiu
)

// GuildCreateCost e o custo em ouro para criar uma guilda.
//
// A origem lia esse valor de um dialogo de NPC (CreateGuildDialogPacket traz
// o campo Rate), que ainda nao esta portado; ate lá vale o valor do .ini.
const GuildCreateCost = 1000000

// MembersPerPacket e quantos membros cabem no $97F.
const MembersPerPacket = 10

// createGuild e o porte de TPacketHandlers.CreateGuild ($341).
func (h *Handler) createGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	if s.Avatar.GuildIndex != 0 || h.Guilds.Of(s.CharID()) != nil {
		h.message(c, "Voce ja esta em uma guilda.")
		return nil
	}

	var pkt packets.CreateGuildDialogPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := strings.TrimSpace(netsrv.CString(pkt.GuildName[:]))
	if nome == "" {
		h.message(c, "Nome de guilda invalido.")
		return nil
	}
	if s.Avatar.Gold < h.guildCost() {
		h.message(c, "Nao possui ouro suficiente para criar a guilda.")
		return nil
	}

	g, err := h.Guilds.Create(nome, uint8(min(s.Avatar.Nation, 255)), guild.Member{
		CharIndex: s.CharID(),
		Name:      s.Avatar.Name,
		ClassInfo: uint8(min(s.Avatar.ClassInfo, 255)),
		Level:     uint8(min(s.Avatar.Level, 255)),
		Logged:    true,
		LastLogin: h.now(),
	})
	if err != nil {
		switch err {
		case guild.ErrNameTaken:
			h.message(c, "Ja existe uma guilda com esse nome.")
		case guild.ErrInvalidName:
			h.message(c, "Nome de guilda invalido.")
		default:
			h.message(c, "Nao foi possivel criar a guilda.")
		}
		return nil
	}

	s.Avatar.Gold = subGold(s.Avatar.Gold, h.guildCost())
	s.Avatar.GuildIndex = g.Index
	s.Avatar.GuildName = g.Name

	h.sendGuildInfo(c, s, g)
	h.sendGuildPlayers(c, s, g)
	h.respawnToVisible(s)
	h.log().Info("guilda criada",
		"guilda", g.Name, "indice", g.Index, "mestre", s.Avatar.Name)
	return nil
}

// guildCost devolve o custo de criar guilda.
func (h *Handler) guildCost() int64 {
	if h.GuildCost > 0 {
		return h.GuildCost
	}
	return GuildCreateCost
}

// inviteGuild e o porte de TPacketHandlers.InviteToGuildRequest ($F7B).
func (h *Handler) inviteGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil || h.Chan == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		h.message(c, "Voce nao esta em uma guilda.")
		return nil
	}
	if !g.CanOf(s.CharID()).Invite {
		h.message(c, "Voce nao tem permissao para isso.")
		return nil
	}
	if g.TotalMembers() >= g.Capacity() {
		h.message(c, "A guilda nao tem mais espaco.")
		return nil
	}

	var pkt packets.InviteToGuildPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := strings.TrimSpace(netsrv.CString(pkt.InviterNick[:]))
	id, ok := h.Chan.FindByCharacter(nome)
	if !ok {
		h.message(c, "Personagem nao encontrado.")
		return nil
	}
	alvo, alvoConn, ok := h.Chan.Lookup(id)
	if !ok || alvo.Status < Playing || alvo.ClientID == s.ClientID {
		h.message(c, "Personagem nao encontrado.")
		return nil
	}
	if h.Guilds.Of(alvo.CharID()) != nil {
		h.message(c, "O jogador ja esta em uma guilda.")
		return nil
	}

	alvo.GuildRequester = s.ClientID
	var convite packets.InviteToGuildPacket
	convite.Header.Index = s.ClientID
	netsrv.PutCString(convite.InviterNick[:], s.Avatar.Name)
	if err := alvoConn.SendStruct(CodeInviteGuild, convite); err != nil {
		h.log().Debug("falha ao enviar convite de guilda",
			"de", s.Avatar.Name, "erro", err)
	}
	return nil
}

// acceptGuild e o porte de TPacketHandlers.InviteToGuildAccept ($67D).
func (h *Handler) acceptGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil || h.Chan == nil {
		return nil
	}
	if s.GuildRequester == 0 {
		return nil
	}
	// O convite vale uma vez, como no grupo.
	quem := s.GuildRequester
	s.GuildRequester = 0

	convidou, convidouConn, ok := h.Chan.Lookup(quem)
	if !ok || convidou.Status < Playing {
		return nil
	}
	g := h.Guilds.Of(convidou.CharID())
	if g == nil {
		h.message(c, "A guilda nao existe mais.")
		return nil
	}

	membro := guild.Member{
		CharIndex: s.CharID(),
		Name:      s.Avatar.Name,
		Rank:      guild.MaxRanks - 1, // entra no rank mais baixo
		ClassInfo: uint8(min(s.Avatar.ClassInfo, 255)),
		Level:     uint8(min(s.Avatar.Level, 255)),
		Logged:    true,
		LastLogin: h.now(),
	}
	if _, err := h.Guilds.Join(g, membro); err != nil {
		switch err {
		case guild.ErrGuildFull:
			h.message(convidouConn, "A guilda nao tem mais espaco.")
		case guild.ErrAlreadyInGuild:
			h.message(convidouConn, "O jogador ja esta em uma guilda.")
		default:
			h.message(convidouConn, "Nao foi possivel adicionar o jogador.")
		}
		return nil
	}

	s.Avatar.GuildIndex = g.Index
	s.Avatar.GuildName = g.Name

	h.sendGuildInfo(c, s, g)
	h.broadcastGuild(g, nil)
	h.broadcastNewGuildMember(g, membro)
	h.respawnToVisible(s)
	h.log().Info("guilda",
		"acao", "entrou", "jogador", s.Avatar.Name,
		"guilda", g.Name, "membros", g.TotalMembers())
	return nil
}

// denyGuild e o porte de TPacketHandlers.InviteToGuildDeny ($F7E).
func (h *Handler) denyGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if s.GuildRequester == 0 || h.Chan == nil {
		return nil
	}
	quem := s.GuildRequester
	s.GuildRequester = 0
	if _, convidouConn, ok := h.Chan.Lookup(quem); ok {
		h.message(convidouConn, s.Avatar.Name+" recusou entrar na guilda.")
	}
	return nil
}

// kickGuild e o porte de TPacketHandlers.KickMemberOfGuild ($F2D).
func (h *Handler) kickGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if !g.CanOf(s.CharID()).Kick {
		h.message(c, "Voce nao tem permissao para isso.")
		return nil
	}

	var pkt packets.ChangeMasterGuild
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	alvo := pkt.CharIndex
	if alvo == 0 || alvo == s.CharID() {
		return nil
	}
	// O mestre nao pode ser expulso.
	if alvo == g.LeaderID {
		h.message(c, "O mestre da guilda nao pode ser expulso.")
		return nil
	}
	if g.FindMember(alvo) < 0 {
		return nil
	}

	restante, desfeita := h.Guilds.Leave(alvo)
	h.clearGuildOf(alvo)
	if !desfeita && restante != nil {
		h.broadcastGuild(restante, nil)
	}
	h.log().Info("guilda",
		"acao", "expulso", "personagem", alvo, "por", s.Avatar.Name)
	return nil
}

// exitGuild e o porte de TPacketHandlers.ExitGuild ($F1C).
func (h *Handler) exitGuild(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	if h.Guilds.Of(s.CharID()) == nil {
		return nil
	}

	restante, desfeita := h.Guilds.Leave(s.CharID())
	s.Avatar.GuildIndex = 0
	s.Avatar.GuildName = ""
	h.respawnToVisible(s)

	if desfeita {
		h.log().Info("guilda desfeita",
			"guilda", restante.Name, "pelo ultimo membro", s.Avatar.Name)
		return nil
	}
	if restante != nil {
		h.broadcastGuild(restante, nil)
		h.broadcastGuildMemberOut(restante, s.CharID())
	}
	h.log().Info("guilda", "acao", "saiu", "jogador", s.Avatar.Name)
	return nil
}

// guildRank e o porte de TPacketHandlers.ChangeGuildMemberRank ($F1D).
func (h *Handler) guildRank(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if g.LeaderID != s.CharID() {
		h.message(c, "So o mestre pode mudar o rank.")
		return nil
	}

	var pkt packets.ChangeGuildMemberRankPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.CharIndex <= 0 || pkt.Rank < 0 || pkt.Rank >= guild.MaxRanks {
		// A origem gravava o rank no registro do membro sem validar a faixa,
		// e dali ele indexava RanksConfig, ARRAY [0..4], fora do vetor.
		h.log().Warn("rank de guilda fora da faixa",
			"rank", pkt.Rank, "usuario", s.Username, "ip", c.IP())
		return nil
	}
	alvo := uint32(pkt.CharIndex)
	if alvo == g.LeaderID {
		h.message(c, "O rank do mestre nao muda por aqui.")
		return nil
	}
	if err := g.SetRank(alvo, uint8(pkt.Rank)); err != nil {
		return nil
	}
	h.broadcastGuild(g, nil)
	return nil
}

// guildMaster e o porte de TPacketHandlers.ChangeMasterGuild ($619).
func (h *Handler) guildMaster(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if g.LeaderID != s.CharID() {
		h.message(c, "So o mestre pode passar a lideranca.")
		return nil
	}

	var pkt packets.ChangeMasterGuild
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.CharIndex == 0 || pkt.CharIndex == s.CharID() {
		return nil
	}
	if err := g.SetLeader(pkt.CharIndex); err != nil {
		h.message(c, "Membro nao encontrado na guilda.")
		return nil
	}
	h.broadcastGuild(g, nil)
	h.log().Info("guilda",
		"acao", "mestre novo", "guilda", g.Name, "mestre", pkt.CharIndex)
	return nil
}

// guildNotices e o porte de TPacketHandlers.UpdateGuildNotices ($F20).
func (h *Handler) guildNotices(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if !g.CanOf(s.CharID()).EditNotices {
		h.message(c, "Voce nao tem permissao para isso.")
		return nil
	}

	var pkt packets.UpdateGuildNoticesPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	avisos := make([]string, 0, guild.MaxNotices)
	for i := range pkt.Notices {
		avisos = append(avisos, netsrv.CString(pkt.Notices[i][:]))
	}
	g.SetNotices(avisos...)
	h.broadcastGuild(g, nil)
	return nil
}

// guildSite e o porte de TPacketHandlers.UpdateGuildSite ($F21).
func (h *Handler) guildSite(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if !g.CanOf(s.CharID()).EditNotices {
		h.message(c, "Voce nao tem permissao para isso.")
		return nil
	}

	var pkt packets.UpdateGuildMessagePacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	g.SetSite(netsrv.CString(pkt.Msg[:]))
	h.broadcastGuild(g, nil)
	return nil
}

// guildRanksConfig e o porte de TPacketHandlers.UpdateGuildRanksConfig ($F22).
//
// A origem fazia "Move(RanksConfig, Self.RanksConfig, 4)": copiava 4 bytes
// num vetor de 5, entao a configuracao do rank 4 nunca era atualizada.
func (h *Handler) guildRanksConfig(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if g.LeaderID != s.CharID() {
		h.message(c, "So o mestre pode mudar as permissoes.")
		return nil
	}

	var pkt packets.GuildInfoPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	g.SetRanksConfig(pkt.RanksConfig[:])
	h.broadcastGuild(g, nil)
	return nil
}

// ---- envio

// sendGuildInfo manda o $965. Porte de TPlayer.SendGuildInfo.
func (h *Handler) sendGuildInfo(c *netsrv.Conn, s *Session, g *guild.Guild) {
	if g == nil {
		return
	}
	var pkt packets.GuildInfoPacket
	pkt.Header.Index = s.ClientID
	pkt.GuildIndex = g.Index
	pkt.GuildIndex_1 = g.Index
	pkt.Nation = g.Nation
	netsrv.PutCString(pkt.GuildName[:], g.Name)
	for i := range pkt.Notices {
		if i < len(g.Notices) {
			netsrv.PutCString(pkt.Notices[i].Text[:], g.Notices[i])
		}
	}
	netsrv.PutCString(pkt.Site[:], g.Site)
	for i, it := range g.Ally.Guilds {
		pkt.GuildsAlly[i].Index = it.Index
		netsrv.PutCString(pkt.GuildsAlly[i].Name[:], it.Name)
	}
	pkt.Exp = g.Exp
	pkt.Level = g.Level
	pkt.RanksConfig = g.RanksConfig
	pkt.BravePoints = g.BravurePoints
	pkt.SkillPoints = uint8(min(g.SkillPoints, 255))
	if g.Promote {
		pkt.Promote = 1
	}
	// Valores fixos do original.
	pkt.Null_4 = 1
	pkt.Unk_1[0] = 1

	if err := c.SendStruct(CodeGuildInfo, pkt); err != nil {
		h.log().Debug("falha ao enviar guilda", "usuario", s.Username, "erro", err)
	}
}

// sendGuildPlayers manda o $97F em lotes de dez. Porte de
// TPlayer.SendGuildPlayers.
func (h *Handler) sendGuildPlayers(c *netsrv.Conn, s *Session, g *guild.Guild) {
	if g == nil {
		return
	}
	var pkt packets.GuildPlayersPacket
	pkt.Header.Index = s.ClientID
	pkt.GuildIndex = uint32(g.Index)

	n := 0
	envia := func() {
		if err := c.SendStruct(CodeGuildPlayers, pkt); err != nil {
			h.log().Debug("falha ao enviar membros da guilda",
				"usuario", s.Username, "erro", err)
		}
		pkt.Players = [MembersPerPacket]model.PlayerFromGuild{}
		n = 0
	}

	for i := range g.Members {
		m := &g.Members[i]
		if m.CharIndex == 0 {
			continue
		}
		pkt.Players[n] = h.guildMemberToPacket(m)
		n++
		if n >= MembersPerPacket {
			envia()
		}
	}
	if n > 0 {
		envia()
	}
}

// guildMemberToPacket converte o membro para o formato do pacote.
func (h *Handler) guildMemberToPacket(m *guild.Member) model.PlayerFromGuild {
	var out model.PlayerFromGuild
	out.CharIndex = m.CharIndex
	netsrv.PutCString(out.Name[:], m.Name)
	out.Rank = m.Rank
	out.ClassInfo = m.ClassInfo
	out.Level = m.Level
	// O estado de online vem de quem esta no canal agora, nao do campo
	// gravado: a origem zerava Logged e o recalculava, mas so para o canal
	// local.
	if h.Chan != nil && h.guildMemberOnline(m.CharIndex) {
		out.Logged = 1
	}
	if !m.LastLogin.IsZero() {
		out.LastLogin = uint32(max(m.LastLogin.Unix(), 0))
	}
	return out
}

// guildMemberOnline diz se o personagem esta em jogo neste canal.
func (h *Handler) guildMemberOnline(charIndex uint32) bool {
	if h.Chan == nil {
		return false
	}
	for _, id := range h.Chan.PlayingIDs() {
		if s, _, ok := h.Chan.Lookup(id); ok && s.CharID() == charIndex {
			return true
		}
	}
	return false
}

// broadcastGuild manda info e lista a todos os membros online.
func (h *Handler) broadcastGuild(g *guild.Guild, exceto *Session) {
	if g == nil || h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		m, mc, ok := h.Chan.Lookup(id)
		if !ok || mc == nil || m == exceto {
			continue
		}
		if g.FindMember(m.CharID()) < 0 {
			continue
		}
		h.sendGuildInfo(mc, m, g)
		h.sendGuildPlayers(mc, m, g)
	}
}

// broadcastNewGuildMember avisa os membros online do membro novo.
func (h *Handler) broadcastNewGuildMember(g *guild.Guild, novo guild.Member) {
	if g == nil || h.Chan == nil {
		return
	}
	var pkt packets.AddPlayerToGuildPacket
	pkt.Header.Index = WorldIndex
	pkt.Player = h.guildMemberToPacket(&novo)
	raw, err := netsrv.Marshal(CodeAddGuildPlayer, pkt)
	if err != nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		m, _, ok := h.Chan.Lookup(id)
		if ok && m.CharID() != novo.CharIndex && g.FindMember(m.CharID()) >= 0 {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// broadcastGuildMemberOut avisa os membros online de quem saiu.
func (h *Handler) broadcastGuildMemberOut(g *guild.Guild, charIndex uint32) {
	if g == nil || h.Chan == nil {
		return
	}
	var pkt packets.GuildMemberLogoutPacket
	pkt.Header.Index = WorldIndex
	pkt.CharIndex = int32(charIndex)
	raw, err := netsrv.Marshal(CodeGuildMemberOut, pkt)
	if err != nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		m, _, ok := h.Chan.Lookup(id)
		if ok && g.FindMember(m.CharID()) >= 0 {
			_ = h.Chan.SendTo(id, raw)
		}
	}
}

// clearGuildOf limpa a guilda da sessao de um personagem, se ele estiver em
// jogo.
func (h *Handler) clearGuildOf(charIndex uint32) {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		m, mc, ok := h.Chan.Lookup(id)
		if !ok || m.CharID() != charIndex {
			continue
		}
		m.Avatar.GuildIndex = 0
		m.Avatar.GuildName = ""
		if mc != nil {
			h.message(mc, "Voce foi expulso da guilda.")
		}
		h.respawnToVisible(m)
		return
	}
}

// guildChat manda a fala a guilda. Porte de TGuild.SendChatMessage.
func (h *Handler) guildChat(c *netsrv.Conn, s *Session, pkt packets.ChatPacket) error {
	if h.Guilds == nil {
		h.message(c, "Voce nao esta em uma guilda.")
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		h.message(c, "Voce nao esta em uma guilda.")
		return nil
	}
	raw, err := netsrv.Marshal(CodeClientSay, pkt)
	if err != nil {
		return err
	}
	for _, id := range h.Chan.PlayingIDs() {
		m, _, ok := h.Chan.Lookup(id)
		if ok && g.FindMember(m.CharID()) >= 0 {
			_ = h.Chan.SendTo(id, raw)
		}
	}
	return nil
}

// guildOnDisconnect marca o membro como offline na saida.
func (h *Handler) guildOnDisconnect(s *Session) {
	if h.Guilds == nil {
		return
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return
	}
	if m := g.MemberAt(g.FindMember(s.CharID())); m != nil {
		m.Logged = false
		m.LastLogin = h.now()
	}
}
