package store

import (
	"context"
	"database/sql"
	"fmt"

	"aika/internal/pran"
)

// PranName e uma pran da conta: id da linha e nome ("" ate ser batizada).
type PranName struct {
	ID   uint32
	Name string
}

// Prans le e grava os nomes da tabela prans.
type Prans struct {
	DB *DB
}

// ByAccount devolve as duas primeiras prans da conta por id, as que a origem
// carregava como Pran1 e Pran2.
func (p Prans) ByAccount(ctx context.Context, accountID uint32) ([]PranName, error) {
	rows, err := p.DB.Query(ctx,
		`SELECT id, name FROM prans WHERE acc_id = ? ORDER BY id LIMIT 2`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: buscar prans da conta %d: %w", accountID, err)
	}
	defer rows.Close()
	var out []PranName
	for rows.Next() {
		var r PranName
		var nome sql.NullString
		if err := rows.Scan(&r.ID, &nome); err != nil {
			return out, fmt.Errorf("store: ler pran: %w", err)
		}
		r.Name = nome.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// NameTaken diz se alguma pran ja usa o nome.
func (p Prans) NameTaken(ctx context.Context, nome string) (bool, error) {
	rows, err := p.DB.Query(ctx, `SELECT 1 FROM prans WHERE name = ? LIMIT 1`, nome)
	if err != nil {
		return false, fmt.Errorf("store: conferir nome de pran: %w", err)
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// SetName batiza a pran. So vale enquanto ela esta sem nome, para dois
// pedidos simultaneos nao se sobrescreverem.
func (p Prans) SetName(ctx context.Context, id uint32, nome string) (bool, error) {
	res, err := p.DB.Exec(ctx, `UPDATE prans SET name = ? WHERE id = ? AND name = ''`, nome, id)
	if err != nil {
		return false, fmt.Errorf("store: batizar pran %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// LoadAccount le as duas primeiras prans da conta por inteiro: atributos,
// equipamento (16), bolsas (42), habilidades (10) e barra (3). Porte do bloco
// de prans de TPlayer.LoadAccountSQL.
func (p Prans) LoadAccount(ctx context.Context, accountID uint32) ([]*pran.Pran, error) {
	rows, err := p.DB.Query(ctx,
		`SELECT id, item_id, name, level, class, hp, max_hp, mp, max_mp, xp, def_p, def_m,
		        food, devotion, p_cute, p_smart, p_sexy, p_energetic, p_tough, p_corrupt,
		        width, chest, leg, created_at, updated_at
		   FROM prans WHERE acc_id = ? ORDER BY id LIMIT 2`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: carregar prans da conta %d: %w", accountID, err)
	}
	var out []*pran.Pran
	for rows.Next() {
		var (
			id, item, lvl, cls, hp, mhp, mp, mmp, xp, dp, dm int64
			food, dev, cu, sm, sx, en, to, co, w, ch, lg     int64
			created, updated                                 int64
			nome                                             sql.NullString
		)
		if err := rows.Scan(&id, &item, &nome, &lvl, &cls, &hp, &mhp, &mp, &mmp, &xp, &dp, &dm,
			&food, &dev, &cu, &sm, &sx, &en, &to, &co, &w, &ch, &lg, &created, &updated); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: ler pran: %w", err)
		}
		out = append(out, &pran.Pran{
			DBID: clampToDword(id), ItemID: clampToWord(item), AccID: accountID, Name: nome.String,
			Level: clampToByte(lvl), Class: clampToByte(cls),
			CurHP: clampToDword(hp), MaxHP: clampToDword(mhp),
			CurMP: clampToDword(mp), MaxMP: clampToDword(mmp), Exp: clampToDword(xp),
			DefFis: clampToWord(dp), DefMag: clampToWord(dm),
			Food: clampToByte(food), Devotion: clampToByte(dev),
			Cute: clampToWord(cu), Smart: clampToWord(sm), Sexy: clampToWord(sx),
			Energetic: clampToWord(en), Tough: clampToWord(to), Corrupt: clampToWord(co),
			Width: clampToByte(w), Chest: clampToByte(ch), Leg: clampToByte(lg),
			CreatedAt: clampToDword(created), UpdatedAt: clampToDword(updated),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ler prans: %w", err)
	}

	items := Items{DB: p.DB}
	bars := ItemBars{DB: p.DB}
	for _, pr := range out {
		eq, err := items.ByOwner(ctx, pr.DBID, pran.SlotTypeEquip, pran.EquipSlots)
		if err != nil {
			return out, err
		}
		copy(pr.Equip[:], eq.Slots)
		inv, err := items.ByOwner(ctx, pr.DBID, pran.SlotTypeInventory, pran.InventorySlots)
		if err != nil {
			return out, err
		}
		copy(pr.Inventory[:], inv.Slots)
		if err := p.loadSkills(ctx, pr); err != nil {
			return out, err
		}
		bar, err := bars.Load(ctx, pr.DBID+pran.BarOwnerBase)
		if err != nil {
			return out, err
		}
		copy(pr.ItemBar[:], bar[:pran.BarSlots])
	}
	return out, nil
}

// loadSkills le as habilidades (tipo 3). Slot fora de 0..9 e ignorado: a
// origem indexava o vetor com o valor do banco sem conferir.
func (p Prans) loadSkills(ctx context.Context, pr *pran.Pran) error {
	rows, err := p.DB.Query(ctx,
		`SELECT slot, item, level FROM skills WHERE owner_charid = ? AND type = ? ORDER BY slot LIMIT 10`,
		pr.DBID, pran.SkillType)
	if err != nil {
		return fmt.Errorf("store: habilidades da pran %d: %w", pr.DBID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var slot, item, lvl int64
		if err := rows.Scan(&slot, &item, &lvl); err != nil {
			return fmt.Errorf("store: ler habilidade de pran: %w", err)
		}
		if slot < 0 || slot >= pran.SkillSlots {
			continue
		}
		pr.Skills[slot] = pran.Skill{Index: clampToWord(item), Level: clampSkillLevel(lvl)}
	}
	return rows.Err()
}

// Save grava a pran: linha de atributos, equipamento, bolsas, habilidades e
// barra. Porte do bloco de prans de TPlayer.SaveInGame.
func (p Prans) Save(ctx context.Context, pr *pran.Pran) error {
	if pr == nil || pr.DBID == 0 {
		return fmt.Errorf("store: gravar pran sem id")
	}
	if _, err := p.DB.Exec(ctx,
		`UPDATE prans SET name = ?, food = ?, devotion = ?, p_cute = ?, p_smart = ?, p_sexy = ?,
		        p_energetic = ?, p_tough = ?, p_corrupt = ?, level = ?, class = ?, hp = ?, max_hp = ?,
		        mp = ?, max_mp = ?, xp = ?, def_p = ?, def_m = ?, width = ?, chest = ?, leg = ?,
		        updated_at = ?
		  WHERE id = ?`,
		pr.Name, pr.Food, pr.Devotion, pr.Cute, pr.Smart, pr.Sexy, pr.Energetic, pr.Tough, pr.Corrupt,
		pr.Level, pr.Class, pr.CurHP, pr.MaxHP, pr.CurMP, pr.MaxMP, pr.Exp, pr.DefFis, pr.DefMag,
		pr.Width, pr.Chest, pr.Leg, pr.UpdatedAt, pr.DBID); err != nil {
		return fmt.Errorf("store: gravar pran %d: %w", pr.DBID, err)
	}
	items := Items{DB: p.DB}
	if err := items.SaveSet(ctx, pr.DBID, pran.SlotTypeEquip, ItemSet{Slots: pr.Equip[:]}); err != nil {
		return err
	}
	if err := items.SaveSet(ctx, pr.DBID, pran.SlotTypeInventory, ItemSet{Slots: pr.Inventory[:]}); err != nil {
		return err
	}
	if err := p.saveSkills(ctx, pr); err != nil {
		return err
	}
	var bar [ItemBarSlots]uint32
	copy(bar[:], pr.ItemBar[:])
	return ItemBars{DB: p.DB}.Save(ctx, pr.DBID+pran.BarOwnerBase, bar)
}

func (p Prans) saveSkills(ctx context.Context, pr *pran.Pran) error {
	tx, err := p.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de habilidades da pran: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM skills WHERE owner_charid = ? AND type = ?`, pr.DBID, pran.SkillType); err != nil {
		return fmt.Errorf("store: limpar habilidades da pran: %w", err)
	}
	for slot, sk := range pr.Skills {
		if sk.Level == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO skills (owner_charid, slot, item, level, type) VALUES (?,?,?,?,?)`,
			pr.DBID, slot, sk.Index, sk.Level, pran.SkillType); err != nil {
			return fmt.Errorf("store: gravar habilidade da pran: %w", err)
		}
	}
	return tx.Commit()
}
