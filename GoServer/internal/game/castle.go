package game

import (
	"context"
	"fmt"
	"time"

	"aika/internal/guild"
	"aika/internal/model"
	"aika/internal/nation"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/siege"
)

// Ids dos objetos do castelo em CollectItem: as tres orbes e a bandeira do
// marechal (o selo).
const (
	castleOrbWater = 3370
	castleOrbWind  = 3371
	castleOrbFire  = 3372
	castleSealFlag = 3373
)

// castleCancelSignal e o dado do sinal $33A que cancela a coleta de quem perdeu
// a orbe ou o selo (SendSignal($7535, $33A)).
const castleCancelSignal = 0x7535

// castleExitDefault e para onde sai quem nao tem posicao salva (3450, 690).
var castleExitDefault = model.Position{X: 3450, Y: 690}

// Dia e hora de a guerra poder comecar (sabado, 20h). A origem dizia "so pode
// se iniciar as 20h todos os sabados" mas recusava justamente no sabado depois
// das 20h (DayOfWeek = 7 and HourOf >= 20): a condicao estava invertida, e o
// resultado era a guerra comecar em qualquer horario menos o certo. Aqui vale
// a mensagem. SiegeAnyTime desliga a regra (servidores de teste).
const (
	siegeStartWeekday = time.Saturday
	siegeStartHour    = 20
)

// SiegeStore guarda o cadastro do cerco de cada nacao. Nil deixa so em memoria.
type SiegeStore interface {
	Load(ctx context.Context, nationID uint8) (siege.Cerco, error)
	Save(ctx context.Context, nationID uint8, c siege.Cerco) error
}

// SiegeState e o estado do cerco de uma sessao no castelo.
type SiegeState struct {
	InCastle    bool
	SavedPos    model.Position // de onde veio, para voltar
	HasSavedPos bool
	// CastleNation e a "nacao" com que o cliente pinta o jogador no castelo
	// (NationForCastle: 6 defesa, 7 a 10 os atacantes).
	CastleNation uint8
}

// loadSiege carrega o cadastro do cerco da nacao do canal.
func (h *Handler) loadSiege(ctx context.Context) {
	if h.Siege == nil || h.SiegeDB == nil {
		return
	}
	id := h.channelNation()
	if id == 0 {
		return
	}
	c, err := h.SiegeDB.Load(ctx, id)
	if err != nil {
		h.log().Error("carregar cerco", "nacao", id, "erro", err)
		return
	}
	h.Siege.SetCerco(c)
}

func (h *Handler) saveSiege() {
	if h.Siege == nil || h.SiegeDB == nil {
		return
	}
	id := h.channelNation()
	if id == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := h.SiegeDB.Save(ctx, id, h.Siege.Cerco()); err != nil {
		h.log().Error("gravar cerco", "nacao", id, "erro", err)
	}
}

// allianceNames monta o cadastro de uma alianca de guildas: a lider primeiro e
// as aliadas nos cargos de estrategista, juiz e tesoureiro (Ally.Guilds[0..3]).
func (h *Handler) allianceNames(g *guild.Guild) siege.Alliance {
	var a siege.Alliance
	slots := [4]*string{&a.Marshal, &a.Tactician, &a.Judge, &a.Treasurer}
	for i, it := range g.Ally.Guilds {
		if it.Index == 0 {
			continue
		}
		if o := h.Guilds.Get(it.Index); o != nil {
			*slots[i] = o.Name
		}
	}
	return a
}

// signInCastle porta TNPCHandlers.SignInCastle: o lider da alianca cadastra a
// alianca como atacante. De segunda a sexta, sem guerra em andamento. Ainda sem
// caminho por pacote: a origem chama de uma opcao de NPC.
func (h *Handler) signInCastle(s *Session) bool {
	c := h.connOf(s)
	g := h.guildOfSession(s)
	if h.Siege == nil || g == nil {
		h.message(c, "Você tem que estar em uma legião para se cadastrar no castelo.")
		return false
	}
	if g.LeaderID != s.CharID() {
		h.message(c, "Você deve ser o lider da legião para cadastrar na guerra.")
		return false
	}
	if g.Ally.Leader != g.Index {
		h.message(c, "Você deve ser o lider da aliança para cadastrar na guerra.")
		return false
	}
	if d := h.now().Weekday(); d == time.Saturday || d == time.Sunday {
		h.message(c, "Você não pode se cadastrar agora.")
		return false
	}
	if _, err := h.Siege.RegisterAttacker(h.allianceNames(g)); err != nil {
		switch err {
		case siege.ErrRegistered:
			h.message(c, "Você já está com a sua aliança cadastrada na guerra.")
		case siege.ErrIsDefender:
			h.message(c, "Sua aliança já está cadastrada como defesa.")
		case siege.ErrNoSlot:
			h.message(c, "Não existe mais espaço para novas alianças atacantes.")
		case siege.ErrWarRunning:
			h.message(c, "Você não pode se registrar com a guerra em andamento.")
		}
		return false
	}
	h.saveSiege()
	h.broadcastToNation(s.Avatar.Nation, "O cerco de guerra do castelo teve alterações.")
	h.resendNationToAll()
	return true
}

