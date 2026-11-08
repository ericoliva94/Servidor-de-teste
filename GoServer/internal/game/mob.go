package game

import (
	"sync"
	"time"

	"aika/internal/entity"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// CodeSpawnMob e o $35E, usado para desenhar um mob.
const CodeSpawnMob = 0x35E

// MinSpawnCoord e o "if((CurrentPos.X < 50) or (CurrentPos.Y < 50)) then Exit"
// da origem: ponto de spawn perto da origem do mapa nao e desenhado.
const MinSpawnCoord = 50

// Mob e o estado vivo de um ponto de spawn.
//
// Porte da parte mutavel de TMobSPosition (MOB.pas). O que e fixo por especie
// fica em mobdata.Species; aqui fica so o que muda em jogo.
type Mob struct {
	ClientID uint16
	// Species e o indice da especie em mobdata.Table.
	Species int
	// Spawn e o indice do ponto dentro da especie.
	Spawn int

	Position model.Position
	CurHP    uint32
	MaxHP    uint32

	Dead   bool
	DeadAt time.Time

	// State diz para onde a patrulha esta indo (o MovedTo da origem).
	State MobState
	// Moves conta os passos desde o ultimo pacote de movimento
	// (MovimentsTime na origem).
	Moves int
	// LastMove e LastAttack sao os instantes do ultimo passo e do ultimo
	// golpe (LastMoviment e LastMyAttack na origem).
	LastMove   time.Time
	LastAttack time.Time

	// AttackerID e quem bateu por ultimo; FirstAttackerID e quem bateu
	// primeiro, que na origem decidia quem leva o drop.
	AttackerID      uint16
	FirstAttackerID uint16
	Attacked        bool
}

// MobWorld guarda os mobs vivos de um canal.
//
// Substitui o "MOBS.TMobS[0..449].MobsP[0..49]" do original, onde o indice da
// especie e o do ponto eram usados sem checagem de faixa em todo lugar que
// tocava mob. Aqui o acesso e por ClientID.
type MobWorld struct {
	table *mobdata.Table

	mu   sync.RWMutex
	mobs map[uint16]*Mob
}

// NewMobWorld cria o mundo de mobs a partir da tabela carregada.
//
// So entram as especies marcadas com IsActiveToSpawn, como na origem
// ("if not(MOBS.TMobS[Mobid].IsActiveToSpawn) then Exit"), e so os pontos
// cuja posicao passa de MinSpawnCoord.
func NewMobWorld(t *mobdata.Table) *MobWorld {
	w := &MobWorld{table: t, mobs: make(map[uint16]*Mob)}
	if t == nil {
		return w
	}
	for si := range t.Species {
		sp := &t.Species[si]
		if !sp.ActiveToSpawn {
			continue
		}
		for pi := range sp.Spawns {
			pt := &sp.Spawns[pi]
			if pt.InitPos.X < MinSpawnCoord || pt.InitPos.Y < MinSpawnCoord {
				continue
			}
			if !ValidPosition(pt.InitPos) {
				continue
			}
			w.mobs[pt.ClientID] = &Mob{
				ClientID: pt.ClientID,
				Species:  si,
				Spawn:    pi,
				Position: pt.InitPos,
				CurHP:    sp.HP,
				MaxHP:    sp.HP,
			}
		}
	}
	return w
}

// Count conta os mobs no mundo.
func (w *MobWorld) Count() int {
	if w == nil {
		return 0
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.mobs)
}

// Table devolve a tabela estatica.
func (w *MobWorld) Table() *mobdata.Table {
	if w == nil {
		return nil
	}
	return w.table
}

// Get devolve o mob de um ClientID.
func (w *MobWorld) Get(clientID uint16) (*Mob, bool) {
	if w == nil {
		return nil, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	m, ok := w.mobs[clientID]
	return m, ok
}

// Species devolve a especie de um mob.
func (w *MobWorld) SpeciesOf(m *Mob) *mobdata.Species {
	if w == nil || w.table == nil || m == nil {
		return nil
	}
	if m.Species < 0 || m.Species >= len(w.table.Species) {
		return nil
	}
	return &w.table.Species[m.Species]
}

// Near devolve os mobs vivos a no maximo dist celulas de pos.
//
// A origem varria MOBS.TMobS[0..449].MobsP[0..49] inteiro — 22.500 posicoes —
// a cada atualizacao de visibilidade de cada jogador. Aqui a varredura e
// sobre os mobs que existem de verdade; uma grade por regiao fica para quando
// o custo pesar.
func (w *MobWorld) Near(pos model.Position, dist uint16) []*Mob {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []*Mob
	for _, m := range w.mobs {
		if m.Dead {
			continue
		}
		if pos.InRange(m.Position, dist) {
			out = append(out, m)
		}
	}
	return out
}

// Hit desconta vida de um mob e diz se ele morreu com este golpe.
//
// Seguro para uso concorrente: dois jogadores podem bater no mesmo mob ao
// mesmo tempo. Na origem isso era "Dec(Self.HP, Dano)" sem trava nenhuma, com
// uma thread por jogador, entao dois golpes simultaneos podiam perder um dos
// descontos ou matar o mob duas vezes — e a segunda morte dava exp de novo.
func (w *MobWorld) Hit(m *Mob, dano int, atacante uint16, agora time.Time) (morreu bool) {
	if w == nil || m == nil || dano <= 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if m.Dead {
		return false
	}
	if !m.Attacked {
		m.Attacked = true
		m.FirstAttackerID = atacante
	}
	m.AttackerID = atacante
	if uint32(dano) >= m.CurHP {
		m.CurHP = 0
		m.Dead = true
		m.DeadAt = agora
		return true
	}
	m.CurHP -= uint32(dano)
	return false
}

// Respawn devolve ao mundo os mobs cujo tempo de respawn passou.
//
// Porte do bloco de reespawn de UpdateSpawnToPlayers. Ali o teste era
// "if (Self.DeadTime.Year >= Now.Year)", que descarta o reespawn de um mob
// morto em 31 de dezembro quando o ano vira: ele ficava morto para sempre.
// Aqui a conta e sobre a diferenca de tempo, que nao depende do ano.
func (w *MobWorld) Respawn(agora time.Time) []*Mob {
	if w == nil || w.table == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	var voltaram []*Mob
	for _, m := range w.mobs {
		if !m.Dead || m.DeadAt.IsZero() {
			continue
		}
		sp := w.speciesLocked(m)
		if sp == nil {
			continue
		}
		espera := time.Duration(sp.RespawnSeconds) * time.Second
		if espera <= 0 {
			// Respawn zero no arquivo faria o mob voltar no mesmo instante em
			// que morre, e o jogador nunca o veria cair. A origem tratava
			// zero como "ja passou"; aqui o piso e um segundo.
			espera = time.Second
		}
		if agora.Sub(m.DeadAt) < espera {
			continue
		}
		pt := w.spawnLocked(m)
		if pt == nil {
			continue
		}
		m.Dead = false
		m.DeadAt = time.Time{}
		m.Attacked = false
		m.AttackerID = 0
		m.FirstAttackerID = 0
		m.Position = pt.InitPos
		m.CurHP = sp.HP
		m.MaxHP = sp.HP
		voltaram = append(voltaram, m)
	}
	return voltaram
}

func (w *MobWorld) speciesLocked(m *Mob) *mobdata.Species {
	if m.Species < 0 || m.Species >= len(w.table.Species) {
		return nil
	}
	return &w.table.Species[m.Species]
}

func (w *MobWorld) spawnLocked(m *Mob) *mobdata.Spawn {
	sp := w.speciesLocked(m)
	if sp == nil || m.Spawn < 0 || m.Spawn >= len(sp.Spawns) {
		return nil
	}
	return &sp.Spawns[m.Spawn]
}

// ---- pacotes

// buildSpawnMob monta o $35E de um mob. Porte de TPlayer.SpawnMob.
//
// Duas correcoes sobre a origem:
//
// Ali "Packet.CurHP := InitHP" (o maximo da especie) e
// "Packet.MaxHp := MobsP[].HP" (a vida atual) estavam trocados, entao um mob
// machucado aparecia com a barra cheia e, ao morrer, o cliente mostrava a
// barra cair de um valor que nunca foi o maximo.
//
// E "Packet.CurMp := InitHP" pegava o MP do HP, o mesmo erro de copia que
// aparece em GetCreateMob.
func (h *Handler) buildSpawnMob(m *Mob, sp *mobdata.Species) packets.SpawnMobPacket {
	var pkt packets.SpawnMobPacket
	pkt.Header.Index = m.ClientID
	pkt.Equip[0] = sp.Equip[0]
	pkt.Equip[1] = sp.Equip[1]
	pkt.Equip[6] = sp.Equip[6]
	pkt.Position.X = m.Position.X
	pkt.Position.Y = m.Position.Y
	pkt.Rotation = sp.Rotation
	pkt.CurHP = m.CurHP
	pkt.MaxHP = m.MaxHP
	pkt.CurMP = m.CurHP
	pkt.MaxMP = m.MaxHP
	pkt.Level = sp.Level
	if sp.IsService {
		pkt.IsService = 1
	}
	pkt.SpawnType = sp.SpawnType
	pkt.Altura = sp.Elevation
	pkt.Tronco = sp.Cabeca
	pkt.Perna = sp.Perna
	pkt.MobType = uint8(min(sp.MobType, 255))
	pkt.MobName = sp.IntName
	return pkt
}

// sendMobSpawn manda o $35E de um mob a um jogador.
//
// A origem chamava Self.SendPacket duas vezes seguidas (a primeira sem ponto
// e virgula), mandando o mesmo pacote em dobro a cada spawn de mob. Aqui vai
// uma vez.
func (h *Handler) sendMobSpawn(para *Session, m *Mob) {
	if h.Chan == nil || h.Mobs == nil {
		return
	}
	sp := h.Mobs.SpeciesOf(m)
	if sp == nil {
		return
	}
	raw, err := netsrv.Marshal(CodeSpawnMob, h.buildSpawnMob(m, sp))
	if err != nil {
		h.log().Error("montar spawn de mob", "mob", m.ClientID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(para.ClientID, raw)
}

// sendMobRemove apaga um mob da tela de um jogador. Porte de
// TPlayer.UnspawnMob.
func (h *Handler) sendMobRemove(para *Session, mobID uint16) {
	if h.Chan == nil {
		return
	}
	var pkt packets.SendRemoveMobPacket
	pkt.Header.Index = mobID
	pkt.Index = uint32(mobID)
	raw, err := netsrv.Marshal(CodeRemoveMob, pkt)
	if err != nil {
		h.log().Error("montar remocao de mob", "mob", mobID, "erro", err)
		return
	}
	_ = h.Chan.SendTo(para.ClientID, raw)
}

// refreshVisibleMobs recalcula quais mobs o jogador ve.
//
// Porte de TMobSPosition.UpdateSpawnToPlayers visto do lado do jogador. A
// origem rodava esse procedimento por mob, varrendo o vetor inteiro de 22.500
// posicoes; aqui a varredura e por jogador sobre os mobs que existem.
func (h *Handler) refreshVisibleMobs(s *Session) {
	// Dentro de uma dungeon os mobs sao os da instancia. A origem nao fazia
	// essa separacao: o laco de visibilidade do mundo seguia rodando para
	// quem estava dentro, e o laco da dungeon tambem, entao o jogador recebia
	// spawn dos mobs do mundo na posicao da dungeon.
	if h.Dungeons != nil {
		if _, dentro := h.Dungeons.Of(s.CharID()); dentro {
			h.SendDungeonMobs(s)
			return
		}
	}
	if h.Mobs == nil || h.Chan == nil {
		return
	}
	if s.Status < Playing || s.Dead {
		return
	}

	ver := h.watchDistance()
	esquecer := h.forgetDistance()

	// Entra quem esta a ver celulas e nao estava visivel.
	perto := h.Mobs.Near(s.Position, ver)
	vistos := make(map[uint16]bool, len(perto))
	for _, m := range perto {
		vistos[m.ClientID] = true
		if s.Base.VisibleMobs.Contains(m.ClientID) {
			continue
		}
		s.Base.VisibleMobs.Add(m.ClientID)
		h.sendMobSpawn(s, m)
	}

	// Sai quem passou de esquecer celulas, ou morreu.
	for _, id := range s.Base.VisibleMobs.IDs() {
		if vistos[id] {
			continue
		}
		m, ok := h.Mobs.Get(id)
		if !ok || m.Dead || !s.Position.InRange(m.Position, esquecer) {
			s.Base.VisibleMobs.Remove(id)
			if s.Base.Targets != nil {
				s.Base.Targets.Remove(id)
			}
			h.sendMobRemove(s, id)
		}
	}
}

// broadcastMobRemove apaga um mob da tela de todos que o viam.
func (h *Handler) broadcastMobRemove(m *Mob) {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if !ok || !outro.Base.VisibleMobs.Contains(m.ClientID) {
			continue
		}
		outro.Base.VisibleMobs.Remove(m.ClientID)
		if outro.Base.Targets != nil {
			outro.Base.Targets.Remove(m.ClientID)
		}
		h.sendMobRemove(outro, m.ClientID)
	}
}

// broadcastMobSpawn desenha um mob que voltou para quem esta perto.
func (h *Handler) broadcastMobSpawn(m *Mob) {
	if h.Chan == nil {
		return
	}
	ver := h.watchDistance()
	for _, id := range h.Chan.PlayingIDs() {
		outro, _, ok := h.Chan.Lookup(id)
		if !ok || outro.Dead {
			continue
		}
		if !outro.Position.InRange(m.Position, ver) {
			continue
		}
		if outro.Base.VisibleMobs.Contains(m.ClientID) {
			continue
		}
		outro.Base.VisibleMobs.Add(m.ClientID)
		h.sendMobSpawn(outro, m)
	}
}

// IsMobTarget diz se o id vindo do cliente e de mob.
func IsMobTarget(id uint16) bool { return entity.IsMobID(id) }

// HitKeepOne desconta vida de um mob sem matá-lo (RemoveHP com StayOneHP): o
// dano contínuo deixa o mob com 1 de vida no mínimo. Devolve false se o mob já
// estava morto.
func (w *MobWorld) HitKeepOne(m *Mob, dano int, atacante uint16, agora time.Time) bool {
	if w == nil || m == nil || dano <= 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if m.Dead {
		return false
	}
	if !m.Attacked {
		m.Attacked = true
		m.FirstAttackerID = atacante
	}
	m.AttackerID = atacante
	if uint32(dano) >= m.CurHP {
		m.CurHP = 1
	} else {
		m.CurHP -= uint32(dano)
	}
	return true
}
