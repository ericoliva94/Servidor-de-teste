package store

import (
	"context"
	"database/sql"
	"fmt"
)

// MaxFriends e o teto da lista de amigos (LIMIT 50 de readFriendList e o
// "Count >= 50" dos handlers).
const MaxFriends = 50

// FriendRow e um amigo como o banco o devolve.
type FriendRow struct {
	CharID uint32
	Name   string
}

// Friends le e grava a tabela friend_list.
//
// A origem abria uma conexao MySQL nova (TQuery.Create) a cada chamada de
// EntityFriend e gravava os dois lados da amizade em dois INSERT separados:
// se o segundo falhasse, a amizade ficava de um lado so. Aqui os dois lados
// vao na mesma transacao.
type Friends struct {
	DB *DB
}

// List devolve os amigos ativos de um personagem, do mais novo para o mais
// velho, no limite de MaxFriends (a mesma consulta de readFriendList).
func (f Friends) List(ctx context.Context, charID uint32) ([]FriendRow, error) {
	if charID == 0 {
		return nil, nil
	}
	const q = `SELECT friends.friend_characterId, characters.name
	             FROM friend_list AS friends
	            INNER JOIN characters ON friends.friend_characterId = characters.id
	            WHERE friends.active > 0 AND friends.owner_characterId = :dono
	            ORDER BY friends.id DESC
	            LIMIT 50`
	rows, err := f.DB.Query(ctx, q, sql.Named("dono", charID))
	if err != nil {
		return nil, fmt.Errorf("store: lista de amigos de %d: %w", charID, err)
	}
	defer rows.Close()

	var out []FriendRow
	seen := map[uint32]bool{}
	for rows.Next() {
		var r FriendRow
		if err := rows.Scan(&r.CharID, &r.Name); err != nil {
			return out, fmt.Errorf("store: ler amigo de %d: %w", charID, err)
		}
		// A origem ignorava a repeticao com ContainsKey; mantido.
		if seen[r.CharID] {
			continue
		}
		seen[r.CharID] = true
		out = append(out, r)
	}
	return out, rows.Err()
}

// Add grava a amizade nos dois sentidos, numa transacao. Linha ativa que ja
// existe nao e duplicada.
func (f Friends) Add(ctx context.Context, a, b uint32) error {
	if a == 0 || b == 0 || a == b {
		return fmt.Errorf("store: amizade invalida entre %d e %d", a, b)
	}
	tx, err := f.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de amizade: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	for _, par := range [2][2]uint32{{a, b}, {b, a}} {
		var existe int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM friend_list
			  WHERE owner_characterId = ? AND friend_characterId = ? AND active > 0`,
			par[0], par[1]).Scan(&existe); err != nil {
			return fmt.Errorf("store: conferir amizade: %w", err)
		}
		if existe > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO friend_list
			   (owner_characterId, friend_characterId, registerDate, lastUpdateDate)
			 VALUES (?, ?, NOW(), NOW())`, par[0], par[1]); err != nil {
			return fmt.Errorf("store: gravar amizade: %w", err)
		}
	}
	return tx.Commit()
}

// Remove desativa a amizade nos dois sentidos, numa transacao (a origem fazia
// dois UPDATE seguidos e saia no primeiro que falhasse, deixando metade).
func (f Friends) Remove(ctx context.Context, a, b uint32) error {
	if a == 0 || b == 0 {
		return fmt.Errorf("store: amizade invalida entre %d e %d", a, b)
	}
	tx, err := f.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de amizade: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	for _, par := range [2][2]uint32{{a, b}, {b, a}} {
		if _, err := tx.ExecContext(ctx,
			`UPDATE friend_list SET active = 0, lastUpdateDate = NOW()
			  WHERE owner_characterId = ? AND friend_characterId = ?`,
			par[0], par[1]); err != nil {
			return fmt.Errorf("store: desfazer amizade: %w", err)
		}
	}
	return tx.Commit()
}
