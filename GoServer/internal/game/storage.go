package game

import (
	"context"
	"errors"
	"fmt"
	"math"

	"aika/internal/guild"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos do armazem e do bau de guilda.
const (
	// CodeOpenNPC e o $30F: cliente -> servidor, a opcao escolhida na janela
	// do NPC. E por aqui que o armazem e o bau abrem.
	CodeOpenNPC = 0x30F
	// CodeCloseNPCOption e o $348: cliente -> servidor, fechou a janela.
	CodeCloseNPCOption = 0x348
	// CodeStorage e o $137: servidor -> cliente, o armazem inteiro.
	CodeStorage = 0x137
	// CodeStorageOpened e o $310: servidor -> cliente, qual armazem abriu.
	CodeStorageOpened = 0x310
	// CodeChangeChestGold e o $F59: cliente -> servidor, mover ouro entre a
	// bolsa e o armazem ou o bau.
	CodeChangeChestGold = 0xF59
	// CodeGuildChest e o $D58: servidor -> cliente, o bau inteiro.
	CodeGuildChest = 0xD58
	// CodeGuildChestPermission e o $F10: servidor -> cliente, se o membro
	// pode mexer no bau.
	CodeGuildChestPermission = 0xF10
	// CodeRefreshGuildChestGold e o $D18: servidor -> cliente, o ouro do bau.
	CodeRefreshGuildChestGold = 0xD18
	// CodeCloseGuildChest e o $F2F: nos dois sentidos. Do servidor e o aviso
	// de que o prazo venceu; do cliente, o fechamento.
	CodeCloseGuildChest = 0xF2F
	// CodeNPCOptionDenied e o $10F que a origem mandava no ramo "else" do
	// case de opcoes: a opcao nao existe.
	CodeNPCOptionDenied = 0x10F
)

// Opcoes da janela de NPC ($30F, campo Type1) que este porte atende.
//
// A origem tinha mais de trinta ramos neste case. Os que dependem de
// sistemas ainda nao portados (loja, skill de NPC, bigorna, encantamento,
// castelo, nacao, reliquia) caem no ramo de recusa, que e o mesmo que a
// origem usava para opcao inexistente.
const (
	NPCOptionShowMenu    = 0x0
	NPCOptionTalk        = 0x1
	NPCOptionStorage     = 0x7
	NPCOptionCloseMenu   = 0x8
	NPCOptionGuildChest  = 0xB
	NPCOptionPranStorage = 0xD
	NPCOptionDungeon     = 0x1A // 26
)

// Tipos de armazem do $310, as constantes STORAGE_TYPE_* de GlobalDefs.pas.
const (
	StorageTypePlayer = 1
	StorageTypePrans  = 2
	StorageTypeGuild  = 3
)

// Faixas de slot do armazem, iguais as da origem.
const (
	// StorageItemSlots e o "if (Packet.srcSlot < 80)": os slots 0 a 79
	// guardam item.
	StorageItemSlots = 80
	// StorageFirstPranSlot e StorageLastPranSlot sao os dois slots da central
	// de pran (84 e 85), que a origem so deixava receber item de ItemType 10.
	StorageFirstPranSlot = 84
	StorageLastPranSlot  = 85
)

// MaxStorageGold e o teto do ouro do armazem.
//
// E o 2.000.000.000 que a origem conferia em TPacketHandlers.ChangeGold.
const MaxStorageGold int64 = 2_000_000_000

// MinNPCIndexToClose e o "if not(Player.OpennedNPC >= 2048)" de
// TPacketHandlers.CloseNPCOption: abaixo disso o indice nao e de NPC.
const MinNPCIndexToClose = 2048

// Erros do armazem.
var (
	ErrStorageClosed   = errors.New("game: armazem nao esta aberto")
	ErrStorageBagEmpty = errors.New("game: expansao do armazem ausente")
	ErrPranNotPorted   = errors.New("game: central de pran nao portada")
)

// storageBagSlot devolve o slot da expansao que habilita um slot do armazem.
//
// Porte do "case Packet.srcSlot of 0..19: SrcBag := 80; ..." da origem: cada
// grupo de 20 slots depende de um item nos slots 80 a 83.
func storageBagSlot(slot int) int {
	switch {
	case slot < 20:
		return 80
	case slot < 40:
		return 81
	case slot < 60:
		return 82
	case slot < 80:
		return 83
	}
	return -1
}

// storageOpen diz se a janela do armazem esta aberta.
//
// A origem exigia "OpennedOption in [7, 13] and OpennedNPC > 0" so no DESTINO
// do arrasto e no DEPOSITO de ouro; no lado de ORIGEM a mesma guarda estava
// comentada (PacketHandlers.pas:3514 e 1210). O resultado e que tirar item e
// ouro do armazem funcionava de qualquer ponto do mapa, sem NPC nenhum. Aqui
// a guarda vale para os dois lados.
func storageOpen(s *Session) bool {
	if s == nil || s.OpenedNPC == 0 {
		return false
	}
	return s.OpenedOption == NPCOptionStorage ||
		s.OpenedOption == NPCOptionPranStorage
}

// openNPC e o porte de TPacketHandlers.OpenNPC ($30F), na parte que leva ao
// armazem, ao bau da guilda e a dungeon.
//
// Diferencas centrais em relacao a origem:
//
//   - Packet.Index e DWORD e ia direto para "Servers[].NPCs[Packet.Index]",
//     um vetor de tamanho fixo, em varios ramos do case. Aqui o indice e
//     cortado para WORD e so e guardado, nunca usado como indice de vetor.
//
//   - A origem punha "OpennedNPC := Packet.Index; OpennedOption := Type1" no
//     FIM da funcao, depois do case, e muitos ramos saiam antes com "Exit".
//     Isso deixava pares incoerentes: a opcao $7 (armazem) mandava o armazem
//     mas NAO marcava a janela como aberta, de modo que a guarda
//     "OpennedOption = 7" do deposito nunca passava — e e exatamente por isso
//     que ela esta comentada no codigo da origem. Aqui cada ramo marca o que
//     abriu.
func (h *Handler) openNPC(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.OpenNPCPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}

	// "if (Packet.Index = 0) and not(Packet.Type1 = 8)": sem NPC e sem ser o
	// fechamento, nao ha o que abrir.
	if pkt.Index == 0 && pkt.Type1 != NPCOptionCloseMenu {
		h.closeNPCWindow(s)
		return nil
	}
	if pkt.Index > math.MaxUint16 {
		h.log().Debug("indice de NPC acima do WORD",
			"usuario", s.Username, "indice", pkt.Index)
		h.closeNPCWindow(s)
		return nil
	}
	npc := uint16(pkt.Index)

	switch pkt.Type1 {
	case NPCOptionCloseMenu, NPCOptionShowMenu:
		h.closeNPCWindow(s)
		return nil

	case NPCOptionStorage:
		s.OpenedNPC = npc
		s.OpenedOption = NPCOptionStorage
		h.sendStorage(c, s, StorageTypePlayer)
		return nil

	case NPCOptionPranStorage:
		// A origem mandava o mesmo armazem com STORAGE_TYPE_PRANS. Os dois
		// slots de pran (84 e 85) fazem parte do mesmo vetor, entao o pacote
		// e igual; o que nao existe aqui e o sistema de pran para fazer algo
		// com eles.
		s.OpenedNPC = npc
		s.OpenedOption = NPCOptionPranStorage
		h.sendStorage(c, s, StorageTypePrans)
		return nil

	case NPCOptionGuildChest:
		return h.openGuildChest(ctx, c, s, npc)

	case NPCOptionDungeon:
		s.OpenedNPC = npc
		s.OpenedOption = NPCOptionDungeon
		h.sendDungeonDialog(c, s)
		return nil

	default:
		// O ramo "else" da origem: sinal de recusa e janela fechada.
		h.closeNPCWindow(s)
		if err := h.sendHeaderSignal(c, s.ClientID, CodeNPCOptionDenied); err != nil {
			h.log().Debug("enviar recusa de opcao de NPC",
				"usuario", s.Username, "erro", err)
		}
		h.log().Debug("opcao de NPC ainda nao portada",
			"usuario", s.Username, "npc", npc,
			"opcao", fmt.Sprintf("%#x", pkt.Type1))
		return nil
	}
}

