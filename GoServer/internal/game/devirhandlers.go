package game

import (
	"context"
	"fmt"

	"aika/internal/devir"
	"aika/internal/itemfn"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos das reliquias e dos devires.
const (
	CodeRequestDevirInfo = 0xB52 // cliente -> servidor: pedir o estado dos templos
	CodeMoveToReliquary  = 0xE51 // cliente -> servidor: entregar ou puxar reliquia
	CodeDevirInfo        = 0x953 // servidor -> cliente: estado dos templos
	CodeSignal10F        = 0x10F // servidor -> cliente: sinal depois de mexer na reliquia
	CodeRefreshKills     = 0x12A // servidor -> cliente: honra e abates
)

// NoDevir marca "nenhum templo aberto" em Session.OpenedDevir/OpenedTemple.
const NoDevir = 255

// Quem esta do outro lado do $E51: o NPC do templo (entregar) ou o NPC da
// reliquia (puxar), os ClientIDs fixos da origem.
const (
	devirTempleNPC = 263
	devirRelicNPC  = 1793
)

// Constantes de regra da origem.
const (
	honorPerRelicLevel = 250  // INC_HONOR_RELIQ_LEVEL
	relicReward        = 8544 // item dado a quem entrega
	partyShareRange    = 20
	itemTypeRelic      = 40
)

// DevirStore guarda os templos.
type DevirStore interface {
	Load(ctx context.Context, nation uint8) ([]devir.Row, error)
	Save(ctx context.Context, nation uint8, rows []devir.Row) error
}

// isSacredTreasure diz se o item e um tesouro sagrado entregavel
// (6370..6444 e 12000..12032, como na origem).
func isSacredTreasure(index uint16) bool {
	return index >= 6370 && index <= 6444 || index >= 12000 && index <= 12032
}

// LoadDevires carrega os templos do banco. Sem banco, os templos ficam
// vazios e habilitados.
func (h *Handler) LoadDevires(ctx context.Context) error {
	if h.Devir == nil || h.DevirDB == nil {
		return nil
	}
	rows, err := h.DevirDB.Load(ctx, h.Devir.Nation)
	if err != nil {
		return err
	}
	h.Devir.Load(rows, h.relicEffect)
	return nil
}

// relicEffect e EF[0]/EFV[0] do item (o efeito sagrado da reliquia).
func (h *Handler) relicEffect(item uint16) (devir.Effect, bool) {
	info := h.data().Item(int(item))
	if info == nil {
		return devir.Effect{}, false
	}
	return devir.Effect{Index: info.EF[0], Value: int(info.EFV[0])}, true
}

// requestDevirInfo e o porte de TPacketHandlers.RequestUpdateReliquare ($B52).
// O Go tem um canal so, entao o numero do canal pedido nao muda a resposta.
func (h *Handler) requestDevirInfo(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Devir == nil {
		return nil
	}
	_ = c.SendStruct(CodeDevirInfo, h.buildDevirInfo())
	return nil
}

// buildDevirInfo monta o $953 como TPlayer.SendUpdateReliquareInformation.
func (h *Handler) buildDevirInfo() packets.DevirTimeRelikInfoPacket {
	var pkt packets.DevirTimeRelikInfoPacket
	pkt.Header.Index = WorldIndex
	pkt.Nation = uint32(h.Devir.Nation)
	slots := [devir.TemplesPerNation]*model.DevirForPacket{
		&pkt.DevirAmk, &pkt.DevirSig, &pkt.DevirCah, &pkt.DevirMir, &pkt.DevirZel}
	infos := [devir.TemplesPerNation]*model.DevirInfoForPacket{
		&pkt.DevirAmkInfo, &pkt.DevirSigInfo, &pkt.DevirCahInfo, &pkt.DevirMirInfo, &pkt.DevirZelInfo}
	for d, t := range h.Devir.Snapshot() {
		for i, sl := range t.Slots {
			if sl.ItemID != 0 {
				slots[d].Slots[i] = model.ReliquareForPacket{
					ItemID: sl.ItemID, APP: sl.APP,
					TimeToEstabilish: uint32(max(sl.TimeToEstablish.Unix(), 0)),
					UnkByte1:         2, UnkByte2: 1,
				}
				infos[d].Slots[i].ItemID = uint32(sl.ItemID)
				netsrv.PutCString(infos[d].Slots[i].NameCapped[:], sl.Name)
				infos[d].Slots[i].TimeCapped = uint32(max(sl.TimeCapped.Unix(), 0))
			}
			if sl.Able {
				infos[d].Slots[i].IsActive = 1
			}
		}
	}
	return pkt
}

// saveDevires grava o estado dos templos; sem banco conta como gravado.
func (h *Handler) saveDevires(ctx context.Context) error {
	if h.DevirDB == nil {
		return nil
	}
	return h.DevirDB.Save(ctx, h.Devir.Nation, h.Devir.Rows())
}

// nationMessage manda uma mensagem a todos os jogadores da nacao.
func (h *Handler) nationMessage(nation uint16, msg string) {
	if h.Chan == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		if o, conn, ok := h.Chan.Lookup(id); ok && o.Avatar.Nation == nation {
			h.message(conn, msg)
		}
	}
}

