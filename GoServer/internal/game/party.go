package game

import (
	"errors"
	"sync"
)

// MaxPartyMembers e o limite de 6 do original ("if (Members.Count = 6)").
const MaxPartyMembers = 6

// MaxParties e o tamanho do vetor "Parties: ARRAY [1..30] OF TParty" do
// original: 30 grupos por canal.
//
// Aqui o numero e um limite, nao um vetor: grupo nasce e morre num mapa, e
// quando os 30 estao ocupados o pedido e recusado com mensagem em vez de
// falhar em silencio como no original, cujo laco de busca simplesmente nao
// achava slot livre e devolvia False sem avisar ninguem.
const MaxParties = 30

// Modos de divisao de experiencia e de item, os valores padrao de
// CreateParty.
const (
	AllocateDefault = 1

	// Divisao de exp (Party.ExpAlocate).
	ExpEqual      = 1 // igualmente
	ExpIndividual = 2 // so quem bateu primeiro

	// Divisao de item (Party.ItemAlocate).
	ItemInOrder    = 1 // em ordem
	ItemRandom     = 2 // aleatorio (decide o lider)
	ItemIndividual = 3 // de quem bateu primeiro
	ItemLeader     = 4 // do lider
)

// Erros do grupo.
var (
	ErrNoParty        = errors.New("game: jogador nao esta em grupo")
	ErrPartyFull      = errors.New("game: grupo cheio")
	ErrNoFreeParty    = errors.New("game: nao ha grupo livre no canal")
	ErrAlreadyInParty = errors.New("game: jogador ja esta em um grupo")
	ErrNotPartyLeader = errors.New("game: so o lider pode fazer isso")
)

// Party e um grupo. Porte de TParty (PartyData.pas), sem a parte de raid, que
// depende de sistemas ainda nao portados.
type Party struct {
	Index  uint16
	Leader uint16
	// Members esta na ordem de entrada; o primeiro a entrar e quem criou.
	Members []uint16

	ExpAllocate  uint8
	ItemAllocate uint8
	// LastItemSlot e o LastSlotItemReceived: de quem e a vez no modo "em ordem".
	LastItemSlot uint8
}

// Has diz se o ClientID esta no grupo.
func (p *Party) Has(clientID uint16) bool {
	if p == nil {
		return false
	}
	for _, id := range p.Members {
		if id == clientID {
			return true
		}
	}
	return false
}

// Size conta os membros.
func (p *Party) Size() int {
	if p == nil {
		return 0
	}
	return len(p.Members)
}

// PartyRegistry guarda os grupos de um canal.
//
// Substitui o vetor "Parties: ARRAY [1..30]" e o ponteiro
// "Players[i].Party: PParty" que o original guardava dentro do registro do
// jogador: aquele ponteiro apontava para dentro de um vetor global que podia
// ser zerado por outro caminho (DestroyParty fazia ZeroMemory no registro) e
// seguia sendo lido depois, inclusive em RefreshParty, que faz
// "Self.Party.Members" sem checar nil.
type PartyRegistry struct {
	mu       sync.Mutex
	parties  map[uint16]*Party
	byMember map[uint16]uint16 // ClientID -> indice do grupo
	raids    map[uint16]*Raid
	nextHint uint16
	max      uint16
}

// NewPartyRegistry cria o registro.
func NewPartyRegistry() *PartyRegistry {
	return &PartyRegistry{
		parties:  make(map[uint16]*Party, 8),
		byMember: make(map[uint16]uint16, 32),
		nextHint: 1,
		max:      MaxParties,
	}
}

// Count conta os grupos ativos.
func (r *PartyRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parties)
}

// Of devolve o grupo de um jogador, ou nil.
func (r *PartyRegistry) Of(clientID uint16) *Party {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	idx, ok := r.byMember[clientID]
	if !ok {
		return nil
	}
	return r.parties[idx]
}

// Get devolve o grupo de um indice.
func (r *PartyRegistry) Get(index uint16) *Party {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.parties[index]
}

