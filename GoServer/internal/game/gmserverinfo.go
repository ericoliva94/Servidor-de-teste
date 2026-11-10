package game

import (
	"context"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos do pedido de informacoes do servidor.
const (
	CodeGMServerInfo      = 0x3214 // painel -> servidor
	CodeGMServerInfoReply = 0x3215 // servidor -> painel
)

// gmServerInfo porta TPacketHandlers.GMRequestServerInformation ($3214): o
// painel recebe, por nacao, quantos jogadores estao em jogo e quantas
// reliquias estao nos templos. So de moderador para cima.
//
// A origem somava por "Servers[i].NationID" de 1 a 5; aqui cada canal (este e
// os de Channels) entra na linha da nacao que ele da. Canal sem nacao nao
// entra. Mais de um canal da mesma nacao soma os jogadores e fica com a
// contagem de reliquias do ultimo, como a origem (que atribuia, nao somava).
func (h *Handler) gmServerInfo(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if !s.gmAtLeast(PrivModerator) {
		return nil
	}
	var out packets.SendServerInfos
	out.Header.Index = s.ClientID

	canais := h.Channels
	if len(canais) == 0 {
		canais = []*Handler{h}
	}
	online := [6]uint32{}
	reliq := [6]uint32{}
	for _, ch := range canais {
		n := int(ch.channelNation())
		if n < 1 || n > 5 || ch.Chan == nil {
			continue
		}
		online[n] += uint32(len(ch.Chan.PlayingIDs()))
		reliq[n] = ch.relicCount()
	}
	out.Server01Online, out.Server02Online, out.Server03Online = online[1], online[2], online[3]
	out.Server04Online, out.Server05Online = online[4], online[5]
	out.Server01Reliq, out.Server02Reliq, out.Server03Reliq = reliq[1], reliq[2], reliq[3]
	out.Server04Reliq, out.Server05Reliq = reliq[4], reliq[5]
	return c.SendStruct(CodeGMServerInfoReply, out)
}

// relicCount conta as reliquias guardadas nos templos do canal.
func (h *Handler) relicCount() uint32 {
	if h.Devir == nil {
		return 0
	}
	var n uint32
	for _, t := range h.Devir.Snapshot() {
		for _, sl := range t.Slots {
			if sl.ItemID != 0 {
				n++
			}
		}
	}
	return n
}
