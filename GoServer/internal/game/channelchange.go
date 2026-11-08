package game

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos da troca de canal.
const (
	CodeChangeChannel    = 0xF05 // nos dois sentidos: pedido do cliente e ordem do servidor
	CodeLoginIntoChannel = 0xF06 // cliente -> servidor, ja no canal novo
	CodeSendToWorldSends = 0xF0B // cliente -> servidor, mundo carregado
)

// Prazos da troca: a ordem vale 30 s, e o canal novo espera ate 10 s o canal
// velho terminar de gravar o personagem.
const (
	handoffTTL  = 30 * time.Second
	handoffWait = 10 * time.Second
)

// Erros da troca de canal.
var (
	ErrNoHandoff       = errors.New("game: nao ha troca de canal pendente para esta conta")
	ErrHandoffExpired  = errors.New("game: troca de canal vencida")
	ErrHandoffMismatch = errors.New("game: troca de canal de outro canal, conta ou endereco")
	ErrHandoffNotSaved = errors.New("game: o canal de origem nao terminou de gravar")
)

// ChannelState e o que um canal informa a quem quer ir para ele.
type ChannelState uint8

// Estados de um canal na troca.
const (
	ChannelMissing ChannelState = iota // nao existe
	ChannelDown                        // existe, mas esta desligado
	ChannelUp
)

// Handoff e a ordem de troca de canal de uma conta.
type Handoff struct {
	AccountID uint32
	Username  string
	Slot      int
	From, To  uint8
	IP        string
	Created   time.Time
	// Group e o grupo que viaja junto, ou nil.
	Group *PartyMove

	saved chan struct{}
	once  sync.Once
}

// Handoffs guarda as trocas em andamento. E compartilhado por todos os canais
// do processo: o canal de origem registra e o de destino confere.
//
// Substitui o ChangeChannelList da origem, um dicionario global que levava o
// registro inteiro da conta e do personagem na memoria. Aqui o que passa de um
// canal para o outro e so a ordem; o personagem vem do banco, onde o canal de
// origem o gravou ao soltar a conexao (por isso a espera).
type Handoffs struct {
	mu       sync.Mutex
	m        map[uint32]*Handoff
	groupSeq uint32
}

// NewHandoffs cria o registro.
func NewHandoffs() *Handoffs { return &Handoffs{m: map[uint32]*Handoff{}} }

// Begin registra a ordem. Devolve falso se a conta ja tem uma ordem viva.
func (r *Handoffs) Begin(accountID uint32, username string, slot int, from, to uint8, ip string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.m[accountID]; ok && now.Sub(old.Created) < handoffTTL {
		return false
	}
	r.m[accountID] = &Handoff{
		AccountID: accountID, Username: username, Slot: slot, From: from, To: to,
		IP: ip, Created: now, saved: make(chan struct{}),
	}
	return true
}

// PartyMove descreve um grupo que troca de canal junto. Cada membro leva o mesmo
// ponteiro na sua ordem; no canal novo o grupo e refeito conforme eles chegam.
type PartyMove struct {
	Key          uint32
	LeaderAcc    uint32
	ExpAllocate  uint8
	ItemAllocate uint8
}

// SetGroup liga a ordem da conta a um grupo que viaja.
func (r *Handoffs) SetGroup(accountID uint32, g *PartyMove) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h := r.m[accountID]; h != nil {
		h.Group = g
	}
}

// NextGroupKey devolve uma chave nova para um grupo que viaja.
func (r *Handoffs) NextGroupKey() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groupSeq++
	return r.groupSeq
}

// Saved avisa que o canal de origem terminou de gravar a conta.
func (r *Handoffs) Saved(accountID uint32) {
	r.mu.Lock()
	h := r.m[accountID]
	r.mu.Unlock()
	if h != nil {
		h.once.Do(func() { close(h.saved) })
	}
}

// Cancel apaga a ordem (o cliente nao apareceu, ou o pedido falhou).
func (r *Handoffs) Cancel(accountID uint32) {
	r.mu.Lock()
	delete(r.m, accountID)
	r.mu.Unlock()
}

