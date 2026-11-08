package game

import (
	"context"
	"fmt"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos dos pedidos diversos do personagem.
const (
	CodeRequestCash    = 0x20A  // cliente -> servidor: pedir o saldo premium
	CodeGetStatusPoint = 0x213  // cliente -> servidor: distribuir pontos
	CodeUpdateAction   = 0x304  // os dois sentidos: animacao de acao
	CodePKMode         = 0x307  // os dois sentidos: ligar/desligar PvP
	CodeRemoveBuff     = 0x329  // cliente -> servidor: cancelar buff
	CodeBackToCharList = 0x668  // cliente -> servidor: voltar a lista
	CodeRequestPing    = 0xF93A // cliente -> servidor: pedir o ping
	CodeRefreshBuffs   = 0x16E  // servidor -> cliente: buffs ativos
)

// Acoes que o servidor guarda (so elas viram estado; as demais so repassam).
const (
	actionSit      = 40
	actionSitAlter = 65
)

// requestCash e o porte de TPlayer.SendPlayerCash chamado pelo $20A.
func (h *Handler) requestCash(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	h.sendPlayerCash(c, s)
	return nil
}

// backToCharList e o $668. A origem responde que a funcao esta desativada
// (a chamada real a BackToCharList esta comentada); mantido.
func (h *Handler) backToCharList(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	h.message(c, "Essa funcao foi temporariamente desativada.")
	return nil
}

// getStatusPoint e o porte de TPacketHandlers.GetStatusPoint ($213).
//
// Defeitos da origem corrigidos:
//   - StatusIndex fora de 0..4 caia fora do case e os pontos eram gastos do
//     mesmo jeito: o jogador perdia a reserva sem receber atributo;
//   - o atributo e um WORD e "Inc" dava a volta em 65535;
//   - quantidade zero passava e disparava o recalculo e tres pacotes a toa.
func (h *Handler) getStatusPoint(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.GetStatusPointPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	a := &s.Avatar
	if pkt.StatusAmount == 0 || pkt.StatusAmount > uint32(a.StatusPoints) {
		return nil
	}
	var alvo *uint16
	switch pkt.StatusIndex {
	case 0:
		alvo = &a.Str
	case 1:
		alvo = &a.Agility
	case 2:
		alvo = &a.Int
	case 3:
		alvo = &a.Cons
	case 4:
		alvo = &a.Luck
	default:
		return nil
	}
	if uint32(*alvo)+pkt.StatusAmount > 0xFFFF {
		return nil
	}
	*alvo += uint16(pkt.StatusAmount)
	a.StatusPoints -= uint16(pkt.StatusAmount)

	a.ApplyEquipBonus(h.data(), s.Equip)
	h.sendStatus(c, s)
	return nil
}

// updateAction e o porte de TPacketHandlers.UpdateAction ($304): guarda as
// acoes 40 e 65 e repassa a quem ve o jogador, menos a ele mesmo.
func (h *Handler) updateAction(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SendActionPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Index == actionSit || pkt.Index == actionSitAlter {
		s.Action = pkt.Index
	}
	pkt.Header.Index = s.ClientID
	raw, err := netsrv.Marshal(CodeUpdateAction, pkt)
	if err != nil {
		return err
	}
	if h.Chan != nil {
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
	return nil
}

// pkMode e o porte de TPacketHandlers.PKMode ($307): 1 liga o PvP, qualquer
// outro valor desliga. Repassa o pacote a quem ve o jogador e a ele.
//
// Duelo nao esta portado: a origem recusava LIGAR o PvP durante um duelo
// ("Voce nao pode ligar o PvP em duelo"), e essa conferencia falta aqui.
func (h *Handler) pkMode(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Data > 255 {
		return nil
	}
	s.Avatar.PlayerKill = pkt.Data == 1
	pkt.Header.Index = s.ClientID
	raw, err := netsrv.Marshal(CodePKMode, pkt)
	if err != nil {
		return err
	}
	_ = c.SendRaw(raw)
	if h.Chan != nil {
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
	return nil
}

// removeBuff e o porte de TPacketHandlers.RemoveBuff ($329): o jogador cancela
// um buff proprio. A lista de buffs e reenviada e o status recalculado.
func (h *Handler) removeBuff(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.RemoveBuffPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.BuffIndex > 0xFFFF || s.Base.Buffs == nil {
		return nil
	}
	h.removeBuffAndNotify(s, uint16(pkt.BuffIndex))
	return nil
}

// sendRefreshBuffs manda o $16E com os buffs ativos e a hora em que cada um
// termina, ao jogador e a quem o ve. Porte de TBaseMob.SendRefreshBuffs.
func (h *Handler) sendRefreshBuffs(s *Session) {
	var pkt packets.SendBuffsPacket
	pkt.Header.Index = s.ClientID
	if s.Base.Buffs != nil {
		i := 0
		for _, idx := range s.Base.Buffs.Indexes() {
			if i >= len(pkt.Buffs) {
				break
			}
			inicio, _ := s.Base.Buffs.StartedAt(idx)
			fim := inicio.Add(time.Duration(h.durations().BuffSeconds(idx)) * time.Second)
			pkt.Buffs[i] = idx
			pkt.Time[i] = uint32(max(fim.Unix(), 0))
			i++
		}
	}
	raw, err := netsrv.Marshal(CodeRefreshBuffs, pkt)
	if err != nil {
		h.log().Error("montar buffs", "cliente", s.ClientID, "erro", err)
		return
	}
	if h.Chan != nil {
		_ = h.Chan.SendTo(s.ClientID, raw)
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	}
}

// requestPing e o porte de TPacketHandlers.RequestServerPing ($F93A): diz ao
// jogador quanto tempo passou desde o comando `ping.
func (h *Handler) requestPing(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var ms int64
	if !s.LastPing.IsZero() {
		ms = h.now().Sub(s.LastPing).Milliseconds()
	}
	h.message(c, fmt.Sprintf("%d ms.", ms))
	return nil
}
