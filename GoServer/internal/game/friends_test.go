package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// ---- infraestrutura de teste

// fakeFriends e uma FriendStore em memoria.
type fakeFriends struct {
	mu                    sync.Mutex
	pares                 map[[2]uint32]bool
	nomes                 map[uint32]string
	falhaAdd, falhaRemove error
	adds, removes         int
}

func novoFakeFriends() *fakeFriends {
	return &fakeFriends{pares: map[[2]uint32]bool{}, nomes: map[uint32]string{}}
}

func (f *fakeFriends) List(_ context.Context, id uint32) ([]store.FriendRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.FriendRow
	for par, ativo := range f.pares {
		if ativo && par[0] == id {
			out = append(out, store.FriendRow{CharID: par[1], Name: f.nomes[par[1]]})
		}
	}
	return out, nil
}

func (f *fakeFriends) Add(_ context.Context, a, b uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falhaAdd != nil {
		return f.falhaAdd
	}
	f.adds++
	f.pares[[2]uint32{a, b}] = true
	f.pares[[2]uint32{b, a}] = true
	return nil
}

func (f *fakeFriends) Remove(_ context.Context, a, b uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falhaRemove != nil {
		return f.falhaRemove
	}
	f.removes++
	f.pares[[2]uint32{a, b}] = false
	f.pares[[2]uint32{b, a}] = false
	return nil
}

func (f *fakeFriends) ativo(a, b uint32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pares[[2]uint32{a, b}] && f.pares[[2]uint32{b, a}]
}

func handlerComAmigos(t *testing.T) (*Handler, *fakeFriends) {
	t.Helper()
	h := testHandler()
	h.Chan = NewChannel(0, 60)
	f := novoFakeFriends()
	h.FriendDB = f
	return h, f
}

// jogador poe alguem em jogo, ja com a lista de amigos inicializada.
func jogador(t *testing.T, h *Handler, f *fakeFriends, nome string, charID uint32) (*Session, *espiao) {
	t.Helper()
	s, e := entraComPersonagem(t, h, nome, charID)
	s.Avatar.Nation = 1
	s.Friends.reset()
	f.mu.Lock()
	f.nomes[charID] = nome
	f.mu.Unlock()
	return s, e
}

func friendPacket[T any](code uint16, v T) []byte {
	buf, err := netsrv.Marshal(code, v)
	if err != nil {
		panic(err)
	}
	return buf
}

