package store

import (
	"context"
	"database/sql"
	"fmt"

	"aika/internal/devir"
)

// Devires le e grava a tabela devires (templos e reliquias).
type Devires struct {
	DB *DB
}

// Load le os 5 templos de uma nacao (devir_id de (n-1)*5+1 a n*5).
func (d Devires) Load(ctx context.Context, nation uint8) ([]devir.Row, error) {
	first := devir.FirstDevirID(nation)
	rows, err := d.DB.Query(ctx,
		`SELECT devir_id,
		        slot1_itemid, slot1_name, slot1_timecap, slot1_able,
		        slot2_itemid, slot2_name, slot2_timecap, slot2_able,
		        slot3_itemid, slot3_name, slot3_timecap, slot3_able,
		        slot4_itemid, slot4_name, slot4_timecap, slot4_able,
		        slot5_itemid, slot5_name, slot5_timecap, slot5_able
		   FROM devires WHERE devir_id BETWEEN :a AND :b ORDER BY devir_id`,
		sql.Named("a", first), sql.Named("b", first+devir.TemplesPerNation-1))
	if err != nil {
		return nil, fmt.Errorf("store: carregar templos da nacao %d: %w", nation, err)
	}
	defer rows.Close()
	var out []devir.Row
	for rows.Next() {
		var r devir.Row
		var (
			it   [devir.SlotsPerTemple]int64
			nm   [devir.SlotsPerTemple]sql.NullString
			tc   [devir.SlotsPerTemple]sql.NullInt64
			able [devir.SlotsPerTemple]int64
		)
		if err := rows.Scan(&r.DevirID,
			&it[0], &nm[0], &tc[0], &able[0], &it[1], &nm[1], &tc[1], &able[1],
			&it[2], &nm[2], &tc[2], &able[2], &it[3], &nm[3], &tc[3], &able[3],
			&it[4], &nm[4], &tc[4], &able[4]); err != nil {
			return out, fmt.Errorf("store: ler templo: %w", err)
		}
		for i := range r.Slots {
			r.Slots[i] = devir.RowSlot{ItemID: int(it[i]), Name: nm[i].String, TimeCap: tc[i].Int64, Able: able[i] != 0}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Save grava os 5 templos de uma nacao numa transacao. A origem montava um
// UPDATE por concatenacao de texto, com o nome do jogador dentro.
func (d Devires) Save(ctx context.Context, nation uint8, rows []devir.Row) error {
	tx, err := d.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao dos templos: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	for _, r := range rows {
		args := make([]any, 0, 4*devir.SlotsPerTemple+2)
		for _, s := range r.Slots {
			able := 0
			if s.Able {
				able = 1
			}
			args = append(args, s.ItemID, s.Name, s.TimeCap, able)
		}
		args = append(args, r.DevirID, nation)
		if _, err := tx.ExecContext(ctx,
			`UPDATE devires SET
			   slot1_itemid=?, slot1_name=?, slot1_timecap=?, slot1_able=?,
			   slot2_itemid=?, slot2_name=?, slot2_timecap=?, slot2_able=?,
			   slot3_itemid=?, slot3_name=?, slot3_timecap=?, slot3_able=?,
			   slot4_itemid=?, slot4_name=?, slot4_timecap=?, slot4_able=?,
			   slot5_itemid=?, slot5_name=?, slot5_timecap=?, slot5_able=?
			 WHERE devir_id=? AND nation_id=?`, args...); err != nil {
			return fmt.Errorf("store: gravar templo %d: %w", r.DevirID, err)
		}
	}
	return tx.Commit()
}
