package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MaxCharacterSlots e o numero de personagens por conta
// (TAccountFile.Characters e ARRAY [0..2]).
const MaxCharacterSlots = 3

// Character e o que a lista de personagens precisa saber de cada slot.
type Character struct {
	ID         uint32
	Slot       int
	Name       string
	ClassInfo  uint16
	Level      uint16
	Exp        int64
	Gold       int64
	GuildIndex uint16

	Strength     uint16
	Agility      uint16
	Intelligence uint16
	Constitution uint16
	Luck         uint16

	Altura uint8
	Tronco uint8
	Perna  uint8
	Corpo  uint8

	NumericToken  string
	NumericErrors uint8
	Deleted       bool
	DeleteTime    time.Time
}

// Characters le a tabela characters.
type Characters struct {
	DB *DB
}

// ByAccount devolve os personagens de uma conta, indexados pelo slot.
//
// O slot vem do banco e e usado como indice de um vetor de 3 posicoes. Em
// Player.pas isso era
// "Self.Account.Characters[Query.FieldByName('slot').AsInteger].Index := ...",
// sem checar a faixa: um slot fora de 0..2 no banco escrevia fora do vetor,
// no meio do registro da conta. Aqui o slot invalido e ignorado e reportado.
func (c Characters) ByAccount(ctx context.Context, accountID uint32) ([MaxCharacterSlots]Character, []string, error) {
	var out [MaxCharacterSlots]Character
	var avisos []string

	const q = `
		SELECT id, slot, name, classinfo, level, experience, gold, guildindex,
		       strength, agility, intelligence, constitution, luck,
		       altura, tronco, perna, corpo,
		       numeric_token, numeric_errors, deleted, delete_time
		  FROM characters
		 WHERE owner_accid = :acc
		 ORDER BY slot
		 LIMIT 16`

	rows, err := c.DB.Query(ctx, q, sql.Named("acc", accountID))
	if err != nil {
		return out, nil, fmt.Errorf("store: buscar personagens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ch         Character
			slot       int64
			level      int64
			classInfo  int64
			guild      int64
			str, agi   int64
			intel, con int64
			luck       int64
			alt, tro   int64
			per, cor   int64
			name       sql.NullString
			numTok     sql.NullString
			numErr     int64
			deleted    int64
			deleteTime sql.NullString
		)
		if err := rows.Scan(&ch.ID, &slot, &name, &classInfo, &level, &ch.Exp,
			&ch.Gold, &guild, &str, &agi, &intel, &con, &luck,
			&alt, &tro, &per, &cor, &numTok, &numErr, &deleted, &deleteTime); err != nil {
			return out, avisos, fmt.Errorf("store: ler personagem: %w", err)
		}

		if slot < 0 || slot >= MaxCharacterSlots {
			avisos = append(avisos, fmt.Sprintf(
				"conta %d: personagem %d tem slot %d fora de 0..%d, ignorado",
				accountID, ch.ID, slot, MaxCharacterSlots-1))
			continue
		}

		ch.Slot = int(slot)
		ch.Name = name.String
		ch.ClassInfo = clampToWord(classInfo)
		ch.Level = clampToWord(level)
		ch.GuildIndex = clampToWord(guild)
		ch.Strength = clampToWord(str)
		ch.Agility = clampToWord(agi)
		ch.Intelligence = clampToWord(intel)
		ch.Constitution = clampToWord(con)
		ch.Luck = clampToWord(luck)
		ch.Altura = clampToByte(alt)
		ch.Tronco = clampToByte(tro)
		ch.Perna = clampToByte(per)
		ch.Corpo = clampToByte(cor)
		ch.NumericToken = numTok.String
		ch.NumericErrors = clampToByte(numErr)
		ch.Deleted = deleted != 0
		if deleteTime.Valid && deleteTime.String != "" {
			// A coluna e VARCHAR com a data em texto, como o Delphi grava.
			// StrToDateTime no original levantava excecao com texto invalido e
			// derrubava o envio da lista; aqui a data invalida fica zerada.
			if t, err := parseDelphiDateTime(deleteTime.String); err == nil {
				ch.DeleteTime = t
			} else {
				avisos = append(avisos, fmt.Sprintf(
					"conta %d slot %d: delete_time %q invalido",
					accountID, ch.Slot, deleteTime.String))
			}
		}
		out[ch.Slot] = ch
	}
	if err := rows.Err(); err != nil {
		return out, avisos, fmt.Errorf("store: ler personagens: %w", err)
	}
	return out, avisos, nil
}

// clampToWord converte coluna INT para o WORD do protocolo sem estourar.
func clampToWord(v int64) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

// formatos aceitos para a data em texto da coluna delete_time.
var delphiDateFormats = []string{
	"02/01/2006 15:04:05",
	"02/01/2006 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"02/01/2006",
	"2006-01-02",
}

func parseDelphiDateTime(s string) (time.Time, error) {
	for _, f := range delphiDateFormats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("store: data %q em formato desconhecido", s)
}
