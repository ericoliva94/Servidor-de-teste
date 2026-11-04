// Package num tem a aritmetica com limite que o servidor usa em todo lugar.
//
// Porte de Server/Src/Functions/Util.pas. Essas funcoes nao sao detalhe de
// estilo: no Delphi elas sobrepoem o Dec e o Inc nativos para Word, Byte e
// Int64, entao uma chamada "Dec(X, N)" no codigo original limita em zero em
// vez de estourar — mas somente para esses tipos. Para Cardinal nao existe
// sobrecarga e o Dec nativo entra, com estouro silencioso. Traduzir isso
// errado troca "fica em zero" por "vira 4 bilhoes".
package num

// Limites de GlobalDefs.pas.
const (
	MaxByte = 255
	MaxWord = 65535
)

// DecWord subtrai limitando em [0, 65535]. Porte de Util.DecWORD e da
// sobrecarga Dec(var x: Word).
func DecWord(x uint16, v int) uint16 {
	res := int(x) - v
	switch {
	case res < 0:
		return 0
	case res > MaxWord:
		return MaxWord
	default:
		return uint16(res)
	}
}

// IncWord soma limitando em [0, 65535]. Porte de Util.IncWord.
func IncWord(x uint16, v int) uint16 {
	res := int(x) + v
	switch {
	case res >= MaxWord:
		return MaxWord
	case res <= 0:
		return 0
	default:
		return uint16(res)
	}
}

// DecByte subtrai limitando em [0, 255]. Porte da sobrecarga
// Dec(var x: Byte).
func DecByte(x uint8, v int) uint8 {
	res := int(x) - v
	switch {
	case res < 0:
		return 0
	case res > MaxByte:
		return MaxByte
	default:
		return uint8(res)
	}
}

// IncByte soma limitando em [0, 255]. Porte de Util.IncByte.
func IncByte(x uint8, v int) uint8 {
	res := int(x) + v
	switch {
	case res >= MaxByte:
		return MaxByte
	case res <= 0:
		return 0
	default:
		return uint8(res)
	}
}

// DecInt subtrai limitando em zero. Porte de Util.DecInt.
//
// Cuidado ao ler o codigo original: Util tem DUAS funcoes para inteiro com
// sentidos diferentes. DecInt limita em zero; Dec(var x: integer) subtrai
// direto e pode ficar negativo (ver DecIntRaw).
func DecInt(x, v int32) int32 {
	res := int64(x) - int64(v)
	if res < 0 {
		return 0
	}
	return int32(res)
}

// DecIntRaw subtrai sem limite. Porte da sobrecarga Dec(var x: integer), que
// ao contrario das outras nao trata o resultado negativo.
func DecIntRaw(x, v int32) int32 { return x - v }

// DecCardinal subtrai limitando em zero. Porte de Util.DecCardinal.
//
// Para Cardinal nao existe sobrecarga de Dec no Util, entao "Dec(X, N)" no
// original usa o intrinseco e estoura: X menor que N vira um numero perto de
// 4 bilhoes. Onde esse padrao aparecer com valor de jogo (ouro, cash), use
// esta funcao.
func DecCardinal(x uint32, v int64) uint32 {
	res := int64(x) - v
	if res < 0 {
		return 0
	}
	return uint32(res)
}

// DecInt64 subtrai limitando em zero. Porte da sobrecarga
// Dec(var x: Int64; Value: Variant).
func DecInt64(x, v int64) int64 {
	if v > 0 && x < v {
		return 0
	}
	res := x - v
	if res < 0 {
		return 0
	}
	return res
}

// DecUint64 subtrai limitando em zero. Porte de Util.DecUInt64.
func DecUint64(x uint64, v int64) uint64 {
	if v < 0 {
		return x + uint64(-v)
	}
	if x < uint64(v) {
		return 0
	}
	return x - uint64(v)
}

// Limites dos atributos, como em Util.pas.
const (
	MinSpeedMove = 15
	MaxSpeedMove = 70
	MaxCooldown  = 70
	MaxCritical  = 255
)

// IncSpeedMove soma limitando em [15, 70]. Porte de Util.IncSpeedMove.
func IncSpeedMove(x uint16, v int) uint16 {
	res := int(x) + v
	switch {
	case res >= MaxSpeedMove:
		return MaxSpeedMove
	case res <= MinSpeedMove:
		return MinSpeedMove
	default:
		return uint16(res)
	}
}

// IncCooldown soma limitando em [0, 70]. Porte de Util.IncCooldown.
func IncCooldown(x uint16, v int) uint16 {
	res := int(x) + v
	switch {
	case res >= MaxCooldown:
		return MaxCooldown
	case res <= 0:
		return 0
	default:
		return uint16(res)
	}
}

// IncCritical soma limitando em [0, 255]. Porte de Util.IncCritical.
func IncCritical(x uint16, v int) uint16 {
	res := int(x) + v
	switch {
	case res >= MaxCritical:
		return MaxCritical
	case res <= 0:
		return 0
	default:
		return uint16(res)
	}
}

// IfThen devolve a ou b conforme cond. Porte de Util.IFThen, que o original
// usa com um unico argumento para converter condicao em Boolean.
func IfThen[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
