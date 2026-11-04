package game

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"aika/internal/gamedata"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// pacoteVisto e um pacote que chegou ao espiao, ja decifrado.
type pacoteVisto struct {
	code  uint16
	corpo []byte
}

// espiao e uma conexao de teste que guarda os pacotes recebidos.
type espiao struct {
	conn    *netsrv.Conn
	outra   net.Conn
	pacotes chan pacoteVisto
}

// novoEspiao devolve uma Conn ligada a um leitor que decifra o que chega e
// anota o codigo de cada pacote.
func novoEspiao(t *testing.T) *espiao {
	t.Helper()
	esq, dir := net.Pipe()
	t.Cleanup(func() { esq.Close(); dir.Close() })
	e := &espiao{
		conn:    netsrv.NewConnForTest(esq),
		outra:   dir,
		pacotes: make(chan pacoteVisto, 64),
	}
	go func() {
		for {
			code, corpo, err := lerPacote(dir)
			if err != nil {
				close(e.pacotes)
				return
			}
			e.pacotes <- pacoteVisto{code: code, corpo: corpo}
		}
	}()
	return e
}

// proximo devolve o proximo pacote recebido, ou falha.
func (e *espiao) proximo(t *testing.T) pacoteVisto {
	t.Helper()
	select {
	case p, ok := <-e.pacotes:
		if !ok {
			t.Fatal("conexao fechou antes do pacote")
		}
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("nenhum pacote chegou")
	}
	return pacoteVisto{}
}

// espera devolve o codigo do proximo pacote recebido.
func (e *espiao) espera(t *testing.T) uint16 {
	t.Helper()
	return e.proximo(t).code
}

// drena esvazia a fila de pacotes, esperando ate d por cada um.
func (e *espiao) drena(d time.Duration) {
	for {
		select {
		case _, ok := <-e.pacotes:
			if !ok {
				return
			}
		case <-time.After(d):
			return
		}
	}
}

// nada confere que nenhum pacote chegou na janela dada.
func (e *espiao) nada(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case p, ok := <-e.pacotes:
		if ok {
			t.Fatalf("chegou pacote %#x e nao deveria", p.code)
		}
	case <-time.After(d):
	}
}

// lerPacote le um pacote do fluxo e devolve o codigo e o corpo decifrado.
func lerPacote(c net.Conn) (uint16, []byte, error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		return 0, nil, err
	}
	size := int(binary.LittleEndian.Uint16(head))
	if size < 12 || size > 60000 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, size)
	copy(buf, head)
	if _, err := io.ReadFull(c, buf[2:]); err != nil {
		return 0, nil, err
	}
	if _, err := protocol.Decrypt(buf, size); err != nil {
		return 0, nil, err
	}
	return binary.LittleEndian.Uint16(buf[6:]), buf, nil
}

// ---- spawn

// Dois jogadores perto um do outro recebem o $349 um do outro.
func TestSpawnPairEnviaCriacaoAosDoisLados(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(102, 100)
	a.Avatar.Name = "Alfa"
	b.Avatar.Name = "Beta"
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	h.refreshVisible(a)

	if got := ea.espera(t); got != CodeCreateMob {
		t.Errorf("A recebeu %#x, queria %#x", got, CodeCreateMob)
	}
	if got := eb.espera(t); got != CodeCreateMob {
		t.Errorf("B recebeu %#x, queria %#x", got, CodeCreateMob)
	}
	if !a.Base.VisiblePlayers.Contains(b.ClientID) {
		t.Error("A deveria ter B na lista")
	}
	if !b.Base.VisiblePlayers.Contains(a.ClientID) {
		t.Error("B deveria ter A na lista")
	}
}

