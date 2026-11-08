// Package game e o servidor de canal: o que atende o cliente depois do login.
//
// Porte do despacho de ServerSocket.pas e dos handlers de PacketHandlers.pas
// que o caminho de entrada usa.
package game

import (
	"time"

	"aika/internal/auth"
	"aika/internal/combat"
	"aika/internal/entity"
	"aika/internal/model"
	"aika/internal/pran"
	"aika/internal/store"
)

// Status e TPlayerStatus de PlayerData.pas, na mesma ordem.
type Status uint8

const (
	WaitingLogin Status = iota
	CharList
	Senha2
	Waiting
	Playing
)

var statusNames = [...]string{"WaitingLogin", "CharList", "Senha2", "Waiting", "Playing"}

func (s Status) String() string {
	if int(s) < len(statusNames) {
		return statusNames[s]
	}
	return "Status(?)"
}

// Session e o estado de um cliente conectado ao canal.
//
// Fica pendurada na conexao (netsrv.Conn.Session) em vez de num slot de um
// vetor global de 1000 posicoes como no original, onde o indice do slot era o
// ClientID e cada acesso precisava acertar a faixa.
type Session struct {
	// ClientID e o identificador que vai no cabecalho dos pacotes. Zero
	// enquanto o cliente nao recebeu um.
	ClientID uint16
	// Channel e o canal a que esta conexao pertence.
	Channel uint8

	Status    Status
	SubStatus Status

	Authenticated bool
	Username      string
	Account       auth.Account
	Characters    [store.MaxCharacterSlots]store.Character

	// SelectedSlot e o personagem escolhido, -1 enquanto nenhum.
	SelectedSlot int

	// Position e a posicao atual. Equivale a
	// Base.PlayerCharacter.LastPos do original.
	Position model.Position
	// AnchorPosition e a posicao em que a lista de visiveis foi calculada
	// pela ultima vez. No original era Character.LastPos, que servia para o
	// mesmo fim: so recalcular a cada 3 celulas andadas.
	AnchorPosition model.Position
	SpeedMove      uint16
	// LastMoved e o instante do ultimo passo aceito. Era
	// TBaseMob.LastMovedTime no original.
	LastMoved time.Time
	// LastShout e o instante do ultimo grito (ShoutTime no original).
	LastShout time.Time
	// LastPing e o instante do ultimo `ping (PingCommandUsed no original).
	LastPing time.Time
	// LastAttack e o instante do ultimo golpe aceito (LastBasicAttack).
	LastAttack time.Time
	// AtkAnim e AtkSkill sao a animacao e a skill do golpe em curso, que vao
	// no $102 (Animation e SkillID). Sem a animacao o cliente nao toca o
	// golpe: no Delphi ela vinha de Packet.Anim, copiada pelo SendDamage.
	AtkAnim  uint8
	AtkSkill uint32
	// AttacksAccumulated conta os golpes para o desgaste da arma.
	AttacksAccumulated int

	// Roller e a roleta de tipo de dano desta sessao. Fica aqui, e nao numa
	// global, porque o vetor de 1000 posicoes e reusado a cada golpe.
	Roller combat.Roller

	Dead bool

	// OpenedNPC e OpenedOption sao o NPC e a opcao de dialogo abertos.
	OpenedNPC    uint16
	OpenedOption uint16

	// PartyRequester e quem convidou este jogador para um grupo, zero quando
	// nao ha convite pendente.
	PartyRequester uint16
	// RaidRequester e quem convidou o grupo deste jogador para uma legiao.
	RaidRequester uint16
	// AllyByLegion marca que o proximo $343 responde a um convite de aliança de
	// guildas e AllyRequester e o lider da aliança que chamou.
	AllyByLegion  bool
	AllyRequester uint16
	// Duel e o duelo do jogador (desafio pendente e duelo em curso).
	Duel duelState
	// couponFails e couponLast limitam as tentativas de cupom.
	couponFails int
	couponLast  time.Time
	// GuildRequester e quem convidou este jogador para uma guilda.
	GuildRequester uint16
	// CanSendMailTo e o destinatario que passou pela conferencia do $3F16.
	CanSendMailTo string
	// ItemBar e a barra de atalhos (24 posicoes) e Teleports a lista de posicoes
	// de teleporte do mago.
	ItemBar   [store.ItemBarSlots]uint32
	Teleports [MaxTeleportSlots]model.Position
	// Action e a ultima animacao de acao (sentar, etc.) que o cliente mandou.
	Action uint32
	// OpenedDevir e OpenedTemple sao o templo (devir) e a vaga que o jogador
	// abriu, NoDevir quando nenhum.
	OpenedDevir  uint8
	OpenedTemple uint8
	// Trade e a negociacao em curso (comercio entre jogadores).
	Trade tradeState
	// Shop e a loja pessoal que o jogador mantem aberta e a que esta olhando.
	Shop shopState
	// NegarCura conta as curas trocadas por dano (EF_ANTICURE); regen guarda a
	// regeneracao de HP e MP que cada buff ativo mantem.
	NegarCura int
	// DefesaPoints (buff 32), BolhaPoints (buff 36) e UniaoDivina (buff 35) sao os
	// contadores que a origem guardava no TBaseMob.
	DefesaPoints uint8
	BolhaPoints  uint8
	UniaoDivina  string
	// ResolutoPoints/ResolutoTime marcam o alvo do Resoluto; LongSkillPos e a posicao
	// do ultimo lancamento (LastPositionLongSkill).
	ResolutoPoints uint8
	ResolutoTime   time.Time
	LongSkillPos   model.Position
	regen          map[uint16]regenState
	dots           map[uint16]dotState
	// Friends e a lista de amigos, as janelas de conversa abertas e os
	// pedidos de amizade pendentes.
	Friends friendState

	// DungeonInstance e a instancia em que o jogador esta, zero fora de
	// dungeon. A origem tinha cinco campos para isso (DungeonLobbyIndex,
	// DungeonLobbyDificult, DungeonID, DungeonIDDificult, DungeonInstanceID)
	// mais dois booleanos (InDungeon, IsInsideDungeon), zerados em sete
	// lugares diferentes e cada um zerando um subconjunto diferente.
	DungeonInstance uint32
	// DungeonID e DungeonDificult sao a dungeon em que ele esta.
	DungeonID       uint32
	DungeonDificult uint32
	// ReturnPosition e onde ele estava antes de entrar. A origem nao
	// guardava: quem saia era jogado num ponto fixo (3399, 564).
	ReturnPosition model.Position

	// Avatar e o personagem escolhido, do jeito que os outros clientes o
	// veem. So tem conteudo a partir do Playing.
	Avatar Avatar

	// Prans sao as duas prans da conta (nil ate o login carregar) e PranClientID
	// o id da que esta no mundo; SpawnedPran e o indice dela, -1 se nenhuma.
	Prans        [2]*pran.Pran
	PranClientID uint16
	SpawnedPran  int
	// pranApplied guarda os efeitos da pran somados ao Ability do dono.
	pranApplied []efv
	// Titles e a lista de titulos do personagem (TTitleList, 96); ActiveTitle e
	// o titulo ativo, com o nivel que a lista tem dele; titleApplied guarda os
	// efeitos do titulo ativo somados ao Ability.
	Titles       [store.MaxTitles]model.TitleData
	ActiveTitle  model.TitleData
	titleApplied []efv
	// handoffAccount e a conta cuja troca de canal esta em andamento; zero se
	// nenhuma.
	handoffAccount uint32
	// gm e o estado de painel da conexao (login em $3202).
	gm gmSession
	// Equip e Inventory sao os itens do personagem em jogo.
	Equip     store.ItemSet
	Inventory store.ItemSet

	// Storage e o armazem da CONTA, nao do personagem: na origem era
	// Account.Header.Storage, compartilhado por todos os personagens da
	// conta. StorageGold e o Gold daquele mesmo registro.
	Storage     store.ItemSet
	StorageGold int64

	// Cash e o inventario premium da CONTA, e CashBalance o saldo dela. Na
	// origem eram Account.Header.CashInventory.Items e .Cash. O que cada slot
	// guarda nao e um indice de item: e o INDICE DO CATALOGO premium, que so
	// vira item ao ser usado.
	Cash        store.ItemSet
	CashBalance uint32

	// Skills e a arvore de habilidades do personagem.
	Skills model.SkillsList
	// UsingSkill e a skill em lancamento, zero quando nao ha nenhuma.
	UsingSkill uint16

	// Base reune as listas de visiveis, buffs e recargas da entidade.
	Base EntityState
}

