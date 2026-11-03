package game

import (
	"context"
	"strings"

	"aika/internal/entity"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de grupo.
const (
	CodeSendParty       = 0x322 // cliente -> servidor: convidar
	CodeAcceptParty     = 0x323 // cliente -> servidor: aceitar ou recusar
	CodeKickParty       = 0x324 // cliente -> servidor: expulsar ou sair
	CodeDestroyParty    = 0x325 // cliente -> servidor: desfazer
	CodeUpdateParty     = 0x326 // servidor -> cliente: estado do grupo
	CodeGiveLeaderParty = 0x34B // cliente -> servidor: passar lideranca
	CodeAddSelfParty    = 0x38F // cliente -> servidor: pedir para entrar
	CodePartyCoord      = 0x11D // servidor -> cliente: posicao do membro
)

// Respostas do aceite, as constantes BOOL_* de GlobalDefs.pas.
const (
	BoolRefuse = 0
	BoolAccept = 1
)

// MaxRaidSlots e o numero de grupos aliados numa raid. So existe aqui para o
// pacote levar os campos que o cliente espera; raid nao esta portada.
const MaxRaidSlots = 3

// sendParty e o porte de TPacketHandlers.SendParty ($322): o convite.
//
// A origem resolvia o alvo com
// "if not(PlayerIndex = 0) and not(PlayerIndex > MAX_CONNECTIONS) then
//
//	OtherPlayer := @Players[Packet.PlayerIndex]", o que cobre a faixa, mas
//
// depois usava OtherPlayer sem checar se o ramo do nome tinha achado alguem:
// quando GetPlayerByName falhava o Exit salvava, mas o ponteiro ficava
// indefinido em qualquer outro caminho que o lesse.
func (h *Handler) sendParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Parties == nil {
		return nil
	}

	var pkt packets.SendPartyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	// O grupo de quem convida tem de ter espaco.
	if meu := h.Parties.Of(s.ClientID); meu != nil {
		if meu.Leader != s.ClientID {
			h.message(c, "So o lider pode convidar.")
			return nil
		}
		if meu.Size() >= MaxPartyMembers {
			h.message(c, "O grupo esta cheio.")
			return nil
		}
	}

	alvo, alvoConn, ok := h.resolvePartyTarget(pkt.PlayerIndex, pkt.Name[:])
	if !ok {
		h.message(c, "Personagem nao encontrado.")
		return nil
	}
	if alvo.ClientID == s.ClientID {
		return nil
	}
	if h.Parties.Of(alvo.ClientID) != nil {
		h.message(c, "O jogador ja esta em um grupo.")
		return nil
	}

	// O convite fica guardado no convidado, que responde com o $323.
	alvo.PartyRequester = s.ClientID

	var convite packets.SendPartyPacket
	convite.Header.Index = s.ClientID
	convite.PlayerIndex = uint32(s.ClientID)
	netsrv.PutCString(convite.Name[:], s.Avatar.Name)
	if err := alvoConn.SendStruct(CodeSendParty, convite); err != nil {
		h.log().Debug("falha ao enviar convite de grupo",
			"de", s.Avatar.Name, "para", alvo.Avatar.Name, "erro", err)
	}
	return nil
}

// resolvePartyTarget acha o alvo por indice ou por nome.
func (h *Handler) resolvePartyTarget(index uint32, nome []byte) (*Session, *netsrv.Conn, bool) {
	if index != 0 {
		if index > uint32(h.Chan.MaxConns()) || !entity.IsPlayerID(uint16(index), h.Chan.MaxConns()) {
			return nil, nil, false
		}
		s, c, ok := h.Chan.Lookup(uint16(index))
		if !ok || s.Status < Playing {
			return nil, nil, false
		}
		return s, c, true
	}
	n := strings.TrimSpace(netsrv.CString(nome))
	if n == "" {
		return nil, nil, false
	}
	id, ok := h.Chan.FindByCharacter(n)
	if !ok {
		return nil, nil, false
	}
	s, c, ok := h.Chan.Lookup(id)
	if !ok || s.Status < Playing {
		return nil, nil, false
	}
	return s, c, true
}

