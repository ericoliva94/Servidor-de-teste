package game

import (
	"context"
	"errors"
	"strings"

	"aika/internal/entity"
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
	armazem := h.loadStorage(ctx, s)
	premium := h.loadCash(ctx, s)
	skills := h.loadSkills(ctx, s, src.ID)

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
	s.ItemBar = h.loadItemBar(ctx, src.ID)
	for i := range ch.ItemBar {
		ch.ItemBar[i] = s.ItemBar[i]
	}

	s.SelectedSlot = slot
	s.Equip = equip
	s.Inventory = inventario
	s.Storage = armazem
	// O armazem e da conta, entao o ouro dele vem do registro da conta e nao
	// do personagem. A origem lia o mesmo campo (accounts.storage_gold).
	s.StorageGold = s.Account.StorageGold
	s.Cash = premium
	s.CashBalance = s.Account.Cash
	s.Skills = skills
	s.Avatar = AvatarFromCharacter(src, uint16(s.Account.Nation))
	h.loadTitles(ctx, s, src.ID, src.ActiveTitle)
	h.refreshPassives(s)
	// Buffs e efeitos sao do personagem: trocar de personagem na mesma conexao
	// nao pode carregar os do anterior.
	s.Base.Buffs = entity.NewBuffs()
	s.Avatar.SetEquip(equip)
	s.Avatar.Relic = relicEffectsFor(h.Devir, s.Avatar.Nation)
	s.Avatar.ApplyEquipBonus(h.Data, equip)
	h.loadTeleports(ctx, s, src.ID)
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
	// Os derivados vao no pacote: sem eles o cliente mostra tudo zerado.
	ch.CurrentScore.DNFis = s.Avatar.AtkFis
	ch.CurrentScore.DefFis = s.Avatar.DefFis
	ch.CurrentScore.DNMag = s.Avatar.AtkMag
	ch.CurrentScore.DefMag = s.Avatar.DefMag
	ch.CurrentScore.KillPoint = src.KillPoint
	ch.CurrentScore.Infamia = src.Infamia
	ch.CurrentScore.SkillPoint = src.SkillPoint
	ch.ActiveTitle = src.ActiveTitle
	h.fillTitleProgress(s, ch)
	// O $925 leva a mascara de niveis em SkillList; a arvore em si fica na
	// sessao e vai pelos pacotes $106 e $107.
	h.fillSkillMask(s, ch.SkillList[:])

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
	h.sendSkillList(c, s)
	h.enterGuild(c, s)
	h.enterMail(ctx, c, s)
	h.loadFriends(ctx, c, s)
	h.enterTitle(c, s)
	h.loadPrans(ctx, s)
	h.sendNationInformation(c, s)
	h.sendReliques(c, s)
	h.enterPran(c, s)
	// O $925 nao leva posicao: e o pacote de criacao do proprio jogador que
	// diz ao cliente onde ele esta (SendToWorldSends, Player.pas:
	// "Base.SendCreateMob(SPAWN_NORMAL, Self.Base.clientId)"). Sem ele o
	// cliente ignorava posx/posy do banco e nao ia para onde foi gravado.
	h.sendSpawn(s, s.ClientID)
	h.refreshVisible(s)
	h.refreshVisibleMobs(s)
	h.sendMapObjectsNear(s)
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
	garantirBolsaInicial(&inv)
	return equip, inv
}

// BolsaInicial e o item "Bag" que fica no slot 60 do inventario.
const (
	BolsaInicial     = 5300
	slotBolsaInicial = 60
)

// garantirBolsaInicial porta o final de LoadCharacters (Player.pas:1253): sem
// nada no slot 60 o Delphi grava a Bag 5300 ali. Sem ela os 15 primeiros
// slots ficam sem bolsa e o moveItem recusa qualquer movimento (ErrBolsaAusente).
func garantirBolsaInicial(inv *store.ItemSet) {
	if len(inv.Slots) <= slotBolsaInicial {
		return
	}
	// Personagens criados antes da correcao tem a bolsa so com o Index (invisivel
	// no cliente): completa APP e quantidade.
	if b := &inv.Slots[slotBolsaInicial]; b.Index == BolsaInicial {
		if b.APP == 0 {
			b.APP = BolsaInicial
		}
		if b.Refi == 0 {
			b.Refi = 1
		}
		return
	}
	if !inv.Slots[slotBolsaInicial].IsEmpty() {
		return
	}
	inv.Slots[slotBolsaInicial] = model.Item{Index: BolsaInicial, APP: BolsaInicial, Refi: 1}
}

