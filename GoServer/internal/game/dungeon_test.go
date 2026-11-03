package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/dungeon"
	"aika/internal/dungeondata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// tabelaDungeon e uma tabela de dungeon em memoria, com uma dungeon de id 1
// em tres dificuldades.
type tabelaDungeon struct {
	defs map[uint32][3]*dungeondata.Definition
}

func novaTabelaDungeon(pontos int) *tabelaDungeon {
	t := &tabelaDungeon{defs: map[uint32][3]*dungeondata.Definition{}}
	var porDif [3]*dungeondata.Definition
	for dif := 0; dif < 3; dif++ {
		d := &dungeondata.Definition{
			Index:                  1,
			Name:                   "Teste",
			Dificult:               uint8(dif),
			LevelMin:               uint8(20 + dif*10),
			EntranceNPCID:          2109,
			SpawnInDungeonPosition: model.Position{X: 3000, Y: 3000},
		}
		sp := dungeondata.Species{
			IntName: 172, Name: "Bicho", HP: 100,
			AtkFis: 50, DefFis: 100, Exp: 1000, DropIndex: 11,
		}
		for i := 0; i < pontos; i++ {
			sp.Spawns = append(sp.Spawns, dungeondata.Spawn{
				ClientID: uint16(2600 + i), SecondIndex: i, HP: 100, MP: 100,
				InitPos: model.Position{X: 3000, Y: 3000},
			})
		}
		d.Species = []dungeondata.Species{sp}
		d.Drops[dungeondata.NoCrown] = []uint16{1001}
		porDif[dif] = d
	}
	t.defs[1] = porDif
	return t
}

func (t *tabelaDungeon) Get(id, dif uint32) (*dungeondata.Definition, bool) {
	porDif, ok := t.defs[id]
	if !ok || dif >= 3 {
		return nil, false
	}
	return porDif[dif], true
}

func (t *tabelaDungeon) ByEntranceNPC(npc uint16, dif uint32) (*dungeondata.Definition, bool) {
	for id := range t.defs {
		if d, ok := t.Get(id, dif); ok && d.EntranceNPCID == npc {
			return d, true
		}
	}
	return nil, false
}

func handlerComDungeon(t *testing.T, pontos int) (*Handler, *tabelaDungeon) {
	h := handlerComGuildas(t)
	tab := novaTabelaDungeon(pontos)
	h.Dungeons = dungeon.NewRegistry()
	h.DungeonLobbies = dungeon.NewLobbies()
	h.DungeonTable = tab
	h.MapObjs = NewMapObjects()
	return h, tab
}

// naPortaDaDungeon poe o jogador em jogo com a janela do NPC aberta.
func naPortaDaDungeon(t *testing.T, h *Handler, nome string, charID uint32,
	nivel uint16) (*Session, *espiao) {
	s, e := entraComPersonagem(t, h, nome, charID)
	s.OpenedNPC = 2109
	s.OpenedOption = DungeonNPCOption
	s.Avatar.Level = nivel
	s.Position = model.Position{X: 100, Y: 100}
	e.drena(100 * time.Millisecond)
	return s, e
}

func enterDungeonPacket(dif uint32) []byte {
	var p packets.SelectDungeonToEnter
	p.Dificult = dif
	buf, err := netsrv.Marshal(CodeDungeonEnter, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func confirmDungeonPacket(index uint32) []byte {
	var p packets.ConfirmDungeonEnter
	p.Index = index
	buf, err := netsrv.Marshal(CodeDungeonConfirm, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- pedido de entrada

func TestPedirDungeonSozinhoAbreSaguao(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)

	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	lobby, ok := h.DungeonLobbies.Of(100)
	if !ok {
		t.Fatal("o saguao nao abriu")
	}
	if lobby.DungeonID != 1 || lobby.Dificult != 0 {
		t.Errorf("saguao = dungeon %d dif %d", lobby.DungeonID, lobby.Dificult)
	}
	vis := e.proximo(t)
	if vis.code != CodeDungeonLobby {
		t.Errorf("codigo %#x, queria %#x", vis.code, CodeDungeonLobby)
	}
}

// A dificuldade e um DWORD do cliente e a origem indexava com ela um vetor de
// um elemento.
func TestDificuldadeForaDaFaixaERecusada(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 99)

	for _, dif := range []uint32{3, 4, 255, 1 << 20, 0xFFFFFFFF} {
		if err := h.requestEnterDungeon(context.Background(), e.conn, s,
			asPacket(t, enterDungeonPacket(dif))); err != nil {
			t.Fatalf("dif %d: %v", dif, err)
		}
		if _, ok := h.DungeonLobbies.Of(100); ok {
			t.Fatalf("dif %d abriu saguao", dif)
		}
	}
}

func TestSemNivelMinimoNaoEntra(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 19) // minimo 20

	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("abriu saguao sem nivel")
	}
}

