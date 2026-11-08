package game

import (
	"context"
	"errors"
	"strings"
	"sync"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos da lista de amigos.
const (
	CodeAddFriend         = 0x372 // cliente -> servidor: pedir amizade; servidor -> alvo: o pedido
	CodeAddFriendResponse = 0x673 // cliente -> servidor: aceitar ou recusar
	CodeDeleteFriend      = 0xF74 // os dois sentidos: desfazer amizade
	CodeFriendSay         = 0xF26 // os dois sentidos: conversa
	CodeOpenFriendWindow  = 0xF27 // os dois sentidos: abrir janela de conversa
	CodeCloseFriendWindow = 0xF30 // os dois sentidos: fechar janela de conversa
	CodeFriendToSocial    = 0x870 // servidor -> cliente: um amigo da lista
	CodeFriendLogin       = 0x96F // servidor -> cliente: amigo entrou
	CodeFriendLogout      = 0x971 // servidor -> cliente: amigo saiu
	CodeFriendUpdate      = 0x975 // servidor -> cliente: dados do amigo mudaram
)

// Estados de amigo, TFriendStatus de PlayerData.pas.
const (
	FriendBlockedOff uint8 = iota
	FriendBlockedOn
	FriendOffline
	FriendOnline
)

// MaxFriendWindows e quantas janelas de conversa de amigo ficam abertas.
const MaxFriendWindows = 6

// Erros da lista de amigos.
var (
	errFriendListFull = errors.New("game: lista de amigos cheia")
	errFriendKnown    = errors.New("game: amigo ja esta na lista")
)

// FriendStore guarda as amizades.
type FriendStore interface {
	List(ctx context.Context, charID uint32) ([]store.FriendRow, error)
	// Add grava a amizade nos dois sentidos de uma vez.
	Add(ctx context.Context, a, b uint32) error
	// Remove desfaz a amizade nos dois sentidos de uma vez.
	Remove(ctx context.Context, a, b uint32) error
}

// friendState e a lista de amigos de uma sessao.
//
// Substitui TPlayer.FriendList (TDictionary), FriendOpenWindowns e o objeto
// TEntityFriend. Tem trava propria porque outra sessao mexe aqui: quando
// alguem aceita ou desfaz uma amizade, a lista do outro lado muda na goroutine
// de quem agiu. A origem fazia isso sem trava nenhuma.
type friendState struct {
	mu      sync.Mutex
	names   map[uint32]string // id do personagem -> nome
	order   []uint32          // ordem de chegada, para a lista sair estavel
	windows map[uint32]uint32 // indice da janela -> id do personagem
	pending map[uint16]bool   // quem me pediu amizade e ainda nao respondi
}

type friendEntry struct {
	CharID uint32
	Name   string
}

func (f *friendState) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names = map[uint32]string{}
	f.order = nil
	f.windows = map[uint32]uint32{}
	f.pending = map[uint16]bool{}
}

func (f *friendState) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.names)
}

func (f *friendState) has(id uint32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.names[id]
	return ok
}

func (f *friendState) name(id uint32) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.names[id]
	return n, ok
}

// add poe um amigo na lista; recusa quando cheia ou repetido.
func (f *friendState) add(id uint32, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.names == nil {
		f.names = map[uint32]string{}
	}
	if _, ok := f.names[id]; ok {
		return errFriendKnown
	}
	if len(f.names) >= store.MaxFriends {
		return errFriendListFull
	}
	f.names[id] = name
	f.order = append(f.order, id)
	return nil
}

// remove tira o amigo e fecha a janela de conversa dele. Devolve o indice da
// janela que estava aberta.
func (f *friendState) remove(id uint32) (window uint32, hadWindow bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.names, id)
	for i, v := range f.order {
		if v == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	for idx, v := range f.windows {
		if v == id {
			delete(f.windows, idx)
			return idx, true
		}
	}
	return 0, false
}

func (f *friendState) snapshot() []friendEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]friendEntry, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, friendEntry{CharID: id, Name: f.names[id]})
	}
	return out
}

// windowOf devolve o indice da janela aberta para o personagem.
func (f *friendState) windowOf(id uint32) (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for idx, v := range f.windows {
		if v == id {
			return idx, true
		}
	}
	return 0, false
}

func (f *friendState) windowChar(idx uint32) (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.windows[idx]
	return v, ok
}

