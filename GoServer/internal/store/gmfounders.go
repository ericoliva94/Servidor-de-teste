package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Founder e um comprovante de pagamento (tabela founders).
type Founder struct {
	ID            uint32
	TransactionID string
	Name          string
	Date          time.Time
	Value         float64
	Coupon        string
	Validated     bool
	ValidatedBy   string
}

const founderSelect = `SELECT f.id, f.idtransaction, f.name, f.dateofcomprovant, f.valueofcomprovant,
	f.coupom, f.validated, COALESCE(a.username, '')
	FROM founders f LEFT JOIN gm_accounts a ON a.id = f.validated_gmid`

func scanFounders(rows *sql.Rows) ([]Founder, error) {
	defer rows.Close()
	var out []Founder
	for rows.Next() {
		var f Founder
		var d sql.NullTime
		var v int
		if err := rows.Scan(&f.ID, &f.TransactionID, &f.Name, &d, &f.Value, &f.Coupon, &v, &f.ValidatedBy); err != nil {
			return out, fmt.Errorf("store: ler comprovante: %w", err)
		}
		f.Date, f.Validated = d.Time, v != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// FoundersByTransaction acha o comprovante pelo id da transacao.
func (g GMAccounts) FoundersByTransaction(ctx context.Context, id string) ([]Founder, error) {
	rows, err := g.DB.Query(ctx, founderSelect+` WHERE f.idtransaction = ?`, strings.ToUpper(id))
	if err != nil {
		return nil, fmt.Errorf("store: buscar comprovante: %w", err)
	}
	return scanFounders(rows)
}

// FoundersByName busca comprovantes cujo nome contem o texto (ate 50). O texto
// vai como parametro; a origem o concatenava na consulta.
func (g GMAccounts) FoundersByName(ctx context.Context, nome string) ([]Founder, error) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(nome)
	rows, err := g.DB.Query(ctx, founderSelect+` WHERE f.name LIKE ? ORDER BY f.id LIMIT 50`, "%"+esc+"%")
	if err != nil {
		return nil, fmt.Errorf("store: buscar comprovante por nome: %w", err)
	}
	return scanFounders(rows)
}

// CreateFounder grava o comprovante e, se houver produto, o cupom de ativacao
// (ja aprovado), numa transacao. Devolve false se a transacao ja existe.
func (g GMAccounts) CreateFounder(ctx context.Context, f Founder, gmid uint32, product int64, now time.Time) (bool, error) {
	tx, err := g.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: abrir transacao do comprovante: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	var existe int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM founders WHERE idtransaction = ? FOR UPDATE`, f.TransactionID).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("store: conferir transacao: %w", err)
	}
	if existe > 0 {
		return false, nil
	}
	ts := now.Format("2006-01-02 15:04:05")
	if product > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO gm_commands (owner_gmid, command_type, runned, command, created_at, runned_at,
			   runned_by, target_name, target_itemid, target_itemcnt, refused, refused_at,
			   reason_run, reason_refuse, reason_create, coupom)
			 VALUES (?, ?, 1, ?, ?, ?, ?, '', ?, 1, 0, '1899-12-30', '', '', ?, ?)`,
			gmid, GMCmdFounder, f.Coupon, ts, ts, f.ValidatedBy, product,
			"Criação de cupom para recebimento de founder via comprovante.", f.Coupon); err != nil {
			return false, fmt.Errorf("store: cupom do comprovante: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO founders (idtransaction, name, dateofcomprovant, valueofcomprovant, coupom)
		 VALUES (?, ?, ?, ?, ?)`,
		f.TransactionID, f.Name, f.Date.Format("2006-01-02"), f.Value, f.Coupon); err != nil {
		return false, fmt.Errorf("store: gravar comprovante: %w", err)
	}
	return true, tx.Commit()
}

// ValidateFounder marca o comprovante como validado. found diz se existe e
// already se ja estava validado.
func (g GMAccounts) ValidateFounder(ctx context.Context, id uint32, gmid uint32) (found, already bool, err error) {
	var v int
	e := g.DB.SQL().QueryRowContext(ctx, `SELECT validated FROM founders WHERE id = ?`, id).Scan(&v)
	if e == sql.ErrNoRows {
		return false, false, nil
	}
	if e != nil {
		return false, false, fmt.Errorf("store: ler comprovante: %w", e)
	}
	if v == 1 {
		return true, true, nil
	}
	res, e := g.DB.Exec(ctx, `UPDATE founders SET validated = 1, validated_gmid = ? WHERE id = ? AND validated = 0`, gmid, id)
	if e != nil {
		return true, false, fmt.Errorf("store: validar comprovante: %w", e)
	}
	n, _ := res.RowsAffected()
	return true, n != 1, nil
}

// ResetPrans e o procedimento de GMRequestDeletePrans: para a conta que tem
// prans, apaga as quests de evolucao do personagem da pran (39, 40, 41, 406 e
// 407) e os itens 100 a 105 dele (equipado e inventario) e do armazem da conta.
// Nenhuma pran e apagada. Numa transacao; devolve false se a conta nao tem pran.
func (g GMAccounts) ResetPrans(ctx context.Context, accountID uint32) (bool, error) {
	tx, err := g.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: abrir transacao das prans: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	var charID uint32
	err = tx.QueryRowContext(ctx, `SELECT char_id FROM prans WHERE acc_id = ? ORDER BY id LIMIT 1`, accountID).Scan(&charID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: pran da conta: %w", err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM quests WHERE questid IN (39, 40, 41, 406, 407) AND charid = ?`, []any{charID}},
		{`DELETE FROM items WHERE owner_id = ? AND slot_type = 0 AND item_id IN (100,101,102,103,104,105)`, []any{charID}},
		{`DELETE FROM items WHERE owner_id = ? AND slot_type = 1 AND item_id IN (100,101,102,103,104,105)`, []any{charID}},
		{`DELETE FROM items WHERE owner_id = ? AND slot_type = 2 AND item_id IN (100,101,102,103,104,105)`, []any{accountID}},
	} {
		if _, err := tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return true, fmt.Errorf("store: zerar prans: %w", err)
		}
	}
	return true, tx.Commit()
}