// Claim confere a ordem e a entrega ao canal de destino, uma unica vez. A conta
// tem de ter pedido a troca para ESTE canal, com o mesmo usuario e vindo do
// mesmo IP; o canal de origem tem de ter gravado o personagem.
func (r *Handoffs) Claim(ctx context.Context, accountID uint32, to uint8, username, ip string, now time.Time) (*Handoff, error) {
	r.mu.Lock()
	h, ok := r.m[accountID]
	r.mu.Unlock()
	if !ok {
		return nil, ErrNoHandoff
	}
	if now.Sub(h.Created) >= handoffTTL {
		r.Cancel(accountID)
		return nil, ErrHandoffExpired
	}
	if h.To != to || !strings.EqualFold(h.Username, strings.TrimSpace(username)) || h.IP != ip {
		return nil, ErrHandoffMismatch
	}
	select {
	case <-h.saved:
	case <-time.After(handoffWait):
		return nil, ErrHandoffNotSaved
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m[accountID] != h {
		return nil, ErrNoHandoff // outro pedido ja a levou
	}
	delete(r.m, accountID)
	return h, nil
}

// changeChannel e o porte de TPacketHandlers.ChangeChannel ($F05), para quem
// joga sozinho ou como membro de um grupo.
//
// O cliente pede o canal em Info1. O servidor responde com o mesmo codigo
// (Info1 = conta, TypeChanel = canal) e solta a conexao; o cliente reconecta no
// canal novo e manda o $F06.
//
// Nao portado: o lider de um grupo com outros membros leva o grupo inteiro
// (e uma raide, os grupos aliados) para o canal novo, recriando o grupo la. Isso
// pede mover o grupo entre dois PartyRegistry de canais diferentes; por ora o
// lider e avisado e fica onde esta.
func (h *Handler) changeChannel(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.ChangeChannelPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if h.Handoffs == nil || h.ChannelStatus == nil || pkt.Info1 > 255 {
		h.message(c, "Canal destino não válido. ERR_1.")
		return nil
	}
	target := uint8(pkt.Info1)
	switch h.ChannelStatus(target) {
	case ChannelMissing:
		h.message(c, "Canal destino não válido. ERR_1.")
		return nil
	case ChannelDown:
		h.message(c, "Canal destino não válido. ERR_2.")
		return nil
	}
	if target == h.Channel {
		h.message(c, "Canal destino não válido. ERR_3.")
		return nil
	}
	var viajam []*Session
	var grupo *PartyMove
	if g := h.Parties.Of(s.ClientID); g != nil && g.Leader == s.ClientID && g.Size() > 1 {
		if h.Parties.RaidOf(g.Index) != nil {
			h.message(c, "Troca de canal com a legião ainda não está disponível. Saia da legião para trocar de canal.")
			return nil
		}
		viajam = h.partyTravellers(g, s)
		grupo = &PartyMove{Key: h.Handoffs.NextGroupKey(), LeaderAcc: s.Account.ID,
			ExpAllocate: g.ExpAllocate, ItemAllocate: g.ItemAllocate}
	}
	if s.SelectedSlot < 0 || s.Account.ID == 0 {
		return nil
	}
	if !h.Handoffs.Begin(s.Account.ID, s.Username, s.SelectedSlot, h.Channel, target, c.IP(), h.now()) {
		h.message(c, "Troca de canal já em andamento.")
		return nil
	}
	s.handoffAccount = s.Account.ID
	h.Handoffs.SetGroup(s.Account.ID, grupo)

	reply := packets.ChangeChannelPacket{Info1: s.Account.ID, TypeChanel: uint32(target)}
	reply.Header.Index = s.ClientID
	if err := c.SendStruct(CodeChangeChannel, reply); err != nil {
		h.Handoffs.Cancel(s.Account.ID)
		s.handoffAccount = 0
		return err
	}
	h.log().Info("troca de canal", "usuario", s.Username, "de", h.Channel, "para", target)
	// Soltar a conexao grava o personagem (OnDisconnect), e so entao o canal novo
	// pode le-lo.
	c.Close()
	for _, m := range viajam {
		h.sendMemberAway(m, target, grupo)
	}
	return nil
}

// loginIntoChannel e o porte de TPacketHandlers.LoginIntoChannel ($F06): o
// cliente que veio de outro canal se apresenta com a conta (Serial) e o usuario.
// Com a ordem conferida, o personagem entra direto no mundo, sem passar pela
// lista nem pela numerica.
//
// A origem recebia a conta e o personagem por memoria, num dicionario global, e
// esperava o jogador antigo sumir do vetor; aqui o personagem e relido do banco
// depois de o canal de origem grava-lo.
func (h *Handler) loginIntoChannel(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.ChannelSendInfoPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		c.Close()
		return err
	}
	username := strings.TrimSpace(netsrv.CString(pkt.Username[:]))
	if username == "" || h.Handoffs == nil || h.Accounts == nil || h.Chars == nil {
		c.Close()
		return ErrNotAuthenticated
	}
	if s.Authenticated {
		c.Close()
		return ErrNotAuthenticated
	}
	hd, err := h.Handoffs.Claim(ctx, pkt.Serial, h.Channel, username, c.IP(), h.now())
	if err != nil {
		h.log().Warn("troca de canal recusada", "usuario", username, "ip", c.IP(), "erro", err)
		c.Close()
		return err
	}

	acc, found, err := h.Accounts.ByUsername(ctx, strings.ToLower(username))
	if err != nil || !found || acc.ID != hd.AccountID {
		c.Close()
		if err == nil {
			err = ErrAccountNotFound
		}
		return err
	}
	chars, avisos, err := h.Chars.ByAccount(ctx, acc.ID)
	if err != nil {
		c.Close()
		return err
	}
	for _, a := range avisos {
		h.log().Warn("lista de personagens", "aviso", a)
	}
	s.Authenticated = true
	s.Username = username
	s.Account = acc
	s.Characters = chars
	if h.Chan != nil {
		if _, err := h.Chan.Join(c, s); err != nil {
			h.message(c, "Servidor cheio. Tente novamente em instantes.")
			c.Close()
			return err
		}
	}
	h.log().Info("usuario chegou de outro canal",
		"usuario", username, "conta", acc.ID, "de", hd.From, "para", h.Channel, "id", s.ClientID)
	if err := h.sendToWorld(ctx, c, s, hd.Slot); err != nil {
		return err
	}
	h.rejoinParty(s, hd.Group)
	return nil
}