// closeNPCOption e o porte de TPacketHandlers.CloseNPCOption ($348).
func (h *Handler) closeNPCOption(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SignalData
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Data == 0 {
		return nil
	}
	if s.OpenedNPC < MinNPCIndexToClose {
		return nil
	}
	h.closeNPCWindow(s)
	return nil
}

// closeNPCWindow zera a janela aberta e solta o bau, se era ele.
//
// A origem zerava OpennedNPC e OpennedOption em mais de quarenta pontos, cada
// um esquecendo alguma coisa: nenhum deles soltava o bau da guilda.
func (h *Handler) closeNPCWindow(s *Session) {
	if s == nil {
		return
	}
	if s.OpenedOption == NPCOptionGuildChest && h.Guilds != nil {
		h.Guilds.CloseChestOf(s.CharID())
	}
	s.OpenedNPC = 0
	s.OpenedOption = 0
}

// sendDungeonDialog e o porte de TNPCHandlers.ShowDungeonDialog ($119).
//
// A origem repetia o bloco cinco vezes, um por NPC, e lia sempre
// "DG<nome>[0].LevelMin" — o nivel minimo da PRIMEIRA dificuldade, mesmo
// quando o jogador ia escolher outra. Aqui o nivel vem da tabela, pela
// dificuldade mais facil, que e o que o dialogo mostra.
func (h *Handler) sendDungeonDialog(c *netsrv.Conn, s *Session) {
	if c == nil || h.DungeonTable == nil {
		return
	}
	def, ok := h.DungeonTable.ByEntranceNPC(s.OpenedNPC, 0)
	if !ok {
		h.log().Debug("NPC sem dungeon na tabela",
			"usuario", s.Username, "npc", s.OpenedNPC)
		return
	}
	var pkt packets.SendDungeonDialog
	pkt.Header.Index = WorldIndex
	pkt.Dungeon = def.Index
	pkt.LevelMin = def.LevelMin
	if err := c.SendStruct(CodeDungeonDialog, pkt); err != nil {
		h.log().Debug("enviar dialogo de dungeon",
			"usuario", s.Username, "erro", err)
	}
}

