package game

import (
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// Codigos de pran: nivel/experiencia e devocao/comida.
const (
	CodePranLevelExp    = 0x116
	CodePranDevotionFed = 0x96B
)

// pranExpAt le a tabela PranExpList; false fora dela. A origem lia o vetor
// dinamico sem conferir o indice.
func (h *Handler) pranExpAt(i int) (uint32, bool) {
	if i < 0 || i >= len(h.PranExp) {
		return 0, false
	}
	return h.PranExp[i], true
}

// activePran devolve a pran invocada do jogador.
func activePran(s *Session) (int, bool) {
	if s.SpawnedPran < 0 || s.SpawnedPran >= len(s.Prans) || s.Prans[s.SpawnedPran] == nil {
		return 0, false
	}
	return s.SpawnedPran, true
}

// awardPranExp e o bloco de experiencia da pran de TBaseMob (apos o abate): a
// pran invocada ganha um terco da experiencia do dono, com tetos por fase.
//
//	nivel 0..3   fada: ate o nivel 4
//	nivel 4      precisa evoluir se for fada (61/71/81)
//	nivel 5..18  crianca: ate o nivel 19
//	nivel 19     precisa evoluir se for crianca (62/72/82)
//	nivel 20..48 adolescente: ate o nivel 49
//	nivel 49     precisa evoluir se for adolescente (63/73/83)
//	nivel 50..69 adulta: sem teto
func (h *Handler) awardPranExp(c *netsrv.Conn, s *Session, exp int64) {
	slot, ok := activePran(s)
	if !ok || exp <= 0 {
		return
	}
	p := s.Prans[slot]
	gain := uint32(min(exp/3, 0xFFFFFFFF))

	// capped diz o nivel de teto da fase: (indice que fecha a fase, indice do
	// nivel anterior, ultimo nivel que sobe por aqui).
	type fase struct{ fecha, anterior, ultimo int }
	var f *fase
	switch {
	case p.Level <= 3:
		f = &fase{5, 4, 3}
	case p.Level == 4, p.Level == 19, p.Level == 49:
		if p.ClassNeedsEvolution() {
			h.message(c, "A sua pran precisa evoluir para ganhar exp.")
			return
		}
	case p.Level >= 5 && p.Level <= 18:
		f = &fase{20, 19, 18}
	case p.Level >= 20 && p.Level <= 48:
		f = &fase{50, 49, 48}
	}
	if f == nil {
		h.addPranExp(c, s, slot, gain)
		return
	}
	teto, ok1 := h.pranExpAt(f.fecha)
	ant, ok2 := h.pranExpAt(f.anterior)
	if ok1 && ok2 && uint64(gain)+uint64(p.Exp) > uint64(teto) {
		p.Exp = ant
		for i := int(p.Level); i <= f.ultimo; i++ {
			h.addPranLevel(c, s, slot)
		}
		return
	}
	h.addPranExp(c, s, slot, gain)
}

// addPranExp e o porte de TPlayer.AddPranExp: soma, sobe de nivel enquanto a
// experiencia passa da linha do nivel seguinte e avisa o cliente ($116).
func (h *Handler) addPranExp(c *netsrv.Conn, s *Session, slot int, value uint32) {
	p := s.Prans[slot]
	if p == nil {
		return
	}
	if uint64(p.Exp)+uint64(value) > 0xFFFFFFFF {
		p.Exp = 0xFFFFFFFF
	} else {
		p.Exp += value
	}
	for p.DisplayLevel() < pranMaxLevel {
		need, ok := h.pranExpAt(p.DisplayLevel())
		if !ok || p.Exp <= need {
			break
		}
		h.addPranLevel(c, s, slot)
	}
	if p.DisplayLevel() == pranMaxLevel {
		if need, ok := h.pranExpAt(p.DisplayLevel()); ok && p.Exp > need {
			p.Exp = need
		}
	}
	h.sendPranLevelAndExp(c, p.DisplayLevel(), p.Exp)
}

const pranMaxLevel = 50 // MAX_PRAN_LEVEL

// addPranLevel e o porte de TPlayer.AddPranLevel: um nivel, mais HP e MP, vida
// cheia, e nos marcos (5, 10... 50) mais um nivel nas habilidades. As passivas
// saem antes e voltam depois, para valerem com o nivel novo.
func (h *Handler) addPranLevel(c *netsrv.Conn, s *Session, slot int) {
	p := s.Prans[slot]
	if p == nil || !p.LevelUp() {
		return
	}
	h.message(c, "Sua pran subiu de nível.")
	h.sendPranLevelAndExp(c, p.DisplayLevel(), p.Exp)
	ativa := s.SpawnedPran == slot
	if ativa {
		h.removePranEffects(s)
	}
	for _, b := range p.ApplyMilestone() {
		h.sendItemBarSlot(c, uint32(b.Bar), 3, uint32(b.NewIndex))
	}
	if ativa {
		h.applyPranEffects(s, p)
	}
	h.sendPranToWorld(c, s, slot)
}

// sendPranLevelAndExp manda o $116.
func (h *Handler) sendPranLevelAndExp(c *netsrv.Conn, level int, exp uint32) {
	if c == nil {
		return
	}
	var pkt packets.RefreshPranLevelExpPacket
	pkt.Header.Index = shopHeaderIndex
	pkt.Level = uint32(level)
	pkt.Exp = int64(exp)
	_ = c.SendStruct(CodePranLevelExp, pkt)
}

// feedPran e o porte de ITEM_TYPE_PRAN_FOOD: alimenta a pran invocada. Devolve
// se o item deve ser gasto (sem pran invocada ou pran cheia, nao).
func (h *Handler) feedPran(c *netsrv.Conn, s *Session, item uint16) bool {
	slot, ok := activePran(s)
	if !ok {
		return false
	}
	if !s.Prans[slot].Feed(item) {
		h.message(c, "Sua pran não consegue comer mais.")
		return false
	}
	h.sendPranToWorld(c, s, slot)
	return true
}

// digestPran e o porte de ITEM_TYPE_PRAN_DIGEST.
func (h *Handler) digestPran(c *netsrv.Conn, s *Session) bool {
	slot, ok := activePran(s)
	if !ok {
		return false
	}
	if !s.Prans[slot].Digest() {
		h.message(c, "Sua pran está com muita fome para usar o Digestivo.")
		return false
	}
	h.sendPranToWorld(c, s, slot)
	return true
}
