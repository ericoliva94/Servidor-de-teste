package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aika/internal/auction"
	"aika/internal/model"
)

// Auctions le e grava as ofertas do leilao.
//
// A origem apoiava o leilao em visoes e tabelas que nao acompanham este
// checkout (vwauction_getactiveoffers, auction_items, auction) e pegava o
// indice da oferta com um "SELECT max(id)" feito FORA da transacao do
// insert: dois registros ao mesmo tempo liam o mesmo id e a segunda oferta
// apontava para o item da primeira. Aqui a oferta e uma linha so, com o
// indice vindo antes da gravacao, e o fechamento e condicional.
type Auctions struct {
	DB *DB
}

const auctionColumns = `offer_index, seller_char_index, seller_name, price,
	registered_at, expires_at, hours, active, sold_to,
	item_id, item_app, item_identific, item_e1i, item_e2i, item_e3i,
	item_e1v, item_e2v, item_e3v, item_min, item_max, item_refine, item_time`

// NextIndex devolve o proximo indice de oferta.
func (a Auctions) NextIndex(ctx context.Context) (uint64, error) {
	var maior sql.NullInt64
	row := a.DB.SQL().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(offer_index), 0) FROM auction_offers`)
	if err := row.Scan(&maior); err != nil {
		return 0, fmt.Errorf("store: proximo indice de oferta: %w", err)
	}
	return uint64(max(maior.Int64, 0)) + 1, nil
}

// Register grava uma oferta nova.
func (a Auctions) Register(ctx context.Context, o auction.Offer) error {
	if o.Index == 0 || o.SellerCharIndex == 0 {
		return auction.ErrOfferNotFound
	}
	const q = `INSERT INTO auction_offers (` + auctionColumns + `)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	_, err := a.DB.SQL().ExecContext(ctx, q,
		o.Index, o.SellerCharIndex, o.SellerName, o.Price,
		unixOrZero(o.RegisteredAt), unixOrZero(o.ExpiresAt), o.Hours,
		b2i(o.Active), o.SoldTo,
		o.Item.Index, o.Item.APP, o.Item.Identific,
		o.Item.Effects.Index[0], o.Item.Effects.Index[1], o.Item.Effects.Index[2],
		o.Item.Effects.Value[0], o.Item.Effects.Value[1], o.Item.Effects.Value[2],
		o.Item.Min, o.Item.Max, o.Item.Refi, o.Item.Time)
	if err != nil {
		return fmt.Errorf("store: registrar oferta %d: %w", o.Index, err)
	}
	return nil
}

