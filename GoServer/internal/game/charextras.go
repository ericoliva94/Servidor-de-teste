package game

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"aika/internal/entity"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/stats"
	"aika/internal/store"
)

// Codigos deste arquivo.
const (
	CodeItemLink                 = 0x22D  // cliente -> servidor: fala com item linkado
	CodeChatItemLink             = 0xF6F  // servidor -> cliente: a fala com o item
	CodeChangeItemBar            = 0x31E  // os dois sentidos: atalho da barra
	CodeSetTeleport              = 0x34A  // os dois sentidos: gravar posicao de teleporte
	CodeAllAttributes            = 0x23FE // cliente -> servidor: pedir atributos
	CodeAllAttributesReply       = 0x23FF // servidor -> cliente
	CodeAllAttributesTarget      = 0x23FB // cliente -> servidor: atributos do alvo
	CodeAllAttributesTargetReply = 0x23FC // servidor -> cliente
)

// ItemBarStore guarda a barra de atalhos.
type ItemBarStore interface {
	Load(ctx context.Context, charID uint32) ([store.ItemBarSlots]uint32, error)
	Save(ctx context.Context, charID uint32, bar [store.ItemBarSlots]uint32) error
}

// TeleportStore guarda as posicoes de teleporte do mago.
type TeleportStore interface {
	TeleportPositions(ctx context.Context, charID uint32) (string, error)
	SaveTeleportPositions(ctx context.Context, accountID, charID uint32, txt string) error
}

// MaxTeleportSlots e o tamanho da lista de posicoes (TeleportList e
// ARRAY [0..4] OF TPosition).
const MaxTeleportSlots = 5

// maxSkillIndex e o limite de indice de skill aceito na barra: SkillData tem
// 12.000 linhas.
const maxSkillIndex = 12000

// Tipos de atalho do $31E.
const (
	barClear = 0 // limpar o slot
	barSkill = 2 // skill
	barPran  = 3 // skill de pran
	barItem  = 6 // item do inventario
)

// ---- barra de itens

// loadItemBar le a barra do banco ao entrar no mundo.
func (h *Handler) loadItemBar(ctx context.Context, charID uint32) [store.ItemBarSlots]uint32 {
	if h.ItemBarDB == nil || charID == 0 {
		return [store.ItemBarSlots]uint32{}
	}
	bar, err := h.ItemBarDB.Load(ctx, charID)
	if err != nil {
		h.log().Error("carregar barra de itens", "personagem", charID, "erro", err)
	}
	return bar
}

// saveItemBar grava a barra na saida.
func (h *Handler) saveItemBar(s *Session) {
	if h.ItemBarDB == nil {
		return
	}
	charID := s.CharID()
	if charID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := h.ItemBarDB.Save(ctx, charID, s.ItemBar); err != nil {
		h.log().Error("gravar barra de itens", "usuario", s.Username, "personagem", charID, "erro", err)
	}
}

func (h *Handler) sendItemBarSlot(c *netsrv.Conn, slot, tipo, index uint32) {
	pkt := packets.ChangeItemBarPacket{DestSlot: slot, SrcType: tipo, SrcIndex: index}
	pkt.Header.Index = WorldIndex
	_ = c.SendStruct(CodeChangeItemBar, pkt)
}

// changeItemBar e o porte de TPacketHandlers.ChangeItemBar ($31E).
//
// A origem indexava ItemBar[DestSlot] com um DWORD do cliente (a barra tem 24
// posicoes) e guardava "SrcIndex * 16 + 2" sem limite. Skill de pran depende
// do pran, que nao esta portado.
func (h *Handler) changeItemBar(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.ChangeItemBarPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.DestSlot >= store.ItemBarSlots {
		return nil
	}
	switch pkt.SrcType {
	case barClear:
		s.ItemBar[pkt.DestSlot] = 0
		h.sendItemBarSlot(c, pkt.DestSlot, barClear, pkt.SrcIndex)
	case barSkill:
		if pkt.SrcIndex >= maxSkillIndex {
			return nil
		}
		s.ItemBar[pkt.DestSlot] = pkt.SrcIndex*16 + 2
		h.sendItemBarSlot(c, pkt.DestSlot, barSkill, pkt.SrcIndex)
	case barPran:
		h.message(c, "Pran ainda nao esta disponivel neste servidor.")
	case barItem:
		if pkt.SrcIndex > 0xFFFF || h.firstSlotWith(s, uint16(pkt.SrcIndex)) < 0 {
			return nil // so entra o item que o jogador tem
		}
		s.ItemBar[pkt.DestSlot] = pkt.SrcIndex
		h.sendItemBarSlot(c, pkt.DestSlot, barItem, pkt.SrcIndex)
	}
	return nil
}

