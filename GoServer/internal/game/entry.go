package game

import (
	"context"
	"errors"
	"strings"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos do caminho de entrada no mundo.
const (
	CodeNumericToken = 0xF02 // cliente -> servidor
	CodeSendToWorld  = 0x925 // servidor -> cliente
	CodeP131         = 0x131 // servidor -> cliente (precede a entrada)
)

// WorldIndex e o Header.Index que SendToWorld usa, fixo na origem.
const WorldIndex = 0x7535

// MaxNumericErrors e o limite de erros da numerica por slot. Na origem o
// teste era "NumError[Slot] >= 5".
const MaxNumericErrors = 5

// Modos de RequestChange do pacote $F02.
const (
	NumericRegister = 0 // cadastrar quando ainda nao ha numerica
	NumericVerify   = 1 // conferir a numerica
	NumericChange   = 2 // trocar a numerica
)

// Erros do caminho de entrada.
var (
	ErrSlotOutOfRange   = errors.New("game: slot de personagem fora da faixa")
	ErrEmptySlot        = errors.New("game: slot de personagem vazio")
	ErrNumericLocked    = errors.New("game: numerica bloqueada por erros")
	ErrNumericWrong     = errors.New("game: numerica incorreta")
	ErrNumericExists    = errors.New("game: numerica ja cadastrada")
	ErrNotAuthenticated = errors.New("game: sessao nao autenticada")
)

// NumericStore grava a numerica e o contador de erros.
type NumericStore interface {
	// SaveNumeric grava a numerica do personagem.
	SaveNumeric(ctx context.Context, charID uint32, numeric string) error
	// SaveNumericErrors grava o contador de erros.
	SaveNumericErrors(ctx context.Context, charID uint32, errorsCount int) error
}

// numericToken e o porte de TPacketHandlers.NumericToken ($F02).
//
// A diferenca central em relacao a origem e a validacao do slot. Ali o
// primeiro acesso era "if (Player.Account.Header.NumError[Packet.Slot] >= 5)",
// com Packet.Slot vindo do cliente como DWORD e NumError sendo
// Array [0..2] of byte: um slot fora de 0..2 lia e depois ESCREVIA fora do
// vetor, no meio do registro da conta, antes de qualquer checagem.
func (h *Handler) numericToken(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if !s.Authenticated {
		c.Close()
		return ErrNotAuthenticated
	}

	var pkt packets.NumericTokenPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		c.Close()
		return err
	}

	slot := int(pkt.Slot)
	if slot < 0 || slot >= store.MaxCharacterSlots {
		c.Close()
		return ErrSlotOutOfRange
	}
	ch := s.Characters[slot]
	if ch.ID == 0 {
		c.Close()
		return ErrEmptySlot
	}
	if int(ch.NumericErrors) >= MaxNumericErrors {
		c.Close()
		return ErrNumericLocked
	}

	atual := strings.TrimSpace(ch.NumericToken)
	n1 := numericField(pkt.Numeric_1[:])
	n2 := numericField(pkt.Numeric_2[:])

	switch pkt.RequestChange {
	case NumericRegister:
		// Na origem o cadastro so acontecia com a numerica vazia, e quando
		// havia uma o pacote era ignorado em silencio: o cliente ficava
		// esperando. Aqui a recusa e explicita.
		if atual != "" {
			return h.rejectNumeric(ctx, c, s, slot, ErrNumericExists)
		}
		if n1 == "" {
			return h.rejectNumeric(ctx, c, s, slot, ErrNumericWrong)
		}
		if err := h.Numerics.SaveNumeric(ctx, ch.ID, n1); err != nil {
			c.Close()
			return err
		}
		s.Characters[slot].NumericToken = n1

	case NumericVerify:
		if atual == "" || !strings.EqualFold(n1, atual) {
			return h.rejectNumeric(ctx, c, s, slot, ErrNumericWrong)
		}
		// Acerto zera o contador de erros.
		if ch.NumericErrors != 0 {
			if err := h.Numerics.SaveNumericErrors(ctx, ch.ID, 0); err != nil {
				c.Close()
				return err
			}
			s.Characters[slot].NumericErrors = 0
		}

	case NumericChange:
		// Numeric_2 traz a numerica atual e Numeric_1 a nova.
		if atual == "" || !strings.EqualFold(n2, atual) {
			return h.rejectNumeric(ctx, c, s, slot, ErrNumericWrong)
		}
		if n1 == "" {
			return h.rejectNumeric(ctx, c, s, slot, ErrNumericWrong)
		}
		if err := h.Numerics.SaveNumeric(ctx, ch.ID, n1); err != nil {
			c.Close()
			return err
		}
		s.Characters[slot].NumericToken = n1

	default:
		// Modo desconhecido nao pode cair no caminho de entrada: na origem o
		// case sem ramo deixava o cliente sem resposta.
		h.log().Warn("modo de numerica desconhecido",
			"modo", pkt.RequestChange, "ip", c.IP(), "usuario", s.Username)
		return nil
	}

	return h.sendToWorld(ctx, c, s, slot)
}

