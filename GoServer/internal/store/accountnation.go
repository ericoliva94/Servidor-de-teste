package store

import (
	"context"
	"fmt"
)

// AccountNations grava a nacao escolhida pela conta.
type AccountNations struct {
	DB *DB
}

// Choose grava a nacao da conta, so se ela ainda nao tem uma (nation = 0).
// Devolve false se a conta ja tinha nacao: dois usos do item ao mesmo tempo nao
// escolhem duas vezes.
func (a AccountNations) Choose(ctx context.Context, accountID uint32, nation uint8) (bool, error) {
	res, err := a.DB.Exec(ctx, `UPDATE accounts SET nation = ? WHERE id = ? AND nation = 0`, nation, accountID)
	if err != nil {
		return false, fmt.Errorf("store: gravar nacao da conta %d: %w", accountID, err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