// A origem exigia OpennedOption = 26; sem isso o pacote valia de qualquer
// lugar. Mantido.
func TestSemJanelaDoNpcNaoEntra(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	s.OpenedOption = 0

	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("abriu saguao sem a janela do NPC")
	}
}

func TestNpcQueNaoAbreDungeon(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	s.OpenedNPC = 9999

	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("abriu saguao com NPC que nao e portal")
	}
}

func TestSoOLiderPedeDungeon(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	a, ea := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	b, eb := naPortaDaDungeon(t, h, "Beta", 200, 30)
	grupo, err := h.Parties.Create(a.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Parties.Add(grupo, b.ClientID); err != nil {
		t.Fatal(err)
	}
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	// O membro pede: recusado.
	if err := h.requestEnterDungeon(context.Background(), eb.conn, b,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(200); ok {
		t.Error("membro abriu saguao")
	}
	// O lider pede: abre para os dois.
	if err := h.requestEnterDungeon(context.Background(), ea.conn, a,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	for _, charID := range []uint32{100, 200} {
		if _, ok := h.DungeonLobbies.Of(charID); !ok {
			t.Errorf("o membro %d nao entrou no saguao", charID)
		}
	}
}

// Na origem o nivel do grupo era conferido para os jogadores das conexoes
// 0,1,2... e nao para os membros, no caso do NPC do Kinary.
func TestNivelDeTodoMembroEConferido(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	a, ea := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	b, eb := naPortaDaDungeon(t, h, "Beta", 200, 5) // abaixo do minimo
	grupo, _ := h.Parties.Create(a.ClientID)
	h.Parties.Add(grupo, b.ClientID)
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)
	_ = b

	if err := h.requestEnterDungeon(context.Background(), ea.conn, a,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("abriu saguao com membro abaixo do nivel")
	}
}

func TestJaEstaEmDungeonNaoPedeOutra(t *testing.T) {
	h, tab := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	def, _ := tab.Get(1, 0)
	if _, err := h.Dungeons.Open(def, 0, []uint32{100}, h.now()); err != nil {
		t.Fatal(err)
	}
	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("abriu saguao estando em dungeon")
	}
}

// ---- confirmacao

func TestConfirmarSozinhoEntra(t *testing.T) {
	h, _ := handlerComDungeon(t, 3)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	antes := s.Position

	h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0)))
	e.drena(100 * time.Millisecond)

	if err := h.confirmDungeon(context.Background(), e.conn, s,
		asPacket(t, confirmDungeonPacket(uint32(s.ClientID)))); err != nil {
		t.Fatal(err)
	}
	in, ok := h.Dungeons.Of(100)
	if !ok {
		t.Fatal("a instancia nao abriu")
	}
	if in.MobCount() != 3 {
		t.Errorf("%d mobs", in.MobCount())
	}
	if s.DungeonInstance != in.ID || s.DungeonID != 1 {
		t.Errorf("sessao = instancia %d dungeon %d", s.DungeonInstance, s.DungeonID)
	}
	// Foi teleportado para dentro e a posicao de volta ficou guardada.
	if s.Position != (model.Position{X: 3000, Y: 3000}) {
		t.Errorf("posicao = %+v", s.Position)
	}
	if s.ReturnPosition != antes {
		t.Errorf("volta = %+v, queria %+v", s.ReturnPosition, antes)
	}
	// O saguao sumiu.
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("o saguao ficou aberto")
	}
}

