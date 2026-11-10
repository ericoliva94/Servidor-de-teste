package game

import (
	"aika/internal/charcreate"
	"aika/internal/devir"
	"aika/internal/nation"
	"aika/internal/pran"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aika/internal/auction"
	"aika/internal/auth"
	"aika/internal/combat"
	"aika/internal/dropdata"
	"aika/internal/dungeon"
	"aika/internal/entity"
	"aika/internal/guild"
	"aika/internal/itemfn"
	"aika/internal/mail"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/skillfn"
	"aika/internal/store"
	"aika/internal/titledata"
)

// Codigos de pacote do canal.
const (
	CodeCheckLogin = 0x685 // cliente -> servidor
	CodeCharList   = 0x901 // servidor -> cliente
	CodeClientMsg  = 0x984 // servidor -> cliente (mensagem de texto, PPacket_984 de Packets.pas)
)

// Offsets dos campos do $685 depois de decifrado, como CheckLogin os le a mao.
const (
	offLoginUsername    = 16
	offLoginVersion     = 66
	offLoginTokenLegacy = 72
	offLoginTokenModern = 227
	loginFieldSize      = 32
	minCheckLoginPacket = 104
)

// Characters busca os personagens de uma conta.
type Characters interface {
	ByAccount(ctx context.Context, accountID uint32) ([store.MaxCharacterSlots]store.Character, []string, error)
}

// SkillStore le e grava a arvore de skills.
type SkillStore interface {
	ByCharacter(ctx context.Context, charID uint32) (model.SkillsList, []string, error)
	Save(ctx context.Context, charID uint32, list model.SkillsList) error
}

// GuildStore le e grava as guildas.
type GuildStore interface {
	All(ctx context.Context) ([]*guild.Guild, []string, error)
	Save(ctx context.Context, g *guild.Guild) error
	Delete(ctx context.Context, index uint16) error
	// SaveChestGold grava so o ouro do bau: um UPDATE de uma coluna, para
	// mover ouro nao reescrever a lista de membros inteira.
	SaveChestGold(ctx context.Context, index uint16, gold uint64) error
}

// MailStore guarda o correio dos personagens.
type MailStore interface {
	// Box le a caixa de um personagem.
	Box(ctx context.Context, charID uint32) (*mail.Box, error)
	// Add poe uma carta na caixa de um personagem, achado pelo nome.
	Add(ctx context.Context, nome string, m mail.Mail) error
	// AddTo poe uma carta na caixa de um personagem pelo id.
	AddTo(ctx context.Context, charID uint32, m mail.Mail) error
	// Save grava uma carta que mudou.
	Save(ctx context.Context, charID uint32, m mail.Mail) error
	// Remove apaga uma carta.
	Remove(ctx context.Context, charID uint32, index uint64) error
	// MarkRead marca a carta como lida.
	MarkRead(ctx context.Context, charID uint32, index uint64) error
	// NextIndex devolve o proximo indice de carta.
	NextIndex(ctx context.Context) (uint64, error)
	// CharacterExists diz se o personagem existe.
	CharacterExists(ctx context.Context, nome string) (bool, error)
}

// AuctionStore guarda as ofertas do leilao.
type AuctionStore interface {
	// NextIndex devolve o proximo indice de oferta.
	NextIndex(ctx context.Context) (uint64, error)
	// Register grava uma oferta nova.
	Register(ctx context.Context, o auction.Offer) error
	// Get busca uma oferta pelo indice.
	Get(ctx context.Context, index uint64) (*auction.Offer, error)
	// Active lista as ofertas valendo.
	Active(ctx context.Context, limite int) ([]auction.Offer, error)
	// ByOwner lista as ofertas de um personagem.
	ByOwner(ctx context.Context, charIndex uint32, limite int) ([]auction.Offer, error)
	// Close fecha a oferta e diz se ela estava aberta. Fechar e o que impede
	// dois compradores de levar o mesmo item.
	Close(ctx context.Context, index uint64, compradorCharIndex uint32) (bool, error)
	// Expired lista as ofertas que passaram do prazo.
	Expired(ctx context.Context, agora time.Time) ([]auction.Offer, error)
}

