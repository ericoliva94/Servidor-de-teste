package num

import "testing"

func TestDecWord(t *testing.T) {
	cases := []struct {
		x    uint16
		v    int
		want uint16
	}{
		{100, 30, 70},
		{100, 100, 0},
		{100, 200, 0}, // limita em zero em vez de estourar para 65436
		{0, 1, 0},
		{0, -5, 5},          // valor negativo soma
		{65535, -10, 65535}, // limita no teto
		{65535, 0, 65535},
	}
	for _, c := range cases {
		if got := DecWord(c.x, c.v); got != c.want {
			t.Errorf("DecWord(%d, %d) = %d, queria %d", c.x, c.v, got, c.want)
		}
	}
}

func TestIncWord(t *testing.T) {
	cases := []struct {
		x    uint16
		v    int
		want uint16
	}{
		{100, 30, 130},
		{65000, 1000, 65535},
		{65535, 1, 65535},
		{10, -20, 0},
		// O original usa ">= MAX" e "<= 0", entao o valor exato no limite
		// tambem e fixado no limite.
		{65534, 1, 65535},
		{5, -5, 0},
	}
	for _, c := range cases {
		if got := IncWord(c.x, c.v); got != c.want {
			t.Errorf("IncWord(%d, %d) = %d, queria %d", c.x, c.v, got, c.want)
		}
	}
}

func TestDecByteIncByte(t *testing.T) {
	if got := DecByte(10, 20); got != 0 {
		t.Errorf("DecByte(10,20) = %d, queria 0", got)
	}
	if got := DecByte(200, 50); got != 150 {
		t.Errorf("DecByte(200,50) = %d", got)
	}
	if got := IncByte(250, 10); got != 255 {
		t.Errorf("IncByte(250,10) = %d, queria 255", got)
	}
	if got := IncByte(10, -20); got != 0 {
		t.Errorf("IncByte(10,-20) = %d, queria 0", got)
	}
}

// Util.pas tem duas funcoes para inteiro com sentidos diferentes, e trocar uma
// pela outra muda o resultado.
func TestDecIntVersusRaw(t *testing.T) {
	if got := DecInt(10, 25); got != 0 {
		t.Errorf("DecInt(10,25) = %d, queria 0 (limita)", got)
	}
	if got := DecIntRaw(10, 25); got != -15 {
		t.Errorf("DecIntRaw(10,25) = %d, queria -15 (nao limita)", got)
	}
}

// Para Cardinal o original cai no Dec nativo e estoura; esta funcao e o que
// se deve usar onde o valor e de jogo.
func TestDecCardinalClampsInsteadOfWrapping(t *testing.T) {
	if got := DecCardinal(100, 500); got != 0 {
		t.Errorf("DecCardinal(100,500) = %d, queria 0", got)
	}
	// O que o Dec nativo do Delphi faria com Cardinal: estourar para perto de
	// 4 bilhoes. Calculado em variavel para o compilador nao recusar a
	// constante negativa.
	x, v := uint32(100), uint32(500)
	if wrapped := x - v; wrapped != 4294966896 {
		t.Fatalf("o estouro de uint32 deu %d", wrapped)
	}
	if got := DecCardinal(1000, 1); got != 999 {
		t.Errorf("DecCardinal(1000,1) = %d, queria 999", got)
	}
}

func TestDecInt64(t *testing.T) {
	if got := DecInt64(100, 500); got != 0 {
		t.Errorf("DecInt64(100,500) = %d, queria 0", got)
	}
	if got := DecInt64(500, 100); got != 400 {
		t.Errorf("DecInt64(500,100) = %d", got)
	}
	if got := DecInt64(0, 1); got != 0 {
		t.Errorf("DecInt64(0,1) = %d", got)
	}
}

func TestDecUint64(t *testing.T) {
	if got := DecUint64(100, 500); got != 0 {
		t.Errorf("DecUint64(100,500) = %d, queria 0", got)
	}
	if got := DecUint64(500, 100); got != 400 {
		t.Errorf("DecUint64(500,100) = %d", got)
	}
	if got := DecUint64(10, -5); got != 15 {
		t.Errorf("DecUint64(10,-5) = %d, queria 15", got)
	}
}

func TestAttributeClamps(t *testing.T) {
	if got := IncSpeedMove(60, 100); got != MaxSpeedMove {
		t.Errorf("IncSpeedMove teto = %d", got)
	}
	if got := IncSpeedMove(20, -100); got != MinSpeedMove {
		t.Errorf("IncSpeedMove piso = %d, queria 15", got)
	}
	if got := IncCooldown(10, 100); got != MaxCooldown {
		t.Errorf("IncCooldown teto = %d", got)
	}
	if got := IncCooldown(10, -100); got != 0 {
		t.Errorf("IncCooldown piso = %d", got)
	}
	if got := IncCritical(200, 100); got != MaxCritical {
		t.Errorf("IncCritical teto = %d", got)
	}
}

func TestIfThen(t *testing.T) {
	if got := IfThen(true, 1, 2); got != 1 {
		t.Errorf("IfThen(true) = %d", got)
	}
	if got := IfThen(false, "a", "b"); got != "b" {
		t.Errorf("IfThen(false) = %q", got)
	}
}
