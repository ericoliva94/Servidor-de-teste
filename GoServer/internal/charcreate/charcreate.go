// Package charcreate monta o personagem novo: o modelo da classe, as skills
// iniciais, a validacao do nome e a posicao de partida.
//
// Porte de TPacketHandlers.CreateCharacter (PacketHandlers.pas:612),
// TFunctions.LoadBasicCharacter / CreateBasicCharacter e
// TSkillFunctions.GetSkillIndex. Nao faz I/O de banco.
//
// ATENCAO sobre os modelos (.acc): os arquivos de Server/Bin/Data/BaseAccs
// tem 7.249 bytes e o registro TBasicCharacter calculado pelo gerador tem
// 5.081. So o comeco do arquivo foi conferido contra o layout (classe,
// atributos, tamanhos do corpo, nivel, equipamento e inventario); a partir
// dai o arquivo diverge. Por isso Parse usa so esse comeco e as skills vem da
// regra de CreateBasicCharacter. Ver KNOWN_ISSUES.md.
package charcreate

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"aika/internal/model"
)

// Classes na ordem de InitialAccounts / ClasseChar.
const (
	ClassWarrior = iota
	ClassTemplar
	ClassSniper
	ClassGunslinger
	ClassMage
	ClassCleric
	NumClasses
)

// FileNames sao os modelos, na ordem das classes (Load.pas:66).
var FileNames = [NumClasses]string{
	"Guerreiro", "Templaria", "Atirador", "Pistoleira", "Feiticeiro", "Cleriga",
}

// Limites do nome e do cabelo (CreateCharacter).
const (
	MaxNameLen = 14
	HairMin    = 7700
	HairMax    = 7731
	MaxSlot    = 2
)

// Itens fixos da criacao.
const (
	ItemStarterBag   = 5300 // Inventory[60]
	ItemStorageBag   = 5310 // Storage.Itens[80] da conta
	StarterBagSlot   = 60
	StorageBagSlot   = 80
	ItemBulletSniper = 4615
	ItemBulletGunner = 4600
	BulletAmount     = 1000
	EquipFace        = 0
	EquipHair        = 1
	EquipBullet      = 15
	InventoryBullet1 = 5
	InventoryBullet2 = 6
)

// Erros de validacao.
var (
	ErrNameEmpty  = errors.New("charcreate: nome vazio")
	ErrNameChars  = errors.New("charcreate: so letras e numeros")
	ErrNameLong   = errors.New("charcreate: nome acima de 14 caracteres")
	ErrBadClass   = errors.New("charcreate: classe invalida")
	ErrBadHair    = errors.New("charcreate: cabelo invalido")
	ErrBadSlot    = errors.New("charcreate: slot invalido")
	ErrNoTemplate = errors.New("charcreate: modelo da classe indisponivel")
)

// ValidName confere o nome. A origem aceitava nome vazio (IsLetter devolve
// True para texto de tamanho zero) e contava o tamanho em caracteres Unicode
// de um campo ANSI; aqui o vazio e recusado e o tamanho e em bytes.
func ValidName(name string) error {
	if name == "" {
		return ErrNameEmpty
	}
	if len(name) > MaxNameLen {
		return ErrNameLong
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !ok {
			return ErrNameChars
		}
	}
	return nil
}

// ClassGroup traduz o ClassIndex do pacote (10 a 69) na classe 0 a 5.
func ClassGroup(classIndex uint16) (int, error) {
	if classIndex < 10 || classIndex > 69 {
		return 0, ErrBadClass
	}
	return int(classIndex)/10 - 1, nil
}

// ValidHair confere o cabelo.
func ValidHair(h uint16) error {
	if h < HairMin || h > HairMax {
		return ErrBadHair
	}
	return nil
}

// StartPosition e onde o personagem nasce, por cidade (Packet.Local). Local
// fora de 0 e 1 deixa a posicao zerada, que o login troca pelo ponto de
// partida padrao.
func StartPosition(local uint32) (x, y float32) {
	switch local {
	case 0:
		return 3432, 672
	case 1:
		return 3443, 912
	}
	return 0, 0
}

// SkillIndex e TSkillFunctions.GetSkillIndex(Classe, Skill, Level), com
// Classe de 1 a 6.
func SkillIndex(classe, skill, level int) int {
	result := 1
	if classe > 1 {
		result = (classe - 1) * 960
	}
	if skill > 1 {
		result += (skill - 1) * 16
	}
	if level > 1 {
		result += level - 1
	} else if classe != 1 {
		result += level // "inc(Result, Level)" com Level = 1
	}
	return result
}

// Skills monta as skills iniciais como CreateBasicCharacter: indice de cada
// uma das 6 basicas e das 40 outras pela regra, nivel 1 nas quatro primeiras
// basicas e na primeira das outras. classe vai de 0 a 5.
func Skills(classe int) model.SkillsList {
	var l model.SkillsList
	c := classe + 1
	for i := range l.Basics {
		l.Basics[i].Index = uint16(SkillIndex(c, i+1, 1))
	}
	for i := range l.Others {
		l.Others[i].Index = uint16(SkillIndex(c, i+7, 1))
	}
	for i := 0; i < 4; i++ {
		l.Basics[i].Level = 1
	}
	l.Others[0].Level = 1
	return l
}