// loadStorage le o armazem da conta.
//
// O dono e a CONTA: na origem o armazem ficava em Account.Header.Storage e
// era compartilhado por todos os personagens dela, com owner_id = id da conta
// e slot_type 2 na tabela items.
func (h *Handler) loadStorage(ctx context.Context, s *Session) store.ItemSet {
	vazio := store.ItemSet{Slots: make([]model.Item, store.StorageSlots)}
	if h.Items == nil || s.Account.ID == 0 {
		return vazio
	}
	set, err := h.Items.Storage(ctx, s.Account.ID)
	if err != nil {
		h.log().Error("carregar armazem",
			"conta", s.Account.ID, "usuario", s.Username, "erro", err)
		return vazio
	}
	for _, a := range set.Avisos {
		h.log().Warn("armazem", "conta", s.Account.ID, "aviso", a)
	}
	if len(set.Slots) < store.StorageSlots {
		// Um conjunto curto faria o resolveSlot recusar slots validos; melhor
		// completar do que depender do tamanho que veio.
		completo := make([]model.Item, store.StorageSlots)
		copy(completo, set.Slots)
		set.Slots = completo
	}
	return set
}

// loadSkills le a arvore de skills do personagem.
//
// Falha de leitura nao impede a entrada: o jogador entra sem skill e o erro
// vai para o log, como a origem fazia por acidente (o try/except em volta da
// consulta engolia a excecao) mas sem registrar qual personagem ficou sem.
func (h *Handler) loadSkills(ctx context.Context, s *Session, charID uint32) model.SkillsList {
	var out model.SkillsList
	if h.SkillDB == nil {
		return out
	}
	list, avisos, err := h.SkillDB.ByCharacter(ctx, charID)
	if err != nil {
		h.log().Error("carregar skills",
			"personagem", charID, "usuario", s.Username, "erro", err)
		return out
	}
	for _, a := range avisos {
		h.log().Warn("skills", "personagem", charID, "aviso", a)
	}
	return list
}

// enterGuild poe o jogador de volta na guilda ao entrar no mundo.
//
// Na origem isso dependia de SendMemberLogin, chamado do caminho de login; a
// guilda vem do indice gravado no personagem, e o slot no vetor global era
// recalculado por SearchAndSetGuildSlot em varios pontos. Aqui a guilda e
// achada pelo id do personagem.
func (h *Handler) enterGuild(c *netsrv.Conn, s *Session) {
	if h.Guilds == nil {
		return
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil {
		// Personagem com guildindex no banco e sem linha em guilds_players
		// nao esta em guilda nenhuma: a origem mandava o pacote de guilda com
		// o indice gravado e o cliente mostrava uma guilda que nao existe.
		if s.Avatar.GuildIndex != 0 {
			h.log().Warn("personagem com guilda que nao existe",
				"personagem", s.Avatar.Name, "indice", s.Avatar.GuildIndex)
			s.Avatar.GuildIndex = 0
		}
		return
	}
	s.Avatar.GuildIndex = g.Index
	s.Avatar.GuildName = g.Name
	if m := g.MemberAt(g.FindMember(s.CharID())); m != nil {
		m.Logged = true
		m.LastLogin = h.now()
		// O nivel e a classe do membro acompanham o personagem.
		m.Level = uint8(min(s.Avatar.Level, 255))
		m.ClassInfo = uint8(min(s.Avatar.ClassInfo, 255))
	}
	h.sendGuildInfo(c, s, g)
	h.sendGuildPlayers(c, s, g)
	h.broadcastGuild(g, s)
}