// moveToReliquary e o porte de TPacketHandlers.MoveItemToReliquare ($E51).
//
// Pre-condicao: Session.OpenedDevir/OpenedTemple dizem qual templo o jogador
// abriu. Na origem quem os grava e o dialogo do NPC do templo
// (PacketHandlers.pas, entre as linhas 1689 e 2300), que depende das pedras e
// dos guardas do devir e NAO esta portado: enquanto isso, este pedido so
// funciona se algo mais preencher esses campos. Ver KNOWN_ISSUES.md.
//
// Defeitos da origem corrigidos:
//   - SrcSlot (WORD) indexava Slots[0..4] sem faixa, e puxar de um slot vazio
//     dava ao jogador o item 0;
//   - se a gravacao do roubo falhava, o slot era restaurado mas o item ja
//     tinha ido para o inventario: duplicava. Aqui grava antes de entregar;
//   - OpenedDevir 255 (nenhum aberto) indexava o vetor de templos.
func (h *Handler) moveToReliquary(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.MoveItemToReliquare
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if h.Devir == nil {
		h.message(c, "Reliquias nao estao disponiveis neste servidor.")
		return nil
	}
	if s.OpenedDevir >= devir.TemplesPerNation {
		h.message(c, "Nenhum templo aberto.")
		return nil
	}
	switch pkt.DevirClientID {
	case devirTempleNPC:
		return h.deliverRelic(ctx, c, s, pkt)
	case devirRelicNPC:
		return h.pullRelic(ctx, c, s, p, pkt)
	}
	return nil
}

