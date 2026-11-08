package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// deleteTimeLayout e o formato da coluna delete_time: texto no formato local
// do Delphi (DateTimeToStr), que ByAccount sabe ler.
const deleteTimeLayout = "02/01/2006 15:04:05"

// SetDeleteSchedule marca ou desmarca o personagem para exclusao (colunas
// deleted e delete_time). Porte da parte de SaveCharOnCharRoom que a exclusao
// usa. A conta dona entra no WHERE.
func (c Characters) SetDeleteSchedule(ctx context.Context, accountID, charID uint32, deleted bool, when time.Time) error {
	if accountID == 0 || charID == 0 {
		return fmt.Errorf("store: exclusao com ids invalidos (conta %d, personagem %d)", accountID, charID)
	}
	d, txt := 0, ""
	if deleted {
		d, txt = 1, when.Format(deleteTimeLayout)
	}
	res, err := c.DB.Exec(ctx,
		`UPDATE characters SET deleted = :d, delete_time = :t WHERE id = :id AND owner_accid = :acc`,
		sql.Named("d", d), sql.Named("t", txt), sql.Named("id", charID), sql.Named("acc", accountID))
	if err != nil {
		return fmt.Errorf("store: marcar exclusao de %d: %w", charID, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		// O MySQL devolve 0 tambem quando nada mudou; so e erro se a linha nao
		// e da conta.
		var existe int
		rows, qerr := c.DB.Query(ctx, `SELECT COUNT(*) FROM characters WHERE id = :id AND owner_accid = :acc`,
			sql.Named("id", charID), sql.Named("acc", accountID))
		if qerr == nil {
			defer rows.Close()
			if rows.Next() {
				_ = rows.Scan(&existe)
			}
		}
		if existe == 0 {
			return fmt.Errorf("store: personagem %d nao pertence a conta %d", charID, accountID)
		}
	}
	return nil
}

// Delete apaga o personagem e o que e dele, numa transacao.
//
// Porte de TPlayer.DeleteCharacter. A origem apagava
// "DELETE FROM items WHERE owner_id = <id do personagem>" SEM olhar o tipo do
// slot: a tabela items guarda tambem o armazem e o inventario premium (dono =
// id da CONTA) e o bau de guilda (dono = indice da guilda), entao apagar o
// personagem 57 apagava os itens da conta 57 e da guilda 57. Aqui so os tipos
// que pertencem ao personagem (equipamento e inventario).
//
// A conta dona entra na conferencia, e as amizades do personagem sao
// desativadas (a origem as deixava soltas).
func (c Characters) Delete(ctx context.Context, accountID, charID uint32) error {
	if accountID == 0 || charID == 0 {
		return fmt.Errorf("store: exclusao com ids invalidos (conta %d, personagem %d)", accountID, charID)
	}
	tx, err := c.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de exclusao: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	var dono int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM characters WHERE id = ? AND owner_accid = ?`, charID, accountID).Scan(&dono); err != nil {
		return fmt.Errorf("store: conferir dono do personagem: %w", err)
	}
	if dono == 0 {
		return fmt.Errorf("store: personagem %d nao pertence a conta %d", charID, accountID)
	}
	for _, q := range []string{
		`DELETE FROM itembars WHERE owner_charid = ?`,
		`DELETE FROM skills WHERE owner_charid = ?`,
		`DELETE FROM items WHERE owner_id = ? AND slot_type IN (0, 1)`,
		`UPDATE friend_list SET active = 0, lastUpdateDate = NOW()
		  WHERE owner_characterId = ? OR friend_characterId = ?`,
		`DELETE FROM characters WHERE id = ?`,
	} {
		args := []any{charID}
		if q[:6] == "UPDATE" {
			args = []any{charID, charID}
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("store: apagar personagem %d: %w", charID, err)
		}
	}
	return tx.Commit()
}
