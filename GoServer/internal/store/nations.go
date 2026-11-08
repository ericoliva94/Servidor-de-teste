package store

import (
	"context"
	"fmt"

	"aika/internal/nation"
)

// Nations le e grava a tabela nations.
type Nations struct {
	DB *DB
}

// Load le as tres nacoes. Nacao ausente no banco fica fora da lista.
func (n Nations) Load(ctx context.Context) ([]nation.State, error) {
	rows, err := n.DB.Query(ctx,
		`SELECT nation_id, nation_name, nation_rank, guild_id_marshal, guild_id_tactician,
		        guild_id_judge, guild_id_treasurer, citizen_tax, visitor_tax, settlement,
		        nation_ally, marechal_ally, ally_date, nation_gold
		   FROM nations WHERE nation_id BETWEEN 1 AND 3 ORDER BY nation_id`)
	if err != nil {
		return nil, fmt.Errorf("store: carregar nacoes: %w", err)
	}
	defer rows.Close()
	var out []nation.State
	for rows.Next() {
		var (
			s                                nation.State
			id, rank, gm, gt, gj, gz, ct, vt int64
			settle, ally, date               int64
		)
		if err := rows.Scan(&id, &s.Name, &rank, &gm, &gt, &gj, &gz, &ct, &vt,
			&settle, &ally, &s.MarshalAllyName, &date, &s.Gold); err != nil {
			return out, fmt.Errorf("store: ler nacao: %w", err)
		}
		s.ID = uint8(id)
		s.Rank = clampToByte(rank)
		s.MarshalGuild, s.TacticianGuild = uint16(gm), uint16(gt)
		s.JudgeGuild, s.TreasurerGuild = uint16(gj), uint16(gz)
		s.CitizenTax, s.VisitorTax = uint16(ct), uint16(vt)
		s.Settlement, s.AllyNation, s.AllyDate = int32(settle), int32(ally), uint32(date)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveTaxes grava taxas e tesouro da nacao (SaveNationTaxes da origem, que
// montava o UPDATE por concatenacao de texto).
func (n Nations) SaveTaxes(ctx context.Context, st nation.State) error {
	_, err := n.DB.Exec(ctx,
		`UPDATE nations SET citizen_tax = ?, visitor_tax = ?, nation_gold = ? WHERE nation_id = ?`,
		st.CitizenTax, st.VisitorTax, st.Gold, st.ID)
	if err != nil {
		return fmt.Errorf("store: gravar taxas da nacao %d: %w", st.ID, err)
	}
	return nil
}

// SaveGrade grava as guildas de grade da nacao (marechal, estrategista, juiz e
// tesoureiro).
func (n Nations) SaveGrade(ctx context.Context, st nation.State) error {
	_, err := n.DB.Exec(ctx,
		`UPDATE nations SET guild_id_marshal = ?, guild_id_tactician = ?,
		        guild_id_judge = ?, guild_id_treasurer = ? WHERE nation_id = ?`,
		st.MarshalGuild, st.TacticianGuild, st.JudgeGuild, st.TreasurerGuild, st.ID)
	if err != nil {
		return fmt.Errorf("store: gravar grade da nacao %d: %w", st.ID, err)
	}
	return nil
}
