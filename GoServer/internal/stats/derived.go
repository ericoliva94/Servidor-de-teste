package stats

import "aika/internal/num"

// Derived sao os atributos calculados a partir dos pontos do personagem.
//
// Porte de TBaseMob.GetCurrentScore (BaseMob.pas). Nesta fase entram so as
// contas sobre os pontos base; equipamento, buff, skill e reliquia somam por
// cima atraves dos campos de Bonus, que ficam em zero enquanto esses sistemas
// nao estiverem portados. Ver GoServer/PORTE.md.
//
// Os limites de cada campo sao os das funcoes de Util.pas que a origem usava
// para somar: IncWord satura em 65535, IncByte em 255, IncCritical em 255,
// IncCooldown em 70 e IncSpeedMove em [15, 70].
type Derived struct {
	AtkFis uint16 // DNFis
	AtkMag uint16 // DNMAG
	DefFis uint16 // DEFFis
	DefMag uint16 // DEFMAG

	SpeedMove uint16
	DuploAtk  uint16
	Critical  uint16
	// DamageCritical e o dano extra do critico.
	DamageCritical uint16
	FisPenetration uint16
	MagPenetration uint16
	HabAtk         uint16
	CureTax        uint16
	CritRes        uint16
	ResDamageCrit  uint16
	DuploRes       uint16
	Acerto         uint8
	Esquiva        uint8
	Resistence     uint16
	ReduceCooldown uint16
}

// Bonus e o que vem de fora dos pontos: equipamento, buff, skill, reliquia.
//
// Na origem cada um destes era uma chamada a GetMobAbility(EF_*) espalhada
// pela conta. Reunidos aqui, a conta base fica legivel e o dia em que o
// sistema de efeitos entrar basta preencher a struct.
type Bonus struct {
	Str, Agility, Int, Cons, Luck int

	AtkFis, AtkMag int
	DefFis, DefMag int
	// AtkFisPercent e AtkMagPercent sao EF_PER_DAMAGE1 e 2, em pontos
	// percentuais sobre o valor ja somado.
	AtkFisPercent, AtkMagPercent int
	DefFisPercent, DefMagPercent int

	SpeedMove      int
	Double         int
	Critical       int
	CriticalPower  int
	FisPenetration int
	MagPenetration int
	SkillDamage    int
	CritRes        int
	CritDefence    int
	DuploRes       int
	Acerto         int
	Esquiva        int
	Resistence     int
	Cooldown       int

	// Unarmor e o EF_UNARMOR: zera as duas defesas.
	Unarmor bool
}

// Score reune os pontos do personagem que entram na conta.
type Score struct {
	Str     uint16
	Agility uint16
	Int     uint16
	Cons    uint16
	Luck    uint16
}

// baseSpeedMove e o 40 de "IncSpeedMove(SpeedMove, 40 + EF_RUNSPEED)".
const baseSpeedMove = 40

