package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Progress e o que o personagem muda jogando e precisa voltar ao banco: as
// colunas que ByAccount le e que nenhum outro caminho de gravacao cobre.
//
// Ficam de fora de proposito: numeric_token e numeric_errors (SaveNumeric),
// guildindex (a guilda grava o proprio vinculo), itens, skills e o ouro do
// armazem (cada um tem a sua gravacao) e as colunas de cosmetica criadas na
// criacao do personagem.
type Progress struct {
	Level        uint16
	Exp          int64
	Gold         int64
	Strength     uint16
	Agility      uint16
	Intelligence uint16
	Constitution uint16
	Luck         uint16
	Status       uint16
	CurHP        uint32
	CurMP        uint32
	Honor        uint32
	KillPoint    uint32
	Infamia      uint32
	SkillPoint   uint16
	PosX, PosY   float32
	SpeedMove    uint16
	Rotation     uint32
	PlayerKill   bool
	// ActiveTitle e o indice do titulo ativo (0 = nenhum); so o handler de
	// titulo o muda.
	ActiveTitle uint16
}

// SaveProgress grava o progresso do personagem num UPDATE so.
//
// A origem gravava isto no UPDATE gigante de TPlayer.SaveInGame, montado por
// concatenacao de texto e chamado de varios pontos. Aqui os valores vao como
// parametros e a conta dona entra no WHERE: um id errado nao escreve no
// personagem de outra conta.
func (c Characters) SaveProgress(ctx context.Context, accountID, charID uint32, p Progress) error {
	if charID == 0 || accountID == 0 {
		return fmt.Errorf("store: gravar progresso: ids invalidos (conta %d, personagem %d)", accountID, charID)
	}
	const q = `UPDATE characters SET
	             level = :level, experience = :exp, gold = :gold,
	             strength = :str, agility = :agi, intelligence = :int,
	             constitution = :con, luck = :luck, status = :status,
	             curhp = :hp, curmp = :mp, honor = :honor, killpoint = :kp,
	             infamia = :inf, skillpoint = :sp,
	             posx = :px, posy = :py, speedmove = :spd, rotation = :rot,
	             playerkill = :pk, active_title = :at
	           WHERE id = :id AND owner_accid = :acc`
	pk := 0
	if p.PlayerKill {
		pk = 1
	}
	_, err := c.DB.Exec(ctx, q,
		sql.Named("level", p.Level), sql.Named("exp", p.Exp), sql.Named("gold", p.Gold),
		sql.Named("str", p.Strength), sql.Named("agi", p.Agility), sql.Named("int", p.Intelligence),
		sql.Named("con", p.Constitution), sql.Named("luck", p.Luck), sql.Named("status", p.Status),
		sql.Named("hp", p.CurHP), sql.Named("mp", p.CurMP), sql.Named("honor", p.Honor),
		sql.Named("kp", p.KillPoint), sql.Named("inf", p.Infamia), sql.Named("sp", p.SkillPoint),
		sql.Named("px", p.PosX), sql.Named("py", p.PosY), sql.Named("spd", p.SpeedMove),
		sql.Named("rot", p.Rotation), sql.Named("pk", pk), sql.Named("at", p.ActiveTitle),
		sql.Named("id", charID), sql.Named("acc", accountID))
	if err != nil {
		return fmt.Errorf("store: gravar progresso de %d: %w", charID, err)
	}
	return nil
}
