// Package dungeon cuida das instancias de dungeon: o saguao onde o grupo
// confirma, a instancia com os mobs daquele grupo, e o fechamento.
//
// Porte de Dungeon.pas (TDungeonInstance, TDungeonMainThread) e das partes de
// dungeon de Player.pas (CreateDungeonInstance, GetFreeDungeonInstance) e
// PacketHandlers.pas (RequestEnterDungeon, DungeonLobbyConfirm).
//
// Diferencas de fundo em relacao a origem:
//
//   - "DungeonInstances: Array [1 .. 1] of TDungeonInstance"
//     (GlobalDefs.pas:18): o servidor inteiro tinha UMA instancia. O segundo
//     grupo a entrar recebia GetFreeDungeonInstance = 255, mas so DEPOIS de
//     ja ter sido teleportado para dentro e marcado InDungeon — ficava preso
//     num mapa sem mob nenhum, com DungeonInstanceID = 0, que e um indice
//     fora do vetor [1..1] e era usado em todo acesso seguinte. Aqui cada
//     grupo tem a sua instancia, num mapa, e a entrada so acontece depois de
//     a instancia existir.
//
//   - A instancia guardava os mobs em "MOBS: Array [1 .. 450]" e enchia com
//     um contador sem checagem. Aqui os mobs ficam num slice e num mapa por
//     id de cliente.
//
//   - Cada instancia abria uma TThread propria que lia
//     DungeonInstances[i].Party uma vez e usava o ponteiro pela vida toda,
//     enquanto o fechamento punha Party := nil e o grupo podia ser desfeito:
//     a thread seguia lendo memoria liberada. Aqui a instancia guarda os
//     CharIndex dos membros e e varrida por um tick unico.
package dungeon

import (
	"errors"
	"sync"
	"time"

	"aika/internal/dungeondata"
	"aika/internal/model"
)

// LobbyTimeout e quanto o saguao espera por todas as confirmacoes. A origem
// nao tinha prazo nenhum: um membro que nunca respondesse deixava o grupo
// preso no saguao para sempre.
const LobbyTimeout = 60 * time.Second

// MaxLifetime e o prazo da instancia. A origem guardava CreateTime e nunca
// olhava: a instancia ficava aberta ate alguem sair.
const MaxLifetime = 2 * time.Hour

// ExitPosition e o (3399, 564) para onde a origem teleportava quem morria
// sozinho na dungeon.
var ExitPosition = model.Position{X: 3399, Y: 564}

// Raios de agressao do mob de dungeon. A origem usava 7 quando o grupo tinha
// mais de um membro e 35 quando tinha so um (Dungeon.pas:535 e 614), e 15 no
// proprio handler do mob (linha 736).
const (
	AggroRangeParty = 7
	AggroRangeSolo  = 35
	AggroRangeTick  = 15
)

// LeashRange e a distancia do ponto de origem em que o mob desiste e volta. A
// origem usava 20 na varredura e 15 no handler, com o mesmo efeito.
const LeashRange = 20

// AttackRange e a distancia em que o mob da dungeon bate.
const AttackRange = 5

// AttackInterval e o "SecondsBetween(Now, LastMyAttack) >= 2" da origem.
const AttackInterval = 2 * time.Second

var (
	// ErrNoDungeon diz que a dungeon ou a dificuldade nao existe.
	ErrNoDungeon = errors.New("dungeon: dungeon ou dificuldade inexistente")
	// ErrLevel diz que alguem nao tem o nivel minimo.
	ErrLevel = errors.New("dungeon: nivel minimo")
	// ErrBusy diz que o personagem ja esta numa instancia.
	ErrBusy = errors.New("dungeon: ja esta numa dungeon")
)

// Mob e um mob dentro de uma instancia.
type Mob struct {
	// ClientID e o id que vai nos pacotes, vindo do MobsPosition.
	ClientID uint16
	// Slot e a posicao do mob na instancia, o Base.mobid da origem.
	Slot int
	// Species aponta para a especie na tabela: nao e copiada.
	Species *dungeondata.Species

	MaxHP uint32
	CurHP uint32

	Pos     model.Position
	OrigPos model.Position

	Dead     bool
	DeadAt   time.Time
	LastMove time.Time

	// AttackerCharIndex e quem o mob persegue, 0 para ninguem.
	AttackerCharIndex uint32
	// FirstAttacker e quem bateu primeiro, para a divisao individual de exp.
	FirstAttacker uint32
	LastAttack    time.Time
}