// A origem creditava a confirmacao ao indice do cabecalho, que vem do
// cliente: um jogador confirmava pelos outros e arrastava o grupo.
func TestUmJogadorNaoConfirmaPelosOutros(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	a, ea := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	b, eb := naPortaDaDungeon(t, h, "Beta", 200, 30)
	grupo, _ := h.Parties.Create(a.ClientID)
	h.Parties.Add(grupo, b.ClientID)
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	h.requestEnterDungeon(context.Background(), ea.conn, a,
		asPacket(t, enterDungeonPacket(0)))
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	// Alfa confirma mandando o indice do Beta, e depois o proprio: so conta
	// uma confirmacao, a dele.
	for _, idx := range []uint32{uint32(b.ClientID), uint32(a.ClientID), 777} {
		if err := h.confirmDungeon(context.Background(), ea.conn, a,
			asPacket(t, confirmDungeonPacket(idx))); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := h.Dungeons.Of(100); ok {
		t.Fatal("a instancia abriu com uma confirmacao so")
	}
	lobby, _ := h.DungeonLobbies.Of(100)
	if lobby.Confirmed() != 1 {
		t.Errorf("%d confirmacoes, queria 1", lobby.Confirmed())
	}
	// Agora Beta confirma: entra.
	if err := h.confirmDungeon(context.Background(), eb.conn, b,
		asPacket(t, confirmDungeonPacket(uint32(b.ClientID)))); err != nil {
		t.Fatal(err)
	}
	inA, okA := h.Dungeons.Of(100)
	inB, okB := h.Dungeons.Of(200)
	if !okA || !okB || inA != inB {
		t.Fatal("os dois deveriam estar na mesma instancia")
	}
	if a.DungeonInstance == 0 || b.DungeonInstance == 0 {
		t.Errorf("instancias nas sessoes: %d e %d",
			a.DungeonInstance, b.DungeonInstance)
	}
}

func TestRecusaFechaParaTodos(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	a, ea := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	b, eb := naPortaDaDungeon(t, h, "Beta", 200, 30)
	grupo, _ := h.Parties.Create(a.ClientID)
	h.Parties.Add(grupo, b.ClientID)
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	h.requestEnterDungeon(context.Background(), ea.conn, a,
		asPacket(t, enterDungeonPacket(0)))
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	// Index = 0 e a recusa.
	if err := h.confirmDungeon(context.Background(), eb.conn, b,
		asPacket(t, confirmDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	for _, charID := range []uint32{100, 200} {
		if _, ok := h.DungeonLobbies.Of(charID); ok {
			t.Errorf("o saguao do %d ficou aberto", charID)
		}
	}
	if h.Dungeons.Count() != 0 {
		t.Error("a instancia abriu depois da recusa")
	}
}

func TestConfirmarSemSaguaoNaoFazNada(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	if err := h.confirmDungeon(context.Background(), e.conn, s,
		asPacket(t, confirmDungeonPacket(1))); err != nil {
		t.Fatal(err)
	}
	if h.Dungeons.Count() != 0 {
		t.Error("abriu instancia sem saguao")
	}
}

func TestSaguaoVencidoNaoEntra(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0)))
	e.drena(50 * time.Millisecond)

	// O relogio do handler anda alem do prazo do saguao.
	base := h.now()
	h.Now = func() time.Time { return base.Add(dungeon.LobbyTimeout + time.Second) }

	if err := h.confirmDungeon(context.Background(), e.conn, s,
		asPacket(t, confirmDungeonPacket(uint32(s.ClientID)))); err != nil {
		t.Fatal(err)
	}
	if h.Dungeons.Count() != 0 {
		t.Error("entrou com o saguao vencido")
	}
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("o saguao vencido ficou no registro")
	}
}