// Afastar manda o $101 aos dois lados.
func TestDespawnPairEnviaRemocao(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(102, 100)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	h.refreshVisible(a)
	ea.espera(t) // o spawn
	eb.espera(t)

	a.Position = model.Position{X: 5000, Y: 5000}
	h.refreshVisible(a)

	if got := ea.espera(t); got != CodeRemoveMob {
		t.Errorf("A recebeu %#x, queria %#x", got, CodeRemoveMob)
	}
	if got := eb.espera(t); got != CodeRemoveMob {
		t.Errorf("B recebeu %#x, queria %#x", got, CodeRemoveMob)
	}
	if a.Base.VisiblePlayers.Len() != 0 || b.Base.VisiblePlayers.Len() != 0 {
		t.Error("as listas deveriam estar vazias")
	}
}

// A folga entre ver e esquecer impede o liga-desliga na borda.
//
// Com um limite so, como na origem, andar um passo para fora e um para dentro
// mandava um spawn e um remove por passo.
func TestHisterereseNaBordaNaoRepeteSpawn(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	h.WatchDistance = 10
	h.ForgetDistance = 20

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(100, 100)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)

	// Dentro do raio de ver: entra.
	a.Position = model.Position{X: 105, Y: 100}
	h.refreshVisible(a)
	if got := ea.espera(t); got != CodeCreateMob {
		t.Fatalf("esperava spawn, veio %#x", got)
	}
	eb.espera(t)

	// Entre ver e esquecer: continua visivel, sem pacote novo.
	for _, x := range []float32{112, 118, 115, 111} {
		a.Position = model.Position{X: x, Y: 100}
		h.refreshVisible(a)
	}
	if !a.Base.VisiblePlayers.Contains(b.ClientID) {
		t.Error("deveria continuar visivel na faixa de folga")
	}
	ea.nada(t, 150*time.Millisecond)

	// Acima de esquecer: sai.
	a.Position = model.Position{X: 130, Y: 100}
	h.refreshVisible(a)
	if got := ea.espera(t); got != CodeRemoveMob {
		t.Errorf("esperava remocao, veio %#x", got)
	}
}

// forgetDistance nunca fica abaixo de watchDistance.
func TestForgetNuncaMenorQueWatch(t *testing.T) {
	h := &Handler{WatchDistance: 50, ForgetDistance: 10}
	if got := h.forgetDistance(); got != 50 {
		t.Errorf("forgetDistance = %d, queria 50", got)
	}
	h2 := &Handler{}
	if got := h2.watchDistance(); got != DefaultWatchDistance {
		t.Errorf("watchDistance padrao = %d, queria %d", got, DefaultWatchDistance)
	}
	if got := h2.forgetDistance(); got != DefaultForgetDistance {
		t.Errorf("forgetDistance padrao = %d, queria %d", got, DefaultForgetDistance)
	}
}

// Os padroes sao os de GlobalDefs.pas.
func TestDistanciasPadraoSaoAsDoOriginal(t *testing.T) {
	if DefaultWatchDistance != 50 {
		t.Errorf("DISTANCE_TO_WATCH = %d, GlobalDefs.pas diz 50", DefaultWatchDistance)
	}
	if DefaultForgetDistance != 60 {
		t.Errorf("DISTANCE_TO_FORGET = %d, GlobalDefs.pas diz 60", DefaultForgetDistance)
	}
}

// ---- conteudo do pacote de spawn

func TestCreateMobLevaNomeEPosicao(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(321, 654)
	s.ClientID = 7
	s.Avatar.Name = "Fulano"
	s.Avatar.MaxHP = 1000
	s.Avatar.CurHP = 400
	s.Avatar.MaxMP = 500
	s.Avatar.CurMP = 200
	s.Avatar.SpeedMove = 30

	pkt := h.buildCreateMob(s, SpawnNormal)
	if got := netsrv.CString(pkt.Name[:]); got != "Fulano" {
		t.Errorf("nome = %q", got)
	}
	if pkt.Header.Index != 7 {
		t.Errorf("Index = %d, queria 7", pkt.Header.Index)
	}
	if pkt.Position.X != 321 || pkt.Position.Y != 654 {
		t.Errorf("posicao = %v", pkt.Position)
	}
	if pkt.MaxHP != 1000 || pkt.CurHP != 400 {
		t.Errorf("HP = %d/%d", pkt.CurHP, pkt.MaxHP)
	}
	// A origem preenchia MaxMP com MaxHP no ramo de mob; aqui nao.
	if pkt.MaxMP != 500 || pkt.CurMP != 200 {
		t.Errorf("MP = %d/%d", pkt.CurMP, pkt.MaxMP)
	}
	if pkt.SpeedMove != 30 {
		t.Errorf("SpeedMove = %d", pkt.SpeedMove)
	}
}

