package game

import (
	"context"
	"strings"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de exclusao de personagem.
const (
	CodeRequestDeleteChar = 0x603  // cliente -> servidor: pedir ou cancelar a exclusao
	CodeDeleteChar        = 0x3E01 // cliente -> servidor: excluir de vez
)

// DeleteDays e DELETE_DAYS_INC de GlobalDefs.pas: dias entre pedir a exclusao
// e poder concluir.
const DeleteDays = 1

// CharacterDeleter marca e conclui a exclusao de personagens.
type CharacterDeleter interface {
	SetDeleteSchedule(ctx context.Context, accountID, charID uint32, deleted bool, when time.Time) error
	Delete(ctx context.Context, accountID, charID uint32) error
}

// deleteNumeric confere a numerica de um pedido de exclusao e conta o erro.
//
// A origem comparava a numerica sem contar tentativa nenhuma: o pedido de
// exclusao era uma forma de testar as 10.000 numericas de 4 digitos sem
// limite, e a mesma numerica protege a entrada no mundo. Aqui o erro soma no
// mesmo contador da entrada, e o personagem trancado derruba a conexao.
func (h *Handler) deleteNumeric(ctx context.Context, c *netsrv.Conn, s *Session, slot int, dado []byte) bool {
	ch := &s.Characters[slot]
	if int(ch.NumericErrors) >= MaxNumericErrors {
		c.Close()
		return false
	}
	if strings.EqualFold(strings.TrimSpace(ch.NumericToken), numericField(dado)) {
		return true
	}
	if ch.NumericErrors < 255 {
		ch.NumericErrors++
	}
	if h.Numerics != nil {
		if err := h.Numerics.SaveNumericErrors(ctx, ch.ID, int(ch.NumericErrors)); err != nil {
			h.log().Error("numerica: gravar contador de erros", "personagem", ch.ID, "erro", err)
		}
	}
	_ = h.sendCharList(c, s)
	h.message(c, "Numerica nao correspondente!")
	return false
}

// canManageChars diz se a sessao esta na tela de personagens.
func canManageChars(s *Session) bool { return s.Authenticated && s.Status != Playing }

// requestDeleteChar e o porte de TPacketHandlers.RequestDeleteChar ($603).
//
// Primeiro pedido: marca o personagem para exclusao daqui a DeleteDays. Com
// Delete ligado e a exclusao ja marcada: cancela. O pedido so vale com a
// numerica certa.
//
// Defeitos da origem: SlotIndex e um DWORD do cliente que indexava
// Account.Characters (3 posicoes) sem faixa; o teste de "prazo vencido" tinha
// o sentido trocado (avisava que ainda nao dava para excluir justamente
// depois que o prazo passou). Esse aviso foi mantido, porque e o que o cliente
// recebe hoje, mas o pedido de exclusao propriamente dito so acontece no
// $3E01.
func (h *Handler) requestDeleteChar(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if !canManageChars(s) {
		return ErrNotAuthenticated
	}
	var pkt packets.DeleteCharacterRequestPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.SlotIndex >= store.MaxCharacterSlots {
		return nil
	}
	slot := int(pkt.SlotIndex)
	ch := &s.Characters[slot]
	if ch.ID == 0 || ch.Name == "" {
		_ = h.sendCharList(c, s)
		h.message(c, "Parece que esse personagem nao existe!")
		return nil
	}
	if strings.TrimSpace(ch.NumericToken) == "" {
		_ = h.sendCharList(c, s)
		h.message(c, "Voce nao possui uma numerica. Crie uma.")
		return nil
	}
	if !h.deleteNumeric(ctx, c, s, slot, pkt.Numeric[:]) {
		return nil
	}

	switch {
	case !ch.Deleted:
		quando := h.now().AddDate(0, 0, DeleteDays)
		if !h.saveDeleteSchedule(ctx, s, ch, true, quando) {
			h.message(c, "Nao foi possivel marcar a exclusao.")
			return nil
		}
		_ = h.sendCharList(c, s)
	case pkt.Delete != 0:
		if !h.saveDeleteSchedule(ctx, s, ch, false, time.Time{}) {
			h.message(c, "Nao foi possivel cancelar a exclusao.")
			return nil
		}
		_ = h.sendCharList(c, s)
		h.message(c, "Exclusao de personagem cancelada!")
	case ch.DeleteTime.Before(h.now()):
		_ = h.sendCharList(c, s)
		h.message(c, "Voce ainda nao pode excluir este personagem!")
	}
	return nil
}

// saveDeleteSchedule grava a marca no banco e na sessao.
func (h *Handler) saveDeleteSchedule(ctx context.Context, s *Session, ch *store.Character, deleted bool, when time.Time) bool {
	if h.CharDelete != nil {
		if err := h.CharDelete.SetDeleteSchedule(ctx, s.Account.ID, ch.ID, deleted, when); err != nil {
			h.log().Error("marcar exclusao", "usuario", s.Username, "personagem", ch.ID, "erro", err)
			return false
		}
	}
	ch.Deleted, ch.DeleteTime = deleted, when
	return true
}

// deleteChar e o porte de TPacketHandlers.DeleteChar ($3E01): a exclusao
// definitiva, depois do prazo.
//
// Defeitos da origem: o slot era convertido com BYTE(Slot), entao o slot 256
// virava 0 e apagava o primeiro personagem; a data de exclusao vazia
// levantava excecao; e o DELETE de itens apagava itens da conta e da guilda
// (ver store.Characters.Delete). Mantido: se o personagem nao tem numerica, a
// conferencia e pulada (so o $603 a exige).
func (h *Handler) deleteChar(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if !canManageChars(s) {
		return ErrNotAuthenticated
	}
	var pkt packets.DeleteChar
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Slot >= store.MaxCharacterSlots {
		return nil
	}
	slot := int(pkt.Slot)
	ch := &s.Characters[slot]
	if ch.ID == 0 || ch.Name == "" || !ch.Deleted {
		_ = h.sendCharList(c, s)
		h.message(c, "Voce nao pode deletar esse personagem!")
		return nil
	}
	if strings.TrimSpace(ch.NumericToken) != "" && !h.deleteNumeric(ctx, c, s, slot, pkt.Numeric[:]) {
		return nil
	}
	if ch.DeleteTime.IsZero() || h.now().Before(ch.DeleteTime) {
		_ = h.sendCharList(c, s)
		h.message(c, "Voce ainda nao pode excluir este personagem!")
		return nil
	}

	// Sai da guilda antes: o membro apagado nao pode ficar na lista.
	if h.Guilds != nil && h.Guilds.Of(ch.ID) != nil {
		h.Guilds.Leave(ch.ID)
	}
	if h.CharDelete != nil {
		if err := h.CharDelete.Delete(ctx, s.Account.ID, ch.ID); err != nil {
			h.log().Error("excluir personagem", "usuario", s.Username, "personagem", ch.ID, "erro", err)
			_ = h.sendCharList(c, s)
			h.message(c, "Nao foi possivel excluir o personagem.")
			return nil
		}
	}
	h.log().Info("personagem excluido", "usuario", s.Username, "personagem", ch.ID, "nome", ch.Name)
	s.Characters[slot] = store.Character{}
	_ = h.sendCharList(c, s)
	h.message(c, "Personagem deletado com sucesso!")
	return nil
}
