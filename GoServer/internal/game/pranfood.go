package game

import "aika/internal/netsrv"

// Passo da fome da pran: a origem dormia 200 s entre passadas
// (TPranFoodThread.Create(200000, ...)).
const (
	pranFoodStep       = 3  // comida perdida por passada
	pranHungryBelow    = 25 // abaixo disto o jogador e avisado
	pranHungryMsgNull  = 32 // o "Null" de SendClientMessage(msg, 16, 32)
	pranHungryMsgType1 = 16
)

// PranFoodTick porta TPranFoodThread.CheckFood: a pran invocada perde 3 de
// comida por passada (piso zero) e, abaixo de 25, o dono recebe "Pran esta com
// fome." Devolve quantas prans perderam comida. O intervalo e do chamador.
//
// O que a origem fazia com a comida zerada (mandar a pran para a estacao e
// baixar a devocao) esta todo comentado la, e portanto nao existe aqui.
func (h *Handler) PranFoodTick() int {
	if h.Chan == nil {
		return 0
	}
	n := 0
	for _, id := range h.Chan.PlayingIDs() {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || s.Status < Playing {
			continue
		}
		slot, ok := activePran(s)
		if !ok {
			continue
		}
		p := s.Prans[slot]
		if p.Food == 0 {
			continue
		}
		if p.Food <= pranFoodStep {
			p.Food = 0
		} else {
			p.Food -= pranFoodStep
		}
		n++
		if p.Food < pranHungryBelow {
			h.hungryMessage(c)
		}
	}
	return n
}

func (h *Handler) hungryMessage(c *netsrv.Conn) {
	if c == nil {
		return
	}
	var pkt clientMessage
	pkt.Null = pranHungryMsgNull
	pkt.Type1 = pranHungryMsgType1
	netsrv.PutCString(pkt.Msg[:], "Pran está com fome.")
	_ = c.SendStruct(CodeClientMsg, pkt)
}
