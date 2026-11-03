package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aika/internal/mail"
	"aika/internal/model"
)

// Mails le e grava o correio.
//
// A origem guardava a caixa em arquivo por personagem (Data/Mails) e a camada
// de leitura estava esvaziada: TMailFunctions.GetMails e so "Result := True"
// com o corpo comentado. O efeito, descrito em internal/mail, e que toda
// carta recebida apagava a caixa e nenhuma podia ser lida de volta. Aqui a
// caixa fica no banco, como o resto do estado de jogo.
type Mails struct {
	DB *DB
}

// Box le a caixa de um personagem, da mais antiga para a mais nova.
func (m Mails) Box(ctx context.Context, charID uint32) (*mail.Box, error) {
	box := &mail.Box{}
	if charID == 0 {
		return box, nil
	}

	const q = `SELECT mail_index, from_char_index, from_nick, titulo, texto,
	                  gold, sent_at, return_at, checked, item_return,
	                  check_item, leilao,
	                  item0_id, item0_app, item0_identific, item0_min, item0_max,
	                  item0_refine, item0_time,
	                  item1_id, item1_app, item1_identific, item1_min, item1_max,
	                  item1_refine, item1_time,
	                  item2_id, item2_app, item2_identific, item2_min, item2_max,
	                  item2_refine, item2_time,
	                  item3_id, item3_app, item3_identific, item3_min, item3_max,
	                  item3_refine, item3_time,
	                  item4_id, item4_app, item4_identific, item4_min, item4_max,
	                  item4_refine, item4_time
	             FROM mails
	            WHERE owner_char_index = :dono
	            ORDER BY mail_index
	            LIMIT 32000`

	rows, err := m.DB.Query(ctx, q, sql.Named("dono", charID))
	if err != nil {
		return box, fmt.Errorf("store: buscar correio de %d: %w", charID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			index                           int64
			fromChar                        int64
			nick, titulo, texto             sql.NullString
			gold                            int64
			enviado, retorno                sql.NullInt64
			lida, retornar, temItem, leilao int64
			itens                           [mail.MaxItems][7]int64
		)
		alvos := []any{&index, &fromChar, &nick, &titulo, &texto,
			&gold, &enviado, &retorno, &lida, &retornar, &temItem, &leilao}
		for i := range itens {
			for j := range itens[i] {
				alvos = append(alvos, &itens[i][j])
			}
		}
		if err := rows.Scan(alvos...); err != nil {
			return box, fmt.Errorf("store: ler carta: %w", err)
		}

		carta := mail.Mail{
			Index:     uint64(max(index, 0)),
			CharIndex: clampToDword(fromChar),
			Nick:      nick.String,
			Title:     titulo.String,
			Text:      texto.String,
			Gold:      clampToDword(gold),
			Checked:   lida != 0,
			Return:    retornar != 0,
			CheckItem: temItem != 0,
			Auction:   leilao != 0,
		}
		if enviado.Valid && enviado.Int64 > 0 {
			carta.SentAt = time.Unix(enviado.Int64, 0).UTC()
		}
		if retorno.Valid && retorno.Int64 > 0 {
			carta.ReturnAt = time.Unix(retorno.Int64, 0).UTC()
		}
		for i := range itens {
			carta.Items[i] = model.Item{
				Index:     clampToWord(itens[i][0]),
				APP:       clampToWord(itens[i][1]),
				Identific: clampToInt32(itens[i][2]),
				Min:       clampToByte(itens[i][3]),
				Max:       clampToByte(itens[i][4]),
				Refi:      clampToWord(itens[i][5]),
				Time:      clampToWord(itens[i][6]),
			}
		}
		if err := box.Add(carta); err != nil {
			// Caixa no teto: a origem tambem parava em 32000.
			break
		}
	}
	if err := rows.Err(); err != nil {
		return box, fmt.Errorf("store: ler correio: %w", err)
	}
	return box, nil
}

// Add poe uma carta na caixa de um personagem achado pelo nome.
func (m Mails) Add(ctx context.Context, nome string, carta mail.Mail) error {
	dono, ok, err := m.charIDByName(ctx, nome)
	if err != nil {
		return err
	}
	if !ok {
		return mail.ErrInvalidTarget
	}
	return m.insert(ctx, dono, carta)
}

