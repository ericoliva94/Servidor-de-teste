// Package nation guarda o estado das tres nacoes: tesouro, taxas, guildas de
// grade (marechal, estrategista, juiz, tesoureiro) e aliança entre nacoes.
// Porte de TNationData (Nation.pas).
package nation

import "sync"

// Numero de nacoes e limites das taxas (UpdateNationTaxes da origem).
const (
	Count = 3

	MinCitizenTax = 5
	MaxCitizenTax = 15
	MinVisitorTax = 10
	MaxVisitorTax = 20
)

// State e uma nacao. Os campos seguem a tabela nations.
type State struct {
	ID   uint8 // 1..3
	Name string
	Rank uint8

	MarshalGuild    uint16
	TacticianGuild  uint16
	JudgeGuild      uint16
	TreasurerGuild  uint16
	CitizenTax      uint16
	VisitorTax      uint16
	Settlement      int32
	AllyNation      int32
	MarshalAllyName string
	AllyDate        uint32
	Gold            int64
}

// Registry guarda as tres nacoes com um mutex proprio.
type Registry struct {
	mu      sync.Mutex
	nations [Count]State
	loaded  [Count]bool
}

// New cria o registro com as nacoes vazias (ID preenchido).
func New() *Registry {
	r := &Registry{}
	for i := range r.nations {
		r.nations[i].ID = uint8(i + 1)
	}
	return r
}

// valid diz se id e uma nacao de verdade (1..3).
func valid(id uint8) bool { return id >= 1 && id <= Count }

// Get devolve uma copia da nacao id; ok e falso para id fora de 1..3.
func (r *Registry) Get(id uint8) (State, bool) {
	if !valid(id) {
		return State{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nations[id-1], true
}

// Put substitui o estado da nacao (carga do banco).
func (r *Registry) Put(st State) {
	if !valid(st.ID) {
		return
	}
	r.mu.Lock()
	r.nations[st.ID-1] = st
	r.loaded[st.ID-1] = true
	r.mu.Unlock()
}

// Update aplica fn sobre a nacao id sob o mutex e devolve a copia nova.
func (r *Registry) Update(id uint8, fn func(*State)) (State, bool) {
	if !valid(id) {
		return State{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.nations[id-1])
	return r.nations[id-1], true
}

// TaxesValid diz se as taxas estao nas faixas aceitas.
func TaxesValid(citizen, visitor uint16) bool {
	return citizen >= MinCitizenTax && citizen <= MaxCitizenTax &&
		visitor >= MinVisitorTax && visitor <= MaxVisitorTax
}

// SetTaxes grava as taxas se estiverem nas faixas; devolve a nacao nova.
func (r *Registry) SetTaxes(id uint8, citizen, visitor uint16) (State, bool) {
	if !valid(id) || !TaxesValid(citizen, visitor) {
		return State{}, false
	}
	return r.Update(id, func(s *State) {
		s.CitizenTax = citizen
		s.VisitorTax = visitor
	})
}
