package game

import (
	"context"
	"errors"

	"aika/internal/guild"
	"aika/internal/nation"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// CodeGuildAllyRequest e o $F12, o pedido de aliança entre guildas. O convite
// segue pelo $342 e a resposta volta pelo $343.
const CodeGuildAllyRequest = 0xF12

// requestGuildAlly e o porte de TPacketHandlers.RequestGuildToAlly ($F12): o
// lider da guilda lider da aliança chama outra guilda pelo nome; o convite vai
// para o lider dela.
//
// Corrigido: a conferencia de vaga na origem misturava indices de guildas
// diferentes e, sem vaga, a entrada escrevia fora do vetor; aqui a vaga e a
// regra de entrada sao conferidas em guild.CanAddAlly, de novo ao aceitar.
func (h *Handler) requestGuildAlly(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.GuildRequestAllyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := netsrv.CString(pkt.GuildName[:])
	if nome == "" || h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		return nil
	}
	if g.LeaderID != s.CharID() {
		h.message(c, "Você não é o líder da guild.")
		return nil
	}
	if g.Ally.Leader != g.Index {
		h.message(c, "Você não é o líder da aliança")
		return nil
	}
	outra := h.Guilds.ByName(nome)
	if outra == nil {
		h.message(c, "Erro de busca por guild para inclusão na aliança. Ticket pro suporte.")
		return nil
	}
	switch err := guild.CanAddAlly(g, outra); {
	case errors.Is(err, guild.ErrAllySelf):
		h.message(c, "Você não pode adicionar a própia legião na aliança.")
		return nil
	case errors.Is(err, guild.ErrAlreadyAllied):
		h.message(c, "A guild já está em uma aliança.")
		return nil
	case errors.Is(err, guild.ErrAllyFull):
		h.message(c, "Não há mais espaço na aliança.")
		return nil
	case errors.Is(err, guild.ErrAllyNation):
		h.message(c, "A guild escolhida é de outra nação.")
		return nil
	case err != nil:
		return nil
	}
	lid, lidConn, ok := h.sessionByCharID(outra.LeaderID)
	if !ok {
		h.message(c, "O lider da guild escolhida está offline.")
		return nil
	}

	var inv packets.SendInviteToRaid
	inv.Header.Index = lid.ClientID
	inv.SendTo = uint32(s.ClientID)
	lid.AllyByLegion = true
	lid.AllyRequester = s.ClientID
	h.message(lidConn, "A aliança de <"+g.Name+"> lhe convida a participar da aliança com sua Legião.")
	_ = lidConn.SendStruct(CodeSendRaid, inv)
	return nil
}

// sessionByCharID acha a sessao em jogo do personagem.
func (h *Handler) sessionByCharID(charID uint32) (*Session, *netsrv.Conn, bool) {
	if h.Chan == nil {
		return nil, nil, false
	}
	for _, id := range h.Chan.PlayingIDs() {
		o, conn, ok := h.Chan.Lookup(id)
		if ok && o.Status >= Playing && o.CharID() == charID {
			return o, conn, true
		}
	}
	return nil, nil, false
}

// acceptGuildAlly e o ramo de aliança de TPacketHandlers.AcceptRaid ($343).
// Quem aceita e o lider da guilda convidada; s.AllyRequester e o lider da
// aliança que chamou. Tudo e conferido de novo: o convite pode ter envelhecido.
func (h *Handler) acceptGuildAlly(ctx context.Context, c *netsrv.Conn, s *Session, aceitou bool) error {
	reqID := s.AllyRequester
	s.AllyByLegion, s.AllyRequester = false, 0
	req, reqConn, ok := h.sessionOf(reqID)
	if !ok {
		return nil
	}
	if !aceitou {
		h.message(reqConn, "O lider da guild escolhida recusou-se a entrar na aliança.")
		return nil
	}
	if h.Guilds == nil {
		return nil
	}
	joiner, lead := h.Guilds.Of(s.CharID()), h.Guilds.Of(req.CharID())
	if joiner == nil || lead == nil || joiner.LeaderID != s.CharID() || lead.LeaderID != req.CharID() {
		h.message(reqConn, "Não foi possível criar a aliança.")
		return nil
	}
	changed, err := h.Guilds.JoinAlliance(lead, joiner)
	if err != nil {
		h.message(reqConn, "Não foi possível criar a aliança.")
		h.message(c, "Não foi possível entrar na aliança.")
		return nil
	}

	// Aliança do marechal: a guilda nova ocupa o primeiro cargo de grade livre.
	novaGrade := false
	if h.isMarshal(req) {
		novaGrade = h.takeGradeSlot(ctx, uint8(min(req.Avatar.Nation, 255)), joiner)
	}
	for _, g := range changed {
		g := g
		for i := range g.Members {
			m := &g.Members[i]
			if m.CharIndex == 0 || !m.Logged {
				continue
			}
			o, conn, ok := h.sessionByCharID(m.CharIndex)
			if !ok {
				continue
			}
			h.sendGuildInfo(conn, o, g)
			h.message(conn, "Sua aliança teve mudanças.")
			// Quem entrou depois (guildas 2 e 3 na origem) e todos quando a
			// grade mudou recebem a nacao de novo.
			if novaGrade || (g != lead && g != joiner) {
				h.sendNationInformation(conn, o)
			}
		}
	}
	if h.GuildDB != nil {
		for _, g := range changed {
			if err := h.GuildDB.Save(ctx, g); err != nil {
				h.log().Error("gravar guilda da aliança", "guilda", g.Index, "erro", err)
			}
		}
	}
	return nil
}

// takeGradeSlot poe a guilda no primeiro cargo livre entre estrategista, juiz
// e tesoureiro da nacao e grava. Devolve se algum cargo mudou.
func (h *Handler) takeGradeSlot(ctx context.Context, id uint8, g *guild.Guild) bool {
	if h.Nations == nil {
		return false
	}
	mudou := false
	st, ok := h.Nations.Update(id, func(s *nation.State) {
		switch {
		case s.TacticianGuild == 0:
			s.TacticianGuild = g.Index
		case s.JudgeGuild == 0:
			s.JudgeGuild = g.Index
		case s.TreasurerGuild == 0:
			s.TreasurerGuild = g.Index
		default:
			return
		}
		mudou = true
	})
	if ok && mudou && h.NationDB != nil {
		if err := h.NationDB.SaveGrade(ctx, st); err != nil {
			h.log().Error("gravar grade da nacao", "nacao", id, "erro", err)
		}
	}
	return mudou
}
