package store

import (
	"context"
	"fmt"

	"aika/internal/model"
)

// MaxTitles e o tamanho da lista de titulos do personagem (TTitleList).
const MaxTitles = 96

// Titles le e grava a tabela titles.
type Titles struct {
	DB *DB
}

// Load devolve os titulos do personagem, ate MaxTitles, sem as linhas de
// indice 0 (a origem as pulava).
func (t Titles) Load(ctx context.Context, charID uint32) ([]model.TitleData, error) {
	rows, err := t.DB.Query(ctx,
		`SELECT title_index, title_level, title_progress FROM titles WHERE owner_charid = ?`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: buscar titulos do personagem %d: %w", charID, err)
	}
	defer rows.Close()
	var out []model.TitleData
	for rows.Next() {
		var idx, lvl, prog int64
		if err := rows.Scan(&idx, &lvl, &prog); err != nil {
			return out, fmt.Errorf("store: ler titulo: %w", err)
		}
		if idx <= 0 || len(out) >= MaxTitles {
			continue
		}
		out = append(out, model.TitleData{
			Index: uint8(min(idx, 255)), Level: uint8(min(lvl, 255)), Progress: uint16(min(prog, 65535)),
		})
	}
	return out, rows.Err()
}

// Save troca os titulos do personagem pelos da lista, numa transacao. A origem
// apagava e inserevia por concatenacao de texto, sem transacao: uma queda no
// meio deixava o personagem sem titulos.
func (t Titles) Save(ctx context.Context, charID uint32, list []model.TitleData) error {
	if charID == 0 {
		return fmt.Errorf("store: gravar titulos: personagem invalido")
	}
	tx, err := t.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao dos titulos: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM titles WHERE owner_charid = ?`, charID); err != nil {
		return fmt.Errorf("store: limpar titulos: %w", err)
	}
	for _, d := range list {
		if d.Index == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO titles (title_index, title_level, title_progress, owner_charid) VALUES (?, ?, ?, ?)`,
			d.Index, d.Level, d.Progress, charID); err != nil {
			return fmt.Errorf("store: gravar titulo %d: %w", d.Index, err)
		}
	}
	return tx.Commit()
}