// CurHP acima do maximo nao vaza para o pacote.
func TestCreateMobLimitaCurHPAoMaximo(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.MaxHP = 100
	s.Avatar.CurHP = 999999
	s.Avatar.MaxMP = 50
	s.Avatar.CurMP = 999999

	pkt := h.buildCreateMob(s, SpawnNormal)
	if pkt.CurHP != 100 {
		t.Errorf("CurHP = %d, queria 100", pkt.CurHP)
	}
	if pkt.CurMP != 50 {
		t.Errorf("CurMP = %d, queria 50", pkt.CurMP)
	}
}

// SpeedMove e WORD no personagem e BYTE no pacote: nao pode dar a volta.
func TestCreateMobSpeedMoveNaoDaVolta(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.SpeedMove = 1000
	if pkt := h.buildCreateMob(s, SpawnNormal); pkt.SpeedMove != 255 {
		t.Errorf("SpeedMove = %d, queria 255", pkt.SpeedMove)
	}
}

// O titulo de nivel 0 nao pode ir como 255: a origem fazia "Level - 1" num
// BYTE sem piso.
func TestCreateMobTituloNivelZeroNaoEstoura(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.TitleLevel = 0
	if pkt := h.buildCreateMob(s, SpawnNormal); pkt.TitleLevel != 0 {
		t.Errorf("TitleLevel = %d, queria 0", pkt.TitleLevel)
	}
	s.Avatar.TitleLevel = 3
	if pkt := h.buildCreateMob(s, SpawnNormal); pkt.TitleLevel != 2 {
		t.Errorf("TitleLevel = %d, queria 2", pkt.TitleLevel)
	}
}

// Nacao e guilda compartilham um WORD: a guilda nao pode invadir a nacao.
//
// A origem montava o campo concatenando texto hexadecimal; com indice de
// guilda acima de $FFF o numero passava de 16 bits.
func TestGuildIndexNaoInvadeNacao(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.Nation = 2
	s.Avatar.GuildIndex = 0xABCD // acima dos 12 bits
	s.Avatar.GuildName = "Teste"

	pkt := h.buildCreateMob(s, SpawnNormal)
	if nac := pkt.GuildIndexAndNation / nationShift; nac != 2 {
		t.Errorf("nacao lida de volta = %d, queria 2", nac)
	}
	if g := pkt.GuildIndexAndNation & guildIndexMask; g != 0x0BCD {
		t.Errorf("guilda = %#x, queria 0xBCD", g)
	}
}

// Sem guilda o campo leva so a nacao.
func TestSemGuildaSoNacao(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.Nation = 3
	pkt := h.buildCreateMob(s, SpawnNormal)
	if pkt.GuildIndexAndNation != 3*nationShift {
		t.Errorf("campo = %#x", pkt.GuildIndexAndNation)
	}
	if netsrv.CString(pkt.Title[:]) != "" {
		t.Error("Title deveria estar vazio sem guilda")
	}
}

// O nome vermelho vira bit no SpawnType.
func TestPlayerKillViraBitNoSpawnType(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.PlayerKill = true
	if pkt := h.buildCreateMob(s, SpawnNormal); pkt.SpawnType != SpawnPlayerKillFlag {
		t.Errorf("SpawnType = %#x, queria %#x", pkt.SpawnType, SpawnPlayerKillFlag)
	}
}

// Loja pessoal troca o titulo e marca o efeito.
func TestLojaPessoalTrocaTitulo(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	s.Avatar.ShopName = "Vendo tudo"
	pkt := h.buildCreateMob(s, SpawnNormal)
	if got := netsrv.CString(pkt.Title[:]); got != "Vendo tudo" {
		t.Errorf("Title = %q", got)
	}
	if pkt.EffectType != EffectTypeShop {
		t.Errorf("EffectType = %d, queria %d", pkt.EffectType, EffectTypeShop)
	}
	if pkt.Corpo != 3 {
		t.Errorf("Corpo = %d, queria 3", pkt.Corpo)
	}
}

