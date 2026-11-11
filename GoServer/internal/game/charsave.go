package game

import (
	"context"

	"aika/internal/store"
)

// CharacterSaver grava o progresso do personagem.
type CharacterSaver interface {
	SaveProgress(ctx context.Context, accountID, charID uint32, p store.Progress) error
}

// progressOf monta o que vai ao banco a partir do personagem em jogo.
func progressOf(s *Session) store.Progress {
	a := &s.Avatar
	return store.Progress{
		Level: a.Level, Exp: a.Exp, Gold: a.Gold,
		Strength: a.Str, Agility: a.Agility, Intelligence: a.Int,
		Constitution: a.Cons, Luck: a.Luck, Status: a.StatusPoints,
		CurHP: a.CurHP, CurMP: a.CurMP, Honor: a.Honor, KillPoint: a.KillPoint,
		Infamia: a.Infamia, SkillPoint: a.SkillPoint,
		PosX: s.Position.X, PosY: s.Position.Y,
		SpeedMove: a.SpeedMove, Rotation: a.Rotation,
		PlayerKill:  a.PlayerKill,
		ActiveTitle: uint16(s.ActiveTitle.Index),
	}
}

// saveCharacter grava nivel, experiencia, ouro, atributos, vida, posicao e o
// resto do progresso. E chamado no ponto unico de saida (OnDisconnect).
//
// Ate aqui o porte so gravava itens, skills, ouro do armazem e cash: nivel,
// experiencia, ouro do personagem, pontos distribuidos e posicao se perdiam a
// cada saida. A origem gravava tudo no UPDATE de TPlayer.SaveInGame.
func (h *Handler) saveCharacter(s *Session) {
	if h.CharSave == nil || s.SelectedSlot < 0 || s.SelectedSlot >= store.MaxCharacterSlots {
		return
	}
	charID := s.Characters[s.SelectedSlot].ID
	if charID == 0 || s.Account.ID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := h.CharSave.SaveProgress(ctx, s.Account.ID, charID, progressOf(s)); err != nil {
		h.log().Error("gravar progresso",
			"usuario", s.Username, "personagem", charID, "erro", err)
	}
}

// pushUnreadMail porta o TEntityMail.SendUnreadMails que as threads da origem
// chamavam junto de cada gravacao periodica: reenvia o $3F1B com o numero de
// cartas nao lidas, para a janela avisar quem recebeu carta com o jogo aberto.
func (h *Handler) pushUnreadMail(s *Session) {
	if h.MailDB == nil || s.CharID() == 0 {
		return
	}
	c := h.connOf(s)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	caixa, err := h.MailDB.Box(ctx, s.CharID())
	if err != nil || caixa == nil {
		return
	}
	h.sendMailUnread(c, s, caixa.Unread())
}

// SaveAllTick grava o estado de todo jogador em jogo e devolve quantos foram
// gravados. E o complemento do ponto unico de saida (OnDisconnect): sem ele, uma
// queda do processo perdia tudo que a sessao tinha mudado desde o login.
//
// A origem gravava em varios pontos soltos (TPlayer.SaveInGame) e tambem so na
// saida; aqui o intervalo e do chamador (main). Cada jogador e gravado com
// tradeMu seguro, para nunca fotografar uma troca ou compra de loja pela
// metade (item ja saiu de um lado e ainda nao entrou no outro).
//
// Limite conhecido: os handlers escrevem na sessao sem trava, entao esta
// leitura, feita por outra goroutine, e uma corrida benigna em campos simples
// que so o -race (indisponivel nesta maquina) mediria.
func (h *Handler) SaveAllTick() int {
	if h.Chan == nil {
		return 0
	}
	n := 0
	for _, id := range h.Chan.PlayingIDs() {
		s, _, ok := h.Chan.Lookup(id)
		if !ok || s.Status < Playing {
			continue
		}
		tradeMu.Lock()
		h.saveItems(s)
		h.saveCharacter(s)
		h.saveItemBar(s)
		h.savePrans(s)
		h.saveTitles(s)
		h.saveQuests(s)
		tradeMu.Unlock()
		h.pushUnreadMail(s)
		n++
	}
	return n
}
