package game

import (
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// pranSpawnOffsets sao as 8 casas vizinhas onde a pran pode aparecer.
var pranSpawnOffsets = [8][2]float32{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

// fairyEffect e o efeito que o dono recebe quando a pran esta na forma de fada
// (SendPranSpawn, ramo "pran modo elfa").
func fairyEffect(class uint8) (uint32, bool) {
	switch {
	case class >= 61 && class <= 64:
		return 2, true
	case class >= 71 && class <= 74:
		return 4, true
	case class >= 81 && class <= 84:
		return 8, true
	}
	return 0, false
}

// buildPranSpawn monta o $349 da pran do slot, com a posicao que a pran ja
// tem. Devolve false se nao ha pran, ela e fada, ou nao ha id livre.
func (h *Handler) buildPranSpawn(c *netsrv.Conn, s *Session, slot int) (packets.SendCreatePranPacket, bool) {
	var pkt packets.SendCreatePranPacket
	p := s.Prans[slot]
	if p == nil || IsFairyClass(p.Class) || h.PranIDs == nil {
		return pkt, false
	}
	if s.PranClientID == 0 {
		id, ok := h.PranIDs.Alloc(s.ClientID)
		if !ok {
			h.message(c, "O ClientID da pran não pode ser vazio. Nos envie um ticket.")
			return pkt, false
		}
		s.PranClientID = id
	}
	pkt.Header.Index = s.PranClientID
	pkt.PranClientID = s.PranClientID
	netsrv.PutCString(pkt.Name[:], p.Name)
	for i := 0; i < 8; i++ {
		pkt.Equip[i] = p.Equip[i].Index
	}
	pkt.MaxHP, pkt.CurHP = p.MaxHP, p.CurHP
	pkt.MaxMP, pkt.CurMP = p.MaxMP, p.CurMP
	pkt.SpeedMove = uint8(min(s.Avatar.SpeedMove, 255))
	pkt.SpawnType = SpawnNormal
	pkt.Altura, pkt.Tronco, pkt.Perna = p.Width, p.Chest, p.Leg
	if weapon := p.Equip[5].Index; weapon != 0 {
		if info := h.data().Item(int(weapon)); info != nil {
			if info.DelayUse != 0 {
				pkt.Equip = [8]uint16{}
				pkt.Equip[0] = uint16(info.DelayUse)
				pkt.Equip[6] = info.MeshIDWeapon
			} else {
				for i := 0; i < 8; i++ {
					pkt.Equip[i] = s.Equip.Get(i).Index
				}
			}
			pkt.Altura = uint8(min(info.Duration, 255))
		}
		if slot == 0 {
			pkt.Effects[1] = 74 // so a primeira tem este efeito na origem
		}
	}
	netsrv.PutCString(pkt.Title[:], s.Avatar.Name+" 's Pran")
	pkt.Position = p.Position
	return pkt, true
}

// sendPranSpawn e o porte de TPlayer.SendPranSpawn com SendTo = 0: escolhe uma
// das 8 casas ao redor do dono e mostra a pran ao dono e a quem o ve. Pran em
// forma de fada nao aparece: o dono recebe so o efeito da classe.
func (h *Handler) sendPranSpawn(c *netsrv.Conn, s *Session, slot int) {
	p := s.Prans[slot]
	if p == nil || c == nil {
		return
	}
	if IsFairyClass(p.Class) {
		if ef, ok := fairyEffect(p.Class); ok {
			h.sendEffect(s, ef)
		}
		return
	}
	off := pranSpawnOffsets[h.rand().IntN(len(pranSpawnOffsets))]
	p.Position = model.Position{X: s.Position.X + off[0], Y: s.Position.Y + off[1]}
	pkt, ok := h.buildPranSpawn(c, s, slot)
	if !ok {
		return
	}
	if err := c.SendStruct(CodePranSpawn, pkt); err != nil {
		h.log().Debug("enviar spawn de pran", "usuario", s.Username, "erro", err)
	}
	if h.Chan != nil {
		if raw, err := netsrv.Marshal(CodePranSpawn, pkt); err == nil {
			h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
		}
	}
}

// showPranTo mostra a pran invocada de s a quem acabou de ve-lo (o ramo
// SendTo <> 0 de SendPranSpawn: usa a posicao ja guardada).
func (h *Handler) showPranTo(s *Session, toID uint16) {
	slot, ok := activePran(s)
	if !ok || !s.Prans[slot].Spawned || s.PranClientID == 0 || h.Chan == nil {
		return
	}
	pkt, ok := h.buildPranSpawn(h.connOf(s), s, slot)
	if !ok {
		return
	}
	if raw, err := netsrv.Marshal(CodePranSpawn, pkt); err == nil {
		_ = h.Chan.SendTo(toID, raw)
	}
}

// hidePranFrom apaga a pran de s da tela de toID (SendPranUnspawn com SendTo).
func (h *Handler) hidePranFrom(s *Session, toID uint16) {
	slot, ok := activePran(s)
	if !ok || !s.Prans[slot].Spawned || s.PranClientID == 0 || h.Chan == nil {
		return
	}
	if raw, err := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(s.PranClientID)); err == nil {
		_ = h.Chan.SendTo(toID, raw)
	}
}

// sendPranUnspawn e o porte de SendPranUnspawn com SendTo = 0. Pran de nivel
// abaixo de 4 so tem o efeito, que o dono limpa; as demais somem da tela do
// dono e de quem o ve, e o id volta ao alocador.
func (h *Handler) sendPranUnspawn(s *Session, slot int) {
	p := s.Prans[slot]
	if p == nil || p.DBID == 0 {
		return
	}
	if p.Level < 4 {
		h.sendEffect(s, 0)
		return
	}
	if h.Chan != nil && s.PranClientID != 0 {
		if raw, err := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(s.PranClientID)); err == nil {
			_ = h.Chan.SendTo(s.ClientID, raw)
			h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
		}
	}
	if s.SpawnedPran < 0 {
		if h.PranIDs != nil && s.PranClientID != 0 {
			h.PranIDs.Free(s.PranClientID, s.ClientID)
		}
		s.PranClientID = 0
		p.Spawned = false
	}
}

