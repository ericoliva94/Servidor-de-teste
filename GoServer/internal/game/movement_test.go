package game

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"testing"

	"aika/internal/entity"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"time"
)

// movePacket monta um $301 com o destino dado.
func movePacket(x, y float32) []byte {
	var p packets.MovementPacket
	p.Destination = model.Position{X: x, Y: y}
	p.MoveType = MoveNormal
	p.Speed = 25
	buf, err := netsrv.Marshal(CodeMovement, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// sessaoEmJogo devolve uma sessao ja no mundo, numa posicao conhecida.
func sessaoEmJogo(x, y float32) *Session {
	s := sessaoAutenticada()
	s.Status = Playing
	s.SelectedSlot = 0
	s.Position = model.Position{X: x, Y: y}
	s.AnchorPosition = s.Position
	s.SpeedMove = 25
	return s
}

// O original nao validava o destino: IsValid e toda a checagem de velocidade
// estao dentro de um bloco comentado, entao NaN e infinito iam direto para a
// posicao e dali para as contas de distancia.
func TestValidPosition(t *testing.T) {
	validas := []model.Position{
		{X: 0, Y: 0}, {X: 100, Y: 200}, {X: MapLimit, Y: MapLimit},
	}
	for _, p := range validas {
		if !ValidPosition(p) {
			t.Errorf("%v deveria ser valida", p)
		}
	}

	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	invalidas := []model.Position{
		{X: nan, Y: 0}, {X: 0, Y: nan},
		{X: inf, Y: 0}, {X: 0, Y: inf},
		{X: float32(math.Inf(-1)), Y: 0},
		{X: -1, Y: 0}, {X: 0, Y: -1},
		{X: MapLimit + 1, Y: 0}, {X: 0, Y: MapLimit + 1},
	}
	for _, p := range invalidas {
		if ValidPosition(p) {
			t.Errorf("%v deveria ser recusada", p)
		}
	}
}

func TestMovementUpdatesPosition(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)
	s := sessaoEmJogo(100, 100)
	conn := connDescartavel(t)
	h.Chan.Join(conn, s)

	if err := h.movement(context.Background(), conn, s,
		asPacket(t, movePacket(102, 101))); err != nil {
		t.Fatal(err)
	}
	if s.Position.X != 102 || s.Position.Y != 101 {
		t.Errorf("posicao = %v", s.Position)
	}
}

// Posicao invalida nao entra no estado e o cliente e reposicionado.
func TestMovementRejectsInvalidPosition(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)
	s := sessaoEmJogo(100, 100)
	conn := connDescartavel(t)
	h.Chan.Join(conn, s)

	nan := float32(math.NaN())
	if err := h.movement(context.Background(), conn, s,
		asPacket(t, movePacket(nan, 50))); err != nil {
		t.Fatalf("a recusa nao deveria virar erro: %v", err)
	}
	if s.Position.X != 100 || s.Position.Y != 100 {
		t.Errorf("posicao mudou para %v: NaN entrou no estado", s.Position)
	}
	if math.IsNaN(float64(s.Position.X)) {
		t.Error("a posicao virou NaN")
	}
}

func TestMovementRequiresPlaying(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)
	s := sessaoAutenticada() // ainda em CharList
	if err := h.movement(context.Background(), connDescartavel(t), s,
		asPacket(t, movePacket(10, 10))); !errors.Is(err, ErrNotPlaying) {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

func TestMovementIgnoredWhenDead(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)
	s := sessaoEmJogo(100, 100)
	s.Dead = true
	if err := h.movement(context.Background(), connDescartavel(t), s,
		asPacket(t, movePacket(200, 200))); err != nil {
		t.Fatal(err)
	}
	if s.Position.X != 100 {
		t.Errorf("morto se moveu para %v", s.Position)
	}
}

