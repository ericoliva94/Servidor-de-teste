package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GMAccount e uma linha de gm_accounts.
type GMAccount struct {
	ID             uint32
	Username       string
	Password       string
	Status         int // 0 = bloqueada por erros de senha
	PasswordErrors int
	MasterPriv     int
}

// GMAccounts le e grava gm_accounts e as acoes de painel que mexem em varias
// linhas.
type GMAccounts struct {
	DB *DB
}

// ByUsername acha a conta de painel.
func (g GMAccounts) ByUsername(ctx context.Context, username string) (GMAccount, bool, error) {
	var a GMAccount
	err := g.DB.SQL().QueryRowContext(ctx,
		`SELECT id, username, password, account_status, password_errors, master_priv
		   FROM gm_accounts WHERE username = ?`, username).
		Scan(&a.ID, &a.Username, &a.Password, &a.Status, &a.PasswordErrors, &a.MasterPriv)
	if err == sql.ErrNoRows {
		return a, false, nil
	}
	if err != nil {
		return a, false, fmt.Errorf("store: conta de painel %q: %w", username, err)
	}
	return a, true, nil
}

// SetPasswordErrors grava o contador de erros de senha.
func (g GMAccounts) SetPasswordErrors(ctx context.Context, username string, n int) error {
	_, err := g.DB.Exec(ctx, `UPDATE gm_accounts SET password_errors = ? WHERE username = ?`, n, username)
	if err != nil {
		return fmt.Errorf("store: erros de senha do painel: %w", err)
	}
	return nil
}

// Block bloqueia a conta de painel (account_status = 0).
func (g GMAccounts) Block(ctx context.Context, username string) error {
	_, err := g.DB.Exec(ctx, `UPDATE gm_accounts SET account_status = 0 WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("store: bloquear conta de painel: %w", err)
	}
	return nil
}

// AddEventItemToAll poe um item de evento na fila de todos os personagens, num
// INSERT so (a origem fazia um INSERT por personagem). Devolve quantos
// personagens receberam.
func (g GMAccounts) AddEventItemToAll(ctx context.Context, itemID, amount uint32) (int64, error) {
	res, err := g.DB.Exec(ctx,
		`INSERT INTO items (slot_type, owner_id, slot, item_id, refine)
		 SELECT ?, id, 0, ?, ? FROM characters`, EventSlotType, itemID, amount)
	if err != nil {
		return 0, fmt.Errorf("store: item de evento para todos: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// BanAccount bane a conta: account_status 8 e, se nao for permanente, os dias.
func (g GMAccounts) BanAccount(ctx context.Context, accountID uint32, days uint16, permanent bool) error {
	var err error
	if permanent {
		_, err = g.DB.Exec(ctx, `UPDATE accounts SET account_status = 8 WHERE id = ?`, accountID)
	} else {
		_, err = g.DB.Exec(ctx, `UPDATE accounts SET account_status = 8, ban_days = ? WHERE id = ?`, days, accountID)
	}
	if err != nil {
		return fmt.Errorf("store: banir conta %d: %w", accountID, err)
	}
	return nil
}

// Tipos de comando de painel (command_type de gm_commands).
const (
	GMCmdLevel   = 1 // LEVEL_GMCOMMAND: target_name + target_itemcnt niveis
	GMCmdItem    = 2 // ITEM_GMCOMMAND: item de evento
	GMCmdCoupon  = CouponCash
	GMCmdFounder = CouponItem
)

// GMCommand e uma linha de gm_commands esperando aprovacao (ou ja decidida).
type GMCommand struct {
	ID        uint32
	OwnerID   uint32
	OwnerName string
	Type      int
	Command   string
	CreatedAt time.Time
	Target    string
	ItemID    int64
	ItemCount int64
	Reason    string
	Runned    bool
	Refused   bool
}

// CreateCommand grava um pedido que espera a aprovacao de um administrador.
// Os valores vao como parametros. O texto em command e so informativo: o
// servidor nunca o executa.
func (g GMAccounts) CreateCommand(ctx context.Context, c GMCommand) (uint32, error) {
	res, err := g.DB.Exec(ctx,
		`INSERT INTO gm_commands (owner_gmid, command_type, runned, command, created_at,
		   runned_at, runned_by, target_name, target_itemid, target_itemcnt, refused,
		   refused_at, reason_run, reason_refuse, reason_create, coupom)
		 VALUES (?, ?, 0, ?, ?, '1899-12-30', '', ?, ?, ?, 0, '1899-12-30', '', '', ?, ?)`,
		c.OwnerID, c.Type, c.Command, c.CreatedAt.Format("2006-01-02 15:04:05"),
		c.Target, c.ItemID, c.ItemCount, c.Reason, couponOf(c))
	if err != nil {
		return 0, fmt.Errorf("store: criar comando de painel: %w", err)
	}
	id, _ := res.LastInsertId()
	return uint32(id), nil
}

// couponOf: para cupons o proprio codigo (guardado em command) tambem vai na
// coluna coupom, como a origem.
func couponOf(c GMCommand) string {
	if c.Type == GMCmdCoupon || c.Type == GMCmdFounder {
		return c.Command
	}
	return ""
}

// GMIDByUsername acha o id da conta de painel.
func (g GMAccounts) GMIDByUsername(ctx context.Context, username string) (uint32, bool, error) {
	var id uint32
	err := g.DB.SQL().QueryRowContext(ctx, `SELECT id FROM gm_accounts WHERE username = ?`, username).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return id, err == nil, err
}

// PendingCommands lista os pedidos que ainda nao foram aprovados nem recusados.
func (g GMAccounts) PendingCommands(ctx context.Context) ([]GMCommand, error) {
	rows, err := g.DB.Query(ctx,
		`SELECT c.id, c.owner_gmid, COALESCE(a.username, ''), c.command_type, COALESCE(c.command, ''),
		        c.created_at, c.target_name, c.target_itemid, c.target_itemcnt, c.reason_create
		   FROM gm_commands c LEFT JOIN gm_accounts a ON a.id = c.owner_gmid
		  WHERE c.runned = 0 AND c.refused = 0 ORDER BY c.id`)
	if err != nil {
		return nil, fmt.Errorf("store: comandos pendentes: %w", err)
	}
	defer rows.Close()
	var out []GMCommand
	for rows.Next() {
		var c GMCommand
		var criado sql.NullTime
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.OwnerName, &c.Type, &c.Command, &criado,
			&c.Target, &c.ItemID, &c.ItemCount, &c.Reason); err != nil {
			return out, fmt.Errorf("store: ler comando: %w", err)
		}
		c.CreatedAt = criado.Time
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClaimCommand marca o pedido como aprovado, so se ele ainda esta pendente, e o
// devolve. Quem aprova primeiro leva: dois administradores aprovando o mesmo
// pedido nao o aplicam duas vezes (a origem lia, aplicava e so depois marcava).
func (g GMAccounts) ClaimCommand(ctx context.Context, id uint32, by, reason string, now time.Time) (GMCommand, bool, error) {
	tx, err := g.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return GMCommand{}, false, fmt.Errorf("store: abrir transacao de aprovacao: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	var c GMCommand
	err = tx.QueryRowContext(ctx,
		`SELECT id, command_type, COALESCE(command, ''), target_name, target_itemid, target_itemcnt
		   FROM gm_commands WHERE id = ? AND runned = 0 AND refused = 0 FOR UPDATE`, id).
		Scan(&c.ID, &c.Type, &c.Command, &c.Target, &c.ItemID, &c.ItemCount)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("store: ler comando %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE gm_commands SET runned = 1, reason_run = ?, runned_at = ?, runned_by = ?,
		   command = CONCAT(command, ?), coupom = ? WHERE id = ?`,
		reason, now.Format("2006-01-02 15:04:05"), by, "\r\nMotivo Aprovado: "+reason,
		couponOf(c), id); err != nil {
		return c, false, fmt.Errorf("store: aprovar comando %d: %w", id, err)
	}
	return c, true, tx.Commit()
}

