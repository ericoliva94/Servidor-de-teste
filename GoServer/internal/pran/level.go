package pran

// Constantes de nivel da pran (GlobalDefs.pas e PlayerData.pas).
const (
	MaxLevel   = 50  // MAX_PRAN_LEVEL
	HPPerLevel = 209 // PRAN_HP_INC_PER_LEVEL
	MPPerLevel = 356 // PRAN_MP_INC_PER_LEVEL
)

// milestone diz, para o nivel mostrado (Level+1) de cada marco, quais slots de
// habilidade ganham um nivel e de qual a qual slot a barra e conferida. Porte
// da tabela de TPlayer.AddPranLevel.
type milestone struct {
	skills   []int
	barFirst int
	barLast  int
}

var milestones = map[int]milestone{
	5:  {[]int{0, 1, 2}, 0, 2},
	10: {[]int{0, 1, 2, 4}, 0, 4},
	15: {[]int{0, 1, 2, 4, 5}, 0, 5},
	20: {[]int{0, 1, 2, 4, 5, 6}, 0, 6},
	25: {[]int{0, 1, 2, 4, 5, 6, 7}, 0, 7},
	30: {[]int{0, 1, 2, 4, 5, 6, 7, 8}, 0, 8},
	35: {[]int{0, 1, 2, 4, 5, 6, 7, 8, 9}, 0, 9},
	40: {[]int{0, 1, 2, 4, 5, 6, 7, 8, 9}, 0, 9},
	45: {[]int{0, 1, 2, 4, 5, 6, 7, 8, 9}, 0, 9},
	50: {[]int{1, 2, 4, 5, 6, 7, 8, 9}, 1, 9},
}

// BarChange diz que a habilidade de indice NewIndex foi para o slot de barra
// Bar (o cliente precisa ser avisado).
type BarChange struct {
	Bar      int
	NewIndex uint16
}

// DisplayLevel e o nivel que o cliente mostra (Level + 1).
func (p *Pran) DisplayLevel() int { return int(p.Level) + 1 }

// LevelUp sobe um nivel: mais HP e MP maximos, vida e mana cheias. Devolve
// false no teto.
func (p *Pran) LevelUp() bool {
	if p.Level >= MaxLevel {
		return false
	}
	p.Level++
	p.MaxHP += HPPerLevel
	p.MaxMP += MPPerLevel
	p.CurHP, p.CurMP = p.MaxHP, p.MaxMP
	return true
}

// ApplyMilestone da os niveis de habilidade do marco do nivel atual, se houver.
//
// Para cada slot do trecho conferido, uma habilidade que esta na barra tem o
// indice avancado em um (a origem fazia isso alem de subir o Level, que ja
// avanca a linha lida em SkillData[Index+Level-1]; mantido como esta, ver
// KNOWN_ISSUES.md). Slot com indice 0 nunca conta como "na barra": a origem
// casava o zero de uma barra vazia.
func (p *Pran) ApplyMilestone() []BarChange {
	m, ok := milestones[p.DisplayLevel()]
	if !ok {
		return nil
	}
	for _, i := range m.skills {
		p.Skills[i].Level++
	}
	var out []BarChange
	for i := m.barFirst; i <= m.barLast; i++ {
		idx := p.Skills[i].Index
		if idx == 0 {
			continue
		}
		bar := p.barOf(uint32(idx))
		if bar < 0 {
			continue
		}
		p.Skills[i].Index++
		out = append(out, BarChange{Bar: bar, NewIndex: p.Skills[i].Index})
	}
	return out
}

// barOf acha a habilidade na barra da pran, ou -1 (PranBarExistsIndex).
func (p *Pran) barOf(index uint32) int {
	for i, v := range p.ItemBar {
		if v == index {
			return i
		}
	}
	return -1
}

// ClassNeedsEvolution diz se a classe esta na fronteira onde a pran so segue
// ganhando experiencia depois de evoluir: nivel 4 (fada 61/71/81), 19 (crianca
// 62/72/82) e 49 (adolescente 63/73/83).
func (p *Pran) ClassNeedsEvolution() bool {
	switch p.Level {
	case 4:
		return p.Class == 61 || p.Class == 71 || p.Class == 81
	case 19:
		return p.Class == 62 || p.Class == 72 || p.Class == 82
	case 49:
		return p.Class == 63 || p.Class == 73 || p.Class == 83
	}
	return false
}

// Fome e devocao (comida de pran).
const (
	MaxFood     = 121
	FoodPerMeal = 15
	MaxDevotion = 226 // a devocao so sobe enquanto for menor que isto
)

// Item de comida de personalidade: indice do item -> personalidade (0 a 5).
const (
	FirstPersonalityFood = 8105
	LastPersonalityFood  = 8110
)

// Feed da uma refeicao. Devolve false se a pran nao come mais (Food >= 121).
//
// Os itens 8105 a 8110 empurram a personalidade correspondente em +2 e baixam
// as outras cinco em 3 (com piso em zero) e somam 1 a devocao (ate 226).
func (p *Pran) Feed(item uint16) bool {
	if p.Food >= MaxFood {
		p.Food = MaxFood
		return false
	}
	if item >= FirstPersonalityFood && item <= LastPersonalityFood {
		vals := [6]*uint16{&p.Cute, &p.Sexy, &p.Smart, &p.Energetic, &p.Tough, &p.Corrupt}
		for i, v := range vals {
			if i == int(item-FirstPersonalityFood) {
				*v = incWord(*v, 2)
			} else {
				*v = decWord(*v, 3)
			}
		}
		if p.Devotion < MaxDevotion {
			p.Devotion++
		}
	}
	if int(p.Food)+FoodPerMeal > MaxFood {
		p.Food = MaxFood
	} else {
		p.Food += FoodPerMeal
	}
	return true
}

func incWord(v uint16, n int) uint16 {
	if int(v)+n > 0xFFFF {
		return 0xFFFF
	}
	return v + uint16(n)
}

func decWord(v uint16, n int) uint16 {
	if int(v) < n {
		return 0
	}
	return v - uint16(n)
}

// Digest reduz a fome pela metade. Devolve false se a pran esta com fome
// demais (Food <= 13).
func (p *Pran) Digest() bool {
	if p.Food <= 13 {
		return false
	}
	p.Food /= 2
	return true
}
