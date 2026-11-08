package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aika/internal/guild"
)

// Guilds le e grava as tabelas guilds e guilds_players.
type Guilds struct {
	DB *DB
}

// All le todas as guildas com os seus membros.
//
// Porte de TLoad (o "SELECT * FROM guilds") com LoadGuildMembersFromSQL.
//
// Diferenca central: ali os membros entravam em "Guilds[Slot].Members[R]" com
// R so incrementando por linha, SEM checar o limite do vetor ARRAY [0..127].
// Uma guilda com 129 linhas em guilds_players escrevia depois do fim de
// Members — e o que vem depois no registro e o bau da guilda. E o nome do
// membro era copiado com "Move(..., Length(nome))", sem limite: um nome de
// mais de 20 caracteres passava por cima de Rank, ClassInfo, Level e Logged.
func (g Guilds) All(ctx context.Context) ([]*guild.Guild, []string, error) {
	var avisos []string

	const q = `SELECT id, name, nation, exp, level, leader_char_index,
	                  notice1, notice2, notice3, site, ranks_config,
	                  bravure_points, skill_points, promote,
	                  COALESCE(storage_gold, 0) AS storage_gold,
	                  ally_leader, guild_ally1_index, guild_ally1_name, guild_ally2_index, guild_ally2_name,
	                  guild_ally3_index, guild_ally3_name, guild_ally4_index, guild_ally4_name
	             FROM guilds
	            ORDER BY id
	            LIMIT 10000`

	rows, err := g.DB.Query(ctx, q)
	if err != nil {
		return nil, nil, fmt.Errorf("store: buscar guildas: %w", err)
	}

	var guildas []*guild.Guild
	for rows.Next() {
		var (
			id                 int64
			nome               sql.NullString
			nation, exp, nivel int64
			lider              int64
			n1, n2, n3, site   sql.NullString
			ranks              sql.NullString
			bravura, skillPts  int64
			promote            int64
			ouroBau            int64
			allyLeader         int64
			allyIdx            [4]int64
			allyName           [4]sql.NullString
		)
		if err := rows.Scan(&id, &nome, &nation, &exp, &nivel, &lider,
			&n1, &n2, &n3, &site, &ranks,
			&bravura, &skillPts, &promote, &ouroBau,
			&allyLeader, &allyIdx[0], &allyName[0], &allyIdx[1], &allyName[1],
			&allyIdx[2], &allyName[2], &allyIdx[3], &allyName[3]); err != nil {
			rows.Close()
			return nil, avisos, fmt.Errorf("store: ler guilda: %w", err)
		}
		if id <= 0 || id > 65535 {
			avisos = append(avisos, fmt.Sprintf(
				"guilda com id %d fora da faixa do WORD, ignorada", id))
			continue
		}

		gd := &guild.Guild{
			Index:         uint16(id),
			Slot:          int(id),
			Nation:        clampToByte(nation),
			Exp:           clampToDword(exp),
			Level:         clampGuildLevel(nivel),
			Name:          nome.String,
			LeaderID:      clampToDword(lider),
			BravurePoints: clampToDword(bravura),
			SkillPoints:   clampToDword(skillPts),
			Promote:       promote != 0,
			Chest:         guild.NewChest(),
		}
		// O ouro do bau e UInt64 na origem mas vinha do banco por
		// "AsLargeInt" (Int64, Load.pas:158) e era gravado por ToString: um
		// valor negativo no banco virava um UInt64 enorme. Aqui o negativo e
		// zero e o teto e aplicado no SetGold.
		if gd.Chest.SetGold(uint64(max(ouroBau, 0))) {
			avisos = append(avisos, fmt.Sprintf(
				"guilda %d com %d de ouro no bau, acima do teto %d, limitado",
				id, ouroBau, guild.MaxChestGold))
		}
		if nivel != int64(gd.Level) {
			avisos = append(avisos, fmt.Sprintf(
				"guilda %d com nivel %d fora de 0..%d, limitado a %d",
				id, nivel, guild.MaxLevel, gd.Level))
		}
		gd.Ally.Leader = uint16(clampToWord(allyLeader))
		for i := range gd.Ally.Guilds {
			gd.Ally.Guilds[i] = guild.AllyItem{Index: uint16(clampToWord(allyIdx[i])), Name: allyName[i].String}
		}
		gd.SetNotices(n1.String, n2.String, n3.String)
		gd.SetSite(site.String)
		gd.SetRanksConfig(parseRanksConfig(ranks.String))
		guildas = append(guildas, gd)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, avisos, fmt.Errorf("store: ler guildas: %w", err)
	}

	for _, gd := range guildas {
		av, err := g.loadMembers(ctx, gd)
		avisos = append(avisos, av...)
		if err != nil {
			return guildas, avisos, err
		}
	}
	return guildas, avisos, nil
}

