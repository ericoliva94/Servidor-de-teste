package game

import (
	"context"
	"fmt"

	"aika/internal/guild"
	"aika/internal/nation"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de nacao.
const (
	CodeUpdateNationTaxes = 0xE3A // cliente -> servidor: marechal muda as taxas
	CodeNationGold        = 0xF34 // ida e volta: tesouro da nacao
	CodeNationGrade       = 0x936 // servidor -> cliente: guildas de grade e taxas
	CodeNationSiege       = 0x91A // servidor -> cliente: cerco
	CodeNationAlly        = 0x967 // servidor -> cliente: nacao aliada
)

// Buffs de grade da nacao (GlobalDefs.pas e Player.pas).
const (
	buffMarshal      = 6600
	buffMarshalGuild = 6601
	buffArchon       = 6602 // ARCHON_BUFF
	buffArchonGuild  = 6648 // CAVALEIROS_ARCHON
)

// NationStore guarda a tabela nations.
type NationStore interface {
	Load(ctx context.Context) ([]nation.State, error)
	SaveTaxes(ctx context.Context, st nation.State) error
	SaveGrade(ctx context.Context, st nation.State) error
}

// LoadNations carrega as tres nacoes do banco. Sem banco ficam vazias.
func (h *Handler) LoadNations(ctx context.Context) error {
	h.loadSiege(ctx)
	if h.Nations == nil || h.NationDB == nil {
		return nil
	}
	rows, err := h.NationDB.Load(ctx)
	if err != nil {
		return err
	}
	for _, st := range rows {
		h.Nations.Put(st)
	}
	return nil
}

// channelNation e a nacao deste canal (0 se o servidor nao tem nacao).
func (h *Handler) channelNation() uint8 {
	if h.Devir != nil {
		return h.Devir.Nation
	}
	return 0
}

// nationOf e a nacao do jogador; quem nao tem nacao usa a do canal (como a
// origem em SendNationInformation, IsArchon e IsGradeMarshal).
func (h *Handler) nationOf(s *Session) uint8 {
	if s.Avatar.Nation != 0 {
		return uint8(min(s.Avatar.Nation, 255))
	}
	return h.channelNation()
}

func (h *Handler) guildOfSession(s *Session) *guild.Guild {
	if h.Guilds == nil {
		return nil
	}
	return h.Guilds.Of(s.CharID())
}

// isMarshal e o porte de TPlayer.IsMarshal: lider da guilda marechal da nacao
// da conta (aqui sem o recurso do canal, como a origem).
func (h *Handler) isMarshal(s *Session) bool {
	if h.Nations == nil || s.Avatar.Nation == 0 {
		return false
	}
	st, ok := h.Nations.Get(uint8(min(s.Avatar.Nation, 255)))
	if !ok || st.MarshalGuild == 0 {
		return false
	}
	g := h.guildOfSession(s)
	return g != nil && g.Index == st.MarshalGuild && g.LeaderID == s.CharID()
}

// archonGuild diz se g e uma das tres guildas archon (estrategista, juiz,
// tesoureiro).
func archonGuild(st nation.State, g *guild.Guild) bool {
	return g != nil && g.Index != 0 &&
		(g.Index == st.TacticianGuild || g.Index == st.JudgeGuild || g.Index == st.TreasurerGuild)
}

// updateNationGold e o porte de TPacketHandlers.UpdateNationGold ($F34): o
// cliente pede e recebe o tesouro da propria nacao (0 sem nacao).
func (h *Handler) updateNationGold(c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var req packets.UpdateNationTreasure
	if err := netsrv.Unmarshal(p, &req); err != nil {
		return err
	}
	var out packets.UpdateNationTreasure
	out.Header.Index = s.ClientID
	if st, ok := h.nationState(uint8(min(s.Avatar.Nation, 255))); ok {
		out.Gold = st.Gold
	}
	return c.SendStruct(CodeNationGold, out)
}

func (h *Handler) nationState(id uint8) (nation.State, bool) {
	if h.Nations == nil {
		return nation.State{}, false
	}
	return h.Nations.Get(id)
}

// updateNationTaxes e o porte de TPacketHandlers.UpdateNationTaxes ($E3A): so o
// marechal muda as taxas (5..15 cidadao, 10..20 visitante). Salva e avisa todos
// os jogadores da nacao.
//
// Corrigido: a origem convertia a taxa para BYTE antes de checar (256+10 virava
// 10); aqui vale o valor inteiro do pacote.
func (h *Handler) updateNationTaxes(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.UpdateNationTaxes
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !h.isMarshal(s) {
		return nil // a origem ignora em silencio
	}
	id := uint8(min(s.Avatar.Nation, 255))
	st, ok := h.Nations.SetTaxes(id, pkt.CitizenTax, pkt.VisitorTax)
	if !ok {
		return nil
	}
	if h.NationDB != nil {
		if err := h.NationDB.SaveTaxes(ctx, st); err != nil {
			h.log().Error("gravar taxas da nacao", "nacao", id, "erro", err)
		}
	}
	msg := fmt.Sprintf("A taxa de sua nação foi setada em %d%% para cidadãos e %d%% para não-cidadãos.",
		st.CitizenTax, st.VisitorTax)
	for _, cid := range h.Chan.PlayingIDs() {
		o, conn, ok := h.Chan.Lookup(cid)
		if !ok || o.Status < Playing || o.Avatar.Nation != s.Avatar.Nation {
			continue
		}
		h.message(conn, msg)
		h.sendNationInformation(conn, o)
	}
	return nil
}

// sendNationInformation e o porte de TPlayer.SendNationInformation: grade,
// cerco, nacao aliada e tesouro, e os buffs de marechal e archon.
func (h *Handler) sendNationInformation(c *netsrv.Conn, s *Session) {
	if c == nil || h.Nations == nil {
		return
	}
	st, ok := h.Nations.Get(h.nationOf(s))
	if !ok {
		return
	}
	name := func(index uint16) string {
		if index == 0 || h.Guilds == nil {
			return ""
		}
		if g := h.Guilds.Get(index); g != nil {
			return g.Name
		}
		return ""
	}

	var grade packets.GuildsGradePacket
	grade.Header.Index = s.ClientID
	netsrv.PutCString(grade.Guilds.LordMarechal[:], name(st.MarshalGuild))
	netsrv.PutCString(grade.Guilds.Estrategista[:], name(st.TacticianGuild))
	netsrv.PutCString(grade.Guilds.Juiz[:], name(st.JudgeGuild))
	netsrv.PutCString(grade.Guilds.Tesoureiro[:], name(st.TreasurerGuild))
	grade.GuildsId = [4]uint16{st.MarshalGuild, st.TacticianGuild, st.JudgeGuild, st.TreasurerGuild}
	grade.Nation = uint32(st.ID)
	grade.RegisterBonus = int64(st.Settlement)
	grade.CitizenTax = uint8(min(st.CitizenTax, 255))
	grade.NoCitizenTax = uint8(min(st.VisitorTax, 255))
	grade.NationAlly = 0 // a aba de nacao aliada nao funciona no cliente (comentario da origem)
	grade.AllyanceDate = st.AllyDate
	netsrv.PutCString(grade.MarshalAlly[:], st.MarshalAllyName)
	grade.RankNation = st.Rank
	grade.Estabilization = 100

	var siege packets.GuildsSiegePacket
	siege.Header.Index = s.ClientID
	siege.Defensoras = grade.Guilds

	var ally packets.AllyanceInfoPacket
	ally.Header.Index = s.ClientID
	ally.Nation = uint16(h.channelNation())
	ally.NationAlly = uint16(max(st.AllyNation, 0))
	ally.AllyDate = st.AllyDate
	netsrv.PutCString(ally.MarshalAlly[:], st.MarshalAllyName)

	var gold packets.UpdateNationTreasure
	gold.Header.Index = s.ClientID
	gold.Gold = st.Gold

	_ = c.SendStruct(CodeNationGrade, grade)
	_ = c.SendStruct(CodeNationSiege, siege)
	_ = c.SendStruct(CodeNationAlly, ally)
	_ = c.SendStruct(CodeNationGold, gold)
	h.syncNationBuffs(s, st)
}

// syncNationBuffs poe e tira os buffs de grade conforme o cargo do jogador.
func (h *Handler) syncNationBuffs(s *Session, st nation.State) {
	g := h.guildOfSession(s)
	marshal := h.isMarshal(s)
	inMarshalGuild := g != nil && st.MarshalGuild != 0 && g.Index == st.MarshalGuild
	archon := archonGuild(st, g) && g.LeaderID == s.CharID()
	inArchonGuild := archonGuild(st, g)

	h.setBuff(s, buffMarshal, marshal)
	h.setBuff(s, buffMarshalGuild, inMarshalGuild && !marshal)
	h.setBuff(s, buffArchon, archon)
	h.setBuff(s, buffArchonGuild, inArchonGuild && !archon)
}

// setBuff garante o buff ligado ou desligado.
func (h *Handler) setBuff(s *Session, skill uint16, on bool) {
	has := s.Base.Buffs != nil && s.Base.Buffs.Has(skill)
	switch {
	case on && !has:
		h.addBuff(s, skill)
	case !on && has:
		h.removeBuffAndNotify(s, skill)
	}
}
