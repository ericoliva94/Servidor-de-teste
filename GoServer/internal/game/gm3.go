package game

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos dos comprovantes e da limpeza de prans.
const (
	CodeGMFounderByID   = 0x3240
	CodeGMFounderByName = 0x3242
	CodeGMFounderRow    = 0x3244
	CodeGMFounderCreate = 0x3246
	CodeGMFounderOK     = 0x3248
	CodeGMResetPrans    = 0x324A
)

// GMFounderStore guarda os comprovantes de pagamento.
type GMFounderStore interface {
	GMIDByUsername(ctx context.Context, username string) (uint32, bool, error)
	FoundersByTransaction(ctx context.Context, id string) ([]store.Founder, error)
	FoundersByName(ctx context.Context, nome string) ([]store.Founder, error)
	CreateFounder(ctx context.Context, f store.Founder, gmid uint32, product int64, now time.Time) (bool, error)
	ValidateFounder(ctx context.Context, id uint32, gmid uint32) (found, already bool, err error)
	ResetPrans(ctx context.Context, accountID uint32) (bool, error)
}

// founderProduct e o produto de founder pelo valor pago.
func founderProduct(valor float32) int64 {
	switch v := int(math.Round(float64(valor))); {
	case v >= 1 && v <= 30:
		return 14134
	case v >= 31 && v <= 59:
		return 14135
	case v >= 60 && v <= 228:
		return 14136
	case v >= 229 && v <= 330:
		return 14137
	}
	return 0
}

func (h *Handler) sendFounders(c *netsrv.Conn, s *Session, lista []store.Founder) {
	for _, f := range lista {
		var pkt packets.ComprovantReceive
		pkt.Header.Index = s.ClientID
		pkt.ComprovantDBID = int32(f.ID)
		netsrv.PutCString(pkt.TransactionID[:], f.TransactionID)
		netsrv.PutCString(pkt.NameOfComprovant[:], f.Name)
		pkt.DateOfComprovant = delphiDate(f.Date)
		pkt.ValueOfComprovant = float32(f.Value)
		if f.Validated {
			pkt.IsValidated = 1
		}
		netsrv.PutCString(pkt.ValidatedBy[:], f.ValidatedBy)
		netsrv.PutCString(pkt.CoupomAttributed[:], f.Coupon)
		_ = c.SendStruct(CodeGMFounderRow, pkt)
	}
}

// gmFounderByID e o porte de GMRequestComprovantSearchID ($3240). Game master
// ou acima.
func (h *Handler) gmFounderByID(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SearchComprovantByID
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	id := strings.TrimSpace(netsrv.CString(pkt.TransactionID[:]))
	if !s.gmAtLeast(PrivGameMaster) || h.GMFounders == nil || id == "" {
		return nil
	}
	lista, err := h.GMFounders.FoundersByTransaction(ctx, id)
	if err != nil {
		h.log().Error("comprovante por id", "erro", err)
		return nil
	}
	if len(lista) == 0 {
		h.panelMessage(c, s, "O comprovante em questão não foi encontrado.")
		return nil
	}
	h.sendFounders(c, s, lista[:1])
	return nil
}

// gmFounderByName e o porte de GMRequestComprovantSearchName ($3242).
func (h *Handler) gmFounderByName(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.SearchComprovantByName
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nome := strings.TrimSpace(netsrv.CString(pkt.NameOfComprovant[:]))
	if !s.gmAtLeast(PrivGameMaster) || h.GMFounders == nil || nome == "" {
		return nil
	}
	lista, err := h.GMFounders.FoundersByName(ctx, nome)
	if err != nil {
		h.log().Error("comprovante por nome", "erro", err)
		return nil
	}
	if len(lista) == 0 {
		h.panelMessage(c, s, "O comprovante em questão não foi encontrado.")
		return nil
	}
	h.sendFounders(c, s, lista)
	return nil
}

