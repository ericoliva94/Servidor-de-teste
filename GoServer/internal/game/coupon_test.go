package game

import (
	"context"
	"sync"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/store"
)

// fakeCoupons e um CouponStore em memoria com a mesma regra do banco: o
// premio e entregue dentro do resgate e so depois o cupom e marcado.
type fakeCoupons struct {
	mu     sync.Mutex
	livres map[string]store.Coupon
	usados map[string]string
}

func novoFakeCoupons() *fakeCoupons {
	return &fakeCoupons{livres: map[string]store.Coupon{}, usados: map[string]string{}}
}

func (f *fakeCoupons) Redeem(_ context.Context, codigo, nome string, apply func(store.Coupon) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp, ok := f.livres[codigo]
	if !ok {
		return store.ErrCouponInvalid
	}
	if err := apply(cp); err != nil {
		return err
	}
	delete(f.livres, codigo)
	f.usados[codigo] = nome
	return nil
}

const codigoCupom = "ABCD1234EFGH5678"
const codigoFmt = "ABCD-1234-EFGH-5678"

func prepCupom(t *testing.T) (*Handler, *fakeCoupons, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(700, gamedata.ItemFromList{ItemType: 6})
	tab.poeItem(701, gamedata.ItemFromList{CanAgroup: 1})
	f := novoFakeCoupons()
	h.CouponDB = f
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Name = "Alfa"
	s.Avatar.Gold = 1000
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, f, s, e
}

func resgata(t *testing.T, h *Handler, s *Session, e *espiao, codigo string) {
	t.Helper()
	var p packets.PacketReclaimCoupom
	copy(p.Coupom[:], codigo)
	if err := h.reclaimCoupon(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeReclaimCoupon, p))); err != nil {
		t.Fatal(err)
	}
}

func TestCodigoDoCupomGanhaOsTracos(t *testing.T) {
	var raw [16]byte
	copy(raw[:], codigoCupom)
	if got, ok := couponCode(raw); !ok || got != codigoFmt {
		t.Errorf("codigo = %q ok=%v", got, ok)
	}
	copy(raw[:], "ABCD1234EFGH567'")
	if _, ok := couponCode(raw); ok {
		t.Error("caractere fora de letra/digito deveria ser recusado")
	}
	raw[0] = 0 // pacote curto: bytes nulos
	if _, ok := couponCode(raw); ok {
		t.Error("byte nulo deveria ser recusado")
	}
}

func TestCupomDeCashSomaOSaldo(t *testing.T) {
	h, f, s, e := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 1, Kind: store.CouponCash, Target: 500}
	s.CashBalance = 100
	resgata(t, h, s, e, codigoCupom)
	if s.CashBalance != 600 {
		t.Errorf("saldo = %d, queria 600", s.CashBalance)
	}
	if got := mensagemDe(t, e); got != "Voce ativou um PinCode [500 AikaCash]." {
		t.Errorf("mensagem = %q", got)
	}
	if f.usados[codigoFmt] != "Alfa" {
		t.Error("o cupom deveria ficar marcado com o personagem")
	}
}

// A origem somava o ouro duas vezes.
func TestCupomDeOuroSomaUmaVez(t *testing.T) {
	h, f, s, e := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 2, Kind: store.CouponGold, Target: 5000}
	resgata(t, h, s, e, codigoCupom)
	if s.Avatar.Gold != 6000 {
		t.Errorf("ouro = %d, queria 6000", s.Avatar.Gold)
	}
}

func TestCupomDeItemEntregaEPilha(t *testing.T) {
	h, f, s, e := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 3, Kind: store.CouponItem, Target: 701, Count: 30}
	resgata(t, h, s, e, codigoCupom)
	if contaItem(s, 701) != 30 {
		t.Errorf("pilha = %d, queria 30", contaItem(s, 701))
	}
}