// openWindow registra a janela. Recusa indice repetido e janelas demais; a
// origem deixava o TDictionary.Add levantar excecao no indice repetido.
func (f *friendState) openWindow(idx, id uint32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.windows == nil {
		f.windows = map[uint32]uint32{}
	}
	if _, ok := f.windows[idx]; ok || len(f.windows) >= MaxFriendWindows {
		return false
	}
	f.windows[idx] = id
	return true
}

// openFreeWindow abre a janela no primeiro indice livre de 0 a 5.
func (f *friendState) openFreeWindow(id uint32) (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.windows == nil {
		f.windows = map[uint32]uint32{}
	}
	if len(f.windows) >= MaxFriendWindows {
		return 0, false
	}
	for i := uint32(0); i < MaxFriendWindows; i++ {
		if _, ok := f.windows[i]; !ok {
			f.windows[i] = id
			return i, true
		}
	}
	return 0, false
}

func (f *friendState) closeWindow(idx uint32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.windows[idx]; !ok {
		return false
	}
	delete(f.windows, idx)
	return true
}

func (f *friendState) addPending(from uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending == nil {
		f.pending = map[uint16]bool{}
	}
	f.pending[from] = true
}

// takePending consome o pedido: ele vale uma resposta so.
func (f *friendState) takePending(from uint16) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pending[from] {
		return false
	}
	delete(f.pending, from)
	return true
}

// friendLevel e "Level - 1" do pacote, com piso em zero: a origem fazia a
// conta em BYTE e nivel 0 virava 255.
func friendLevel(s *Session) uint8 {
	if s.Avatar.Level <= 1 {
		return 0
	}
	if l := s.Avatar.Level - 1; l < 255 {
		return uint8(l)
	}
	return 255
}

// onlineFriend acha a sessao em jogo de um amigo. O nome vem da lista; o id
// do personagem confere para um nome reaproveitado nao enganar.
func (h *Handler) onlineFriend(name string, charID uint32) (*Session, *netsrv.Conn, bool) {
	if h.Chan == nil {
		return nil, nil, false
	}
	id, ok := h.Chan.FindByCharacter(name)
	if !ok {
		return nil, nil, false
	}
	s, c, ok := h.Chan.Lookup(id)
	if !ok || s.Status < Playing || s.CharID() != charID {
		return nil, nil, false
	}
	return s, c, true
}

// onlineFriends lista as sessoes em jogo dos amigos de s.
func (h *Handler) onlineFriends(s *Session) []*onlinePeer {
	var out []*onlinePeer
	for _, f := range s.Friends.snapshot() {
		if o, c, ok := h.onlineFriend(f.Name, f.CharID); ok {
			out = append(out, &onlinePeer{s: o, c: c})
		}
	}
	return out
}

type onlinePeer struct {
	s *Session
	c *netsrv.Conn
}

// loadFriends le a lista do banco ao entrar no mundo e avisa os amigos.
//
// Porte de readFriendList seguido de RefreshSocialFriends, SendFriendLogin e
// RefreshMeToFriends (Player.pas:6725). Falha do banco deixa a lista vazia e
// registra: o jogador entra sem amigos em vez de nao entrar.
func (h *Handler) loadFriends(ctx context.Context, c *netsrv.Conn, s *Session) {
	s.Friends.reset()
	if h.FriendDB != nil {
		if id := s.CharID(); id != 0 {
			rows, err := h.FriendDB.List(ctx, id)
			if err != nil {
				h.log().Error("carregar lista de amigos",
					"personagem", s.Avatar.Name, "erro", err)
			}
			for _, r := range rows {
				if err := s.Friends.add(r.CharID, r.Name); err != nil {
					h.log().Warn("amigo ignorado na carga",
						"personagem", s.Avatar.Name, "amigo", r.Name, "erro", err)
				}
			}
		}
	}
	for _, f := range s.Friends.snapshot() {
		h.sendFriendToSocial(c, s, f.CharID)
	}
	h.broadcastFriendLogin(s)
	h.broadcastFriendUpdate(s)
}

