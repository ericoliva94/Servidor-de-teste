package store

import (
	"context"
	"fmt"

	"aika/internal/model"
)

// MaxQuests e o tamanho da lista de quests do personagem (PlayerQuests).
const MaxQuests = 50

// Quests le e grava a tabela quests.
type Quests struct {
	DB *DB
}

// Load devolve as quests do personagem, ate MaxQuests.
func (q Quests) Load(ctx context.Context, charID uint32) ([]model.QuestProgress, error) {
	rows, err := q.DB.Query(ctx,
		`SELECT questid, isdone, req1, req2, req3, req4, req5, updated_at, created_at
		   FROM quests WHERE charid = ?`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: buscar quests do personagem %d: %w", charID, err)
	}
	defer rows.Close()
	var out []model.QuestProgress
	for rows.Next() {
		var id, done, r1, r2, r3, r4, r5, upd, cre int64
		if err := rows.Scan(&id, &done, &r1, &r2, &r3, &r4, &r5, &upd, &cre); err != nil {
			return out, fmt.Errorf("store: ler quest: %w", err)
		}
		if id <= 0 || len(out) >= MaxQuests {
			continue
		}
		clamp := func(v int64) uint8 { return uint8(min(max(v, 0), 255)) }
		out = append(out, model.QuestProgress{
			QuestID: uint32(id), Done: done != 0,
			Complete:  [5]uint8{clamp(r1), clamp(r2), clamp(r3), clamp(r4), clamp(r5)},
			UpdatedAt: upd, CreatedAt: cre,
		})
	}
	return out, rows.Err()
}

// Save troca as quests do personagem pelas da lista, numa transacao. A origem
// so gravava se o personagem tivesse alguma quest, entao quem abandonava a
// ultima ficava com ela no banco; aqui a lista vazia limpa a tabela.
func (q Quests) Save(ctx context.Context, charID uint32, list []model.QuestProgress) error {
	if charID == 0 {
		return fmt.Errorf("store: gravar quests: personagem invalido")
	}
	tx, err := q.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao das quests: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM quests WHERE charid = ?`, charID); err != nil {
		return fmt.Errorf("store: limpar quests: %w", err)
	}
	for _, d := range list {
		if d.QuestID == 0 {
			continue
		}
		done := 0
		if d.Done {
			done = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quests (charid, questid, req1, req2, req3, req4, req5, isdone, updated_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			charID, d.QuestID, d.Complete[0], d.Complete[1], d.Complete[2], d.Complete[3], d.Complete[4],
			done, d.UpdatedAt, d.CreatedAt); err != nil {
			return fmt.Errorf("store: gravar quest %d: %w", d.QuestID, err)
		}
	}
	return tx.Commit()
}