// ItemSource busca e grava os itens de um personagem e de uma conta.
type ItemSource interface {
	Equip(ctx context.Context, charID uint32) (store.ItemSet, error)
	Inventory(ctx context.Context, charID uint32) (store.ItemSet, error)
	Storage(ctx context.Context, accountID uint32) (store.ItemSet, error)
	Cash(ctx context.Context, accountID uint32) (store.ItemSet, error)
	SaveEquip(ctx context.Context, charID uint32, set store.ItemSet) error
	SaveInventory(ctx context.Context, charID uint32, set store.ItemSet) error
	SaveStorage(ctx context.Context, accountID uint32, set store.ItemSet) error
	// GuildChest e SaveGuildChest tomam o INDICE da guilda como dono, que e o
	// que a origem gravava em owner_id com slot_type 3.
	GuildChest(ctx context.Context, guildIndex uint32) (store.ItemSet, error)
	SaveGuildChest(ctx context.Context, guildIndex uint32, set store.ItemSet) error
	SaveCash(ctx context.Context, accountID uint32, set store.ItemSet) error
}

// StorageGoldStore grava o ouro do armazem da conta.
type StorageGoldStore interface {
	SaveStorageGold(ctx context.Context, accountID uint32, gold int64) error
}

// CashStore grava o saldo premium da conta.
type CashStore interface {
	SaveCash(ctx context.Context, accountID uint32, cash uint32) error
}

// Handler atende os pacotes do canal.
type Handler struct {
	Channel  uint8
	Accounts auth.Accounts
	// AccountGold grava o ouro do armazem. Nil desliga a gravacao.
	AccountGold StorageGoldStore
	// AccountCash grava o saldo premium. Nil desliga a gravacao.
	AccountCash CashStore
	// Premium e o catalogo da loja premium (PI.bin); nil recusa a loja em vez
	// de ler fora do vetor como a origem fazia.
	Premium   PremiumTable
	Chars     Characters
	Items     ItemSource
	SkillDB   SkillStore
	GuildDB   GuildStore
	MailDB    MailStore
	AuctionDB AuctionStore
	// ItemBarDB guarda a barra de atalhos e TeleportDB as posicoes de teleporte.
	// Nil desliga a persistencia.
	ItemBarDB  ItemBarStore
	TeleportDB TeleportStore
	// Devir e o estado dos templos da nacao deste servidor e DevirDB a sua
	// persistencia. Nil desliga reliquias e devires.
	Devir   *devir.Registry
	DevirDB DevirStore
	// Nations guarda tesouro, taxas e guildas de grade das tres nacoes e
	// NationDB a persistencia. Nil desliga as informacoes de nacao.
	Nations  *nation.Registry
	NationDB NationStore
	// CharCreate grava personagens novos e Templates sao os modelos de classe
	// (BaseAccs). Sem eles a criacao e recusada com aviso.
	CharCreate CharacterCreator
	Templates  [charcreate.NumClasses]charcreate.Template
	// CharDelete marca e conclui a exclusao de personagens. Nil so mexe na
	// memoria da sessao.
	CharDelete CharacterDeleter
	// CharSave grava o progresso do personagem. Nil desliga a gravacao.
	CharSave CharacterSaver
	// FriendDB guarda as amizades. Nil mantem a lista so em memoria.
	FriendDB FriendStore
	// CouponDB resgata cupons. Nil responde Cupom indisponivel.
	CouponDB CouponStore
	// PranDB guarda os nomes das prans. Nil responde Pran indisponivel.
	PranDB PranStore
	// TitleDB guarda os titulos do personagem e TitleTable e o Title.bin. Nil
	// deixa o jogador sem titulos.
	TitleDB TitleStore
	// EventDB guarda os itens de evento e o premio diario. Nil responde que nao
	// ha itens.
	EventDB    EventStore
	TitleTable *titledata.Table
	// Handoffs e ChannelStatus ligam os canais do processo na troca de canal.
	// Nil recusa a troca.
	Handoffs *Handoffs
	// moved liga a chave de um grupo que viajou ao grupo refeito neste canal.
	moved         map[uint32]uint16
	movedMu       sync.Mutex
	ChannelStatus func(id uint8) ChannelState
	// GMDB guarda as contas de painel; nil desliga a ferramenta de GM. Channels
	// sao todos os canais do processo (o painel age em qualquer um).
	GMDB       GMStore
	GMCmds     GMCommandStore
	GMFounders GMFounderStore
	// Nation e a nacao do canal (NationIndex do SL.bin); zero = o canal nao da
	// nacao. AccountNation grava a escolha de nacao da conta.
	Nation        uint8
	AccountNation AccountNationStore
	Channels      []*Handler
	// PranIDs reparte os ClientID das prans em jogo.
	PranIDs  *pran.IDs
	Numerics NumericStore
	// Chan e o registro de quem esta conectado a este canal.
	Chan *Channel
	// Mobs e o mundo de mobs deste canal.
	Mobs *MobWorld
	// Parties guarda os grupos deste canal.
	Parties *PartyRegistry
	// Guilds guarda as guildas.
	Guilds *guild.Registry
	// GuildCost e o custo em ouro de criar guilda; zero usa o padrao.
	GuildCost int64
	// Drops e a tabela de listas de drop.
	Drops *dropdata.Table
	// MapObjs guarda os objetos no chao.
	MapObjs *MapObjects

	// dots guarda o dano continuo em mobs (jogadores guardam o seu na sessao).
	dots dotRegistry

	// duelSeq numera as bandeiras de duelo.
	duelSeq duelSeq

	// Dungeons guarda as instancias de dungeon abertas e DungeonLobbies as
	// telas de confirmacao. DungeonTable e a tabela carregada dos CSV.
	Dungeons       *dungeon.Registry
	DungeonLobbies *dungeon.Lobbies
	DungeonTable   DungeonTable
	// MutantTest liga o MODO DE TESTE dos mutantes do nivel 60 (drop de acessorio
	// e defesa reduzida). Nil por padrao.
	MutantTest *MutantTest
	// DropRateBonus e o efeito de reliquia que empurra o sorteio para as
	// raridades maiores.
	DropRateBonus uint16
	// Exp e a tabela de experiencia por nivel (ExpList).
	Exp ExpTable
	// PranExp e a PranExpList (experiencia por nivel da pran).
	PranExp []uint32
	// ExpMultiplier e o EXP_MULTIPLIER do .ini; zero usa 1.
	ExpMultiplier uint16
	// RateEffect5 e o RATE_EFFECT5 do .ini; zero usa 10, como GlobalDefs.pas.
	RateEffect5 uint16
	// Atributos de defesa que todo mob tem, do .ini: MOB_ESQUIVA,
	// MOB_CRIT_RES e MOB_DUPLO_RES.
	MobEsquiva  uint16
	MobCritRes  uint16
	MobDuploRes uint16
	// WatchDistance e o DISTANCE_TO_WATCH do .ini; zero usa o padrao.
	WatchDistance uint16
	// ForgetDistance e o DISTANCE_TO_FORGET do .ini; zero usa o padrao.
	ForgetDistance uint16
	// Durations da a duracao dos buffs (SkillData); nil trata tudo como 0.
	Durations entity.Durations
	// Data sao as tabelas .bin que as contas de item precisam; nil zera os
	// bonus em vez de quebrar.
	Data itemfn.Tabelas
	// Skills e a tabela SkillData; nil recusa toda skill em vez de ler
	// memoria fora do vetor como a origem fazia.
	SkillTable skillfn.Tabela
	// Rand e a fonte de sorteio do combate; nil usa a global.
	Rand combat.Rand
	Now  func() time.Time
	Log  *slog.Logger
	// ServerVersion e o SERVER_VERSION do .ini, so para registro: o original
	// aceita qualquer versao e apenas anota a diferenca.
	ServerVersion uint16
}

