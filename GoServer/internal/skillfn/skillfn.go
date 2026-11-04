// Package skillfn tem as contas sobre skill que dependem da tabela SkillData.
//
// Porte de Server/Src/Functions/SkillFunctions.pas e da parte declarativa de
// TBaseMob.HandleSkill. O que e caso especial por skill (as tabelas
// "case SkillData[Skill].Index of" de WarriorSkill, TemplarSkill e as demais,
// com avanco, corrente, invocacao) nao entra aqui: cada uma e um
// comportamento proprio e vem numa fase seguinte. Ver GoServer/PORTE.md.
package skillfn

import (
	"aika/internal/gamedata"
	"aika/internal/model"
)

// LevelsPerSkill e a faixa de niveis de um grupo de skill.
//
// A origem testava "if (Skill >= dwSkill) and (Skill <= dwSkill + 15)": o
// indice guardado e o da base do grupo e o nivel soma a ele, ate 15.
const LevelsPerSkill = 16

// MaxSkillLevel e o maior nivel que um grupo aceita.
//
// A origem fazia "inc(Skills.Basics[i].Level)" sem teto nenhum: passando de
// 15, "Index + Level" entrava no grupo de skill seguinte, e o jogador usava
// skill de outra arvore de graca.
const MaxSkillLevel = LevelsPerSkill - 1

// Tamanhos dos vetores de skill do personagem, de PlayerData.pas.
const (
	BasicSlots = 6  // Basics: ARRAY [0..5]
	OtherSlots = 40 // Others: ARRAY [0..39]
)

// Tipos de linha da tabela skills do banco.
const (
	SkillTypeBasic = 1
	SkillTypeOther = 2
)

// BlockedSkillIndex e a skill que o servidor nao deixa aprender
// ("if(SkillData[SkillIndex].Index = 427)").
const BlockedSkillIndex = 427

// Tabela e a parte de gamedata.Tables que estas contas leem.
type Tabela interface {
	Skill(i int) *gamedata.T_SkillData
}

// Data devolve a linha da tabela, ou nil fora da faixa.
//
// A origem escrevia "SkillData[Packet.SkillIndex]" com o indice vindo do
// cliente como DWORD num ARRAY [0..11998], e fazia isso cinco vezes antes de
// qualquer validacao: um indice qualquer lia memoria fora da tabela e o
// resultado decidia nivel minimo, custo e classe.
func Data(t Tabela, index uint32) *gamedata.T_SkillData {
	if t == nil || index > uint32(gamedata.MaxSkills) {
		return nil
	}
	return t.Skill(int(index))
}

// CooldownMillis devolve a recarga da skill em milissegundos.
//
// Implementa entity.CooldownMillis.
type CooldownMillis struct{ T Tabela }

// CooldownMillis devolve a recarga da skill de indice i.
func (c CooldownMillis) CooldownMillis(i uint16) int {
	d := Data(c.T, uint32(i))
	if d == nil {
		return 0
	}
	return int(d.Cooldown)
}

// BuffSeconds devolve a duracao do buff da skill.
//
// Implementa entity.Durations, para o buff da skill expirar no tempo certo.
type BuffSeconds struct{ T Tabela }

// BuffSeconds devolve a duracao em segundos.
func (b BuffSeconds) BuffSeconds(i uint16) int {
	d := Data(b.T, uint32(i))
	if d == nil {
		return 0
	}
	return int(d.Duration)
}

// GroupBase devolve o indice de base do grupo a que a skill pertence, dado o
// conjunto do personagem, e o nivel atual do grupo.
//
// Porte da busca de IncremmentSkillLevel: o grupo e aquele cuja base esta a
// no maximo 15 abaixo do indice pedido.
func GroupBase(sk model.SkillsList, index uint32) (base uint16, nivel uint16, slot int, basico bool, ok bool) {
	for i := range sk.Basics {
		b := sk.Basics[i].Index
		if b == 0 {
			continue
		}
		if index >= uint32(b) && index <= uint32(b)+MaxSkillLevel {
			return b, sk.Basics[i].Level, i, true, true
		}
	}
	for i := range sk.Others {
		b := sk.Others[i].Index
		if b == 0 {
			continue
		}
		if index >= uint32(b) && index <= uint32(b)+MaxSkillLevel {
			return b, sk.Others[i].Level, i, false, true
		}
	}
	return 0, 0, 0, false, false
}