// Mais buffs que o pacote aguenta nao escreve fora do vetor.
//
// A origem fazia "for Index in _buffs.Keys" com i crescendo sem teto.
func TestBuffsAcimaDoLimiteNaoEstouramOPacote(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	agora := time.Now()
	for i := uint16(1); i <= 60; i++ {
		s.Base.Buffs.Set(i, agora)
	}
	pkt := h.buildCreateMob(s, SpawnNormal)
	// Nao entra em panico e o tamanho do pacote nao muda.
	if n := len(pkt.Buffs); n != 60 {
		t.Fatalf("vetor de buffs tem %d", n)
	}
	cheios := 0
	for _, b := range pkt.Buffs {
		if b != 0 {
			cheios++
		}
	}
	if cheios != 60 {
		t.Errorf("%d buffs no pacote, queria 60", cheios)
	}
}

// A aparencia substitui o indice nos slots 2..7, e nao nos 0 e 1.
func TestAparenciaSoValeDoSlotDois(t *testing.T) {
	var a Avatar
	for i := range a.Equip {
		a.Equip[i] = uint16(100 + i)
		a.EquipAPP[i] = uint16(900 + i)
	}
	vis := a.VisibleEquip()
	if vis[0] != 100 || vis[1] != 101 {
		t.Errorf("slots 0 e 1 deveriam usar o indice real: %v", vis[:2])
	}
	for i := 2; i < len(vis); i++ {
		if vis[i] != uint16(900+i) {
			t.Errorf("slot %d = %d, queria %d", i, vis[i], 900+i)
		}
	}
	// Sem aparencia, volta ao indice real.
	a.EquipAPP[3] = 0
	if got := a.VisibleEquip()[3]; got != 103 {
		t.Errorf("slot 3 sem aparencia = %d, queria 103", got)
	}
}

// ---- desconexao

// Sair do canal apaga o jogador da tela e da lista de quem o via.
//
// Na origem isso dependia de Connection.Destroy ser chamado; quando um
// caminho de saida nao chamava, o jogador ficava na lista dos outros e eles
// viam um boneco parado para sempre.
func TestDesconexaoApagaDeTodoMundo(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(101, 100)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	h.refreshVisible(a)
	ea.espera(t)
	eb.espera(t)

	idA := a.ClientID
	h.OnDisconnect(ea.conn)

	if got := eb.espera(t); got != CodeRemoveMob {
		t.Errorf("B recebeu %#x, queria %#x", got, CodeRemoveMob)
	}
	if b.Base.VisiblePlayers.Contains(idA) {
		t.Error("A deveria ter saido da lista de B")
	}
	if _, _, ok := h.Chan.Lookup(idA); ok {
		t.Error("A deveria ter saido do canal")
	}
	if h.Chan.Count() != 1 {
		t.Errorf("canal com %d sessoes, queria 1", h.Chan.Count())
	}
}

// Desconexao antes do login nao pode entrar em panico.
func TestDesconexaoSemSessaoNaoQuebra(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	c := connDescartavel(t)
	h.OnDisconnect(c) // Session nil
	if h.Chan.Count() != 0 {
		t.Error("nada deveria ter sido removido")
	}
}

// O id volta a ficar livre depois da desconexao.
func TestIDVoltaAoCanalDepoisDeSair(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 1)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	if _, err := h.Chan.Join(e.conn, a); err != nil {
		t.Fatal(err)
	}
	// Canal cheio: o segundo nao entra.
	if _, err := h.Chan.Join(connDescartavel(t), sessaoEmJogo(0, 0)); err == nil {
		t.Fatal("canal de 1 lugar aceitou o segundo")
	}
	h.OnDisconnect(e.conn)
	if _, err := h.Chan.Join(connDescartavel(t), sessaoEmJogo(0, 0)); err != nil {
		t.Fatalf("depois da saida deveria caber: %v", err)
	}
}

