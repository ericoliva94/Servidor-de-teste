package game

import (
	"context"
	"sync/atomic"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos do duelo.
const (
	CodeDuelRequest  = 0x395 // cliente -> servidor e servidor -> cliente: desafio
	CodeDuelResponse = 0x396 // cliente -> servidor: aceitar ou recusar
	CodeDuelEnd      = 0x1A0 // servidor -> cliente: ganhou (1) ou perdeu (0)
	CodeDuelTime     = 0x1A2 // servidor -> cliente: contagem para comecar
)

// Tempos e limites do duelo (GlobalDefs.pas e TDuelThread).
const (
	duelWait       = 15 * time.Second // DUEL_TIME_WAIT: ninguem bate antes disso
	duelMax        = 5 * time.Minute  // limite de duracao, depois da espera
	duelOutLimit   = 5                // segundos fora da area antes de perder
	duelMinLevel   = 10
	duelFlagBaseID = 10148 // a bandeira do duelo n tem id 10148 + n
	duelFlagFace   = 413
	duelFlagWeapon = 1481
	duelEndHP      = 10 // o duelo acaba quando alguem chega a 10 de vida
)

// duelState e o duelo de um jogador. Os dois lados guardam o mesmo duelo.
type duelState struct {
	// requester e quem desafiou este jogador e espera a resposta.
	requester uint16
	active    bool
	with      uint16
	// host diz que este jogador desafiou: a origem tratava P1 (desafiante) e P2
	// de forma assimetrica no desempate.
	host    bool
	start   time.Time
	flagPos model.Position
	flagID  uint16
	outSecs int
}

// duelSeq numera as bandeiras de duelo (Servers[].DuelCount).
type duelSeq struct{ n atomic.Uint32 }

func (d *duelSeq) next() uint16 {
	// A origem somava sem teto e o id voltava ao estourar; aqui ele gira em 1000.
	return duelFlagBaseID + uint16(d.n.Add(1)%1000)
}

// duelWaiting diz se o duelo de s ainda esta na espera de 15 s.
func (h *Handler) duelWaiting(s *Session) bool {
	return h.now().Sub(s.Duel.start) <= duelWait
}

// requestDuel e o porte de TPacketHandlers.SendRequestDuel ($395).
//
// A origem lia Players[ClientId] sem conferir a faixa nem se era o proprio
// jogador, e o desafiante ja em duelo podia desafiar outro.
func (h *Handler) requestDuel(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SendDuelResquest
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	var alvoID uint16
	if pkt.ClientID > 0 {
		if pkt.ClientID > 0xFFFF {
			return nil
		}
		alvoID = uint16(pkt.ClientID)
	} else if h.Chan != nil {
		alvoID, _ = h.Chan.FindByCharacter(netsrv.CString(pkt.Nick[:]))
	}
	alvo, alvoConn, ok := h.sessionOf(alvoID)
	if !ok || alvo == s {
		h.message(c, "Alvo nao esta logado.")
		return nil
	}
	switch {
	case s.Avatar.PlayerKill:
		h.message(c, "Nao e possivel duelar com o PvP ligado.")
	case s.Duel.active:
		h.message(c, "Voce ja esta em um duelo.")
	case alvo.Avatar.Nation != s.Avatar.Nation:
		h.message(c, "O alvo nao e da sua nacao.")
	case s.Avatar.Level < duelMinLevel || alvo.Avatar.Level < duelMinLevel:
		h.message(c, "E necessario que os dois jogadores sejam nivel 10 para duelar.")
	case h.samePartyOrNil(s, alvo):
		h.message(c, "Jogador invalido. Pertence ao seu grupo.")
	case alvo.Duel.active:
		h.message(c, "O jogador ja esta em um duelo.")
	default:
		pkt.Header.Index = s.ClientID
		pkt.ClientID = uint32(s.ClientID)
		netsrv.PutCString(pkt.Nick[:], s.Avatar.Name)
		alvo.Duel.requester = s.ClientID
		_ = alvoConn.SendStruct(CodeDuelRequest, pkt)
	}
	return nil
}

func (h *Handler) samePartyOrNil(a, b *Session) bool {
	if h.Parties == nil {
		return false
	}
	ga := h.Parties.Of(a.ClientID)
	return ga != nil && ga == h.Parties.Of(b.ClientID)
}

// answerDuel e o porte de TPacketHandlers.DuelResponse ($396).
func (h *Handler) answerDuel(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SendDuelResponse
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if s.Duel.requester == 0 {
		return nil
	}
	reqID := s.Duel.requester
	s.Duel.requester = 0
	req, reqConn, ok := h.sessionOf(reqID)
	if !ok {
		h.message(c, "O jogador desafiante nao esta mais online.")
		return nil
	}
	switch {
	case pkt.Response == 0:
		h.message(reqConn, "O jogador desafiado recusou seu convite de duelo.")
	case s.Position.Distance(req.Position) > h.watchDistance():
		h.message(c, "O jogador desafiante esta muito longe.")
	case req.Dead:
		h.message(c, "O jogador desafiante esta morto.")
	case s.Avatar.PlayerKill:
		h.message(c, "Nao possivel duelar com o PvP ligado.")
	case s.Duel.active || req.Duel.active:
		h.message(c, "Voce ja esta em um duelo.")
	default:
		h.startDuel(req, s)
	}
	return nil
}

// startDuel e o porte de TPlayer.CreateDuelSession: marca a bandeira ao lado do
// desafiante e liga o duelo nos dois.
func (h *Handler) startDuel(host, guest *Session) {
	pos := model.Position{X: host.Position.X + float32(h.rand().IntN(3)) - 1, Y: host.Position.Y + float32(h.rand().IntN(3)) - 1}
	flag := h.duelSeq.next()
	now := h.now()
	host.Duel = duelState{active: true, with: guest.ClientID, host: true, start: now, flagPos: pos, flagID: flag}
	guest.Duel = duelState{active: true, with: host.ClientID, start: now, flagPos: pos, flagID: flag}

	for _, s := range []*Session{host, guest} {
		if c := h.connOf(s); c != nil {
			_ = c.SendStruct(CodeDuelTime, packets.SendDuelTime{Header: packets.PacketHeader{Index: s.ClientID}, SecondsCount: uint32(duelWait / time.Second)})
		}
	}
	h.broadcastDuelFlag(host, guest, flag, pos)
}

// broadcastDuelFlag desenha a bandeira para os dois e para quem os ve.
func (h *Handler) broadcastDuelFlag(host, guest *Session, flag uint16, pos model.Position) {
	if h.Chan == nil {
		return
	}
	var pkt packets.SendCreateMobPacket
	pkt.Header.Index = flag
	netsrv.PutCString(pkt.Name[:], "365")
	pkt.Equip[0], pkt.Equip[6] = duelFlagFace, duelFlagWeapon
	pkt.Position.X, pkt.Position.Y = pos.X, pos.Y
	pkt.MaxHP, pkt.CurHP = 20000, 20000
	pkt.Unk0 = 0x0A
	pkt.Effects[1] = 0x1D
	pkt.EffectType = 1
	pkt.Altura, pkt.Tronco, pkt.Perna = 4, 0x77, 0x77
	netsrv.PutCString(pkt.Title[:], host.Avatar.Name+" vs. "+guest.Avatar.Name)
	raw, err := netsrv.Marshal(CodeCreateMob, pkt)
	if err != nil {
		return
	}
	alvos := append([]uint16{host.ClientID, guest.ClientID}, host.Base.VisiblePlayers.IDs()...)
	h.Chan.Broadcast(alvos, raw)
}

// duelTick avanca todos os duelos em um segundo. Chamado pelo laco de buffs.
// Porte do laco de TDuelThread: a espera de 15 s, a area, o limite de 5 minutos
// e o fim por vida em 10 ou menos.
func (h *Handler) duelTick() {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		s, _, ok := h.Chan.Lookup(id)
		if !ok || !s.Duel.active || !s.Duel.host {
			continue
		}
		h.stepDuel(s)
	}
}

