package store

import (
	"context"
	"database/sql"
	"fmt"

	"aika/internal/model"
)

// Tipos de slot da tabela items, as constantes *_TYPE de GlobalDefs.pas.
const (
	SlotTypeEquip      = 0  // EQUIP_TYPE
	SlotTypeInventory  = 1  // INV_TYPE
	SlotTypeStorage    = 2  // STORAGE_TYPE
	SlotTypeGuildChest = 3  // GUILDCHEST_TYPE
	SlotTypePranEquip  = 5  // PRAN_EQUIP_TYPE
	SlotTypePranInv    = 6  // PRAN_INV_TYPE
	SlotTypeItemUse    = 9  // ITEM_USE_TYPE
	SlotTypeCash       = 10 // CASH_TYPE
	SlotTypeVIPStorage = 21 // VIP_STORAGE_TYPE
)

// Tamanhos dos vetores de item, iguais aos do registro do personagem.
const (
	EquipSlots      = 16 // Equip: ARRAY [0..15] OF TItem
	InventorySlots  = 64 // Inventory: ARRAY [0..63] OF TItem
	StorageSlots    = 86 // o LIMIT 86 da consulta de armazem
	CashSlots       = 24 // o LIMIT 24 da consulta de premium
	GuildChestSlots = 50 // Chest.Items: ARRAY [0..49] OF TITEM
	PranEquipSlots  = 6
	PranInvSlots    = 16
)

// Items le a tabela items.
type Items struct {
	DB *DB
}

// ItemSet e um conjunto de itens indexado pelo slot.
type ItemSet struct {
	Slots []model.Item
	// Avisos descreve o que foi ignorado: slot fora da faixa, item repetido.
	Avisos []string
}

// Get devolve o item do slot, ou o item vazio fora da faixa.
func (s ItemSet) Get(slot int) model.Item {
	if slot < 0 || slot >= len(s.Slots) {
		return model.Item{}
	}
	return s.Slots[slot]
}

// itemColumns sao as colunas que todas as consultas de item leem.
const itemColumns = `slot, item_id, app, identific,
	effect1_index, effect2_index, effect3_index,
	effect1_value, effect2_value, effect3_value,
	min, max, refine, time`

// ByOwner le os itens de um dono num tipo de slot.
//
// owner e o id do personagem para equipamento e inventario, e o id da conta
// para armazem e premium — e assim na origem, que passava
// Account.Header.AccountId nas consultas de slot_type 2 e 10.
//
// Diferenca central em relacao a origem: o slot vinha do banco e indexava
// "Equip[ItemSlot]" e "Inventory[ItemSlot]" sem nenhuma checagem de faixa.
// Um slot adulterado, ou so errado, escrevia fora do vetor, no meio do
// registro do personagem — e esse registro e o que vai no $925 e e gravado de
// volta. Aqui o slot fora da faixa e ignorado e reportado.
func (it Items) ByOwner(ctx context.Context, owner uint32, slotType int, size int) (ItemSet, error) {
	out := ItemSet{Slots: make([]model.Item, size)}
	if owner == 0 {
		// Dono zero nao existe. A origem consultava "owner_id = 0" e aceitava
		// o que viesse: lixo de uma linha orfa entrava como equipamento.
		return out, nil
	}

	q := `SELECT ` + itemColumns + `
		  FROM items
		 WHERE owner_id = :owner AND slot_type = :tipo
		 ORDER BY slot
		 LIMIT ` + fmt.Sprint(size*2)

	rows, err := it.DB.Query(ctx, q,
		sql.Named("owner", owner), sql.Named("tipo", slotType))
	if err != nil {
		return out, fmt.Errorf("store: buscar itens (dono %d, tipo %d): %w",
			owner, slotType, err)
	}
	defer rows.Close()

	vistos := make(map[int]bool, size)
	for rows.Next() {
		var r itemRow
		if err := rows.Scan(&r.Slot, &r.ItemID, &r.APP, &r.Identific,
			&r.E1Index, &r.E2Index, &r.E3Index,
			&r.E1Value, &r.E2Value, &r.E3Value,
			&r.Min, &r.Max, &r.Refine, &r.Time); err != nil {
			return out, fmt.Errorf("store: ler item: %w", err)
		}
		applyItemRow(&out, owner, slotType, r, vistos)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("store: ler itens: %w", err)
	}
	return out, nil
}

