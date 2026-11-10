package store

import (
	"context"
	"fmt"

	"aika/internal/siege"
)

// Siege le e grava o cadastro do cerco de cada nacao (tabela nation_siege).
type Siege struct {
	DB *DB
}

// Lado do cadastro: 0 defesa, 1 a 4 os atacantes (slot + 1).
// Cargo: 0 marechal, 1 estrategista, 2 juiz, 3 tesoureiro.

// Load devolve o cadastro da nacao.
func (s Siege) Load(ctx context.Context, nationID uint8) (siege.Cerco, error) {
	var c siege.Cerco
	rows, err := s.DB.Query(ctx,
		`SELECT side, role, guild_name FROM nation_siege WHERE nation_id = ?`, nationID)
	if err != nil {
		return c, fmt.Errorf("store: carregar cerco da nacao %d: %w", nationID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var side, role int
		var name string
		if err := rows.Scan(&side, &role, &name); err != nil {
			return c, fmt.Errorf("store: ler cerco: %w", err)
		}
		if side < 0 || side > siege.MaxAttackers || role < 0 || role > 3 {
			continue
		}
		a := &c.Defenders
		if side > 0 {
			a = &c.Attackers[side-1]
		}
		switch role {
		case 0:
			a.Marshal = name
		case 1:
			a.Tactician = name
		case 2:
			a.Judge = name
		case 3:
			a.Treasurer = name
		}
	}
	return c, rows.Err()
}

// Save troca o cadastro da nacao pelo dado, numa transacao.
func (s Siege) Save(ctx context.Context, nationID uint8, c siege.Cerco) error {
	tx, err := s.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao do cerco: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM nation_siege WHERE nation_id = ?`, nationID); err != nil {
		return fmt.Errorf("store: limpar cerco: %w", err)
	}
	put := func(side int, a siege.Alliance) error {
		for role, name := range [4]string{a.Marshal, a.Tactician, a.Judge, a.Treasurer} {
			if name == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO nation_siege (nation_id, side, role, guild_name) VALUES (?, ?, ?, ?)`,
				nationID, side, role, name); err != nil {
				return fmt.Errorf("store: gravar cerco: %w", err)
			}
		}
		return nil
	}
	if err := put(0, c.Defenders); err != nil {
		return err
	}
	for i, a := range c.Attackers {
		if err := put(i+1, a); err != nil {
			return err
		}
	}
	return tx.Commit()
}