// sendToWorldSends e o $F0B. O Go manda tudo o que a origem mandava depois dele
// (guilda, correio, amigos, pran, nacao, reliquias) logo ao entrar no mundo,
// ainda dentro de sendToWorld; aqui o pedido so e reconhecido.
func (h *Handler) sendToWorldSends(_ context.Context, _ *netsrv.Conn, s *Session, _ protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	return nil
}

// partyTravellers escolhe quem do grupo viaja com o lider: so quem esta perto
// dele (DISTANCE_TO_WATCH). Quem ficou longe sai do grupo, como na origem.
func (h *Handler) partyTravellers(g *Party, lider *Session) []*Session {
	var out []*Session
	for _, id := range h.Parties.MembersOf(g) {
		if id == lider.ClientID {
			continue
		}
		m, _, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		if !m.Position.InRange(lider.Position, h.WatchDistance) {
			soltos := []uint16{id}
			if resto, _ := h.Parties.Remove(id); resto != nil {
				h.refreshParty(resto)
			}
			h.releaseParty(soltos)
			continue
		}
		out = append(out, m)
	}
	return out
}

// sendMemberAway manda um membro do grupo para o canal do lider: registra a
// ordem dele, avisa o cliente e solta a conexao (o que grava o personagem).
func (h *Handler) sendMemberAway(m *Session, target uint8, grupo *PartyMove) {
	_, c, ok := h.Chan.Lookup(m.ClientID)
	if !ok || c == nil || m.Status < Playing || m.SelectedSlot < 0 || m.Account.ID == 0 {
		return
	}
	if !h.Handoffs.Begin(m.Account.ID, m.Username, m.SelectedSlot, h.Channel, target, c.IP(), h.now()) {
		return
	}
	m.handoffAccount = m.Account.ID
	h.Handoffs.SetGroup(m.Account.ID, grupo)
	reply := packets.ChangeChannelPacket{Info1: m.Account.ID, TypeChanel: uint32(target)}
	reply.Header.Index = m.ClientID
	if err := c.SendStruct(CodeChangeChannel, reply); err != nil {
		h.Handoffs.Cancel(m.Account.ID)
		m.handoffAccount = 0
		return
	}
	h.log().Info("troca de canal em grupo", "usuario", m.Username, "de", h.Channel, "para", target)
	c.Close()
}

// rejoinParty refaz no canal novo o grupo que viajou: o primeiro a chegar cria
// o grupo, os outros entram nele, e o lider verdadeiro assume quando chegar.
// A origem criava o grupo no login do lider e juntava quem ja estivesse la.
func (h *Handler) rejoinParty(s *Session, mv *PartyMove) {
	if mv == nil || h.Parties == nil {
		return
	}
	h.movedMu.Lock()
	defer h.movedMu.Unlock()
	if h.moved == nil {
		h.moved = map[uint32]uint16{}
	}
	var g *Party
	if idx, ok := h.moved[mv.Key]; ok {
		g = h.Parties.Get(idx)
	}
	if g == nil {
		novo, err := h.Parties.Create(s.ClientID)
		if err != nil {
			h.log().Warn("refazer grupo no canal novo", "usuario", s.Username, "erro", err)
			return
		}
		novo.ExpAllocate, novo.ItemAllocate = mv.ExpAllocate, mv.ItemAllocate
		h.moved[mv.Key] = novo.Index
		g = novo
	} else if err := h.Parties.Add(g, s.ClientID); err != nil {
		h.log().Warn("entrar no grupo refeito", "usuario", s.Username, "erro", err)
		return
	}
	if s.Account.ID == mv.LeaderAcc {
		_ = h.Parties.SetLeader(g, s.ClientID)
	}
	if g.Size() > 1 {
		h.refreshParty(g)
	}
}