// stepDuel avanca o duelo cujo desafiante e p1.
func (h *Handler) stepDuel(p1 *Session) {
	p2, _, ok := h.Chan.Lookup(p1.Duel.with)
	if !ok || !p2.Duel.active || p2.Duel.with != p1.ClientID {
		// O outro lado saiu: quem ficou ganha.
		h.endDuel(p1, nil, p1)
		return
	}
	if h.duelWaiting(p1) {
		return
	}
	// Fora da area: 5 segundos para voltar.
	for _, par := range [2][2]*Session{{p1, p2}, {p2, p1}} {
		me, outro := par[0], par[1]
		if me.Position.Distance(me.Duel.flagPos) <= h.watchDistance() {
			me.Duel.outSecs = 0
			continue
		}
		me.Duel.outSecs++
		if me.Duel.outSecs == 1 {
			h.message(h.connOf(me), "Volte em ate 5 segundos para a area de duelo.")
			h.message(h.connOf(outro), "Seu alvo esta fora da area de duelo. 5 Segundos para voltar.")
		} else if me.Duel.outSecs >= duelOutLimit {
			h.endDuel(p1, p2, outro)
			return
		}
	}
	if h.now().Sub(p1.Duel.start) >= duelMax+duelWait {
		h.message(h.connOf(p1), "Duelo terminou. Limite maximo de tempo e de 5 minutos.")
		h.message(h.connOf(p2), "Duelo terminou. Limite maximo de tempo e de 5 minutos.")
		h.endDuel(p1, p2, nil)
		return
	}
	switch {
	case p1.Avatar.CurHP <= duelEndHP:
		h.endDuel(p1, p2, p2)
	case p2.Avatar.CurHP <= duelEndHP:
		h.endDuel(p1, p2, p1)
	}
}