// Erros de recusa do canal.
var (
	ErrShortLoginPacket = errors.New("game: pacote de login curto")
	ErrAccountNotFound  = errors.New("game: conta nao encontrada")
	ErrAccountBlocked   = errors.New("game: conta bloqueada")
	ErrInvalidToken     = errors.New("game: token invalido")
)

// HandlePacket despacha pelo codigo do pacote.
func (h *Handler) HandlePacket(ctx context.Context, c *netsrv.Conn, p protocol.Packet) error {
	s, _ := c.Session.(*Session)
	if s == nil {
		s = NewSession(h.Channel)
		c.Session = s
	}

	switch p.Code {
	case CodeCheckLogin:
		return h.checkLogin(ctx, c, s, p)
	case CodeNumericToken:
		return h.numericToken(ctx, c, s, p)
	case CodeMovement:
		return h.movement(ctx, c, s, p)
	case CodeClientSay:
		return h.clientSay(ctx, c, s, p)
	case CodeUpdateRotation:
		return h.updateRotation(ctx, c, s, p)
	case CodeRequestCharInfo:
		return h.requestCharInfo(ctx, c, s, p)
	case CodeRequestTime:
		return h.requestServerTime(ctx, c, s, p)
	case CodeMoveItem:
		return h.moveItem(ctx, c, s, p)
	case CodeDeleteItem:
		return h.deleteItem(ctx, c, s, p)
	case CodeAttack:
		return h.attack(ctx, c, s, p)
	case CodeRevive:
		return h.revive(ctx, c, s, p)
	case CodeSendParty:
		return h.sendParty(ctx, c, s, p)
	case CodeAcceptParty:
		return h.acceptParty(ctx, c, s, p)
	case CodeKickParty:
		return h.kickParty(ctx, c, s, p)
	case CodeDestroyParty:
		return h.destroyParty(ctx, c, s, p)
	case CodeGiveLeaderParty:
		return h.giveLeaderParty(ctx, c, s, p)
	case CodeAddSelfParty:
		return h.addSelfParty(ctx, c, s, p)
	case CodeLearnSkill:
		return h.learnSkill(ctx, c, s, p)
	case CodeUseSkill:
		return h.useSkill(ctx, c, s, p)
	case CodeResetSkills:
		return h.resetSkills(ctx, c, s, p)
	case CodeCancelSkill:
		return h.cancelSkill(ctx, c, s, p)
	case CodeCollectMapItem:
		return h.collectMapItem(ctx, c, s, p)
	case CodeCancelCollect:
		return h.cancelCollect(ctx, c, s, p)
	case CodeCreateGuild:
		return h.createGuild(ctx, c, s, p)
	case CodeInviteGuild:
		return h.inviteGuild(ctx, c, s, p)
	case CodeInviteGuildAccept:
		return h.acceptGuild(ctx, c, s, p)
	case CodeInviteGuildDeny:
		return h.denyGuild(ctx, c, s, p)
	case CodeKickGuild:
		return h.kickGuild(ctx, c, s, p)
	case CodeExitGuild:
		return h.exitGuild(ctx, c, s, p)
	case CodeGuildRank:
		return h.guildRank(ctx, c, s, p)
	case CodeGuildMaster:
		return h.guildMaster(ctx, c, s, p)
	case CodeGuildNotices:
		return h.guildNotices(ctx, c, s, p)
	case CodeGuildSite:
		return h.guildSite(ctx, c, s, p)
	case CodeGuildRanksConfig:
		return h.guildRanksConfig(ctx, c, s, p)
	case CodeMailConfirm:
		return h.confirmMail(ctx, c, s, p)
	case CodeMailSend:
		return h.sendMail(ctx, c, s, p)
	case CodeMailOpen:
		return h.openMail(ctx, c, s, p)
	case CodeMailWithdraw:
		return h.withdrawMail(ctx, c, s, p)
	case CodeAuctionRegister:
		return h.registerAuctionItem(ctx, c, s, p)
	case CodeAuctionList:
		return h.listAuctionItems(ctx, c, s, p)
	case CodeAuctionOwn:
		return h.ownAuctionItems(ctx, c, s, p)
	case CodeAuctionBuy:
		return h.buyAuctionItem(ctx, c, s, p)
	case CodeAuctionCancel:
		return h.cancelAuctionOffer(ctx, c, s, p)
	case CodeDungeonEnter:
		return h.requestEnterDungeon(ctx, c, s, p)
	case CodeDungeonConfirm:
		return h.confirmDungeon(ctx, c, s, p)
	case CodeOpenNPC:
		return h.openNPC(ctx, c, s, p)
	case CodeCloseNPCOption:
		return h.closeNPCOption(ctx, c, s, p)
	case CodeChangeChestGold:
		return h.changeChestGold(ctx, c, s, p)
	case CodeCloseGuildChest:
		return h.closeGuildChest(ctx, c, s, p)
	case CodeUseItem:
		return h.useItem(ctx, c, s, p)
	case CodeUseBuffItem:
		return h.useBuffItem(ctx, c, s, p)
	case CodeBuyItemCash:
		return h.buyItemCash(ctx, c, s, p)
	case CodeSendGift:
		return h.sendGift(ctx, c, s, p)
	case CodeUnsealItem:
		return h.unsealItem(ctx, c, s, p)
	case CodeGuildAllyRequest:
		return h.requestGuildAlly(ctx, c, s, p)
	case CodeUpdateNationTaxes:
		return h.updateNationTaxes(ctx, c, s, p)
	case CodeNationGold:
		return h.updateNationGold(c, s, p)
	case CodeRequestDevirInfo:
		return h.requestDevirInfo(ctx, c, s, p)
	case CodeMoveToReliquary:
		return h.moveToReliquary(ctx, c, s, p)
	case CodeTradeRequest:
		return h.tradeRequest(ctx, c, s, p)
	case CodeTradeResponse:
		return h.tradeResponse(ctx, c, s, p)
	case CodeTradeRefresh:
		return h.tradeRefresh(ctx, c, s, p)
	case CodeTradeCancel:
		return h.tradeCancel(ctx, c, s, p)
	case CodeChangeChannel:
		return h.changeChannel(ctx, c, s, p)
	case CodeLoginIntoChannel:
		return h.loginIntoChannel(ctx, c, s, p)
	case CodeSendToWorldSends:
		return h.sendToWorldSends(ctx, c, s, p)
	case CodeGMLogin:
		return h.gmLogin(ctx, c, s, p)
	case CodeGMMove:
		return h.gmMove(ctx, c, s, p)
	case CodeGMChat:
		return h.gmChat(ctx, c, s, p)
	case CodeGMGold:
		return h.gmGold(ctx, c, s, p)
	case CodeGMCash:
		return h.gmCash(ctx, c, s, p)
	case CodeGMDisconnect:
		return h.gmDisconnect(ctx, c, s, p)
	case CodeGMLevel:
		return h.gmLevel(ctx, c, s, p)
	case CodeGMBuffs:
		return h.gmBuffs(ctx, c, s, p)
	case CodeGMBan:
		return h.gmBan(ctx, c, s, p)
	case CodeGMRequestPending:
		return h.gmRequestPending(ctx, c, s, p)
	case CodeGMApprove:
		return h.gmApprove(ctx, c, s, p)
	case CodeGMReprove:
		return h.gmReprove(ctx, c, s, p)
	case CodeGMCreateCoupon:
		return h.gmCreateCoupon(ctx, c, s, p)
	case CodeGMRequestNames:
		return h.gmRequestNames(ctx, c, s, p)
	case CodeGMEffect:
		return h.gmEffect(ctx, c, s, p)
	case CodeGMFounderByID:
		return h.gmFounderByID(ctx, c, s, p)
	case CodeGMFounderByName:
		return h.gmFounderByName(ctx, c, s, p)
	case CodeGMFounderCreate:
		return h.gmFounderCreate(ctx, c, s, p)
	case CodeGMFounderOK:
		return h.gmFounderValidate(ctx, c, s, p)
	case CodeGMResetPrans:
		return h.gmResetPrans(ctx, c, s, p)
	case CodeGMSpawnMob:
		return h.gmSpawnMob(ctx, c, s, p)
	case CodeGMEventItem:
		return h.gmEventItem(ctx, c, s, p)
	case CodeGMEventItemAll:
		return h.gmEventItemAll(ctx, c, s, p)
	case CodeReceiveEventItem:
		return h.receiveEventItem(ctx, c, s, p)
	case CodeUpdateActiveTitle:
		return h.updateActiveTitle(ctx, c, s, p)
	case CodeRenamePran:
		return h.renamePran(ctx, c, s, p)
	case CodeReclaimCoupon:
		return h.reclaimCoupon(ctx, c, s, p)
	case CodeDuelRequest:
		return h.requestDuel(ctx, c, s, p)
	case CodeDuelResponse:
		return h.answerDuel(ctx, c, s, p)
	case CodeSendRaid:
		return h.sendRaid(ctx, c, s, p)
	case CodeAcceptRaid:
		return h.acceptRaid(ctx, c, s, p)
	case CodeExitRaid:
		return h.exitRaid(ctx, c, s, p)
	case CodeGiveLeaderRaid:
		return h.giveLeaderRaid(ctx, c, s, p)
	case CodeShopCreate:
		return h.createShop(ctx, c, s, p)
	case CodeShopOpen:
		return h.openShop(ctx, c, s, p)
	case CodeShopBuy:
		return h.buyShopItem(ctx, c, s, p)
	case CodeUpdateMemberPosition:
		return h.updateMemberPosition(ctx, c, s, p)
	case CodeCreateCharacter:
		return h.createCharacter(ctx, c, s, p)
	case CodeRequestDeleteChar:
		return h.requestDeleteChar(ctx, c, s, p)
	case CodeDeleteChar:
		return h.deleteChar(ctx, c, s, p)
	case CodeItemLink:
		return h.itemLink(ctx, c, s, p)
	case CodeChangeItemBar:
		return h.changeItemBar(ctx, c, s, p)
	case CodeSetTeleport:
		return h.setTeleport(ctx, c, s, p)
	case CodeAllAttributes:
		return h.requestAllAttributes(ctx, c, s, p)
	case CodeAllAttributesTarget:
		return h.requestAllAttributesTarget(ctx, c, s, p)
	case CodeRequestCash:
		return h.requestCash(ctx, c, s, p)
	case CodeBackToCharList:
		return h.backToCharList(ctx, c, s, p)
	case CodeGetStatusPoint:
		return h.getStatusPoint(ctx, c, s, p)
	case CodeUpdateAction:
		return h.updateAction(ctx, c, s, p)
	case CodePKMode:
		return h.pkMode(ctx, c, s, p)
	case CodeRemoveBuff:
		return h.removeBuff(ctx, c, s, p)
	case CodeRequestPing:
		return h.requestPing(ctx, c, s, p)
	case CodeMakeItem:
		return h.makeItem(ctx, c, s, p)
	case CodeRepairItems:
		return h.repairItems(ctx, c, s, p)
	case CodeChangeItemAttribute:
		return h.changeItemAttribute(ctx, c, s, p)
	case CodeRenewItem, CodeRenewItemPran:
		return h.renewItem(ctx, c, s, p)
	case CodeAgroupItem:
		return h.agroupItem(ctx, c, s, p)
	case CodeUngroupItem:
		return h.ungroupItem(ctx, c, s, p)
	case CodeAddFriend:
		return h.addFriend(ctx, c, s, p)
	case CodeAddFriendResponse:
		return h.addFriendResponse(ctx, c, s, p)
	case CodeDeleteFriend:
		return h.deleteFriend(ctx, c, s, p)
	case CodeOpenFriendWindow:
		return h.openFriendWindow(ctx, c, s, p)
	case CodeCloseFriendWindow:
		return h.closeFriendWindow(ctx, c, s, p)
	case CodeFriendSay:
		return h.friendSay(ctx, c, s, p)
	default:
		// O original caia no fim do case sem ramo e seguia sem responder. Aqui
		// o codigo desconhecido e registrado, o que torna visivel o que falta
		// portar em vez de o cliente travar calado.
		h.log().Debug("codigo de pacote sem handler",
			"codigo", fmt.Sprintf("%#x", p.Code), "ip", c.IP(),
			"tamanho", p.Size, "estado", s.Status.String())
		return nil
	}
}

