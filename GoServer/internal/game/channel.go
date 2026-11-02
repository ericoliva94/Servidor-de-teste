package game

import (
	"errors"
	"strings"
	"sync"

	"aika/internal/entity"
	"aika/internal/netsrv"
)

// ErrChannelFull indica que nao ha ClientID livre.
var ErrChannelFull = errors.New("game: canal cheio")

// Channel guarda quem esta conectado a um canal.
//
// Substitui o "Servers[n].Players: ARRAY [1..1000] OF TPlayer" do original.
// Ali o indice do vetor era o proprio ClientID, e cada acesso precisava
// acertar a faixa: um id 0 ou acima do limite lia e escrevia fora do vetor, o
// que aparece em MOB.pas, PET.pas, NPC.pas, BaseMob.pas e Dungeon.pas sempre
// com o mesmo teste incompleto "if (i <= MAX_CONNECTIONS)".
//
// Aqui o acesso e por mapa e id invalido simplesmente nao encontra ninguem.
// Seguro para uso concorrente: varias goroutines de conexao mexem nisso.
type Channel struct {
	id       uint8
	maxConns uint16

	mu       sync.RWMutex
	sessions map[uint16]*member
	byUser   map[string]uint16
	nextHint uint16
}

type member struct {
	session *Session
	conn    *netsrv.Conn
}

// NewChannel cria o canal. maxConns e limitado ao tamanho do vetor original.
func NewChannel(id uint8, maxConns uint16) *Channel {
	if maxConns == 0 || maxConns > entity.MaxPlayerIDHardLimit {
		maxConns = entity.MaxPlayerIDHardLimit
	}
	return &Channel{
		id:       id,
		maxConns: maxConns,
		sessions: make(map[uint16]*member, 64),
		byUser:   make(map[string]uint16, 64),
		nextHint: entity.MinPlayerID,
	}
}

// ID devolve o numero do canal.
func (ch *Channel) ID() uint8 { return ch.id }

// Count conta as sessoes registradas.
func (ch *Channel) Count() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.sessions)
}

// Join registra a sessao e devolve o ClientID alocado.
//
// Os ids vao de 1 a maxConns, nunca zero: zero e o valor de "sem id" em todo
// o protocolo, e era justamente ele que fazia Players[0] cair antes do inicio
// do vetor.
func (ch *Channel) Join(c *netsrv.Conn, s *Session) (uint16, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	id, ok := ch.freeIDLocked()
	if !ok {
		return 0, ErrChannelFull
	}
	s.ClientID = id
	s.Channel = ch.id
	ch.sessions[id] = &member{session: s, conn: c}
	if c != nil {
		// A conexao passa a apontar para a sessao: e por aqui que o fim da
		// conexao (netsrv.DisconnectHandler) acha quem sair do canal.
		c.Session = s
	}
	if u := normalizeUser(s.Username); u != "" {
		ch.byUser[u] = id
	}
	return id, nil
}

// freeIDLocked acha o primeiro id livre a partir da ultima alocacao, dando a
// volta. Reaproveita id liberado sem varrer tudo a cada entrada.
func (ch *Channel) freeIDLocked() (uint16, bool) {
	if len(ch.sessions) >= int(ch.maxConns) {
		return 0, false
	}
	start := ch.nextHint
	if start < entity.MinPlayerID || start > ch.maxConns {
		start = entity.MinPlayerID
	}
	for i := uint16(0); i < ch.maxConns; i++ {
		id := start + i
		if id > ch.maxConns {
			id -= ch.maxConns // volta para o inicio da faixa
			if id < entity.MinPlayerID {
				id = entity.MinPlayerID
			}
		}
		if _, usado := ch.sessions[id]; !usado {
			ch.nextHint = id + 1
			return id, true
		}
	}
	return 0, false
}

// Leave remove a sessao.
func (ch *Channel) Leave(clientID uint16) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	m, ok := ch.sessions[clientID]
	if !ok {
		return
	}
	delete(ch.sessions, clientID)
	if u := normalizeUser(m.session.Username); u != "" {
		// So apaga se ainda aponta para este id: um login novo do mesmo
		// usuario pode ter assumido a entrada.
		if cur, ok := ch.byUser[u]; ok && cur == clientID {
			delete(ch.byUser, u)
		}
	}
}

// Lookup devolve a sessao e a conexao de um ClientID.
func (ch *Channel) Lookup(clientID uint16) (*Session, *netsrv.Conn, bool) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	m, ok := ch.sessions[clientID]
	if !ok {
		return nil, nil, false
	}
	return m.session, m.conn, true
}

// FindByUsername acha o ClientID de um usuario conectado.
//
// Porte de GetPlayerByUsername, que no original varria o vetor de 1000
// posicoes a cada chamada.
func (ch *Channel) FindByUsername(username string) (uint16, bool) {
	u := normalizeUser(username)
	if u == "" {
		return 0, false
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	id, ok := ch.byUser[u]
	return id, ok
}

// PlayingIDs lista quem esta em jogo.
func (ch *Channel) PlayingIDs() []uint16 {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	out := make([]uint16, 0, len(ch.sessions))
	for id, m := range ch.sessions {
		if m.session.Status >= Playing {
			out = append(out, id)
		}
	}
	return out
}

// SendTo envia um pacote ja cifrado a um ClientID, se ele estiver em jogo.
func (ch *Channel) SendTo(clientID uint16, packet []byte) error {
	ch.mu.RLock()
	m, ok := ch.sessions[clientID]
	ch.mu.RUnlock()
	if !ok || m.session.Status < Playing {
		return nil
	}
	return m.conn.SendRaw(packet)
}

// Broadcast envia o pacote a uma lista de ClientIDs.
//
// Porte de TBaseMob.SendToVisible. Erro de envio a um destinatario nao
// interrompe os outros: no original o try/except dentro do laco fazia
// Continue, e a intencao e a mesma — um cliente com o socket morto nao pode
// impedir os demais de receber.
func (ch *Channel) Broadcast(ids []uint16, packet []byte) {
	for _, id := range ids {
		_ = ch.SendTo(id, packet)
	}
}

// BroadcastExcept envia a todos os que estao em jogo, menos um.
func (ch *Channel) BroadcastExcept(except uint16, packet []byte) {
	ch.mu.RLock()
	destinos := make([]uint16, 0, len(ch.sessions))
	for id, m := range ch.sessions {
		if id != except && m.session.Status >= Playing {
			destinos = append(destinos, id)
		}
	}
	ch.mu.RUnlock()
	for _, id := range destinos {
		_ = ch.SendTo(id, packet)
	}
}

func normalizeUser(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