// sendStorage e o porte de TPlayer.SendStorage ($137 + $310).
func (h *Handler) sendStorage(c *netsrv.Conn, s *Session, tipo uint32) {
	if c == nil || s == nil {
		return
	}
	var pkt packets.StoragePacket
	pkt.Header.Index = s.ClientID
	pkt.Storage.Gold = s.StorageGold
	// A origem copiava o registro inteiro com Move; aqui o copy para no menor
	// dos dois, entao um conjunto curto (leitura parcial do banco) manda o
	// resto zerado em vez de ler fora.
	copy(pkt.Storage.Itens[:], s.Storage.Slots)
	if err := c.SendStruct(CodeStorage, pkt); err != nil {
		h.log().Debug("enviar armazem", "usuario", s.Username, "erro", err)
		return
	}
	if err := h.sendSignalData(c, s.ClientID, CodeStorageOpened, tipo); err != nil {
		h.log().Debug("enviar tipo de armazem", "usuario", s.Username, "erro", err)
	}
	// A origem reenviava os dois slots de pran depois do pacote grande.
	for slot := StorageFirstPranSlot; slot <= StorageLastPranSlot; slot++ {
		h.sendRefreshItem(c, s, store.SlotTypeStorage, uint16(slot),
			s.Storage.Get(slot))
	}
}