// friendsOnDisconnect avisa os amigos de que o jogador saiu (SendFriendLogout).
// Fica no ponto unico de saida; a origem dependia de cada caminho de saida
// lembrar de chamar.
func (h *Handler) friendsOnDisconnect(s *Session) {
	id := s.CharID()
	if id == 0 {
		return
	}
	// O Header.Index do $971 na origem era lixo de pilha: o record nao era
	// zerado. Aqui vai zero.
	pkt := packets.FriendLogoutPacket{CharIndex: id}
	for _, p := range h.onlineFriends(s) {
		_ = p.c.SendStruct(CodeFriendLogout, pkt)
	}
}

// sendFriendToSocial manda um amigo da lista ao cliente ($870).
func (h *Handler) sendFriendToSocial(c *netsrv.Conn, s *Session, charID uint32) {
	name, ok := s.Friends.name(charID)
	if !ok {
		return
	}
	pkt := packets.SendFriendToSocialPacket{CharIndex: charID, FriendStatus: FriendOffline}
	pkt.Header.Index = s.ClientID
	netsrv.PutCString(pkt.Nick[:], name)
	if o, _, on := h.onlineFriend(name, charID); on {
		pkt.FriendStatus = FriendOnline
		pkt.Channel = o.Channel
		pkt.PlayerIndex = o.ClientID
		pkt.Classe = uint8(o.Avatar.ClassInfo)
		pkt.Level = friendLevel(o)
		// City: a cidade sai de GetCurrentCityID, que depende da tabela de
		// mapas e ainda nao esta portada. Vai zero; ver KNOWN_ISSUES.md.
	}
	_ = c.SendStruct(CodeFriendToSocial, pkt)
}

// broadcastFriendLogin e SendFriendLogin ($96F).
func (h *Handler) broadcastFriendLogin(s *Session) {
	pkt := packets.FriendLoginPacket{
		CharIndex:    s.CharID(),
		PlayerIndex:  s.ClientID,
		FriendStatus: FriendOnline,
		Channel:      s.Channel,
		Nation:       s.Account.Nation,
		Level:        friendLevel(s),
		Classe:       uint8(s.Avatar.ClassInfo),
	}
	for _, p := range h.onlineFriends(s) {
		_ = p.c.SendStruct(CodeFriendLogin, pkt)
	}
}

// broadcastFriendUpdate e RefreshMeToFriends ($975).
func (h *Handler) broadcastFriendUpdate(s *Session) {
	pkt := packets.AtualizeFriendInfosPacket{
		CharIndex:    s.CharID(),
		PlayerIndex:  s.ClientID,
		FriendStatus: FriendOnline,
		Channel:      s.Channel,
		Level:        friendLevel(s),
		Classe:       uint8(s.Avatar.ClassInfo),
	}
	for _, p := range h.onlineFriends(s) {
		_ = p.c.SendStruct(CodeFriendUpdate, pkt)
	}
}

// addFriend e o porte de TPacketHandlers.AddFriendRequest ($372).
func (h *Handler) addFriend(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil {
		return nil
	}
	var pkt packets.AddFriendRequestPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	nick := strings.TrimSpace(netsrv.CString(pkt.Nick[:]))
	if nick == "" {
		return nil
	}
	id, found := h.Chan.FindByCharacter(nick)
	var other *Session
	var otherConn *netsrv.Conn
	if found {
		var ok bool
		other, otherConn, ok = h.Chan.Lookup(id)
		found = ok && other.Status >= Playing
	}
	if !found {
		h.message(c, "Jogador nao encontrado.")
		return nil
	}
	if other == s {
		h.message(c, "Voce nao pode se adicionar como amigo.")
		return nil
	}
	if s.Friends.count() >= store.MaxFriends {
		h.message(c, "Lista de amigos cheia.")
		return nil
	}
	if s.Friends.has(other.CharID()) {
		h.message(c, "Jogador ja adicionado.")
		return nil
	}
	if other.Friends.count() >= store.MaxFriends {
		h.message(c, "Lista de amigos do usuario cheia.")
		return nil
	}
	if s.Avatar.Nation != 0 && other.Avatar.Nation != 0 && s.Avatar.Nation != other.Avatar.Nation {
		h.message(c, "Nao e possivel adicionar jogadores de outras nacoes.")
		return nil
	}

	// O pedido fica guardado no convidado, que responde com o $673. Sem isso
	// qualquer cliente mandava o $673 com o id de outro jogador e virava
	// amigo dele sem pedido nem aceite.
	other.Friends.addPending(s.ClientID)

	var fwd packets.AddFriendRequestPacket
	fwd.Header.Index = s.ClientID
	fwd.ID = uint32(s.ClientID)
	netsrv.PutCString(fwd.Nick[:], s.Avatar.Name)
	if err := otherConn.SendStruct(CodeAddFriend, fwd); err != nil {
		h.log().Debug("falha ao enviar pedido de amizade", "erro", err)
	}
	return nil
}