// acceptParty e o porte de TPacketHandlers.AcceptParty ($323).
func (h *Handler) acceptParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Parties == nil {
		return nil
	}
	if s.PartyRequester == 0 {
		return nil
	}

	var pkt packets.AcceptPartyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	quemConvidou := s.PartyRequester
	// O convite vale uma vez: sem isso o mesmo $323 entrava no grupo de novo.
	s.PartyRequester = 0

	convidou, convidouConn, ok := h.Chan.Lookup(quemConvidou)
	if !ok || convidou.Status < Playing {
		return nil
	}

	if pkt.AceptType == BoolRefuse {
		h.message(convidouConn, s.Avatar.Name+" recusou entrar no grupo.")
		return nil
	}
	if pkt.AceptType != BoolAccept {
		// Valor fora de 0 e 1: a origem caia no fim do case e nao fazia nada.
		return nil
	}

	grupo := h.Parties.Of(quemConvidou)
	if grupo == nil {
		novo, err := h.Parties.Create(quemConvidou)
		if err != nil {
			h.message(convidouConn, "Nao foi possivel criar o grupo.")
			return nil
		}
		grupo = novo
	}
	if err := h.Parties.Add(grupo, s.ClientID); err != nil {
		h.message(convidouConn, "Nao foi possivel adicionar o jogador ao grupo.")
		// Grupo que ficou com um membro so por causa da falha nao e grupo.
		if grupo.Size() <= 1 {
			h.releaseParty(h.Parties.Destroy(grupo))
		}
		return nil
	}

	h.refreshParty(grupo)
	h.log().Info("grupo",
		"acao", "entrou", "jogador", s.Avatar.Name,
		"grupo", grupo.Index, "membros", grupo.Size())
	return nil
}

// kickParty e o porte de TPacketHandlers.KickParty ($324): o lider expulsa,
// ou o proprio membro sai.
func (h *Handler) kickParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Parties == nil {
		return nil
	}
	grupo := h.Parties.Of(s.ClientID)
	if grupo == nil {
		return nil
	}

	var pkt packets.KickPartyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	// "if (Packet.PlayerIndex > MAX_CONNECTIONS) then Exit" da origem, com o
	// limite de baixo que faltava.
	if pkt.PlayerIndex == 0 || pkt.PlayerIndex > uint32(h.Chan.MaxConns()) {
		return nil
	}
	alvo := uint16(pkt.PlayerIndex)

	// Expulsar outro so o lider pode; sair de si mesmo qualquer um.
	if alvo != s.ClientID && grupo.Leader != s.ClientID {
		h.message(c, "So o lider pode expulsar.")
		return nil
	}
	if !grupo.Has(alvo) {
		return nil
	}

	restante, soltos := h.Parties.Remove(alvo)
	h.releaseParty(soltos)
	if restante != nil {
		h.refreshParty(restante)
	}
	h.log().Info("grupo", "acao", "saiu",
		"jogador", alvo, "por", s.Avatar.Name)
	return nil
}

// destroyParty e o porte de TPacketHandlers.DestroyParty ($325).
func (h *Handler) destroyParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Parties == nil {
		return nil
	}
	grupo := h.Parties.Of(s.ClientID)
	if grupo == nil {
		return nil
	}
	if grupo.Leader != s.ClientID {
		h.message(c, "So o lider pode desfazer o grupo.")
		return nil
	}
	h.releaseParty(h.Parties.Destroy(grupo))
	h.log().Info("grupo", "acao", "desfeito", "por", s.Avatar.Name)
	return nil
}

