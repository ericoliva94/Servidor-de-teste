package game

import (
	"fmt"

	"aika/internal/dropdata"
	"aika/internal/itemfn"
	"aika/internal/mobdata"
)

// MutantTest e o MODO DE TESTE dos mutantes do nivel 60. Desligado por padrao;
// quem liga e a flag -teste-mutantes do main. Voltar ao normal e reiniciar sem
// a flag: nada aqui e gravado no banco nem nos arquivos de dados.
//
// O que faz, so para as especies-alvo:
//   - todo mob morto deixa um acessorio (um dos acessorios que existem nas
//     listas de drop dos mutantes do nivel 60), em vez do sorteio de raridade;
//   - a defesa fisica e a magica dessas especies passam a ter um teto, porque
//     os arquivos trazem 10.000 a 15.000 (contra 20 a 700 dos mobs normais do
//     mesmo nivel) e, como o dano e ataque - defesa/8, o golpe do jogador cai
//     para 1.
type MutantTest struct {
	// Species e o conjunto de indices de especie afetados.
	Species map[int]bool
	// Pool sao os acessorios sorteados.
	Pool []uint16
	// Report descreve o que foi ligado, para o log de inicializacao.
	Report []string
}

// Nomes (como nos CSV) dos mutantes afetados: Mutated Cannibal Plant, Mutated
// Croshu (as duas versoes) e Mutated Angler (Buto Mutante, a leitura mais
// provavel; ver KNOWN_ISSUES.md).
var mutantTestTargets = []string{
	"Planta_Carnivora_Mutante",
	"Cao_Mutante_Croshu",
	"Croshu_Mutante_das_Cinzas",
	"Buto_Mutante",
}

// Os seis mutantes do nivel 60; so as suas listas de drop alimentam o sorteio.
var cap60Mutants = []string{
	"Planta_Carnivora_Mutante", "Cao_Mutante_Croshu", "Buto_Mutante",
	"Croshu_Mutante_das_Cinzas", "Penza_Poderosa_Mutante", "Verit_Mutante",
}

// Slots de equipamento dos acessorios (anel, brinco, bracelete, colar).
const (
	accessorySlotMin = 11
	accessorySlotMax = 14
)

// NewMutantTest liga o modo de teste: reduz a defesa das especies-alvo na
// tabela de mobs carregada (em memoria) e monta o sorteio de acessorios.
// maxDef e o teto da defesa fisica e da magica.
func NewMutantTest(mobs *mobdata.Table, drops *dropdata.Table, items itemfn.Tabelas, maxDef uint32) *MutantTest {
	mt := &MutantTest{Species: map[int]bool{}}
	if mobs == nil {
		return mt
	}
	byName := map[string]int{}
	for i := range mobs.Species {
		byName[mobs.Species[i].Name] = i
	}

	for _, nome := range mutantTestTargets {
		i, ok := byName[nome]
		if !ok {
			mt.Report = append(mt.Report, fmt.Sprintf("especie %q nao encontrada", nome))
			continue
		}
		sp := &mobs.Species[i]
		mt.Species[i] = true
		antesF, antesM := sp.DefFis, sp.DefMag
		sp.DefFis, sp.DefMag = min(sp.DefFis, maxDef), min(sp.DefMag, maxDef)
		mt.Report = append(mt.Report, fmt.Sprintf("%s (especie %d): defesa fisica %d->%d, magica %d->%d",
			nome, i, antesF, sp.DefFis, antesM, sp.DefMag))
	}

	seenCat := map[int]bool{}
	seenItem := map[uint16]bool{}
	for _, nome := range cap60Mutants {
		i, ok := byName[nome]
		if !ok {
			continue
		}
		cat := int(mobs.Species[i].DropIndex)
		if seenCat[cat] || drops == nil {
			continue
		}
		seenCat[cat] = true
		c := drops.Category(cat)
		for r := dropdata.Normal; r <= dropdata.Legendary; r++ {
			for _, it := range c.Of(r) {
				if es := itemfn.EquipSlot(items, it); es >= accessorySlotMin && es <= accessorySlotMax && !seenItem[it] {
					seenItem[it] = true
					mt.Pool = append(mt.Pool, it)
				}
			}
		}
	}
	mt.Report = append(mt.Report, fmt.Sprintf("acessorios no sorteio: %v", mt.Pool))
	return mt
}

// mutantTestDrop devolve o acessorio forcado para o mob, se ele for um alvo.
func (h *Handler) mutantTestDrop(m *Mob) (uint16, bool) {
	mt := h.MutantTest
	if mt == nil || len(mt.Pool) == 0 || !mt.Species[m.Species] {
		return 0, false
	}
	return mt.Pool[h.rand().IntN(len(mt.Pool))], true
}
