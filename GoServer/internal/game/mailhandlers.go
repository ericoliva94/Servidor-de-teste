package game

import (
	"context"
	"strings"

	"aika/internal/mail"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de correio.
const (
	CodeMailSend     = 0x3F15 // nos dois sentidos: enviar / resultado
	CodeMailConfirm  = 0x3F16 // cliente -> servidor: conferir destinatario
	CodeMailList     = 0x3F17 // servidor -> cliente: lista de cartas
	CodeMailOpen     = 0x3F18 // nos dois sentidos: abrir carta
	CodeMailWithdraw = 0x3F1A // cliente -> servidor: retirar anexo
	CodeMailUnread   = 0x3F1B // servidor -> cliente: quantas nao lidas
)

// Modos de abertura do $3F18.
const (
	MailOpenRead   = 0 // ler a carta
	MailOpenDelete = 1 // apagar
)

// confirmMail e o porte de TMailFunctions.ConfirmSendMail ($3F16): o cliente
// pede para conferir o destinatario antes de escrever.
func (h *Handler) confirmMail(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}

	var pkt packets.SendMailPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := strings.TrimSpace(netsrv.CString(pkt.Nick[:]))
	if nome == "" || strings.EqualFold(nome, s.Avatar.Name) {
		h.message(c, "Destinatario invalido.")
		s.CanSendMailTo = ""
		return h.sendMailResult(c, s, BoolRefuse)
	}
	if s.Avatar.Gold < mail.SendCost {
		h.message(c, "Nao possui ouro suficiente.")
		s.CanSendMailTo = ""
		return h.sendMailResult(c, s, BoolRefuse)
	}

	// O destinatario tem de existir. A origem so guardava o nome em
	// Player.CanSendMailTo e conferia a existencia depois, na gravacao do
	// arquivo — que criava o arquivo do destinatario mesmo que ele nao
	// existisse, deixando caixa de correio de personagem inexistente no
	// disco.
	if h.MailDB != nil {
		existe, err := h.MailDB.CharacterExists(ctx, nome)
		if err != nil {
			h.log().Error("conferir destinatario", "nome", nome, "erro", err)
			return h.sendMailResult(c, s, BoolRefuse)
		}
		if !existe {
			h.message(c, "Personagem nao encontrado.")
			s.CanSendMailTo = ""
			return h.sendMailResult(c, s, BoolRefuse)
		}
	}

	s.CanSendMailTo = nome
	return h.sendMailResult(c, s, BoolAccept)
}

// sendMail e o porte de TMailFunctions.SendCharacterMail ($3F15).
func (h *Handler) sendMail(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.MailDB == nil {
		h.message(c, "O correio nao esta disponivel.")
		return nil
	}

	var pkt packets.ContentMailPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	destino := strings.TrimSpace(netsrv.CString(pkt.Content.Nick[:]))

	// O destinatario tem de ser o mesmo que passou pelo $3F16.
	if destino == "" || !strings.EqualFold(destino, s.CanSendMailTo) {
		return nil
	}
	custo := int64(mail.SendCost) + int64(pkt.Content.Gold)
	if s.Avatar.Gold < custo {
		s.CanSendMailTo = ""
		h.message(c, "Nao possui ouro suficiente.")
		return h.sendMailResult(c, s, BoolRefuse)
	}

	indice, err := h.MailDB.NextIndex(ctx)
	if err != nil {
		h.log().Error("proximo indice de carta", "erro", err)
		return h.sendMailResult(c, s, BoolRefuse)
	}
	carta := mail.New(indice, s.Avatar.Name, pkt.Content, h.now())
	carta.CharIndex = s.CharID()

	// Os anexos saem do inventario. A origem percorria os cinco slots e
	// recusava o indice acima de 63; o slot $FF significa "vazio".
	var retirados []int
	n := 0
	for i := range pkt.Content.ItemSlot {
		if n >= mail.MaxItems {
			break
		}
		slot := int(pkt.Content.ItemSlot[i])
		if slot == 0xFF {
			continue
		}
		if slot < 0 || slot >= store.InventorySlots || slot >= len(s.Inventory.Slots) {
			continue
		}
		it := s.Inventory.Get(slot)
		if it.IsEmpty() {
			continue
		}
		carta.Items[n] = it
		carta.CheckItem = true
		carta.Return = true
		n++
		retirados = append(retirados, slot)
	}

	if err := h.MailDB.Add(ctx, destino, carta); err != nil {
		// Falha na gravacao nao pode tirar o item do jogador: na origem os
		// itens saiam do inventario ANTES do AddMail, e um AddMail que
		// falhasse devolvia False sem repor nada — o item sumia.
		h.log().Error("gravar carta",
			"de", s.Avatar.Name, "para", destino, "erro", err)
		h.message(c, "Nao foi possivel enviar o correio.")
		return h.sendMailResult(c, s, BoolRefuse)
	}

	// Gravou: agora sim o inventario e o ouro mudam.
	for _, slot := range retirados {
		s.Inventory.Slots[slot] = model.Item{}
		h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot),
			s.Inventory.Slots[slot])
	}
	s.Avatar.Gold = subGold(s.Avatar.Gold, custo)
	s.CanSendMailTo = ""

	h.message(c, "Correio enviado.")
	h.log().Info("correio enviado",
		"de", s.Avatar.Name, "para", destino,
		"ouro", carta.Gold, "anexos", n)
	return h.sendMailResult(c, s, BoolAccept)
}

