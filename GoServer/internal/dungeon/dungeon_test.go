package dungeon

import (
	"sync"
	"testing"
	"time"

	"aika/internal/dungeondata"
	"aika/internal/model"
)

func defTeste(pontos int) *dungeondata.Definition {
	d := &dungeondata.Definition{
		Index:                  dungeondata.ZantorianCitadel,
		Name:                   "Teste",
		LevelMin:               20,
		EntranceNPCID:          2109,
		SpawnInDungeonPosition: model.Position{X: 3621, Y: 1},
	}
	sp := dungeondata.Species{
		Index: 0, IntName: 172, Name: "Bicho", HP: 100,
		AtkFis: 50, DefFis: 100, Exp: 180, DropIndex: 11,
	}
	for i := 0; i < pontos; i++ {
		sp.Spawns = append(sp.Spawns, dungeondata.Spawn{
			ClientID:    uint16(2600 + i),
			SecondIndex: i,
			HP:          100,
			MP:          100,
			InitPos:     model.Position{X: float32(3600 + i), Y: 1000},
		})
	}
	d.Species = []dungeondata.Species{sp}
	d.Drops[dungeondata.NoCrown] = []uint16{1001, 1002}
	return d
}

func TestAbrirInstancia(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, err := r.Open(defTeste(3), 7, []uint32{100, 200}, agora)
	if err != nil {
		t.Fatal(err)
	}
	if in.ID == 0 {
		t.Error("instancia sem id")
	}
	if in.MobCount() != 3 || in.AliveCount() != 3 {
		t.Errorf("%d mobs, %d vivos", in.MobCount(), in.AliveCount())
	}
	if in.PartyIndex != 7 {
		t.Errorf("grupo = %d", in.PartyIndex)
	}
	if !in.HasMember(100) || !in.HasMember(200) || in.HasMember(300) {
		t.Error("membros errados")
	}
	// Os dois membros apontam para a instancia.
	for _, c := range []uint32{100, 200} {
		got, ok := r.Of(c)
		if !ok || got != in {
			t.Errorf("Of(%d) nao achou a instancia", c)
		}
	}
	// O slot comeca em 1, como o vetor [1..450] da origem.
	if in.Mobs()[0].Slot != 1 {
		t.Errorf("primeiro slot = %d, queria 1", in.Mobs()[0].Slot)
	}
}

// A origem tinha UMA instancia para o servidor inteiro.
func TestVariasInstanciasAoMesmoTempo(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	for i := uint32(1); i <= 20; i++ {
		if _, err := r.Open(defTeste(2), uint16(i), []uint32{i * 10}, agora); err != nil {
			t.Fatalf("instancia %d: %v", i, err)
		}
	}
	if r.Count() != 20 {
		t.Errorf("%d instancias, queria 20", r.Count())
	}
	// Cada uma com os seus proprios mobs.
	in1, _ := r.Of(10)
	in2, _ := r.Of(20)
	if in1 == in2 {
		t.Fatal("dois grupos na mesma instancia")
	}
	m1 := in1.Mobs()[0]
	in1.Hit(m1, 1000, 10, agora)
	if !m1.Dead {
		t.Fatal("o mob deveria ter morrido")
	}
	if in2.AliveCount() != 2 {
		t.Errorf("a instancia do outro grupo perdeu mob: %d vivos", in2.AliveCount())
	}
}

func TestNaoEntraEmDuasInstancias(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	if _, err := r.Open(defTeste(1), 1, []uint32{100}, agora); err != nil {
		t.Fatal(err)
	}
	_, err := r.Open(defTeste(1), 2, []uint32{100, 200}, agora)
	if err != ErrBusy {
		t.Errorf("erro = %v, queria ErrBusy", err)
	}
	// E o segundo membro nao ficou preso em nada.
	if _, ok := r.Of(200); ok {
		t.Error("o membro 200 ficou numa instancia que nao abriu")
	}
	if r.Count() != 1 {
		t.Errorf("%d instancias", r.Count())
	}
}

func TestInstanciaSemMembroERecusada(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Open(defTeste(1), 0, nil, time.Now()); err == nil {
		t.Error("deveria recusar instancia sem membro")
	}
	if _, err := r.Open(nil, 0, []uint32{1}, time.Now()); err != ErrNoDungeon {
		t.Error("deveria recusar definicao nil")
	}
}

