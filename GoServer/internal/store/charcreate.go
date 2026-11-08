package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Erros da criacao de personagem.
var (
	ErrNameTaken = errors.New("store: ja existe um personagem com esse nome")
	ErrSlotTaken = errors.New("store: slot ocupado")
	ErrCharLimit = errors.New("store: a conta ja tem o maximo de personagens")
)

// FullVitals e o valor de HP e MP gravado num personagem novo: acima de
// qualquer maximo possivel, para o login limitar ao maximo calculado. A origem
// gravava zero, e a entrada no mundo trata zero como 1 de vida.
const FullVitals = 0x7FFFFFFF

// NewCharacter e a linha de characters de um personagem recem-criado.
type NewCharacter struct {
	Name       string
	Slot       int
	ClassInfo  uint8
	Str, Agi   uint16
	Int, Cons  uint16
	Luck       uint16
	Status     uint16
	Altura     uint8
	Tronco     uint8
	Perna      uint8
	Corpo      uint8
	Exp        int64
	Level      uint16
	Gold       int64
	PosX, PosY float32
}

// Create grava o personagem e devolve o id. Tudo numa transacao: conta no
// limite, slot livre, nome livre e o INSERT. Porte de TPlayer.VerifyAmount,
// NameExists e da primeira parte de SaveCreatedChar.
//
// A origem conferia so a quantidade de personagens e gravava no slot pedido
// mesmo ocupado (duas linhas no mesmo slot), checava o nome em consulta
// separada (dois pedidos iguais passavam) e depois buscava o id por nome. Aqui
// a conferencia e a gravacao ficam na mesma transacao, o nome unico e
// garantido tambem pela chave da tabela e o id vem do proprio INSERT.
func (c Characters) Create(ctx context.Context, accountID uint32, n NewCharacter) (uint32, error) {
	if accountID == 0 || n.Name == "" {
		return 0, fmt.Errorf("store: criar personagem com dados invalidos")
	}
	tx, err := c.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: abrir transacao de criacao: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // sem efeito depois do Commit

	count := func(q string, args ...any) (int, error) {
		var v int
		err := tx.QueryRowContext(ctx, q, args...).Scan(&v)
		return v, err
	}
	total, err := count(`SELECT COUNT(*) FROM characters WHERE owner_accid = ? FOR UPDATE`, accountID)
	if err != nil {
		return 0, fmt.Errorf("store: contar personagens: %w", err)
	}
	if total >= MaxCharacterSlots {
		return 0, ErrCharLimit
	}
	if v, err := count(`SELECT COUNT(*) FROM characters WHERE owner_accid = ? AND slot = ?`, accountID, n.Slot); err != nil {
		return 0, fmt.Errorf("store: conferir slot: %w", err)
	} else if v > 0 {
		return 0, ErrSlotTaken
	}
	if v, err := count(`SELECT COUNT(*) FROM characters WHERE name = ?`, n.Name); err != nil {
		return 0, fmt.Errorf("store: conferir nome: %w", err)
	} else if v > 0 {
		return 0, ErrNameTaken
	}

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO characters
		   (owner_accid, name, slot, classinfo, strength, agility, intelligence,
		    constitution, luck, status, altura, tronco, perna, corpo,
		    curhp, curmp, experience, level, gold, posx, posy,
		    creationtime, last_diary_event)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		accountID, n.Name, n.Slot, n.ClassInfo, n.Str, n.Agi, n.Int,
		n.Cons, n.Luck, n.Status, n.Altura, n.Tronco, n.Perna, n.Corpo,
		FullVitals, FullVitals, n.Exp, n.Level, n.Gold, n.PosX, n.PosY,
		now, fmt.Sprint(now))
	if err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return 0, ErrNameTaken
		}
		return 0, fmt.Errorf("store: gravar personagem: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("store: id do personagem novo: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: confirmar criacao: %w", err)
	}
	return uint32(id), nil
}

// EnsureStorageBag poe a expansao inicial do armazem (item 5310, slot 80) se a
// conta ainda nao tem nada nesse slot. A origem fazia
// "Storage.Itens[80].Index := 5310" em memoria a cada criacao, por cima do que
// estivesse ali.
func (c Characters) EnsureStorageBag(ctx context.Context, accountID uint32, itemID uint16, slot int) error {
	if accountID == 0 {
		return fmt.Errorf("store: armazem sem conta")
	}
	_, err := c.DB.Exec(ctx,
		`INSERT INTO items (slot_type, owner_id, slot, item_id, app)
		 SELECT :tipo, :acc, :slot, :item, :item FROM DUAL
		  WHERE NOT EXISTS (SELECT 1 FROM items WHERE slot_type = :tipo AND owner_id = :acc AND slot = :slot)`,
		sql.Named("tipo", SlotTypeStorage), sql.Named("acc", accountID),
		sql.Named("slot", slot), sql.Named("item", itemID))
	if err != nil {
		return fmt.Errorf("store: expansao inicial do armazem: %w", err)
	}
	return nil
}