// Create cria um grupo com um membro so. Porte de TParty.CreateParty.
func (r *PartyRegistry) Create(leader uint16) (*Party, error) {
	if r == nil {
		return nil, ErrNoFreeParty
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byMember[leader]; ok {
		return nil, ErrAlreadyInParty
	}
	idx, ok := r.freeIndexLocked()
	if !ok {
		return nil, ErrNoFreeParty
	}
	p := &Party{
		Index:        idx,
		Leader:       leader,
		Members:      []uint16{leader},
		ExpAllocate:  AllocateDefault,
		ItemAllocate: AllocateDefault,
	}
	r.parties[idx] = p
	r.byMember[leader] = idx
	return p, nil
}

// freeIndexLocked acha o primeiro indice livre a partir da ultima alocacao.
func (r *PartyRegistry) freeIndexLocked() (uint16, bool) {
	if len(r.parties) >= int(r.max) {
		return 0, false
	}
	start := r.nextHint
	if start < 1 || start > r.max {
		start = 1
	}
	for i := uint16(0); i < r.max; i++ {
		idx := start + i
		if idx > r.max {
			idx -= r.max
			if idx < 1 {
				idx = 1
			}
		}
		if _, usado := r.parties[idx]; !usado {
			r.nextHint = idx + 1
			return idx, true
		}
	}
	return 0, false
}

// Add poe um membro no grupo. Porte de TParty.AddMember.
func (r *PartyRegistry) Add(p *Party, clientID uint16) error {
	if r == nil || p == nil {
		return ErrNoParty
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.parties[p.Index]; !ok {
		// O grupo foi desfeito entre o convite e o aceite.
		return ErrNoParty
	}
	if _, ok := r.byMember[clientID]; ok {
		return ErrAlreadyInParty
	}
	if len(p.Members) >= MaxPartyMembers {
		return ErrPartyFull
	}
	p.Members = append(p.Members, clientID)
	r.byMember[clientID] = p.Index
	return nil
}

// Remove tira um membro. Devolve o grupo (ou nil se ele foi desfeito) e a
// lista de quem ficou sem grupo.
//
// Porte de TParty.RemoveMember: com dois membros ou menos o grupo e desfeito,
// e se quem saiu era o lider a lideranca passa ao primeiro da lista.
func (r *PartyRegistry) Remove(clientID uint16) (restante *Party, soltos []uint16) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	idx, ok := r.byMember[clientID]
	if !ok {
		return nil, nil
	}
	p := r.parties[idx]
	if p == nil {
		delete(r.byMember, clientID)
		return nil, []uint16{clientID}
	}

	// Grupo de dois vira grupo de um, que nao e grupo: desfaz.
	if len(p.Members) <= 2 {
		return nil, r.destroyLocked(p)
	}

	novos := make([]uint16, 0, len(p.Members)-1)
	for _, id := range p.Members {
		if id != clientID {
			novos = append(novos, id)
		}
	}
	p.Members = novos
	delete(r.byMember, clientID)

	if p.Leader == clientID {
		p.Leader = p.Members[0]
	}
	return p, []uint16{clientID}
}

// Destroy desfaz o grupo e devolve quem ficou sem grupo.
func (r *PartyRegistry) Destroy(p *Party) []uint16 {
	if r == nil || p == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.destroyLocked(p)
}

func (r *PartyRegistry) destroyLocked(p *Party) []uint16 {
	if _, ok := r.parties[p.Index]; !ok {
		return nil
	}
	r.leaveRaidLocked(p.Index)
	soltos := make([]uint16, len(p.Members))
	copy(soltos, p.Members)
	for _, id := range p.Members {
		if cur, ok := r.byMember[id]; ok && cur == p.Index {
			delete(r.byMember, id)
		}
	}
	p.Members = nil
	delete(r.parties, p.Index)
	return soltos
}

// SetLeader passa a lideranca a um membro.
func (r *PartyRegistry) SetLeader(p *Party, clientID uint16) error {
	if r == nil || p == nil {
		return ErrNoParty
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !p.Has(clientID) {
		return ErrNoParty
	}
	p.Leader = clientID
	return nil
}

// MembersOf devolve uma copia da lista de membros, segura para iterar.
func (r *PartyRegistry) MembersOf(p *Party) []uint16 {
	if r == nil || p == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]uint16, len(p.Members))
	copy(out, p.Members)
	return out
}