func TestSairFechaQuandoEsvazia(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(1), 1, []uint32{100, 200}, agora)

	got, fechou := r.Leave(100)
	if got != in || fechou {
		t.Errorf("primeira saida: fechou = %v", fechou)
	}
	if in.Closed() {
		t.Error("a instancia nao deveria ter fechado com um membro dentro")
	}
	got, fechou = r.Leave(200)
	if got != in || !fechou {
		t.Errorf("segunda saida: fechou = %v", fechou)
	}
	if !in.Closed() {
		t.Error("a instancia deveria ter fechado")
	}
	if r.Count() != 0 {
		t.Errorf("%d instancias abertas", r.Count())
	}
	if _, ok := r.Of(100); ok {
		t.Error("o membro continua apontando para a instancia")
	}
}

func TestSairDeQuemNaoEstaEmInstancia(t *testing.T) {
	r := NewRegistry()
	if in, fechou := r.Leave(999); in != nil || fechou {
		t.Errorf("= %v, %v", in, fechou)
	}
}

func TestFecharSoltaTodos(t *testing.T) {
	r := NewRegistry()
	in, _ := r.Open(defTeste(1), 1, []uint32{100, 200, 300}, time.Now())
	membros := r.Close(in)
	if len(membros) != 3 {
		t.Errorf("%d membros devolvidos", len(membros))
	}
	for _, c := range []uint32{100, 200, 300} {
		if _, ok := r.Of(c); ok {
			t.Errorf("o membro %d continua na instancia", c)
		}
	}
	if !in.Closed() || r.Count() != 0 {
		t.Error("a instancia nao fechou")
	}
	if r.Close(nil) != nil {
		t.Error("Close(nil) deveria devolver nil")
	}
}

// Dois golpes simultaneos nao podem matar o mesmo mob duas vezes.
func TestGolpeConcorrenteMataUmaVez(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(1), 1, []uint32{100, 200}, agora)
	m := in.Mobs()[0]

	var wg sync.WaitGroup
	mortes := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mortes <- in.Hit(m, 1000, uint32(100+n), agora)
		}(i)
	}
	wg.Wait()
	close(mortes)
	total := 0
	for matou := range mortes {
		if matou {
			total++
		}
	}
	if total != 1 {
		t.Errorf("%d golpes mataram o mob, queria 1", total)
	}
	if in.AliveCount() != 0 {
		t.Errorf("%d vivos", in.AliveCount())
	}
}

func TestDanoParcialEMorte(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(1), 1, []uint32{100}, agora)
	m := in.Mobs()[0]
	if m.MaxHP != 100 || m.CurHP != 100 {
		t.Fatalf("hp = %d/%d", m.CurHP, m.MaxHP)
	}
	if in.Hit(m, 30, 100, agora) {
		t.Error("30 de dano nao deveria matar")
	}
	if m.CurHP != 70 {
		t.Errorf("hp = %d", m.CurHP)
	}
	if m.FirstAttacker != 100 {
		t.Errorf("primeiro atacante = %d", m.FirstAttacker)
	}
	// Outro jogador bate: o primeiro atacante nao muda.
	in.Hit(m, 10, 200, agora)
	if m.FirstAttacker != 100 || m.AttackerCharIndex != 200 {
		t.Errorf("primeiro = %d, atual = %d", m.FirstAttacker, m.AttackerCharIndex)
	}
	// Dano exato no HP mata.
	if !in.Hit(m, 60, 200, agora) {
		t.Error("dano igual ao HP deveria matar")
	}
	if m.CurHP != 0 || !m.Dead {
		t.Errorf("hp = %d, morto = %v", m.CurHP, m.Dead)
	}
	// Bater em morto nao faz nada.
	if in.Hit(m, 10, 100, agora) {
		t.Error("bater em morto nao deveria matar de novo")
	}
	if in.Hit(nil, 10, 100, agora) {
		t.Error("Hit(nil) deveria ser inofensivo")
	}
}

func TestLimpaEPrazo(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(2), 1, []uint32{100}, agora)
	if in.Cleared() {
		t.Error("instancia nova nao esta limpa")
	}
	for _, m := range in.Mobs() {
		in.Hit(m, 1000, 100, agora)
	}
	if !in.Cleared() {
		t.Error("deveria estar limpa")
	}
	if in.Expired(agora) {
		t.Error("nao deveria ter vencido")
	}
	if !in.Expired(agora.Add(MaxLifetime)) {
		t.Error("deveria ter vencido")
	}
	// Instancia sem mob nenhum nao conta como limpa.
	vazia, _ := r.Open(defTeste(0), 1, []uint32{300}, agora)
	if vazia.Cleared() {
		t.Error("instancia sem mob nao deveria contar como limpa")
	}
}

func TestAcharMobPeloIdDoCliente(t *testing.T) {
	r := NewRegistry()
	in, _ := r.Open(defTeste(3), 1, []uint32{100}, time.Now())
	m, ok := in.Mob(2601)
	if !ok || m.ClientID != 2601 {
		t.Errorf("= %v, %v", m, ok)
	}
	// Id que o cliente pode mandar e nao existe.
	for _, id := range []uint16{0, 1, 2599, 2700, 65535} {
		if m, ok := in.Mob(id); ok {
			t.Errorf("id %d achou o mob %d", id, m.ClientID)
		}
	}
}

