package game

import (
	"context"
	"fmt"
	"time"

	"aika/internal/itemfn"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de renovacao de item.
const (
	CodeRenewItem     = 0x21A // cliente -> servidor: renovar item
	CodeRenewItemPran = 0x22A // cliente -> servidor: renovar roupa de pran (mesmo tratamento)
)

// ItemType dos itens de renovacao.
const (
	itemTypePranOutfitRepair = 35  // reparador de roupa de pran
	itemTypeMountLicense     = 520 // licenca de montaria
)

// Faixa de Classe das roupas de pran (ItemList[].Classe 100..104).
const (
	pranOutfitClassMin = 100
	pranOutfitClassMax = 104
)

// baseDatetime e o BASE_DATETIME de GlobalDefs.pas ('31/12/1969 22:00'), a
// referencia da contagem de dias da montaria.
var baseDatetime = time.Date(1969, 12, 31, 22, 0, 0, 0, time.UTC)

// renewItem e o porte de TPacketHandlers.RenoveItem ($21A e $22A).
//
// SlotItem1 e o item de renovacao e SlotItemToRenove o alvo. Mantido da
// origem: o prazo novo conta a partir de AGORA, nao do prazo atual — renovar
// uma montaria que ainda tem 30 dias com uma licenca de 7 deixa 7. Ver
// KNOWN_ISSUES.md.
//
// Diferencas da origem: a origem excluia o slot 0 ("> 0"), nao conferia a
// bolsa dos slots, aceitava os dois campos apontando para o mesmo slot e
// consumia a licenca mesmo com UseEffect zero (prazo "agora", o que expirava
// o item). Aqui o slot 0 vale, a bolsa e conferida e UseEffect zero e recusado.
func (h *Handler) renewItem(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.PacketRenoveItem
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.SlotItem1 == pkt.SlotItemToRenove {
		return nil
	}
	renewer, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.SlotItem1), true)
	if err != nil || renewer.IsEmpty() {
		return nil
	}
	target, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.SlotItemToRenove), true)
	if err != nil || target.IsEmpty() {
		return nil
	}

	t := h.data()
	rinfo := t.Item(int(renewer.Index))
	tinfo := t.Item(int(target.Index))
	if rinfo == nil || tinfo == nil || rinfo.UseEffect == 0 {
		return nil
	}
	horas := time.Duration(rinfo.UseEffect) * time.Hour

	switch rinfo.ItemType {
	case itemTypePranOutfitRepair:
		if tinfo.Classe < pranOutfitClassMin || tinfo.Classe > pranOutfitClassMax {
			return nil
		}
		target.Min = 0
		target.Max = 0
	case itemTypeMountLicense:
		if itemfn.EquipSlot(t, target.Index) != itemfn.ItemTypeMount {
			return nil
		}
	default:
		return nil
	}
	target.SetExpire(tinfo.ItemType, baseDatetime, h.now().Add(horas))
	nome := itemfn.ItemName(t, target.Index)

	itemfn.DecreaseAmount(renewer, 1)

	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.SlotItem1), *renewer)
	h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(pkt.SlotItemToRenove), *target)

	eco := pkt
	eco.Header.Index = s.ClientID
	_ = c.SendStruct(p.Code, eco)
	h.message(c, fmt.Sprintf("Voce renovou o item [%s] por mais %d dias.", nome, rinfo.UseEffect/24))
	return nil
}
