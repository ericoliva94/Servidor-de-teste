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
		       account_status, nation, ban_days,
		       COALESCE(account_type, 0) AS account_type,
		       COALESCE(storage_gold, 0) AS storage_gold,
		       COALESCE(cash, 0) AS cash
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
		accType      int64
		storageGold  int64
		cash         int64
	)
	if err := rows.Scan(&acc.ID, &user, &pwHash, &token, &tokenCreated,
		&status, &nation, &banDays, &accType, &storageGold, &cash); err != nil {
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
	acc.Type = clampToByte(accType)
	// A origem lia storage_gold com AsInteger (32 bits) num campo Int64: um
	// armazem com mais de 2.147.483.647 de ouro voltava do banco truncado, e
	// acima de 4.294.967.295 voltava negativo. Aqui o valor vem inteiro e
	// so o negativo, que nao existe em jogo, e cortado.
	acc.StorageGold = max(storageGold, 0)
	// Cash e Cardinal na origem; um valor negativo no banco virava um numero
	// enorme ao ser lido para o campo sem sinal.
	acc.Cash = clampToDwordAuth(cash)
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

// clampToDwordAuth corta um valor do banco na faixa do DWORD sem sinal.
func clampToDwordAuth(v int64) uint32 {
	if v <= 0 {
		return 0
	}
	if v > 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	return uint32(v)
}

// SaveCash grava o saldo premium de uma conta.
//
// A origem gravava este campo no mesmo UPDATE gigante do personagem
// (Player.pas:2661), montado por concatenacao de texto, e so quando
// SaveInGame era chamado: uma compra feita pouco antes de o servidor cair
// cobrava o cash e nao entregava o item, ou o contrario.
func (a Accounts) SaveCash(ctx context.Context, accountID uint32, cash uint32) error {
	if accountID == 0 {
		return fmt.Errorf("store: gravar cash sem conta")
	}
	const q = `
		UPDATE accounts
		   SET cash = :cash
		 WHERE id = :id`
	if _, err := a.DB.Exec(ctx, q,
		sql.Named("cash", cash),
		sql.Named("id", accountID)); err != nil {
		return fmt.Errorf("store: gravar cash: %w", err)
	}
	return nil
}

// SaveStorageGold grava o ouro do armazem de uma conta.
//
// A origem gravava este campo no mesmo UPDATE gigante do personagem, montado
// por concatenacao de texto (Player.pas:2661), e so quando TPlayer.SaveInGame
// era chamado: quem movia ouro para o armazem e caia antes da proxima
// gravacao perdia o deposito.
func (a Accounts) SaveStorageGold(ctx context.Context, accountID uint32, gold int64) error {
	if accountID == 0 {
		return fmt.Errorf("store: gravar ouro de armazem sem conta")
	}
	const q = `
		UPDATE accounts
		   SET storage_gold = :ouro
		 WHERE id = :id`
	_, err := a.DB.Exec(ctx, q,
		sql.Named("ouro", max(gold, 0)),
		sql.Named("id", accountID))
	if err != nil {
		return fmt.Errorf("store: gravar ouro de armazem: %w", err)
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