// ---- movimento endurecido

// O cabecalho do movimento repassado leva o id do autor.
//
// A origem repassava o pacote como chegou, e Header.Index e escrito pelo
// cliente: dava para fazer o boneco de outro jogador andar.
func TestMovimentoNaoAceitaIndexDoCliente(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea := novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	vitima := sessaoEmJogo(101, 100)
	h.Chan.Join(connDescartavel(t), a)
	h.Chan.Join(ea.conn, vitima)
	a.Base.VisiblePlayers.Add(vitima.ClientID)

	// O cliente de A finge ser a vitima no cabecalho.
	var p packets.MovementPacket
	p.Header.Index = vitima.ClientID
	p.Destination = model.Position{X: 102, Y: 100}
	p.MoveType = MoveNormal
	raw, err := netsrv.Marshal(CodeMovement, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.movement(context.Background(), connDescartavel(t), a,
		asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}

	// A vitima recebe o movimento com o id de A, nao com o dela.
	vis := ea.proximo(t)
	if vis.code != CodeMovement {
		t.Fatalf("codigo %#x", vis.code)
	}
	if idx := binary.LittleEndian.Uint16(vis.corpo[4:]); idx != a.ClientID {
		t.Errorf("Index repassado = %d, queria o de A (%d)", idx, a.ClientID)
	}
}

// Teleporte alem do alcance de visao e recusado e o jogador volta.
func TestSaltoAbsurdoERecusado(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	h.WatchDistance = 10
	h.ForgetDistance = 20

	e := novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	h.Chan.Join(e.conn, a)

	if err := h.movement(context.Background(), e.conn, a,
		asPacket(t, movePacket(9000, 9000))); err != nil {
		t.Fatal(err)
	}
	if a.Position.X != 100 || a.Position.Y != 100 {
		t.Errorf("posicao mudou para %v", a.Position)
	}
	if got := e.espera(t); got != CodeMovement {
		t.Errorf("esperava o resync (%#x), veio %#x", CodeMovement, got)
	}
}

// Passo dentro do limite e aceito.
func TestPassoNormalEAceito(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	a := sessaoEmJogo(100, 100)
	h.Chan.Join(connDescartavel(t), a)

	if err := h.movement(context.Background(), connDescartavel(t), a,
		asPacket(t, movePacket(103, 101))); err != nil {
		t.Fatal(err)
	}
	if a.Position.X != 103 || a.Position.Y != 101 {
		t.Errorf("posicao = %v", a.Position)
	}
	if a.LastMoved.IsZero() {
		t.Error("LastMoved deveria ter sido marcado")
	}
}

// ---- Avatar

func TestAvatarDoBancoCalculaMaximos(t *testing.T) {
	src := store.Character{
		ID: 1, Name: "Teste", ClassInfo: 1, Level: 10,
		Constitution: 20, Luck: 10, CurHP: 50, CurMP: 20,
		SpeedMove: 25, Rotation: 3, PlayerKill: true, ActiveTitle: 4,
	}
	a := AvatarFromCharacter(src, 2)
	if a.MaxHP == 0 || a.MaxMP == 0 {
		t.Fatalf("maximos zerados: %d/%d", a.MaxHP, a.MaxMP)
	}
	if a.CurHP != 50 || a.CurMP != 20 {
		t.Errorf("atuais = %d/%d", a.CurHP, a.CurMP)
	}
	if a.Nation != 2 || a.SpeedMove != 25 || a.Rotation != 3 {
		t.Errorf("campos copiados errados: %+v", a)
	}
	if !a.PlayerKill || a.TitleID != 4 {
		t.Errorf("playerkill/titulo errados: %+v", a)
	}
}

// Personagem gravado com HP acima do maximo entra com o teto, nao com a barra
// inflada.
func TestAvatarLimitaHPGravadoAcimaDoMaximo(t *testing.T) {
	src := store.Character{ID: 1, ClassInfo: 1, Level: 1, CurHP: 999999, CurMP: 999999}
	a := AvatarFromCharacter(src, 0)
	if a.CurHP != a.MaxHP {
		t.Errorf("CurHP = %d, MaxHP = %d", a.CurHP, a.MaxHP)
	}
	if a.CurMP != a.MaxMP {
		t.Errorf("CurMP = %d, MaxMP = %d", a.CurMP, a.MaxMP)
	}
}

// HP zerado no banco nao deixa o personagem entrar morto e travado.
func TestAvatarHPZeroEntraComUm(t *testing.T) {
	src := store.Character{ID: 1, ClassInfo: 1, Level: 1, CurHP: 0}
	if a := AvatarFromCharacter(src, 0); a.CurHP != 1 {
		t.Errorf("CurHP = %d, queria 1", a.CurHP)
	}
}

// Velocidade zero no banco nao pode travar o boneco.
func TestAvatarVelocidadeZeroUsaPadrao(t *testing.T) {
	src := store.Character{ID: 1, ClassInfo: 1, Level: 1, SpeedMove: 0}
	if a := AvatarFromCharacter(src, 0); a.SpeedMove != DefaultSpeedMove {
		t.Errorf("SpeedMove = %d, queria %d", a.SpeedMove, DefaultSpeedMove)
	}
}

// ---- posicao de entrada

func TestEntryPositionUsaOBanco(t *testing.T) {
	p := entryPosition(store.Character{PosX: 1234, PosY: 4321})
	if p.X != 1234 || p.Y != 4321 {
		t.Errorf("posicao = %v", p)
	}
}

func TestEntryPositionZeroCaiNoPadrao(t *testing.T) {
	if p := entryPosition(store.Character{}); p != StartPosition {
		t.Errorf("posicao = %v, queria %v", p, StartPosition)
	}
}

func TestEntryPositionInvalidaCaiNoPadrao(t *testing.T) {
	if p := entryPosition(store.Character{PosX: 999999, PosY: 10}); p != StartPosition {
		t.Errorf("posicao = %v, queria %v", p, StartPosition)
	}
}

// ---- equipamento

// itensFalsos responde conjuntos montados a mao.
type itensFalsos struct {
	equip map[uint32]store.ItemSet
	inv   map[uint32]store.ItemSet
	arm   map[uint32]store.ItemSet
	bau   map[uint32]store.ItemSet
	cash  map[uint32]store.ItemSet
	erro  error
	// gravados guarda o que os Save* receberam.
	gravados map[string]store.ItemSet
}

func (f itensFalsos) Equip(_ context.Context, id uint32) (store.ItemSet, error) {
	if f.erro != nil {
		return store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}, f.erro
	}
	if s, ok := f.equip[id]; ok {
		return s, nil
	}
	return store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}, nil
}