// sendEmptyPranToWorld limpa o painel da pran no cliente (SendPranToWorld(255)
// manda o pacote zerado).
func (h *Handler) sendEmptyPranToWorld(c *netsrv.Conn) {
	if c == nil {
		return
	}
	var pkt packets.SendPranToWorld
	pkt.Header.Index = shopHeaderIndex
	_ = c.SendStruct(CodePranToWorld, pkt)
}

// pranSlotOf acha qual pran da conta e o item (pelo Identific), ou -1.
func pranSlotOf(s *Session, identific int32) int {
	if identific <= 0 {
		return -1
	}
	for i, p := range s.Prans {
		if p != nil && int32(p.ItemID) == identific {
			return i
		}
	}
	return -1
}

// pranMoveIntent diz, para um movimento inventario <-> Equip[10], qual pran
// aparece e qual some. Porte de SpawnPran/UnspawnPran do MoveItem: o item que
// entra no equipamento chama a sua pran, e o que sai (ou o que e trocado)
// manda a dele embora.
func pranMoveIntent(s *Session, pkt packets.MoveItemPacket, src, dest model.Item, srcIsPran bool) (spawn, unspawn int) {
	spawn, unspawn = -1, -1
	if int(pkt.SrcSlot) != pranEquipSlot && int(pkt.DestSlot) != pranEquipSlot {
		return
	}
	switch {
	case pkt.SrcType == store.SlotTypeInventory && pkt.DestType == store.SlotTypeEquip:
		if srcIsPran {
			spawn = pranSlotOf(s, src.Identific)
			if !dest.IsEmpty() {
				unspawn = pranSlotOf(s, dest.Identific)
			}
		}
	case pkt.SrcType == store.SlotTypeEquip && pkt.DestType == store.SlotTypeInventory:
		if srcIsPran {
			unspawn = pranSlotOf(s, src.Identific)
			if !dest.IsEmpty() {
				spawn = pranSlotOf(s, dest.Identific)
			}
		}
	}
	return
}

// applyPranMove executa o que pranMoveIntent decidiu, na ordem da origem:
// primeiro a pran que sai, depois a que entra.
func (h *Handler) applyPranMove(c *netsrv.Conn, s *Session, spawn, unspawn int) {
	if unspawn >= 0 {
		s.SpawnedPran = -1
		h.sendEmptyPranToWorld(c)
		h.sendPranUnspawn(s, unspawn)
		h.removePranEffects(s)
	}
	if spawn >= 0 {
		p := s.Prans[spawn]
		h.sendPranSpawn(c, s, spawn)
		h.sendPranToWorld(c, s, spawn)
		s.SpawnedPran = spawn
		p.Spawned = true
		h.applyPranEffects(s, p)
	}
}

// pranRemoveRaw monta o pacote que apaga a pran de s, ou nil se nao ha uma no
// mundo.
func (h *Handler) pranRemoveRaw(s *Session) []byte {
	slot, ok := activePran(s)
	if !ok || !s.Prans[slot].Spawned || s.PranClientID == 0 {
		return nil
	}
	raw, err := netsrv.Marshal(CodeRemoveMob, buildRemoveMob(s.PranClientID))
	if err != nil {
		return nil
	}
	return raw
}

// centralPranOk confere a regra da central de pran (armazem 84 e 85): o item que
// entra ou sai dali tem de ser de ItemType 10.
func (h *Handler) centralPranOk(pkt packets.MoveItemPacket, src model.Item) bool {
	central := func(tipo uint32, slot uint16) bool {
		return int(tipo) == store.SlotTypeStorage && (slot == StorageFirstPranSlot || slot == StorageLastPranSlot)
	}
	if !central(uint32(pkt.SrcType), pkt.SrcSlot) && !central(uint32(pkt.DestType), pkt.DestSlot) {
		return true
	}
	return itemfn.ItemType(h.data(), src.Index) == itemfn.ItemTypePran
}