func decodifica[T any](t *testing.T, p pacoteVisto) T {
	t.Helper()
	var v T
	n := binary.Size(v)
	if n > len(p.corpo) {
		t.Fatalf("pacote %#x tem %d bytes, %T precisa de %d", p.code, len(p.corpo), v, n)
	}
	if err := binary.Read(bytes.NewReader(p.corpo[:n]), binary.LittleEndian, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// proximoDe devolve o proximo pacote do codigo pedido, pulando os outros.
func proximoDe(t *testing.T, e *espiao, code uint16) pacoteVisto {
	t.Helper()
	for i := 0; i < 20; i++ {
		p := e.proximo(t)
		if p.code == code {
			return p
		}
	}
	t.Fatalf("pacote %#x nunca chegou", code)
	return pacoteVisto{}
}

func mensagemDe(t *testing.T, e *espiao) string {
	t.Helper()
	m := decodifica[clientMessage](t, proximoDe(t, e, CodeClientMsg))
	return netsrv.CString(m.Msg[:])
}

func pedeAmizade(t *testing.T, h *Handler, de *Session, deE *espiao, nome string) {
	t.Helper()
	var p packets.AddFriendRequestPacket
	netsrv.PutCString(p.Nick[:], nome)
	if err := h.addFriend(context.Background(), deE.conn, de,
		asPacket(t, friendPacket(CodeAddFriend, p))); err != nil {
		t.Fatal(err)
	}
}

func respondeAmizade(t *testing.T, h *Handler, quem *Session, quemE *espiao, id uint16, resp uint16) {
	t.Helper()
	p := packets.AddFriendResponsePacket{Response: resp, ID: id}
	if err := h.addFriendResponse(context.Background(), quemE.conn, quem,
		asPacket(t, friendPacket(CodeAddFriendResponse, p))); err != nil {
		t.Fatal(err)
	}
}

// fazAmigos leva dois jogadores ate a amizade pelo caminho normal.
func fazAmigos(t *testing.T, h *Handler, a *Session, ae *espiao, b *Session, be *espiao) {
	t.Helper()
	pedeAmizade(t, h, a, ae, b.Avatar.Name)
	respondeAmizade(t, h, b, be, a.ClientID, 1)
	ae.drena(20 * time.Millisecond)
	be.drena(20 * time.Millisecond)
}

// ---- pedido e resposta

func TestPedidoDeAmizadeChegaAoAlvoComOIdEONomeDoAutor(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	_, be := jogador(t, h, f, "Beta", 2)

	pedeAmizade(t, h, a, ae, "Beta")

	req := decodifica[packets.AddFriendRequestPacket](t, proximoDe(t, be, CodeAddFriend))
	if req.ID != uint32(a.ClientID) {
		t.Errorf("ID = %d, queria %d", req.ID, a.ClientID)
	}
	if got := netsrv.CString(req.Nick[:]); got != "Alfa" {
		t.Errorf("Nick = %q, queria Alfa", got)
	}
	if req.Header.Index != a.ClientID {
		t.Errorf("Header.Index = %d, queria %d", req.Header.Index, a.ClientID)
	}
}

func TestAceitarFazAmigosNosDoisLadosEGravaUmaVez(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)

	pedeAmizade(t, h, a, ae, "Beta")
	be.drena(20 * time.Millisecond)
	respondeAmizade(t, h, b, be, a.ClientID, 1)

	if !a.Friends.has(2) || !b.Friends.has(1) {
		t.Fatal("a amizade nao entrou nas duas listas")
	}
	if !f.ativo(1, 2) || f.adds != 1 {
		t.Errorf("banco: ativo=%v adds=%d, queria ativo e 1 gravacao", f.ativo(1, 2), f.adds)
	}

	// Cada lado recebe o amigo novo no $870, online, com o id do outro.
	sa := decodifica[packets.SendFriendToSocialPacket](t, proximoDe(t, ae, CodeFriendToSocial))
	if sa.CharIndex != 2 || sa.FriendStatus != FriendOnline || sa.PlayerIndex != b.ClientID {
		t.Errorf("$870 de Alfa = %+v", sa)
	}
	if got := netsrv.CString(sa.Nick[:]); got != "Beta" {
		t.Errorf("Nick = %q, queria Beta", got)
	}
	sb := decodifica[packets.SendFriendToSocialPacket](t, proximoDe(t, be, CodeFriendToSocial))
	if sb.CharIndex != 1 || sb.PlayerIndex != a.ClientID {
		t.Errorf("$870 de Beta = %+v", sb)
	}
}

func TestMensagensDeAceite(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)

	pedeAmizade(t, h, a, ae, "Beta")
	respondeAmizade(t, h, b, be, a.ClientID, 1)

	if got := mensagemDe(t, be); got != "Voce aceitou amizade de Alfa" {
		t.Errorf("mensagem de Beta = %q", got)
	}
	if got := mensagemDe(t, ae); got != "Jogador Beta aceitou seu pedido de amizade." {
		t.Errorf("mensagem de Alfa = %q", got)
	}
}

// Defeito da origem: o $673 nao conferia se havia pedido. Qualquer cliente
// mandava o pacote com o id de outro jogador e virava amigo dele sem aceite.
func TestRespostaSemPedidoNaoFazAmizade(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, _ := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)

	// Beta forja a resposta ao "pedido" de Alfa, que nunca pediu nada.
	respondeAmizade(t, h, b, be, a.ClientID, 1)

	if a.Friends.has(2) || b.Friends.has(1) {
		t.Fatal("amizade criada sem pedido")
	}
	if f.adds != 0 {
		t.Errorf("%d gravacoes no banco, queria 0", f.adds)
	}
}