// enterCastle porta TNPCHandlers.EnterInCastle: quem esta no cadastro entra
// pelo ponto do seu lado. O lider da alianca lider abre a guerra se ela ainda
// nao comecou. Sem guerra aberta o jogador nao entra.
func (h *Handler) enterCastle(s *Session) bool {
	c := h.connOf(s)
	g := h.guildOfSession(s)
	if h.Siege == nil || g == nil {
		h.message(c, "Você tem que estar em uma legião para entrar no castelo.")
		return false
	}
	if s.Avatar.Nation == 0 {
		h.message(c, "Você tem que estar em uma nação para entrar no castelo.")
		return false
	}
	entry, err := h.Siege.Entry(g.Name)
	if err != nil {
		h.message(c, "Você tem que estar registrado na guerra para entrar no castelo.")
		return false
	}
	if g.LeaderID == s.CharID() && g.Index == g.Ally.Leader && !h.Siege.InProgress() {
		if !h.SiegeAnyTime {
			now := h.now()
			if now.Weekday() != siegeStartWeekday || now.Hour() < siegeStartHour {
				h.message(c, "A guerra só pode se iniciar as 20h todos os sábados.")
				return false
			}
		}
		if h.Siege.Start(h.now()) {
			h.broadcastServerMessage("A guerra de castelo começou. Preparem-se...", 16)
		}
	}
	if !h.Siege.InProgress() {
		h.message(c, "A guerra ainda não começou.")
		return false
	}
	s.Siege.SavedPos, s.Siege.HasSavedPos = s.Position, true
	s.Siege.InCastle, s.Siege.CastleNation = true, entry.Nation
	h.teleportTo(s, model.Position{X: entry.X, Y: entry.Y})
	return true
}

// castleCollect trata a coleta de uma orbe ou da bandeira do marechal
// ($336 com os ids 3370 a 3373). Devolve true se o id era do castelo.
func (h *Handler) castleCollect(c *netsrv.Conn, s *Session, id uint32) bool {
	if id < castleOrbWater || id > castleSealFlag {
		return false
	}
	if h.Siege == nil || !h.Siege.InProgress() {
		return true
	}
	// Muito longe: a origem media contra a posicao do objeto.
	var alvo [2]float32
	var tol float32
	if id == castleSealFlag {
		alvo, tol = siege.SealPos, siege.SealRange
	} else {
		alvo, tol = siege.OrbPos[id-castleOrbWater], siege.OrbRange
	}
	if !s.Position.InRange(model.Position{X: alvo[0], Y: alvo[1]}, uint16(tol)+2) {
		h.message(c, "Você está muito longe da orbe.")
		return true
	}
	if s.Avatar.Nation == 0 {
		h.message(c, "Você não pode abrir templos. Registre-se em uma nação primeiro!")
		return true
	}
	if id != castleSealFlag {
		if h.Siege.HoldOrb(int(id-castleOrbWater), s.ClientID) {
			h.sendCastleHold(c, s, uint32(id), 0xFFFFFFFF)
		}
		return true
	}
	if h.Siege.OrbsHeld() < siege.Orbs {
		h.message(c, "As orbes precisam ser ocupadas antes de iniciar o selo")
		return true
	}
	g := h.guildOfSession(s)
	if g == nil || g.Index != g.Ally.Leader || g.LeaderID != s.CharID() {
		h.message(c, "Somente lider da aliança pode pegar a bandeira.")
		return true
	}
	if h.Siege.BeginSeal(s.ClientID, h.allianceNames(g), h.now()) {
		h.sendCastleHold(c, s, castleSealFlag, uint32(siege.SealDuration.Seconds()))
	}
	return true
}

// sendCastleHold manda o $336 que mostra a barra de coleta da orbe ou do selo.
func (h *Handler) sendCastleHold(c *netsrv.Conn, s *Session, id, tempo uint32) {
	if c == nil {
		return
	}
	pkt := packets.CollectItem{Index: id, Time: tempo, Null: 5}
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(CodeCollectMapItem, pkt)
}

// siegeHolderBroken diz se quem segura uma orbe ou o selo deve soltar: saiu,
// morreu ou se afastou. A origem tambem soltava quem levou skill do castelo nos
// ultimos 3 s; sem skills de castelo isso fica de fora.
func (h *Handler) siegeHolderBroken(id uint16, alvo [2]float32, tol float32) bool {
	s, _, ok := h.Chan.Lookup(id)
	if !ok || s.Status < Playing || s.Dead {
		return true
	}
	return !s.Position.InRange(model.Position{X: alvo[0], Y: alvo[1]}, uint16(tol))
}

