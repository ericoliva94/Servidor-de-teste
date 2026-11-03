package store

import (
	"context"
	"database/sql"
	"fmt"

	"aika/internal/model"
)

// Tipos de linha da tabela skills, a coluna "type".
const (
	SkillTypeBasic = 1
	SkillTypeOther = 2
)

// Tamanhos dos vetores de skill do personagem, de PlayerData.pas.
const (
	BasicSkillSlots = 6  // Basics: ARRAY [0..5]
	OtherSkillSlots = 40 // Others: ARRAY [0..39]
)

// Skills le e grava a tabela skills.
type Skills struct {
	DB *DB
}

// ByCharacter le a arvore de skills de um personagem.
//
// Diferenca central: o slot vinha do banco e indexava
// "Skills.Basics[ItemSlot]" (6 posicoes) e "Skills.Others[ItemSlot]" (40)
// sem nenhuma checagem de faixa. Um slot de 40 ou mais escrevia depois do fim
// do vetor Others, dentro do registro do personagem — e esse registro e o que
// vai no $925 e e gravado de volta. Aqui o slot fora da faixa e ignorado e
// reportado.
func (sk Skills) ByCharacter(ctx context.Context, charID uint32) (model.SkillsList, []string, error) {
	var out model.SkillsList
	var avisos []string
	if charID == 0 {
		return out, nil, nil
	}

	const q = `SELECT slot, type, item, level
		  FROM skills
		 WHERE owner_charid = :char
		 ORDER BY slot
		 LIMIT 120`

	rows, err := sk.DB.Query(ctx, q, sql.Named("char", charID))
	if err != nil {
		return out, nil, fmt.Errorf("store: buscar skills (personagem %d): %w", charID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var slot, tipo, item, nivel int64
		if err := rows.Scan(&slot, &tipo, &item, &nivel); err != nil {
			return out, avisos, fmt.Errorf("store: ler skill: %w", err)
		}
		if item == 0 {
			// A origem pulava item zero; o efeito e o mesmo de slot vazio.
			continue
		}

		var limite int64
		switch tipo {
		case SkillTypeBasic:
			limite = BasicSkillSlots
		case SkillTypeOther:
			limite = OtherSkillSlots
		default:
			avisos = append(avisos, fmt.Sprintf(
				"personagem %d: skill com tipo %d desconhecido, ignorada", charID, tipo))
			continue
		}
		if slot < 0 || slot >= limite {
			avisos = append(avisos, fmt.Sprintf(
				"personagem %d: skill no slot %d fora de 0..%d (tipo %d), ignorada",
				charID, slot, limite-1, tipo))
			continue
		}

		entrada := model.SkillFromList{
			Index: clampToWord(item),
			Level: clampSkillLevel(nivel),
		}
		if nivel != int64(entrada.Level) {
			avisos = append(avisos, fmt.Sprintf(
				"personagem %d: skill %d com nivel %d fora da faixa, limitado a %d",
				charID, item, nivel, entrada.Level))
		}
		if tipo == SkillTypeBasic {
			out.Basics[slot] = entrada
		} else {
			out.Others[slot] = entrada
		}
	}
	if err := rows.Err(); err != nil {
		return out, avisos, fmt.Errorf("store: ler skills: %w", err)
	}
	return out, avisos, nil
}

// MaxSkillLevelDB e o maior nivel que um grupo de skill aceita.
//
// A origem fazia "inc(Level)" sem teto e o nivel ia para o banco como
// estava; na leitura, "Index + Level" com nivel acima de 15 entrava no grupo
// de skill seguinte da tabela.
const MaxSkillLevelDB = 15

// clampSkillLevel limita o nivel lido do banco.
func clampSkillLevel(v int64) uint16 {
	if v < 0 {
		return 0
	}
	if v > MaxSkillLevelDB {
		return MaxSkillLevelDB
	}
	return uint16(v)
}

// Save grava a arvore de skills, substituindo o que estava la.
//
// Porte do bloco de gravacao de SaveInGame, que fazia
// "DELETE FROM skills WHERE owner_charid=X" e depois um INSERT por linha.
func (sk Skills) Save(ctx context.Context, charID uint32, list model.SkillsList) error {
	if charID == 0 {
		return fmt.Errorf("store: gravar skills sem personagem")
	}

	tx, err := sk.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de skills: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // o Commit abaixo e o caminho bom

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM skills WHERE owner_charid = ?`, charID); err != nil {
		return fmt.Errorf("store: limpar skills (personagem %d): %w", charID, err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO skills (owner_charid, slot, type, item, level) VALUES (?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("store: preparar insercao de skills: %w", err)
	}
	defer stmt.Close()

	grava := func(slot int, tipo int, e model.SkillFromList) error {
		if e.Index == 0 {
			return nil
		}
		_, err := stmt.ExecContext(ctx, charID, slot, tipo, e.Index, e.Level)
		if err != nil {
			return fmt.Errorf("store: gravar skill (personagem %d, slot %d, tipo %d): %w",
				charID, slot, tipo, err)
		}
		return nil
	}

	for i := range list.Basics {
		if err := grava(i, SkillTypeBasic, list.Basics[i]); err != nil {
			return err
		}
	}
	for i := range list.Others {
		if err := grava(i, SkillTypeOther, list.Others[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar skills (personagem %d): %w", charID, err)
	}
	return nil
}