// checkLogin e o porte de TPacketHandlers.CheckLogin ($685).
func (h *Handler) checkLogin(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if p.Size < minCheckLoginPacket {
		c.Close()
		return ErrShortLoginPacket
	}

	username := strings.TrimSpace(loginField(p, offLoginUsername))
	legacy := strings.TrimSpace(loginField(p, offLoginTokenLegacy))
	modern := strings.TrimSpace(loginField(p, offLoginTokenModern))
	version := uint16(0)
	if offLoginVersion+2 <= len(p.Data) {
		version = binary.LittleEndian.Uint16(p.Data[offLoginVersion:])
	}

	// O original aceita qualquer versao e so anota a diferenca; mantido.
	if version != h.ServerVersion {
		h.log().Info("versao de cliente diferente da configurada",
			"cliente", version, "configurada", h.ServerVersion, "usuario", username)
	}

	acc, found, err := h.Accounts.ByUsername(ctx, strings.ToLower(username))
	if err != nil {
		c.Close()
		return err
	}
	if !found {
		h.message(c, "Conta nao encontrada.")
		c.Close()
		return ErrAccountNotFound
	}
	if acc.Status == auth.StatusBanned {
		h.message(c, "Conta bloqueada. Entre em contato com o suporte.")
		c.Close()
		return ErrAccountBlocked
	}

	// Prefere o token do cliente atual quando ele casa com o guardado; o
	// offset 72 continua valendo para o layout antigo.
	saved := strings.TrimSpace(acc.Token)
	token := legacy
	if modern != "" && strings.EqualFold(modern, saved) {
		token = modern
	}
	if saved == "" || !strings.EqualFold(strings.TrimSpace(token), saved) {
		h.message(c, "Token invalido.")
		c.Close()
		return ErrInvalidToken
	}
	// Mesma validade do login (300 s). Data no futuro tambem nao vale: no
	// original "(Now - CreationTime) >= 300s" era falso para diferenca
	// negativa, entao uma data adiantada no banco valia para sempre.
	age := h.now().Sub(acc.TokenCreated)
	if age < 0 || age >= auth.TokenLifetime {
		h.message(c, "Token invalido.")
		c.Close()
		return ErrInvalidToken
	}

	chars, avisos, err := h.Chars.ByAccount(ctx, acc.ID)
	if err != nil {
		c.Close()
		return err
	}
	for _, a := range avisos {
		h.log().Warn("lista de personagens", "aviso", a)
	}

	s.Authenticated = true
	s.Username = username
	s.Account = acc
	s.Characters = chars

	// O ClientID sai do registro do canal, nao de uma varredura de vetor.
	//
	// O original percorria Players[1..MAX_CONNECTIONS] procurando slot livre
	// e, quando nao achava, seguia com o indice do fim do laco: o jogador
	// novo escrevia por cima do ultimo slot, que era de outra pessoa.
	if h.Chan != nil {
		id, err := h.Chan.Join(c, s)
		if err != nil {
			h.message(c, "Servidor cheio. Tente novamente em instantes.")
			c.Close()
			return err
		}
		h.log().Debug("id de cliente alocado", "usuario", username, "id", id)
	}

	h.log().Info("usuario conectado",
		"usuario", username, "conta", acc.ID, "canal", h.Channel,
		"id", s.ClientID, "ip", c.IP())

	return h.sendCharList(c, s)
}