// Alive diz se o mob esta vivo.
func (m *Mob) Alive() bool { return m != nil && !m.Dead }

// Instance e uma dungeon aberta para um grupo.
type Instance struct {
	mu sync.Mutex

	ID  uint32
	Def *dungeondata.Definition

	// PartyIndex e 0 quando o jogador entrou sozinho.
	PartyIndex uint16
	// Members sao os CharIndex de quem esta dentro.
	Members []uint32

	CreatedAt time.Time

	mobs    []*Mob
	porID   map[uint16]*Mob
	vivos   int
	fechada bool
}

// MobCount e quantos mobs a instancia tem.
func (in *Instance) MobCount() int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.mobs)
}

// AliveCount e quantos mobs ainda estao vivos.
func (in *Instance) AliveCount() int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.vivos
}

// Closed diz se a instancia ja fechou.
func (in *Instance) Closed() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.fechada
}

// Cleared diz se todos os mobs morreram.
func (in *Instance) Cleared() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.mobs) > 0 && in.vivos == 0
}

// Expired diz se a instancia passou do prazo.
func (in *Instance) Expired(agora time.Time) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return agora.Sub(in.CreatedAt) >= MaxLifetime
}

// Mob acha o mob pelo id de cliente. A origem varria o vetor inteiro com
// GetMobDgGeralID e devolvia -1 quando nao achava; num dos usos o -1 voltava
// como indice do vetor (MOB.pas:1215).
func (in *Instance) Mob(clientID uint16) (*Mob, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	m, ok := in.porID[clientID]
	return m, ok
}

// Mobs devolve uma copia da lista de ponteiros, para varrer sem a trava.
func (in *Instance) Mobs() []*Mob {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]*Mob, len(in.mobs))
	copy(out, in.mobs)
	return out
}

// HasMember diz se o personagem esta nesta instancia.
func (in *Instance) HasMember(charIndex uint32) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	for _, c := range in.Members {
		if c == charIndex {
			return true
		}
	}
	return false
}

// MemberList devolve uma copia dos membros.
func (in *Instance) MemberList() []uint32 {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]uint32, len(in.Members))
	copy(out, in.Members)
	return out
}

// RemoveMember tira um membro e diz se a instancia ficou vazia.
func (in *Instance) RemoveMember(charIndex uint32) (vazia bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	for i, c := range in.Members {
		if c == charIndex {
			in.Members = append(in.Members[:i], in.Members[i+1:]...)
			break
		}
	}
	return len(in.Members) == 0
}