// openMail e o porte de TMailFunctions.OpenMail ($3F18): ler ou apagar.
func (h *Handler) openMail(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.MailDB == nil {
		return nil
	}

	var pkt packets.RecvOpenMailPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Index == 0 {
		return nil
	}

	caixa, err := h.MailDB.Box(ctx, s.CharID())
	if err != nil {
		h.log().Error("ler caixa de correio",
			"personagem", s.CharID(), "erro", err)
		return nil
	}
	carta := caixa.Get(pkt.Index)
	if carta == nil {
		// A origem varria um vetor que GetMails nunca preencheu, entao
		// nenhuma carta era achada: ler correio simplesmente nao funcionava.
		return nil
	}

	if pkt.OpenType == MailOpenDelete {
		// Carta com anexo nao se apaga: o item ficaria perdido.
		if !carta.Empty() {
			h.message(c, "Retire os anexos antes de apagar a carta.")
			return nil
		}
		if err := h.MailDB.Remove(ctx, s.CharID(), carta.Index); err != nil {
			h.log().Error("apagar carta", "indice", carta.Index, "erro", err)
			return nil
		}
		return h.sendMailList(ctx, c, s)
	}

	if !carta.Checked {
		carta.Checked = true
		if err := h.MailDB.MarkRead(ctx, s.CharID(), carta.Index); err != nil {
			h.log().Error("marcar carta lida", "indice", carta.Index, "erro", err)
		}
	}
	h.sendMailContent(c, s, carta)
	h.sendMailUnread(c, s, caixa.Unread())
	return nil
}

// withdrawMail e o porte de TMailFunctions.withdrawMailItem ($3F1A).
func (h *Handler) withdrawMail(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.MailDB == nil {
		return nil
	}

	var pkt packets.RecvOpenMailPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Index == 0 {
		return nil
	}

	caixa, err := h.MailDB.Box(ctx, s.CharID())
	if err != nil {
		h.log().Error("ler caixa de correio", "erro", err)
		return nil
	}
	carta := caixa.Get(pkt.Index)
	if carta == nil {
		return nil
	}
	if carta.Empty() {
		return nil
	}

	// Os itens precisam caber antes de qualquer coisa sair da carta.
	var destinos []int
	ocupados := map[int]bool{}
	for i := range carta.Items {
		if carta.Items[i].IsEmpty() {
			continue
		}
		slot, ok := h.freeInventorySlotExcept(s, ocupados)
		if !ok {
			h.message(c, "Seu inventario esta cheio.")
			return nil
		}
		ocupados[slot] = true
		destinos = append(destinos, slot)
	}

	retirado := *carta
	carta.Items = [mail.MaxItems]model.Item{}
	carta.Gold = 0
	carta.CheckItem = false
	carta.Return = false

	if err := h.MailDB.Save(ctx, s.CharID(), *carta); err != nil {
		// A carta nao foi alterada no banco: nada entra no inventario.
		h.log().Error("gravar carta apos retirada",
			"indice", carta.Index, "erro", err)
		*carta = retirado
		h.message(c, "Nao foi possivel retirar os anexos.")
		return nil
	}

	// Gravou: entrega.
	j := 0
	for i := range retirado.Items {
		if retirado.Items[i].IsEmpty() {
			continue
		}
		slot := destinos[j]
		j++
		s.Inventory.Slots[slot] = retirado.Items[i]
		h.sendRefreshItem(c, s, store.SlotTypeInventory, uint16(slot),
			s.Inventory.Slots[slot])
	}
	if retirado.Gold > 0 {
		s.Avatar.Gold += int64(retirado.Gold)
	}

	h.log().Info("anexo retirado",
		"personagem", s.Avatar.Name, "carta", carta.Index,
		"itens", j, "ouro", retirado.Gold)
	h.sendMailContent(c, s, carta)
	return h.sendMailList(ctx, c, s)
}

// freeInventorySlotExcept acha um slot livre ignorando os ja reservados nesta
// operacao.
func (h *Handler) freeInventorySlotExcept(s *Session, reservados map[int]bool) (int, bool) {
	for slot := 0; slot < MovableInventorySlots; slot++ {
		if reservados[slot] {
			continue
		}
		bolsa := inventoryBagSlot(slot)
		if bolsa < 0 || s.Inventory.Get(bolsa).IsEmpty() {
			continue
		}
		if s.Inventory.Get(slot).IsEmpty() {
			return slot, true
		}
	}
	return 0, false
}

// ---- envio