// gmFounderCreate e o porte de GMRequestCreateComprovant ($3246): administrador
// cadastra um comprovante e, se o valor cai numa faixa, o cupom de founder ja
// aprovado. Corrigido: o codigo do cupom e sorteado (a origem usava o MD5 da
// hora), valor negativo/NaN e recusado, e o comprovante e o cupom entram numa
// transacao so.
func (h *Handler) gmFounderCreate(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.ComprovantCreate
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivAdmin) || h.GMFounders == nil {
		return nil
	}
	f := store.Founder{
		TransactionID: strings.TrimSpace(netsrv.CString(pkt.TransactionID[:])),
		Name:          strings.TrimSpace(netsrv.CString(pkt.NameOfComprovant[:])),
		Value:         float64(pkt.ValueOfComprovant),
	}
	v := float64(pkt.ValueOfComprovant)
	if f.TransactionID == "" || f.Name == "" || !(v > 0) || math.IsInf(v, 0) || pkt.DateOfComprovant <= 0 {
		h.panelMessage(c, s, "Dados corrompidos durante transmissão.")
		return nil
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	f.Date = base.Add(time.Duration(pkt.DateOfComprovant * float64(24*time.Hour)))
	gmid, ok, err := h.GMFounders.GMIDByUsername(ctx, s.gm.username)
	if err != nil || !ok {
		return nil
	}
	produto := founderProduct(pkt.ValueOfComprovant)
	if produto > 0 {
		if f.Coupon, err = couponCodeNew(); err != nil {
			return err
		}
	}
	f.ValidatedBy = s.gm.username
	criado, err := h.GMFounders.CreateFounder(ctx, f, gmid, produto, h.now())
	if err != nil {
		h.log().Error("criar comprovante", "erro", err)
		return nil
	}
	if !criado {
		h.panelMessage(c, s, "Esse ID de transação já existe.")
		return nil
	}
	h.gmLog(s, "cadastrar comprovante "+f.TransactionID, fmt.Sprintf("valor %.2f produto %d", f.Value, produto))
	if f.Coupon != "" {
		h.panelMessage(c, s, "Comprovante cadastrado com sucesso.\rCupom de ativação do founder: "+f.Coupon)
	} else {
		h.panelMessage(c, s, "Comprovante cadastrado com sucesso.\rParece que o valor foi diferente do cadastrado. Necessária criação manual do cupom.")
	}
	return nil
}

// gmFounderValidate e o porte de GMRequestComprovantValidate ($3248): game
// master ou acima marca o comprovante como validado, uma vez.
func (h *Handler) gmFounderValidate(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.ComprovantValidate
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivGameMaster) || h.GMFounders == nil || pkt.ComprovantDBID <= 0 {
		return nil
	}
	gmid, ok, err := h.GMFounders.GMIDByUsername(ctx, s.gm.username)
	if err != nil || !ok {
		return nil
	}
	found, already, err := h.GMFounders.ValidateFounder(ctx, uint32(pkt.ComprovantDBID), gmid)
	switch {
	case err != nil:
		h.log().Error("validar comprovante", "erro", err)
	case !found:
		h.panelMessage(c, s, "Solicitação não encontrada.")
	case already:
		h.panelMessage(c, s, "Solicitação já foi validada.")
	default:
		h.gmLog(s, fmt.Sprintf("validar comprovante %d", pkt.ComprovantDBID), "")
		h.panelMessage(c, s, "Você LEU o ticket, ACHOU o compovante, MARCOU como entregue e ENTREGOU o produto no mesmo ticket.")
	}
	return nil
}

// gmResetPrans e o porte de GMRequestDeletePrans ($324A): game master ou acima
// limpa as quests de evolucao e os itens de pran de uma conta que tem pran.
func (h *Handler) gmResetPrans(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	var pkt packets.PacketSendDeletePrans
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.gmAtLeast(PrivGameMaster) || h.GMFounders == nil || pkt.AccountIndex <= 0 {
		return nil
	}
	found, err := h.GMFounders.ResetPrans(ctx, uint32(pkt.AccountIndex))
	switch {
	case err != nil:
		h.log().Error("zerar prans", "conta", pkt.AccountIndex, "erro", err)
	case !found:
		h.panelMessage(c, s, "Não existem prans no banco de dados linkados a conta.")
	default:
		h.gmLog(s, "zerar quests e itens de pran", fmt.Sprintf("conta %d", pkt.AccountIndex))
		h.panelMessage(c, s, "Procedimento executado.\rtables affected: prans, items, quests")
	}
	return nil
}