// rejectNumeric conta o erro, devolve o pacote ao cliente e reenvia a lista,
// como o ramo de falha da origem.
func (h *Handler) rejectNumeric(ctx context.Context, c *netsrv.Conn, s *Session, slot int, cause error) error {
	ch := &s.Characters[slot]
	if ch.NumericErrors < 255 {
		ch.NumericErrors++
	}
	if err := h.Numerics.SaveNumericErrors(ctx, ch.ID, int(ch.NumericErrors)); err != nil {
		h.log().Error("numerica: gravar contador de erros",
			"personagem", ch.ID, "erro", err)
	}
	s.SubStatus = Waiting
	h.log().Info("numerica recusada",
		"usuario", s.Username, "slot", slot,
		"erros", ch.NumericErrors, "motivo", cause)

	// Reenvia a lista para o cliente voltar a tela de selecao.
	if err := h.sendCharList(c, s); err != nil {
		return err
	}
	return nil
}

// sendToWorld monta e envia o $925. Porte de TPlayer.SendToWorld.
//
// Nesta fase o pacote leva o que a tabela characters fornece: identidade,
// nivel, atributos, ouro e posicao. Inventario, equipamento, skills, buffs,
// quests e titulos ficam zerados porque vivem em tabelas proprias e entram
// nos incrementos seguintes; ver GoServer/PORTE.md.
func (h *Handler) sendToWorld(ctx context.Context, c *netsrv.Conn, s *Session, slot int) error {
	if slot < 0 || slot >= store.MaxCharacterSlots {
		return ErrSlotOutOfRange
	}
	src := s.Characters[slot]
	if src.ID == 0 {
		return ErrEmptySlot
	}

	// O cliente espera o $131 antes da entrada.
	var p131 packets.Tp131
	p131.Unk_1 = 0xFFFFFFFF
	if err := c.SendStruct(CodeP131, p131); err != nil {
		return err
	}

	// Os itens entram antes de montar o pacote: o cliente desenha o boneco a
	// partir do que vem no $925, e sem equipamento ele aparece nu.
	//
	// Na origem o carregamento de equipamento usava o indice do laco como
	// slot do personagem ("Slot := i" dentro de "for i := 0 to CharCount-1"),
	// e nao o slot que veio do banco: uma conta com personagens nos slots 0 e
	// 2 lia os itens de Characters[1], que esta vazio, e o personagem do slot
	// 2 entrava sem equipamento.
	equip, inventario := h.loadItems(ctx, s, src.ID)

	var pkt packets.SendToWorldPacket
	pkt.Header.Index = WorldIndex
	pkt.AcountSerial = s.Account.ID

	ch := &pkt.Character
	ch.ClientId = uint32(s.ClientID)
	ch.CharIndex = src.ID
	netsrv.PutCString(ch.Name[:], src.Name)
	ch.Nation = s.Account.Nation
	ch.ClassInfo = uint8(src.ClassInfo)
	ch.Level = src.Level
	ch.Exp = src.Exp
	ch.Gold = src.Gold
	ch.GuildIndex = src.GuildIndex
	ch.CurrentScore = model.Status{
		Str:     src.Strength,
		Agility: src.Agility,
		Int:     src.Intelligence,
		Cons:    src.Constitution,
		Luck:    src.Luck,
		Sizes: model.Sizes{
			Altura: src.Altura, Tronco: src.Tronco,
			Perna: src.Perna, Corpo: src.Corpo,
		},
		CurHP: src.CurHP,
		CurMp: src.CurMP,
		Honor: src.Honor,
	}
	netsrv.PutCString(ch.Numeric[:], src.NumericToken)

	for i := range ch.Equip {
		ch.Equip[i] = equip.Get(i)
	}
	for i := range ch.Inventory {
		ch.Inventory[i] = inventario.Get(i)
	}

	s.SelectedSlot = slot
	s.Equip = equip
	s.Inventory = inventario
	s.Avatar = AvatarFromCharacter(src, uint16(s.Account.Nation))
	s.Avatar.SetEquip(equip)
	s.Avatar.ApplyEquipBonus(h.Data, equip)
	s.SpeedMove = s.Avatar.SpeedMove
	s.Position = entryPosition(src)
	s.AnchorPosition = s.Position
	s.Dead = false
	s.LastMoved = h.now()

	// O pacote leva o maximo calculado, nao o que estava no banco: o banco
	// guarda so o HP atual.
	ch.CurrentScore.MaxHp = s.Avatar.MaxHP
	ch.CurrentScore.MaxMP = s.Avatar.MaxMP
	ch.CurrentScore.CurHP = s.Avatar.CurHP
	ch.CurrentScore.CurMp = s.Avatar.CurMP
	ch.CurrentScore.Status = src.Status
	ch.CurrentScore.KillPoint = src.KillPoint
	ch.CurrentScore.Infamia = src.Infamia
	ch.CurrentScore.SkillPoint = src.SkillPoint
	ch.ActiveTitle = src.ActiveTitle

	s.Status = Playing
	s.SubStatus = Playing

	h.log().Info("personagem entrou no mundo",
		"usuario", s.Username, "personagem", src.Name, "id", s.ClientID,
		"slot", slot, "nivel", src.Level, "hp", s.Avatar.MaxHP,
		"mp", s.Avatar.MaxMP, "canal", h.Channel)

	if h.Chan != nil {
		h.Chan.RegisterCharacter(s.ClientID, src.Name)
	}

	if err := c.SendStruct(CodeSendToWorld, pkt); err != nil {
		return err
	}

	// Com o jogador em jogo, ele e os vizinhos passam a se ver. Isto so pode
	// acontecer depois do $925: antes dele o cliente nao tem mundo desenhado
	// e descartaria os spawns.
	h.refreshVisible(s)
	return nil
}

