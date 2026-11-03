package game

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aika/internal/auction"
	"aika/internal/auth"
	"aika/internal/combat"
	"aika/internal/dropdata"
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
)

// Codigos de pacote do canal.
const (
	CodeCheckLogin = 0x685 // cliente -> servidor
	CodeCharList   = 0x901 // servidor -> cliente
	CodeClientMsg  = 0x100 // servidor -> cliente (mensagem de texto)
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
}

// Handler atende os pacotes do canal.
type Handler struct {
	Channel   uint8
	Accounts  auth.Accounts
	Chars     Characters
	Items     ItemSource
	SkillDB   SkillStore
	GuildDB   GuildStore
	MailDB    MailStore
	AuctionDB AuctionStore
	Numerics  NumericStore
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
	// DropRateBonus e o efeito de reliquia que empurra o sorteio para as
	// raridades maiores.
	DropRateBonus uint16
	// Exp e a tabela de experiencia por nivel (ExpList).
	Exp ExpTable
	// ExpMultiplier e o EXP_MULTIPLIER do .ini; zero usa 1.
	ExpMultiplier uint16
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
		h.guildOnDisconnect(s)
		h.leaveParty(s)
		h.removeFromEveryone(s)
	}
	if h.Chan != nil && s.ClientID != 0 {
		h.Chan.Leave(s.ClientID)
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

// message envia uma mensagem de texto ao cliente.
func (h *Handler) message(c *netsrv.Conn, msg string) {
	var pkt packets.ClientMessagePacket
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
	if h.SkillDB != nil {
		if err := h.SkillDB.Save(ctx, charID, s.Skills); err != nil {
			h.log().Error("gravar skills",
				"usuario", s.Username, "personagem", charID, "erro", err)
		}
	}
}

// saveTimeout limita a gravacao de saida.
const saveTimeout = 10 * time.Second