// loadMembers le os membros de uma guilda.
func (g Guilds) loadMembers(ctx context.Context, gd *guild.Guild) ([]string, error) {
	var avisos []string

	const q = `SELECT char_index, name, player_rank, classinfo, level, logged, last_login
	             FROM guilds_players
	            WHERE guild_index = :guilda
	            ORDER BY char_index
	            LIMIT 500`

	rows, err := g.DB.Query(ctx, q, sql.Named("guilda", gd.Index))
	if err != nil {
		return nil, fmt.Errorf("store: buscar membros da guilda %d: %w", gd.Index, err)
	}
	defer rows.Close()

	slot := 0
	for rows.Next() {
		var (
			charIndex           int64
			nome                sql.NullString
			rank, classe, nivel int64
			logged, lastLogin   int64
		)
		if err := rows.Scan(&charIndex, &nome, &rank, &classe, &nivel,
			&logged, &lastLogin); err != nil {
			return avisos, fmt.Errorf("store: ler membro de guilda: %w", err)
		}
		if charIndex <= 0 {
			continue
		}
		if slot >= guild.MaxMembers {
			// O vetor da origem tem 128 posicoes e era preenchido sem teto.
			avisos = append(avisos, fmt.Sprintf(
				"guilda %d tem mais de %d membros no banco: os excedentes foram ignorados",
				gd.Index, guild.MaxMembers))
			break
		}
		if rank < 0 || rank >= guild.MaxRanks {
			avisos = append(avisos, fmt.Sprintf(
				"guilda %d: membro %d com rank %d fora de 0..%d, posto no mais baixo",
				gd.Index, charIndex, rank, guild.MaxRanks-1))
			rank = guild.MaxRanks - 1
		}

		m := guild.Member{
			CharIndex: clampToDword(charIndex),
			Name:      nome.String,
			Rank:      uint8(rank),
			ClassInfo: clampToByte(classe),
			Level:     clampToByte(nivel),
			// Logged do banco nao vale: quem esta online e quem esta no
			// canal agora. A origem gravava a coluna e a lia de volta, o que
			// deixava membro marcado como online depois de o servidor cair.
			Logged: false,
		}
		if lastLogin > 0 {
			m.LastLogin = time.Unix(lastLogin, 0).UTC()
		}
		if _, err := gd.AddMember(m); err != nil {
			avisos = append(avisos, fmt.Sprintf(
				"guilda %d: membro %d recusado: %v", gd.Index, charIndex, err))
			continue
		}
		slot++
		_ = logged
	}
	if err := rows.Err(); err != nil {
		return avisos, fmt.Errorf("store: ler membros da guilda %d: %w", gd.Index, err)
	}
	return avisos, nil
}