// Save grava uma carta que mudou.
func (m Mails) Save(ctx context.Context, charID uint32, carta mail.Mail) error {
	if charID == 0 || carta.Index == 0 {
		return mail.ErrMailNotFound
	}
	q := `UPDATE mails SET gold=?, checked=?, item_return=?, check_item=?`
	args := []any{carta.Gold, b2i(carta.Checked), b2i(carta.Return), b2i(carta.CheckItem)}
	for i := range carta.Items {
		q += fmt.Sprintf(`, item%d_id=?, item%d_app=?, item%d_identific=?,
			item%d_min=?, item%d_max=?, item%d_refine=?, item%d_time=?`,
			i, i, i, i, i, i, i)
		it := carta.Items[i]
		args = append(args, it.Index, it.APP, it.Identific,
			it.Min, it.Max, it.Refi, it.Time)
	}
	q += ` WHERE owner_char_index=? AND mail_index=?`
	args = append(args, charID, carta.Index)

	if _, err := m.DB.SQL().ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("store: gravar carta %d: %w", carta.Index, err)
	}
	return nil
}

// Remove apaga uma carta.
func (m Mails) Remove(ctx context.Context, charID uint32, index uint64) error {
	if charID == 0 || index == 0 {
		return nil
	}
	_, err := m.DB.SQL().ExecContext(ctx,
		`DELETE FROM mails WHERE owner_char_index=? AND mail_index=?`,
		charID, index)
	if err != nil {
		return fmt.Errorf("store: apagar carta %d: %w", index, err)
	}
	return nil
}

// MarkRead marca a carta como lida.
func (m Mails) MarkRead(ctx context.Context, charID uint32, index uint64) error {
	if charID == 0 || index == 0 {
		return nil
	}
	_, err := m.DB.SQL().ExecContext(ctx,
		`UPDATE mails SET checked=1 WHERE owner_char_index=? AND mail_index=?`,
		charID, index)
	if err != nil {
		return fmt.Errorf("store: marcar carta %d lida: %w", index, err)
	}
	return nil
}

// NextIndex devolve o proximo indice de carta.
//
// A origem guardava esse contador num arquivo de 8 bytes
// (Data/Mails/LastIndex.t3c) lido e reescrito a cada envio, sem trava: dois
// envios ao mesmo tempo liam o mesmo valor e as duas cartas ficavam com o
// mesmo indice — e o indice e a chave que identifica a carta na caixa.
func (m Mails) NextIndex(ctx context.Context) (uint64, error) {
	var maior sql.NullInt64
	row := m.DB.SQL().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(mail_index), 0) FROM mails`)
	if err := row.Scan(&maior); err != nil {
		return 0, fmt.Errorf("store: proximo indice de carta: %w", err)
	}
	return uint64(max(maior.Int64, 0)) + 1, nil
}

// CharacterExists diz se o personagem existe.
func (m Mails) CharacterExists(ctx context.Context, nome string) (bool, error) {
	_, ok, err := m.charIDByName(ctx, nome)
	return ok, err
}

// charIDByName acha o id de um personagem pelo nome.
func (m Mails) charIDByName(ctx context.Context, nome string) (uint32, bool, error) {
	if nome == "" {
		return 0, false, nil
	}
	rows, err := m.DB.Query(ctx,
		`SELECT id FROM characters WHERE name = :nome AND deleted = 0 LIMIT 1`,
		sql.Named("nome", nome))
	if err != nil {
		return 0, false, fmt.Errorf("store: achar personagem %q: %w", nome, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, false, rows.Err()
	}
	var id int64
	if err := rows.Scan(&id); err != nil {
		return 0, false, fmt.Errorf("store: ler id de personagem: %w", err)
	}
	return clampToDword(id), true, nil
}

// insert grava uma carta nova.
func (m Mails) insert(ctx context.Context, dono uint32, carta mail.Mail) error {
	colunas := `owner_char_index, mail_index, from_char_index, from_nick,
		titulo, texto, gold, sent_at, return_at,
		checked, item_return, check_item, leilao`
	valores := `?,?,?,?,?,?,?,?,?,?,?,?,?`
	args := []any{dono, carta.Index, carta.CharIndex, carta.Nick,
		carta.Title, carta.Text, carta.Gold,
		unixOrZero(carta.SentAt), unixOrZero(carta.ReturnAt),
		b2i(carta.Checked), b2i(carta.Return), b2i(carta.CheckItem),
		b2i(carta.Auction)}

	for i := range carta.Items {
		colunas += fmt.Sprintf(`, item%d_id, item%d_app, item%d_identific,
			item%d_min, item%d_max, item%d_refine, item%d_time`,
			i, i, i, i, i, i, i)
		valores += ",?,?,?,?,?,?,?"
		it := carta.Items[i]
		args = append(args, it.Index, it.APP, it.Identific,
			it.Min, it.Max, it.Refi, it.Time)
	}

	q := "INSERT INTO mails (" + colunas + ") VALUES (" + valores + ")"
	if _, err := m.DB.SQL().ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("store: inserir carta para %d: %w", dono, err)
	}
	return nil
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
