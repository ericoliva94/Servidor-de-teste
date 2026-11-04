package gamedata

import (
	"errors"
	"path/filepath"
)

// Capacidades das tabelas de tamanho fixo, iguais as de FilesData.pas.
const (
	MaxItems    = 20478 // TItemList  = ARRAY [0 .. 20477]
	MaxSkills   = 11999 // TSkillData = ARRAY [0 .. 11998]
	MaxSetItems = 801   // TSetItem   = ARRAY [0 .. 800]
	MaxConjunts = 20478 // TConjunts  = ARRAY [0 .. 20477]
)

// Tables reune as tabelas estaticas que o servidor carrega no inicio.
// Equivale as variaveis globais do bloco "Files defs" de GlobalDefs.pas.
type Tables struct {
	Items        []ItemFromList // ItemList
	Skills       []T_SkillData  // SkillData
	SetItems     []T_SetItem    // SetItem
	Conjuntos    []uint16       // Conjuntos
	Exp          []uint64       // ExpList
	PranExp      []uint32       // PranExpList
	Quests       []QuestFromList
	Recipes      []RecipeData
	Premium      []PremiumItem
	MobPos       []MobPos
	ScrollPos    []ScrollTeleportPos
	Reinforce2   []ItemAttributeReinforce
	Reinforce3   []ArmorAttributeReinforce
	ReinforceW01 []ItemChanceReinforce // armas
	ReinforceA01 []ItemChanceReinforce // armaduras
	NPCOptions   NPCFileOptions
	Maps         FileMapsData

	// Loaded lista o que foi lido, para o log de inicializacao.
	Loaded []Table
	// Missing lista os arquivos ausentes. O Delphi registrava aviso e seguia,
	// entao isso nao e erro fatal por si.
	Missing []string
}

// Load le todas as tabelas de dir (a pasta Data do servidor).
//
// Arquivo ausente entra em Missing e nao interrompe a carga, como em Load.pas.
// Qualquer outro erro (arquivo ilegivel, registro truncado no meio) e
// devolvido, porque ai o dado esta corrompido e seguir em frente esconderia o
// problema.
func Load(dir string) (*Tables, error) {
	t := &Tables{}
	var firstErr error

	// note registra o resultado de cada arquivo.
	note := func(tab Table, err error) bool {
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				t.Missing = append(t.Missing, filepath.Base(tab.Path))
				return false
			}
			if firstErr == nil {
				firstErr = err
			}
			return false
		}
		t.Loaded = append(t.Loaded, tab)
		return true
	}

	p := func(name string) string { return filepath.Join(dir, name) }

	// Tabelas de tamanho fixo no Delphi: o arquivo pode ter mais registros do
	// que a tabela comporta (ItemList.bin tem 26000, a tabela cabe 20478) e o
	// excedente nao e lido, igual ao Read() de arquivo tipado.
	t.Items = make([]ItemFromList, MaxItems)
	if tab, err := LoadArray(p("ItemList.bin"), t.Items); !note(tab, err) {
		t.Items = nil
	}
	t.Skills = make([]T_SkillData, MaxSkills)
	if tab, err := LoadArray(p("SkillData.bin"), t.Skills); !note(tab, err) {
		t.Skills = nil
	}
	t.SetItems = make([]T_SetItem, MaxSetItems)
	if tab, err := LoadArray(p("SetItem.bin"), t.SetItems); !note(tab, err) {
		t.SetItems = nil
	}
	t.Conjuntos = make([]uint16, MaxConjunts)
	if tab, err := LoadArray(p("Conjunts.bin"), t.Conjuntos); !note(tab, err) {
		t.Conjuntos = nil
	}

	// Tabelas de tamanho variavel (array of ... no Delphi).
	loadSlice(p("ExpList.bin"), &t.Exp, note)
	loadSlice(p("PranExpList.bin"), &t.PranExp, note)
	loadSlice(p("Quest.bin"), &t.Quests, note)
	loadSlice(p("Recipes.bin"), &t.Recipes, note)
	loadSlice(p("PI.bin"), &t.Premium, note)
	loadSlice(p("MobPos.bin"), &t.MobPos, note)
	loadSlice(p("ScrollPos.bin"), &t.ScrollPos, note)
	loadSlice(p("Reinforce2.bin"), &t.Reinforce2, note)
	loadSlice(p("Reinforce3.bin"), &t.Reinforce3, note)
	loadSlice(p("ReinforceW01.bin"), &t.ReinforceW01, note)
	loadSlice(p("ReinforceA01.bin"), &t.ReinforceA01, note)

	// Registros unicos.
	if tab, err := LoadRecord(p("NPCOptionsText.bin"), &t.NPCOptions); !note(tab, err) {
		t.NPCOptions = NPCFileOptions{}
	}
	if tab, err := LoadRecord(p("Map.bin"), &t.Maps); !note(tab, err) {
		t.Maps = FileMapsData{}
	}

	return t, firstErr
}