// giveLeaderParty e o porte de TPacketHandlers.GiveLeaderParty ($34B).
func (h *Handler) giveLeaderParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Parties == nil {
		return nil
	}
	grupo := h.Parties.Of(s.ClientID)
	if grupo == nil {
		return nil
	}
	if grupo.Leader != s.ClientID {
		h.message(c, "So o lider pode passar a lideranca.")
		return nil
	}

	var pkt packets.KickPartyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.PlayerIndex == 0 || pkt.PlayerIndex > uint32(h.Chan.MaxConns()) {
		return nil
	}
	novo := uint16(pkt.PlayerIndex)
	if novo == s.ClientID {
		return nil
	}
	if err := h.Parties.SetLeader(grupo, novo); err != nil {
		return nil
	}
	h.refreshParty(grupo)
	h.log().Info("grupo", "acao", "lider novo",
		"grupo", grupo.Index, "lider", novo)
	return nil
}

// addSelfParty e o porte de TPacketHandlers.AddSelfParty ($38F): o jogador
// pede para entrar no grupo de alguem.
func (h *Handler) addSelfParty(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil || h.Parties == nil {
		return nil
	}
	if h.Parties.Of(s.ClientID) != nil {
		h.message(c, "Voce ja esta em um grupo.")
		return nil
	}

	var pkt packets.SendPartyPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	alvo, alvoConn, ok := h.resolvePartyTarget(pkt.PlayerIndex, pkt.Name[:])
	if !ok || alvo.ClientID == s.ClientID {
		h.message(c, "Personagem nao encontrado.")
		return nil
	}

	// O pedido chega ao lider do grupo, ou ao proprio jogador se ele nao tem
	// grupo ainda.
	destino, destinoConn := alvo, alvoConn
	if grupo := h.Parties.Of(alvo.ClientID); grupo != nil {
		if grupo.Size() >= MaxPartyMembers {
			h.message(c, "O grupo esta cheio.")
			return nil
		}
		if lider, liderConn, ok := h.Chan.Lookup(grupo.Leader); ok {
			destino, destinoConn = lider, liderConn
		}
	}

	destino.PartyRequester = s.ClientID
	var convite packets.SendPartyPacket
	convite.Header.Index = s.ClientID
	convite.PlayerIndex = uint32(s.ClientID)
	netsrv.PutCString(convite.Name[:], s.Avatar.Name)
	if err := destinoConn.SendStruct(CodeAddSelfParty, convite); err != nil {
		h.log().Debug("falha ao enviar pedido de grupo",
			"de", s.Avatar.Name, "erro", err)
	}
	return nil
}

// releaseParty avisa quem ficou sem grupo.
func (h *Handler) releaseParty(soltos []uint16) {
	if h.Chan == nil {
		return
	}
	for _, id := range soltos {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || c == nil {
			continue
		}
		h.sendPartyUpdate(c, s, nil)
	}
}

// refreshParty manda o estado do grupo a todos os membros. Porte de
// TParty.RefreshParty com TPlayer.RefreshParty.
func (h *Handler) refreshParty(grupo *Party) {
	if h.Chan == nil || grupo == nil {
		return
	}
	membros := h.Parties.MembersOf(grupo)
	for _, id := range membros {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || c == nil {
			continue
		}
		h.sendPartyUpdate(c, s, grupo)
		// Cada membro recebe a posicao dos outros.
		for _, outro := range membros {
			if outro == id {
				continue
			}
			if os, _, ok := h.Chan.Lookup(outro); ok {
				h.sendPartyCoord(c, os)
			}
		}
	}
}

