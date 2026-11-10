package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

func spawnPacket(t *testing.T, id uint32, pos model.Position) []byte {
	t.Helper()
	buf, err := netsrv.Marshal(CodeGMSpawnMob, packets.SendSpawnMob{MobID: id, Position: pos})
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func gmComMobs(t *testing.T, priv int) (*Handler, *Session, *espiao) {
	t.Helper()
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Painel", 1000, 1000)
	s.gm.online, s.gm.priv = true, priv
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.drena(10 * time.Millisecond)
	return h, s, e
}

func TestGMSpawnMobAtPosition(t *testing.T) {
	h, s, e := gmComMobs(t, PrivModerator)
	antes := h.Mobs.Count()
	if err := h.gmSpawnMob(context.Background(), e.conn, s,
		asPacket(t, spawnPacket(t, 0, model.Position{X: 1500, Y: 1500}))); err != nil {
		t.Fatal(err)
	}
	if h.Mobs.Count() != antes+1 {
		t.Fatalf("mobs = %d, queria %d", h.Mobs.Count(), antes+1)
	}
	var temp *Mob
	for _, id := range []uint16{3370} {
		temp, _ = h.Mobs.Get(id)
	}
	if temp == nil || !temp.Temp || temp.Position.X != 1500 || temp.CurHP != 100 {
		t.Errorf("mob temporario = %+v", temp)
	}
}

func TestGMSpawnMobNeedsPrivilegeAndValidSpecies(t *testing.T) {
	h, s, e := gmComMobs(t, 1)
	antes := h.Mobs.Count()
	h.gmSpawnMob(context.Background(), e.conn, s, asPacket(t, spawnPacket(t, 0, model.Position{X: 1500, Y: 1500})))
	if h.Mobs.Count() != antes {
		t.Error("sem privilegio nao spawna")
	}
	s.gm.priv = PrivGameMaster
	h.gmSpawnMob(context.Background(), e.conn, s, asPacket(t, spawnPacket(t, 451, model.Position{X: 1500, Y: 1500})))
	h.gmSpawnMob(context.Background(), e.conn, s, asPacket(t, spawnPacket(t, 9, model.Position{X: 1500, Y: 1500})))
	if h.Mobs.Count() != antes {
		t.Error("especie fora da tabela nao spawna")
	}
	if got := mensagemDe(t, e); got == "" {
		t.Error("deveria avisar que nao foi possivel")
	}
}

func TestGMSpawnMobNeighborWhenPositionInvalid(t *testing.T) {
	h, s, e := gmComMobs(t, PrivAdmin)
	h.gmSpawnMob(context.Background(), e.conn, s, asPacket(t, spawnPacket(t, 0, model.Position{})))
	m, _ := h.Mobs.Get(3370)
	if m == nil {
		t.Fatal("o mob deveria nascer ao lado do personagem")
	}
	if d := m.Position.Distance(s.Position); d > 2 || d == 0 {
		t.Errorf("distancia = %d", d)
	}
}

func TestTempMobCapAndNoRespawn(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	for i := 0; i < maxTempMobsPerSpecies; i++ {
		if _, ok := h.Mobs.SpawnTemp(0, model.Position{X: 1500, Y: 1500}); !ok {
			t.Fatalf("mob %d recusado", i)
		}
	}
	m, ok := h.Mobs.SpawnTemp(0, model.Position{X: 1500, Y: 1500})
	if ok || m != nil {
		t.Error("o 51o mob da especie deveria ser recusado")
	}
	// Ao morrer, sai do mundo em vez de reaparecer.
	tmp, _ := h.Mobs.Get(3370)
	h.Mobs.Hit(tmp, 1000, 1, h.now())
	h.Mobs.Respawn(h.now().Add(time.Minute))
	if _, existe := h.Mobs.Get(3370); existe {
		t.Error("o mob temporario morto deveria sair do mundo")
	}
	_ = mobdata.FirstMobClientID
}
