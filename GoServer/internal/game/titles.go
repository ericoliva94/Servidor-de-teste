package game

import (
	"context"
	"strconv"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
	"aika/internal/titledata"
)

// CodeUpdateActiveTitle e o $361: o cliente pede o titulo ativo e o servidor o
// repete a quem ve o jogador.
const CodeUpdateActiveTitle = 0x361

// titleCategories e o tamanho de TitleCategoryLevel no pacote de entrada.
const titleCategories = 12

// TitleStore guarda os titulos do personagem.
type TitleStore interface {
	Load(ctx context.Context, charID uint32) ([]model.TitleData, error)
	Save(ctx context.Context, charID uint32, list []model.TitleData) error
}

// titleLevelValue e TFunctions.GetTitleLevelValue: o mapa de bits que diz ao
// cliente que niveis do titulo (4 bits por titulo da categoria) o jogador tem.
func titleLevelValue(slot, level uint8) uint32 {
	if level == 0 {
		return 0
	}
	x := uint(slot)*4 + uint(level-1)
	v := uint32(1) << x
	if x >= 32 {
		v = 0 // fora dos 32 bits da categoria: a origem estourava o shift
	}
	if level > 1 {
		return v + titleLevelValue(slot, level-1)
	}
	return v
}

// loadTitles le os titulos do personagem e acha o nivel do titulo ativo. O
// indice do titulo ativo vem da coluna active_title; o nivel, da lista
// (LoadCharacterMisc).
func (h *Handler) loadTitles(ctx context.Context, s *Session, charID uint32, active uint16) {
	s.Titles = [store.MaxTitles]model.TitleData{}
	s.ActiveTitle = model.TitleData{}
	s.titleApplied = nil
	if h.TitleDB != nil && charID != 0 {
		list, err := h.TitleDB.Load(ctx, charID)
		if err != nil {
			h.log().Error("carregar titulos", "usuario", s.Username, "personagem", charID, "erro", err)
		}
		for i, t := range list {
			if i >= len(s.Titles) {
				break
			}
			s.Titles[i] = t
		}
	}
	if active > 0 && active <= 255 {
		s.ActiveTitle.Index = uint8(active)
		for _, t := range s.Titles {
			if t.Index == s.ActiveTitle.Index {
				s.ActiveTitle.Level = t.Level
				break
			}
		}
	}
	h.syncAvatarTitle(s)
}

// syncAvatarTitle copia o titulo ativo para o avatar, que e o que o pacote de
// criacao do boneco mostra.
func (h *Handler) syncAvatarTitle(s *Session) {
	s.Avatar.TitleID = s.ActiveTitle.Index
	s.Avatar.TitleLevel = s.ActiveTitle.Level
}

// fillTitleProgress e o bloco "Set Titles List" de SendToWorld: monta no pacote
// de entrada o mapa de niveis por categoria e o progresso de cada tipo de
// titulo.
//
// Protecoes que a origem nao tinha: titulo de nivel fora de 1..4 ou sem
// registro na tabela e ignorado (a origem indexava a tabela com Level-1), e o
// TitleIndex da tabela precisa caber no vetor do tipo.
func (h *Handler) fillTitleProgress(s *Session, ch *model.Character) {
	ch.TitleCategoryLevel = [titleCategories]uint32{}
	for _, t := range s.Titles {
		if t.Index == 0 || t.Level == 0 {
			continue
		}
		cat := int(t.Index) / 8
		if cat >= titleCategories {
			continue
		}
		lv, ok := h.TitleTable.Level(int(t.Index), int(t.Level))
		if !ok {
			continue
		}
		ch.TitleCategoryLevel[cat] += titleLevelValue(t.Index%8, t.Level)
		i := int(lv.Index) - 1
		switch lv.Type {
		case 8:
			if i >= 0 && i < len(ch.TitleProgressType8) {
				ch.TitleProgressType8[i] = t.Progress
			}
		case 9:
			ch.TitleProgressType9[1] = t.Progress
		case 4:
			ch.TitleProgressType4 = t.Progress
		case 10:
			ch.TitleProgressType10 = t.Progress
		case 7:
			ch.TitleProgressType7 = t.Progress
		case 11:
			ch.TitleProgressType11 = t.Progress
		case 12:
			ch.TitleProgressType12 = t.Progress
		case 13:
			ch.TitleProgressType13 = t.Progress
		case 15:
			ch.TitleProgressType15 = t.Progress
		case 16:
			if i >= 0 && i < len(ch.TitleProgressType16) {
				ch.TitleProgressType16[i] = t.Progress
			}
		case 23:
			ch.TitleProgressType23 = t.Progress
		}
	}
}