// SiegeTick porta o laco de TCastleSiegeThread (CheckCastleOrbs,
// CountOrbsHolding, CheckMarshallSeal e UpdateSiegeStatus). O chamador roda uma
// vez por segundo. Devolve verdadeiro quando a guerra acabou nesta passada.
func (h *Handler) SiegeTick() bool {
	if h.Siege == nil || h.Chan == nil || !h.Siege.InProgress() {
		return false
	}
	for i := 0; i < siege.Orbs; i++ {
		id := h.Siege.OrbHolder(i)
		if id != 0 && h.siegeHolderBroken(id, siege.OrbPos[i], siege.OrbRange) {
			h.Siege.ReleaseOrb(i)
			h.cancelHold(id)
		}
	}
	if id := h.Siege.SealHolder(); id != 0 {
		if h.siegeHolderBroken(id, siege.SealPos, siege.SealRange) {
			h.Siege.ReleaseSeal()
			h.cancelHold(id)
		} else if left := h.Siege.SealRemaining(h.now()); left >= 0 {
			h.broadcastServerMessage(fmt.Sprintf("Selo do Marechal: %d seg. restantes.", left), 32)
		}
	}
	out := h.Siege.Tick(h.now())
	switch out.Kind {
	case siege.NoWinner:
		h.endWar("A guerra acabou e não há vencedores.")
		h.broadcastServerMessage("A guerra de castelo foi finalizada. A liderança atual conseguiu sustentar seu poder.", 16)
	case siege.AttackersWin:
		h.crownWinner(out.Winner)
		h.endWar("A guerra acabou. Os vencedores foram definidos.")
	default:
		return false
	}
	h.saveSiege()
	h.resendNationToAll()
	return true
}

// cancelHold manda o $33A a quem perdeu a orbe ou o selo.
func (h *Handler) cancelHold(id uint16) {
	if s, c, ok := h.Chan.Lookup(id); ok && c != nil {
		pkt := packets.SignalData{Data: castleCancelSignal}
		pkt.Header.Index = s.ClientID
		_ = c.SendStruct(CodeCancelCollect, pkt)
	}
}

// endWar tira do castelo quem estava la, com a mensagem, e o devolve de onde
// veio (ou a (3450, 690)).
//
// A origem teleportava todo membro logado de toda guilda do cadastro, onde
// quer que estivesse (inclusive quem nunca entrou); aqui so sai quem esta no
// castelo.
func (h *Handler) endWar(msg string) {
	for _, id := range h.Chan.PlayingIDs() {
		s, c, ok := h.Chan.Lookup(id)
		if !ok || !s.Siege.InCastle {
			continue
		}
		h.message(c, msg)
		dest := castleExitDefault
		if s.Siege.HasSavedPos && ValidPosition(s.Siege.SavedPos) {
			dest = s.Siege.SavedPos
		}
		s.Siege = SiegeState{}
		h.teleportTo(s, dest)
	}
}

// crownWinner poe a alianca vencedora no comando da nacao: a guilda do selo e a
// marechal; as aliadas viram estrategista, juiz e tesoureiro.
func (h *Handler) crownWinner(w siege.Alliance) {
	if h.Nations == nil || h.Guilds == nil {
		return
	}
	idx := func(name string) uint16 {
		if g := h.Guilds.ByName(name); g != nil && name != "" {
			return g.Index
		}
		return 0
	}
	id := h.channelNation()
	st, ok := h.Nations.Update(id, func(n *nation.State) {
		if m := idx(w.Marshal); m != 0 {
			n.MarshalGuild = m
		}
		if v := idx(w.Tactician); v != 0 {
			n.TacticianGuild = v
		}
		if v := idx(w.Judge); v != 0 {
			n.JudgeGuild = v
		}
		if v := idx(w.Treasurer); v != 0 {
			n.TreasurerGuild = v
		}
	})
	if ok && h.NationDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		if err := h.NationDB.SaveGrade(ctx, st); err != nil {
			h.log().Error("gravar grade da nacao", "nacao", id, "erro", err)
		}
	}
}

func (h *Handler) resendNationToAll() {
	for _, pid := range h.Chan.PlayingIDs() {
		if o, conn, ok := h.Chan.Lookup(pid); ok {
			h.sendNationInformation(conn, o)
		}
	}
}

// broadcastToNation manda a mensagem a quem esta em jogo na nacao
// (SendServerMsgForNation).
func (h *Handler) broadcastToNation(nationID uint16, msg string) {
	for _, pid := range h.Chan.PlayingIDs() {
		if o, conn, ok := h.Chan.Lookup(pid); ok && o.Avatar.Nation == nationID {
			h.messageType(conn, msg, 16)
		}
	}
}