func TestPedidoValeUmaRespostaSo(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)

	pedeAmizade(t, h, a, ae, "Beta")
	respondeAmizade(t, h, b, be, a.ClientID, 1)
	if f.adds != 1 {
		t.Fatalf("adds = %d", f.adds)
	}
	// Desfaz e repete a mesma resposta sem novo pedido.
	f.mu.Lock()
	f.pares = map[[2]uint32]bool{}
	f.mu.Unlock()
	b.Friends.remove(1)
	a.Friends.remove(2)
	respondeAmizade(t, h, b, be, a.ClientID, 1)
	if b.Friends.has(1) || f.adds != 1 {
		t.Error("o mesmo pedido serviu duas vezes")
	}
}

func TestRecusarAvisaQuemPediuENaoGrava(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)

	pedeAmizade(t, h, a, ae, "Beta")
	respondeAmizade(t, h, b, be, a.ClientID, 0)

	if got := mensagemDe(t, ae); got != "Pedido de amizade recusado." {
		t.Errorf("mensagem = %q", got)
	}
	if a.Friends.has(2) || f.adds != 0 {
		t.Error("recusa nao pode criar amizade")
	}
}

func TestPedidosRecusados(t *testing.T) {
	cheio := func(s *Session) {
		for i := uint32(100); i < 100+store.MaxFriends; i++ {
			if err := s.Friends.add(i, "x"); err != nil {
				panic(err)
			}
		}
	}
	casos := []struct {
		nome  string
		alvo  string
		setup func(a, b *Session)
		msg   string
	}{
		{"nome inexistente", "Fantasma", func(a, b *Session) {}, "Jogador nao encontrado."},
		{"a si mesmo", "Alfa", func(a, b *Session) {}, "Voce nao pode se adicionar como amigo."},
		{"ja e amigo", "Beta", func(a, b *Session) { a.Friends.add(2, "Beta") }, "Jogador ja adicionado."},
		{"minha lista cheia", "Beta", func(a, b *Session) { cheio(a) }, "Lista de amigos cheia."},
		{"lista dele cheia", "Beta", func(a, b *Session) { cheio(b) }, "Lista de amigos do usuario cheia."},
		{"nacoes diferentes", "Beta", func(a, b *Session) { b.Avatar.Nation = 2 },
			"Nao e possivel adicionar jogadores de outras nacoes."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, f := handlerComAmigos(t)
			a, ae := jogador(t, h, f, "Alfa", 1)
			b, be := jogador(t, h, f, "Beta", 2)
			c.setup(a, b)

			pedeAmizade(t, h, a, ae, c.alvo)

			if got := mensagemDe(t, ae); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			be.nada(t, 30*time.Millisecond)
		})
	}
}

// Nacao zero (sem nacao) pode ser amiga de qualquer uma, como na origem.
func TestSemNacaoPodeAdicionarQualquerNacao(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	a.Avatar.Nation = 0
	b.Avatar.Nation = 2

	pedeAmizade(t, h, a, ae, "Beta")
	proximoDe(t, be, CodeAddFriend)
}

func TestFalhaDoBancoNaoDeixaAmizadeSoNaMemoria(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	f.falhaAdd = errors.New("mysql caiu")

	pedeAmizade(t, h, a, ae, "Beta")
	respondeAmizade(t, h, b, be, a.ClientID, 1)

	if a.Friends.has(2) || b.Friends.has(1) {
		t.Fatal("amizade ficou na memoria apesar da falha do banco")
	}
	if got := mensagemDe(t, be); got != "Nao foi possivel adicionar o amigo." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestSessaoForaDoJogoRecusaPedidos(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	a.Status = CharList
	var p packets.AddFriendRequestPacket
	netsrv.PutCString(p.Nick[:], "Beta")
	err := h.addFriend(context.Background(), ae.conn, a, asPacket(t, friendPacket(CodeAddFriend, p)))
	if !errors.Is(err, ErrNotPlaying) {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

// ---- remover

func TestRemoverAmigoLimpaOsDoisLadosEAvisaOsDois(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)

	p := packets.DeleteFriendPacket{CharIndex: 2}
	if err := h.deleteFriend(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeDeleteFriend, p))); err != nil {
		t.Fatal(err)
	}

	if a.Friends.has(2) || b.Friends.has(1) {
		t.Fatal("amigo continua na lista")
	}
	if f.ativo(1, 2) || f.removes != 1 {
		t.Errorf("banco: ativo=%v removes=%d", f.ativo(1, 2), f.removes)
	}
	da := decodifica[packets.DeleteFriendPacket](t, proximoDe(t, ae, CodeDeleteFriend))
	if da.CharIndex != 2 {
		t.Errorf("Alfa recebeu CharIndex %d, queria 2", da.CharIndex)
	}
	db := decodifica[packets.DeleteFriendPacket](t, proximoDe(t, be, CodeDeleteFriend))
	if db.CharIndex != 1 {
		t.Errorf("Beta recebeu CharIndex %d, queria 1", db.CharIndex)
	}
}