// A lista de visiveis so e recalculada a cada 3 celulas, como no original.
func TestVisibilityThreshold(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)
	s := sessaoEmJogo(100, 100)
	conn := connDescartavel(t)
	h.Chan.Join(conn, s)

	// Dois passos curtos nao mexem na ancora.
	h.movement(context.Background(), conn, s, asPacket(t, movePacket(101, 100)))
	if s.AnchorPosition.X != 100 {
		t.Errorf("ancora mudou com 1 celula: %v", s.AnchorPosition)
	}
	h.movement(context.Background(), conn, s, asPacket(t, movePacket(102, 100)))
	if s.AnchorPosition.X != 100 {
		t.Errorf("ancora mudou com 2 celulas: %v", s.AnchorPosition)
	}
	// Na terceira, recalcula.
	h.movement(context.Background(), conn, s, asPacket(t, movePacket(103, 100)))
	if s.AnchorPosition.X != 103 {
		t.Errorf("ancora = %v, deveria ter acompanhado", s.AnchorPosition)
	}
}

// Andar perto de outro jogador coloca os dois na lista um do outro; afastar
// remove.
func TestVisibilityAddAndRemove(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)

	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(500, 500)
	ca, cb := connDescartavel(t), connDescartavel(t)
	h.Chan.Join(ca, a)
	h.Chan.Join(cb, b)

	// Longe: ninguem ve ninguem.
	h.refreshVisible(a)
	if a.Base.VisiblePlayers.Contains(b.ClientID) {
		t.Fatal("deveriam estar longe")
	}

	// Aproxima o A do B.
	a.Position = model.Position{X: 495, Y: 500}
	h.refreshVisible(a)
	if !a.Base.VisiblePlayers.Contains(b.ClientID) {
		t.Error("A deveria ver B")
	}
	if !b.Base.VisiblePlayers.Contains(a.ClientID) {
		t.Error("B deveria ver A")
	}

	// Afasta de novo.
	a.Position = model.Position{X: 100, Y: 100}
	h.refreshVisible(a)
	if a.Base.VisiblePlayers.Contains(b.ClientID) {
		t.Error("A nao deveria mais ver B")
	}
	if b.Base.VisiblePlayers.Contains(a.ClientID) {
		t.Error("B nao deveria mais ver A")
	}
}

// Movimento chega aos visiveis.
func TestMovementReachesVisiblePlayers(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 100)

	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(102, 100)

	// B escuta de verdade, para conferir que o pacote chega.
	esq, dir := net.Pipe()
	defer esq.Close()
	defer dir.Close()
	recebido := make(chan int, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := dir.Read(buf)
		recebido <- n
	}()

	h.Chan.Join(connDescartavel(t), a)
	h.Chan.Join(netsrv.NewConnForTest(esq), b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	if err := h.movement(context.Background(), connDescartavel(t), a,
		asPacket(t, movePacket(103, 100))); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-recebido:
		if n == 0 {
			t.Error("B nao recebeu bytes")
		}
	case <-timeoutChan():
		t.Error("B nao recebeu o movimento de A")
	}
}

// ---- Channel

func TestChannelJoinAllocatesIDsFromOne(t *testing.T) {
	ch := NewChannel(0, 5)
	vistos := map[uint16]bool{}
	for i := 0; i < 5; i++ {
		s := NewSession(0)
		id, err := ch.Join(connDescartavel(t), s)
		if err != nil {
			t.Fatalf("Join %d: %v", i, err)
		}
		// Zero e "sem id" no protocolo e era o que fazia Players[0] cair
		// antes do inicio do vetor.
		if id == 0 {
			t.Fatal("ClientID zero nao pode ser alocado")
		}
		if id > 5 {
			t.Fatalf("ClientID %d acima do limite", id)
		}
		if vistos[id] {
			t.Fatalf("ClientID %d repetido", id)
		}
		vistos[id] = true
		if s.ClientID != id {
			t.Errorf("sessao com id %d, alocado %d", s.ClientID, id)
		}
	}
	if _, err := ch.Join(connDescartavel(t), NewSession(0)); !errors.Is(err, ErrChannelFull) {
		t.Errorf("erro = %v, queria ErrChannelFull", err)
	}
}

func TestChannelReusesFreedID(t *testing.T) {
	ch := NewChannel(0, 3)
	var ids []uint16
	for i := 0; i < 3; i++ {
		id, _ := ch.Join(connDescartavel(t), NewSession(0))
		ids = append(ids, id)
	}
	ch.Leave(ids[1])
	if ch.Count() != 2 {
		t.Fatalf("Count = %d", ch.Count())
	}
	novo, err := ch.Join(connDescartavel(t), NewSession(0))
	if err != nil {
		t.Fatal(err)
	}
	if novo != ids[1] {
		t.Errorf("reaproveitou %d, o liberado foi %d", novo, ids[1])
	}
}

