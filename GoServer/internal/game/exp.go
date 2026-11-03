package game

import (
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/stats"
)

// CodeCurrentLevel e o $108: nivel e experiencia do jogador.
const CodeCurrentLevel = 0x108

// Constantes de progressao, de GlobalDefs.pas.
const (
	LevelCap            = 90
	SkillPointPerLevel  = 1
	StatusPointPerLevel = 2
	// SkillPointX e StatusPointX sao o bonus extra dos niveis 51, 61, 71 e 81.
	SkillPointX  = 12
	StatusPointX = 10
	// ClassAdvanceLevel e o nivel em que ClassInfo avanca uma casa.
	ClassAdvanceLevel = 51
)

// Tipos de experiencia, as constantes EXP_TYPE_* da origem.
const (
	ExpTypeNormal = 0
	ExpTypeMob    = 1
	ExpTypeQuest  = 2
)

// ExpTable da a experiencia necessaria para passar de cada nivel.
type ExpTable interface {
	// ExpForLevel devolve o total de experiencia do nivel, e false fora da
	// faixa da tabela.
	ExpForLevel(level uint16) (uint64, bool)
}

// addExp soma experiencia e sobe de nivel enquanto der. Porte de
// TPlayer.AddExp com TPlayer.AddLevel.
//
// A origem testava "Self.Character.Base.Exp > ExpList[Level]" com ExpList
// sendo um vetor dinamico e Level um WORD: nivel acima do tamanho da tabela
// lia fora do vetor, e e exatamente o que acontece ao chegar no teto se a
// tabela do arquivo for menor que LEVEL_CAP. Aqui a tabela responde se o
// nivel existe.
func (h *Handler) addExp(c *netsrv.Conn, s *Session, valor int64, tipo int) int64 {
	if valor <= 0 {
		return 0
	}
	ganho := valor

	if tipo == ExpTypeMob {
		mult := int64(h.ExpMultiplier)
		if mult <= 0 {
			// EXP_MULTIPLIER vale 1 por omissao em GlobalDefs.pas; zero no
			// .ini faria todo mob dar zero de experiencia.
			mult = 1
		}
		ganho *= mult
	}

	antes := s.Avatar.Level
	s.Avatar.Exp += ganho
	h.applyLevelUps(s)

	if s.Avatar.Level != antes {
		h.log().Info("nivel novo",
			"usuario", s.Username, "personagem", s.Avatar.Name,
			"de", antes, "para", s.Avatar.Level)
		// Subir de nivel muda os maximos e os atributos derivados.
		s.Avatar.Recalc(stats.Bonus{})
		s.Avatar.CurHP = s.Avatar.MaxHP
		s.Avatar.CurMP = s.Avatar.MaxMP
		h.sendStatus(c, s)
	}
	h.sendRefreshLevel(c, s)
	return ganho
}

// applyLevelUps sobe de nivel enquanto a experiencia der.
func (h *Handler) applyLevelUps(s *Session) {
	if h.Exp == nil {
		return
	}
	for s.Avatar.Level < LevelCap {
		necessario, ok := h.Exp.ExpForLevel(s.Avatar.Level)
		if !ok {
			// Sem entrada na tabela nao ha como decidir: parar aqui e melhor
			// que ler fora do vetor, como a origem fazia.
			return
		}
		if s.Avatar.Exp <= int64(necessario) {
			return
		}
		h.addLevel(s)
	}
	// No teto, a experiencia para no valor do ultimo nivel.
	if s.Avatar.Level == LevelCap {
		if necessario, ok := h.Exp.ExpForLevel(s.Avatar.Level); ok {
			if s.Avatar.Exp > int64(necessario) {
				s.Avatar.Exp = int64(necessario)
			}
		}
	}
}

// addLevel sobe um nivel. Porte de TPlayer.AddLevel.
func (h *Handler) addLevel(s *Session) {
	a := &s.Avatar
	if a.Level >= LevelCap {
		return
	}
	// A classe avanca uma casa ao passar do nivel 50.
	if a.Level <= 50 && a.Level+1 >= ClassAdvanceLevel {
		a.ClassInfo++
	}
	a.Level++
	a.SkillPoint = addPoints(a.SkillPoint, SkillPointPerLevel)

	if a.Level > 50 {
		a.StatusPoints = addPoints(a.StatusPoints, StatusPointPerLevel)
		switch a.Level {
		case 51, 61, 71, 81:
			a.SkillPoint = addPoints(a.SkillPoint, SkillPointX)
			a.StatusPoints = addPoints(a.StatusPoints, StatusPointX)
		}
	}
}

// addPoints soma sem dar a volta no WORD. A origem usava Inc, que num WORD
// com 65535 pontos voltava a zero.
func addPoints(x uint16, v int) uint16 {
	soma := int(x) + v
	if soma > 65535 {
		return 65535
	}
	if soma < 0 {
		return 0
	}
	return uint16(soma)
}

// sendRefreshLevel manda o $108. Porte de TBaseMob.SendRefreshLevel.
func (h *Handler) sendRefreshLevel(c *netsrv.Conn, s *Session) {
	var pkt packets.SendCurrentLevel
	pkt.Header.Index = s.ClientID
	// "Character.Level - 1" num WORD: com nivel 0 a origem mandava 65535.
	if s.Avatar.Level > 0 {
		pkt.Level = s.Avatar.Level - 1
	}
	pkt.Unk = 0xCC
	pkt.Exp = uint64(max(s.Avatar.Exp, 0))
	if err := c.SendStruct(CodeCurrentLevel, pkt); err != nil {
		h.log().Debug("falha ao enviar nivel", "usuario", s.Username, "erro", err)
	}
}