// ---- posicoes de teleporte

// parseTeleports le "x,y;x,y;" como LoadCharacterTeleportList. Posicao 0,0 e a
// marca de slot vazio.
func parseTeleports(txt string) [MaxTeleportSlots]model.Position {
	var out [MaxTeleportSlots]model.Position
	for i, part := range strings.Split(txt, ";") {
		if i >= MaxTeleportSlots {
			break
		}
		xy := strings.Split(strings.TrimSpace(part), ",")
		if len(xy) != 2 {
			continue
		}
		x, errx := strconv.Atoi(strings.TrimSpace(xy[0]))
		y, erry := strconv.Atoi(strings.TrimSpace(xy[1]))
		if errx != nil || erry != nil {
			continue
		}
		out[i] = model.Position{X: float32(x), Y: float32(y)}
	}
	return out
}

// formatTeleports escreve a lista. A origem pulava os slots vazios, o que
// deslocava os seguintes na proxima leitura (o slot 3 virava o 2); aqui o slot
// vazio vai como "0,0", que a leitura da origem aceita e que continua vazio.
func formatTeleports(l [MaxTeleportSlots]model.Position) string {
	var b strings.Builder
	for _, p := range l {
		if p.X == 0 && p.Y == 0 || !ValidPosition(p) {
			b.WriteString("0,0;")
			continue
		}
		fmt.Fprintf(&b, "%d,%d;", int(math.Round(float64(p.X))), int(math.Round(float64(p.Y))))
	}
	return b.String()
}

// loadTeleports le a lista do mago ao entrar no mundo (so mago tem lista).
func (h *Handler) loadTeleports(ctx context.Context, s *Session, charID uint32) {
	s.Teleports = [MaxTeleportSlots]model.Position{}
	if h.TeleportDB == nil || charID == 0 || stats.MobClass(s.Avatar.ClassInfo) != stats.ClassMage {
		return
	}
	txt, err := h.TeleportDB.TeleportPositions(ctx, charID)
	if err != nil {
		h.log().Error("carregar posicoes de teleporte", "personagem", charID, "erro", err)
		return
	}
	s.Teleports = parseTeleports(txt)
}

// setTeleport e o porte de TPacketHandlers.TeleportSetPosition ($34A): grava a
// posicao atual no slot e devolve o pacote com ela.
//
// A origem aceitava slot ate 5 numa lista de 5 posicoes (indices 0 a 4): o
// slot 5 escrevia fora do vetor. Aqui vale 0 a 4. Como na origem, so o mago
// grava no banco.
func (h *Handler) setTeleport(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.TeleportFcPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Slot >= MaxTeleportSlots || !ValidPosition(s.Position) {
		return nil
	}
	pos := model.Position{
		X: float32(math.Round(float64(s.Position.X))),
		Y: float32(math.Round(float64(s.Position.Y))),
	}
	s.Teleports[pkt.Slot] = pos
	pkt.PosX, pkt.PosY = uint32(pos.X), uint32(pos.Y)
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(CodeSetTeleport, pkt)

	if h.TeleportDB != nil && stats.MobClass(s.Avatar.ClassInfo) == stats.ClassMage {
		if id := s.CharID(); id != 0 && s.Account.ID != 0 {
			if err := h.TeleportDB.SaveTeleportPositions(ctx, s.Account.ID, id, formatTeleports(s.Teleports)); err != nil {
				h.log().Error("gravar posicoes de teleporte", "personagem", id, "erro", err)
			}
		}
	}
	return nil
}

// ---- atributos detalhados

func sat16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return uint16(v)
}

// detailedAttributes monta a tela de atributos. Porte de
// RequestAllAttributes / RequestAllAttributesTarget.
//
// PARCIAL: a origem soma a esses campos efeitos de equipamento e buff
// (GetMobAbility: EF_REGENHP, EF_IM_*...), alem de PvPDamage e PvPDefense. O
// porte ainda nao calcula esses efeitos, entao esses numeros saem sem eles
// (so a parte do personagem: Constituicao x 3 na regeneracao e Resistence
// nas resistencias). Ver KNOWN_ISSUES.md.
func detailedAttributes(a *Avatar) packets.ResponseAllAttributes {
	regen := sat16(int(a.Cons) * 3)
	return packets.ResponseAllAttributes{
		Resfriamento: a.ReduceCooldown, DanoHab: a.HabAtk, Cura: a.CureTax,
		PerFis: a.FisPenetration, PerMag: a.MagPenetration,
		RecHP: regen, RecMP: regen,
		DanoCrit: a.DamageCritical, ResisCrit: a.CritRes, ResisDuplo: a.DuploRes,
		DiminDanoCrit: a.ResDamageCrit,
		ResisLent:     a.Resistence, ResisStun: a.Resistence, ResisSilence: a.Resistence,
		ResisChoque: a.Resistence, ResisImobi: a.Resistence, ResisMedo: a.Resistence,
	}
}

