// Package model tem os tipos de dados do jogo, com o mesmo layout binario do
// Delphi. Eles viajam na rede e vao para o banco, entao o tamanho de cada um
// e travado por teste contra o sizeof() do original.
package model

import (
	"encoding/binary"
	"time"
)

// Position e TPosition de MiscData.pas: dois Single, 8 bytes.
type Position struct {
	X float32
	Y float32
}

// Distance e a distancia em celulas, como TPosition.Distance.
func (p Position) Distance(o Position) uint16 {
	dx := p.X - o.X
	dy := p.Y - o.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return uint16(dx)
	}
	return uint16(dy)
}

// InRange diz se o alvo esta a no maximo range celulas.
func (p Position) InRange(o Position, r uint16) bool {
	return p.Distance(o) <= r
}

// ItemEffect e TItemEffect: 3 indices e 3 valores, 6 bytes.
type ItemEffect struct {
	Index [3]byte
	Value [3]byte
}

// Item e TItem de MiscData.pas, 20 bytes (a constante TITEM_SIZE do Delphi).
//
// Os campos Refi e Time compartilham funcao com a data de expiracao: os 3
// bytes em Refi[1] + Time guardam o unixtime deslocado 8 bits. Ver Expire.
type Item struct {
	Index     uint16
	APP       uint16
	Identific int32
	Effects   ItemEffect
	Min       uint8
	Max       uint8
	Refi      uint16
	Time      uint16
}

// ItemSize e o sizeof(TItem) do Delphi.
const ItemSize = 20

// IsEmpty diz se o slot esta vazio. No Delphi o teste era Index = 0 espalhado
// pelo codigo.
func (it Item) IsEmpty() bool { return it.Index == 0 }

// expireUnix monta o unixtime a partir dos 3 bytes guardados (byte alto de
// Refi + os dois de Time).
//
// O Delphi fazia isso com aritmetica de ponteiro:
//
//	Move(PDWORD(LPARAM(@self.Time) - 1)^, PDWORD(LPARAM(@UnixDate) + 1)^, 3)
//
// e deixava o byte 0 de UnixDate sem inicializar, entao a data vinha com ate
// 255 segundos de lixo a cada leitura — o proprio arquivo marca o trecho com
// "erro aqui por conta do x64". Aqui o byte baixo e zero, o que torna a
// leitura estavel e no mesmo passo de 256 s em que a escrita grava.
func (it Item) expireUnix() uint32 {
	var b [4]byte
	b[1] = byte(it.Refi >> 8)
	b[2] = byte(it.Time)
	b[3] = byte(it.Time >> 8)
	return binary.LittleEndian.Uint32(b[:])
}

// setExpireUnix grava os 3 bytes altos do unixtime, preservando o byte baixo
// de Refi, exatamente como o Move do Delphi fazia.
func (it *Item) setExpireUnix(unix uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], unix)
	it.Refi = it.Refi&0x00FF | uint16(b[1])<<8
	it.Time = uint16(b[2]) | uint16(b[3])<<8
}

// Expire devolve a data de expiracao do item. Para itens cujo ItemType e 9 a
// contagem e em dias desde base; para os demais e o unixtime deslocado.
// O chamador informa o ItemType porque a tabela ItemList vive em outro pacote.
func (it Item) Expire(itemType uint16, base time.Time) time.Time {
	if itemType == 9 {
		return base.AddDate(0, 0, int(it.Time))
	}
	return time.Unix(int64(it.expireUnix()), 0).UTC()
}

// SetExpire grava a data de expiracao, espelhando TItem.SetExpire.
func (it *Item) SetExpire(itemType uint16, base time.Time, when time.Time) {
	if itemType == 9 {
		days := int(when.Sub(base).Hours() / 24)
		if days < 0 {
			days = 0
		}
		if days > 0xFFFF {
			days = 0xFFFF
		}
		it.Time = uint16(days)
		return
	}
	unix := when.Unix()
	if unix < 0 {
		unix = 0
	}
	it.setExpireUnix(uint32(unix))
}

// IsSealed diz se o item esta selado. TItem.GetIsSealed leia os 4 bytes de
// Refi+Time como um DWORD e considera selado quando sao todos zero.
func (it Item) IsSealed() bool {
	return it.Refi == 0 && it.Time == 0
}

// SetSealed sela ou desela o item. Ao desselar, o Delphi zera Refi e grava a
// expiracao em Now + Duration + 2 horas; duration vem da ItemList.
func (it *Item) SetSealed(sealed bool, itemType uint16, base time.Time, duration int, now time.Time) {
	if sealed {
		it.Refi = 0
		it.Time = 0
		return
	}
	it.Refi = 0
	it.SetExpire(itemType, base, now.Add(time.Duration(duration+2)*time.Hour))
}

// EquipSellPrice e o preco de venda proporcional a durabilidade.
//
// TItem.GetEquipSellPrice fazia `MIN / MAX` sem checar MAX: com um item de
// durabilidade maxima zero (slot zerado, item recem-criado) isso levanta
// divisao por zero no Delphi. Aqui MAX = 0 vale preco zero.
func (it Item) EquipSellPrice(priceGold uint32) uint32 {
	if it.Max == 0 {
		return 0
	}
	perc := float64(it.Min) / float64(it.Max)
	return uint32(float64(priceGold)*perc + 0.5)
}