// Get busca uma oferta pelo indice.
func (a Auctions) Get(ctx context.Context, index uint64) (*auction.Offer, error) {
	if index == 0 {
		return nil, auction.ErrOfferNotFound
	}
	rows, err := a.DB.Query(ctx,
		`SELECT `+auctionColumns+` FROM auction_offers WHERE offer_index = :idx LIMIT 1`,
		sql.Named("idx", index))
	if err != nil {
		return nil, fmt.Errorf("store: buscar oferta %d: %w", index, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("store: buscar oferta %d: %w", index, err)
		}
		return nil, auction.ErrOfferNotFound
	}
	o, err := scanOffer(rows)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Active lista as ofertas valendo, da mais barata para a mais cara.
//
// A origem ordenava por "SellingPrice ASC"; mantido.
func (a Auctions) Active(ctx context.Context, limite int) ([]auction.Offer, error) {
	if limite <= 0 {
		limite = auction.MaxListItems
	}
	q := fmt.Sprintf(`SELECT %s FROM auction_offers
		 WHERE active = 1 ORDER BY price ASC LIMIT %d`, auctionColumns, limite)
	return a.list(ctx, q)
}

// ByOwner lista as ofertas de um personagem.
func (a Auctions) ByOwner(ctx context.Context, charIndex uint32, limite int) ([]auction.Offer, error) {
	if charIndex == 0 {
		return nil, nil
	}
	if limite <= 0 {
		limite = auction.MaxOwnItems
	}
	q := fmt.Sprintf(`SELECT %s FROM auction_offers
		 WHERE seller_char_index = %d AND active = 1
		 ORDER BY price ASC LIMIT %d`, auctionColumns, charIndex, limite)
	return a.list(ctx, q)
}

// Expired lista as ofertas que passaram do prazo.
func (a Auctions) Expired(ctx context.Context, agora time.Time) ([]auction.Offer, error) {
	q := fmt.Sprintf(`SELECT %s FROM auction_offers
		 WHERE active = 1 AND expires_at > 0 AND expires_at < %d
		 LIMIT 500`, auctionColumns, agora.Unix())
	return a.list(ctx, q)
}

// Close fecha a oferta e diz se ela estava aberta.
//
// O UPDATE condicional e o que impede dois compradores de levar o mesmo item:
// so uma das duas chamadas afeta uma linha. A origem conferia a oferta com um
// SELECT e gravava depois, sem nada entre as duas coisas.
func (a Auctions) Close(ctx context.Context, index uint64, comprador uint32) (bool, error) {
	if index == 0 {
		return false, nil
	}
	res, err := a.DB.SQL().ExecContext(ctx,
		`UPDATE auction_offers SET active = 0, sold_to = ?
		  WHERE offer_index = ? AND active = 1`, comprador, index)
	if err != nil {
		return false, fmt.Errorf("store: fechar oferta %d: %w", index, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: fechar oferta %d: %w", index, err)
	}
	return n > 0, nil
}

// list roda uma consulta que devolve ofertas.
func (a Auctions) list(ctx context.Context, q string) ([]auction.Offer, error) {
	rows, err := a.DB.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: listar ofertas: %w", err)
	}
	defer rows.Close()

	var out []auction.Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("store: listar ofertas: %w", err)
	}
	return out, nil
}

// scanOffer le uma linha de auction_offers.
func scanOffer(rows *sql.Rows) (auction.Offer, error) {
	var (
		o                      auction.Offer
		index, vendedor        int64
		nome                   sql.NullString
		preco                  int64
		registrado, expira     int64
		horas                  int64
		ativo, vendidoPara     int64
		itemID, app, identific int64
		e1i, e2i, e3i          int64
		e1v, e2v, e3v          int64
		minDur, maxDur         int64
		refino, tempo          int64
	)
	if err := rows.Scan(&index, &vendedor, &nome, &preco,
		&registrado, &expira, &horas, &ativo, &vendidoPara,
		&itemID, &app, &identific, &e1i, &e2i, &e3i,
		&e1v, &e2v, &e3v, &minDur, &maxDur, &refino, &tempo); err != nil {
		return o, fmt.Errorf("store: ler oferta: %w", err)
	}

	o.Index = uint64(max(index, 0))
	o.SellerCharIndex = clampToDword(vendedor)
	o.SellerName = nome.String
	o.Price = clampToDword(preco)
	o.Hours = clampToWord(horas)
	o.Active = ativo != 0
	o.SoldTo = clampToDword(vendidoPara)
	if registrado > 0 {
		o.RegisteredAt = time.Unix(registrado, 0).UTC()
	}
	if expira > 0 {
		o.ExpiresAt = time.Unix(expira, 0).UTC()
	}
	o.Item = model.Item{
		Index:     clampToWord(itemID),
		APP:       clampToWord(app),
		Identific: clampToInt32(identific),
		Effects: model.ItemEffect{
			Index: [3]byte{clampToByte(e1i), clampToByte(e2i), clampToByte(e3i)},
			Value: [3]byte{clampToByte(e1v), clampToByte(e2v), clampToByte(e3v)},
		},
		Min:  clampToByte(minDur),
		Max:  clampToByte(maxDur),
		Refi: clampToWord(refino),
		Time: clampToWord(tempo),
	}
	return o, nil
}
