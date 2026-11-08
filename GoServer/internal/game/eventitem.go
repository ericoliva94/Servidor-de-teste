package game

import (
	"context"
	"time"

	"aika/internal/netsrv"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeReceiveEventItem e o $359: o cliente pede os itens de evento guardados.
const CodeReceiveEventItem = 0x359

// diaryItemID e o item do premio diario (TPacketHandlers.ReceiveEventItem).
const diaryItemID = 10467

// EventStore guarda a fila de itens de evento e a data do premio diario.
type EventStore interface {
	List(ctx context.Context, charID uint32) ([]store.EventItem, error)
	Delete(ctx context.Context, charID, id uint32) (bool, error)
	Add(ctx context.Context, charID uint32, itemID, amount uint16) error
	DiaryDue(ctx context.Context, charID uint32, now time.Time) (bool, error)
}

// receiveEventItem e o porte de TPacketHandlers.ReceiveEventItem e
// TPlayer.GetAllEventItems ($359).
//
// Sem espaco no inventario nada acontece. Com o premio diario liberado (24 h
// desde o ultimo) entra mais um item na fila. Depois a fila e entregue, um item
// por slot livre; ao final o servidor responde com o sinal $359, que o cliente
// usa para destravar o botao.
//
// Corrigido: a origem apagava as linhas pelo item_id, o que fazia sumir as
// repetidas; aqui cada linha e apagada pelo proprio id antes de entregar, e se
// a entrega falhar a linha volta para a fila.
func (h *Handler) receiveEventItem(ctx context.Context, c *netsrv.Conn, s *Session, _ protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.freeSlots(s) == 0 {
		h.message(c, "Inventário cheio.")
		return nil
	}
	charID := h.selectedCharID(s)
	if h.EventDB == nil || charID == 0 {
		h.message(c, "Não existem itens de evento para receber.")
		return h.sendHeaderSignal(c, s.ClientID, CodeReceiveEventItem)
	}

	if due, err := h.EventDB.DiaryDue(ctx, charID, h.now()); err != nil {
		h.log().Error("premio diario", "usuario", s.Username, "erro", err)
	} else if due {
		if err := h.EventDB.Add(ctx, charID, diaryItemID, 1); err != nil {
			h.log().Error("guardar premio diario", "usuario", s.Username, "erro", err)
		}
	}

	fila, err := h.EventDB.List(ctx, charID)
	if err != nil {
		h.log().Error("listar itens de evento", "usuario", s.Username, "erro", err)
		return nil
	}
	if len(fila) == 0 {
		h.message(c, "Não existem itens de evento para receber.")
		return h.sendHeaderSignal(c, s.ClientID, CodeReceiveEventItem)
	}
	vagas := h.freeSlots(s)
	if vagas == 0 {
		h.message(c, "Inventário cheio.")
		return h.sendHeaderSignal(c, s.ClientID, CodeReceiveEventItem)
	}
	for _, ev := range fila {
		if vagas == 0 {
			break
		}
		ok, err := h.EventDB.Delete(ctx, charID, ev.ID)
		if err != nil {
			h.log().Error("apagar item de evento", "usuario", s.Username, "erro", err)
			break
		}
		if !ok {
			continue // outro pedido ja o levou
		}
		if !h.putItem(c, s, h.newStackItem(ev.ItemID, ev.Amount)) {
			if err := h.EventDB.Add(ctx, charID, ev.ItemID, ev.Amount); err != nil {
				h.log().Error("devolver item de evento", "usuario", s.Username, "item", ev.ItemID, "erro", err)
			}
			break
		}
		vagas--
	}
	h.message(c, "Não existem mais itens de evento para receber.")
	return h.sendHeaderSignal(c, s.ClientID, CodeReceiveEventItem)
}

// selectedCharID devolve o id do personagem em jogo, ou 0.
func (h *Handler) selectedCharID(s *Session) uint32 {
	if s.SelectedSlot < 0 || s.SelectedSlot >= store.MaxCharacterSlots {
		return 0
	}
	return s.Characters[s.SelectedSlot].ID
}