// sendMailResult manda o $3F15 com aceite ou recusa.
func (h *Handler) sendMailResult(c *netsrv.Conn, s *Session, resultado uint32) error {
	var pkt packets.SignalData
	pkt.Header.Index = s.ClientID
	pkt.Data = resultado
	return c.SendStruct(CodeMailSend, pkt)
}

// sendMailUnread manda o $3F1B com o numero de cartas nao lidas.
func (h *Handler) sendMailUnread(c *netsrv.Conn, s *Session, quantas int) {
	var pkt packets.SignalData
	pkt.Header.Index = s.ClientID
	pkt.Data = uint32(max(quantas, 0))
	if err := c.SendStruct(CodeMailUnread, pkt); err != nil {
		h.log().Debug("falha ao enviar nao lidas", "usuario", s.Username, "erro", err)
	}
}

// sendMailList manda o $3F17 com as ultimas cartas. Porte de
// SendRefreshMails.
func (h *Handler) sendMailList(ctx context.Context, c *netsrv.Conn, s *Session) error {
	if h.MailDB == nil {
		return nil
	}
	caixa, err := h.MailDB.Box(ctx, s.CharID())
	if err != nil {
		h.log().Error("ler caixa de correio", "erro", err)
		return nil
	}

	var pkt packets.PlayerMailsPacket
	pkt.Header.Index = s.ClientID
	// A origem deixava MailsAmount comentado, entao o cliente nunca sabia o
	// total de cartas da caixa.
	pkt.MailsAmount = uint32(caixa.Count())

	for i, m := range caixa.Latest(mail.MailsPerPacket) {
		if i >= len(pkt.Cartas) {
			break
		}
		pkt.Cartas[i] = h.mailToCarta(m)
	}
	if err := c.SendStruct(CodeMailList, pkt); err != nil {
		h.log().Debug("falha ao enviar cartas", "usuario", s.Username, "erro", err)
	}
	h.sendMailUnread(c, s, caixa.Unread())
	return nil
}

// mailToCarta monta a entrada da lista. Porte de ParseMail(TStructCarta).
func (h *Handler) mailToCarta(m *mail.Mail) model.StructCarta {
	var out model.StructCarta
	out.Index = m.Index
	netsrv.PutCString(out.NickEnviado[:], m.Nick)
	netsrv.PutCString(out.Titulo[:], m.Title)
	if !m.ReturnAt.IsZero() {
		netsrv.PutCString(out.DataRetorno[:], m.ReturnAt.Format("2006-01-02"))
	}
	if m.Checked {
		out.Checked = 1
	}
	if m.Return {
		out.Return = 1
	}
	if m.CheckItem {
		out.CheckItem = 1
	}
	if m.Auction {
		out.Leilao = 1
	}
	return out
}

// sendMailContent manda o $3F18 com a carta aberta. Porte de
// ParseMail(TRecvOpenMailPacket).
func (h *Handler) sendMailContent(c *netsrv.Conn, s *Session, m *mail.Mail) {
	var pkt packets.RecvOpenMailPacket
	pkt.Header.Index = s.ClientID
	pkt.Index = m.Index
	pkt.Index2 = m.Index
	pkt.CharIndex = m.CharIndex
	netsrv.PutCString(pkt.Nick[:], m.Nick)
	netsrv.PutCString(pkt.Titulo[:], m.Title)
	netsrv.PutCString(pkt.Texto[:], m.Text)
	if !m.SentAt.IsZero() {
		netsrv.PutCString(pkt.DataEnvio[:], m.SentAt.Format("2006-01-02 15:04:05"))
	}
	pkt.Items = m.Items
	pkt.Gold = m.Gold
	if m.Return {
		pkt.Return = 1
	}
	if m.Checked {
		pkt.Unk_B01 = 1
	}
	if m.CheckItem {
		pkt.Unk_B02 = 1
	}
	if m.Auction {
		pkt.Unk_B03 = 1
	}
	if err := c.SendStruct(CodeMailOpen, pkt); err != nil {
		h.log().Debug("falha ao enviar carta", "usuario", s.Username, "erro", err)
	}
}

// enterMail manda a caixa ao entrar no mundo e limpa as cartas vencidas.
func (h *Handler) enterMail(ctx context.Context, c *netsrv.Conn, s *Session) {
	if h.MailDB == nil {
		return
	}
	caixa, err := h.MailDB.Box(ctx, s.CharID())
	if err != nil {
		h.log().Error("ler caixa de correio", "erro", err)
		return
	}
	// Carta vencida sem anexo e apagada; com anexo fica, para o jogador nao
	// perder item por prazo. A origem apagava em RemoveExpiredMails sem olhar
	// o anexo.
	for _, idx := range caixa.Expired(h.now()) {
		if m := caixa.Get(idx); m != nil && !m.Empty() {
			continue
		}
		if err := h.MailDB.Remove(ctx, s.CharID(), idx); err != nil {
			h.log().Error("apagar carta vencida", "indice", idx, "erro", err)
		}
	}
	_ = h.sendMailList(ctx, c, s)
}