// Compute calcula os atributos derivados.
//
// As constantes multiplicativas (2.6 por forca e por agilidade no dano
// fisico, 3.2 por inteligencia no magico, e assim por diante) sao as da
// origem, inclusive as que la estao comentadas e nao somam: elas ficaram de
// fora aqui tambem, para o resultado ser o mesmo.
func Compute(sc Score, b Bonus) Derived {
	var d Derived

	// Os pontos efetivos somam o bonus, saturando em 255 como IncCritical.
	str := num.IncCritical(0, int(sc.Str)+b.Str)
	agi := num.IncCritical(0, int(sc.Agility)+b.Agility)
	inte := num.IncCritical(0, int(sc.Int)+b.Int)
	cons := num.IncCritical(0, int(sc.Cons)+b.Cons)
	luck := num.IncCritical(0, int(sc.Luck)+b.Luck)

	d.SpeedMove = num.IncSpeedMove(0, baseSpeedMove+b.SpeedMove)

	d.DuploAtk = num.IncCritical(0, trunc(float64(str)*0.21))
	d.DuploAtk = num.IncCritical(d.DuploAtk, b.Double)

	d.Critical = num.IncCritical(0, trunc(float64(agi)*0.13))
	d.Critical = num.IncCritical(d.Critical, b.Critical)

	d.DamageCritical = num.IncCritical(0, trunc(float64(str)*0.2))
	d.DamageCritical = num.IncCritical(d.DamageCritical, b.CriticalPower)

	d.FisPenetration = num.IncCooldown(0, trunc(float64(str)*0.04))
	d.FisPenetration = num.IncCooldown(d.FisPenetration, b.FisPenetration)
	d.MagPenetration = num.IncCooldown(0, trunc(float64(inte)*0.34))
	d.MagPenetration = num.IncCooldown(d.MagPenetration, b.MagPenetration)

	d.HabAtk = num.IncWord(0, int(luck)*6)
	d.HabAtk = num.IncWord(d.HabAtk, b.SkillDamage)

	d.CureTax = num.IncCritical(0, trunc(float64(inte)*0.7))
	d.CureTax = num.IncCritical(d.CureTax, trunc(float64(cons)*0.3))
	d.CureTax = num.IncCritical(d.CureTax, trunc(float64(str)*0.2))
	d.CureTax = num.IncCritical(d.CureTax, trunc(float64(agi)*0.1))

	d.CritRes = num.IncCritical(0, trunc(float64(cons)*0.15))
	d.CritRes = num.IncCritical(d.CritRes, trunc(float64(luck)*0.2))
	d.CritRes = num.IncCritical(d.CritRes, b.CritRes)

	d.ResDamageCrit = num.IncCritical(0, trunc(float64(cons)*0.2))
	d.ResDamageCrit = num.IncCritical(d.ResDamageCrit, b.CritDefence)

	d.DuploRes = num.IncCritical(0, trunc(float64(cons)*0.34))
	d.DuploRes = num.IncCritical(d.DuploRes, b.DuploRes)

	d.Acerto = num.IncByte(0, trunc(float64(agi)*0.1))
	d.Acerto = num.IncByte(d.Acerto, b.Acerto)

	d.Esquiva = num.IncByte(0, trunc(float64(agi)*0.021))
	d.Esquiva = num.IncByte(d.Esquiva, b.Esquiva)

	// Resistence usa Round na origem, e nao Trunc.
	d.Resistence = num.IncCritical(0, round(float64(luck)*0.1))
	d.Resistence = num.IncCritical(d.Resistence, b.Resistence)

	d.ReduceCooldown = num.IncCooldown(0, trunc(float64(inte)*0.25))
	d.ReduceCooldown = num.IncCooldown(d.ReduceCooldown, b.Cooldown)

	// Defesa: o equipamento entra primeiro (GetEquipsDefense), depois o
	// percentual sobre o total e depois os somadores fixos.
	d.DefFis = num.IncWord(0, b.DefFis)
	d.DefMag = num.IncWord(0, b.DefMag)
	d.DefFis = num.IncWord(d.DefFis, b.DefFisPercent*(int(d.DefFis)/100))
	d.DefMag = num.IncWord(d.DefMag, b.DefMagPercent*(int(d.DefMag)/100))
	if b.Unarmor {
		d.DefFis = 0
		d.DefMag = 0
	}

	// Ataque: a arma entra primeiro (GetEquipDamage do slot 6), depois os
	// pontos, depois o percentual sobre o total.
	d.AtkFis = num.IncWord(0, b.AtkFis)
	d.AtkFis = num.IncWord(d.AtkFis, trunc(float64(str)*2.6))
	d.AtkFis = num.IncWord(d.AtkFis, trunc(float64(agi)*2.6))
	d.AtkFis = num.IncWord(d.AtkFis, b.AtkFisPercent*(int(d.AtkFis)/100))

	d.AtkMag = num.IncWord(0, b.AtkMag)
	d.AtkMag = num.IncWord(d.AtkMag, trunc(float64(inte)*3.2))
	d.AtkMag = num.IncWord(d.AtkMag, b.AtkMagPercent*(int(d.AtkMag)/100))

	return d
}

// trunc e o Trunc do Delphi: corta para zero.
func trunc(v float64) int { return int(v) }

// round e o Round do Delphi: para o par mais proximo no empate.
func round(v float64) int {
	i := int(v)
	frac := v - float64(i)
	switch {
	case frac > 0.5 || frac < -0.5:
		if v > 0 {
			return i + 1
		}
		return i - 1
	case frac == 0.5 || frac == -0.5:
		// Empate vai para o par.
		if i%2 == 0 {
			return i
		}
		if v > 0 {
			return i + 1
		}
		return i - 1
	default:
		return i
	}
}