// itemRow e uma linha da tabela items como ela vem do banco: tudo em int64,
// porque as colunas sao INT e os campos de destino tem larguras diferentes.
type itemRow struct {
	Slot      int64
	ItemID    int64
	APP       int64
	Identific int64
	E1Index   int64
	E2Index   int64
	E3Index   int64
	E1Value   int64
	E2Value   int64
	E3Value   int64
	Min       int64
	Max       int64
	Refine    int64
	Time      int64
}

// applyItemRow poe uma linha no conjunto, recusando o que nao cabe.
//
// Separado da consulta para poder ser testado sem banco: e aqui que estao as
// checagens que a origem nao tinha.
func applyItemRow(out *ItemSet, owner uint32, slotType int, r itemRow, vistos map[int]bool) {
	size := len(out.Slots)
	if r.Slot < 0 || r.Slot >= int64(size) {
		// O slot vinha do banco e indexava "Equip[ItemSlot]" e
		// "Inventory[ItemSlot]" sem checagem: fora da faixa escrevia no meio
		// do registro do personagem, que e o que vai no $925 e e gravado de
		// volta.
		out.Avisos = append(out.Avisos, fmt.Sprintf(
			"dono %d tipo %d: slot %d fora de 0..%d, ignorado",
			owner, slotType, r.Slot, size-1))
		return
	}
	if r.ItemID == 0 {
		// A origem pulava item_id zero no inventario e o aceitava no
		// equipamento, onde ele zerava o slot. O efeito e o mesmo de nao ter
		// item, entao aqui pula sempre.
		return
	}
	if vistos[int(r.Slot)] {
		// Duas linhas no mesmo slot: a origem deixava a ultima vencer em
		// silencio, e com isso um item desaparecia sem rastro.
		out.Avisos = append(out.Avisos, fmt.Sprintf(
			"dono %d tipo %d: slot %d repetido, mantido o primeiro",
			owner, slotType, r.Slot))
		return
	}
	vistos[int(r.Slot)] = true

	out.Slots[r.Slot] = model.Item{
		Index:     clampToWord(r.ItemID),
		APP:       clampToWord(r.APP),
		Identific: clampToInt32(r.Identific),
		Effects: model.ItemEffect{
			Index: [3]byte{
				clampToByte(r.E1Index), clampToByte(r.E2Index), clampToByte(r.E3Index)},
			Value: [3]byte{
				clampToByte(r.E1Value), clampToByte(r.E2Value), clampToByte(r.E3Value)},
		},
		Min:  clampToByte(r.Min),
		Max:  clampToByte(r.Max),
		Refi: clampToWord(r.Refine),
		Time: clampToWord(r.Time),
	}
}

// Equip le o equipamento de um personagem (slot_type 0).
func (it Items) Equip(ctx context.Context, charID uint32) (ItemSet, error) {
	return it.ByOwner(ctx, charID, SlotTypeEquip, EquipSlots)
}

// Inventory le o inventario de um personagem (slot_type 1).
func (it Items) Inventory(ctx context.Context, charID uint32) (ItemSet, error) {
	return it.ByOwner(ctx, charID, SlotTypeInventory, InventorySlots)
}

// Storage le o armazem de uma conta (slot_type 2).
func (it Items) Storage(ctx context.Context, accountID uint32) (ItemSet, error) {
	return it.ByOwner(ctx, accountID, SlotTypeStorage, StorageSlots)
}

// Cash le o premium de uma conta (slot_type 10).
func (it Items) Cash(ctx context.Context, accountID uint32) (ItemSet, error) {
	return it.ByOwner(ctx, accountID, SlotTypeCash, CashSlots)
}

// clampToInt32 converte coluna INT para o Integer de TItem.Identific.
func clampToInt32(v int64) int32 {
	if v < -2147483648 {
		return -2147483648
	}
	if v > 2147483647 {
		return 2147483647
	}
	return int32(v)
}