// titleEffects sao os efeitos do titulo ativo: os ate 3 pares EF/EFV do nivel
// dele na tabela.
func (h *Handler) titleEffects(t model.TitleData) []efv {
	lv, ok := h.TitleTable.Level(int(t.Index), int(t.Level))
	if !ok {
		return nil
	}
	var out []efv
	for i := 0; i < 3; i++ {
		if lv.EF[i] > 0 {
			out = append(out, efv{int(lv.EF[i]), int(lv.EFV[i])})
		}
	}
	return out
}

// applyTitleEffect liga os efeitos do titulo ativo (SetOnTitleActiveEffect).
func (h *Handler) applyTitleEffect(s *Session) {
	h.removeTitleEffect(s)
	s.titleApplied = h.titleEffects(s.ActiveTitle)
	for _, e := range s.titleApplied {
		s.Avatar.Ability.increase(e.ef, e.v)
	}
}

// removeTitleEffect desfaz o que applyTitleEffect somou. A origem recalculava o
// que subtrair a partir do titulo ativo no momento, o que desequilibrava o
// acumulador se o titulo mudasse de nivel antes; aqui sai exatamente o que
// entrou.
func (h *Handler) removeTitleEffect(s *Session) {
	for _, e := range s.titleApplied {
		s.Avatar.Ability.decrease(e.ef, e.v)
	}
	s.titleApplied = nil
}

// sendActiveTitle e TPlayer.SendUpdateActiveTitle: o $361 vai ao proprio
// jogador e a quem o ve. O nivel vai menos um (0 sem titulo).
func (h *Handler) sendActiveTitle(c *netsrv.Conn, s *Session) {
	var pkt packets.UpdateActiveTitlePacket
	pkt.Header.Index = s.ClientID
	pkt.TitleIndex = uint32(s.ActiveTitle.Index)
	if s.ActiveTitle.Level > 1 {
		pkt.TitleLevel = uint32(s.ActiveTitle.Level) - 1
	}
	if c != nil {
		if err := c.SendStruct(CodeUpdateActiveTitle, pkt); err != nil {
			h.log().Debug("enviar titulo ativo", "usuario", s.Username, "erro", err)
		}
	}
	if h.Chan != nil {
		if raw, err := netsrv.Marshal(CodeUpdateActiveTitle, pkt); err == nil {
			h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
		}
	}
}

// enterTitle roda ao entrar no mundo: manda o titulo ativo e liga o efeito dele.
func (h *Handler) enterTitle(c *netsrv.Conn, s *Session) {
	h.sendActiveTitle(c, s)
	h.applyTitleEffect(s)
	if s.ActiveTitle.Index > 0 {
		h.recalcAfterPran(s)
	}
}

// updateActiveTitle e o porte de TPacketHandlers.UpdateActiveTitle ($361).
//
// Dado 0 tira o titulo. Outro valor procura o titulo na lista do jogador, com
// nivel acima de zero, e o torna ativo. Quando o jogador nao o tem a origem
// desligava o efeito do titulo antigo mas o mantinha como ativo, e a proxima
// troca subtraia o efeito de novo; aqui nada muda.
func (h *Handler) updateActiveTitle(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Data == 0 {
		h.removeTitleEffect(s)
		s.ActiveTitle = model.TitleData{}
		h.syncAvatarTitle(s)
		h.sendActiveTitle(c, s)
	} else if pkt.Data <= 255 {
		for _, t := range s.Titles {
			if uint32(t.Index) != pkt.Data || t.Level == 0 {
				continue
			}
			h.removeTitleEffect(s)
			s.ActiveTitle = t
			h.applyTitleEffect(s)
			h.syncAvatarTitle(s)
			h.sendActiveTitle(c, s)
			break
		}
	}
	h.recalcAfterPran(s) // refaz os atributos e manda pontos e status
	h.sendRefreshLevel(c, s)
	h.sendCurrentHPMP(c, s)
	return nil
}

// saveTitles grava a lista de titulos do personagem em jogo.
func (h *Handler) saveTitles(s *Session) {
	if h.TitleDB == nil || s.SelectedSlot < 0 || s.SelectedSlot >= store.MaxCharacterSlots {
		return
	}
	charID := s.Characters[s.SelectedSlot].ID
	if charID == 0 || s.Account.ID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := h.TitleDB.Save(ctx, charID, s.Titles[:]); err != nil {
		h.log().Error("gravar titulos", "usuario", s.Username, "personagem", charID, "erro", err)
	}
}

// CodeTitleUpdate e o $17D: avisa o cliente de um titulo novo (IDAcquire) ou de
// um titulo que subiu de nivel (IDLeveled, ja menos um).
const CodeTitleUpdate = 0x17D