// A origem desativava as linhas e mandava o $F74 para qualquer id.
func TestRemoverQuemNaoEAmigoNaoFazNada(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	_, be := jogador(t, h, f, "Beta", 2)

	p := packets.DeleteFriendPacket{CharIndex: 2}
	if err := h.deleteFriend(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeDeleteFriend, p))); err != nil {
		t.Fatal(err)
	}
	ae.nada(t, 30*time.Millisecond)
	be.nada(t, 30*time.Millisecond)
	if f.removes != 0 {
		t.Errorf("removes = %d, queria 0", f.removes)
	}
}

func TestRemoverComAmigoOfflineSoMexeNaMinhaLista(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	h.OnDisconnect(be.conn) // Beta sai

	p := packets.DeleteFriendPacket{CharIndex: 2}
	if err := h.deleteFriend(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeDeleteFriend, p))); err != nil {
		t.Fatal(err)
	}
	if a.Friends.has(2) {
		t.Error("amigo offline continua na lista")
	}
	if f.ativo(1, 2) {
		t.Error("o banco deve desfazer os dois lados mesmo com o amigo offline")
	}
}

func TestFalhaDoBancoAoRemoverMantemAmizade(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	f.falhaRemove = errors.New("mysql caiu")

	p := packets.DeleteFriendPacket{CharIndex: 2}
	_ = h.deleteFriend(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeDeleteFriend, p)))

	if !a.Friends.has(2) || !b.Friends.has(1) {
		t.Fatal("a amizade saiu da memoria apesar de o banco ter falhado")
	}
}

// ---- janelas de conversa

func abreJanela(t *testing.T, h *Handler, s *Session, e *espiao, charID, janela uint32) {
	t.Helper()
	p := packets.OpenFriendWindowPacket{CharIndex: charID, WindowIndex: janela}
	if err := h.openFriendWindow(context.Background(), e.conn, s,
		asPacket(t, friendPacket(CodeOpenFriendWindow, p))); err != nil {
		t.Fatal(err)
	}
}

func fala(t *testing.T, h *Handler, s *Session, e *espiao, janela uint32, texto string) {
	t.Helper()
	var p packets.SendFriendChatPacket
	p.WindowIndex = janela
	netsrv.PutCString(p.Fala[:], texto)
	netsrv.PutCString(p.Nick[:], "Impostor")
	if err := h.friendSay(context.Background(), e.conn, s,
		asPacket(t, friendPacket(CodeFriendSay, p))); err != nil {
		t.Fatal(err)
	}
}

func TestAbrirJanelaDeAmigoEcoaEGuarda(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)

	abreJanela(t, h, a, ae, 2, 3)

	eco := decodifica[packets.OpenFriendWindowPacket](t, proximoDe(t, ae, CodeOpenFriendWindow))
	if eco.CharIndex != 2 || eco.WindowIndex != 3 {
		t.Errorf("eco = %+v", eco)
	}
	if id, ok := a.Friends.windowChar(3); !ok || id != 2 {
		t.Error("janela 3 nao ficou guardada")
	}
}

// Defeito da origem: so conferia se o personagem estava online, entao dava
// para abrir conversa com qualquer jogador pelo id.
func TestAbrirJanelaComQuemNaoEAmigoE_Recusado(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	jogador(t, h, f, "Beta", 2)

	abreJanela(t, h, a, ae, 2, 0)

	ae.nada(t, 30*time.Millisecond)
	if _, ok := a.Friends.windowChar(0); ok {
		t.Error("janela aberta para quem nao e amigo")
	}
}

