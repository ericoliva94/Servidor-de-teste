package itemfn

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

type fakeSet struct {
	conj  map[int]uint16
	linha gamedata.T_SetItem
}

func (f fakeSet) ConjuntoDe(item int) uint16 { return f.conj[item] }
func (f fakeSet) SetItemAt(i int) *gamedata.T_SetItem {
	if i != 26 {
		return nil
	}
	return &f.linha
}

func novoFake() fakeSet {
	f := fakeSet{conj: map[int]uint16{1: 26, 2: 26, 3: 26, 4: 26, 5: 26, 9: 7}}
	// Espelha o conjunto 26 (Cometa Ardente): 3 pecas HP/MP, 4 pecas
	// critico/CON/dano, 5 pecas a skill de efeito.
	f.linha.EffSlot = [10]uint16{3, 3, 4, 4, 4, 5}
	f.linha.EF = [6]uint16{EFHP, EFMP, EFCritical, EFCon, EFDamage1, EFCallSkill}
	f.linha.EFV = [6]uint16{1570, 780, 8, 80, 172, 8402}
	return f
}

func pecas(ids ...uint16) []model.Item {
	out := make([]model.Item, 16)
	for i, id := range ids {
		out[2+i] = model.Item{Index: id}
	}
	return out
}

func temEF(es []SetEffect, ef uint16) (uint16, bool) {
	for _, e := range es {
		if e.EF == ef {
			return e.EFV, true
		}
	}
	return 0, false
}

func TestSetEffects_PorQuantidadeDePecas(t *testing.T) {
	f := novoFake()
	casos := []struct {
		nome  string
		itens []uint16
		tem   []uint16
		nao   []uint16
	}{
		{"duas pecas nao ativam nada", []uint16{1, 2}, nil,
			[]uint16{EFHP, EFMP, EFCritical, EFCallSkill}},
		{"tres pecas dao HP e MP", []uint16{1, 2, 3},
			[]uint16{EFHP, EFMP}, []uint16{EFCritical, EFCon, EFDamage1, EFCallSkill}},
		{"quatro pecas somam critico, CON e dano", []uint16{1, 2, 3, 4},
			[]uint16{EFHP, EFMP, EFCritical, EFCon, EFDamage1}, []uint16{EFCallSkill}},
		{"cinco pecas liberam a skill de efeito", []uint16{1, 2, 3, 4, 5},
			[]uint16{EFHP, EFMP, EFCritical, EFCon, EFDamage1, EFCallSkill}, nil},
		{"pecas de outro conjunto nao contam", []uint16{1, 2, 9, 9}, nil,
			[]uint16{EFHP, EFCallSkill}},
	}
	for _, c := range casos {
		es := SetEffects(f, pecas(c.itens...))
		for _, ef := range c.tem {
			if _, ok := temEF(es, ef); !ok {
				t.Errorf("%s: faltou o efeito %d", c.nome, ef)
			}
		}
		for _, ef := range c.nao {
			if _, ok := temEF(es, ef); ok {
				t.Errorf("%s: efeito %d nao devia estar ativo", c.nome, ef)
			}
		}
	}
}

func TestSetEffects_ValorDaSkillDeEfeito(t *testing.T) {
	es := SetEffects(novoFake(), pecas(1, 2, 3, 4, 5))
	if v, ok := temEF(es, EFCallSkill); !ok || v != 8402 {
		t.Fatalf("EF_CALLSKILL = %d (achou=%v), queria 8402", v, ok)
	}
}

func TestSetEffects_SemTabelaNaoPanica(t *testing.T) {
	if es := SetEffects(nil, pecas(1, 2, 3)); len(es) != 0 {
		t.Fatalf("sem tabela devia devolver nada, veio %v", es)
	}
}