// Varios grupos ao mesmo tempo: a origem tinha um slot de instancia para o
// servidor inteiro.
func TestDoisGruposEntramAoMesmoTempo(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	var sessoes []*Session
	for i := 0; i < 6; i++ {
		charID := uint32(100 + i)
		s, e := naPortaDaDungeon(t, h, "Jog"+string(rune('A'+i)), charID, 30)
		h.requestEnterDungeon(context.Background(), e.conn, s,
			asPacket(t, enterDungeonPacket(0)))
		e.drena(50 * time.Millisecond)
		if err := h.confirmDungeon(context.Background(), e.conn, s,
			asPacket(t, confirmDungeonPacket(uint32(s.ClientID)))); err != nil {
			t.Fatal(err)
		}
		sessoes = append(sessoes, s)
	}
	if h.Dungeons.Count() != 6 {
		t.Errorf("%d instancias, queria 6", h.Dungeons.Count())
	}
	// Cada um na sua.
	vistos := map[uint32]bool{}
	for _, s := range sessoes {
		if s.DungeonInstance == 0 {
			t.Errorf("%s ficou sem instancia", s.Avatar.Name)
		}
		if vistos[s.DungeonInstance] {
			t.Errorf("instancia %d repetida", s.DungeonInstance)
		}
		vistos[s.DungeonInstance] = true
	}
}

// ---- combate dentro da dungeon

func TestBaterNoMobDaInstancia(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	m := in.Mobs()[0]
	s.Position = m.Pos

	if !h.attackDungeonMob(context.Background(), e.conn, s, m.ClientID) {
		t.Fatal("o golpe deveria ter sido tratado pela dungeon")
	}
	if m.CurHP == m.MaxHP {
		t.Error("o mob nao perdeu HP")
	}
}

// Id que nao esta na instancia nao cai no handler de jogador.
func TestIdForaDaInstanciaERecusado(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)

	for _, id := range []uint16{0, 1, 2, 2599, 3048, 65535} {
		if !h.attackDungeonMob(context.Background(), e.conn, s, id) {
			t.Errorf("id %d escapou da dungeon", id)
		}
	}
}

// Fora de dungeon o golpe segue o caminho normal.
func TestForaDaDungeonOGolpeSegue(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	if h.attackDungeonMob(context.Background(), e.conn, s, 3048) {
		t.Error("o golpe fora da dungeon nao deveria ser tratado aqui")
	}
}

func TestMobLongeNaoLevaGolpe(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	m := in.Mobs()[0]
	s.Position = model.Position{X: m.Pos.X + 1000, Y: m.Pos.Y}

	h.attackDungeonMob(context.Background(), e.conn, s, m.ClientID)
	if m.CurHP != m.MaxHP {
		t.Error("o mob longe levou golpe")
	}
}

// A exp do grupo e dividida uma vez, nao uma vez por membro.
func TestExpDoGrupoEDivididaUmaVez(t *testing.T) {
	h, _ := handlerComDungeon(t, 1)
	var sessoes []*Session
	var espioes []*espiao
	for i := 0; i < 4; i++ {
		s, e := naPortaDaDungeon(t, h, "Jog"+string(rune('A'+i)), uint32(100+i), 30)
		s.Avatar.Exp = 0
		sessoes = append(sessoes, s)
		espioes = append(espioes, e)
	}
	grupo, _ := h.Parties.Create(sessoes[0].ClientID)
	for _, s := range sessoes[1:] {
		h.Parties.Add(grupo, s.ClientID)
	}
	for _, e := range espioes {
		e.drena(50 * time.Millisecond)
	}
	h.requestEnterDungeon(context.Background(), espioes[0].conn, sessoes[0],
		asPacket(t, enterDungeonPacket(0)))
	for i, s := range sessoes {
		espioes[i].drena(50 * time.Millisecond)
		if err := h.confirmDungeon(context.Background(), espioes[i].conn, s,
			asPacket(t, confirmDungeonPacket(uint32(s.ClientID)))); err != nil {
			t.Fatal(err)
		}
	}
	in, ok := h.Dungeons.Of(100)
	if !ok {
		t.Fatal("a instancia nao abriu")
	}
	if len(in.MemberList()) != 4 {
		t.Fatalf("%d membros", len(in.MemberList()))
	}
	m := in.Mobs()[0]
	in.Hit(m, m.MaxHP, 100, h.now())
	h.dungeonMobDied(context.Background(), in, m)

	// Exp 1000 / 4 membros = 250 para cada. Na origem: 250, 62, 15, 3.
	for _, s := range sessoes {
		if s.Avatar.Exp != 250 {
			t.Errorf("%s ficou com %d de exp, queria 250", s.Avatar.Name, s.Avatar.Exp)
		}
	}
}