// loadSlice e o atalho generico para as tabelas de tamanho variavel.
func loadSlice[T any](path string, dst *[]T, note func(Table, error) bool) {
	v, tab, err := LoadSlice[T](path)
	if note(tab, err) {
		*dst = v
	}
}

// Item devolve o item de indice i, ou nil quando i esta fora da tabela.
//
// Todo acesso a ItemList no Delphi era ItemList[Indice] direto, com Indice
// vindo do inventario ou do pacote do cliente: indice alem de 20477 lia
// memoria fora do array. Aqui o acesso fora da faixa devolve nil e o chamador
// decide o que fazer.
func (t *Tables) Item(i int) *ItemFromList {
	if t == nil || i < 0 || i >= len(t.Items) {
		return nil
	}
	return &t.Items[i]
}

// Skill devolve a skill de indice i, ou nil fora da faixa.
func (t *Tables) Skill(i int) *T_SkillData {
	if t == nil || i < 0 || i >= len(t.Skills) {
		return nil
	}
	return &t.Skills[i]
}

// ExpFor devolve a experiencia necessaria para o nivel, ou 0 fora da faixa.
func (t *Tables) ExpFor(level int) uint64 {
	if t == nil || level < 0 || level >= len(t.Exp) {
		return 0
	}
	return t.Exp[level]
}

// BuffSeconds devolve a duracao do buff de indice i, em segundos.
//
// Implementa entity.Durations. Na origem isto era SkillData[Index].Duration
// com SkillData sendo ARRAY [0..11998] acessado sem checagem: um indice de
// buff acima da faixa lia memoria fora da tabela. Aqui indice invalido, ou
// tabela nao carregada, responde 0 — o buff expira na hora em vez de durar
// um tempo tirado de memoria qualquer.
func (t *Tables) BuffSeconds(i uint16) int {
	s := t.Skill(int(i))
	if s == nil {
		return 0
	}
	// Duration e DWORD no arquivo; um valor absurdo viraria um buff eterno.
	if s.Duration > maxBuffSeconds {
		return maxBuffSeconds
	}
	return int(s.Duration)
}

// maxBuffSeconds limita a duracao lida do arquivo a 24 horas.
const maxBuffSeconds = 24 * 60 * 60

// Reinforce3At devolve a linha i de Reinforce3, ou nil fora da faixa.
//
// A origem indexava "Reinforce3[GetItemReinforce3Index(Index)]" sem checagem,
// e o indice calculado podia passar do fim da tabela.
// PremiumAt devolve a linha da tabela premium pela POSICAO, ou nil fora da
// faixa.
//
// A origem fazia "PremiumItems[Packet.Data]" com Packet.Data vindo do cliente
// como DWORD, num vetor dinamico, sem nenhuma checagem
// (PacketHandlers.pas:4578) — e "PremiumItems[Premium.Index]" com o valor
// guardado no inventario premium (ItemFunctions.pas:2005).
func (t *Tables) PremiumAt(i int) *PremiumItem {
	if t == nil || i < 0 || i >= len(t.Premium) {
		return nil
	}
	return &t.Premium[i]
}

// PremiumByIndex acha a linha pelo campo Index, nao pela posicao.
//
// Porte de TItemFunctions.FindPremiumIndex, que existe na origem exatamente
// para isso — e que NENHUM dos dois pontos que indexam a tabela usa.
func (t *Tables) PremiumByIndex(index uint32) (*PremiumItem, int) {
	if t == nil || index == 0 {
		return nil, -1
	}
	for i := range t.Premium {
		if t.Premium[i].Index == index {
			return &t.Premium[i], i
		}
	}
	return nil, -1
}

func (t *Tables) Reinforce3At(i int) *ArmorAttributeReinforce {
	if t == nil || i < 0 || i >= len(t.Reinforce3) {
		return nil
	}
	return &t.Reinforce3[i]
}

// ExpForLevel devolve a experiencia total do nivel.
//
// Implementa game.ExpTable. A origem fazia "ExpList[Level]" com ExpList sendo
// vetor dinamico e Level um WORD: nivel acima do tamanho da tabela lia fora do
// vetor, e isso acontece ao chegar no teto se o arquivo for menor que
// LEVEL_CAP. Aqui o nivel fora da faixa e dito explicitamente.
func (t *Tables) ExpForLevel(level uint16) (uint64, bool) {
	if t == nil || int(level) >= len(t.Exp) {
		return 0, false
	}
	return t.Exp[level], true
}