// Dois pontos com o mesmo id de cliente: so um entra.
func TestIdRepetidoNaoDuplicaMob(t *testing.T) {
	d := defTeste(2)
	d.Species[0].Spawns[1].ClientID = d.Species[0].Spawns[0].ClientID
	r := NewRegistry()
	in, _ := r.Open(d, 1, []uint32{100}, time.Now())
	if in.MobCount() != 1 {
		t.Errorf("%d mobs, queria 1", in.MobCount())
	}
}

// O ponto sem HP herda o HP da especie.
func TestPontoSemHpUsaOdaEspecie(t *testing.T) {
	d := defTeste(1)
	d.Species[0].Spawns[0].HP = 0
	d.Species[0].HP = 777
	r := NewRegistry()
	in, _ := r.Open(d, 1, []uint32{100}, time.Now())
	if m := in.Mobs()[0]; m.MaxHP != 777 || m.CurHP != 777 {
		t.Errorf("hp = %d/%d, queria 777", m.CurHP, m.MaxHP)
	}
}

func TestAlvoEIntervaloDeGolpe(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(1), 1, []uint32{100}, agora)
	m := in.Mobs()[0]
	pos := m.Pos

	in.SetTarget(m, 100, agora)
	if m.AttackerCharIndex != 100 {
		t.Errorf("alvo = %d", m.AttackerCharIndex)
	}
	if m.OrigPos != pos {
		t.Errorf("OrigPos = %+v, queria %+v", m.OrigPos, pos)
	}
	// O primeiro golpe passa, o seguinte no mesmo instante nao.
	if !in.MarkAttack(m, agora.Add(AttackInterval)) {
		t.Error("o primeiro golpe deveria passar")
	}
	if in.MarkAttack(m, agora.Add(AttackInterval)) {
		t.Error("dois golpes no mesmo instante")
	}
	if !in.MarkAttack(m, agora.Add(2*AttackInterval)) {
		t.Error("depois do intervalo deveria passar")
	}
	// Mob morto nao bate e nao pega alvo.
	in.Hit(m, 1000, 100, agora)
	if in.MarkAttack(m, agora.Add(10*AttackInterval)) {
		t.Error("mob morto nao bate")
	}
	in.SetTarget(m, 200, agora)
	if m.AttackerCharIndex == 200 {
		t.Error("mob morto nao pega alvo")
	}
	in.SetTarget(nil, 1, agora)
	in.MoveMob(nil, pos, agora)
	if in.MarkAttack(nil, agora) {
		t.Error("MarkAttack(nil) deveria ser falso")
	}
}

func TestMoverMob(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	in, _ := r.Open(defTeste(1), 1, []uint32{100}, agora)
	m := in.Mobs()[0]
	nova := model.Position{X: 1, Y: 2}
	in.MoveMob(m, nova, agora.Add(time.Second))
	if m.Pos != nova {
		t.Errorf("pos = %+v", m.Pos)
	}
	if !m.LastMove.After(agora) {
		t.Error("LastMove nao andou")
	}
}

func TestAliveDeMobNil(t *testing.T) {
	var m *Mob
	if m.Alive() {
		t.Error("mob nil nao esta vivo")
	}
}

func TestTodasAsInstancias(t *testing.T) {
	r := NewRegistry()
	agora := time.Now()
	r.Open(defTeste(1), 1, []uint32{100}, agora)
	r.Open(defTeste(1), 2, []uint32{200}, agora)
	if len(r.All()) != 2 {
		t.Errorf("%d instancias", len(r.All()))
	}
}

// ---- saguao

func TestSaguaoCompletoComTodosConfirmando(t *testing.T) {
	agora := time.Now()
	l := NewLobby(1, 0, 7, 100, []uint32{100, 200, 300}, agora)
	if l.Confirmed() != 0 {
		t.Error("saguao novo com confirmacao")
	}
	if completo, ok := l.Confirm(100); completo || !ok {
		t.Errorf("1a: completo=%v ok=%v", completo, ok)
	}
	if completo, ok := l.Confirm(200); completo || !ok {
		t.Errorf("2a: completo=%v ok=%v", completo, ok)
	}
	if completo, ok := l.Confirm(300); !completo || !ok {
		t.Errorf("3a: completo=%v ok=%v", completo, ok)
	}
}