// ---- saida

func TestSairDaDungeonVolta(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	antes := s.Position
	entraNaDungeon(t, h, s, e)

	h.leaveDungeon(context.Background(), e.conn, s)
	if _, ok := h.Dungeons.Of(100); ok {
		t.Error("continua na instancia")
	}
	if s.DungeonInstance != 0 || s.DungeonID != 0 {
		t.Errorf("sessao = %d/%d", s.DungeonInstance, s.DungeonID)
	}
	if s.Position != antes {
		t.Errorf("voltou para %+v, queria %+v", s.Position, antes)
	}
	if h.Dungeons.Count() != 0 {
		t.Error("a instancia ficou aberta sem ninguem")
	}
}

func TestQuedaSolteDaInstancia(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)

	h.dungeonOnDisconnect(s)
	if _, ok := h.Dungeons.Of(100); ok {
		t.Error("continua na instancia depois da queda")
	}
	if h.Dungeons.Count() != 0 {
		t.Error("a instancia ficou aberta: na origem isso travava a dungeon " +
			"do servidor inteiro")
	}
}

func TestQuedaNoSaguaoCancelaParaOGrupo(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	a, ea := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	b, eb := naPortaDaDungeon(t, h, "Beta", 200, 30)
	grupo, _ := h.Parties.Create(a.ClientID)
	h.Parties.Add(grupo, b.ClientID)
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)
	h.requestEnterDungeon(context.Background(), ea.conn, a,
		asPacket(t, enterDungeonPacket(0)))
	ea.drena(50 * time.Millisecond)
	eb.drena(50 * time.Millisecond)

	h.dungeonOnDisconnect(b)
	for _, charID := range []uint32{100, 200} {
		if _, ok := h.DungeonLobbies.Of(charID); ok {
			t.Errorf("o saguao do %d ficou aberto", charID)
		}
	}
}

// ---- tick

func TestTickFechaInstanciaLimpa(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	for _, m := range in.Mobs() {
		in.Hit(m, m.MaxHP, 100, h.now())
	}
	if n := h.DungeonTick(context.Background()); n != 1 {
		t.Errorf("%d instancias fechadas, queria 1", n)
	}
	if h.Dungeons.Count() != 0 {
		t.Error("a instancia limpa ficou aberta")
	}
	if s.DungeonInstance != 0 {
		t.Errorf("a sessao ficou com a instancia %d", s.DungeonInstance)
	}
}

func TestTickFechaInstanciaVencida(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	base := h.now()
	h.Now = func() time.Time { return base.Add(dungeon.MaxLifetime + time.Minute) }

	if n := h.DungeonTick(context.Background()); n != 1 {
		t.Errorf("%d fechadas", n)
	}
	if h.Dungeons.Count() != 0 {
		t.Error("a instancia vencida ficou aberta")
	}
	_ = s
}

func TestTickVenceSaguao(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0)))
	e.drena(50 * time.Millisecond)
	base := h.now()
	h.Now = func() time.Time { return base.Add(dungeon.LobbyTimeout + time.Second) }

	h.DungeonTick(context.Background())
	if _, ok := h.DungeonLobbies.Of(100); ok {
		t.Error("o saguao vencido ficou aberto")
	}
}

