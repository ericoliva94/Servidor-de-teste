package entity

import "slices"

// Target e uma entrada da lista de alvos visiveis (TMobTarget em
// PlayerData.pas). Player e Mob eram PPlayer/PBaseMob na origem; aqui o dono
// da lista guarda apenas o identificador e consulta o canal quando precisa do
// objeto, o que evita ponteiro pendurado para um slot ja reciclado.
type Target struct {
	ClientID   uint16
	TargetType uint8
	X, Y       float32
}

// InUse diz se o slot esta ocupado. A origem usava ClientID > 0 como marca.
func (t Target) InUse() bool { return t.ClientID > 0 }

// TargetList e a lista de alvos visiveis de uma entidade.
//
// Porte de VisibleTargets/VisibleTargetsCnt do TBaseMob. O original tinha dois
// defeitos que o porte nao repete:
//
//   - AddTargetToList sempre fazia SetLength(VisibleTargets, Length+1) antes de
//     procurar um slot livre, entao o vetor crescia a cada alvo novo mesmo
//     quando havia buraco para reaproveitar: a lista de um mob movimentado
//     crescia sem limite enquanto a sessao durasse.
//   - VisibleTargetsCnt recebia o tamanho do vetor na inclusao mas era
//     decrementado de 1 na remocao, entao contador e tamanho divergiam e
//     nenhum dos dois dizia quantos alvos havia de fato.
//
// Aqui o slot livre e reaproveitado e Len() conta os ocupados.
type TargetList struct {
	slots []Target
	max   int
}

// DefaultMaxTargets limita a lista. A origem nao tinha limite, o que deixava
// a memoria crescer junto com a quantidade de alvos vistos.
const DefaultMaxTargets = 256

// NewTargetList cria a lista com o limite padrao.
func NewTargetList() *TargetList {
	return &TargetList{max: DefaultMaxTargets}
}

// Len conta os slots ocupados.
func (l *TargetList) Len() int {
	if l == nil {
		return 0
	}
	n := 0
	for i := range l.slots {
		if l.slots[i].InUse() {
			n++
		}
	}
	return n
}

// Cap devolve quantos slots existem, ocupados ou nao.
func (l *TargetList) Cap() int {
	if l == nil {
		return 0
	}
	return len(l.slots)
}

// IndexOf acha o slot de um alvo. Equivale a ContainsTargetInList.
func (l *TargetList) IndexOf(clientID uint16) (int, bool) {
	if l == nil || clientID == 0 {
		return 0, false
	}
	for i := range l.slots {
		if l.slots[i].ClientID == clientID {
			return i, true
		}
	}
	return 0, false
}

// Has diz se o alvo esta na lista.
func (l *TargetList) Has(clientID uint16) bool {
	_, ok := l.IndexOf(clientID)
	return ok
}

// Add inclui ou atualiza um alvo e devolve o slot usado.
//
// Reaproveita o primeiro slot livre; so cresce quando nao ha nenhum. Devolve
// ok=false quando o limite foi atingido ou o ClientID e zero (zero e a marca
// de slot livre na origem, entao nao pode ser um alvo valido).
func (l *TargetList) Add(t Target) (int, bool) {
	if l == nil || t.ClientID == 0 {
		return 0, false
	}
	if l.max == 0 {
		l.max = DefaultMaxTargets
	}
	if i, ok := l.IndexOf(t.ClientID); ok {
		l.slots[i] = t
		return i, true
	}
	for i := range l.slots {
		if !l.slots[i].InUse() {
			l.slots[i] = t
			return i, true
		}
	}
	if len(l.slots) >= l.max {
		return 0, false
	}
	l.slots = append(l.slots, t)
	return len(l.slots) - 1, true
}

// Remove libera o slot do alvo. Equivale a RemoveTargetFromList.
func (l *TargetList) Remove(clientID uint16) bool {
	i, ok := l.IndexOf(clientID)
	if !ok {
		return false
	}
	l.slots[i] = Target{}
	return true
}

// At devolve o alvo do slot, ou nil fora da faixa. A origem indexava
// VisibleTargets[id] com o id devolvido por funcoes que podiam nao te-lo
// atribuido.
func (l *TargetList) At(i int) *Target {
	if l == nil || i < 0 || i >= len(l.slots) {
		return nil
	}
	return &l.slots[i]
}

// All devolve os alvos ocupados.
func (l *TargetList) All() []Target {
	if l == nil {
		return nil
	}
	out := make([]Target, 0, len(l.slots))
	for i := range l.slots {
		if l.slots[i].InUse() {
			out = append(out, l.slots[i])
		}
	}
	return out
}

// Clear esvazia a lista.
func (l *TargetList) Clear() {
	if l == nil {
		return
	}
	l.slots = l.slots[:0]
}

// Compact descarta os slots livres do fim, devolvendo memoria depois de uma
// limpeza grande.
func (l *TargetList) Compact() {
	if l == nil {
		return
	}
	last := -1
	for i := range l.slots {
		if l.slots[i].InUse() {
			last = i
		}
	}
	l.slots = l.slots[:last+1]
}

// VisibleSet e uma das listas de visiveis do TBaseMob (TList<WORD> para mobs,
// NPCs e jogadores). Mantem a ordem de inclusao, como a TList da origem, e nao
// aceita duplicata nem o id zero.
type VisibleSet struct {
	ids []uint16
}

// Add inclui o id se ainda nao estiver na lista.
func (s *VisibleSet) Add(id uint16) bool {
	if s == nil || id == 0 || s.Contains(id) {
		return false
	}
	s.ids = append(s.ids, id)
	return true
}

// Remove tira o id da lista.
func (s *VisibleSet) Remove(id uint16) bool {
	if s == nil {
		return false
	}
	i := slices.Index(s.ids, id)
	if i < 0 {
		return false
	}
	s.ids = slices.Delete(s.ids, i, i+1)
	return true
}

// Contains diz se o id esta na lista.
func (s *VisibleSet) Contains(id uint16) bool {
	return s != nil && slices.Contains(s.ids, id)
}

// IDs devolve os ids na ordem de inclusao.
func (s *VisibleSet) IDs() []uint16 {
	if s == nil {
		return nil
	}
	return slices.Clone(s.ids)
}

// Len conta os ids.
func (s *VisibleSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.ids)
}

// Clear esvazia a lista.
func (s *VisibleSet) Clear() {
	if s != nil {
		s.ids = s.ids[:0]
	}
}
