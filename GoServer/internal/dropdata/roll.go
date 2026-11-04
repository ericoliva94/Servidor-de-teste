package dropdata

// Rand sorteia. Mesma interface de combat.Rand.
type Rand interface {
	// IntN devolve um valor em [0, n).
	IntN(n int) int
}

// Faixas do sorteio de raridade, de TBaseMob.DropItemFor.
//
// A origem sorteava "ItemTax := RandomRange(1, 101)", isto e, 1 a 100, e
// mapeava: 1 e lendaria, 2 a 13 rara, 14 a 33 superior, 34 a 255 normal.
const (
	RollMax          = 100
	LegendaryMaxRoll = 1
	RareMaxRoll      = 13
	SuperiorMaxRoll  = 33
)

// RarityForRoll traduz o sorteio em raridade.
func RarityForRoll(roll int) Rarity {
	switch {
	case roll <= LegendaryMaxRoll:
		return Legendary
	case roll <= RareMaxRoll:
		return Rare
	case roll <= SuperiorMaxRoll:
		return Superior
	default:
		return Normal
	}
}

// Roll sorteia um item da categoria. Devolve 0 quando a categoria nao tem
// item nenhum.
//
// dropRateBonus e o efeito de reliquia que a origem descontava do sorteio
// ("decint(ItemTax, ReliqEffect[EF_RELIQUE_DROP_RATE])"): descontar do numero
// sorteado empurra o resultado para as faixas raras.
//
// Diferenca central em relacao a origem: quando a raridade sorteada nao tem
// item, a busca desce para a raridade MAIS COMUM, e nao para a mais rara.
//
// Na origem o encadeamento era "ItemTypeFrom := ItemTypeFrom + 1; goto", isto
// e, normal vazio caia em superior, superior em rara e rara em lendaria. Com
// os dados que acompanham o servidor isso e grave: as categorias de Planta,
// Croshu Azul e Croshu Vermelho tem item APENAS na raridade lendaria, e a
// faixa normal do sorteio cobre 67 dos 100 valores — ou seja, dois tercos das
// mortes desses mobs entregavam um item lendario. Descer na raridade mantem o
// sorteio com o sentido que ele tem.
//
// Havia tambem uma diferenca entre os dois ramos da origem: no de mob comum a
// lendaria vazia fazia "Exit" (nenhum drop), e no de chefe
// ("InitHP > 999999") fazia "ItemTypeFrom := 1; goto ReCase", voltando a
// primeira raridade. Com as quatro raridades vazias esse segundo ramo gira
// para sempre dentro da morte do mob. Nenhuma categoria dos arquivos de hoje
// esta com as quatro vazias, mas basta um arquivo vazio para o servidor
// travar ali.
func (c *Category) Roll(roll int, dropRateBonus int, rnd Rand) (item uint16, r Rarity, ok bool) {
	if c == nil || rnd == nil {
		return 0, 0, false
	}
	// DecInt da origem tem piso em zero, e o zero era corrigido para 2.
	roll -= dropRateBonus
	if roll <= 0 {
		roll = 2
	}
	alvo := RarityForRoll(roll)

	// Desce da raridade sorteada para as mais comuns.
	for r := alvo; r >= Normal; r-- {
		if lista := c.Of(r); len(lista) > 0 {
			return lista[rnd.IntN(len(lista))], r, true
		}
	}
	// Nenhuma das mais comuns tem item: sobe, para o mob nao ficar sem drop.
	for r := alvo + 1; r <= Legendary; r++ {
		if lista := c.Of(r); len(lista) > 0 {
			return lista[rnd.IntN(len(lista))], r, true
		}
	}
	return 0, 0, false
}
