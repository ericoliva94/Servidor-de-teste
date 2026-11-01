package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"aika/internal/auth"
)

// Accounts le a tabela accounts do aika_db4.
type Accounts struct {
	DB *DB
}

// ByUsername busca a conta pelo nome de usuario.
//
// As colunas sao as de Server/Database/aika_db4.sql. last_token_creation_time
// e unix em BIGINT, como o Delphi grava.
func (a Accounts) ByUsername(ctx context.Context, username string) (auth.Account, bool, error) {
	const q = `
		SELECT id, username, last_token, last_token_creation_time,
		       account_status, nation
		  FROM accounts
		 WHERE username = :username
		 LIMIT 1`

	rows, err := a.DB.Query(ctx, q, sql.Named("username", username))
	if err != nil {
		return auth.Account{}, false, fmt.Errorf("store: buscar conta: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return auth.Account{}, false, fmt.Errorf("store: buscar conta: %w", err)
		}
		return auth.Account{}, false, nil
	}

	var (
		acc          auth.Account
		tokenCreated int64
		status       int64
		nation       int64
		token        sql.NullString
		user         sql.NullString
	)
	if err := rows.Scan(&acc.ID, &user, &token, &tokenCreated, &status, &nation); err != nil {
		return auth.Account{}, false, fmt.Errorf("store: ler conta: %w", err)
	}
	if err := rows.Err(); err != nil {
		return auth.Account{}, false, fmt.Errorf("store: ler conta: %w", err)
	}

	acc.Username = user.String
	acc.Token = token.String
	// Unix zero significa "sem token", nao 1970: deixar a data zerada faria o
	// token parecer velhissimo, que e o resultado certo, mas uma data zero
	// explicita evita depender disso.
	if tokenCreated > 0 {
		acc.TokenCreated = time.Unix(tokenCreated, 0)
	}
	acc.Status = clampToByte(status)
	acc.Nation = clampToByte(nation)
	return acc, true, nil
}

// clampToByte converte a coluna INT para o byte que o protocolo usa, sem
// estourar. account_status e nation sao INT no banco mas BYTE no pacote.
func clampToByte(v int64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// ErrNoAccounts indica tabela de contas inacessivel.
var ErrNoAccounts = errors.New("store: tabela de contas inacessivel")