// RefuseCommand marca o pedido como recusado, so se ainda estiver pendente.
func (g GMAccounts) RefuseCommand(ctx context.Context, id uint32, by, reason string, now time.Time) (bool, error) {
	res, err := g.DB.Exec(ctx,
		`UPDATE gm_commands SET refused = 1, reason_refuse = ?, refused_at = ?, runned_by = ?,
		   command = CONCAT(command, ?) WHERE id = ? AND runned = 0 AND refused = 0`,
		reason, now.Format("2006-01-02 15:04:05"), by, "\r\nMotivo Reprovado: "+reason, id)
	if err != nil {
		return false, fmt.Errorf("store: recusar comando %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// OfflineCharacter acha um personagem pelo nome (para aprovar nivel e item com
// o jogador fora do jogo).
func (g GMAccounts) OfflineCharacter(ctx context.Context, name string) (id uint32, level uint16, found bool, err error) {
	err = g.DB.SQL().QueryRowContext(ctx, `SELECT id, level FROM characters WHERE name = ?`, name).Scan(&id, &level)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	return id, level, err == nil, err
}

// SetLevelExp grava nivel e experiencia de um personagem fora do jogo.
func (g GMAccounts) SetLevelExp(ctx context.Context, charID uint32, level uint16, exp uint64) error {
	_, err := g.DB.Exec(ctx, `UPDATE characters SET level = ?, experience = ? WHERE id = ?`, level, exp, charID)
	if err != nil {
		return fmt.Errorf("store: gravar nivel: %w", err)
	}
	return nil
}

// UsernamesOfGM lista os nomes das contas de painel.
func (g GMAccounts) GMUsernames(ctx context.Context) ([]string, error) {
	rows, err := g.DB.Query(ctx, `SELECT username FROM gm_accounts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: contas de painel: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return out, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