// OnDisconnect tira o jogador do canal e da tela de quem o via.
//
// Implementa netsrv.DisconnectHandler. No original a limpeza vivia no
// Connection.Destroy e dependia de cada caminho de saida chamar o Destroy;
// quando um nao chamava, o jogador ficava no vetor Players com Status Playing
// e continuava na lista de visiveis dos outros, que viam um boneco parado.
func (h *Handler) OnDisconnect(c *netsrv.Conn) {
	s, _ := c.Session.(*Session)
	if s == nil {
		return
	}
	if s.Status >= Playing {
		// Gravar antes de tirar da lista: os itens mudaram em memoria e nada
		// mais vai ler esta sessao depois daqui.
		h.saveItems(s)
		h.saveCharacter(s)
		h.saveItemBar(s)
		h.savePrans(s)
		h.saveTitles(s)
		h.pranOnDisconnect(s)
		h.chestOnDisconnect(s)
		h.guildOnDisconnect(s)
		h.dungeonOnDisconnect(s)
		h.friendsOnDisconnect(s)
		h.tradeOnDisconnect(s)
		h.shopOnDisconnect(s)
		h.duelOnDisconnect(s)
		h.leaveParty(s)
		h.removeFromEveryone(s)
	}
	if h.Chan != nil && s.ClientID != 0 {
		h.Chan.Leave(s.ClientID)
	}
	// Em troca de canal o personagem ja foi gravado acima: libera o canal novo,
	// que esta esperando para le-lo.
	if s.handoffAccount != 0 && h.Handoffs != nil {
		h.Handoffs.Saved(s.handoffAccount)
		s.handoffAccount = 0
	}
	s.Status = WaitingLogin
	if s.Username != "" {
		h.log().Info("usuario desconectado",
			"usuario", s.Username, "id", s.ClientID,
			"canal", h.Channel, "ip", c.IP())
	}
}