// entryPosition decide onde o personagem entra.
//
// O original usava posx/posy do banco direto, sem olhar o valor: personagem
// criado por fora, ou gravado com a coluna nula, entrava em 0,0 — canto do
// mapa, fora de qualquer cidade, onde nao da para fazer nada. Aqui a posicao
// gravada so vale se for utilizavel, e o resto cai no ponto de partida.
func entryPosition(src store.Character) model.Position {
	p := model.Position{X: src.PosX, Y: src.PosY}
	if p.X == 0 && p.Y == 0 {
		return StartPosition
	}
	if !ValidPosition(p) {
		return StartPosition
	}
	return p
}

// DefaultSpeedMove e a velocidade de caminhada padrao, usada quando a coluna
// speedmove esta zerada.
const DefaultSpeedMove = 20

// StartPosition e onde o personagem entra sem posicao gravada valida.
var StartPosition = model.Position{X: 2100, Y: 2100}

// numericField le a numerica de um campo de tamanho fixo.
func numericField(b []byte) string {
	return strings.TrimSpace(netsrv.CString(b))
}

// loadItems le equipamento e inventario do personagem.
//
// Falha de leitura nao impede a entrada: o jogador entra sem o que nao pudo
// ser lido e o erro vai para o log. A origem fazia o mesmo por acidente — o
// try/except em volta de cada consulta engolia a excecao e seguia — mas sem
// registrar qual personagem ficou sem item.
func (h *Handler) loadItems(ctx context.Context, s *Session, charID uint32) (equip, inv store.ItemSet) {
	equip = store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}
	inv = store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}
	if h.Items == nil {
		return equip, inv
	}
	if e, err := h.Items.Equip(ctx, charID); err != nil {
		h.log().Error("carregar equipamento",
			"personagem", charID, "usuario", s.Username, "erro", err)
	} else {
		equip = e
	}
	if i, err := h.Items.Inventory(ctx, charID); err != nil {
		h.log().Error("carregar inventario",
			"personagem", charID, "usuario", s.Username, "erro", err)
	} else {
		inv = i
	}
	for _, a := range append(equip.Avisos, inv.Avisos...) {
		h.log().Warn("itens", "personagem", charID, "aviso", a)
	}
	return equip, inv
}
