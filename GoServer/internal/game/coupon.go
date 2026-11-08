package game

import (
	"context"
	"errors"
	"fmt"
	"time"

	"aika/internal/itemfn"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// CodeReclaimCoupon e o $3E05, o resgate de cupom de 16 caracteres.
const CodeReclaimCoupon = 0x3E05

// Limite contra tentativa de adivinhar cupom: 5 erros trancam por 1 minuto.
const (
	couponMaxFails = 5
	couponLockout  = time.Minute
)

// CouponStore resgata cupons. Redeem entrega o premio por apply, dentro do
// resgate; erro de apply desfaz.
type CouponStore interface {
	Redeem(ctx context.Context, codigo, nome string, apply func(store.Coupon) error) error
}

// errCouponDelivery marca falha de entrega ja avisada ao jogador.
var errCouponDelivery = errors.New("game: cupom nao entregue")

// couponCode monta o codigo AAAA-BBBB-CCCC-DDDD dos 16 caracteres do pacote.
// So letras e digitos valem; o resto e codigo invalido (a origem aceitava
// qualquer byte e o montava numa consulta).
func couponCode(raw [16]byte) (string, bool) {
	out := make([]byte, 0, 19)
	for i, b := range raw {
		ok := b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
		if !ok {
			return "", false
		}
		out = append(out, b)
		if i == 3 || i == 7 || i == 11 {
			out = append(out, '-')
		}
	}
	return string(out), true
}

// reclaimCoupon e o porte de TPacketHandlers.ReclaimCoupom ($3E05).
//
// Corrigido: o ouro era somado DUAS vezes (dois AddGold seguidos) e avisado uma;
// o resgate nao era atomico (ver store.Coupons); qualquer byte virava consulta.
func (h *Handler) reclaimCoupon(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.PacketReclaimCoupom
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if h.CouponDB == nil {
		h.message(c, "Cupom indisponivel.")
		return nil
	}
	if s.couponFails >= couponMaxFails {
		if h.now().Sub(s.couponLast) < couponLockout {
			h.message(c, "Muitas tentativas. Aguarde um minuto.")
			return nil
		}
		s.couponFails = 0
	}
	codigo, ok := couponCode(pkt.Coupom)
	if !ok {
		h.couponFail(s)
		h.message(c, "Cupom invalido.")
		return nil
	}

	err := h.CouponDB.Redeem(ctx, codigo, s.Avatar.Name, func(cp store.Coupon) error {
		return h.deliverCoupon(c, s, cp)
	})
	switch {
	case err == nil:
		s.couponFails = 0
	case errors.Is(err, store.ErrCouponInvalid):
		h.couponFail(s)
		h.message(c, "Cupom invalido.")
	case errors.Is(err, errCouponDelivery):
		// ja avisado; o cupom continua livre
	default:
		h.log().Error("resgatar cupom", "usuario", s.Username, "erro", err)
		h.message(c, "Nao foi possivel resgatar o cupom agora.")
	}
	return nil
}

func (h *Handler) couponFail(s *Session) {
	s.couponFails++
	s.couponLast = h.now()
}

// deliverCoupon entrega o premio do cupom.
func (h *Handler) deliverCoupon(c *netsrv.Conn, s *Session, cp store.Coupon) error {
	switch cp.Kind {
	case store.CouponCash:
		if cp.Target <= 0 || cp.Target > int64(^uint32(0)-s.CashBalance) {
			h.message(c, "Cupom invalido.")
			return errCouponDelivery
		}
		s.CashBalance += uint32(cp.Target)
		h.persistCash(context.Background(), s)
		h.sendPlayerCash(c, s)
		h.message(c, fmt.Sprintf("Voce ativou um PinCode [%d AikaCash].", cp.Target))
	case store.CouponItem:
		if cp.Target <= 0 || cp.Target > 65535 {
			h.message(c, "Cupom invalido.")
			return errCouponDelivery
		}
		qtd := int(max(cp.Count, 1))
		if h.freeSlots(s) < 1 {
			h.message(c, "ESVAZIE SEU INVENTARIO.")
			return errCouponDelivery
		}
		idx := uint16(cp.Target)
		if itemfn.CanAgroup(h.data(), idx) {
			if !h.putItem(c, s, h.newStackItem(idx, uint16(min(qtd, 65535)))) {
				h.message(c, "ESVAZIE SEU INVENTARIO.")
				return errCouponDelivery
			}
		} else {
			for i := 0; i < min(qtd, 20); i++ {
				if !h.putItem(c, s, h.newStackItem(idx, 1)) {
					h.message(c, "ESVAZIE SEU INVENTARIO.")
					return errCouponDelivery
				}
			}
		}
		h.message(c, "Seu item ["+itemfn.ItemName(h.data(), idx)+"] foi entregue com sucesso.")
	case store.CouponGold:
		if cp.Target <= 0 {
			h.message(c, "Cupom invalido.")
			return errCouponDelivery
		}
		h.giveGoldQuiet(c, s, cp.Target)
		h.message(c, fmt.Sprintf("Voce ativou um PinCode [%d Gold].", cp.Target))
	default:
		h.message(c, "Cupom invalido.")
		return errCouponDelivery
	}
	return nil
}

// giveGoldQuiet soma ouro com teto e atualiza o saldo, sem a mensagem de giveGold.
func (h *Handler) giveGoldQuiet(c *netsrv.Conn, s *Session, valor int64) {
	if valor > MaxGold-s.Avatar.Gold {
		s.Avatar.Gold = MaxGold
	} else {
		s.Avatar.Gold += valor
	}
	h.sendRefreshMoney(c, s)
}