// Knows diz se o personagem tem a skill no nivel pedido.
//
// A origem nao conferia isso em UseSkill: bastava mandar o indice no $320 e a
// skill saia, aprendida ou nao. So o nivel minimo do personagem era testado.
func Knows(sk model.SkillsList, index uint32) bool {
	base, nivel, _, _, ok := GroupBase(sk, index)
	if !ok {
		return false
	}
	// O indice usavel vai de base a base+nivel.
	return index <= uint32(base)+uint32(nivel)
}

// IncLevel sobe um nivel do grupo da skill. Devolve o indice de base e false
// quando a skill nao pertence a nenhum grupo do personagem ou o grupo esta no
// teto.
func IncLevel(sk *model.SkillsList, index uint32) (base uint16, ok bool) {
	if sk == nil {
		return 0, false
	}
	b, nivel, slot, basico, achou := GroupBase(*sk, index)
	if !achou {
		return 0, false
	}
	if nivel >= MaxSkillLevel {
		// Sem isto o nivel passava de 15 e "Index + Level" caia no grupo
		// seguinte da tabela.
		return b, false
	}
	if basico {
		sk.Basics[slot].Level = nivel + 1
	} else {
		sk.Others[slot].Level = nivel + 1
	}
	return b, true
}

// MatchClass diz se a classe da skill serve ao personagem. Porte de
// TBaseMob.MatchClassInfo.
//
// A origem comparava com GetMobClass nos dois lados, o que faz classe 0 cair
// em guerreiro; mantido, porque e o que decide o que cada classe aprende hoje.
func MatchClass(classInfo uint16, skillClasse uint32) bool {
	return mobClass(classInfo) == mobClass(uint16(skillClasse))
}

// IsPranSkill diz se a skill e de pran (classes 61 a 84), que ainda nao esta
// portada.
func IsPranSkill(skillClasse uint32) bool {
	return skillClasse >= 61 && skillClasse <= 84
}

// IsAreaSkill diz se a skill bate em area. Porte do teste
// "(SuccessRate = 1) and (range > 0)" de HandleSkill.
func IsAreaSkill(d *gamedata.T_SkillData) bool {
	return d != nil && d.SuccessRate == 1 && d.Range > 0
}

// IsSingleTargetDamage diz se a skill bate num alvo so. Porte de
// "(SuccessRate = 1) and (range = 0)".
func IsSingleTargetDamage(d *gamedata.T_SkillData) bool {
	return d != nil && d.SuccessRate == 1 && d.Range == 0
}

// IsBuff diz se a skill aplica um efeito com duracao.
func IsBuff(d *gamedata.T_SkillData) bool {
	return d != nil && d.Duration > 0
}

// SkillDamageBonus e o que a skill soma ao dano base.
//
// Porte de "Inc(Dano, (DataSkill^.Damage + PlayerCharacter.HabAtk) div 2)" de
// AttackParse.
func SkillDamageBonus(d *gamedata.T_SkillData, habAtk uint16) int {
	if d == nil {
		return 0
	}
	return (int(d.Damage) + int(habAtk)) / 2
}

// mobClass e a mesma faixa de stats.MobClass, repetida aqui para o pacote nao
// depender dele.
func mobClass(classInfo uint16) int {
	switch {
	case classInfo >= 11 && classInfo <= 19:
		return 1
	case classInfo >= 21 && classInfo <= 29:
		return 2
	case classInfo >= 31 && classInfo <= 39:
		return 3
	case classInfo >= 41 && classInfo <= 49:
		return 4
	case classInfo >= 51 && classInfo <= 59:
		return 5
	}
	return 0
}

// LevelMask devolve a mascara de niveis liberados de um grupo de skill.
//
// Porte de TSkillFunctions.GetSkillLevel, que calculava
// "Trunc(Power(2, Level + 1) - 2)": os bits 1 a Level ligados.
//
// Com nivel 16 ou mais o valor passa de 65535 e, na origem, o Move de 4 bytes
// escrevia por cima do slot seguinte do vetor. Aqui o nivel e limitado ao
// teto do grupo antes da conta, o que mantem a mascara dentro dos 16 bits do
// campo.
func LevelMask(nivel uint16) uint16 {
	if nivel == 0 {
		return 0
	}
	if nivel > MaxSkillLevel {
		nivel = MaxSkillLevel
	}
	return uint16((1 << (nivel + 1)) - 2)
}
