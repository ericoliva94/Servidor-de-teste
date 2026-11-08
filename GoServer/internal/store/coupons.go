package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Tipos de cupom (command_type de gm_commands).
const (
	CouponCash = 3 // COUPOM_GMCOMMAND: target_itemid = AikaCash
	CouponItem = 4 // FOUNDER_GMCOMMAND: target_itemid = item, target_itemcnt = quantidade
	CouponGold = 5 // target_itemid = ouro
)

// Coupon e um cupom resgatavel de gm_commands.
type Coupon struct {
	ID     uint32
	Kind   int
	Target int64 // target_itemid
	Count  int64 // target_itemcnt
}

// ErrCouponInvalid diz que o codigo nao existe, nao foi liberado, foi recusado
// ou ja foi usado.
var ErrCouponInvalid = errors.New("store: cupom invalido")

// Coupons resgata os cupons da tabela gm_commands.
//
// A origem lia o cupom, entregava o premio e SO ENTAO marcava target_name numa
// segunda conexao: dois resgates ao mesmo tempo liam a mesma linha livre e os
// dois ganhavam. Aqui a linha e travada (FOR UPDATE), o premio e entregue
// dentro da transacao e a marca vai junto; se o premio falhar (inventario
// cheio), nada e marcado.
type Coupons struct {
	DB *DB
}

// Redeem resgata o cupom para o personagem nome. apply entrega o premio e, ao
// devolver erro, desfaz o resgate.
func (c Coupons) Redeem(ctx context.Context, codigo, nome string, apply func(Coupon) error) error {
	tx, err := c.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de cupom: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	var cp Coupon
	err = tx.QueryRowContext(ctx,
		`SELECT id, command_type, target_itemid, target_itemcnt
		   FROM gm_commands
		  WHERE coupom = ? AND runned = 1 AND refused = 0 AND target_name = ''
		  FOR UPDATE`, codigo).Scan(&cp.ID, &cp.Kind, &cp.Target, &cp.Count)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCouponInvalid
	}
	if err != nil {
		return fmt.Errorf("store: buscar cupom: %w", err)
	}
	if err := apply(cp); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE gm_commands SET target_name = ? WHERE id = ? AND target_name = ''`, nome, cp.ID)
	if err != nil {
		return fmt.Errorf("store: marcar cupom: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrCouponInvalid
	}
	return tx.Commit()
}