// O mob pega alvo, persegue e bate.
func TestMobDaDungeonPegaAlvoEBate(t *testing.T) {
	h, _ := handlerComDungeon(t, 1)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	m := in.Mobs()[0]
	// Jogador em cima do mob: pega alvo e bate no mesmo tick seguinte.
	s.Position = m.Pos
	s.Avatar.CurHP = s.Avatar.MaxHP
	hpAntes := s.Avatar.CurHP

	agora := h.now()
	h.dungeonMobTick(context.Background(), in, agora)
	if m.AttackerCharIndex != 100 {
		t.Fatalf("o mob nao pegou alvo: %d", m.AttackerCharIndex)
	}
	// O primeiro golpe precisa do intervalo.
	h.dungeonMobTick(context.Background(), in, agora.Add(dungeon.AttackInterval))
	if s.Avatar.CurHP >= hpAntes {
		t.Errorf("HP = %d, nao caiu de %d", s.Avatar.CurHP, hpAntes)
	}
}

func TestMobDaDungeonPerseguePorPassos(t *testing.T) {
	h, _ := handlerComDungeon(t, 1)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	m := in.Mobs()[0]
	// Dentro do alcance de agressao, fora do de golpe.
	s.Position = model.Position{X: m.Pos.X + 6, Y: m.Pos.Y}
	inicio := m.Pos

	agora := h.now()
	h.dungeonMobTick(context.Background(), in, agora)
	h.dungeonMobTick(context.Background(), in, agora.Add(time.Second))
	if m.Pos == inicio {
		t.Error("o mob nao andou")
	}
	if m.Pos == s.Position {
		t.Error("o mob chegou no jogador num passo: a origem sorteava um " +
			"vizinho do jogador e teleportava")
	}
}

func TestMobVoltaQuandoOAlvoSai(t *testing.T) {
	h, _ := handlerComDungeon(t, 1)
	s, e := naPortaDaDungeon(t, h, "Alfa", 100, 30)
	entraNaDungeon(t, h, s, e)
	in, _ := h.Dungeons.Of(100)
	m := in.Mobs()[0]
	s.Position = m.Pos
	agora := h.now()
	h.dungeonMobTick(context.Background(), in, agora)
	if m.AttackerCharIndex == 0 {
		t.Fatal("o mob nao pegou alvo")
	}
	// O jogador sai da instancia.
	h.leaveDungeon(context.Background(), e.conn, s)
	h.dungeonMobTick(context.Background(), in, agora.Add(time.Second))
	// Sem membro o tick nao faz nada e o mob fica como estava.
	if in.AliveCount() != 1 {
		t.Errorf("%d vivos", in.AliveCount())
	}
}

func TestTickSemRegistroNaoEstoura(t *testing.T) {
	h := testHandler()
	if n := h.DungeonTick(context.Background()); n != 0 {
		t.Errorf("= %d", n)
	}
	h.dungeonOnDisconnect(nil)
}

func TestDungeonExigeEstarEmJogo(t *testing.T) {
	h, _ := handlerComDungeon(t, 2)
	s := sessaoAutenticada()
	c := connDescartavel(t)
	ctx := context.Background()
	if err := h.requestEnterDungeon(ctx, c, s,
		asPacket(t, enterDungeonPacket(0))); err != ErrNotPlaying {
		t.Errorf("entrar: %v", err)
	}
	if err := h.confirmDungeon(ctx, c, s,
		asPacket(t, confirmDungeonPacket(1))); err != ErrNotPlaying {
		t.Errorf("confirmar: %v", err)
	}
}

// entraNaDungeon leva a sessao para dentro, pelo caminho dos pacotes.
func entraNaDungeon(t *testing.T, h *Handler, s *Session, e *espiao) {
	t.Helper()
	if err := h.requestEnterDungeon(context.Background(), e.conn, s,
		asPacket(t, enterDungeonPacket(0))); err != nil {
		t.Fatal(err)
	}
	e.drena(100 * time.Millisecond)
	if err := h.confirmDungeon(context.Background(), e.conn, s,
		asPacket(t, confirmDungeonPacket(uint32(s.ClientID)))); err != nil {
		t.Fatal(err)
	}
	e.drena(100 * time.Millisecond)
	if _, ok := h.Dungeons.Of(s.CharID()); !ok {
		t.Fatal("a sessao nao entrou na dungeon")
	}
}