// sendCharList monta e envia o $901. Porte de TPlayer.SendCharList.
func (h *Handler) sendCharList(c *netsrv.Conn, s *Session) error {
	var pkt packets.SendToCharListPacket
	pkt.Header.Index = s.ClientID
	pkt.AcountID = s.Account.ID

	for i := range pkt.CharactersData {
		src := s.Characters[i]
		dst := &pkt.CharactersData[i]

		copy(dst.Name[:], src.Name)
		if n := len(src.Name); n < len(dst.Name) {
			dst.Name[n] = 0
		}
		dst.Nation = uint16(s.Account.Nation)
		dst.ClassInfo = src.ClassInfo

		// O original faz "Level - 1" num WORD: com Level 0 (slot vazio) isso
		// estoura para 65535. O cliente mostra o valor como esta, entao slot
		// vazio ia com nivel 65535. Aqui o piso e zero.
		if src.Level > 0 {
			dst.Level = src.Level - 1
		}

		dst.Exp = src.Exp
		dst.Gold = src.Gold
		dst.Attributes = model.Attributes{
			Str:  src.Strength,
			Agi:  src.Agility,
			Int:  src.Intelligence,
			Cons: src.Constitution,
			Luck: src.Luck,
		}
		dst.Size = model.Sizes{
			Altura: src.Altura,
			Tronco: src.Tronco,
			Perna:  src.Perna,
			Corpo:  src.Corpo,
		}
		dst.NumericError = src.NumericErrors
		if src.NumericToken != "" {
			dst.NumRegister = 1
		}
		if src.Deleted && !src.DeleteTime.IsZero() {
			dst.DeleteTime = uint32(src.DeleteTime.Unix())
		}
	}

	if s.Status != CharList {
		s.Status = CharList
		s.SubStatus = Senha2
	}
	return c.SendStruct(CodeCharList, pkt)
}

