package itemfn

import (
	"aika/internal/gamedata"
	"aika/internal/model"
)

// Efeitos de set que o servidor entende. Valores de GlobalDefs.pas.
const (
	EFDamage1   = 2
	EFHP        = 13
	EFMP        = 14
	EFCon       = 18
	EFCritical  = 50
	EFCallSkill = 180
)

// MaxCallSkills e o tamanho de EFF_5: ate tres skills de efeito por jogador.
const MaxCallSkills = 3

// SetTables e o que SetEffects precisa alem de Tabelas.
type SetTables interface {
	ConjuntoDe(item int) uint16
	SetItemAt(i int) *gamedata.T_SetItem
}

// SetEffect e um efeito (EF, EFV) que um conjunto completo concede.
type SetEffect struct {
	EF, EFV uint16
}

// SetEffects porta SetConjuntEffect e ConfigEffect de BaseMob.pas.
//
// Cada peca vestida conta para o conjunto Conjuntos[item]. Ao chegar a 3, 4, 5
// e 6 pecas, o Delphi soma os efeitos de SetItem[conjunto] cujo EffSlot e
// igual a essa contagem; como as pecas entram uma a uma, com N pecas valem
// todos os efeitos de EffSlot entre 3 e N.
//
// So os seis primeiros efeitos da linha entram, como no laco "for i := 0 to 5".
func SetEffects(t SetTables, equip []model.Item) []SetEffect {
	if t == nil {
		return nil
	}
	contagem := map[uint16]int{}
	for _, it := range equip {
		if it.IsEmpty() {
			continue
		}
		if c := t.ConjuntoDe(int(it.Index)); c != 0 {
			contagem[c]++
		}
	}
	var out []SetEffect
	for conj, n := range contagem {
		if n < 3 {
			continue
		}
		linha := t.SetItemAt(int(conj))
		if linha == nil {
			continue
		}
		for i := 0; i < len(linha.EF); i++ {
			slot := int(linha.EffSlot[i])
			if linha.EF[i] == 0 || slot < 3 || slot > n {
				continue
			}
			out = append(out, SetEffect{EF: linha.EF[i], EFV: linha.EFV[i]})
		}
	}
	return out
}
