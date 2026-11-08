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

// EquipStats e a soma de ataque e defesa dos itens vestidos.
type EquipStats struct {
	AtkFis, AtkMag, DefFis, DefMag int
}

// EquippedStats porta GetEquipDamage (arma, slot 6) e GetEquipsDefense
// (slots 2 a 7, sem o 6) de BaseMob.pas. Item sem durabilidade (Min = 0) nao
// conta, como na origem.
//
// Divergencia: a origem, com Refi >= 16 e item sem prazo, troca o valor da
// tabela de itens pelo de Reinforce2[...].Attribute*[refino-1]. Esse indice
// (GetItemReinforce2Index) ainda nao foi portado, entao o item refinado conta
// com o valor base da tabela de itens, que e o piso do valor real.
func EquippedStats(t Tabelas, equip []model.Item) EquipStats {
	var out EquipStats
	if t == nil {
		return out
	}
	for slot := SlotHelmet; slot <= SlotShield; slot++ {
		if slot >= len(equip) {
			break
		}
		it := equip[slot]
		if it.IsEmpty() || it.Min == 0 {
			continue
		}
		d := t.Item(int(it.Index))
		if d == nil {
			continue
		}
		if slot == SlotRing { // slot 6, a arma
			out.AtkFis += int(d.ATKFis)
			out.AtkMag += int(d.MagATK)
			continue
		}
		out.DefFis += int(d.DefFis)
		out.DefMag += int(d.DefMag)
	}
	return out
}

// MaxSlotAmount e o MAX_SLOT_AMOUNT de GlobalDefs.pas: o maior empilhamento
// num slot.
const MaxSlotAmount = 1000

// CanAgroup diz se o item empilha. Porte de ItemList[i].CanAgroup.
//
// Item que nao esta na tabela nao empilha. A origem lia
// "ItemList[SrcItem.Index].CanAgroup" com Index ate 65535 num vetor menor.
func CanAgroup(t Tabelas, index uint16) bool {
	it := t.Item(int(index))
	return it != nil && it.CanAgroup != 0
}

// Amount devolve a quantidade empilhada do item.
//
// A quantidade vive no campo Refi, o mesmo que guarda o refino nos
// equipamentos: e empilhavel ou refinavel, nunca os dois.
func Amount(it model.Item) uint16 { return it.Refi }

// SetAmount grava a quantidade, se o item empilhar. Porte de SetItemAmount.
func SetAmount(t Tabelas, it *model.Item, quant uint16) {
	if it == nil || !CanAgroup(t, it.Index) {
		return
	}
	it.Refi = quant
}

// AddAmount soma a quantidade, com teto. Porte de SetItemAmount(Somar=True).
//
// A origem fazia "Inc(item.Refi, quant)" num WORD, sem teto: somar acima de
// 65535 dava a volta e a pilha virava um punhado de itens.
func AddAmount(t Tabelas, it *model.Item, quant uint16) {
	if it == nil || !CanAgroup(t, it.Index) {
		return
	}
	soma := int(it.Refi) + int(quant)
	if soma > MaxSlotAmount {
		soma = MaxSlotAmount
	}
	it.Refi = uint16(soma)
}

// DecreaseAmount tira quant da pilha e esvazia o slot quando acaba. Porte de
// TItemFunctions.DecreaseAmount.
//
// A origem testava "(item.Refi - Quanti) > 0" com os dois WORD: a subtracao
// dava a volta e um item com 1 unidade do qual se tirassem 2 ficava com
// 65535. Aqui a conta e com sinal.
func DecreaseAmount(it *model.Item, quant uint16) {
	if it == nil {
		return
	}
	if int(it.Refi)-int(quant) > 0 {
		it.Refi -= quant
		return
	}
	*it = model.Item{}
}

// Agroup junta a pilha de src na de dest. Porte de TItemFunctions.AgroupItem.
//
// Devolve true quando as duas pilhas couberam numa so (dest esvaziado).
//
// Mantido da origem: a pilha junta fica em src, nao em dest, e quando a soma
// passa de MAX_SLOT_AMOUNT com um dos lados ja cheio os dois itens sao
// trocados de lugar em vez de somados.
func Agroup(t Tabelas, src, dest *model.Item) bool {
	if src == nil || dest == nil {
		return false
	}
	if !CanAgroup(t, src.Index) {
		return false
	}
	if int(src.Refi)+int(dest.Refi) > MaxSlotAmount {
		if src.Refi == MaxSlotAmount || dest.Refi == MaxSlotAmount {
			*src, *dest = *dest, *src
			return true
		}
		resto := uint16(int(src.Refi) + int(dest.Refi) - MaxSlotAmount)
		SetAmount(t, src, MaxSlotAmount)
		SetAmount(t, dest, resto)
		return false
	}
	src.Refi += dest.Refi
	*dest = model.Item{}
	return true
}

// ItemType devolve o ItemType do item, ou 0 quando o item nao existe.
func ItemType(t Tabelas, index uint16) uint16 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.ItemType
}

// ItemLevel devolve o nivel exigido pelo item, ou 0.
func ItemLevel(t Tabelas, index uint16) uint16 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.Level
}

// ItemName devolve o nome do item sem a cauda de zeros, ou vazio.
func ItemName(t Tabelas, index uint16) string {
	it := t.Item(int(index))
	if it == nil {
		return ""
	}
	return cstring(it.Name[:])
}

// SellPrice devolve o preco de venda do item, usado pelas moedas de ouro.
func SellPrice(t Tabelas, index uint16) uint32 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.SellPrince
}

// cstring corta a string no primeiro zero.
func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// ItemClass devolve a classe exigida pelo item, ou 0.
func ItemClass(t Tabelas, index uint16) uint16 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.Classe
}

// TypeTrade devolve a regra de troca do item, ou 0.
func TypeTrade(t Tabelas, index uint16) uint8 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.TypeTrade
}

// Rank devolve o rank do item, ou 0.
func Rank(t Tabelas, index uint16) uint8 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.Rank
}

// UseEffect devolve o efeito de uso do item, ou 0.
func UseEffect(t Tabelas, index uint16) uint16 {
	it := t.Item(int(index))
	if it == nil {
		return 0
	}
	return it.UseEffect
}

// Tipos de item que o movimento de inventario precisa distinguir.
const (
	ItemTypePet   = 8  // mascote
	ItemTypeMount = 9  // montaria, pelo slot de equipamento
	ItemTypePran  = 10 // pran
)