func (f itensFalsos) Inventory(_ context.Context, id uint32) (store.ItemSet, error) {
	if f.erro != nil {
		return store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}, f.erro
	}
	if s, ok := f.inv[id]; ok {
		return s, nil
	}
	return store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}, nil
}

func (f itensFalsos) SaveEquip(_ context.Context, id uint32, set store.ItemSet) error {
	if f.gravados != nil {
		f.gravados[fmt.Sprintf("equip:%d", id)] = set
	}
	return f.erro
}

func (f itensFalsos) SaveInventory(_ context.Context, id uint32, set store.ItemSet) error {
	if f.gravados != nil {
		f.gravados[fmt.Sprintf("inv:%d", id)] = set
	}
	return f.erro
}

func (f itensFalsos) Storage(_ context.Context, id uint32) (store.ItemSet, error) {
	if s, ok := f.arm[id]; ok {
		return s, f.erro
	}
	return store.ItemSet{Slots: make([]model.Item, store.StorageSlots)}, f.erro
}

func (f itensFalsos) SaveStorage(_ context.Context, id uint32, set store.ItemSet) error {
	if f.gravados != nil {
		f.gravados[fmt.Sprintf("armazem:%d", id)] = set
	}
	return f.erro
}

func (f itensFalsos) GuildChest(_ context.Context, id uint32) (store.ItemSet, error) {
	if s, ok := f.bau[id]; ok {
		return s, f.erro
	}
	return store.ItemSet{Slots: make([]model.Item, store.GuildChestSlots)}, f.erro
}