func TestLimiteDeSeisJanelasEIndiceRepetido(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	var amigos []*Session
	for i := 0; i < 8; i++ {
		nome := string(rune('B'+i)) + "eta"
		b, be := jogador(t, h, f, nome, uint32(2+i))
		fazAmigos(t, h, a, ae, b, be)
		amigos = append(amigos, b)
	}
	for i := 0; i < 8; i++ {
		abreJanela(t, h, a, ae, uint32(2+i), uint32(i))
	}
	abertas := 0
	for i := uint32(0); i < 8; i++ {
		if _, ok := a.Friends.windowChar(i); ok {
			abertas++
		}
	}
	if abertas != MaxFriendWindows {
		t.Errorf("%d janelas abertas, queria %d", abertas, MaxFriendWindows)
	}
	// Indice repetido nao sobrescreve.
	abreJanela(t, h, a, ae, 3, 0)
	if id, _ := a.Friends.windowChar(0); id != 2 {
		t.Errorf("janela 0 foi sobrescrita para %d", id)
	}
	_ = amigos
}

func TestFecharJanelaSoEsquece(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	abreJanela(t, h, a, ae, 2, 1)
	ae.drena(20 * time.Millisecond)

	p := packets.CloseFriendWindowPacket{WindowIndex: 1}
	if err := h.closeFriendWindow(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeCloseFriendWindow, p))); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Friends.windowChar(1); ok {
		t.Error("janela continua aberta")
	}
	ae.nada(t, 30*time.Millisecond)
}

func TestConversaChegaComAutorEAbreJanelaDoOutroLado(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	abreJanela(t, h, a, ae, 2, 4)
	ae.drena(20 * time.Millisecond)

	fala(t, h, a, ae, 4, "oi, tudo bem?")

	// O autor ve a fala na propria janela.
	eco := decodifica[packets.SendFriendChatPacket](t, proximoDe(t, ae, CodeFriendSay))
	if eco.WindowIndex != 4 || netsrv.CString(eco.Fala[:]) != "oi, tudo bem?" {
		t.Errorf("eco = janela %d fala %q", eco.WindowIndex, netsrv.CString(eco.Fala[:]))
	}

	// O destinatario recebe a abertura da janela e depois a fala.
	ab := decodifica[packets.OpenFriendWindowPacket](t, proximoDe(t, be, CodeOpenFriendWindow))
	if ab.CharIndex != 1 || ab.WindowIndex != 0 {
		t.Errorf("abertura = %+v, queria amigo 1 na janela 0", ab)
	}
	msg := decodifica[packets.SendFriendChatPacket](t, proximoDe(t, be, CodeFriendSay))
	if msg.WindowIndex != ab.WindowIndex {
		t.Errorf("fala na janela %d, aberta %d", msg.WindowIndex, ab.WindowIndex)
	}
	if got := netsrv.CString(msg.Nick[:]); got != "Alfa" {
		t.Errorf("Nick = %q: o outro lado tem de ver o autor, nao o que o pacote trouxe", got)
	}
	if msg.Header.Index != b.ClientID {
		t.Errorf("Header.Index = %d, queria %d", msg.Header.Index, b.ClientID)
	}

	// Segunda fala reaproveita a janela.
	fala(t, h, a, ae, 4, "de novo")
	msg2 := decodifica[packets.SendFriendChatPacket](t, proximoDe(t, be, CodeFriendSay))
	if msg2.WindowIndex != ab.WindowIndex {
		t.Errorf("segunda fala na janela %d, queria %d", msg2.WindowIndex, ab.WindowIndex)
	}
	be.nada(t, 30*time.Millisecond)
}

func TestConversaComAmigoOfflineAvisa(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	abreJanela(t, h, a, ae, 2, 0)
	h.OnDisconnect(be.conn)
	ae.drena(20 * time.Millisecond)

	fala(t, h, a, ae, 0, "voce esta ai?")

	if got := mensagemDe(t, ae); got != "O jogador esta offline." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestConversaEmJanelaInexistenteNaoVaza(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)

	fala(t, h, a, ae, 5, "sem janela")

	ae.nada(t, 30*time.Millisecond)
	be.nada(t, 30*time.Millisecond)
}

