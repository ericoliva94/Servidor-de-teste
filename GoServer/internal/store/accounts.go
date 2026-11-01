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
		SELECT id, username, password_hash, last_token,
		       COALESCE(last_token_creation_time, 0) AS last_token_creation_time,
		       account_status, nation, ban_days
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
		pwHash       sql.NullString
		banDays      int64
	)
	if err := rows.Scan(&acc.ID, &user, &pwHash, &token, &tokenCreated,
		&status, &nation, &banDays); err != nil {
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
	acc.PasswordHash = pwHash.String
	acc.Status = clampToByte(status)
	acc.Nation = clampToByte(nation)
	acc.BanDays = int(banDays)
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

// SaveToken grava o token novo e o instante de criacao.
//
// Porte de TPlayer.SaveAccountToken. last_token_creation_time e unix em
// BIGINT, como o Delphi grava.
func (a Accounts) SaveToken(ctx context.Context, accountID uint32, token string, created time.Time) error {
	const q = `
		UPDATE accounts
		   SET last_token = :token,
		       last_token_creation_time = :criado
		 WHERE id = :id`
	_, err := a.DB.Exec(ctx, q,
		sql.Named("token", token),
		sql.Named("criado", created.Unix()),
		sql.Named("id", accountID))
	if err != nil {
		return fmt.Errorf("store: gravar token: %w", err)
	}
	return nil
}

// ClearBan zera a suspensao quando o prazo venceu.
//
// Porte do ramo de AikaGetToken que liberava a conta. No original os campos
// eram alterados no registro em memoria e gravados por SaveStatus; aqui e um
// UPDATE direto, que nao depende de o resto do registro estar carregado.
func (a Accounts) ClearBan(ctx context.Context, accountID uint32) error {
	const q = `
		UPDATE accounts
		   SET ban_days = 0,
		       account_status = 0
		 WHERE id = :id`
	if _, err := a.DB.Exec(ctx, q, sql.Named("id", accountID)); err != nil {
		return fmt.Errorf("store: liberar suspensao: %w", err)
	}
	return nil
}

// CharacterCount conta os personagens nao apagados da conta.
func (a Accounts) CharacterCount(ctx context.Context, accountID uint32) (int, error) {
	const q = `
		SELECT COUNT(*)
		  FROM characters
		 WHERE owner_accid = :id
		   AND deleted = 0`
	rows, err := a.DB.Query(ctx, q, sql.Named("id", accountID))
	if err != nil {
		return 0, fmt.Errorf("store: contar personagens: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("store: contar personagens: %w", err)
		}
		return 0, nil
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		return 0, fmt.Errorf("store: contar personagens: %w", err)
	}
	if n < 0 {
		return 0, nil
	}
	return int(n), nil
}
