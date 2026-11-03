package auction

import (
	"testing"
	"time"

	"aika/internal/model"
)

func oferta(index uint64, vendedor uint32, preco uint32, agora time.Time) *Offer {
	o := New(index, vendedor, "Vendedor",
		model.Item{Index: 100}, preco, 6, agora)
	return &o
}

// A comissao segue a conta da origem, inclusive o arredondamento bancario.
func TestCommission(t *testing.T) {
	casos := []struct {
		preco uint32
		horas uint16
		quer  uint32
	}{
		// Abaixo de 1000 usa Round: 500/1000 = 0.5, empate vai para o par.
		{100, 3, 0}, {500, 3, 0}, {500, 12, 0},
		{999, 3, 1}, {999, 6, 2}, {999, 12, 4},
		// De 1000 para cima usa Trunc.
		{1000, 3, 1}, {1000, 6, 2}, {1000, 12, 4},
		{5000, 3, 5}, {5000, 6, 10}, {5000, 12, 20},
		{100000, 3, 100}, {100000, 12, 400},
		// Preco alto nao estoura.
		{4000000000, 3, 4000000},
	}
	for _, c := range casos {
		if got := Commission(c.preco, c.horas); got != c.quer {
			t.Errorf("Commission(%d, %d) = %d, queria %d",
				c.preco, c.horas, got, c.quer)
		}
	}
}

// Tempo zero nao cobra nada, e nao divide por zero.
func TestCommissionComTempoZero(t *testing.T) {
	if got := Commission(100000, 0); got != 0 {
		t.Errorf("= %d, queria 0", got)
	}
}

func TestRoundHalfEven(t *testing.T) {
	casos := map[float64]int{
		0.5: 0, 1.5: 2, 2.5: 2, 3.5: 4,
		1.4: 1, 1.6: 2, -0.5: 0, -1.5: -2,
	}
	for in, quer := range casos {
		if got := roundHalfEven(in); got != quer {
			t.Errorf("roundHalfEven(%v) = %d, queria %d", in, got, quer)
		}
	}
}

// ---- compra

// A ORDEM e a correcao central: na origem o ouro do comprador so era
// conferido DEPOIS de as duas cartas terem sido criadas e o item ja estar no
// anexo do comprador. Um comprador sem ouro recebia o item sem pagar.
func TestCanBuyConfereOuroAntes(t *testing.T) {
	agora := time.Now()
	o := oferta(1, 10, 5000, agora)

	if err := CanBuy(o, 20, 4999, agora); err != ErrNotEnoughGold {
		t.Errorf("erro = %v, queria ErrNotEnoughGold", err)
	}
	if err := CanBuy(o, 20, 5000, agora); err != nil {
		t.Errorf("com o preco exato deveria passar: %v", err)
	}
	if err := CanBuy(o, 20, 1_000_000, agora); err != nil {
		t.Errorf("com ouro sobrando deveria passar: %v", err)
	}
}

func TestCanBuyRecusas(t *testing.T) {
	agora := time.Now()

	if err := CanBuy(nil, 20, 1000, agora); err != ErrOfferNotFound {
		t.Errorf("oferta nil: erro = %v", err)
	}

	// Oferta fechada.
	fechada := oferta(1, 10, 100, agora)
	fechada.Active = false
	if err := CanBuy(fechada, 20, 1000, agora); err != ErrOfferInactive {
		t.Errorf("fechada: erro = %v", err)
	}

	// Oferta vencida.
	vencida := oferta(1, 10, 100, agora.Add(-10*time.Hour))
	if err := CanBuy(vencida, 20, 1000, agora); err != ErrOfferInactive {
		t.Errorf("vencida: erro = %v", err)
	}

	// Propria oferta.
	propria := oferta(1, 10, 100, agora)
	if err := CanBuy(propria, 10, 1000, agora); err != ErrOwnOffer {
		t.Errorf("propria: erro = %v", err)
	}
}

// ---- cancelamento

// A origem nao conferia o dono no cancelamento: qualquer jogador cancelava a
// oferta de qualquer outro mandando o id.
func TestCanCancelSoODono(t *testing.T) {
	agora := time.Now()
	o := oferta(1, 10, 100, agora)

	if err := CanCancel(o, 10); err != nil {
		t.Errorf("o dono deveria poder cancelar: %v", err)
	}
	if err := CanCancel(o, 20); err != ErrNotOwner {
		t.Errorf("de outro: erro = %v, queria ErrNotOwner", err)
	}
	if err := CanCancel(nil, 10); err != ErrOfferNotFound {
		t.Errorf("nil: erro = %v", err)
	}
	o.Active = false
	if err := CanCancel(o, 10); err != ErrOfferInactive {
		t.Errorf("fechada: erro = %v", err)
	}
}

// ---- prazo

func TestExpired(t *testing.T) {
	agora := time.Now()
	o := oferta(1, 10, 100, agora) // 6 horas

	if o.Expired(agora) {
		t.Error("oferta nova nao venceu")
	}
	if o.Expired(agora.Add(5 * time.Hour)) {
		t.Error("antes de 6 horas nao venceu")
	}
	if !o.Expired(agora.Add(7 * time.Hour)) {
		t.Error("depois de 6 horas venceu")
	}
	// Sem prazo nao vence.
	semPrazo := &Offer{Active: true}
	if semPrazo.Expired(agora) {
		t.Error("sem prazo nao vence")
	}
	var nulo *Offer
	if nulo.Expired(agora) {
		t.Error("nil nao vence")
	}
}

func TestNew(t *testing.T) {
	agora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	it := model.Item{Index: 555, Refi: 3}
	o := New(7, 42, "Alfa", it, 1234, 12, agora)

	if o.Index != 7 || o.SellerCharIndex != 42 || o.SellerName != "Alfa" {
		t.Errorf("= %+v", o)
	}
	if o.Price != 1234 || o.Hours != 12 {
		t.Errorf("preco/horas = %d/%d", o.Price, o.Hours)
	}
	if o.Item != it {
		t.Errorf("item = %+v", o.Item)
	}
	if !o.Active {
		t.Error("oferta nova deveria estar ativa")
	}
	if !o.RegisteredAt.Equal(agora) {
		t.Errorf("registro = %v", o.RegisteredAt)
	}
	if quer := agora.Add(12 * time.Hour); !o.ExpiresAt.Equal(quer) {
		t.Errorf("prazo = %v, queria %v", o.ExpiresAt, quer)
	}
}

func TestLimitesSaoOsDaOrigem(t *testing.T) {
	if MaxOwnItems != 12 {
		t.Errorf("minhas ofertas = %d, TCharacterAuction tem 12", MaxOwnItems)
	}
	if MaxListItems != 40 {
		t.Errorf("lista = %d, o pacote leva 40", MaxListItems)
	}
}
