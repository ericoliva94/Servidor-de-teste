package game

import (
	"context"
	"fmt"
	"math"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeRepairItems e o $340: reparar itens.
const CodeRepairItems = 0x340

// Modos do $340 (campo Unk do pacote).
const (
	repairSelected = 0 // ate 10 itens escolhidos pelo cliente
	repairAll      = 1 // todo o inventario e o equipamento 2 a 7
)

// Tipos de slot do pedido de reparo.
const (
	repairSlotEquip = 0
	repairSlotInv   = 1
	repairSlotNone  = 255
)

// repairTarget e um item a reparar com o ponteiro para o slot.
type repairTarget struct {
	tipo uint8
	slot int
	item *model.Item
}

// repairItems e o porte de TPacketHandlers.RepairItens ($340).
//
// O preco e SellPrince * 0.00025 * durabilidade perdida, multiplicado por
// (Rank - 1) quando o Rank passa de 1. O total e somado num Single (32 bits)
// e arredondado para o par mais proximo (Round do Delphi); os dois detalhes
// sao mantidos porque o cliente mostra o preco calculado do mesmo jeito.
//
// Defeitos da origem corrigidos:
//   - "Max - Min" e feito em inteiro com sinal. Item com Min maior que Max
//     (o modo "reparar tudo" so excluia Min = Max) dava durabilidade perdida
//     NEGATIVA: o total ficava negativo, DecGold(negativo) PAGAVA o jogador e
//     o reparo ainda baixava Min ate Max. Aqui so entra item com Min < Max;
//   - o mesmo slot repetido no pedido era cobrado duas vezes;
//   - o slot de inventario ia ate 63 (as bolsas) sem conferir se a bolsa do
//     slot existe; agora vale a mesma regra do resto do inventario.
//
// Mantido: o servidor nao confere se o jogador esta diante de um NPC de
// reparo. Ver KNOWN_ISSUES.md.
func (h *Handler) repairItems(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.PacketRepareItens
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	var alvos []repairTarget
	switch pkt.Unk {
	case repairSelected:
		alvos = h.selectedRepairs(s, pkt)
	case repairAll:
		alvos = h.allRepairs(s)
	default:
		return nil
	}

	total := h.repairCost(alvos)
	if s.Avatar.Gold < total {
		h.message(c, fmt.Sprintf("Voce necessita de %d de gold para realizar essa acao.", total))
		return nil
	}
	s.Avatar.Gold -= total
	h.sendRefreshMoney(c, s)

	for _, a := range alvos {
		a.item.Min = a.item.Max
		h.sendRefreshItem(c, s, a.tipo, uint16(a.slot), *a.item)
	}

	eco := pkt
	eco.Header.Index = s.ClientID
	_ = c.SendStruct(p.Code, eco)
	return nil
}

// repairable diz se o slot entra no reparo: tem item e perdeu durabilidade.
func repairable(it *model.Item) bool {
	return it != nil && !it.IsEmpty() && it.Max != 0 && it.Min < it.Max
}

// selectedRepairs resolve os ate 10 slots do pedido, sem repetir.
func (h *Handler) selectedRepairs(s *Session, pkt packets.PacketRepareItens) []repairTarget {
	var out []repairTarget
	seen := map[[2]int]bool{}
	for i := range pkt.ItensToRepareSlot {
		tipo, slot := pkt.ItensToRepareSlotType[i], int(pkt.ItensToRepareSlot[i])
		var it *model.Item
		switch tipo {
		case repairSlotEquip:
			if slot < len(s.Equip.Slots) && slot < store.EquipSlots {
				it = &s.Equip.Slots[slot]
			}
		case repairSlotInv:
			if slot < MovableInventorySlots && slot < len(s.Inventory.Slots) {
				if bolsa := inventoryBagSlot(slot); bolsa >= 0 && !s.Inventory.Get(bolsa).IsEmpty() {
					it = &s.Inventory.Slots[slot]
				}
			}
		default:
			continue // 255 e qualquer outro tipo: ignorado, como na origem
		}
		key := [2]int{int(tipo), slot}
		if !repairable(it) || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, repairTarget{tipo: tipo, slot: slot, item: it})
	}
	return out
}

// allRepairs lista o inventario (0 a 59) e o equipamento (2 a 7).
func (h *Handler) allRepairs(s *Session) []repairTarget {
	var out []repairTarget
	for i := 0; i < MovableInventorySlots && i < len(s.Inventory.Slots); i++ {
		if bolsa := inventoryBagSlot(i); bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		if it := &s.Inventory.Slots[i]; repairable(it) {
			out = append(out, repairTarget{tipo: repairSlotInv, slot: i, item: it})
		}
	}
	for i := 2; i <= 7 && i < len(s.Equip.Slots); i++ {
		if it := &s.Equip.Slots[i]; repairable(it) {
			out = append(out, repairTarget{tipo: repairSlotEquip, slot: i, item: it})
		}
	}
	return out
}

// repairCost soma o preco dos itens como a origem: parcelas em ponto
// flutuante de precisao dupla acumuladas num Single, e Round no fim.
func (h *Handler) repairCost(alvos []repairTarget) int64 {
	t := h.data()
	var total float32
	for _, a := range alvos {
		info := t.Item(int(a.item.Index))
		if info == nil {
			continue
		}
		perdido := float64(int(a.item.Max) - int(a.item.Min))
		parcela := float64(info.SellPrince) * 0.00025 * perdido
		if info.Rank > 1 {
			parcela *= float64(int(info.Rank) - 1)
		}
		total += float32(parcela)
	}
	return int64(math.RoundToEven(float64(total)))
}
