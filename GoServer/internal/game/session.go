// Package game e o servidor de canal: o que atende o cliente depois do login.
//
// Porte do despacho de ServerSocket.pas e dos handlers de PacketHandlers.pas
// que o caminho de entrada usa.
package game

import (
	"time"

	"aika/internal/auth"
	"aika/internal/entity"
	"aika/internal/model"
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

	Dead bool

	// OpenedNPC e OpenedOption sao o NPC e a opcao de dialogo abertos.
	OpenedNPC    uint16
	OpenedOption uint16

	// Avatar e o personagem escolhido, do jeito que os outros clientes o
	// veem. So tem conteudo a partir do Playing.
	Avatar Avatar

	// Equip e Inventory sao os itens do personagem em jogo.
	Equip     store.ItemSet
	Inventory store.ItemSet

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
		Base: EntityState{
			Buffs:     entity.NewBuffs(),
			Cooldowns: entity.NewCooldowns(),
			Targets:   entity.NewTargetList(),
		},
	}
}