// EntityState e a parte de TBaseMob que a sessao carrega.
type EntityState struct {
	VisiblePlayers entity.VisibleSet
	VisibleMobs    entity.VisibleSet
	VisibleNPCs    entity.VisibleSet
	Buffs          *entity.Buffs
	Cooldowns      *entity.Cooldowns
	Targets        *entity.TargetList
}

// NewSession cria a sessao de uma conexao nova.
func NewSession(channel uint8) *Session {
	return &Session{
		Channel:      channel,
		Status:       WaitingLogin,
		SelectedSlot: -1,
		OpenedDevir:  NoDevir,
		OpenedTemple: NoDevir,
		Base: EntityState{
			Buffs:     entity.NewBuffs(),
			Cooldowns: entity.NewCooldowns(),
			Targets:   entity.NewTargetList(),
		},
	}
}

// CharID devolve o id do personagem em jogo, zero quando nenhum.
//
// E a chave que a guilda usa: o original guardava
// Character.Base.CharIndex no registro do personagem e o repetia em
// GuildIndex e GuildSlot, que precisavam ficar em sincronia a mao.
func (s *Session) CharID() uint32 {
	if s == nil || s.SelectedSlot < 0 || s.SelectedSlot >= len(s.Characters) {
		return 0
	}
	return s.Characters[s.SelectedSlot].ID
}