// Entrar e sair muitas vezes nao pode esgotar os ids.
func TestChannelIDsDoNotLeak(t *testing.T) {
	ch := NewChannel(0, 4)
	for i := 0; i < 1000; i++ {
		s := NewSession(0)
		id, err := ch.Join(connDescartavel(t), s)
		if err != nil {
			t.Fatalf("iteracao %d: %v", i, err)
		}
		if !entity.IsPlayerID(id, 4) {
			t.Fatalf("iteracao %d: id %d fora da faixa", i, id)
		}
		ch.Leave(id)
	}
	if ch.Count() != 0 {
		t.Errorf("sobraram %d sessoes", ch.Count())
	}
}

func TestChannelFindByUsername(t *testing.T) {
	ch := NewChannel(0, 10)
	s := NewSession(0)
	s.Username = "Teste"
	id, _ := ch.Join(connDescartavel(t), s)

	for _, u := range []string{"teste", "TESTE", " Teste "} {
		got, ok := ch.FindByUsername(u)
		if !ok || got != id {
			t.Errorf("busca por %q: id=%d ok=%v", u, got, ok)
		}
	}
	if _, ok := ch.FindByUsername("ninguem"); ok {
		t.Error("usuario inexistente nao deveria ser achado")
	}
	ch.Leave(id)
	if _, ok := ch.FindByUsername("teste"); ok {
		t.Error("deveria sair do indice ao deixar o canal")
	}
}

func TestChannelLookupUnknownID(t *testing.T) {
	ch := NewChannel(0, 10)
	for _, id := range []uint16{0, 1, 999, 65535} {
		if _, _, ok := ch.Lookup(id); ok {
			t.Errorf("id %d nao deveria existir", id)
		}
	}
	// Enviar para id inexistente e inofensivo.
	if err := ch.SendTo(0, make([]byte, 16)); err != nil {
		t.Errorf("SendTo em id inexistente: %v", err)
	}
	ch.Broadcast([]uint16{0, 1, 2}, make([]byte, 16))
}

// Status abaixo de Playing nao recebe difusao.
func TestChannelOnlyPlayingReceives(t *testing.T) {
	ch := NewChannel(0, 10)
	s := NewSession(0)
	s.Status = CharList

	esq, dir := net.Pipe()
	defer esq.Close()
	defer dir.Close()
	go io.Copy(io.Discard, dir)

	id, _ := ch.Join(netsrv.NewConnForTest(esq), s)
	if got := ch.PlayingIDs(); len(got) != 0 {
		t.Errorf("PlayingIDs = %v, queria vazio", got)
	}
	s.Status = Playing
	if got := ch.PlayingIDs(); len(got) != 1 || got[0] != id {
		t.Errorf("PlayingIDs = %v", got)
	}
}

// Acesso concorrente ao canal nao pode corromper nem entrar em panico.
func TestChannelConcurrent(t *testing.T) {
	ch := NewChannel(0, 200)
	const n = 50
	pronto := make(chan struct{})
	fim := make(chan struct{}, n)

	for i := 0; i < n; i++ {
		go func() {
			defer func() { fim <- struct{}{} }()
			<-pronto
			for k := 0; k < 50; k++ {
				s := NewSession(0)
				s.Status = Playing
				id, err := ch.Join(connDescartavelSemT(), s)
				if err != nil {
					continue
				}
				ch.Lookup(id)
				ch.PlayingIDs()
				ch.Broadcast([]uint16{id}, make([]byte, 16))
				ch.FindByUsername("x")
				ch.Leave(id)
			}
		}()
	}
	close(pronto)
	for i := 0; i < n; i++ {
		<-fim
	}
	if ch.Count() != 0 {
		t.Errorf("sobraram %d sessoes", ch.Count())
	}
}

// connDescartavelSemT serve as goroutines do teste de concorrencia, que nao
// podem usar t.Cleanup.
func connDescartavelSemT() *netsrv.Conn {
	esq, dir := net.Pipe()
	go io.Copy(io.Discard, dir)
	return netsrv.NewConnForTest(esq)
}

func timeoutChan() <-chan time.Time { return time.After(2 * time.Second) }
