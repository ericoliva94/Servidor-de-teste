package dungeon

import (
	"sync"
	"time"
)

// Lobby e a tela de confirmacao antes de entrar: a origem enviava $145 com a
// lista do grupo e esperava um $355 de cada membro.
//
// Dois problemas da origem ficaram de fora:
//
//   - A confirmacao era creditada a "Packet.Header.Index", um campo que vem
//     do cliente (PacketHandlers.pas:9699). Quem enviasse um $355 com o
//     indice de outro membro confirmava POR ele; mandando um por membro, um
//     jogador sozinho enchia o vetor e arrastava o grupo inteiro para dentro
//     da dungeon sem ninguem mais ter aceitado. Aqui a confirmacao e sempre
//     de quem mandou o pacote.
//
//   - "SetLength(Party.DungeonLobbyConfirm, Members.Count)" rodava a cada
//     confirmacao: um membro que saisse do grupo no meio encurtava o vetor e
//     descartava confirmacoes ja dadas, e o vetor vivia no registro do grupo,
//     entao um saguao novo herdava as confirmacoes do anterior. Aqui o saguao
//     e um objeto proprio, com prazo.
type Lobby struct {
	mu sync.Mutex

	// DungeonID e Dificult sao o que o lider escolheu.
	DungeonID uint32
	Dificult  uint32

	// PartyIndex e 0 quando o jogador vai sozinho.
	PartyIndex uint16
	// LeaderCharIndex e quem abriu o saguao.
	LeaderCharIndex uint32

	// esperados sao os CharIndex que precisam confirmar.
	esperados []uint32
	// confirmados guarda quem ja confirmou.
	confirmados map[uint32]bool

	CreatedAt time.Time
	cancelado bool
	// recusou guarda quem recusou, para a mensagem.
	recusou uint32
}

// NewLobby abre o saguao para os membros dados.
func NewLobby(dungeonID, dificult uint32, partyIndex uint16,
	lider uint32, membros []uint32, agora time.Time) *Lobby {
	return &Lobby{
		DungeonID:       dungeonID,
		Dificult:        dificult,
		PartyIndex:      partyIndex,
		LeaderCharIndex: lider,
		esperados:       append([]uint32(nil), membros...),
		confirmados:     map[uint32]bool{},
		CreatedAt:       agora,
	}
}

// Members devolve uma copia de quem precisa confirmar.
func (l *Lobby) Members() []uint32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]uint32, len(l.esperados))
	copy(out, l.esperados)
	return out
}

// Waits diz se o personagem esta neste saguao.
func (l *Lobby) Waits(charIndex uint32) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.esperados {
		if c == charIndex {
			return true
		}
	}
	return false
}

// Confirm registra a confirmacao de quem mandou o pacote e diz se o saguao
// esta completo. Uma confirmacao de quem nao esta no saguao e ignorada.
func (l *Lobby) Confirm(charIndex uint32) (completo bool, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancelado {
		return false, false
	}
	achou := false
	for _, c := range l.esperados {
		if c == charIndex {
			achou = true
			break
		}
	}
	if !achou {
		return false, false
	}
	l.confirmados[charIndex] = true
	return len(l.confirmados) == len(l.esperados), true
}

// Confirmed e quantos ja confirmaram.
func (l *Lobby) Confirmed() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.confirmados)
}

// Cancel marca o saguao como recusado por alguem.
func (l *Lobby) Cancel(charIndex uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cancelado = true
	l.recusou = charIndex
}

// Cancelled diz se o saguao foi recusado, e por quem.
func (l *Lobby) Cancelled() (uint32, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.recusou, l.cancelado
}

// Expired diz se o saguao passou do prazo.
func (l *Lobby) Expired(agora time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return agora.Sub(l.CreatedAt) >= LobbyTimeout
}

// Lobbies guarda os saguoes abertos, um por personagem.
type Lobbies struct {
	mu      sync.Mutex
	porChar map[uint32]*Lobby
}

// NewLobbies cria o registro vazio.
func NewLobbies() *Lobbies {
	return &Lobbies{porChar: map[uint32]*Lobby{}}
}

// Open abre o saguao e aponta todos os membros para ele, trocando um saguao
// anterior dos mesmos membros.
func (ls *Lobbies) Open(l *Lobby) {
	if l == nil {
		return
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, c := range l.esperados {
		ls.porChar[c] = l
	}
}

// Of acha o saguao de um personagem.
func (ls *Lobbies) Of(charIndex uint32) (*Lobby, bool) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	l, ok := ls.porChar[charIndex]
	return l, ok
}

// Drop tira o saguao de todos os membros.
func (ls *Lobbies) Drop(l *Lobby) {
	if l == nil {
		return
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, c := range l.Members() {
		if ls.porChar[c] == l {
			delete(ls.porChar, c)
		}
	}
}

// DropChar tira so um personagem do saguao em que ele estiver.
func (ls *Lobbies) DropChar(charIndex uint32) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	delete(ls.porChar, charIndex)
}

// Count e quantos personagens estao em algum saguao.
func (ls *Lobbies) Count() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.porChar)
}

// Expired devolve os saguoes que passaram do prazo, sem repetir.
func (ls *Lobbies) Expired(agora time.Time) []*Lobby {
	ls.mu.Lock()
	vistos := map[*Lobby]bool{}
	var out []*Lobby
	for _, l := range ls.porChar {
		if vistos[l] {
			continue
		}
		vistos[l] = true
		if l.Expired(agora) {
			out = append(out, l)
		}
	}
	ls.mu.Unlock()
	return out
}
