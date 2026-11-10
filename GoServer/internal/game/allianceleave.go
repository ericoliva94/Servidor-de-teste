package game

import (
	"context"
	"errors"

	"aika/internal/guild"
	"aika/internal/nation"
)

// Mensagens de recusa das opcoes de alianca do NPC.
const (
	msgAllyNotLeader = "Apenas o líder da guilda pode fazer isso."
	msgAllyNone      = "Sua guilda não está em uma aliança."
	msgAllyInWar     = "Você não pode alterar a aliança estando cadastrado na guerra."
	msgAllyLeaderOut = "Você não pode sair de uma aliança sendo o líder dela."
)

// destroyAlliance porta TNPCHandlers.DestroyAlliance: o lider da aliança a
// desfaz e cada guilda volta a ficar sozinha.
//
// Recusa se a guilda esta cadastrada na guerra como lider (atacante ou defesa).
//
// Ainda nao alcancavel por pacote: a origem chama isto de uma opcao de dialogo
// de NPC (NPCHandlers), e os NPCs dependem de arquivos de dados que nao estao
// no repositorio.
func (h *Handler) destroyAlliance(ctx context.Context, s *Session) bool {
	c := h.connOf(s)
	g := h.guildOfSession(s)
	if g == nil || g.LeaderID != s.CharID() {
		h.message(c, msgAllyNotLeader)
		return false
	}
	if h.inWarAsLeader(g) {
		h.message(c, msgAllyInWar)
		return false
	}
	changed, err := h.Guilds.DisbandAlliance(g)
	if err != nil {
		h.message(c, h.allianceErr(err))
		return false
	}
	h.finishAllianceChange(ctx, s, changed)
	return true
}

// kickAlly porta TNPCHandlers.RemoveMemberAlliance: o lider tira a aliada do
// slot (1 a 3). A guilda retirada perde o cargo de estrategista, juiz ou
// tesoureiro da nacao, se tinha.
func (h *Handler) kickAlly(ctx context.Context, s *Session, slot int) bool {
	c := h.connOf(s)
	g := h.guildOfSession(s)
	if g == nil || g.LeaderID != s.CharID() {
		h.message(c, msgAllyNotLeader)
		return false
	}
	if h.inWarAsLeader(g) {
		h.message(c, msgAllyInWar)
		return false
	}
	removed, changed, err := h.Guilds.KickAlly(g, slot)
	if err != nil {
		h.message(c, h.allianceErr(err))
		return false
	}
	h.clearGradeOf(ctx, s, removed.Index)
	h.finishAllianceChange(ctx, s, changed)
	return true
}

// exitAlliance porta TNPCHandlers.ExitAlliance: a aliada sai. Perde o cargo de
// nacao, se tinha, e some da lista das outras.
func (h *Handler) exitAlliance(ctx context.Context, s *Session) bool {
	c := h.connOf(s)
	g := h.guildOfSession(s)
	if g == nil || g.LeaderID != s.CharID() {
		h.message(c, msgAllyNotLeader)
		return false
	}
	if h.inWar(g) {
		h.message(c, msgAllyInWar)
		return false
	}
	changed, err := h.Guilds.LeaveAlliance(g)
	if err != nil {
		h.message(c, h.allianceErr(err))
		return false
	}
	h.clearGradeOf(ctx, s, g.Index)
	h.finishAllianceChange(ctx, s, changed)
	return true
}

func (h *Handler) allianceErr(err error) string {
	switch {
	case errors.Is(err, guild.ErrNotAllied):
		return msgAllyNone
	case errors.Is(err, guild.ErrIsAllyLeader):
		return msgAllyLeaderOut
	case errors.Is(err, guild.ErrNotAllyLeader):
		return msgAllyNotLeader
	}
	return "Não foi possível alterar a aliança."
}

// clearGradeOf tira a guilda dos cargos de estrategista, juiz e tesoureiro da
// nacao de s e grava. Na origem era "Cerco.Defensoras.* = nome da guilda".
func (h *Handler) clearGradeOf(ctx context.Context, s *Session, guildIndex uint16) {
	if h.Nations == nil || guildIndex == 0 {
		return
	}
	id := h.nationOf(s)
	mudou := false
	st, ok := h.Nations.Update(id, func(n *nation.State) {
		if n.TacticianGuild == guildIndex {
			n.TacticianGuild, mudou = 0, true
		}
		if n.JudgeGuild == guildIndex {
			n.JudgeGuild, mudou = 0, true
		}
		if n.TreasurerGuild == guildIndex {
			n.TreasurerGuild, mudou = 0, true
		}
	})
	if !ok || !mudou {
		return
	}
	if h.NationDB != nil {
		if err := h.NationDB.SaveGrade(ctx, st); err != nil {
			h.log().Error("gravar grade da nacao", "nacao", id, "erro", err)
		}
	}
	// A nacao mudou: todo mundo em jogo recebe de novo.
	for _, pid := range h.Chan.PlayingIDs() {
		if o, conn, ok := h.Chan.Lookup(pid); ok {
			h.sendNationInformation(conn, o)
		}
	}
}

// finishAllianceChange avisa quem esta em jogo nas guildas que mudaram e grava.
func (h *Handler) finishAllianceChange(ctx context.Context, s *Session, changed []*guild.Guild) {
	for _, g := range changed {
		for i := range g.Members {
			m := &g.Members[i]
			if m.CharIndex == 0 || !m.Logged {
				continue
			}
			o, conn, ok := h.sessionByCharID(m.CharIndex)
			if !ok {
				continue
			}
			h.sendGuildInfo(conn, o, g)
			h.message(conn, "Sua aliança teve mudanças.")
			h.sendNationInformation(conn, o)
		}
		if h.GuildDB != nil {
			if err := h.GuildDB.Save(ctx, g); err != nil {
				h.log().Error("gravar guilda da aliança", "guilda", g.Index, "erro", err)
			}
		}
	}
}

// inWarAsLeader diz se a guilda lidera uma alianca cadastrada na guerra, como
// atacante ou como defesa (LordMarechal do cerco).
func (h *Handler) inWarAsLeader(g *guild.Guild) bool {
	if h.Siege == nil {
		return false
	}
	c := h.Siege.Cerco()
	if c.Defenders.Marshal != "" && c.Defenders.Marshal == g.Name {
		return true
	}
	for _, a := range c.Attackers {
		if a.Marshal != "" && a.Marshal == g.Name {
			return true
		}
	}
	return false
}

// inWar diz se a guilda esta no cadastro como atacante (em qualquer cargo) ou
// como marechal da defesa: quem esta assim nao sai da alianca.
func (h *Handler) inWar(g *guild.Guild) bool {
	if h.Siege == nil {
		return false
	}
	c := h.Siege.Cerco()
	if c.Defenders.Marshal != "" && c.Defenders.Marshal == g.Name {
		return true
	}
	for _, a := range c.Attackers {
		if a.Has(g.Name) {
			return true
		}
	}
	return false
}