func (f itensFalsos) SaveGuildChest(_ context.Context, id uint32, set store.ItemSet) error {
	if f.gravados != nil {
		f.gravados[fmt.Sprintf("bau:%d", id)] = set
	}
	return f.erro
}

func (f itensFalsos) Cash(_ context.Context, id uint32) (store.ItemSet, error) {
	if s, ok := f.cash[id]; ok {
		return s, f.erro
	}
	return store.ItemSet{Slots: make([]model.Item, store.CashSlots)}, f.erro
}

func (f itensFalsos) SaveCash(_ context.Context, id uint32, set store.ItemSet) error {
	if f.gravados != nil {
		f.gravados[fmt.Sprintf("cash:%d", id)] = set
	}
	return f.erro
}

// conjuntoEquip monta um ItemSet de equipamento a partir de pares slot/indice.
func conjuntoEquip(pares map[int]model.Item) store.ItemSet {
	s := store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}
	for slot, it := range pares {
		s.Slots[slot] = it
	}
	return s
}

// O equipamento carregado chega ao pacote de spawn.
func TestEquipamentoChegaAoSpawn(t *testing.T) {
	h := testHandler()
	s := sessaoEmJogo(0, 0)
	set := conjuntoEquip(map[int]model.Item{
		0: {Index: 1001},
		1: {Index: 1002},
		3: {Index: 1003, APP: 2003},
		6: {Index: 1006, Refi: 48}, // 48/16 = 3
	})
	s.Avatar.SetEquip(set)

	pkt := h.buildCreateMob(s, SpawnNormal)
	if pkt.Equip[0] != 1001 || pkt.Equip[1] != 1002 {
		t.Errorf("slots 0 e 1 = %d/%d", pkt.Equip[0], pkt.Equip[1])
	}
	// Slot 3 tem aparencia: ela e que vai.
	if pkt.Equip[3] != 2003 {
		t.Errorf("slot 3 = %d, queria a aparencia 2003", pkt.Equip[3])
	}
	// O refino do slot 6 vai em ItemEff[7] como divisao inteira por 16.
	if pkt.ItemEff[7] != 3 {
		t.Errorf("ItemEff[7] = %d, queria 3", pkt.ItemEff[7])
	}
}

// Entrar no mundo carrega os itens e os poe no $925.
func TestEntradaNoMundoCarregaItens(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)
	h.Chan = NewChannel(0, 10)
	personagem := personagens()[7][0]
	h.Items = itensFalsos{
		equip: map[uint32]store.ItemSet{
			personagem.ID: conjuntoEquip(map[int]model.Item{
				2: {Index: 555, Min: 10, Max: 20},
			}),
		},
		inv: map[uint32]store.ItemSet{
			personagem.ID: func() store.ItemSet {
				s := store.ItemSet{Slots: make([]model.Item, store.InventorySlots)}
				s.Slots[5] = model.Item{Index: 777}
				return s
			}(),
		},
	}

	s := sessaoAutenticada()
	e := novoEspiao(t)
	h.Chan.Join(e.conn, s)

	if err := h.sendToWorld(context.Background(), e.conn, s, 0); err != nil {
		t.Fatal(err)
	}
	if s.Equip.Get(2).Index != 555 {
		t.Errorf("equipamento na sessao = %d", s.Equip.Get(2).Index)
	}
	if s.Inventory.Get(5).Index != 777 {
		t.Errorf("inventario na sessao = %d", s.Inventory.Get(5).Index)
	}
	if s.Avatar.Equip[2] != 555 {
		t.Errorf("avatar nao recebeu o equipamento: %d", s.Avatar.Equip[2])
	}

	// O $131 vem antes do $925.
	if got := e.espera(t); got != CodeP131 {
		t.Fatalf("primeiro pacote %#x, queria %#x", got, CodeP131)
	}
	mundo := e.proximo(t)
	if mundo.code != CodeSendToWorld {
		t.Fatalf("segundo pacote %#x, queria %#x", mundo.code, CodeSendToWorld)
	}

	var pkt packets.SendToWorldPacket
	if err := netsrv.Unmarshal(asPacote(t, mundo.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.Character.Equip[2].Index != 555 {
		t.Errorf("equipamento no $925 = %d", pkt.Character.Equip[2].Index)
	}
	if pkt.Character.Inventory[5].Index != 777 {
		t.Errorf("inventario no $925 = %d", pkt.Character.Inventory[5].Index)
	}
}

// Falha ao ler itens nao impede a entrada: o jogador entra sem o que nao pudo
// ser lido, e nao fica preso na tela de selecao.
func TestFalhaAoLerItensNaoImpedeAEntrada(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)
	h.Chan = NewChannel(0, 10)
	h.Items = itensFalsos{erro: errItensQuebrados}

	s := sessaoAutenticada()
	e := novoEspiao(t)
	h.Chan.Join(e.conn, s)

	if err := h.sendToWorld(context.Background(), e.conn, s, 0); err != nil {
		t.Fatalf("entrada deveria ter seguido: %v", err)
	}
	if s.Status != Playing {
		t.Errorf("estado = %v, queria Playing", s.Status)
	}
}