// requestAllAttributes e o $23FE: os atributos do proprio jogador ($23FF).
func (h *Handler) requestAllAttributes(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	r := detailedAttributes(&s.Avatar)
	r.Header.Index = s.ClientID
	_ = c.SendStruct(CodeAllAttributesReply, r)
	return nil
}

// requestAllAttributesTarget e o $23FB: os atributos de outro jogador
// ($23FC), se forem da mesma nacao.
//
// A origem indexava Players[TargetID] com um DWORD do cliente, sem faixa e sem
// ver se havia alguem ali. Aqui o alvo precisa ser um jogador em jogo.
func (h *Handler) requestAllAttributesTarget(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.RequestAllAttributesTarget
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if h.Chan == nil || pkt.TargetID > 0xFFFF || !entity.IsPlayerID(uint16(pkt.TargetID), h.Chan.MaxConns()) {
		return nil
	}
	alvo, _, ok := h.Chan.Lookup(uint16(pkt.TargetID))
	if !ok || alvo.Status < Playing {
		return nil
	}
	if alvo.Avatar.Nation > 0 && alvo.Avatar.Nation != s.Avatar.Nation {
		h.message(c, "O alvo nao pertence a sua nacao.")
		return nil
	}
	r := detailedAttributes(&alvo.Avatar)
	t := packets.ResponseAllAttributesTarget(r)
	t.Header.Index = alvo.ClientID
	_ = c.SendStruct(CodeAllAttributesTargetReply, t)
	return nil
}

// ---- item linkado no chat

// itemLink e o porte de TPacketHandlers.SendItemChat ($22D) e de
// TPlayer.SendItemChat: fala com um item do inventario anexado.
//
// A origem lia Inventory[ItemSlot] com um BYTE do cliente (ate 255, num vetor
// de 64), repassava o Header.Index do pacote e deixava o canal megafone
// mandar a todos sem gastar o item de megafone. O megafone fica recusado
// aqui, como na fala comum.
func (h *Handler) itemLink(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.LinkItemPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.LinkItem == 0 {
		return nil
	}
	it, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.ItemSlot), true)
	if err != nil || it.IsEmpty() {
		return nil
	}

	out := packets.ChatItemLinkPacket{ChatType: uint32(pkt.ChatType), Item: *it}
	out.Header.Index = s.ClientID
	netsrv.PutCString(out.Nick[:], s.Avatar.Name)
	netsrv.PutCString(out.Fala[:], limitarTexto(netsrv.CString(pkt.Fala[:]), MaxChatRunes))
	raw, err := netsrv.Marshal(CodeChatItemLink, out)
	if err != nil {
		return err
	}
	if h.Chan == nil {
		return nil
	}

	switch pkt.ChatType {
	case ChatNormal:
		_ = h.Chan.SendTo(s.ClientID, raw)
		h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), raw)
	case ChatParty:
		if h.Parties == nil || h.Parties.Of(s.ClientID) == nil {
			h.message(c, "Voce nao esta em um grupo.")
			return nil
		}
		for _, id := range h.Parties.MembersOf(h.Parties.Of(s.ClientID)) {
			_ = h.Chan.SendTo(id, raw)
		}
	case ChatGuild:
		if h.Guilds == nil || h.Guilds.Of(s.CharID()) == nil {
			h.message(c, "Voce nao esta em uma guilda.")
			return nil
		}
		gu := h.Guilds.Of(s.CharID())
		for _, id := range h.Chan.PlayingIDs() {
			if m, _, ok := h.Chan.Lookup(id); ok && gu.FindMember(m.CharID()) >= 0 {
				_ = h.Chan.SendTo(id, raw)
			}
		}
	case ChatShout:
		agora := h.now()
		if !s.LastShout.IsZero() && agora.Sub(s.LastShout) < ShoutCooldown {
			h.message(c, "Voce nao pode floodar o grito.")
			return nil
		}
		s.LastShout = agora
		for _, id := range h.Chan.PlayingIDs() {
			_ = h.Chan.SendTo(id, raw)
		}
	default:
		h.message(c, "Este canal de conversa ainda nao esta disponivel.")
	}
	return nil
}
