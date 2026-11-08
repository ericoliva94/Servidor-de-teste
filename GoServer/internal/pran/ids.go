package pran

import "sync"

// Faixa de ClientID das prans no mundo (Servers[].Prans da origem).
const (
	FirstClientID = 10241
	LastClientID  = 11240
)

// IDs reparte os ClientID de pran do canal. A origem dava um id a CADA jogador
// no login (e o guardava no vetor Prans); aqui o id so existe enquanto a pran
// esta no mundo.
type IDs struct {
	mu    sync.Mutex
	owner [LastClientID - FirstClientID + 1]uint16
}

// NewIDs cria o alocador vazio.
func NewIDs() *IDs { return &IDs{} }

// Alloc reserva o primeiro id livre para o dono (ClientID do jogador).
func (r *IDs) Alloc(owner uint16) (uint16, bool) {
	if owner == 0 {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.owner {
		if r.owner[i] == 0 {
			r.owner[i] = owner
			return uint16(FirstClientID + i), true
		}
	}
	return 0, false
}

// Free devolve o id; so o dono dele o libera.
func (r *IDs) Free(id, owner uint16) {
	if id < FirstClientID || id > LastClientID {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner[id-FirstClientID] == owner {
		r.owner[id-FirstClientID] = 0
	}
}

// Owner devolve o dono de um id de pran.
func (r *IDs) Owner(id uint16) (uint16, bool) {
	if id < FirstClientID || id > LastClientID {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.owner[id-FirstClientID]
	return o, o != 0
}