var errItensQuebrados = errors.New("falha de teste ao ler itens")

// asPacote reenquadra bytes ja decifrados para o Unmarshal poder ler.
func asPacote(t *testing.T, corpo []byte) protocol.Packet {
	t.Helper()
	return protocol.Packet{
		Size: len(corpo),
		Code: binary.LittleEndian.Uint16(corpo[6:]),
		Data: corpo,
	}
}

// O bonus de refino do equipamento entra no HP e no MP maximos.
func TestBonusDeRefinoEntraNoMaximo(t *testing.T) {
	var a Avatar
	a.ClassInfo = 1
	a.Level = 10
	a.Cons = 20
	a.Luck = 20
	a.RecalcMax()
	semBonus := a.MaxHP
	semBonusMP := a.MaxMP

	a.EquipHPMPBonus = 500
	a.RecalcMax()
	if a.MaxHP != semBonus+500 {
		t.Errorf("MaxHP = %d, queria %d", a.MaxHP, semBonus+500)
	}
	if a.MaxMP != semBonusMP+500 {
		t.Errorf("MaxMP = %d, queria %d", a.MaxMP, semBonusMP+500)
	}
}

// Perder o bonus reduz o maximo, e o atual nao pode ficar acima dele.
func TestPerderBonusNaoDeixaHPAcimaDoMaximo(t *testing.T) {
	var a Avatar
	a.ClassInfo = 1
	a.Level = 1
	a.EquipHPMPBonus = 10000
	a.RecalcMax()
	a.CurHP = a.MaxHP
	a.CurMP = a.MaxMP

	a.ApplyEquipBonus(tabelaVazia{}, store.ItemSet{
		Slots: make([]model.Item, store.EquipSlots)})
	if a.CurHP > a.MaxHP {
		t.Errorf("CurHP %d acima do MaxHP %d", a.CurHP, a.MaxHP)
	}
	if a.CurMP > a.MaxMP {
		t.Errorf("CurMP %d acima do MaxMP %d", a.CurMP, a.MaxMP)
	}
}

// tabelaVazia responde que nada existe.
type tabelaVazia struct{}

func (tabelaVazia) Item(int) *gamedata.ItemFromList { return nil }

func (tabelaVazia) Reinforce3At(int) *gamedata.ArmorAttributeReinforce { return nil }

// Sem tabela carregada o bonus fica como estava, sem quebrar.
func TestApplyEquipBonusSemTabela(t *testing.T) {
	var a Avatar
	a.ClassInfo = 1
	a.Level = 1
	a.EquipHPMPBonus = 7
	a.ApplyEquipBonus(nil, store.ItemSet{})
	if a.EquipHPMPBonus != 7 {
		t.Errorf("bonus = %d, queria 7", a.EquipHPMPBonus)
	}
}