// Save grava a guilda e os seus membros.
func (g Guilds) Save(ctx context.Context, gd *guild.Guild) error {
	if gd == nil || gd.Index == 0 {
		return fmt.Errorf("store: gravar guilda sem indice")
	}

	tx, err := g.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao de guilda: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // o Commit abaixo e o caminho bom

	const up = `INSERT INTO guilds
		(id, name, nation, exp, level, leader_char_index,
		 notice1, notice2, notice3, site, ranks_config,
		 bravure_points, skill_points, promote, storage_gold,
		 ally_leader, guild_ally1_index, guild_ally1_name, guild_ally2_index, guild_ally2_name,
		 guild_ally3_index, guild_ally3_name, guild_ally4_index, guild_ally4_name)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 name=VALUES(name), nation=VALUES(nation), exp=VALUES(exp),
		 level=VALUES(level), leader_char_index=VALUES(leader_char_index),
		 notice1=VALUES(notice1), notice2=VALUES(notice2),
		 notice3=VALUES(notice3), site=VALUES(site),
		 ranks_config=VALUES(ranks_config),
		 bravure_points=VALUES(bravure_points),
		 skill_points=VALUES(skill_points), promote=VALUES(promote),
		 storage_gold=VALUES(storage_gold),
		 ally_leader=VALUES(ally_leader),
		 guild_ally1_index=VALUES(guild_ally1_index), guild_ally1_name=VALUES(guild_ally1_name),
		 guild_ally2_index=VALUES(guild_ally2_index), guild_ally2_name=VALUES(guild_ally2_name),
		 guild_ally3_index=VALUES(guild_ally3_index), guild_ally3_name=VALUES(guild_ally3_name),
		 guild_ally4_index=VALUES(guild_ally4_index), guild_ally4_name=VALUES(guild_ally4_name)`

	promote := 0
	if gd.Promote {
		promote = 1
	}
	if _, err := tx.ExecContext(ctx, up,
		gd.Index, gd.Name, gd.Nation, gd.Exp, gd.Level, gd.LeaderID,
		gd.Notices[0], gd.Notices[1], gd.Notices[2], gd.Site,
		formatRanksConfig(gd.RanksConfig[:]),
		gd.BravurePoints, gd.SkillPoints, promote,
		gd.Chest.Gold(),
		gd.Ally.Leader,
		gd.Ally.Guilds[0].Index, gd.Ally.Guilds[0].Name, gd.Ally.Guilds[1].Index, gd.Ally.Guilds[1].Name,
		gd.Ally.Guilds[2].Index, gd.Ally.Guilds[2].Name, gd.Ally.Guilds[3].Index, gd.Ally.Guilds[3].Name); err != nil {
		return fmt.Errorf("store: gravar guilda %d: %w", gd.Index, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM guilds_players WHERE guild_index = ?`, gd.Index); err != nil {
		return fmt.Errorf("store: limpar membros da guilda %d: %w", gd.Index, err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO guilds_players
		 (guild_index, char_index, name, player_rank, classinfo, level, logged, last_login)
		 VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("store: preparar insercao de membros: %w", err)
	}
	defer stmt.Close()

	for i := range gd.Members {
		m := &gd.Members[i]
		if m.CharIndex == 0 {
			continue
		}
		logged := 0
		if m.Logged {
			logged = 1
		}
		var ultimo int64
		if !m.LastLogin.IsZero() {
			ultimo = m.LastLogin.Unix()
		}
		if _, err := stmt.ExecContext(ctx, gd.Index, m.CharIndex, m.Name,
			m.Rank, m.ClassInfo, m.Level, logged, ultimo); err != nil {
			return fmt.Errorf("store: gravar membro %d da guilda %d: %w",
				m.CharIndex, gd.Index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar guilda %d: %w", gd.Index, err)
	}
	return nil
}

// SaveChestGold grava so o ouro do bau de uma guilda.
//
// A origem nao tinha nada assim: o ouro do bau ia no UPDATE gigante de
// TFunctions.SaveGuilds, que reescrevia a guilda inteira e todos os membros,
// e que corria por um temporizador. Um deposito feito pouco antes de o
// servidor cair era perdido.
func (g Guilds) SaveChestGold(ctx context.Context, index uint16, gold uint64) error {
	if index == 0 {
		return fmt.Errorf("store: gravar ouro de bau sem guilda")
	}
	const q = `UPDATE guilds SET storage_gold = ? WHERE id = ?`
	if _, err := g.DB.SQL().ExecContext(ctx, q, gold, index); err != nil {
		return fmt.Errorf("store: gravar ouro do bau %d: %w", index, err)
	}
	return nil
}

// Delete apaga a guilda e os seus membros.
func (g Guilds) Delete(ctx context.Context, index uint16) error {
	if index == 0 {
		return nil
	}
	tx, err := g.DB.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: abrir transacao: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM guilds_players WHERE guild_index = ?`, index); err != nil {
		return fmt.Errorf("store: apagar membros da guilda %d: %w", index, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM guilds WHERE id = ?`, index); err != nil {
		return fmt.Errorf("store: apagar guilda %d: %w", index, err)
	}
	return tx.Commit()
}

// clampGuildLevel limita o nivel ao tamanho de MaxGuildPlayersOfLevel.
//
// A origem indexava "MaxGuildPlayersOfLevel[Self.Level]", ARRAY [0..9], com
// Level vindo do banco como DWORD.
func clampGuildLevel(v int64) uint8 {
	if v < 0 {
		return 0
	}
	if v > guild.MaxLevel {
		return guild.MaxLevel
	}
	return uint8(v)
}

// parseRanksConfig le a coluna ranks_config, que guarda os cinco bytes de
// permissao separados por virgula.
func parseRanksConfig(s string) []uint8 {
	out := make([]uint8, 0, guild.MaxRanks)
	campo := 0
	valor := 0
	tem := false
	flush := func() {
		if campo < guild.MaxRanks {
			if valor < 0 {
				valor = 0
			}
			if valor > 255 {
				valor = 255
			}
			out = append(out, uint8(valor))
		}
		campo++
		valor = 0
		tem = false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			valor = valor*10 + int(r-'0')
			tem = true
		case r == ',':
			flush()
		}
	}
	if tem || campo == 0 {
		flush()
	}
	return out
}

// formatRanksConfig grava os cinco bytes separados por virgula.
func formatRanksConfig(cfg []uint8) string {
	var b []byte
	for i, v := range cfg {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, []byte(itoaByte(v))...)
	}
	return string(b)
}

func itoaByte(v uint8) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
