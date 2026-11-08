// Package pran e o modelo da pran (TPran de PlayerData.pas): atributos,
// equipamento, bolsas e habilidades, mais as contas que o cliente espera no
// pacote de entrada ($907).
package pran

import (
	"math"

	"aika/internal/model"
)

// Tamanhos dos vetores da pran.
const (
	EquipSlots     = 16
	InventorySlots = 42
	SkillSlots     = 10
	BarSlots       = 3
	NameLen        = 16
)

// Tipos de slot de item da pran na tabela items (PRAN_EQUIP_TYPE e
// PRAN_INV_TYPE) e tipo da habilidade na tabela skills.
const (
	SlotTypeEquip     = 5
	SlotTypeInventory = 6
	SkillType         = 3
	// BarOwnerBase soma ao id da pran para formar o dono na tabela itembars.
	BarOwnerBase = 1024000
)

// Skill e uma habilidade da pran: indice-base da tabela de skills e nivel.
type Skill struct {
	Index uint16
	Level uint16
}

// Pran e uma pran da conta.
type Pran struct {
	ID     uint16 // ClientID de spawn (0 ate ser invocada)
	DBID   uint32 // id da linha em prans
	ItemID uint16 // identific do item da pran (liga ao Equip[10] do personagem)
	AccID  uint32
	Name   string

	Level, Class uint8
	CurHP, MaxHP uint32
	CurMP, MaxMP uint32
	Exp          uint32
	DefFis       uint16
	DefMag       uint16
	Food         uint8
	Devotion     uint8

	Cute, Smart, Sexy, Energetic, Tough, Corrupt uint16

	Width, Chest, Leg uint8
	CreatedAt         uint32
	UpdatedAt         uint32

	Equip     [EquipSlots]model.Item
	Inventory [InventorySlots]model.Item
	Skills    [SkillSlots]Skill
	ItemBar   [BarSlots]uint32

	Spawned  bool
	Position model.Position
}

// Personality e o byte de personalidade do $907. A origem testava as seis em
// ordem e a ultima com valor >= devocao ganhava (0 se nenhuma); a segunda
// pran comparava com a devocao da PRIMEIRA, o que e corrigido aqui.
func (p *Pran) Personality() uint16 {
	var out uint16
	vals := [6]uint16{p.Cute, p.Smart, p.Sexy, p.Energetic, p.Tough, p.Corrupt}
	for i, v := range vals {
		if v >= uint16(p.Devotion) {
			out = uint16(i)
		}
	}
	return out
}

// IsFairy diz se a classe esta na forma de fada (classes 61, 71 e 81); a pran
// nessa forma nao aparece no mundo, so o jogador ganha o efeito.
func IsFairy(class uint8) bool {
	return class == 61 || class == 71 || class == 81
}

// skillPackedLevel e o porte de TSkillFunctions.GetSkillPranLevel: devolve o
// valor e quantos bytes dele o pacote usa.
func skillPackedLevel(slot int, level uint16) (uint32, int) {
	l := uint32(math.Round(math.Pow(2, float64(level)))) - 1
	if slot == 0 {
		return l, 1
	}
	a := uint32(math.Round(math.Pow(float64(slot), 4)))
	if a == 1 {
		a = 4
	}
	v := l * a
	switch {
	case v <= 255:
		return v, 1
	case v <= 65535:
		return v, 2
	}
	return v, 1 // a origem deixava o Result inicial (1) fora das duas faixas
}

// PackSkills monta os 16 bytes "Unk" do $907: o nivel de cada habilidade
// escrito no deslocamento do seu slot, na ordem, como o Move da origem (um
// valor de 2 bytes invade o byte seguinte, que o proximo slot reescreve).
func (p *Pran) PackSkills() [16]byte {
	var out [16]byte
	for i, sk := range p.Skills {
		if sk.Level == 0 {
			continue
		}
		v, n := skillPackedLevel(i, sk.Level)
		for b := 0; b < n && i+b < len(out); b++ {
			out[i+b] = byte(v >> (8 * b))
		}
	}
	return out
}
