package game

import (
	"context"
	"errors"
	"slices"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos da raid (legiao).
const (
	CodeGiveLeaderRaid = 0x207
	CodeSendRaid       = 0x342
	CodeAcceptRaid     = 0x343
	CodeExitRaid       = 0x344
)

// MaxRaidParties e o limite de grupos numa raid (PartyRaidCount >= 4).
const MaxRaidParties = 4

// raidInviteRange e a distancia maxima para convidar outro grupo.
const raidInviteRange = 20

// Erros da raid.
var (
	ErrNotInRaid     = errors.New("game: grupo nao esta em raid")
	ErrAlreadyInRaid = errors.New("game: grupo ja esta em raid")
	ErrRaidFull      = errors.New("game: raid completa")
)

// Raid e uma legiao: ate 4 grupos, um deles lider.
//
// A origem guardava a raid espalhada nos grupos (InRaid, IsRaidLeader,
// RaidPartyId, PartyRaidCount e PartyAllied[1..3], uma lista de aliados por
// grupo mantida a mao nos dois sentidos). Aqui a raid e um objeto so e cada
// grupo aponta para ela, entao as listas de aliados nao podem ficar
// inconsistentes.
type Raid struct {
	// Parties sao os indices dos grupos, na ordem de entrada.
	Parties []uint16
	// Leader e o indice do grupo lider.
	Leader uint16
}

// Others devolve os indices dos outros grupos, na ordem de entrada. E o
// PartyAllied do grupo.
func (rd *Raid) Others(party uint16) []uint16 {
	var out []uint16
	for _, p := range rd.Parties {
		if p != party {
			out = append(out, p)
		}
	}
	return out
}

func (rd *Raid) clone() *Raid {
	return &Raid{Parties: slices.Clone(rd.Parties), Leader: rd.Leader}
}

// RaidOf devolve uma copia da raid do grupo, ou nil.
func (r *PartyRegistry) RaidOf(party uint16) *Raid {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rd := r.raids[party]; rd != nil {
		return rd.clone()
	}
	return nil
}

// CreateRaid junta dois grupos numa raid nova; req vira o lider. O grupo que
// aceitou adota a divisao de experiencia e de item de quem convidou.
func (r *PartyRegistry) CreateRaid(req, acc uint16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pr, pa := r.parties[req], r.parties[acc]
	if pr == nil || pa == nil || req == acc {
		return ErrNoParty
	}
	if r.raids[req] != nil || r.raids[acc] != nil {
		return ErrAlreadyInRaid
	}
	if r.raids == nil {
		r.raids = map[uint16]*Raid{}
	}
	rd := &Raid{Parties: []uint16{req, acc}, Leader: req}
	r.raids[req], r.raids[acc] = rd, rd
	pa.ExpAllocate, pa.ItemAllocate = pr.ExpAllocate, pr.ItemAllocate
	return nil
}

// AddToRaid junta o grupo acc a raid do grupo req.
func (r *PartyRegistry) AddToRaid(req, acc uint16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pr, pa := r.parties[req], r.parties[acc]
	if pr == nil || pa == nil {
		return ErrNoParty
	}
	rd := r.raids[req]
	if rd == nil {
		return ErrNotInRaid
	}
	if r.raids[acc] != nil {
		return ErrAlreadyInRaid
	}
	if len(rd.Parties) >= MaxRaidParties {
		return ErrRaidFull
	}
	rd.Parties = append(rd.Parties, acc)
	r.raids[acc] = rd
	pa.ExpAllocate, pa.ItemAllocate = pr.ExpAllocate, pr.ItemAllocate
	return nil
}

// LeaveRaid tira o grupo da raid. Devolve os grupos que continuam nela; a
// lideranca passa ao primeiro que ficou se o lider saiu. Raid de um grupo so
// se desfaz.
func (r *PartyRegistry) LeaveRaid(party uint16) (restantes []uint16, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.leaveRaidLocked(party)
}

func (r *PartyRegistry) leaveRaidLocked(party uint16) ([]uint16, bool) {
	rd := r.raids[party]
	if rd == nil {
		return nil, false
	}
	delete(r.raids, party)
	rd.Parties = slices.DeleteFunc(rd.Parties, func(p uint16) bool { return p == party })
	if len(rd.Parties) <= 1 {
		// Sobrou um grupo so: nao e raid.
		for _, p := range rd.Parties {
			delete(r.raids, p)
		}
		rest := slices.Clone(rd.Parties)
		rd.Parties = nil
		return rest, true
	}
	if rd.Leader == party {
		rd.Leader = rd.Parties[0]
	}
	return slices.Clone(rd.Parties), true
}

// SetRaidLeader passa a lideranca da raid a outro grupo da mesma raid.
func (r *PartyRegistry) SetRaidLeader(party, to uint16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rd := r.raids[party]
	if rd == nil || r.raids[to] != rd || !slices.Contains(rd.Parties, to) {
		return ErrNotInRaid
	}
	rd.Leader = to
	return nil
}

// ---- pacotes

// sendRaid e o porte de TPacketHandlers.SendRaid ($342): o lider de um grupo
// convida o lider de outro para a legiao.
func (h *Handler) sendRaid(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SendInviteToRaid
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if h.Parties == nil || pkt.SendTo == 0 || pkt.SendTo > 0xFFFF {
		return nil
	}
	meu := h.Parties.Of(s.ClientID)
	if meu == nil {
		return nil
	}
	alvo, alvoConn, ok := h.sessionOf(uint16(pkt.SendTo))
	if !ok || alvo == s {
		h.message(c, "O alvo nao esta disponivel.")
		return nil
	}
	if s.Position.Distance(alvo.Position) > raidInviteRange {
		h.message(c, "O alvo esta muito longe.")
		return nil
	}
	dele := h.Parties.Of(alvo.ClientID)
	switch {
	case dele == nil:
		h.message(c, "O alvo nao esta em grupo.")
	case dele.Leader != alvo.ClientID:
		h.message(c, "O alvo nao e lider do outro grupo.")
	case h.Parties.RaidOf(dele.Index) != nil:
		h.message(c, "O alvo ja esta em uma legiao.")
	case meu.Leader != s.ClientID:
		h.message(c, "Voce nao e lider do seu grupo.")
	case meu.Index == dele.Index:
		h.message(c, "O alvo e do seu proprio grupo.")
	default:
		rd := h.Parties.RaidOf(meu.Index)
		if rd != nil && rd.Leader != meu.Index {
			h.message(c, "Seu grupo nao pode convidar outros grupos. Peca ao lider da legiao.")
			return nil
		}
		if rd != nil && len(rd.Parties) >= MaxRaidParties {
			h.message(c, "Sua legiao ja esta completa, com 4 grupos.")
			return nil
		}
		if s.Avatar.Nation != 0 && alvo.Avatar.Nation != 0 && s.Avatar.Nation != alvo.Avatar.Nation {
			h.message(c, "Nao e possivel convidar jogadores de outras nacoes para a raid.")
			return nil
		}
		alvo.RaidRequester = s.ClientID
		pkt.Header.Index = alvo.ClientID
		pkt.SendTo = uint32(s.ClientID)
		_ = alvoConn.SendStruct(CodeSendRaid, pkt)
	}
	return nil
}

// acceptRaid e o porte do ramo de legiao de TPacketHandlers.AcceptRaid ($343).
// O ramo de alianca de guildas (AliianceByLegion) nao esta portado.
//
// A origem nao conferia se quem aceitava lidera o grupo nem se o convite ainda
// valia (o convidador podia ter saido do grupo ou perdido a lideranca da
// legiao): aqui os dois sao conferidos de novo.
func (h *Handler) acceptRaid(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.InviteToRaidResponse
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if s.AllyByLegion {
		return h.acceptGuildAlly(ctx, c, s, pkt.Accept == BoolAccept)
	}
	if s.RaidRequester == 0 || h.Parties == nil {
		return nil
	}
	reqID := s.RaidRequester
	s.RaidRequester = 0
	req, reqConn, ok := h.sessionOf(reqID)
	if !ok {
		return nil
	}
	if pkt.Accept != BoolAccept {
		h.message(reqConn, s.Avatar.Name+" recusou entrar na legiao.")
		return nil
	}
	meu, dele := h.Parties.Of(s.ClientID), h.Parties.Of(req.ClientID)
	if meu == nil || dele == nil || meu.Leader != s.ClientID || dele.Leader != req.ClientID {
		h.message(reqConn, "Nao foi possivel criar a raid.")
		return nil
	}
	var err error
	if rd := h.Parties.RaidOf(dele.Index); rd == nil {
		err = h.Parties.CreateRaid(dele.Index, meu.Index)
	} else if rd.Leader != dele.Index {
		err = ErrNotInRaid // quem convidou perdeu a lideranca da legiao
	} else {
		err = h.Parties.AddToRaid(dele.Index, meu.Index)
	}
	if err != nil {
		h.message(reqConn, "Nao foi possivel criar a raid.")
		return nil
	}
	h.refreshRaid(dele.Index)
	return nil
}

// exitRaid e o porte de TPacketHandlers.ExitRaid ($344): o grupo sai da
// legiao. So o lider do grupo pode tirar o grupo (a origem deixava qualquer
// membro).
func (h *Handler) exitRaid(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	meu := h.Parties.Of(s.ClientID)
	if meu == nil || h.Parties.RaidOf(meu.Index) == nil {
		return nil
	}
	if meu.Leader != s.ClientID {
		h.message(c, "So o lider do grupo pode sair da legiao.")
		return nil
	}
	rest, _ := h.Parties.LeaveRaid(meu.Index)
	h.refreshParty(meu)
	for _, idx := range rest {
		h.refreshParty(h.Parties.Get(idx))
	}
	return nil
}

// giveLeaderRaid e o porte de TPacketHandlers.GiveLeaderRaid ($207).
//
// A origem testava "Party.Index = Base.ClientID" para saber se o alvo lidera o
// grupo, comparando um indice de grupo com um id de jogador: so passava por
// coincidencia. Aqui vale Party.Leader.
func (h *Handler) giveLeaderRaid(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.GiveRaidLeader
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	meu := h.Parties.Of(s.ClientID)
	if meu == nil || pkt.GiveTo == 0 {
		return nil
	}
	rd := h.Parties.RaidOf(meu.Index)
	if rd == nil {
		h.message(c, "Voce nao esta em raid.")
		return nil
	}
	if meu.Leader != s.ClientID || rd.Leader != meu.Index {
		h.message(c, "Voce nao e o lider da raid.")
		return nil
	}
	alvo, _, ok := h.sessionOf(pkt.GiveTo)
	if !ok {
		h.message(c, "O alvo nao esta disponivel.")
		return nil
	}
	dele := h.Parties.Of(alvo.ClientID)
	if dele == nil {
		h.message(c, "Alvo nao esta em grupo.")
		return nil
	}
	if !slices.Contains(rd.Parties, dele.Index) || dele.Index == meu.Index {
		h.message(c, "O alvo esta em um grupo que nao pertence a sua raid.")
		return nil
	}
	if dele.Leader != alvo.ClientID {
		h.message(c, "O alvo nao e o lider do grupo.")
		return nil
	}
	if err := h.Parties.SetRaidLeader(meu.Index, dele.Index); err != nil {
		return nil
	}
	h.refreshRaid(meu.Index)
	return nil
}

// refreshRaid reenvia o estado de todos os grupos da raid do grupo dado.
func (h *Handler) refreshRaid(party uint16) {
	rd := h.Parties.RaidOf(party)
	if rd == nil {
		h.refreshParty(h.Parties.Get(party))
		return
	}
	for _, idx := range rd.Parties {
		h.refreshParty(h.Parties.Get(idx))
	}
}

// sendRaidSlots manda, depois do grupo do jogador, um $326 por slot de aliado
// (1 a 3): o grupo aliado do slot, ou o slot vazio. Porte do laco "for k := 1 to
// 3" de TPlayer.RefreshParty.
func (h *Handler) sendRaidSlots(c *netsrv.Conn, s *Session, meu *Party, raid *Raid) {
	outros := raid.Others(meu.Index)
	for k := 1; k <= MaxRaidSlots; k++ {
		var pkt packets.UpdatePartyPacket
		pkt.Header.Index = s.ClientID
		pkt.PartyInRaidIndex = uint8(k)
		if k-1 < len(outros) {
			if g := h.Parties.Get(outros[k-1]); g != nil {
				h.fillPartyPacket(&pkt, g)
				if raid.Leader == g.Index {
					pkt.IsPartyLeaderOfRaid = 1
				}
			}
		}
		_ = c.SendStruct(CodeUpdateParty, pkt)
	}
}

// fillPartyPacket preenche os membros e a divisao de um grupo no $326.
func (h *Handler) fillPartyPacket(pkt *packets.UpdatePartyPacket, g *Party) {
	for j, id := range h.Parties.MembersOf(g) {
		if j >= MaxPartyMembers {
			break
		}
		m, _, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		netsrv.PutCString(pkt.Name[j][:], m.Avatar.Name)
		pkt.PlayerIndex[j] = m.ClientID
		pkt.ClassInfo[j] = uint8(min(m.Avatar.ClassInfo, 255))
		if m.Avatar.Level > 0 {
			pkt.Level[j] = uint8(min(m.Avatar.Level-1, 255))
		}
		pkt.CurHP[j], pkt.MaxHP[j] = m.Avatar.CurHP, m.Avatar.MaxHP
		pkt.CurMP[j], pkt.MaxMP[j] = m.Avatar.CurMP, m.Avatar.MaxMP
	}
	pkt.LeaderIndex = g.Leader
	pkt.ExpAllocate, pkt.ItemAllocate = g.ExpAllocate, g.ItemAllocate
}