// endDuel encerra o duelo: avisa vitoria e derrota (nenhuma na falta de
// vencedor), tira a bandeira da tela de todos e solta os dois.
func (h *Handler) endDuel(p1, p2, winner *Session) {
	flag := p1.Duel.flagID
	vis := p1.Base.VisiblePlayers.IDs()
	solta := func(s *Session) {
		if s != nil {
			s.Duel = duelState{}
		}
	}
	solta(p1)
	solta(p2)

	for _, s := range []*Session{p1, p2} {
		if s == nil {
			continue
		}
		if c := h.connOf(s); c != nil {
			res := uint32(0)
			if s == winner {
				res = 1
			}
			_ = c.SendStruct(CodeDuelEnd, packets.MessageEndDuel{Header: packets.PacketHeader{Index: 0x7535}, WonLose: res})
		}
	}
	var msg string
	switch {
	case winner != nil && p2 != nil:
		perdedor := p1
		if winner == p1 {
			perdedor = p2
		}
		msg = winner.Avatar.Name + " venceu " + perdedor.Avatar.Name + " no duelo."
	case winner != nil:
		msg = winner.Avatar.Name + " venceu o duelo."
	case p2 != nil:
		msg = "Duelo entre " + p1.Avatar.Name + " e " + p2.Avatar.Name + " deu empate."
	}
	ids := vis
	for _, s := range []*Session{p1, p2} {
		if s != nil {
			ids = append(ids, s.ClientID)
		}
	}
	if h.Chan == nil {
		return
	}
	rem, _ := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(flag))
	for _, id := range ids {
		if o, c, ok := h.Chan.Lookup(id); ok && o.Status >= Playing {
			if msg != "" {
				h.message(c, msg)
			}
			_ = h.Chan.SendTo(id, rem)
		}
	}
}

// duelOnDisconnect: quem cai no meio do duelo perde.
func (h *Handler) duelOnDisconnect(s *Session) {
	if !s.Duel.active || h.Chan == nil {
		return
	}
	other, _, ok := h.Chan.Lookup(s.Duel.with)
	s.Duel = duelState{}
	if ok && other.Duel.active && other.Duel.with == s.ClientID {
		h.endDuel(other, nil, other)
	}
}