// sendTitleUpdate e TPlayer.SendTitleUpdate.
func (h *Handler) sendTitleUpdate(c *netsrv.Conn, s *Session, acquire, leveled uint32) {
	if c == nil {
		return
	}
	pkt := packets.UpdateTitleListPacket{IDAcquire: acquire, IDLeveled: leveled}
	pkt.Header.Index = s.ClientID
	if err := c.SendStruct(CodeTitleUpdate, pkt); err != nil {
		h.log().Debug("enviar atualizacao de titulo", "usuario", s.Username, "erro", err)
	}
}

// titleSlot acha o slot do titulo na lista, ou -1.
func titleSlot(s *Session, id int) int {
	for i, t := range s.Titles {
		if t.Index != 0 && int(t.Index) == id {
			return i
		}
	}
	return -1
}

// titleName e o nome do titulo naquele nivel (1 a 4), ou "".
func (h *Handler) titleName(id, level int) string {
	if lv, ok := h.TitleTable.Level(id, level); ok {
		return lv.Name
	}
	return ""
}

// addTitle e o porte de TPlayer.AddTitle: poe o titulo no primeiro slot livre.
// Com xMsg o progresso nasce na meta do nivel, o cliente e avisado e o jogador
// recebe a mensagem; sem xMsg o progresso nasce em 1.
//
// Corrigido: id 0 ou acima de 254 e o nivel fora de 1..4 com xMsg indexavam a
// tabela sem conferir, e um titulo que o jogador ja tem entrava duas vezes.
// Nao portado: a copia dos titulos 17 e 18 para os outros personagens da conta
// (gravava direto nas linhas dos outros personagens).
func (h *Handler) addTitle(c *netsrv.Conn, s *Session, id, level int, xMsg bool) bool {
	if id <= 0 || id >= titledata.MaxTitles || level < 0 || level > titledata.Levels {
		return false
	}
	if xMsg && level < 1 {
		return false
	}
	if titleSlot(s, id) >= 0 {
		return false
	}
	slot := -1
	for i, t := range s.Titles {
		if t.Index == 0 {
			slot = i
			break
		}
	}
	if slot < 0 {
		h.message(c, "Sua lista de títulos está cheia.")
		return false
	}
	t := model.TitleData{Index: uint8(id), Level: uint8(level), Progress: 1}
	if xMsg {
		if lv, ok := h.TitleTable.Level(id, level); ok {
			t.Progress = lv.Goal
		}
	}
	s.Titles[slot] = t
	if xMsg {
		h.sendTitleUpdate(c, s, uint32(id), uint32(level-1))
		h.message(c, "Você obteve um novo título ["+h.titleName(id, level)+"]")
	}
	return true
}

// updateTitleLevel e TPlayer.UpdateTitleLevel. Com xMsg o progresso vai para a
// meta do novo nivel. Se o titulo e o ativo, o efeito e refeito (a origem nao
// refazia e o acumulador ficava com o nivel antigo).
func (h *Handler) updateTitleLevel(c *netsrv.Conn, s *Session, id, level int, xMsg bool) bool {
	slot := titleSlot(s, id)
	if slot < 0 || level < 0 || level > titledata.Levels || (xMsg && level < 1) {
		return false
	}
	s.Titles[slot].Level = uint8(level)
	if xMsg {
		if lv, ok := h.TitleTable.Level(id, level); ok {
			s.Titles[slot].Progress = lv.Goal
		}
	}
	if int(s.ActiveTitle.Index) == id {
		s.ActiveTitle.Level = uint8(level)
		h.applyTitleEffect(s)
		h.syncAvatarTitle(s)
		h.recalcAfterPran(s)
	}
	if xMsg {
		h.message(c, "Você teve o título ["+h.titleName(id, 1)+"] aprimorado para o nível "+strconv.Itoa(level)+".")
		h.sendTitleUpdate(c, s, uint32(id), uint32(level-1))
	}
	return true
}

// removeTitle e TPlayer.RemoveTitle. Se era o titulo ativo, ele sai de ativo
// (a origem deixava o efeito ligado).
func (h *Handler) removeTitle(c *netsrv.Conn, s *Session, id int) bool {
	slot := titleSlot(s, id)
	if slot < 0 {
		return false
	}
	nome := h.titleName(id, 1)
	s.Titles[slot] = model.TitleData{}
	if int(s.ActiveTitle.Index) == id {
		h.removeTitleEffect(s)
		s.ActiveTitle = model.TitleData{}
		h.syncAvatarTitle(s)
		h.sendActiveTitle(c, s)
		h.recalcAfterPran(s)
	}
	h.sendTitleUpdate(c, s, uint32(id), 0)
	h.message(c, "Você teve o título ["+nome+"] deletado.")
	return true
}