// SaveSet grava um conjunto de itens, substituindo o que estava la.
//
// Porte do bloco de gravacao de TPlayer.SaveInGame: a origem fazia
// "DELETE FROM items WHERE owner_id=X AND slot_type=Y" e depois um INSERT com
// todas as linhas, dentro de uma transacao. Mantida a mesma estrategia, que e
// a que casa com o formato da tabela (sem chave unica por slot).
//
// Diferenca: a origem montava o SQL concatenando texto, inclusive o
// Identific, que e Integer com sinal e vem de dados do jogador. Aqui os
// valores vao como parametros.
func (it Items) SaveSet(ctx context.Context, owner uint32, slotType int, set ItemSet) error {
	if owner == 0 {
		// Gravar com dono zero apagaria as linhas orfas de todo mundo.
		return fmt.Errorf("store: gravar itens sem dono (tipo %d)", slotType)
	}

	tx, err := it.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de itens: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // o Commit abaixo e o caminho bom

	const del = `DELETE FROM items WHERE owner_id = ? AND slot_type = ?`
	if _, err := tx.ExecContext(ctx, del, owner, slotType); err != nil {
		return fmt.Errorf("store: limpar itens (dono %d, tipo %d): %w",
			owner, slotType, err)
	}

	const ins = `INSERT INTO items
		(slot_type, owner_id, slot, item_id, app, identific,
		 effect1_index, effect1_value, effect2_index, effect2_value,
		 effect3_index, effect3_value, min, max, refine, time)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

	stmt, err := tx.PrepareContext(ctx, ins)
	if err != nil {
		return fmt.Errorf("store: preparar insercao de itens: %w", err)
	}
	defer stmt.Close()

	for slot, item := range set.Slots {
		if item.IsEmpty() {
			continue
		}
		_, err := stmt.ExecContext(ctx,
			slotType, owner, slot, item.Index, item.APP, item.Identific,
			item.Effects.Index[0], item.Effects.Value[0],
			item.Effects.Index[1], item.Effects.Value[1],
			item.Effects.Index[2], item.Effects.Value[2],
			item.Min, item.Max, item.Refi, item.Time)
		if err != nil {
			return fmt.Errorf("store: gravar item (dono %d, tipo %d, slot %d): %w",
				owner, slotType, slot, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar itens (dono %d, tipo %d): %w",
			owner, slotType, err)
	}
	return nil
}

// SaveEquip grava o equipamento de um personagem.
func (it Items) SaveEquip(ctx context.Context, charID uint32, set ItemSet) error {
	return it.SaveSet(ctx, charID, SlotTypeEquip, set)
}

// SaveInventory grava o inventario de um personagem.
func (it Items) SaveInventory(ctx context.Context, charID uint32, set ItemSet) error {
	return it.SaveSet(ctx, charID, SlotTypeInventory, set)
}

// SaveStorage grava o armazem de uma conta.
//
// A origem gravava o armazem dentro de TPlayer.SaveInGame, junto do
// personagem, com o id da CONTA como owner_id e slot_type 2.
func (it Items) SaveStorage(ctx context.Context, accountID uint32, set ItemSet) error {
	return it.SaveSet(ctx, accountID, SlotTypeStorage, set)
}

// SaveCash grava o inventario premium de uma conta.
func (it Items) SaveCash(ctx context.Context, accountID uint32, set ItemSet) error {
	return it.SaveSet(ctx, accountID, SlotTypeCash, set)
}

// GuildChest le os itens do bau de uma guilda.
//
// owner e o INDICE da guilda, nao o slot dela no vetor global: e o que a
// origem gravava em owner_id (Functions.pas:876, "Guilds[i].Index").
func (it Items) GuildChest(ctx context.Context, guildIndex uint32) (ItemSet, error) {
	return it.ByOwner(ctx, guildIndex, SlotTypeGuildChest, GuildChestSlots)
}

// SaveGuildChest grava os itens do bau de uma guilda.
func (it Items) SaveGuildChest(ctx context.Context, guildIndex uint32, set ItemSet) error {
	return it.SaveSet(ctx, guildIndex, SlotTypeGuildChest, set)
}