// deliverRelic entrega um tesouro sagrado no templo aberto.
func (h *Handler) deliverRelic(ctx context.Context, c *netsrv.Conn, s *Session, pkt packets.MoveItemToReliquare) error {
	it, err := h.resolveSlot(c, s, store.SlotTypeInventory, int(pkt.SrcSlot), true)
	if err != nil || it.IsEmpty() {
		h.message(c, "Slot invalido.")
		return nil
	}
	if !isSacredTreasure(it.Index) {
		h.message(c, "Este item nao e um tesouro sagrado.")
		return nil
	}
	d := int(s.OpenedDevir)
	slot, ok := h.Devir.FreeSpace(d)
	if !ok {
		h.message(c, "Todos os espacos sagrados disponiveis estao lotados.")
		return nil
	}
	if s.Avatar.Nation == 0 || s.Avatar.Nation != uint16(h.Devir.Nation) {
		h.message(c, "Para computar a reliquia, e necessario ter nacionalidade no pais.")
		return nil
	}

	eff, _ := h.relicEffect(it.Index)
	item := *it
	prev, err := h.Devir.Insert(d, slot, item.Index, item.APP, eff, s.Avatar.Name, h.now())
	if err != nil {
		h.message(c, "Slot de templo invalido.")
		return nil
	}
	if err := h.saveDevires(ctx); err != nil {
		h.Devir.UndoInsert(d, slot, prev, eff)
		h.log().Error("gravar templos", "usuario", s.Username, "erro", err)
		h.message(c, "Falha ao salvar reliquia no banco de dados.")
		return nil
	}

	*it = model.Item{}
	h.sendRefreshItem(c, s, store.SlotTypeInventory, pkt.SrcSlot, *it)
	nome := itemfn.ItemName(h.data(), item.Index)
	h.nationMessage(s.Avatar.Nation, "O tesouro sagrado <"+nome+"> foi colocado no templo, efeito sera aplicado.")
	h.updateReliquaresForAll()
	_ = c.SendStruct(CodeSignal10F, packets.SignalData{Header: packets.PacketHeader{Index: s.ClientID}})
	if h.Chan != nil {
		eraw, err := netsrv.Marshal(CodeSendEffect, packets.SendClientIndexPacket{Index: uint32(s.ClientID), Effect: 0})
		if err == nil {
			_ = h.Chan.SendTo(s.ClientID, eraw)
			h.Chan.Broadcast(s.Base.VisiblePlayers.IDs(), eraw)
		}
	}

	// Honra e o item de recompensa para quem entregou e para o grupo por perto.
	honra := uint32(itemfn.ItemLevel(h.data(), item.Index)+1) * honorPerRelicLevel
	for _, alvo := range h.relicBeneficiaries(s) {
		h.giveHonor(alvo.c, alvo.s, honra)
		h.putItem(alvo.c, alvo.s, h.newStackItem(relicReward, 1))
	}
	h.log().Info("reliquia entregue", "jogador", s.Avatar.Name, "item", item.Index, "templo", d, "slot", slot)
	return nil
}

type relicTarget struct {
	s *Session
	c *netsrv.Conn
}

// relicBeneficiaries e quem recebe honra e recompensa: o jogador sozinho, ou
// os membros do grupo a no maximo 20 celulas. Raid nao esta portada, entao os
// grupos aliados ficam de fora.
func (h *Handler) relicBeneficiaries(s *Session) []relicTarget {
	out := []relicTarget{}
	if h.Parties != nil && h.Chan != nil {
		if g := h.Parties.Of(s.ClientID); g != nil {
			for _, id := range h.Parties.MembersOf(g) {
				if o, oc, ok := h.Chan.Lookup(id); ok && o.Status >= Playing && s.Position.InRange(o.Position, partyShareRange) {
					out = append(out, relicTarget{o, oc})
				}
			}
			return out
		}
	}
	if conn := h.connOf(s); conn != nil {
		out = append(out, relicTarget{s, conn})
	}
	return out
}

func (h *Handler) connOf(s *Session) *netsrv.Conn {
	if h.Chan == nil {
		return nil
	}
	_, c, _ := h.Chan.Lookup(s.ClientID)
	return c
}

// giveHonor soma honra, com teto, e manda o $12A.
func (h *Handler) giveHonor(c *netsrv.Conn, s *Session, honra uint32) {
	if sum := uint64(s.Avatar.Honor) + uint64(honra); sum > 0xFFFFFFFF {
		s.Avatar.Honor = 0xFFFFFFFF
	} else {
		s.Avatar.Honor = uint32(sum)
	}
	if c == nil {
		return
	}
	h.message(c, fmt.Sprintf("Adquiriu %d pontos de honra.", honra))
	_ = c.SendStruct(CodeRefreshKills, packets.UpdateHonorAndKills{
		Header: packets.PacketHeader{Index: s.ClientID}, Honor: s.Avatar.Honor, Kills: s.Avatar.KillPoint})
}

