// Package game e o servidor de canal: o que atende o cliente depois do login.
//
// Porte do despacho de ServerSocket.pas e dos handlers de PacketHandlers.pas
// que o caminho de entrada usa.
package game

import (
	"aika/internal/auth"
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
}

// NewSession cria a sessao de uma conexao nova.
func NewSession(channel uint8) *Session {
	return &Session{
		Channel:      channel,
		Status:       WaitingLogin,
		SelectedSlot: -1,
	}
}