// ItemBar monta a barra inicial: so o guerreiro e a cleriga levam a quarta
// skill basica no atalho 3 (GetSkillIndexOnBar = indice * 16 + 2).
func ItemBar(classe int, sk model.SkillsList) [24]uint32 {
	var bar [24]uint32
	if classe == ClassWarrior || classe == ClassCleric {
		bar[3] = uint32(sk.Basics[3].Index)*16 + 2
	}
	return bar
}

// Template e o que se aproveita de um modelo .acc.
type Template struct {
	Loaded    bool
	ClassInfo uint8
	Str, Agi  uint16
	Int, Cons uint16
	Luck      uint16
	Status    uint16
	Sizes     model.Sizes
	Level     uint16
	Exp       int64
	Equip     [16]model.Item
	Inventory [64]model.Item
}

// headSize e quanto do arquivo o Parse le: o Index (4 bytes) e o registro
// TCharacter. Dentro dele so os campos ate o inventario foram conferidos.
var headSize = 4 + binary.Size(model.Character{})

// Parse le o comeco de um modelo .acc.
func Parse(b []byte) (Template, error) {
	if len(b) < headSize {
		return Template{}, fmt.Errorf("charcreate: modelo com %d bytes, preciso de pelo menos %d", len(b), headSize)
	}
	var ch model.Character
	if err := binary.Read(bytes.NewReader(b[4:headSize]), binary.LittleEndian, &ch); err != nil {
		return Template{}, fmt.Errorf("charcreate: ler modelo: %w", err)
	}
	sc := ch.CurrentScore
	t := Template{
		Loaded: true, ClassInfo: ch.ClassInfo,
		Str: sc.Str, Agi: sc.Agility, Int: sc.Int, Cons: sc.Cons, Luck: sc.Luck, Status: sc.Status,
		Sizes: sc.Sizes, Level: ch.Level, Exp: ch.Exp,
		Equip: ch.Equip, Inventory: ch.Inventory,
	}
	if t.ClassInfo == 0 || t.Level == 0 {
		return Template{}, errors.New("charcreate: modelo sem classe ou sem nivel")
	}
	return t, nil
}

// Load le os seis modelos de dir. Modelo ausente ou invalido fica sem Loaded
// e vira aviso: a classe correspondente nao poderia ser criada. A origem,
// quando o arquivo faltava, gerava um com CreateBasicCharacter; isso nao foi
// portado porque o equipamento que o codigo produz e diferente do dos
// arquivos de producao.
func Load(dir string) ([NumClasses]Template, []string) {
	var out [NumClasses]Template
	var warns []string
	for i, name := range FileNames {
		b, err := os.ReadFile(filepath.Join(dir, name+".acc"))
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s.acc: %v", name, err))
			continue
		}
		t, err := Parse(b)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s.acc: %v", name, err))
			continue
		}
		out[i] = t
	}
	return out, warns
}

// New e o personagem pronto para gravar.
type New struct {
	ClassInfo uint8
	Str, Agi  uint16
	Int, Cons uint16
	Luck      uint16
	Status    uint16
	Sizes     model.Sizes
	Level     uint16
	Exp       int64
	Gold      int64
	PosX      float32
	PosY      float32
	Equip     [16]model.Item
	Inventory [64]model.Item
	Skills    model.SkillsList
	ItemBar   [24]uint32
}

// Build monta o personagem a partir do modelo e do pedido.
func Build(t Template, classIndex, hair uint16, local uint32) (New, error) {
	grp, err := ClassGroup(classIndex)
	if err != nil {
		return New{}, err
	}
	if err := ValidHair(hair); err != nil {
		return New{}, err
	}
	if !t.Loaded {
		return New{}, ErrNoTemplate
	}
	n := New{
		ClassInfo: t.ClassInfo, Str: t.Str, Agi: t.Agi, Int: t.Int, Cons: t.Cons, Luck: t.Luck,
		Status: t.Status, Sizes: t.Sizes, Level: t.Level, Exp: t.Exp,
		Equip: t.Equip, Inventory: t.Inventory,
	}
	n.PosX, n.PosY = StartPosition(local)

	n.Equip[EquipFace].Index = classIndex
	n.Equip[EquipHair].Index = hair
	// A bolsa precisa de APP e quantidade 1: so com o Index o cliente a desenha
	// invisivel e ela some ao ser trocada (a origem repara isso ao carregar, so
	// quando o slot esta vazio).
	n.Inventory[StarterBagSlot] = model.Item{Index: ItemStarterBag, APP: ItemStarterBag, Refi: 1}

	bullet := uint16(0)
	switch grp {
	case ClassSniper:
		bullet = ItemBulletSniper
	case ClassGunslinger:
		bullet = ItemBulletGunner
	}
	if bullet != 0 {
		stack := model.Item{Index: bullet, APP: bullet, Refi: BulletAmount}
		n.Equip[EquipBullet] = stack
		n.Inventory[InventoryBullet1] = stack
		n.Inventory[InventoryBullet2] = stack
	}

	n.Skills = Skills(grp)
	n.ItemBar = ItemBar(grp, n.Skills)
	return n, nil
}