// addFriendResponse e o porte de TPacketHandlers.AddFriendResponse ($673).
func (h *Handler) addFriendResponse(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	if h.Chan == nil {
		return nil
	}
	var pkt packets.AddFriendResponsePacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if !s.Friends.takePending(pkt.ID) {
		h.log().Debug("resposta de amizade sem pedido",
			"jogador", s.Avatar.Name, "id", pkt.ID)
		return nil
	}
	other, otherConn, ok := h.Chan.Lookup(pkt.ID)
	if !ok || other.Status < Playing {
		h.message(c, "Jogador nao encontrado.")
		return nil
	}
	if pkt.Response == 0 {
		h.message(otherConn, "Pedido de amizade recusado.")
		return nil
	}

	switch h.makeFriends(ctx, s, other) {
	case friendOK:
		h.message(c, "Voce aceitou amizade de "+other.Avatar.Name)
		h.message(otherConn, "Jogador "+s.Avatar.Name+" aceitou seu pedido de amizade.")
	case friendOffline:
		h.message(c, "Personagem offline ou nao existe.")
	case friendMineFull:
		h.message(c, "Lista de amigos cheia.")
	case friendTheirsFull:
		h.message(c, "Lista de amigos do usuario cheia.")
	case friendSelf:
		h.message(c, "Voce nao pode se adicionar como amigo.")
	case friendKnown:
		h.message(c, "Jogador ja adicionado.")
	default:
		h.message(c, "Nao foi possivel adicionar o amigo.")
	}
	return nil
}

type friendResult int

const (
	friendOK friendResult = iota
	friendOffline
	friendMineFull
	friendTheirsFull
	friendSelf
	friendKnown
	friendFailed
)

// makeFriends e TPlayer.AddFriend: grava a amizade e poe cada um na lista do
// outro. O banco vem primeiro, para uma falha nao deixar so a memoria mudada.
func (h *Handler) makeFriends(ctx context.Context, a, b *Session) friendResult {
	if b.Status < Playing {
		return friendOffline
	}
	if a == b || a.CharID() == b.CharID() {
		return friendSelf
	}
	if a.Friends.has(b.CharID()) {
		return friendKnown
	}
	if a.Friends.count() >= store.MaxFriends {
		return friendMineFull
	}
	if b.Friends.count() >= store.MaxFriends {
		return friendTheirsFull
	}
	if h.FriendDB != nil {
		if err := h.FriendDB.Add(ctx, a.CharID(), b.CharID()); err != nil {
			h.log().Error("gravar amizade",
				"a", a.Avatar.Name, "b", b.Avatar.Name, "erro", err)
			return friendFailed
		}
	}
	if err := a.Friends.add(b.CharID(), b.Avatar.Name); err != nil {
		return friendMineFull
	}
	if err := b.Friends.add(a.CharID(), a.Avatar.Name); err != nil {
		a.Friends.remove(b.CharID())
		return friendTheirsFull
	}
	if _, ac, ok := h.Chan.Lookup(a.ClientID); ok {
		h.sendFriendToSocial(ac, a, b.CharID())
	}
	if _, bc, ok := h.Chan.Lookup(b.ClientID); ok {
		h.sendFriendToSocial(bc, b, a.CharID())
	}
	return friendOK
}

// deleteFriend e o porte de TPacketHandlers.DeleteFriend ($F74) e de
// TEntityFriend.removeFriend.
func (h *Handler) deleteFriend(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.DeleteFriendPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	if pkt.CharIndex == 0 {
		return nil
	}
	name, ok := s.Friends.name(pkt.CharIndex)
	if !ok {
		// A origem desativava as duas linhas e mandava o $F74 mesmo assim.
		return nil
	}
	if h.FriendDB != nil {
		if err := h.FriendDB.Remove(ctx, s.CharID(), pkt.CharIndex); err != nil {
			h.log().Error("desfazer amizade",
				"jogador", s.Avatar.Name, "amigo", name, "erro", err)
			h.message(c, "Nao foi possivel remover o amigo.")
			return nil
		}
	}

	h.sendDeleteFriend(c, s, pkt.CharIndex)
	if idx, had := s.Friends.remove(pkt.CharIndex); had {
		h.sendCloseFriendWindow(c, s, idx)
	}

	if o, oc, on := h.onlineFriend(name, pkt.CharIndex); on {
		h.sendDeleteFriend(oc, o, s.CharID())
		if idx, had := o.Friends.remove(s.CharID()); had {
			h.sendCloseFriendWindow(oc, o, idx)
		}
	}
	return nil
}

