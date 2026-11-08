package game

import (
	"aika/internal/pran"
	"aika/internal/skillfn"
)

// efv e um efeito aplicado ao acumulador do dono: EF e valor.
type efv struct{ ef, v int }

// Efeitos de habilidade passiva da pran. A familia e o campo Index da linha de
// SkillData em (Index da habilidade + nivel - 1).
const (
	passBravura      = 193 // ataque em todos os sentidos 5+2*lv
	passDefElemental = 194 // defesa em todos os sentidos 2*lv
	passCoragemHP    = 197 // HP 23+(13+lv)*lv
	passLaminaFlame  = 199 // critico +lv
	passCoragem3     = 217 // ataque 3+2*lv
	passProtecao218  = 218 // defesa 2*lv
	passSabedoria    = 221 // MP 30+20*lv
	passToqueAssomb  = 223 // resistencia 6 +lv
	passCoragem4     = 241 // ataque 4+2*lv
	passProtecao242  = 242 // defesa 2*lv
	passSerenidade   = 245 // HP 15+(8+lv)*lv e MP 21+(11+lv)*lv
	passVentoClemen  = 247 // idem serenidade
)

// passiveEffects devolve os efeitos de uma habilidade passiva de familia fam e
// nivel lv. Porte do case de TPlayer.SetPranPassiveSkill; familia sem efeito
// devolve nil.
func passiveEffects(fam, lv int) []efv {
	switch fam {
	case passBravura:
		v := 5 + 2*lv
		return []efv{{efPranDamage1, v}, {efPranDamage2, v}}
	case passDefElemental, passProtecao218, passProtecao242:
		v := 2 * lv
		return []efv{{efPranResist1, v}, {efPranResist2, v}}
	case passCoragemHP:
		return []efv{{efHP, 23 + (13+lv)*lv}}
	case passLaminaFlame:
		return []efv{{efCritical, lv}}
	case passCoragem3:
		v := 3 + 2*lv
		return []efv{{efPranDamage1, v}, {efPranDamage2, v}}
	case passSabedoria:
		return []efv{{efMP, 30 + 20*lv}}
	case passToqueAssomb:
		return []efv{{efResistance6, lv}}
	case passCoragem4:
		v := 4 + 2*lv
		return []efv{{efPranDamage1, v}, {efPranDamage2, v}}
	case passSerenidade, passVentoClemen:
		return []efv{{efHP, 15 + (8+lv)*lv}, {efMP, 21 + (11+lv)*lv}}
	}
	return nil
}

// itemTypeSkin e o ItemType 10 (a propria pran), que nao da efeito.
const itemTypeSkin = 10

// pranEffects soma os efeitos da pran: passivas das habilidades e equipamento.
//
// Equipamento (SetEquipEffect da origem): cada um dos 3 efeitos gravados no item
// vale o dobro e os 3 primeiros EF/EFV da tabela entram como estao; item de
// ItemType 10 nao conta. Habilidade: passiveEffects.
//
// Corrigido: a desativacao da segunda pran na origem misturava o nivel das
// habilidades da primeira (casos 197 e 247); aqui o que se desfaz e exatamente
// o que foi somado.
func (h *Handler) pranEffects(p *pran.Pran) []efv {
	var out []efv
	for _, sk := range p.Skills {
		if sk.Level == 0 {
			continue
		}
		d := skillfn.Data(h.SkillTable, uint32(sk.Index)+uint32(sk.Level)-1)
		if d == nil {
			continue
		}
		out = append(out, passiveEffects(int(d.Index), int(sk.Level))...)
	}
	for _, it := range p.Equip {
		if it.Index == 0 {
			continue
		}
		info := h.data().Item(int(it.Index))
		if info == nil || info.ItemType == itemTypeSkin {
			continue
		}
		for i := 0; i < 3; i++ {
			if it.Effects.Index[i] > 0 {
				out = append(out, efv{int(it.Effects.Index[i]), int(it.Effects.Value[i]) * 2})
			}
			if info.EF[i] > 0 {
				out = append(out, efv{int(info.EF[i]), int(info.EFV[i])})
			}
		}
	}
	return out
}

// applyPranEffects liga os efeitos da pran ao dono. Guarda o que somou para o
// desligar igual, mesmo que a pran mude de nivel ou de equipamento depois.
func (h *Handler) applyPranEffects(s *Session, p *pran.Pran) {
	h.removePranEffects(s)
	s.pranApplied = h.pranEffects(p)
	for _, e := range s.pranApplied {
		s.Avatar.Ability.increase(e.ef, e.v)
	}
	h.recalcAfterPran(s)
}

// removePranEffects desfaz o que applyPranEffects somou.
func (h *Handler) removePranEffects(s *Session) {
	if len(s.pranApplied) == 0 {
		return
	}
	for _, e := range s.pranApplied {
		s.Avatar.Ability.decrease(e.ef, e.v)
	}
	s.pranApplied = nil
	h.recalcAfterPran(s)
}

// recalcAfterPran refaz os atributos e reenvia pontos e status ao dono.
func (h *Handler) recalcAfterPran(s *Session) {
	s.Avatar.ApplyEquipBonus(h.data(), s.Equip)
	if c := h.connOf(s); c != nil {
		h.sendRefreshPoint(c, s)
		h.sendStatus(c, s)
	}
}
