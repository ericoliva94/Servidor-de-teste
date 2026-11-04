// Package stats calcula HP e MP maximos de um personagem.
//
// Porte de TBaseMob.GetCurrentHP e TBaseMob.GetCurrentMP (BaseMob.pas), com
// as tabelas HPIncrementPerLevel e MPIncrementPerLevel e com GetMobClass.
//
// O nome na origem engana: GetCurrentHP devolve o HP MAXIMO, nao o atual —
// o atual fica em Character.CurrentScore.CurHP. GetCreateMob usa o retorno
// para preencher Packet.MaxHP justamente por isso. Aqui a funcao se chama
// MaxHP, que e o que ela faz.
package stats

// Classes de personagem, na ordem em que indexam as tabelas de incremento.
const (
	ClassWarrior = 0
	ClassTemplar = 1
	ClassArcher  = 2
	ClassDual    = 3
	ClassMage    = 4
	ClassCleric  = 5
	ClassCount   = 6
)

// HPIncrementPerLevel e MPIncrementPerLevel sao as tabelas de BaseMob.pas.
var (
	HPIncrementPerLevel = [ClassCount]int{150, 140, 115, 120, 110, 130}
	MPIncrementPerLevel = [ClassCount]int{110, 130, 145, 150, 330, 135}
)

// MobClass traduz ClassInfo em indice de classe. Porte de
// TBaseMob.GetMobClass.
//
// A origem testava faixas de 10 em 10 e deixava Result em 0 para qualquer
// valor fora delas, inclusive 0, 10, 20 e acima de 59: um ClassInfo invalido
// era contado como guerreiro em vez de recusado. Mantido, porque o valor vem
// do banco e recusar aqui tiraria o personagem do ar; quem valida ClassInfo e
// a criacao de personagem.
func MobClass(classInfo uint16) int {
	switch {
	case classInfo >= 1 && classInfo <= 9:
		return ClassWarrior
	case classInfo >= 11 && classInfo <= 19:
		return ClassTemplar
	case classInfo >= 21 && classInfo <= 29:
		return ClassArcher
	case classInfo >= 31 && classInfo <= 39:
		return ClassDual
	case classInfo >= 41 && classInfo <= 49:
		return ClassMage
	case classInfo >= 51 && classInfo <= 59:
		return ClassCleric
	}
	return ClassWarrior
}

// Input reune o que entra na conta de HP e MP.
//
// Os campos de bonus existem porque na origem eles vinham de GetMobAbility e
// de GetEquipedItensHPMPInc. Enquanto equipamento, buff e reliquia nao
// estiverem portados eles ficam em zero, e o resultado e o do personagem nu.
type Input struct {
	ClassInfo uint16
	Level     uint16
	// Cons entra no HP e Luck no MP, como na origem.
	Cons uint16
	Luck uint16

	// FlatHP e FlatMP sao EF_HP/EF_MP mais o incremento do equipamento.
	FlatHP int
	FlatMP int
	// PercentHP e PercentMP somam EF_MARSHAL_PER_*, EF_RELIQUE_PER_HP e
	// EF_PER_*, em pontos percentuais.
	PercentHP int
	PercentMP int
}

// MaxHP devolve o HP maximo.
//
// Diferenca da origem: ali hp_inc era DWORD e cada Inc somava valores que
// podiam ser negativos (GetMobAbility devolve Integer), o que dava a volta
// para perto de 4 bilhoes; o teste final "if (hp_inc <= 0)" nunca pegava isso
// porque num DWORD so o zero satisfaz <= 0. Aqui a conta e feita em int com
// sinal e o piso de 1 vale de verdade.
func MaxHP(in Input) uint32 {
	hp := in.FlatHP
	hp += int(float64(HPIncrementPerLevel[MobClass(in.ClassInfo)])*0.3) * int(in.Level)
	hp += int(in.Cons) * 27
	// A origem escrevia "hp_perc * Round(hp_inc div 100)": o div ja trunca e
	// o Round de um inteiro e ele mesmo, entao o efeito e este.
	hp += in.PercentHP * (hp / 100)
	if hp <= 0 {
		return 1
	}
	return uint32(hp)
}

// MaxMP devolve o MP maximo.
//
// Mesma correcao de sinal do MaxHP. Na origem o MP usava
// Servers[].ReliqEffect[EF_RELIQUE_PER_HP] — a constante de HP, nao a de MP;
// e um erro de digitacao da origem que aqui desaparece porque o percentual
// chega somado em PercentMP, e o lado que o monta decide qual efeito usar.
func MaxMP(in Input) uint32 {
	mp := in.FlatMP
	mp += int(float64(MPIncrementPerLevel[MobClass(in.ClassInfo)])*0.3) * int(in.Level)
	mp += int(in.Luck) * 27
	mp += in.PercentMP * (mp / 100)
	if mp <= 0 {
		return 1
	}
	return uint32(mp)
}
