package game

import (
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// Codigos de atualizacao de estado do personagem.
const (
	CodeCurrentHPMP   = 0x103 // servidor -> cliente: HP e MP atuais
	CodeRefreshPoint  = 0x109 // servidor -> cliente: atributos
	CodeRefreshStatus = 0x10A // servidor -> cliente: dano, defesa, velocidade
)

// sendStatus manda o $10A. Porte de TBaseMob.SendStatus.
//
// A origem terminava chamando RequestAllAttributes com um buffer local de 13
// bytes declarado como Array [0..12] e zerado so nos 12 primeiros, passado a
// um handler que o interpreta como pacote: o 13o byte ia com lixo de pilha.
// Aqui a parte que importa — os campos derivados — e enviada direto.
func (h *Handler) sendStatus(c *netsrv.Conn, s *Session) {
	var pkt packets.SendRefreshStatus
	pkt.Header.Index = WorldIndex
	a := &s.Avatar
	pkt.DNFis = a.AtkFis
	pkt.DefFis = a.DefFis
	pkt.DNMag = a.AtkMag
	pkt.DEFMag = a.DefMag
	pkt.SpeedMove = a.SpeedMove
	pkt.Critico = a.Critical
	pkt.Esquiva = a.Esquiva
	pkt.Acerto = a.Acerto
	pkt.Duplo = a.DuploAtk
	pkt.Resist = a.Resistence
	if err := c.SendStruct(CodeRefreshStatus, pkt); err != nil {
		h.log().Debug("falha ao enviar status", "usuario", s.Username, "erro", err)
		return
	}
	h.sendRefreshPoint(c, s)
	h.sendCurrentHPMP(c, s)
}

// sendRefreshPoint manda o $109. Porte de TBaseMob.SendRefreshPoint.
func (h *Handler) sendRefreshPoint(c *netsrv.Conn, s *Session) {
	var pkt packets.SendRefreshPoint
	pkt.Header.Index = WorldIndex
	a := &s.Avatar
	pkt.Pontos = packets.Points{
		For_:   a.Str,
		AGI:    a.Agility,
		INTE:   a.Int,
		CON:    a.Cons,
		SORT:   a.Luck,
		STATUS: a.StatusPoints,
	}
	pkt.StatusPoint = a.StatusPoints
	pkt.SkillsPoint = a.SkillPoint
	if err := c.SendStruct(CodeRefreshPoint, pkt); err != nil {
		h.log().Debug("falha ao enviar atributos", "usuario", s.Username, "erro", err)
	}
}

// sendCurrentHPMP manda o $103 ao jogador e a quem o ve. Porte de
// TBaseMob.SendCurrentHPMP.
//
// A origem recalculava o maximo dentro desta funcao e terminava com Sleep(1)
// — uma pausa dentro do tratamento de pacote, que segurava a thread do
// jogador. Aqui o maximo ja vem calculado e nao ha pausa.
func (h *Handler) sendCurrentHPMP(c *netsrv.Conn, s *Session) {
	a := &s.Avatar
	if a.CurHP > a.MaxHP {
		a.CurHP = a.MaxHP
	}
	if a.CurMP > a.MaxMP {
		a.CurMP = a.MaxMP
	}

	var pkt packets.SendCurrentHPMPPacket
	pkt.Header.Index = s.ClientID
	pkt.MaxHP = a.MaxHP
	pkt.CurHP = a.CurHP
	pkt.MaxMP = a.MaxMP
	pkt.CurMP = a.CurMP

	raw, err := netsrv.Marshal(CodeCurrentHPMP, pkt)
	if err != nil {
		h.log().Error("montar HP/MP", "cliente", s.ClientID, "erro", err)
		return
	}
	if err := c.SendRaw(raw); err != nil {
		h.log().Debug("falha ao enviar HP/MP", "usuario", s.Username, "erro", err)
	}
	if h.Chan != nil {
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
}
