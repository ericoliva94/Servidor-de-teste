package combat

import "aika/internal/num"

// Constantes de efeito de GlobalDefs.pas, com o mesmo valor.
const (
	EFReflection1  = 24
	EFAmpPhysical  = 29
	EFSkillAbsorb1 = 79
	EFPointDefence = 155
	EFType45       = 197
	EFSkillAbsorb2 = 276
)

// Indices de buff usados pelo pipeline, como em BaseMob.pas.
const (
	BuffAbsorb432    = 432
	BuffAbsorb123    = 123
	BuffAbsorb131    = 131
	BuffAbsorb142    = 142
	BuffReflection   = 38
	BuffPointDefence = 32
)

// MaxDamage e o teto de AttackParse. Acima disso o dano vira 1, nao o teto:
// e a guarda contra valor absurdo, e o porte mantem o comportamento.
const MaxDamage = 999999

// Target e o alvo de um golpe, visto pelo pipeline de dano.
//
// Na origem era um PBaseMob e as mensagens saiam por
// Servers[ChannelId].Players[mob.ClientID].SendClientMessage. Como Players e
// ARRAY [1..1000] e mob.ClientID pode ser id de mob (acima de 1000), aquela
// linha indexava fora do vetor sempre que um mob tinha o buff de absorcao —
// e ela aparece oito vezes so em AttackParseForMobs. Aqui quem entrega a
// mensagem e o chamador, que sabe se o alvo e jogador, entao nao ha indice.
type Target interface {
	// HasBuff diz se o buff esta ativo.
	HasBuff(index uint16) bool
	// RemoveBuff remove o buff.
	RemoveBuff(index uint16)
	// Ability devolve o valor acumulado de um efeito (GetMobAbility).
	Ability(eff int) int
	// DecAbility desconta de um efeito (DecreasseMobAbility).
	DecAbility(eff int, value int)
	// Notify manda texto ao alvo. Deve ser inofensivo quando o alvo nao e
	// jogador.
	Notify(msg string)
	// AddHP soma (ou subtrai, com valor negativo) pontos de vida.
	AddHP(value int)
}

// Amplify aplica os aumentos de dano que vem do alvo.
//
// Porte do inicio de AttackParseForMobs: EF_AMP_PHYSICAL soma a sua
// porcentagem e EF_TYPE45 (raio solar) soma 10%.
func Amplify(dano int, t Target) int {
	if amp := t.Ability(EFAmpPhysical); amp > 0 {
		dano += (dano / 100) * amp
	}
	if t.Ability(EFType45) > 0 {
		dano += (dano / 100) * 10
	}
	return dano
}

// absorbRule descreve um buff que absorve dano. Na origem os quatro casos
// eram blocos copiados, iguais a menos do indice do buff e do efeito.
type absorbRule struct {
	buff    uint16
	ability int
}

var absorbRules = [...]absorbRule{
	{BuffAbsorb432, EFSkillAbsorb1},
	{BuffAbsorb123, EFSkillAbsorb1},
	{BuffAbsorb131, EFSkillAbsorb1},
	{BuffAbsorb142, EFSkillAbsorb2},
}

// Absorb aplica os buffs de absorcao na ordem da origem.
//
// Quando o escudo cobre o golpe inteiro, o dano vai a zero, o tipo passa a
// None e o escudo e descontado; quando nao cobre, o escudo e consumido, o
// dano e reduzido e o buff cai.
func Absorb(dano int, dmgType DamageType, t Target) (int, DamageType) {
	for _, r := range absorbRules {
		if dano <= 0 || !t.HasBuff(r.buff) {
			continue
		}
		shield := t.Ability(r.ability)
		if shield > dano {
			t.Notify(absorbMsg(dano))
			t.DecAbility(r.ability, dano)
			return 0, None
		}
		t.Notify(absorbMsg(shield))
		dano = int(num.DecInt(int32(dano), int32(shield)))
		t.RemoveBuff(r.buff)
	}
	return dano, dmgType
}

func absorbMsg(points int) string {
	return "Você absorveu o ataque em " + itoa(points) + " pontos."
}

// PointDefence aplica o buff 32: desconta a porcentagem de EF_POINT_DEFENCE e
// gasta um ponto de defesa.
func PointDefence(dano int, t Target, defesaPoints *uint8) int {
	if !t.HasBuff(BuffPointDefence) {
		return dano
	}
	dano = int(num.DecInt(int32(dano), int32((dano/100)*t.Ability(EFPointDefence))))
	if defesaPoints != nil {
		*defesaPoints = num.DecByte(*defesaPoints, 1)
	}
	return dano
}

// Reflect devolve ao atacante a parte refletida do dano.
//
// Porte do trecho de EF_REFLECTION1 de AttackParseForMobs. Ali o codigo lia
// Helper e aplicava Help1:
//
//	Helper := mob.GetMobAbility(EF_REFLECTION1);
//	if (Helper > 0) then ... ((Dano div 100) * Help1);
//
// Help1 so era atribuido nos caminhos de absorcao, que podiam nem ter
// rodado, entao o reflexo usava um valor de outra conta ou lixo da pilha. Era
// um erro de digitacao: o valor certo e o que acabou de ser lido.
func Reflect(dano int, t Target, attacker Target) int {
	if dano <= 0 || attacker == nil {
		return 0
	}
	pct := t.Ability(EFReflection1)
	if pct <= 0 {
		return 0
	}
	refletido := (dano / 100) * pct
	if refletido > 0 {
		attacker.AddHP(-refletido)
	}
	return refletido
}

// ClampDamage aplica as duas guardas do fim de AttackParse.
//
// Acima de 999999 o dano vira 1, nao o teto: e assim na origem e mexer nisso
// mudaria o jogo. Abaixo de zero tambem vira 1, o que garante que um golpe
// nunca cure o alvo — era isso que faltava no AttackPlayer da dungeon, onde
// dano negativo somava vida.
func ClampDamage(dano int) int {
	if dano > MaxDamage {
		return 1
	}
	if dano < 0 {
		return 1
	}
	return dano
}

// itoa converte sem puxar strconv para o caminho de mensagem.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
