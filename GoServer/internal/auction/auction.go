// Package auction tem o leilao de itens entre personagens.
//
// Porte de Server/Src/Auction/AuctionFunctions.pas e CharacterAutcion.pas.
//
// A origem apoiava o leilao em visoes e tabelas de MySQL que nao acompanham
// este checkout (vwauction_getactiveoffers, auction_items, auction,
// mails_items) e entregava o item por um esquema de correio diferente do que
// o resto do servidor usa. Aqui as ofertas ficam numa tabela propria, descrita
// em GoServer/SCHEMA.sql, e a entrega passa pelo correio portado.
package auction

import (
	"errors"
	"time"

	"aika/internal/model"
)

// Limites e constantes da origem.
const (
	// MaxOwnItems e "TCharacterAuction = ARRAY [0..11]": o pacote $3F11 leva
	// doze ofertas do proprio jogador.
	MaxOwnItems = 12
	// MaxListItems e "Items: ARRAY [0..39]" do $3F0D.
	MaxListItems = 40
	// CommissionDivisor e o 1000 de "Trunc(SellingPrice / 1000)".
	CommissionDivisor = 1000
	// CommissionTimeUnit e o 3 de "AuctionRegisterTime / 3".
	CommissionTimeUnit = 3
)

// Erros do leilao.
var (
	ErrItemNotTradable = errors.New("auction: item nao comercializavel")
	ErrNotEnoughGold   = errors.New("auction: ouro insuficiente")
	ErrOfferNotFound   = errors.New("auction: oferta nao encontrada")
	ErrOfferInactive   = errors.New("auction: oferta nao esta ativa")
	ErrNotOwner        = errors.New("auction: a oferta e de outro personagem")
	ErrOwnOffer        = errors.New("auction: nao se compra a propria oferta")
	ErrSlotInvalid     = errors.New("auction: slot de inventario invalido")
)

// Offer e uma oferta no leilao. Porte de TAutcionItem.
type Offer struct {
	Index uint64
	// SellerCharIndex e o personagem que registrou.
	SellerCharIndex uint32
	SellerName      string
	Price           uint32
	Item            model.Item

	RegisteredAt time.Time
	ExpiresAt    time.Time
	// Hours e o RegisterTime da origem: por quanto tempo a oferta vale.
	Hours uint16

	// Active diz se a oferta ainda esta valendo. Comprada ou cancelada, ela
	// fica no historico com Active falso.
	Active bool
	// SoldTo e quem comprou, zero quando ninguem.
	SoldTo uint32
}

// Expired diz se a oferta passou do prazo.
func (o *Offer) Expired(agora time.Time) bool {
	if o == nil || o.ExpiresAt.IsZero() {
		return false
	}
	return agora.After(o.ExpiresAt)
}

// Commission devolve a taxa de registro.
//
// Porte de TAuctionFunctions.GetAuctionItemComission:
//
//	sellPriceCalculated := Trunc(SellingPrice / 1000);
//	if (SellingPrice < 1000) then
//	  sellPriceCalculated := Round(SellingPrice / 1000);
//	Result := Round(sellPriceCalculated * (AuctionRegisterTime / 3));
//
// O Trunc e o Round do Delphi sao mantidos, inclusive o arredondamento
// bancario: com preco de 500 a conta da 0.5 e o empate vai para o par, entao
// a taxa e zero. Preco abaixo de 750 registra de graca, e e assim na origem.
func Commission(price uint32, hours uint16) uint32 {
	var base int
	if price < CommissionDivisor {
		base = roundHalfEven(float64(price) / CommissionDivisor)
	} else {
		base = int(float64(price) / CommissionDivisor)
	}
	taxa := roundHalfEven(float64(base) * (float64(hours) / CommissionTimeUnit))
	if taxa < 0 {
		return 0
	}
	return uint32(taxa)
}

// roundHalfEven e o Round do Delphi: empate vai para o par.
func roundHalfEven(v float64) int {
	i := int(v)
	frac := v - float64(i)
	switch {
	case frac > 0.5 || frac < -0.5:
		if v > 0 {
			return i + 1
		}
		return i - 1
	case frac == 0.5 || frac == -0.5:
		if i%2 == 0 {
			return i
		}
		if v > 0 {
			return i + 1
		}
		return i - 1
	default:
		return i
	}
}

// CanBuy confere se a oferta pode ser comprada por um personagem com um
// tanto de ouro.
//
// A ORDEM importa, e e aqui que a origem errava: em RequestBuyItem o ouro do
// comprador so era lido e conferido DEPOIS de
// RegisterAquisitionMail e RegisterSellerAquisitionMail terem criado as duas
// cartas e o item ja ter sido inserido no anexo do comprador. Um comprador sem
// ouro recebia o item e saia sem pagar: a funcao conferia o ouro, dava Exit, e
// as cartas ja estavam no banco. Aqui nada e entregue antes de a compra ser
// aceita por inteiro.
func CanBuy(o *Offer, compradorCharIndex uint32, ouro int64, agora time.Time) error {
	if o == nil {
		return ErrOfferNotFound
	}
	if !o.Active {
		return ErrOfferInactive
	}
	if o.Expired(agora) {
		return ErrOfferInactive
	}
	if o.SellerCharIndex == compradorCharIndex {
		return ErrOwnOffer
	}
	if ouro < int64(o.Price) {
		return ErrNotEnoughGold
	}
	return nil
}

// CanCancel confere se a oferta pode ser cancelada por um personagem.
func CanCancel(o *Offer, charIndex uint32) error {
	if o == nil {
		return ErrOfferNotFound
	}
	if !o.Active {
		return ErrOfferInactive
	}
	if o.SellerCharIndex != charIndex {
		return ErrNotOwner
	}
	return nil
}

// New monta uma oferta nova.
func New(index uint64, vendedor uint32, nome string, it model.Item,
	price uint32, hours uint16, agora time.Time) Offer {
	return Offer{
		Index:           index,
		SellerCharIndex: vendedor,
		SellerName:      nome,
		Price:           price,
		Item:            it,
		RegisteredAt:    agora,
		ExpiresAt:       agora.Add(time.Duration(hours) * time.Hour),
		Hours:           hours,
		Active:          true,
	}
}
