package game

import (
	"context"

	"aika/internal/netsrv"
	"aika/internal/packets"
)

// CodeRefreshPlayerInfos e o $130: nacao e guilda do jogador.
const CodeRefreshPlayerInfos = 0x130

// Ids dos titulos que alguns itens dao ao serem usados.
const (
	titleNationChosen   = 18 // escolher a nacao
	itemEffectSetNation = 99
)

// AccountNationStore grava a nacao da conta.
type AccountNationStore interface {
	Choose(ctx context.Context, accountID uint32, nation uint8) (bool, error)
}

// itemTitles diz que titulo (nivel 1) o item de indice dado concede ao ser
// usado: as caixas dos fundadores (1134 a 1137) e os itens do Closed Beta
// (10766 a 10768) de ItemFunctions.UseItem.
var itemTitles = map[uint16]int{
	1134: 80, 1135: 81, 1136: 82, 1137: 83,
	10766: 78, 10767: 94, 10768: 95,
}

// setAccountNation e o ramo ITEM_TYPE_SET_ACCOUNT_NATION (UseEffect 99): a conta
// sem nacao adota a do CANAL em que o item foi usado e ganha o titulo 18. Quem
// ja tem nacao nao gasta o item.
//
// Corrigido: a origem so mudava a conta em memoria (e dependia de um salvamento
// posterior); aqui a escolha e gravada na hora e so uma vez.
func (h *Handler) setAccountNation(ctx context.Context, c *netsrv.Conn, s *Session) bool {
	if s.Account.Nation > 0 || h.Nation == 0 {
		return false
	}
	if h.AccountNation == nil {
		h.message(c, "Escolha de nação indisponível.")
		return false
	}
	ok, err := h.AccountNation.Choose(ctx, s.Account.ID, h.Nation)
	if err != nil {
		h.log().Error("escolher nacao", "usuario", s.Username, "erro", err)
		return false
	}
	if !ok {
		return false
	}
	s.Account.Nation = h.Nation
	s.Avatar.Nation = uint16(h.Nation)
	pkt := packets.RefreshPlayerInfosPacket{Index: uint32(s.ClientID), Nation: uint16(h.Nation)}
	pkt.Header.Index = WorldIndex
	if g := s.Avatar.GuildIndex; g > 0 {
		pkt.GuildIndex = g
		pkt.MarechalGuildIndex = uint32(g)
	}
	_ = c.SendStruct(CodeRefreshPlayerInfos, pkt)
	if h.Chan != nil {
		if raw, err := netsrv.Marshal(CodeRefreshPlayerInfos, pkt); err == nil {
			h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
		}
	}
	h.addTitle(c, s, titleNationChosen, 1, true)
	h.log().Info("nacao escolhida", "usuario", s.Username, "nacao", h.Nation, "canal", h.Channel)
	return true
}
