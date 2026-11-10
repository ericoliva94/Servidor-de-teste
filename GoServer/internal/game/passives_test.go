package game

import (
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
)

func passiveSession(t *testing.T, defs map[int]gamedata.T_SkillData, skills ...model.SkillFromList) (*Handler, *Session) {
	t.Helper()
	h, tab, _ := handlerComSkills(t)
	for idx, d := range defs {
		tab.poe(idx, d)
	}
	s := lutador("Alfa", 1000, 1000)
	for i, sk := range skills {
		s.Skills.Others[i] = sk
	}
	return h, s
}

func TestPassivesSumByLevel(t *testing.T) {
	h, s := passiveSession(t,
		map[int]gamedata.T_SkillData{9: {Agressive: 1}},
		model.SkillFromList{Index: 9, Level: 3})
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efDamage1); got != 6 {
		t.Errorf("Fortitude nv3: dano = %d, queria 6", got)
	}
	if got := s.Avatar.Ability.get(efHit); got != 12 {
		t.Errorf("Fortitude nv3: acerto = %d, queria 12", got)
	}
}

func TestPassivesBaseValuesAndTwoSkills(t *testing.T) {
	h, s := passiveSession(t, map[int]gamedata.T_SkillData{
		23:  {Agressive: 2}, // 30 + 2*L de reducao de area
		47:  {Agressive: 1}, // 8 + 4*L de dano
		130: {Agressive: 1}, // 8 + (2 + L) de MP
	},
		model.SkillFromList{Index: 23, Level: 2},
		model.SkillFromList{Index: 47, Level: 2},
		model.SkillFromList{Index: 130, Level: 4})
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efReduceAOE); got != 34 {
		t.Errorf("reducao de area = %d, queria 34", got)
	}
	if got := s.Avatar.Ability.get(efDamage1); got != 16 {
		t.Errorf("dano = %d, queria 16", got)
	}
	if got := s.Avatar.Ability.get(efCritical); got != 4 { // 2 de cada passiva
		t.Errorf("critico = %d, queria 4", got)
	}
	if got := s.Avatar.Ability.get(efRequireMP); got != 14 {
		t.Errorf("EF_REQUIRE_MP = %d, queria 14", got)
	}
}

func TestPassivesRefreshIsIdempotentAndUndoes(t *testing.T) {
	h, s := passiveSession(t,
		map[int]gamedata.T_SkillData{9: {Agressive: 1}},
		model.SkillFromList{Index: 9, Level: 2})
	h.refreshPassives(s)
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efDamage1); got != 4 {
		t.Fatalf("depois de duas leituras = %d, queria 4 (nao pode acumular)", got)
	}
	// Reset: o nivel cai a zero e tudo sai.
	s.Skills.Others[0].Level = 0
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efDamage1); got != 0 {
		t.Errorf("depois do reset = %d, queria 0", got)
	}
}

func TestPassivesIgnoreNonPassiveSkills(t *testing.T) {
	h, s := passiveSession(t, map[int]gamedata.T_SkillData{
		500: {Index: 9, Agressive: 0},               // nao agressiva
		501: {Index: 9, Agressive: 1, Attribute: 5}, // com atributo: skill ativa
	},
		model.SkillFromList{Index: 500, Level: 3},
		model.SkillFromList{Index: 501, Level: 3})
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efDamage1); got != 0 {
		t.Errorf("nao-passivas somaram %d", got)
	}
}

func TestLearningPassiveRecalculates(t *testing.T) {
	h, s := passiveSession(t, map[int]gamedata.T_SkillData{106: {Agressive: 1}},
		model.SkillFromList{Index: 106, Level: 1})
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efMP); got != 80 {
		t.Errorf("Harmonia de Mana nv1: MP = %d, queria 80", got)
	}
	s.Skills.Others[0].Level = 2
	h.refreshPassives(s)
	if got := s.Avatar.Ability.get(efMP); got != 160 {
		t.Errorf("Harmonia de Mana nv2: MP = %d, queria 160", got)
	}
	if got := s.Avatar.Ability.get(efRegenMP); got != 10 {
		t.Errorf("regeneracao de MP = %d, queria 10", got)
	}
}
