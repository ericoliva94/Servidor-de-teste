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
	// BuffBlock19 e BuffBlock91 zeram o golpe recebido e sao consumidos no
	// uso. Em GetDamage eles davam Block e Miss2.
	BuffBlock19 = 19
	BuffBlock91 = 91
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

// Constantes de efeito e buff dos passos restantes de AttackParseForMobs.
const (
	EFHPConversion = 142
	EFMPEfficiency = 143
	EFReflection2  = 156
)

const (
	BuffPolymorph    = 99
	BuffHPConversion = 101
	BuffNevoaFC      = 111
	BuffChoqueDual   = 86
	BuffChoqueAtt    = 63
	BuffPredador     = 153
)

// consumeOnHitBuffs sao os buffs que simplesmente caem ao receber um golpe.
// Na origem eram quatro ifs seguidos, cada um so com RemoveBuffByIndex.
var consumeOnHitBuffs = [...]uint16{
	BuffNevoaFC,
	BuffChoqueDual,
	BuffChoqueAtt,
	BuffPredador,
}

// ConsumeOnHitBuffs remove os buffs que se gastam ao ser atingido.
func ConsumeOnHitBuffs(t Target) {
	for _, b := range consumeOnHitBuffs {
		if t.HasBuff(b) {
			t.RemoveBuff(b)
		}
	}
}

// Polymorphed e o alvo que pode estar transformado.
type Polymorphed interface {
	Target
	// IsPolymorphed diz se o alvo esta transformado.
	IsPolymorphed() bool
	// ClearPolymorph desfaz a transformacao.
	ClearPolymorph()
	// IsPlayer diz se o alvo e jogador. Na origem o teste era
	// "mob.ClientID <= MAX_CONNECTIONS", sem o limite de baixo, e com isso o
	// id 0 passava como jogador.
	IsPlayer() bool
	// Respawn reenvia o mob ao cliente depois de desfazer a transformacao.
	Respawn()
}

// Polymorph trata o alvo transformado: o golpe conta como duplo critico e a
// transformacao se desfaz. Porte do bloco "if (mob.Polimorfed)".
func Polymorph(dmgType DamageType, t Polymorphed) DamageType {
	if !t.IsPolymorphed() {
		return dmgType
	}
	t.ClearPolymorph()
	if t.IsPlayer() {
		t.RemoveBuff(BuffPolymorph)
		t.Respawn()
	}
	return DoubleCritical
}

// ManaShield e o alvo que converte dano em mana (buff 101).
type ManaShield interface {
	Target
	// CurrentMP devolve a mana atual. Na origem era
	// mob.Character.CurrentScore.CurMP, com Character sendo um ponteiro que
	// podia ser nil e era desreferenciado sem checagem — outras partes do
	// mesmo arquivo testam "if (Self.Character <> nil)" antes de usar.
	CurrentMP() (mp uint32, ok bool)
	// RemoveMP desconta mana.
	RemoveMP(value int)
}

// HPConversion aplica o buff 101: parte do dano sai da mana em vez da vida.
//
// Duas coisas da origem ficam registradas aqui:
//
//   - O custo de mana e "Help1 * (GetMobAbility(EF_MP_EFFICIENCY) div 100)".
//     O valor desse efeito e um numero pequeno (o trecho comentado em
//     BaseMob.pas:9815 usa ele como multiplicador de "Dano div 200"), entao a
//     divisao inteira por 100 da zero e a conversao nao custa mana nenhuma.
//     Quase certamente era "Help1 * eff div 100", mas como a escala pretendida
//     e ambigua o porte mantem a conta da origem: mudar custo de recurso
//     alteraria o balanceamento sem ter como validar.
//   - A comparacao era "DWORD(Help1) >= CurMP". Com dano negativo, o Help1
//     negativo convertido para DWORD virava um numero enorme e o teste dava
//     sempre verdadeiro. Aqui a comparacao e feita com sinal.
func HPConversion(dano int, t ManaShield) int {
	if !t.HasBuff(BuffHPConversion) {
		return dano
	}
	absorvido := (dano / 100) * t.Ability(EFHPConversion)
	custoMana := absorvido * (t.Ability(EFMPEfficiency) / 100)

	mp, ok := t.CurrentMP()
	// Sem Character carregado nao ha mana para gastar: na origem isso era
	// uma desreferencia de ponteiro nil.
	if ok && absorvido >= 0 && uint32(absorvido) >= mp {
		t.RemoveMP(custoMana)
		t.RemoveBuff(BuffHPConversion)
	} else {
		t.RemoveMP(custoMana)
	}
	return int(num.DecInt(int32(dano), int32(absorvido)))
}

// HPConversionPassive aplica a reducao passiva de EF_HP_CONVERSION, aplicada
// fora do buff 101.
func HPConversionPassive(dano int, t Target) int {
	eff := t.Ability(EFHPConversion)
	if eff <= 0 {
		return dano
	}
	return int(num.DecInt(int32(dano), int32((dano/100)*eff)))
}

// Reflect2 e o reflexo do buff 38, que devolve dano e zera o golpe.
//
// Na origem o alvo do reflexo era
// Servers[ChannelId].MOBS.TMobS[Mobid].MobsP[SecondIndex].HP, indexado sem
// checagem de faixa. Aqui o atacante recebe o dano pela interface, igual ao
// Reflect de EF_REFLECTION1.
func Reflect2(dano int, t Target, attacker Target) (novoDano int, refletido int, dmgType DamageType) {
	if !t.HasBuff(BuffReflection) {
		return dano, 0, None
	}
	pct := t.Ability(EFReflection2)
	refletido = (dano / 100) * pct
	if refletido > 0 && attacker != nil {
		attacker.AddHP(-refletido)
	}
	t.RemoveBuff(BuffReflection)
	return 0, refletido, None
}

// FullTarget reune tudo que o pipeline completo precisa do alvo.
type FullTarget interface {
	Polymorphed
	ManaShield
}

// Result e o que sobrou de um golpe depois de passar pelo alvo.
type Result struct {
	Damage    int
	Type      DamageType
	Reflected int
}

// ApplyToTarget roda o pipeline inteiro na ordem de AttackParseForMobs.
//
// A ordem importa: cada passo trabalha sobre o dano que o anterior deixou, e
// alguns zeram o golpe. defesaPoints pode ser nil quando o alvo nao tem
// pontos de defesa.
func ApplyToTarget(dano int, dmgType DamageType, t FullTarget, attacker Target,
	defesaPoints *uint8) Result {

	dano = Amplify(dano, t)
	dano, dmgType = Absorb(dano, dmgType, t)
	dmgType = Polymorph(dmgType, t)
	dano = HPConversion(dano, t)
	ConsumeOnHitBuffs(t)
	dano = HPConversionPassive(dano, t)

	var refletido int
	if novo, r, dt := Reflect2(dano, t, attacker); r != 0 || novo != dano {
		dano, refletido, dmgType = novo, r, dt
	}
	if dano > 0 {
		refletido += Reflect(dano, t, attacker)
	}
	dano = PointDefence(dano, t, defesaPoints)

	return Result{
		Damage:    ClampDamage(dano),
		Type:      dmgType,
		Reflected: refletido,
	}
}
