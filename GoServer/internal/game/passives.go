package game

import "aika/internal/skillfn"

// Indices EF_* usados pelas passivas de classe (GlobalDefs.pas) que ainda nao
// tinham constante no Go.
const (
	efRegenHP     = 64
	efRegenMP     = 65
	efReduceAOE   = 270
	efHPAtkRes    = 395
	efMPCure      = 157
	efGuardRate   = 238
	efGuard       = 237
	efDeceiveAtk  = 358
	efPranReqMP   = 194
	efCastRate    = 28
	efManaBurn    = 305
	efRequireMP   = 112
	efImSilence1  = 246
	efImSkillShck = 249
)

// classPassive e uma linha da tabela de TPlayer.SearchSkillsPassive: o efeito vale
// base + mult*nivel.
type classPassive struct{ ef, base, mult int }

// classPassiveTable e o "case SkillData[Skill].Index" de SearchSkillsPassive, uma
// entrada por skill passiva de classe. A chave e o campo Index da linha de
// SkillData, nao a posicao na tabela.
var classPassiveTable = map[int][]classPassive{
	9:   {{efDamage1, 0, 2}, {efHit, 0, 4}},                 // Fortitude (Guerreiro)
	10:  {{efHP, 0, 145}, {efRegenHP, 0, (2000 / 100) * 5}}, // Corpo Draconiano
	146: {{efCooltime, 0, 2}},                               // Inspirar Coragem
	23:  {{efReduceAOE, 30, 2}, {efCritical, 0, 1}},         // Instinto de Batalha
	33:  {{efHPAtkRes, 0, 1}},                               // Punicao (Templario)
	34:  {{efSkillDamage6, 0, 120}, {efMPCure, 10, 2}},      // Revelacao
	149: {{efGuardRate, 10, 2}, {efGuard, 5, 1}},            // Defesa Automatica
	47:  {{efDamage1, 8, 4}, {efCritical, 0, 1}},            // Julgamento
	57:  {{efHit, 0, 2}},                                    // Concentracao (Atirador)
	58:  {{efCriticalPower, 5, 5}},                          // Poder Critico
	152: {{efRunSpeed, 1, 1}, {efSkillDamage, 50, 50}},      // Ultimato
	71:  {{efResistance6, 0, 3}, {efResistance7, 0, 3}},     // Guarda Fatal
	81:  {{efCritical, 0, 1}, {efCriticalPower, 0, 3}},      // Olhar Penetrante (Dual Gunner)
	82:  {{efParry, 2, 1}},                                  // Movimento Gracioso
	155: {{efImRunSpeed, 0, 5}, {efImImmovable, 0, 3}},      // Vento Cortante
	95:  {{efStateResist, 8, 2}, {efDeceiveAtk, 0, 1}},      // Falsa Pontaria
	105: {{efDamage2, 0, 5}, {efPranReqMP, 0, 3}},           // Tempestade de Mana (Feiticeiro)
	106: {{efMP, 0, 80}, {efRegenMP, 0, 5}},                 // Harmonia de Mana
	158: {{efImSilence1, 0, 5}, {efImFear, 0, 3}},           // Afinidade Negra
	119: {{efCastRate, 0, 4}, {efManaBurn, 4, 1}},           // Focar Magia
	129: {{efSkillDamage6, 0, 120}, {efMPCure, 8, 2}},       // Ativacao Divina (Clerigo)
	130: {{efSkillATime6, 0, 2}, {efRequireMP, 10, 1}},      // Ativacao de Mana: 8 + (2 + nivel)
	161: {{efImImmovable, 0, 5}, {efImSkillShck, 0, 3}},     // Vontade Inabalavel
	143: {{efSkillDamage6, 17, 3}},                          // Penitencia
}

// classPassiveEffects devolve o que uma passiva de classe soma num nivel. Nivel 0
// (skill nao aprendida) nao soma nada: a origem so chama a rotina para o que o
// personagem tem, mas "base" sem nivel daria bonus de graca.
func classPassiveEffects(index, level int) []efv {
	if level <= 0 {
		return nil
	}
	var out []efv
	for _, p := range classPassiveTable[index] {
		out = append(out, efv{p.ef, p.base + p.mult*level})
	}
	return out
}

// refreshPassives porta TPlayer.SearchSkillsPassive: desfaz o que as passivas
// somaram (Mode 1) e soma de novo pelo que o personagem sabe agora (Mode 0).
//
// So entram skills agressivas (Agressive 1 ou 2) com Attribute 0, a marca das
// passivas na tabela. O que entrou fica guardado em passiveApplied, e o que
// sai e exatamente isso, como nos titulos: a origem recalculava o valor para
// subtrair e, se o nivel tivesse mudado no meio, o acumulador desequilibrava.
//
// Diferencas deliberadas da origem:
//   - SetActiveSkillPassive (aprender skill) e SearchSkillsPassive (entrar no
//     jogo) somavam coisas diferentes: aprender nao dava nada para a
//     Punicao e dava EF_UPCURE 1 na Revelacao, enquanto entrar dava o
//     contrario. Aqui as duas passam pela mesma tabela, a de entrar, e o
//     total nao muda de um login para outro.
//   - Nivel 0 nao soma.
func (h *Handler) refreshPassives(s *Session) {
	for _, e := range s.passiveApplied {
		s.Avatar.Ability.decrease(e.ef, e.v)
	}
	s.passiveApplied = nil
	if h.SkillTable == nil {
		return
	}
	for _, sk := range s.Skills.Others {
		if sk.Index == 0 {
			continue
		}
		d := skillfn.Data(h.SkillTable, uint32(sk.Index))
		if d == nil || d.Attribute != 0 || (d.Agressive != 1 && d.Agressive != 2) {
			continue
		}
		for _, e := range classPassiveEffects(int(d.Index), int(sk.Level)) {
			s.Avatar.Ability.increase(e.ef, e.v)
			s.passiveApplied = append(s.passiveApplied, e)
		}
	}
}