func (h *Handler) sendDeleteFriend(c *netsrv.Conn, to *Session, charID uint32) {
	pkt := packets.DeleteFriendPacket{CharIndex: charID}
	pkt.Header.Index = to.ClientID
	_ = c.SendStruct(CodeDeleteFriend, pkt)
}

func (h *Handler) sendCloseFriendWindow(c *netsrv.Conn, to *Session, window uint32) {
	pkt := packets.CloseFriendWindowPacket{WindowIndex: window}
	pkt.Header.Index = to.ClientID
	_ = c.SendStruct(CodeCloseFriendWindow, pkt)
}

func (h *Handler) sendOpenFriendWindow(c *netsrv.Conn, to *Session, charID, window uint32) {
	pkt := packets.OpenFriendWindowPacket{CharIndex: charID, WindowIndex: window}
	pkt.Header.Index = to.ClientID
	_ = c.SendStruct(CodeOpenFriendWindow, pkt)
}

// openFriendWindow e o porte de TPacketHandlers.OpenFriendWindow ($F27).
//
// A origem so conferia se o personagem estava online, nao se era amigo: com
// a janela aberta o $F26 entregava mensagem a qualquer jogador pelo id.
func (h *Handler) openFriendWindow(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.OpenFriendWindowPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	name, ok := s.Friends.name(pkt.CharIndex)
	if !ok {
		return nil
	}
	if _, _, on := h.onlineFriend(name, pkt.CharIndex); !on {
		return nil
	}
	if !s.Friends.openWindow(pkt.WindowIndex, pkt.CharIndex) {
		return nil
	}
	h.sendOpenFriendWindow(c, s, pkt.CharIndex, pkt.WindowIndex)
	return nil
}

// closeFriendWindow e o porte de TPacketHandlers.CloseFriendWindow ($F30):
// so esquece a janela, sem resposta.
func (h *Handler) closeFriendWindow(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.CloseFriendWindowPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	s.Friends.closeWindow(pkt.WindowIndex)
	return nil
}

// friendSay e o porte de TPacketHandlers.SendFriendSay ($F26).
func (h *Handler) friendSay(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if s.Status < Playing {
		return ErrNotPlaying
	}
	var pkt packets.SendFriendChatPacket
	if err := netsrv.Unmarshal(p, &pkt); err != nil {
		return err
	}
	friendID, ok := s.Friends.windowChar(pkt.WindowIndex)
	if !ok {
		return nil
	}
	name, ok := s.Friends.name(friendID)
	if !ok {
		return nil
	}
	other, otherConn, on := h.onlineFriend(name, friendID)
	if !on {
		h.message(c, "O jogador esta offline.")
		return nil
	}

	// A janela do outro lado: reaproveita a que ja mostra este jogador, ou
	// abre em um indice livre (e avisa o cliente dele).
	otherWin, has := other.Friends.windowOf(s.CharID())
	if !has {
		idx, opened := other.Friends.openFreeWindow(s.CharID())
		if !opened {
			return nil
		}
		otherWin = idx
		h.sendOpenFriendWindow(otherConn, other, s.CharID(), otherWin)
	}

	// Fala e apelido vem do cliente. O texto e relido com limite e regravado
	// com terminador (a origem copiava os 128 bytes sem olhar), e o apelido
	// que o outro lado ve e o do autor, nao o que o pacote trouxer.
	fala := netsrv.CString(pkt.Fala[:])
	netsrv.PutCString(pkt.Fala[:], fala)

	pkt.Header.Index = s.ClientID
	_ = c.SendStruct(CodeFriendSay, pkt)

	out := pkt
	out.Header.Index = other.ClientID
	out.WindowIndex = otherWin
	netsrv.PutCString(out.Nick[:], s.Avatar.Name)
	_ = otherConn.SendStruct(CodeFriendSay, out)
	return nil
}