// A fala tem 128 bytes e o cliente pode mandar sem terminador.
func TestFalaSemTerminadorFicaNoCampo(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	abreJanela(t, h, a, ae, 2, 0)
	ae.drena(20 * time.Millisecond)

	var p packets.SendFriendChatPacket
	for i := range p.Fala {
		p.Fala[i] = 'x'
	}
	p.WindowIndex = 0
	if err := h.friendSay(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeFriendSay, p))); err != nil {
		t.Fatal(err)
	}
	msg := decodifica[packets.SendFriendChatPacket](t, proximoDe(t, be, CodeFriendSay))
	if n := len(netsrv.CString(msg.Fala[:])); n != len(msg.Fala) {
		t.Errorf("fala com %d caracteres, queria %d", n, len(msg.Fala))
	}
}

func TestRemoverAmigoFechaAsJanelasDosDoisLados(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	abreJanela(t, h, a, ae, 2, 2)
	fala(t, h, a, ae, 2, "oi") // abre a janela do Beta
	ae.drena(20 * time.Millisecond)
	be.drena(20 * time.Millisecond)

	p := packets.DeleteFriendPacket{CharIndex: 2}
	if err := h.deleteFriend(context.Background(), ae.conn, a,
		asPacket(t, friendPacket(CodeDeleteFriend, p))); err != nil {
		t.Fatal(err)
	}

	ca := decodifica[packets.CloseFriendWindowPacket](t, proximoDe(t, ae, CodeCloseFriendWindow))
	if ca.WindowIndex != 2 {
		t.Errorf("Alfa fechou a janela %d, queria 2", ca.WindowIndex)
	}
	proximoDe(t, be, CodeCloseFriendWindow)
	if _, ok := a.Friends.windowChar(2); ok {
		t.Error("janela de Alfa continua guardada")
	}
	if _, ok := b.Friends.windowOf(1); ok {
		t.Error("janela de Beta continua guardada")
	}
}

// ---- entrada e saida

func TestEntrarNoMundoAvisaOsAmigosOnline(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)

	// Alfa "reentra": a lista e relida do banco, que tem Beta.
	a.Avatar.Level = 51
	h.loadFriends(context.Background(), ae.conn, a)

	social := decodifica[packets.SendFriendToSocialPacket](t, proximoDe(t, ae, CodeFriendToSocial))
	if social.CharIndex != 2 || social.FriendStatus != FriendOnline {
		t.Errorf("$870 = %+v", social)
	}
	login := decodifica[packets.FriendLoginPacket](t, proximoDe(t, be, CodeFriendLogin))
	if login.CharIndex != 1 || login.PlayerIndex != a.ClientID || login.FriendStatus != FriendOnline {
		t.Errorf("$96F = %+v", login)
	}
	if login.Level != 50 { // nivel 51 vai como 50
		t.Errorf("Level = %d, queria 50", login.Level)
	}
	upd := decodifica[packets.AtualizeFriendInfosPacket](t, proximoDe(t, be, CodeFriendUpdate))
	if upd.CharIndex != 1 || upd.PlayerIndex != a.ClientID {
		t.Errorf("$975 = %+v", upd)
	}
}

func TestAmigoOfflineVaiNaListaComoOffline(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	f.mu.Lock()
	f.nomes[9] = "Distante"
	f.pares[[2]uint32{1, 9}] = true
	f.mu.Unlock()

	h.loadFriends(context.Background(), ae.conn, a)

	social := decodifica[packets.SendFriendToSocialPacket](t, proximoDe(t, ae, CodeFriendToSocial))
	if social.CharIndex != 9 || social.FriendStatus != FriendOffline {
		t.Errorf("$870 = %+v", social)
	}
	if got := netsrv.CString(social.Nick[:]); got != "Distante" {
		t.Errorf("Nick = %q", got)
	}
}

func TestSairAvisaOsAmigosOnline(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)

	h.OnDisconnect(ae.conn)

	out := decodifica[packets.FriendLogoutPacket](t, proximoDe(t, be, CodeFriendLogout))
	if out.CharIndex != 1 {
		t.Errorf("$971 CharIndex = %d, queria 1", out.CharIndex)
	}
}