// sendPartyUpdate monta e envia o $326. grupo nil manda o pacote vazio, que e
// como o cliente aprende que o jogador nao tem mais grupo.
func (h *Handler) sendPartyUpdate(c *netsrv.Conn, s *Session, grupo *Party) {
	var pkt packets.UpdatePartyPacket
	pkt.Header.Index = s.ClientID

	if grupo == nil {
		// A origem mandava o pacote vazio e depois um por slot de raid, para
		// o cliente limpar as quatro listas. Mantido.
		if err := c.SendStruct(CodeUpdateParty, pkt); err != nil {
			h.log().Debug("falha ao limpar grupo no cliente",
				"usuario", s.Username, "erro", err)
			return
		}
		for i := 1; i <= MaxRaidSlots; i++ {
			pkt.PartyInRaidIndex = uint8(i)
			_ = c.SendStruct(CodeUpdateParty, pkt)
		}
		return
	}

	membros := h.Parties.MembersOf(grupo)
	for j, id := range membros {
		if j >= MaxPartyMembers {
			// A origem incrementava J sem teto dentro do "for i in Members":
			// com mais membros que o vetor do pacote, escrevia fora dele.
			break
		}
		m, _, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		netsrv.PutCString(pkt.Name[j][:], m.Avatar.Name)
		pkt.PlayerIndex[j] = m.ClientID
		pkt.ClassInfo[j] = uint8(min(m.Avatar.ClassInfo, 255))
		// "Level - 1" num BYTE: com nivel 0 a origem mandava 255.
		if m.Avatar.Level > 0 {
			pkt.Level[j] = uint8(min(m.Avatar.Level-1, 255))
		}
		pkt.CurHP[j] = m.Avatar.CurHP
		pkt.MaxHP[j] = m.Avatar.MaxHP
		pkt.CurMP[j] = m.Avatar.CurMP
		pkt.MaxMP[j] = m.Avatar.MaxMP
	}
	pkt.LeaderIndex = grupo.Leader
	pkt.ExpAllocate = grupo.ExpAllocate
	pkt.ItemAllocate = grupo.ItemAllocate
	pkt.IsPartyLeaderOfRaid = 1

	if err := c.SendStruct(CodeUpdateParty, pkt); err != nil {
		h.log().Debug("falha ao enviar grupo",
			"usuario", s.Username, "erro", err)
	}
}

// sendPartyCoord manda o $11D com a posicao de um membro. Porte de
// TPlayer.SendPositionParty.
func (h *Handler) sendPartyCoord(para *netsrv.Conn, membro *Session) {
	var pkt packets.PartyMemberCoordPacket
	pkt.Header.Index = membro.ClientID
	pkt.Index = uint32(membro.ClientID)
	pkt.PosX = uint32(max(membro.Position.X, 0))
	pkt.PosY = uint32(max(membro.Position.Y, 0))
	if err := para.SendStruct(CodePartyCoord, pkt); err != nil {
		h.log().Debug("falha ao enviar posicao de grupo",
			"membro", membro.ClientID, "erro", err)
	}
}

// partyChat manda a fala ao grupo. Porte de TPlayer.SendToParty.
func (h *Handler) partyChat(c *netsrv.Conn, s *Session, pkt packets.ChatPacket) error {
	grupo := h.Parties.Of(s.ClientID)
	if grupo == nil {
		h.message(c, "Voce nao esta em um grupo.")
		return nil
	}
	raw, err := netsrv.Marshal(CodeClientSay, pkt)
	if err != nil {
		return err
	}
	for _, id := range h.Parties.MembersOf(grupo) {
		_ = h.Chan.SendTo(id, raw)
	}
	return nil
}

// leaveParty tira o jogador do grupo na saida da conexao.
//
// Na origem isso dependia do caminho de desconexao chamar RemoveMember; quando
// nao chamava, o grupo ficava com um membro que nao estava mais no servidor, e
// RefreshParty seguia lendo Players[i] daquele slot — que podia ja ter sido
// reaproveitado por outro jogador.
func (h *Handler) leaveParty(s *Session) {
	if h.Parties == nil || s.ClientID == 0 {
		return
	}
	if h.Parties.Of(s.ClientID) == nil {
		return
	}
	restante, soltos := h.Parties.Remove(s.ClientID)
	// Quem saiu nao precisa de aviso: a conexao esta indo embora.
	_ = soltos
	if restante != nil {
		h.refreshParty(restante)
	}
}
