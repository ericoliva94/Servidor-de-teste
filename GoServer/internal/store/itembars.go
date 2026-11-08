package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ItemBarSlots e o tamanho da barra de atalhos: Base.ItemBar e
// ARRAY [0..23] OF DWORD.
const ItemBarSlots = 24

// ItemBars le e grava a tabela itembars (barra de atalhos do personagem).
type ItemBars struct {
	DB *DB
}

// Load le a barra de um personagem. Linha com slot fora de 0..23 e ignorada: a
// origem fazia "ItemBar[slot] := item" com o slot vindo do banco, sem checar.
func (b ItemBars) Load(ctx context.Context, charID uint32) ([ItemBarSlots]uint32, error) {
	var bar [ItemBarSlots]uint32
	if charID == 0 {
		return bar, nil
	}
	rows, err := b.DB.Query(ctx,
		`SELECT slot, item FROM itembars WHERE owner_charid = :id ORDER BY slot LIMIT 32`,
		sql.Named("id", charID))
	if err != nil {
		return bar, fmt.Errorf("store: barra de itens de %d: %w", charID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var slot, item int64
		if err := rows.Scan(&slot, &item); err != nil {
			return bar, fmt.Errorf("store: ler barra de itens: %w", err)
		}
		if slot < 0 || slot >= ItemBarSlots || item <= 0 || item > 0xFFFFFFFF {
			continue
		}
		bar[slot] = uint32(item)
	}
	return bar, rows.Err()
}

// Save troca a barra do personagem numa transacao (DELETE + INSERT, como a
// origem, mas sem montar o SQL por concatenacao).
func (b ItemBars) Save(ctx context.Context, charID uint32, bar [ItemBarSlots]uint32) error {
	if charID == 0 {
		return fmt.Errorf("store: barra de itens sem personagem")
	}
	tx, err := b.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao da barra: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	if _, err := tx.ExecContext(ctx, `DELETE FROM itembars WHERE owner_charid = ?`, charID); err != nil {
		return fmt.Errorf("store: limpar barra de itens: %w", err)
	}
	for slot, item := range bar {
		if item == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO itembars (owner_charid, slot, item) VALUES (?, ?, ?)`,
			charID, slot, item); err != nil {
			return fmt.Errorf("store: gravar barra de itens: %w", err)
		}
	}
	return tx.Commit()
}

// TeleportPositions le a coluna tp_positions do personagem (texto "x,y;x,y;").
func (c Characters) TeleportPositions(ctx context.Context, charID uint32) (string, error) {
	var txt sql.NullString
	rows, err := c.DB.Query(ctx, `SELECT tp_positions FROM characters WHERE id = :id`, sql.Named("id", charID))
	if err != nil {
		return "", fmt.Errorf("store: posicoes de teleporte de %d: %w", charID, err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&txt); err != nil {
			return "", err
		}
	}
	return txt.String, rows.Err()
}

// SaveTeleportPositions grava tp_positions, com a conta dona no WHERE.
func (c Characters) SaveTeleportPositions(ctx context.Context, accountID, charID uint32, txt string) error {
	_, err := c.DB.Exec(ctx,
		`UPDATE characters SET tp_positions = :t WHERE id = :id AND owner_accid = :acc`,
		sql.Named("t", txt), sql.Named("id", charID), sql.Named("acc", accountID))
	if err != nil {
		return fmt.Errorf("store: gravar posicoes de teleporte de %d: %w", charID, err)
	}
	return nil
}