// clientMessage e o TClientMessagePacket como a origem o enviava: 144 bytes.
// O record tem `Msg: string[128]` (129 bytes), mas SendClientMessage grava
// Header.size := 144, entao o ultimo byte nunca vai e o texto cabe em 127
// caracteres mais o terminador.
type clientMessage struct {
	Header packets.PacketHeader
	Null   uint8
	Type1  uint8
	Type2  uint8
	Null1  uint8
	Msg    [128]byte
}

// MsgYellow e o MsgType padrao de SendClientMessage (16, mensagem amarela).
const MsgYellow = 16

// message envia uma mensagem de texto ao cliente, no tipo padrao da origem.
func (h *Handler) message(c *netsrv.Conn, msg string) {
	h.messageType(c, msg, MsgYellow)
}

// messageType envia uma mensagem com o MsgType dado.
func (h *Handler) messageType(c *netsrv.Conn, msg string, tipo uint8) {
	if c == nil {
		return
	}
	var pkt clientMessage
	pkt.Type1 = tipo
	if len(msg) > len(pkt.Msg)-1 {
		msg = msg[:len(pkt.Msg)-1]
	}
	netsrv.PutCString(pkt.Msg[:], msg)
	if err := c.SendStruct(CodeClientMsg, pkt); err != nil {
		h.log().Debug("falha ao enviar mensagem ao cliente",
			"ip", c.IP(), "erro", err)
	}
}