func TestCupomDeItemComInventarioCheioNaoGasta(t *testing.T) {
	h, f, s, e := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 4, Kind: store.CouponItem, Target: 700, Count: 1}
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: 5}
	}
	resgata(t, h, s, e, codigoCupom)
	if got := mensagemDe(t, e); got != "ESVAZIE SEU INVENTARIO." {
		t.Errorf("mensagem = %q", got)
	}
	if _, ainda := f.livres[codigoFmt]; !ainda {
		t.Fatal("o cupom foi gasto sem entregar o premio")
	}
	// Depois de abrir espaco, resgata.
	s.Inventory.Slots[0] = model.Item{}
	resgata(t, h, s, e, codigoCupom)
	if contaItem(s, 700) != 1 {
		t.Error("o item deveria ter sido entregue")
	}
}

func TestCupomInvalidoOuJaUsado(t *testing.T) {
	h, f, s, e := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 5, Kind: store.CouponCash, Target: 10}
	resgata(t, h, s, e, "ZZZZ1111ZZZZ1111")
	if got := mensagemDe(t, e); got != "Cupom invalido." {
		t.Errorf("desconhecido: %q", got)
	}
	resgata(t, h, s, e, codigoCupom)
	proximoDe(t, e, CodeClientMsg) // sucesso
	e.drena(20 * time.Millisecond)
	resgata(t, h, s, e, codigoCupom)
	if got := mensagemDe(t, e); got != "Cupom invalido." {
		t.Errorf("ja usado: %q", got)
	}
	// Pacote com lixo.
	resgata(t, h, s, e, "ABCD'1234 OR 1=1")
	if got := mensagemDe(t, e); got != "Cupom invalido." {
		t.Errorf("lixo: %q", got)
	}
}

func TestTentativasDeCupomTrancam(t *testing.T) {
	h, _, s, e := prepCupom(t)
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	for i := 0; i < couponMaxFails; i++ {
		resgata(t, h, s, e, "ZZZZ1111ZZZZ1111")
	}
	e.drena(30 * time.Millisecond)
	resgata(t, h, s, e, "ZZZZ1111ZZZZ1111")
	if got := mensagemDe(t, e); got != "Muitas tentativas. Aguarde um minuto." {
		t.Errorf("mensagem = %q", got)
	}
	agora = agora.Add(couponLockout + time.Second)
	resgata(t, h, s, e, "ZZZZ1111ZZZZ1111")
	if got := mensagemDe(t, e); got != "Cupom invalido." {
		t.Errorf("depois do prazo: %q", got)
	}
}

func TestCupomSemBanco(t *testing.T) {
	h, _, s, e := prepCupom(t)
	h.CouponDB = nil
	resgata(t, h, s, e, codigoCupom)
	if got := mensagemDe(t, e); got != "Cupom indisponivel." {
		t.Errorf("mensagem = %q", got)
	}
}

// Dois jogadores resgatando o mesmo cupom ao mesmo tempo: so um ganha.
func TestCupomConcorrenteSoUmGanha(t *testing.T) {
	h, f, _, _ := prepCupom(t)
	f.livres[codigoFmt] = store.Coupon{ID: 6, Kind: store.CouponGold, Target: 100}
	var ss [4]*Session
	var es [4]*espiao
	for i := range ss {
		ss[i], es[i] = entraJogadorSimples(t, h)
		ss[i].Avatar.Name = string(rune('A' + i))
		e := es[i]
		go func() {
			for range e.pacotes {
			}
		}()
	}
	var wg sync.WaitGroup
	for i := range ss {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var p packets.PacketReclaimCoupom
			copy(p.Coupom[:], codigoCupom)
			_ = h.reclaimCoupon(context.Background(), es[i].conn, ss[i], asPacket(t, miscPacket(CodeReclaimCoupon, p)))
		}()
	}
	wg.Wait()
	ganhadores := 0
	for _, s := range ss {
		if s.Avatar.Gold == 100 {
			ganhadores++
		}
	}
	if ganhadores != 1 {
		t.Errorf("ganhadores = %d, queria 1", ganhadores)
	}
}