// openGuildChest e o porte do ramo $B de OpenNPC com TGuild.OpenChest.
//
// Diferencas centrais em relacao a origem:
//
//   - OpenChest recebia "Player.Character.Index", um campo de TPlayerCharacter
//     que NUNCA e atribuido em nenhum ponto do servidor: valia sempre zero.
//     Com isso "GetPlayerByCharIndex(0, Player)" falhava, OpenChest devolvia
//     False e o ramo $B saia — o bau da guilda nao abria NUNCA na origem. O
//     mesmo campo zerado aparece em SendMemberLogout e no pacote $925.
//
//   - "Guild := @Guilds[Player.Character.GuildSlot]" com Guilds declarado
//     "ARRAY [1..50]" e GuildSlot vindo de GetGuildSlotByID, que devolve ZERO
//     quando nao acha a guilda (ServerSocket.pas:4232). Guilds[0] e um
//     registro inteiro de guilda (milhares de bytes) ANTES do inicio do
//     vetor, em cima das tabelas de mapa e de mob carregadas em GlobalDefs.
//     Escrever o ouro ou um item do bau ali corrompia dado de jogo
//     silenciosamente. Aqui a guilda vem de um mapa por personagem.
//
//   - A permissao era lida com "Members[FindMemberFromCharIndex(...)].Rank"
//     sem conferir o retorno: para quem nao era membro, isso e Members[-1].
func (h *Handler) openGuildChest(ctx context.Context, c *netsrv.Conn, s *Session, npc uint16) error {
	if h.Guilds == nil {
		return nil
	}
	if s.Avatar.GuildIndex == 0 {
		h.closeNPCWindow(s)
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil || g.Chest == nil {
		// Personagem com GuildIndex de uma guilda que nao existe mais.
		h.closeNPCWindow(s)
		h.message(c, "Sua guilda nao foi encontrada.")
		return nil
	}
	if g.FindMember(s.CharID()) < 0 {
		h.closeNPCWindow(s)
		h.message(c, "Voce nao e membro desta guilda.")
		return nil
	}
	if !g.CanOf(s.CharID()).OpenChest {
		h.closeNPCWindow(s)
		h.message(c, "Voce nao tem permissao para isso.")
		return nil
	}
	if err := g.Chest.Open(s.CharID(), s.Avatar.Name, h.now()); err != nil {
		h.closeNPCWindow(s)
		if errors.Is(err, guild.ErrChestBusy) {
			h.message(c, fmt.Sprintf("'%s' esta usando o armazem.",
				g.Chest.HolderName()))
			return nil
		}
		h.message(c, "Nao foi possivel abrir o bau.")
		return nil
	}

	s.OpenedNPC = npc
	s.OpenedOption = NPCOptionGuildChest

	if err := h.loadGuildChest(ctx, g); err != nil {
		h.log().Error("carregar bau de guilda",
			"guilda", g.Index, "erro", err)
	}
	h.sendGuildChestPermission(c, s, g)
	h.sendGuildChest(c, s, g)
	h.refreshGuildChestGold(c, s, g)
	return nil
}

// loadGuildChest le os itens do bau do banco na primeira abertura.
//
// A origem carregava o bau de todas as guildas no boot (Load.pas), o que
// obrigava a ter cada bau de cada guilda em memoria desde o inicio. Aqui o
// bau e lido quando alguem o abre.
func (h *Handler) loadGuildChest(ctx context.Context, g *guild.Guild) error {
	if h.Items == nil || g == nil || g.Chest == nil || g.Chest.IsLoaded() {
		return nil
	}
	set, err := h.Items.GuildChest(ctx, uint32(g.Index))
	if err != nil {
		return err
	}
	for _, a := range set.Avisos {
		h.log().Warn("bau de guilda", "guilda", g.Index, "aviso", a)
	}
	for _, a := range g.Chest.LoadItems(set.Slots) {
		h.log().Warn("bau de guilda", "guilda", g.Index, "aviso", a)
	}
	return nil
}

// sendGuildChestPermission e o porte de TPlayer.SendGuildChestPermission
// ($F10).
func (h *Handler) sendGuildChestPermission(c *netsrv.Conn, s *Session, g *guild.Guild) {
	if c == nil {
		return
	}
	var pkt packets.GuildChestPermissionPacket
	pkt.Header.Index = s.ClientID
	if g.CanOf(s.CharID()).UseChest {
		pkt.CanUseGuildChest = 1
	}
	if err := c.SendStruct(CodeGuildChestPermission, pkt); err != nil {
		h.log().Debug("enviar permissao de bau", "usuario", s.Username, "erro", err)
	}
}

// sendGuildChest e o porte de TPlayer.SendGuildChest ($D58).
func (h *Handler) sendGuildChest(c *netsrv.Conn, s *Session, g *guild.Guild) {
	if c == nil || g == nil {
		return
	}
	var pkt packets.GuildChestPacket
	pkt.Header.Index = s.ClientID
	pkt.Content = g.Chest.Snapshot()
	if err := c.SendStruct(CodeGuildChest, pkt); err != nil {
		h.log().Debug("enviar bau de guilda", "usuario", s.Username, "erro", err)
	}
}

// refreshGuildChestGold e o porte de TPlayer.RefreshGuildChestGold ($D18).
func (h *Handler) refreshGuildChestGold(c *netsrv.Conn, s *Session, g *guild.Guild) {
	if c == nil || g == nil {
		return
	}
	var pkt packets.RefreshGuildChestGoldPacket
	pkt.Header.Index = s.ClientID
	pkt.GuildIndex = g.Index
	pkt.Unk = 1
	pkt.Gold = g.Chest.Gold()
	if err := c.SendStruct(CodeRefreshGuildChestGold, pkt); err != nil {
		h.log().Debug("enviar ouro do bau", "usuario", s.Username, "erro", err)
	}
}

// closeGuildChest e o porte de TPacketHandlers.CloseGuildChest ($F2F).
//
// A origem comparava "MemberInChest = FindMemberFromCharIndex(
// Player.Base.Character.CharIndex)" aqui, mas OpenChest tinha gravado o
// indice achado por "Player.Character.Index" — outro campo, sempre zero. Os
// dois lados nunca podiam casar.
func (h *Handler) closeGuildChest(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Guilds == nil {
		return nil
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil || g.Chest == nil {
		return nil
	}
	if !g.Chest.Close(s.CharID()) {
		return nil
	}
	if s.OpenedOption == NPCOptionGuildChest {
		s.OpenedNPC = 0
		s.OpenedOption = 0
	}
	h.persistGuildChest(ctx, g)
	return nil
}

// changeChestGold e o porte de TPacketHandlers.ChangeGold ($F59).
//
// Value e Int64 e vem do cliente: positivo deposita, negativo retira.
//
// Diferencas centrais em relacao a origem:
//
//   - A origem chamava "abs(Packet.Value)" sem recusar math.MinInt64, cujo
//     modulo nao cabe no Int64 e volta negativo. Com esse valor as duas
//     guardas de saldo passavam (comparar com um numero negativo) e o ouro do
//     jogador e do armazem iam os dois para um valor absurdo.
//
//   - No bau, a origem fazia "Chest.Gold := Chest.Gold + UInt64(Value)": para
//     Value negativo o UInt64() e um numero enorme que, por dar a volta,
//     acerta a subtracao — mas a guarda de saldo comparava um UInt64 com um
//     Int64, e no deposito nao havia teto nenhum, entao o bau podia dar a
//     volta e zerar.
//
//   - A origem exigia "OpennedOption = 11 and OpennedNPC > 0" para o bau e
//     tinha a mesma guarda COMENTADA para o armazem.
func (h *Handler) changeChestGold(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.ChangeChestGoldPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.Value == 0 {
		return nil
	}
	if pkt.Value == math.MinInt64 {
		h.log().Warn("pedido de ouro com o minimo do Int64",
			"usuario", s.Username, "personagem", s.Avatar.Name)
		return nil
	}

	switch pkt.ChestType {
	case store.SlotTypeStorage:
		return h.changeStorageGold(ctx, c, s, pkt.Value)
	case store.SlotTypeGuildChest:
		return h.changeGuildChestGold(ctx, c, s, pkt.Value)
	default:
		// A origem recusava fora de 2..3 antes do case; mantido.
		return nil
	}
}

// changeStorageGold move ouro entre a bolsa e o armazem.
func (h *Handler) changeStorageGold(ctx context.Context, c *netsrv.Conn, s *Session, valor int64) error {
	if !storageOpen(s) {
		h.message(c, "Abra o armazem para mover ouro.")
		return nil
	}
	if valor > 0 {
		// Deposito: sai da bolsa, entra no armazem.
		if s.Avatar.Gold < valor {
			return nil
		}
		if s.StorageGold > MaxStorageGold-valor {
			h.message(c, fmt.Sprintf("Ouro maximo no armazem: %d.", MaxStorageGold))
			return nil
		}
		s.Avatar.Gold -= valor
		s.StorageGold += valor
	} else {
		saque := -valor
		if s.StorageGold < saque {
			return nil
		}
		if s.Avatar.Gold > MaxGold-saque {
			h.message(c, fmt.Sprintf("Ouro maximo: %d.", MaxGold))
			return nil
		}
		s.StorageGold -= saque
		s.Avatar.Gold += saque
	}

	h.sendRefreshMoney(c, s)
	h.sendStorage(c, s, StorageTypePlayer)
	h.persistStorageGold(ctx, s)
	h.log().Info("ouro movido no armazem",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"valor", valor, "bolsa", s.Avatar.Gold, "armazem", s.StorageGold)
	return nil
}

// changeGuildChestGold move ouro entre a bolsa e o bau da guilda.
func (h *Handler) changeGuildChestGold(ctx context.Context, c *netsrv.Conn, s *Session, valor int64) error {
	g, ok := h.chestInUse(c, s)
	if !ok {
		return nil
	}

	if valor > 0 {
		if s.Avatar.Gold < valor {
			return nil
		}
	} else if s.Avatar.Gold > MaxGold+valor {
		// valor e negativo, entao MaxGold+valor e o teto menos o saque.
		h.message(c, fmt.Sprintf("Ouro maximo: %d.", MaxGold))
		return nil
	}

	novo, err := g.Chest.MoveGold(valor)
	switch {
	case errors.Is(err, guild.ErrChestNoGold):
		return nil
	case errors.Is(err, guild.ErrChestGoldCap):
		h.message(c, fmt.Sprintf("Ouro maximo no bau: %d.", guild.MaxChestGold))
		return nil
	case err != nil:
		h.log().Error("mover ouro do bau",
			"guilda", g.Index, "usuario", s.Username, "erro", err)
		return nil
	}

	// O ouro do jogador anda ao contrario do do bau.
	s.Avatar.Gold -= valor
	g.Chest.Touch(h.now())

	h.sendRefreshMoney(c, s)
	h.refreshGuildChestGold(c, s, g)
	h.persistGuildChestGold(ctx, g)
	h.log().Info("ouro movido no bau da guilda",
		"usuario", s.Username, "personagem", s.Avatar.Name,
		"guilda", g.Index, "valor", valor, "bau", novo)
	return nil
}

// chestInUse devolve a guilda quando o jogador tem o bau aberto e pode mexer
// nele. Reune as cinco guardas que a origem repetia em cada ponto de acesso.
func (h *Handler) chestInUse(c *netsrv.Conn, s *Session) (*guild.Guild, bool) {
	if h.Guilds == nil {
		return nil, false
	}
	if s.OpenedOption != NPCOptionGuildChest || s.OpenedNPC == 0 {
		return nil, false
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil || g.Chest == nil {
		return nil, false
	}
	if !g.Chest.IsOpenFor(s.CharID()) {
		return nil, false
	}
	if !g.CanOf(s.CharID()).UseChest {
		h.message(c, "Voce nao tem permissao para isso.")
		return nil, false
	}
	return g, true
}

// persistStorageGold grava o ouro do armazem.
func (h *Handler) persistStorageGold(ctx context.Context, s *Session) {
	if h.AccountGold == nil || s.Account.ID == 0 {
		return
	}
	if err := h.AccountGold.SaveStorageGold(ctx, s.Account.ID, s.StorageGold); err != nil {
		h.log().Error("gravar ouro do armazem",
			"usuario", s.Username, "conta", s.Account.ID, "erro", err)
	}
}

// persistGuildChestGold grava o ouro do bau.
func (h *Handler) persistGuildChestGold(ctx context.Context, g *guild.Guild) {
	if h.GuildDB == nil || g == nil {
		return
	}
	if err := h.GuildDB.SaveChestGold(ctx, g.Index, g.Chest.Gold()); err != nil {
		h.log().Error("gravar ouro do bau", "guilda", g.Index, "erro", err)
	}
}

// persistGuildChest grava os itens e o ouro do bau.
func (h *Handler) persistGuildChest(ctx context.Context, g *guild.Guild) {
	if g == nil || g.Chest == nil {
		return
	}
	h.persistGuildChestGold(ctx, g)
	if h.Items == nil {
		return
	}
	set := store.ItemSet{Slots: make([]model.Item, guild.ChestSlots)}
	g.Chest.Do(func(itens []model.Item) { copy(set.Slots, itens) })
	if err := h.Items.SaveGuildChest(ctx, uint32(g.Index), set); err != nil {
		h.log().Error("gravar itens do bau", "guilda", g.Index, "erro", err)
	}
}

// sendSignalData manda um $xxx com um unico DWORD. Porte de TPlayer.SendData.
func (h *Handler) sendSignalData(c *netsrv.Conn, clientID, code uint16, data uint32) error {
	if c == nil {
		return nil
	}
	var pkt packets.SignalData
	pkt.Header.Index = clientID
	pkt.Data = data
	return c.SendStruct(code, pkt)
}

// sendHeaderSignal manda um pacote so com o cabecalho. Porte de
// TPlayer.SendSignal.
func (h *Handler) sendHeaderSignal(c *netsrv.Conn, clientID, code uint16) error {
	if c == nil {
		return nil
	}
	var pkt packets.PacketHeader
	pkt.Index = clientID
	return c.SendStruct(code, pkt)
}

// ChestTick fecha os baus de guilda sem acao e avisa quem os tinha aberto.
//
// Substitui a TCloseChestThread da origem, que era uma thread por abertura e
// que, ao vencer o prazo, mandava o sinal mas NAO fechava o bau.
func (h *Handler) ChestTick() {
	if h.Guilds == nil {
		return
	}
	for _, aviso := range h.Guilds.SweepChests(h.now()) {
		h.log().Info("bau de guilda fechado por inatividade",
			"guilda", aviso.GuildIndex, "personagem", aviso.CharIndex)

		g := h.Guilds.Get(aviso.GuildIndex)
		if g != nil {
			h.persistGuildChest(context.Background(), g)
		}
		if h.Chan == nil {
			continue
		}
		_, c, s, ok := h.byCharID(aviso.CharIndex)
		if !ok || s == nil {
			continue
		}
		if s.OpenedOption == NPCOptionGuildChest {
			s.OpenedNPC = 0
			s.OpenedOption = 0
		}
		if err := h.sendHeaderSignal(c, s.ClientID, CodeCloseGuildChest); err != nil {
			h.log().Debug("avisar fechamento do bau",
				"personagem", aviso.CharIndex, "erro", err)
		}
	}
}

// chestSlotOK confere a faixa do slot do bau.
//
// A origem fazia "if (Packet.srcSlot < 50)" com srcSlot sendo WORD, o que
// cobria a faixa; o que faltava era a guarda de guilda valida antes de
// "@Guild.Chest.Items[Packet.srcSlot]".
func chestSlotOK(slot int) bool {
	return slot >= 0 && slot < guild.ChestSlots
}

// moveGuildChestItem e o porte dos ramos GUILDCHEST_TYPE de
// TPacketHandlers.MoveItem ($70F).
//
// O movimento inteiro acontece com o bau travado: na origem os dois lados
// eram ponteiros crus para a memoria global, e dois membros da mesma guilda
// arrastando o mesmo slot ao mesmo tempo duplicavam o item.
func (h *Handler) moveGuildChestItem(ctx context.Context, c *netsrv.Conn,
	s *Session, pkt packets.MoveItemPacket) error {

	g, ok := h.chestInUse(c, s)
	if !ok {
		return nil
	}

	src, dest := int(pkt.SrcSlot), int(pkt.DestSlot)
	noBauNaOrigem := pkt.SrcType == store.SlotTypeGuildChest

	// Slot a slot dentro do proprio bau.
	if pkt.SrcType == store.SlotTypeGuildChest && pkt.DestType == store.SlotTypeGuildChest {
		if !chestSlotOK(src) || !chestSlotOK(dest) || src == dest {
			return nil
		}
		var mexeu bool
		var recusa string
		g.Chest.Do(func(itens []model.Item) {
			if itens[src].IsEmpty() {
				return
			}
			if m, pode := h.itemPortado(itens[src], itens[dest]); !pode {
				recusa = m
				return
			}
			h.trocarOuEmpilhar(&itens[src], &itens[dest])
			mexeu = true
		})
		if recusa != "" {
			h.message(c, recusa)
			return nil
		}
		if !mexeu {
			return nil
		}
		h.sendChestSlot(c, s, g, dest)
		h.sendChestSlot(c, s, g, src)
		g.Chest.Touch(h.now())
		h.persistGuildChest(ctx, g)
		h.log().Info("item movido dentro do bau",
			"usuario", s.Username, "guilda", g.Index, "de", src, "para", dest)
		return nil
	}

	// Um lado no bau, outro no inventario ou no armazem.
	slotBau, tipoFora, slotFora := dest, int(pkt.SrcType), src
	if noBauNaOrigem {
		slotBau, tipoFora, slotFora = src, int(pkt.DestType), dest
	}
	if !chestSlotOK(slotBau) {
		return nil
	}
	if tipoFora != store.SlotTypeInventory && tipoFora != store.SlotTypeStorage {
		// Equipamento e pran nao trocam direto com o bau.
		return nil
	}
	fora, err := h.resolveSlot(c, s, tipoFora, slotFora, !noBauNaOrigem)
	if err != nil {
		h.log().Debug("movimento de bau recusado fora do bau",
			"usuario", s.Username, "tipo", tipoFora,
			"slot", slotFora, "motivo", err)
		return nil
	}

	var mexeu bool
	var recusa string
	g.Chest.Do(func(itens []model.Item) {
		noBau := &itens[slotBau]
		origem, destino := noBau, fora
		if !noBauNaOrigem {
			origem, destino = fora, noBau
		}
		if origem.IsEmpty() {
			return
		}
		if m, pode := h.itemPortado(*origem, *destino); !pode {
			recusa = m
			return
		}
		h.trocarOuEmpilhar(origem, destino)
		mexeu = true
	})
	if recusa != "" {
		h.message(c, recusa)
		return nil
	}
	if !mexeu {
		return nil
	}

	h.sendChestSlot(c, s, g, slotBau)
	h.sendRefreshItem(c, s, uint8(tipoFora), uint16(slotFora), *fora)
	g.Chest.Touch(h.now())
	h.persistGuildChest(ctx, g)
	h.log().Info("item movido entre o bau e o jogador",
		"usuario", s.Username, "guilda", g.Index,
		"slot_bau", slotBau, "tipo_fora", tipoFora, "slot_fora", slotFora)
	return nil
}

// sendChestSlot manda o $F0E de um slot do bau.
func (h *Handler) sendChestSlot(c *netsrv.Conn, s *Session, g *guild.Guild, slot int) {
	it, err := g.Chest.At(slot)
	if err != nil {
		return
	}
	h.sendRefreshItem(c, s, store.SlotTypeGuildChest, uint16(slot), it)
}

// chestOnDisconnect solta o bau da guilda quando o jogador sai.
//
// A origem nao tinha nada assim: quem abria o bau e caia deixava
// MemberInChest apontando para ele, e como o unico caminho que chamava
// CloseChest era o pacote $F2F do cliente, o bau da guilda ficava trancado
// para todos os membros ate o servidor reiniciar.
func (h *Handler) chestOnDisconnect(s *Session) {
	if h.Guilds == nil || s == nil {
		return
	}
	g := h.Guilds.Of(s.CharID())
	if g == nil || g.Chest == nil {
		return
	}
	if !g.Chest.Close(s.CharID()) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	h.persistGuildChest(ctx, g)
	h.log().Info("bau de guilda solto na saida do jogador",
		"usuario", s.Username, "guilda", g.Index)
}