// Hit desconta dano de um mob e diz se foi este golpe que o matou. A origem
// fazia "Dec(CurrentHP, Dano)" com uma thread por jogador, sem trava: dois
// golpes simultaneos matavam o mesmo mob duas vezes e davam exp duas vezes.
func (in *Instance) Hit(m *Mob, dano uint32, atacante uint32, agora time.Time) (matou bool) {
	if m == nil {
		return false
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if m.Dead {
		return false
	}
	if m.FirstAttacker == 0 {
		m.FirstAttacker = atacante
	}
	m.AttackerCharIndex = atacante
	if dano >= m.CurHP {
		m.CurHP = 0
		m.Dead = true
		m.DeadAt = agora
		m.AttackerCharIndex = 0
		if in.vivos > 0 {
			in.vivos--
		}
		return true
	}
	m.CurHP -= dano
	return false
}

// SetTarget marca o alvo do mob sob a trava.
func (in *Instance) SetTarget(m *Mob, charIndex uint32, agora time.Time) {
	if m == nil {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if m.Dead {
		return
	}
	if m.AttackerCharIndex == 0 && charIndex != 0 {
		m.OrigPos = m.Pos
	}
	m.AttackerCharIndex = charIndex
	if charIndex != 0 {
		m.LastAttack = agora
	}
}

// MoveMob guarda a posicao nova do mob sob a trava.
func (in *Instance) MoveMob(m *Mob, p model.Position, agora time.Time) {
	if m == nil {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	m.Pos = p
	m.LastMove = agora
}

// MarkAttack guarda a hora do golpe e diz se o mob podia bater agora.
func (in *Instance) MarkAttack(m *Mob, agora time.Time) bool {
	if m == nil {
		return false
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if m.Dead || agora.Sub(m.LastAttack) < AttackInterval {
		return false
	}
	m.LastAttack = agora
	return true
}

// Registry guarda as instancias abertas.
type Registry struct {
	mu        sync.Mutex
	porID     map[uint32]*Instance
	porChar   map[uint32]*Instance
	proximoID uint32
}

// NewRegistry cria o registro vazio.
func NewRegistry() *Registry {
	return &Registry{
		porID:   map[uint32]*Instance{},
		porChar: map[uint32]*Instance{},
	}
}

// Count e quantas instancias estao abertas.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.porID)
}

// Open cria a instancia e marca os membros. Devolve ErrBusy se algum membro
// ja estiver numa dungeon.
func (r *Registry) Open(def *dungeondata.Definition, partyIndex uint16,
	membros []uint32, agora time.Time) (*Instance, error) {
	if def == nil {
		return nil, ErrNoDungeon
	}
	if len(membros) == 0 {
		return nil, errors.New("dungeon: instancia sem membro")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range membros {
		if _, ja := r.porChar[c]; ja {
			return nil, ErrBusy
		}
	}
	r.proximoID++
	in := &Instance{
		ID:         r.proximoID,
		Def:        def,
		PartyIndex: partyIndex,
		Members:    append([]uint32(nil), membros...),
		CreatedAt:  agora,
		porID:      map[uint16]*Mob{},
	}
	povoar(in, def, agora)
	r.porID[in.ID] = in
	for _, c := range membros {
		r.porChar[c] = in
	}
	return in, nil
}

// povoar cria os mobs da instancia a partir da tabela.
func povoar(in *Instance, def *dungeondata.Definition, agora time.Time) {
	slot := 1 // a origem comecava em 1 porque MOBS era Array [1..450]
	for i := range def.Species {
		sp := &def.Species[i]
		for j := range sp.Spawns {
			pt := &sp.Spawns[j]
			hp := pt.HP
			if hp == 0 {
				hp = sp.HP
			}
			m := &Mob{
				ClientID: pt.ClientID,
				Slot:     slot,
				Species:  sp,
				MaxHP:    hp,
				CurHP:    hp,
				Pos:      pt.InitPos,
				OrigPos:  pt.InitPos,
				LastMove: agora,
			}
			slot++
			// Dois pontos com o mesmo id de cliente nao podem coexistir: o
			// cliente nao saberia em qual bater. A origem nao conferia e
			// GetMobDgGeralID devolvia o primeiro.
			if _, repetido := in.porID[m.ClientID]; repetido {
				continue
			}
			in.mobs = append(in.mobs, m)
			in.porID[m.ClientID] = m
			in.vivos++
		}
	}
}

// Of acha a instancia de um personagem.
func (r *Registry) Of(charIndex uint32) (*Instance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.porChar[charIndex]
	return in, ok
}

// Get acha a instancia pelo id.
func (r *Registry) Get(id uint32) (*Instance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.porID[id]
	return in, ok
}

// Leave tira um personagem da instancia e fecha a instancia se ela ficou
// vazia. Devolve a instancia de onde ele saiu e se ela fechou.
func (r *Registry) Leave(charIndex uint32) (*Instance, bool) {
	r.mu.Lock()
	in, ok := r.porChar[charIndex]
	if !ok {
		r.mu.Unlock()
		return nil, false
	}
	delete(r.porChar, charIndex)
	r.mu.Unlock()

	vazia := in.RemoveMember(charIndex)
	if !vazia {
		return in, false
	}
	r.close(in)
	return in, true
}

// Close fecha a instancia e solta todos os membros.
func (r *Registry) Close(in *Instance) []uint32 {
	if in == nil {
		return nil
	}
	membros := in.MemberList()
	r.mu.Lock()
	for _, c := range membros {
		if r.porChar[c] == in {
			delete(r.porChar, c)
		}
	}
	r.mu.Unlock()

	in.mu.Lock()
	in.Members = nil
	in.mu.Unlock()
	r.close(in)
	return membros
}

func (r *Registry) close(in *Instance) {
	in.mu.Lock()
	in.fechada = true
	in.mu.Unlock()
	r.mu.Lock()
	delete(r.porID, in.ID)
	r.mu.Unlock()
}

// All devolve uma copia das instancias abertas.
func (r *Registry) All() []*Instance {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Instance, 0, len(r.porID))
	for _, in := range r.porID {
		out = append(out, in)
	}
	return out
}