// pullRelic leva uma reliquia do templo inimigo.
func (h *Handler) pullRelic(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet, pkt packets.MoveItemToReliquare) error {
	if s.Avatar.Nation == uint16(h.Devir.Nation) {
		h.message(c, "Voce nao pode pegar seus proprios tesouros.")
		return nil
	}
	if _, ok := h.freeInventorySlot(s); !ok {
		h.message(c, "Inventario cheio.")
		return nil
	}
	if h.findByItemType(s, itemTypeRelic) >= 0 {
		h.message(c, "Voce nao pode carregar mais de uma reliquia por vez.")
		return nil
	}
	d := int(s.OpenedDevir)
	taken, undo, err := h.Devir.Pull(d, int(pkt.SrcSlot), func(item uint16) devir.Effect {
		e, _ := h.relicEffect(item)
		return e
	}, h.now())
	switch err {
	case nil:
	case devir.ErrNotReleased:
		h.message(c, "Reliquia nao liberada ainda.")
		return nil
	default:
		return nil // slot invalido ou vazio
	}
	if err := h.saveDevires(ctx); err != nil {
		undo()
		h.log().Error("gravar templos", "usuario", s.Username, "erro", err)
		h.message(c, "Falha ao salvar roubo no banco de dados.")
		return nil
	}
	// A reliquia so vai para o inventario depois de gravada.
	if !h.putItem(c, s, h.newStackItem(taken.ItemID, 1)) {
		undo()
		_ = h.saveDevires(ctx)
		h.message(c, "Inventario cheio.")
		return nil
	}

	s.OpenedDevir, s.OpenedTemple = NoDevir, NoDevir
	_ = c.SendStruct(CodeSignal10F, packets.SignalData{Header: packets.PacketHeader{Index: s.ClientID}})
	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(p.Code, pkt)
	nome := itemfn.ItemName(h.data(), taken.ItemID)
	h.nationMessage(uint16(h.Devir.Nation),
		"O tesouro sagrado ["+nome+"] foi roubado do templo. Efeito sagrado cancelado.")
	h.updateReliquaresForAll()
	h.log().Info("reliquia roubada", "jogador", s.Avatar.Name, "item", taken.ItemID, "templo", d)
	return nil
}

// CodeReliques e o $136, a lista dos 25 itens de relíquia dos templos.
const CodeReliques = 0x136

// sendReliques e o porte de TPlayer.SendReliquesToPlayer.
//
// Quem tem nacao so ve as relíquias do canal da propria nacao (Chid da origem
// comparado com o canal do jogador); aqui um canal = uma nacao, entao a lista
// vai vazia para quem e de outra. Nacao 0 (sem nacao) ve a deste canal.
func (h *Handler) sendReliques(c *netsrv.Conn, s *Session) {
	if h.Devir == nil || c == nil {
		return
	}
	var pkt packets.SendReliques
	pkt.Header.Index = shopHeaderIndex // $7535, como a origem
	if s.Avatar.Nation == 0 || s.Avatar.Nation == uint16(h.Devir.Nation) {
		k := 0
		for _, t := range h.Devir.Snapshot() {
			for _, sl := range t.Slots {
				pkt.ReliquesItemID[k] = sl.ItemID
				k++
			}
		}
	}
	_ = c.SendStruct(CodeReliques, pkt)
}

// updateReliquaresForAll e o porte de TServerSocket.UpdateReliquaresForAll:
// depois de uma relíquia entrar ou sair, todo jogador em jogo recebe a lista
// nova e o status/pontos/HP-MP de novo. Antes o estado novo so chegava a quem
// pedisse o $B52.
//
// Os numeros de atributo ainda nao incluem o efeito da relíquia (ver
// KNOWN_ISSUES.md, "efeitos nas contas de jogo"): o reenvio garante que o
// cliente receba o valor assim que a conta passar a somá-lo.
func (h *Handler) updateReliquaresForAll() {
	if h.Chan == nil || h.Devir == nil {
		return
	}
	for _, id := range h.Chan.PlayingIDs() {
		o, conn, ok := h.Chan.Lookup(id)
		if !ok {
			continue
		}
		h.refreshRelics(o)
		h.sendReliques(conn, o)
		h.sendStatus(conn, o)
		h.sendRefreshPoint(conn, o)
		h.sendCurrentHPMP(conn, o)
	}
}