// loginField le um campo de texto de tamanho fixo do pacote de login.
func loginField(p protocol.Packet, off int) string {
	if off < 0 || off+loginFieldSize > len(p.Data) {
		return ""
	}
	return netsrv.CString(p.Data[off : off+loginFieldSize])
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// durations devolve a tabela de duracao de buffs, ou uma vazia.
func (h *Handler) durations() entity.Durations {
	if h.Durations != nil {
		return h.Durations
	}
	return zeroDurations{}
}

// zeroDurations trata todo buff como de duracao zero. E o que acontecia no
// original quando SkillData nao havia sido carregado: o registro vinha
// zerado e o buff expirava na hora.
type zeroDurations struct{}

func (zeroDurations) BuffSeconds(uint16) int { return 0 }

func (h *Handler) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

// saveItems grava equipamento e inventario do personagem em jogo.
//
// A origem gravava em TPlayer.SaveInGame, chamado de varios lugares, e a
// gravacao de saida dependia de o caminho de desconexao chegar la: quando nao
// chegava, tudo que o jogador tinha movido na sessao era perdido. Aqui a
// gravacao fica no mesmo ponto unico de saida que tira o jogador do canal.
func (h *Handler) saveItems(s *Session) {
	if s.SelectedSlot < 0 || s.SelectedSlot >= store.MaxCharacterSlots {
		return
	}
	if h.Items == nil && h.SkillDB == nil {
		return
	}
	charID := s.Characters[s.SelectedSlot].ID
	if charID == 0 {
		return
	}
	// A conexao ja esta morta aqui, entao o contexto e proprio e com prazo:
	// sem isso uma gravacao presa seguraria a goroutine da conexao.
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()

	if h.Items != nil && len(s.Equip.Slots) > 0 {
		if err := h.Items.SaveEquip(ctx, charID, s.Equip); err != nil {
			h.log().Error("gravar equipamento",
				"usuario", s.Username, "personagem", charID, "erro", err)
		}
	}
	if h.Items != nil && len(s.Inventory.Slots) > 0 {
		if err := h.Items.SaveInventory(ctx, charID, s.Inventory); err != nil {
			h.log().Error("gravar inventario",
				"usuario", s.Username, "personagem", charID, "erro", err)
		}
	}
	if h.Items != nil && len(s.Storage.Slots) > 0 && s.Account.ID != 0 {
		if err := h.Items.SaveStorage(ctx, s.Account.ID, s.Storage); err != nil {
			h.log().Error("gravar armazem",
				"usuario", s.Username, "conta", s.Account.ID, "erro", err)
		}
	}
	if h.Items != nil && len(s.Cash.Slots) > 0 && s.Account.ID != 0 {
		if err := h.Items.SaveCash(ctx, s.Account.ID, s.Cash); err != nil {
			h.log().Error("gravar inventario premium",
				"usuario", s.Username, "conta", s.Account.ID, "erro", err)
		}
	}
	if h.AccountCash != nil && s.Account.ID != 0 {
		if err := h.AccountCash.SaveCash(ctx, s.Account.ID, s.CashBalance); err != nil {
			h.log().Error("gravar cash",
				"usuario", s.Username, "conta", s.Account.ID, "erro", err)
		}
	}
	if h.AccountGold != nil && s.Account.ID != 0 {
		if err := h.AccountGold.SaveStorageGold(ctx, s.Account.ID, s.StorageGold); err != nil {
			h.log().Error("gravar ouro do armazem",
				"usuario", s.Username, "conta", s.Account.ID, "erro", err)
		}
	}
	if h.SkillDB != nil {
		if err := h.SkillDB.Save(ctx, charID, s.Skills); err != nil {
			h.log().Error("gravar skills",
				"usuario", s.Username, "personagem", charID, "erro", err)
		}
	}
}

// saveTimeout limita a gravacao de saida.
const saveTimeout = 10 * time.Second