// A origem creditava a confirmacao ao indice que vinha no cabecalho do pacote:
// um jogador sozinho confirmava por todos.
func TestSaguaoSoAceitaConfirmacaoDoProprio(t *testing.T) {
	agora := time.Now()
	l := NewLobby(1, 0, 7, 100, []uint32{100, 200, 300}, agora)
	// O jogador 100 confirma tres vezes: conta uma.
	for i := 0; i < 3; i++ {
		if completo, _ := l.Confirm(100); completo {
			t.Fatal("o saguao completou com um jogador so")
		}
	}
	if l.Confirmed() != 1 {
		t.Errorf("%d confirmacoes, queria 1", l.Confirmed())
	}
	// Quem nao esta no saguao e ignorado.
	if completo, ok := l.Confirm(999); completo || ok {
		t.Errorf("estranho confirmou: %v %v", completo, ok)
	}
	if l.Confirmed() != 1 {
		t.Errorf("%d confirmacoes", l.Confirmed())
	}
}

func TestSaguaoRecusado(t *testing.T) {
	agora := time.Now()
	l := NewLobby(1, 0, 7, 100, []uint32{100, 200}, agora)
	l.Confirm(100)
	l.Cancel(200)
	quem, cancelado := l.Cancelled()
	if !cancelado || quem != 200 {
		t.Errorf("= %d, %v", quem, cancelado)
	}
	// Depois de recusado ninguem mais confirma.
	if completo, ok := l.Confirm(200); completo || ok {
		t.Errorf("confirmou depois de recusado: %v %v", completo, ok)
	}
}

func TestSaguaoPrazo(t *testing.T) {
	agora := time.Now()
	l := NewLobby(1, 0, 0, 100, []uint32{100}, agora)
	if l.Expired(agora) {
		t.Error("saguao novo nao venceu")
	}
	if !l.Expired(agora.Add(LobbyTimeout)) {
		t.Error("deveria ter vencido")
	}
}

func TestRegistroDeSaguao(t *testing.T) {
	agora := time.Now()
	ls := NewLobbies()
	l := NewLobby(1, 0, 7, 100, []uint32{100, 200}, agora)
	ls.Open(l)
	if ls.Count() != 2 {
		t.Errorf("%d no registro", ls.Count())
	}
	for _, c := range []uint32{100, 200} {
		got, ok := ls.Of(c)
		if !ok || got != l {
			t.Errorf("Of(%d)", c)
		}
	}
	if !l.Waits(100) || l.Waits(999) {
		t.Error("Waits errado")
	}
	ls.DropChar(100)
	if _, ok := ls.Of(100); ok {
		t.Error("100 deveria ter saido")
	}
	if _, ok := ls.Of(200); !ok {
		t.Error("200 deveria ter ficado")
	}
	ls.Drop(l)
	if ls.Count() != 0 {
		t.Errorf("%d no registro depois do Drop", ls.Count())
	}
	ls.Open(nil)
	ls.Drop(nil)
}

func TestSaguoesVencidosSemRepetir(t *testing.T) {
	agora := time.Now()
	ls := NewLobbies()
	velho := NewLobby(1, 0, 7, 100, []uint32{100, 200, 300}, agora.Add(-2*LobbyTimeout))
	novo := NewLobby(1, 0, 8, 400, []uint32{400}, agora)
	ls.Open(velho)
	ls.Open(novo)
	venc := ls.Expired(agora)
	if len(venc) != 1 || venc[0] != velho {
		t.Fatalf("%d vencidos", len(venc))
	}
}

// Abrir um saguao novo substitui o anterior dos mesmos membros: a origem
// guardava as confirmacoes no registro do grupo e um saguao novo herdava as
// confirmacoes do anterior.
func TestSaguaoNovoSubstituiOAnterior(t *testing.T) {
	agora := time.Now()
	ls := NewLobbies()
	primeiro := NewLobby(1, 0, 7, 100, []uint32{100, 200}, agora)
	ls.Open(primeiro)
	primeiro.Confirm(100)
	primeiro.Confirm(200)

	segundo := NewLobby(2, 1, 7, 100, []uint32{100, 200}, agora)
	ls.Open(segundo)
	got, _ := ls.Of(100)
	if got != segundo {
		t.Fatal("o saguao nao foi substituido")
	}
	if got.Confirmed() != 0 {
		t.Errorf("%d confirmacoes herdadas", got.Confirmed())
	}
}

func TestMembrosDoSaguao(t *testing.T) {
	l := NewLobby(1, 0, 7, 100, []uint32{100, 200}, time.Now())
	m := l.Members()
	if len(m) != 2 {
		t.Fatalf("%d membros", len(m))
	}
	// A copia nao mexe no original.
	m[0] = 999
	if l.Members()[0] != 100 {
		t.Error("Members devolveu o vetor interno")
	}
}
