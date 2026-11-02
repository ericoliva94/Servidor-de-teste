// Package itemfn tem as contas sobre itens que dependem das tabelas .bin.
//
// Porte de Server/Src/Functions/ItemFunctions.pas. So o que o caminho de
// entrada e de spawn usa entra aqui por ora; o resto do arquivo (refino,
// composicao, preco) vem nos incrementos seguintes.
package itemfn

import (
	"math"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// Tabelas e a parte de gamedata.Tables que estas contas leem. Interface para
// o pacote poder ser testado sem carregar arquivo.
type Tabelas interface {
	Item(i int) *gamedata.ItemFromList
	Reinforce3At(i int) *gamedata.ArmorAttributeReinforce
}

// Slots de equipamento que entram na conta de HP/MP por refino.
const (
	SlotHelmet = 2
	SlotArmor  = 3
	SlotGloves = 4
	SlotShoes  = 5
	SlotRing   = 6
	SlotShield = 7
)

// Faixas de Reinforce3, de FilesData.pas.
const (
	reinforce3SectionSize = 150
	areaHelmet            = 0
	areaShield            = 0 // Reinforce3_Area_Shield = Reinforce3_Area_Helmet
	areaArmor             = 30
	areaGloves            = 60
	areaShoes             = 90
)

// MaxReinforceLevel e o tamanho de HealthIncrementPoints: ARRAY [0..15].
const MaxReinforceLevel = 16

// EquipSlot devolve o slot em que o item se veste. Porte de
// TItemFunctions.GetItemEquipSlot.
//
// A origem escrevia Result := 15 para ItemType 50 e 52 e seguia sem Exit, de
// modo que o teste seguinte podia sobrescrever o 15; na pratica nao
// sobrescreve, porque 50 e 52 nao estao em 1..15 nem em 1001..1010, mas a
// ordem fica explicita aqui.
func EquipSlot(t Tabelas, index uint16) int {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	switch {
	case it.ItemType > 0 && it.ItemType < 16:
		return int(it.ItemType)
	case it.ItemType > 1000 && it.ItemType < 1011:
		return SlotRing
	case it.ItemType == 50 || it.ItemType == 52:
		return 15
	}
	return 0
}

// ReinforceFromItem devolve o nivel de refino do item. Porte de
// TItemFunctions.GetReinforceFromItem.
//
// A origem usava Round(item.Refi / 16) — divisao em ponto flutuante com
// arredondamento bancario do Delphi, nao divisao inteira. Mantido igual: com
// Refi 8 o resultado e 0 (empate vai para o par) e com Refi 24 e 2. Trocar
// por "div 16" mudaria o refino de metade dos itens.
func ReinforceFromItem(it model.Item) int {
	if it.Refi == 0 {
		return 0
	}
	return int(math.RoundToEven(float64(it.Refi) / 16))
}

// Reinforce3Index devolve o indice do item na tabela Reinforce3. Porte de
// TItemFunctions.GetItemReinforce3Index.
//
// Devolve -1 quando o indice nao cai na tabela. A origem devolvia um WORD
// qualquer: com UseEffect acima de 105 a faixa ficava em zero e o resultado
// era o proprio UseEffect, que depois indexava Reinforce3 sem checagem.
func Reinforce3Index(t Tabelas, index uint16) int {
	it := t.Item(int(index))
	if it == nil {
		return -1
	}
	use := int(it.UseEffect)

	var base int
	switch {
	case use <= 35:
		base = reinforce3SectionSize * 0
	case use <= 70:
		base = reinforce3SectionSize*1 - 35
	case use <= 105:
		base = reinforce3SectionSize*2 - 70
	default:
		// Fora das tres faixas que a origem conhece. Ali o indice saia errado
		// e lia a tabela fora do lugar; aqui a conta nao se aplica.
		return -1
	}

	switch EquipSlot(t, index) {
	case SlotHelmet:
		base += areaHelmet
	case SlotArmor:
		base += areaArmor
	case SlotGloves:
		base += areaGloves
	case SlotShoes:
		base += areaShoes
	case SlotShield:
		base += areaShield
	}
	return base + use
}

// ReinforceHPMPInc devolve o bonus de HP/MP do refino de um item. Porte de
// TItemFunctions.GetItemReinforceHPMPInc.
//
// refineLevel e o indice em HealthIncrementPoints, ARRAY [0..15]. A origem
// passava "Refine - 1" com Refine vindo de Round(Refi / 16), que chega a 4095
// num Refi adulterado: isso indexava o vetor de 16 posicoes muito alem do
// fim, lendo memoria de outros registros da tabela. Aqui fora da faixa vale
// zero.
func ReinforceHPMPInc(t Tabelas, index uint16, refineLevel int) uint32 {
	if refineLevel < 0 || refineLevel >= MaxReinforceLevel {
		return 0
	}
	i := Reinforce3Index(t, index)
	if i < 0 {
		return 0
	}
	r := t.Reinforce3At(i)
	if r == nil {
		return 0
	}
	return r.HealthIncrementPoints[refineLevel]
}

// EquippedHPMPInc soma o bonus de HP/MP do refino do equipamento vestido.
// Porte de TBaseMob.GetEquipedItensHPMPInc.
//
// A origem percorria os slots 2 a 7 pulando o 6 e ignorava item com Time > 0
// (item com prazo nao conta). Mantido.
func EquippedHPMPInc(t Tabelas, equip []model.Item) int {
	total := 0
	for slot := SlotHelmet; slot <= SlotShield; slot++ {
		if slot == SlotRing {
			continue
		}
		if slot >= len(equip) {
			break
		}
		it := equip[slot]
		if it.IsEmpty() || it.Time > 0 {
			continue
		}
		refino := ReinforceFromItem(it)
		if refino == 0 {
			continue
		}
		total += int(ReinforceHPMPInc(t, it.Index, refino-1))
	}
	return total
}