func TestLevelDoAmigoTemPisoEmZero(t *testing.T) {
	s := &Session{}
	s.Avatar.Level = 0
	if got := friendLevel(s); got != 0 {
		t.Errorf("nivel 0 = %d, a origem dava 255", got)
	}
	s.Avatar.Level = 1
	if got := friendLevel(s); got != 0 {
		t.Errorf("nivel 1 = %d", got)
	}
	s.Avatar.Level = 60000
	if got := friendLevel(s); got != 255 {
		t.Errorf("nivel enorme = %d, queria teto 255", got)
	}
}

// ---- pacotes e concorrencia

// Os tamanhos de Packets.pas: o cliente desincroniza se qualquer um mudar.
func TestTamanhosDosPacotesDeAmigo(t *testing.T) {
	casos := []struct {
		nome string
		got  int
		want int
	}{
		{"$372 AddFriendRequest", binary.Size(packets.AddFriendRequestPacket{}), 32},
		{"$673 AddFriendResponse", binary.Size(packets.AddFriendResponsePacket{}), 32},
		{"$96F FriendLogin", binary.Size(packets.FriendLoginPacket{}), 24},
		{"$971 FriendLogout", binary.Size(packets.FriendLogoutPacket{}), 16},
		{"$870 SendFriendToSocial", binary.Size(packets.SendFriendToSocialPacket{}), 40},
		{"$975 AtualizeFriendInfos", binary.Size(packets.AtualizeFriendInfosPacket{}), 24},
		{"$F74 DeleteFriend", binary.Size(packets.DeleteFriendPacket{}), 16},
		{"$F26 SendFriendChat", binary.Size(packets.SendFriendChatPacket{}), 160},
		{"$F27 OpenFriendWindow", binary.Size(packets.OpenFriendWindowPacket{}), 20},
		{"$F30 CloseFriendWindow", binary.Size(packets.CloseFriendWindowPacket{}), 16},
	}
	for _, c := range casos {
		if c.got != c.want {
			t.Errorf("%s = %d bytes, o Delphi tem %d", c.nome, c.got, c.want)
		}
	}
}

// Entrada aleatoria nos seis handlers: nada pode dar panic.
func TestHandlersDeAmigoAguentamLixo(t *testing.T) {
	h, f := handlerComAmigos(t)
	a, ae := jogador(t, h, f, "Alfa", 1)
	b, be := jogador(t, h, f, "Beta", 2)
	fazAmigos(t, h, a, ae, b, be)
	go func() {
		for range ae.pacotes {
		}
	}()
	go func() {
		for range be.pacotes {
		}
	}()

	codigos := []uint16{CodeAddFriend, CodeAddFriendResponse, CodeDeleteFriend,
		CodeOpenFriendWindow, CodeCloseFriendWindow, CodeFriendSay}
	var seed uint32 = 12345
	prox := func() byte {
		seed = seed*1664525 + 1013904223
		return byte(seed >> 24)
	}
	for i := 0; i < 3000; i++ {
		buf := make([]byte, 160)
		for j := range buf {
			buf[j] = prox()
		}
		code := codigos[int(prox())%len(codigos)]
		raw := friendPacket(code, packets.SendFriendChatPacket{})
		copy(raw[12:], buf[:len(raw)-12])
		_ = h.HandlePacket(context.Background(), ae.conn, asPacket(t, raw))
	}
}

func TestFriendStateEmParalelo(t *testing.T) {
	var f friendState
	f.reset()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				id := uint32(g*1000 + i)
				_ = f.add(id, "x")
				f.has(id)
				f.openFreeWindow(id)
				f.snapshot()
				f.addPending(uint16(i))
				f.takePending(uint16(i))
				f.remove(id)
			}
		}(g)
	}
	wg.Wait()
	if f.count() != 0 {
		t.Errorf("sobraram %d amigos", f.count())
	}
}

func TestListaDeAmigosTemTeto(t *testing.T) {
	var f friendState
	f.reset()
	for i := uint32(1); i <= store.MaxFriends; i++ {
		if err := f.add(i, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.add(999, "x"); !errors.Is(err, errFriendListFull) {
		t.Errorf("erro = %v, queria lista cheia", err)
	}
	if err := f.add(1, "x"); !errors.Is(err, errFriendKnown) {
		t.Errorf("erro = %v, queria amigo repetido", err)
	}
}
