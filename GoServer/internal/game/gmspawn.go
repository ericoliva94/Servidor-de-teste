package game

import (
	"context"
	"fmt"

	"aika/internal/entity"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// CodeGMSpawnMob e o $3219: o painel pede um mob na posicao dada ou ao lado do
// personagem.
const CodeGMSpawnMob = 0x3219

// maxTempMobsPerSpecies e o "for I := 1 to 50" de GMSendSpawnMob: o painel nao
// cria mais de 50 mobs temporarios da mesma especie ao mesmo tempo.
const maxTempMobsPerSpecies = 50

// SpawnTemp cria um mob temporario da especie na posicao dada. Devolve false se
// a especie nao existe, se ja ha 50 temporarios dela ou se nao sobra ClientID.
//
// A origem dava ao mob o id (MobID + i) + 9148, que cai na faixa dos mobs de
// dungeon (a partir de 9148) e, como a soma repete entre especies, colide com
// outro mob temporario. Aqui o id e o primeiro livre das faixas de mob.
func (w *MobWorld) SpawnTemp(species int, pos model.Position) (*Mob, bool) {
	if w == nil || w.table == nil || species < 0 || species >= len(w.table.Species) {
		return nil, false
	}
	sp := &w.table.Species[species]
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, m := range w.mobs {
		if m.Temp && m.Species == species {
			n++
		}
	}
	if n >= maxTempMobsPerSpecies {
		return nil, false
	}
	for id := uint32(entity.MinMobID2); id <= uint32(entity.MaxMobID2); id++ {
		if _, usado := w.mobs[uint16(id)]; usado {
			continue
		}
		m := &Mob{
			ClientID: uint16(id), Species: species, Spawn: -1, Position: pos,
			CurHP: sp.HP, MaxHP: sp.HP, Temp: true,
		}
		w.mobs[m.ClientID] = m
		return m, true
	}
	return nil, false
}

// gmSpawnMob e o porte de TPacketHandlers.GMSendSpawnMob ($3219). Moderador,
// GM e administrador podem usar. Com posicao valida o mob nasce nela; senao,
// numa das celulas ao redor do personagem.
func (h *Handler) gmSpawnMob(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SendSpawnMob
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivModerator) || h.Mobs == nil {
		return nil
	}
	if pkt.MobID > 450 {
		return nil
	}
	pos, onSpot := pkt.Position, ValidPosition(pkt.Position) && pkt.Position.X >= MinSpawnCoord && pkt.Position.Y >= MinSpawnCoord
	if !onSpot {
		pos = h.neighborOf(s.Position)
	}
	m, ok := h.Mobs.SpawnTemp(int(pkt.MobID), pos)
	if !ok {
		nome := "desconhecido"
		if t := h.Mobs.Table(); t != nil && int(pkt.MobID) < len(t.Species) {
			nome = t.Species[pkt.MobID].Name
		}
		h.message(c, "Não foi possível spawnar o mob "+nome)
		return nil
	}
	h.broadcastMobSpawn(m)
	onde := "posição vizinha do personagem"
	if onSpot {
		onde = "posição específica"
	}
	h.gmLog(s, fmt.Sprintf("SPAWNOU MOB em %s [%d] X: %.0f Y: %.0f", onde, pkt.MobID, pos.X, pos.Y), s.Avatar.Name)
	return nil
}
