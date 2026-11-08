package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// EventSlotType e EVENT_ITEM: o slot_type dos itens de evento esperando na
// tabela items para o jogador resgatar.
const EventSlotType = 17

// DiaryInterval e o intervalo do premio diario.
const DiaryInterval = 24 * time.Hour

// EventItem e uma linha de item de evento.
type EventItem struct {
	ID     uint32
	ItemID uint16
	Amount uint16
}

// Events le e grava os itens de evento e a data do premio diario.
type Events struct {
	DB *DB
}

// List devolve os itens de evento do personagem, do mais antigo ao mais novo.
func (e Events) List(ctx context.Context, charID uint32) ([]EventItem, error) {
	rows, err := e.DB.Query(ctx,
		`SELECT id, item_id, refine FROM items WHERE owner_id = ? AND slot_type = ? ORDER BY id`,
		charID, EventSlotType)
	if err != nil {
		return nil, fmt.Errorf("store: itens de evento do personagem %d: %w", charID, err)
	}
	defer rows.Close()
	var out []EventItem
	for rows.Next() {
		var id uint32
		var item, refine int64
		if err := rows.Scan(&id, &item, &refine); err != nil {
			return out, fmt.Errorf("store: ler item de evento: %w", err)
		}
		out = append(out, EventItem{ID: id, ItemID: uint16(item), Amount: uint16(refine)})
	}
	return out, rows.Err()
}

// Delete apaga uma linha de item de evento e diz se foi este chamado que a
// apagou. A origem apagava TODAS as linhas do mesmo item_id depois de entregar
// uma: quem tinha duas caixas iguais recebia uma e perdia a outra.
func (e Events) Delete(ctx context.Context, charID, id uint32) (bool, error) {
	res, err := e.DB.Exec(ctx,
		`DELETE FROM items WHERE id = ? AND owner_id = ? AND slot_type = ?`, id, charID, EventSlotType)
	if err != nil {
		return false, fmt.Errorf("store: apagar item de evento %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Add poe um item de evento na fila do personagem (PutItemOnEvent).
func (e Events) Add(ctx context.Context, charID uint32, itemID, amount uint16) error {
	_, err := e.DB.Exec(ctx,
		`INSERT INTO items (slot_type, owner_id, item_id, refine, slot) VALUES (?, ?, ?, ?, 0)`,
		EventSlotType, charID, itemID, amount)
	if err != nil {
		return fmt.Errorf("store: guardar item de evento: %w", err)
	}
	return nil
}

// DiaryDue diz se o premio diario esta liberado e, se estiver, grava a hora de
// agora como a do ultimo (TPlayer.DiaryItemAvaliable). A leitura trava a linha
// ate gravar: dois pedidos juntos nao ganham o premio duas vezes. Coluna vazia
// ou invalida conta como "nunca recebeu".
func (e Events) DiaryDue(ctx context.Context, charID uint32, now time.Time) (bool, error) {
	tx, err := e.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: abrir transacao do premio diario: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	var last sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT last_diary_event FROM characters WHERE id = ? FOR UPDATE`, charID).Scan(&last)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: ler premio diario: %w", err)
	}
	var quando int64
	if last.Valid {
		quando, _ = strconv.ParseInt(last.String, 10, 64)
	}
	if now.Sub(time.Unix(quando, 0)) < DiaryInterval {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE characters SET last_diary_event = ? WHERE id = ?`,
		strconv.FormatInt(now.Unix(), 10), charID); err != nil {
		return false, fmt.Errorf("store: gravar premio diario: %w", err)
	}
	return true, tx.Commit()
}
