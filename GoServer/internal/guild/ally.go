package guild

import "errors"

// AllySlots e o tamanho da aliança: a guilda lider no slot 0 e ate 3 aliadas.
const AllySlots = 4

// AllyNameLen e o tamanho do nome de uma aliada no pacote (18 bytes com o nulo).
const AllyNameLen = 18

// Erros da aliança.
var (
	ErrNotAllyLeader = errors.New("guild: a guilda nao lidera a alianca")
	ErrAlreadyAllied = errors.New("guild: a guilda ja esta em uma alianca")
	ErrAllyFull      = errors.New("guild: alianca sem espaco")
	ErrAllyNation    = errors.New("guild: guildas de nacoes diferentes")
	ErrAllySelf      = errors.New("guild: a guilda nao pode se aliar a si mesma")
)

// AllyItem e uma guilda da aliança. Porte de TGuildAllyItem.
type AllyItem struct {
	Index uint16
	Name  string
}

// Ally e a aliança de uma guilda. Porte de TGuildAlly: cada guilda guarda uma
// copia da lista inteira, e o slot 0 e a guilda lider.
type Ally struct {
	Leader uint16
	Guilds [AllySlots]AllyItem
}

// resetAlly poe a guilda sozinha na propria aliança (como TGuild.Create).
func (g *Guild) resetAlly() {
	g.Ally = Ally{Leader: g.Index}
	g.Ally.Guilds[0] = AllyItem{Index: g.Index, Name: truncate(g.Name, AllyNameLen-1)}
}

// EnsureAlly arruma a aliança de uma guilda que veio do banco sem ela.
func (g *Guild) EnsureAlly() {
	if g.Ally.Leader == 0 && g.Ally.Guilds[0].Index == 0 {
		g.resetAlly()
	}
}

// AllyCount conta as guildas da aliança (a propria inclusa).
func (g *Guild) AllyCount() int {
	n := 0
	for _, it := range g.Ally.Guilds {
		if it.Index != 0 {
			n++
		}
	}
	return n
}

// InAlliance diz se a guilda esta numa aliança com outra: os slots 1..3 so
// ficam preenchidos quando ha aliada (a origem testava o mesmo).
func (g *Guild) InAlliance() bool {
	for _, it := range g.Ally.Guilds[1:] {
		if it.Index != 0 {
			return true
		}
	}
	return false
}

// FreeAllySlot devolve o primeiro slot livre de 1 a 3, ou -1.
func (g *Guild) FreeAllySlot() int {
	for i := 1; i < AllySlots; i++ {
		if g.Ally.Guilds[i].Index == 0 {
			return i
		}
	}
	return -1
}

// CanAddAlly confere as regras de entrada de joiner na aliança de lead.
func CanAddAlly(lead, joiner *Guild) error {
	switch {
	case lead == nil || joiner == nil:
		return ErrNoGuild
	case lead.Index == joiner.Index:
		return ErrAllySelf
	case lead.Ally.Leader != lead.Index:
		return ErrNotAllyLeader
	case joiner.InAlliance():
		return ErrAlreadyAllied
	case lead.Nation != joiner.Nation:
		return ErrAllyNation
	case lead.FreeAllySlot() < 0:
		return ErrAllyFull
	}
	return nil
}

// JoinAlliance poe joiner na aliança de lead e copia a lista nova para todas
// as guildas dela. Devolve as guildas que mudaram (lead, joiner e as outras).
//
// Corrigido: a origem usava EmptySlot=255 quando nao havia vaga e escrevia
// fora do vetor; e conferia a vaga com Guilds[I].Ally.Guilds[I], um indice
// misturado. A vaga e conferida de verdade, no mesmo trava da entrada.
func (r *Registry) JoinAlliance(lead, joiner *Guild) ([]*Guild, error) {
	if r == nil {
		return nil, ErrNoGuild
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	lead.EnsureAlly()
	joiner.EnsureAlly()
	if err := CanAddAlly(lead, joiner); err != nil {
		return nil, err
	}
	slot := lead.FreeAllySlot()
	lead.Ally.Guilds[slot] = AllyItem{Index: joiner.Index, Name: truncate(joiner.Name, AllyNameLen-1)}

	var changed []*Guild
	for _, it := range lead.Ally.Guilds {
		if it.Index == 0 {
			continue
		}
		g := r.byIndex[it.Index]
		if g == nil {
			continue
		}
		g.Ally = lead.Ally
		changed = append(changed, g)
	}
	return changed, nil
}

// leaveAllianceLocked tira g das aliancas onde estava. Se g era a lider, a
// aliança acaba e cada aliada volta a ficar sozinha; senao so o slot e limpo
// nas copias. A origem deixava o indice morto na lista.
func (r *Registry) leaveAllianceLocked(g *Guild) {
	lider := g.Ally.Leader == g.Index
	for _, it := range g.Ally.Guilds {
		if it.Index == 0 || it.Index == g.Index {
			continue
		}
		o := r.byIndex[it.Index]
		if o == nil {
			continue
		}
		if lider {
			o.resetAlly()
			continue
		}
		for i := range o.Ally.Guilds {
			if o.Ally.Guilds[i].Index == g.Index {
				o.Ally.Guilds[i] = AllyItem{}
			}
		}
	}
	g.resetAlly()
}
