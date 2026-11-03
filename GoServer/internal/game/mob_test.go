package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// tabelaDeMobs monta uma mobdata.Table a mao.
func tabelaDeMobs(especies ...mobdata.Species) *mobdata.Table {
	t := &mobdata.Table{
		ByName:     map[string]int{},
		ByClientID: map[uint16]mobdata.Ref{},
	}
	id := uint16(mobdata.FirstMobClientID)
	for i := range especies {
		sp := especies[i]
		sp.Index = i
		for j := range sp.Spawns {
			sp.Spawns[j].ClientID = id
			sp.Spawns[j].SecondIndex = j + 1
			t.ByClientID[id] = mobdata.Ref{Species: i, Spawn: j}
			id++
		}
		t.Species = append(t.Species, sp)
		t.ByName[sp.Name] = i
	}
	return t
}

// especie devolve uma especie simples com n pontos de spawn em volta de (x,y).
func especie(nome string, hp uint32, n int, x, y float32) mobdata.Species {
	sp := mobdata.Species{
		Name: nome, IntName: 7, HP: hp, Level: 10,
		AtkFis: 100, DefFis: 80, Exp: 500,
		RespawnSeconds: 30, ActiveToSpawn: true, MoveSpeed: 22,
	}
	for i := 0; i < n; i++ {
		sp.Spawns = append(sp.Spawns, mobdata.Spawn{
			InitPos: model.Position{X: x + float32(i), Y: y},
			DestPos: model.Position{X: x + float32(i) + 5, Y: y + 5},
		})
	}
	return sp
}

// handlerComMobs devolve um handler com mundo de mobs e relogio controlado.
func handlerComMobs(t *testing.T, tab *mobdata.Table) (*Handler, *time.Time) {
	h, _, agora := handlerDeCombate(t)
	h.Mobs = NewMobWorld(tab)
	h.Exp = expFixa{}
	h.ExpMultiplier = 1
	return h, agora
}

// expFixa e uma tabela de experiencia simples: 1000 por nivel.
type expFixa struct{}

func (expFixa) ExpForLevel(level uint16) (uint64, bool) {
	if level > LevelCap {
		return 0, false
	}
	return uint64(level) * 1000, true
}

// ---- mundo de mobs

// So entram as especies marcadas para spawn, como na origem.
func TestMundoSoAceitaEspeciesAtivas(t *testing.T) {
	ativa := especie("Ativa", 100, 2, 1000, 1000)
	inativa := especie("Inativa", 100, 3, 1000, 1000)
	inativa.ActiveToSpawn = false

	w := NewMobWorld(tabelaDeMobs(ativa, inativa))
	if w.Count() != 2 {
		t.Errorf("%d mobs, queria 2", w.Count())
	}
}

// Ponto perto da origem do mapa nao entra: a origem tinha
// "if((CurrentPos.X < 50) or (CurrentPos.Y < 50)) then Exit".
func TestMundoRecusaPontoPertoDaOrigem(t *testing.T) {
	sp := especie("Bicho", 100, 0, 0, 0)
	sp.Spawns = []mobdata.Spawn{
		{InitPos: model.Position{X: 10, Y: 1000}},
		{InitPos: model.Position{X: 1000, Y: 10}},
		{InitPos: model.Position{X: 1000, Y: 1000}},
	}
	w := NewMobWorld(tabelaDeMobs(sp))
	if w.Count() != 1 {
		t.Errorf("%d mobs, queria 1", w.Count())
	}
}

// Posicao invalida (NaN, fora do mapa) nao entra.
func TestMundoRecusaPosicaoInvalida(t *testing.T) {
	sp := especie("Bicho", 100, 0, 0, 0)
	sp.Spawns = []mobdata.Spawn{
		{InitPos: model.Position{X: 99999, Y: 1000}},
		{InitPos: model.Position{X: 1000, Y: 1000}},
	}
	w := NewMobWorld(tabelaDeMobs(sp))
	if w.Count() != 1 {
		t.Errorf("%d mobs, queria 1", w.Count())
	}
}

func TestMundoVazioNaoQuebra(t *testing.T) {
	w := NewMobWorld(nil)
	if w.Count() != 0 {
		t.Error("mundo de tabela nil deveria estar vazio")
	}
	if _, ok := w.Get(3048); ok {
		t.Error("nao deveria achar nada")
	}
	if w.Near(model.Position{}, 50) != nil {
		t.Error("Near deveria devolver nil")
	}
	if w.Respawn(time.Now()) != nil {
		t.Error("Respawn deveria devolver nil")
	}
	var nulo *MobWorld
	if nulo.Count() != 0 || nulo.SpeciesOf(nil) != nil || nulo.Table() != nil {
		t.Error("MobWorld nil deveria responder vazio")
	}
}

func TestNearSoTrazVivosEPerto(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Bicho", 100, 3, 1000, 1000)))
	perto := w.Near(model.Position{X: 1000, Y: 1000}, 10)
	if len(perto) != 3 {
		t.Fatalf("%d perto, queria 3", len(perto))
	}
	// Mata um: ele sai da lista.
	w.Hit(perto[0], 1000, 1, time.Now())
	if n := len(w.Near(model.Position{X: 1000, Y: 1000}, 10)); n != 2 {
		t.Errorf("%d perto depois da morte, queria 2", n)
	}
	// Longe nao traz ninguem.
	if n := len(w.Near(model.Position{X: 5000, Y: 5000}, 10)); n != 0 {
		t.Errorf("%d perto de longe, queria 0", n)
	}
}

// Hit marca quem bateu primeiro e quem bateu por ultimo.
func TestHitGuardaOsAtacantes(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	m, _ := w.Get(mobdata.FirstMobClientID)

	if morreu := w.Hit(m, 10, 5, time.Now()); morreu {
		t.Error("nao deveria ter morrido")
	}
	if m.FirstAttackerID != 5 || m.AttackerID != 5 || !m.Attacked {
		t.Errorf("atacantes = %d/%d", m.FirstAttackerID, m.AttackerID)
	}
	w.Hit(m, 10, 9, time.Now())
	if m.FirstAttackerID != 5 {
		t.Errorf("o primeiro atacante mudou para %d", m.FirstAttackerID)
	}
	if m.AttackerID != 9 {
		t.Errorf("o ultimo atacante = %d, queria 9", m.AttackerID)
	}
	if m.CurHP != 80 {
		t.Errorf("HP = %d, queria 80", m.CurHP)
	}
}

// O golpe fatal mata uma vez so: na origem "Dec(Self.HP, Dano)" sem trava
// deixava dois golpes simultaneos matarem o mob duas vezes, e a segunda morte
// dava experiencia de novo.
func TestMobMorreUmaVezSo(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	m, _ := w.Get(mobdata.FirstMobClientID)

	if !w.Hit(m, 100, 1, time.Now()) {
		t.Fatal("deveria ter morrido")
	}
	if w.Hit(m, 100, 2, time.Now()) {
		t.Error("nao pode morrer duas vezes")
	}
	if m.CurHP != 0 || !m.Dead {
		t.Errorf("HP = %d, morto = %v", m.CurHP, m.Dead)
	}
}

// Dois atacantes ao mesmo tempo: a soma dos danos nao se perde e so um ganha
// a morte.
func TestHitConcorrente(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Tanque", 100000, 1, 1000, 1000)))
	m, _ := w.Get(mobdata.FirstMobClientID)

	const goroutines, golpes = 20, 50
	pronto := make(chan int, goroutines)
	for g := range goroutines {
		go func(id int) {
			mortes := 0
			for range golpes {
				if w.Hit(m, 10, uint16(id+1), time.Now()) {
					mortes++
				}
			}
			pronto <- mortes
		}(g)
	}
	total := 0
	for range goroutines {
		total += <-pronto
	}
	if total > 1 {
		t.Errorf("%d mortes, no maximo 1", total)
	}
	esperado := uint32(100000 - goroutines*golpes*10)
	if m.CurHP != esperado {
		t.Errorf("HP = %d, queria %d: algum golpe se perdeu", m.CurHP, esperado)
	}
}

// Dano zero ou negativo nao mexe no mob.
func TestHitComDanoNaoPositivo(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	m, _ := w.Get(mobdata.FirstMobClientID)
	for _, d := range []int{0, -1, -1000} {
		if w.Hit(m, d, 1, time.Now()) {
			t.Errorf("dano %d nao deveria matar", d)
		}
	}
	if m.CurHP != 100 || m.Attacked {
		t.Errorf("mob mexido: HP %d, atacado %v", m.CurHP, m.Attacked)
	}
	if w.Hit(nil, 10, 1, time.Now()) {
		t.Error("mob nil nao deveria matar")
	}
}

// ---- respawn

func TestRespawnDevolveNoTempo(t *testing.T) {
	sp := especie("Bicho", 100, 1, 1000, 1000)
	sp.RespawnSeconds = 30
	w := NewMobWorld(tabelaDeMobs(sp))
	m, _ := w.Get(mobdata.FirstMobClientID)

	t0 := time.Now()
	w.Hit(m, 100, 1, t0)
	if !m.Dead {
		t.Fatal("deveria estar morto")
	}

	// Antes do tempo nao volta.
	if n := len(w.Respawn(t0.Add(29 * time.Second))); n != 0 {
		t.Errorf("%d voltaram antes do tempo", n)
	}
	// Depois volta, com a vida cheia e na posicao inicial.
	voltaram := w.Respawn(t0.Add(31 * time.Second))
	if len(voltaram) != 1 {
		t.Fatalf("%d voltaram", len(voltaram))
	}
	if m.Dead || m.CurHP != sp.HP {
		t.Errorf("morto = %v, HP = %d", m.Dead, m.CurHP)
	}
	if m.Attacked || m.AttackerID != 0 || m.FirstAttackerID != 0 {
		t.Error("os atacantes deveriam ter sido esquecidos")
	}
	if m.Position != sp.Spawns[0].InitPos {
		t.Errorf("posicao = %v, queria %v", m.Position, sp.Spawns[0].InitPos)
	}
}

// Respawn zero no arquivo nao faz o mob voltar no mesmo instante da morte.
func TestRespawnZeroTemPisoDeUmSegundo(t *testing.T) {
	sp := especie("Bicho", 100, 1, 1000, 1000)
	sp.RespawnSeconds = 0
	w := NewMobWorld(tabelaDeMobs(sp))
	m, _ := w.Get(mobdata.FirstMobClientID)

	t0 := time.Now()
	w.Hit(m, 100, 1, t0)
	if n := len(w.Respawn(t0)); n != 0 {
		t.Error("nao deveria voltar no mesmo instante")
	}
	if n := len(w.Respawn(t0.Add(2 * time.Second))); n != 1 {
		t.Error("deveria voltar depois de um segundo")
	}
}

// O bug da virada de ano: o teste da origem era
// "if (Self.DeadTime.Year >= Now.Year)", entao um mob morto em 31 de dezembro
// nunca voltava depois da meia-noite.
func TestRespawnFuncionaNaViradaDeAno(t *testing.T) {
	sp := especie("Bicho", 100, 1, 1000, 1000)
	sp.RespawnSeconds = 30
	w := NewMobWorld(tabelaDeMobs(sp))
	m, _ := w.Get(mobdata.FirstMobClientID)

	reveillon := time.Date(2026, 12, 31, 23, 59, 50, 0, time.UTC)
	w.Hit(m, 100, 1, reveillon)
	anoNovo := reveillon.Add(40 * time.Second) // ja e 2027
	if n := len(w.Respawn(anoNovo)); n != 1 {
		t.Error("o mob deveria voltar depois da virada de ano")
	}
}

// Mob vivo nao entra no respawn.
func TestRespawnIgnoraVivo(t *testing.T) {
	w := NewMobWorld(tabelaDeMobs(especie("Bicho", 100, 3, 1000, 1000)))
	if n := len(w.Respawn(time.Now().Add(time.Hour))); n != 0 {
		t.Errorf("%d voltaram e nenhum tinha morrido", n)
	}
}

// ---- visibilidade

func TestJogadorVeMobPerto(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	h.refreshVisibleMobs(s)
	if !s.Base.VisibleMobs.Contains(mobdata.FirstMobClientID) {
		t.Error("deveria estar vendo o mob")
	}
	if got := e.espera(t); got != CodeSpawnMob {
		t.Errorf("recebeu %#x, queria %#x", got, CodeSpawnMob)
	}
}

func TestJogadorDeixaDeVerMobLonge(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	h.refreshVisibleMobs(s)
	e.espera(t) // o spawn

	s.Position = model.Position{X: 5000, Y: 5000}
	h.refreshVisibleMobs(s)
	if s.Base.VisibleMobs.Contains(mobdata.FirstMobClientID) {
		t.Error("nao deveria mais ver o mob")
	}
	if got := e.espera(t); got != CodeRemoveMob {
		t.Errorf("recebeu %#x, queria %#x", got, CodeRemoveMob)
	}
}

// Mob visto duas vezes nao gera dois spawns.
func TestMobVisivelNaoRepeteSpawn(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	h.refreshVisibleMobs(s)
	e.espera(t)
	h.refreshVisibleMobs(s)
	h.refreshVisibleMobs(s)
	e.nada(t, 150*time.Millisecond)
}

// Morto nao recebe spawn de mob.
func TestJogadorMortoNaoVeMob(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Dead = true
	h.Chan.Join(e.conn, s)

	h.refreshVisibleMobs(s)
	e.nada(t, 150*time.Millisecond)
}

// O pacote de spawn leva a vida atual no lugar certo: a origem trocava CurHP
// com MaxHP, e um mob machucado aparecia com a barra cheia.
func TestSpawnDeMobLevaAVidaAtual(t *testing.T) {
	tab := tabelaDeMobs(especie("Bicho", 1000, 1, 1000, 1000))
	h, _ := handlerComMobs(t, tab)
	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 600, 1, time.Now())

	pkt := h.buildSpawnMob(m, h.Mobs.SpeciesOf(m))
	if pkt.CurHP != 400 {
		t.Errorf("CurHP = %d, queria 400", pkt.CurHP)
	}
	if pkt.MaxHP != 1000 {
		t.Errorf("MaxHP = %d, queria 1000", pkt.MaxHP)
	}
	if pkt.Header.Index != m.ClientID {
		t.Errorf("Index = %d, queria %d", pkt.Header.Index, m.ClientID)
	}
	if pkt.MobName != 7 {
		t.Errorf("MobName = %d, queria 7", pkt.MobName)
	}
}

// ---- combate contra mob

func TestGolpeNoMobTiraVida(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100000, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	antes := m.CurHP
	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if m.CurHP >= antes {
		t.Errorf("vida do mob = %d, antes %d", m.CurHP, antes)
	}
	if got := e.espera(t); got != CodeRecvDamage {
		t.Errorf("recebeu %#x, queria %#x", got, CodeRecvDamage)
	}
}

// Mob longe nao e atingido.
func TestGolpeNoMobExigeDistancia(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100000, 1, 5000, 5000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	antes := m.CurHP
	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	if m.CurHP != antes {
		t.Error("mob longe nao deveria ter levado dano")
	}
}

// Id de mob que nao existe nao quebra.
func TestGolpeEmMobInexistente(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	for _, id := range []uint16{3047, 3100, 9000, 65535} {
		if err := h.attack(context.Background(), e.conn, s,
			asPacket(t, atkPacket(id, 0))); err != nil {
			t.Fatalf("id %d: %v", id, err)
		}
	}
}

// Matar o mob da experiencia e apaga o mob da tela.
func TestMatarMobDaExperiencia(t *testing.T) {
	sp := especie("Bicho", 1, 1, 1000, 1000)
	sp.Exp = 777
	h, _ := handlerComMobs(t, tabelaDeMobs(sp))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 50
	s.Avatar.Exp = 0
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	if !m.Dead {
		t.Fatal("o mob deveria estar morto")
	}
	if s.Avatar.Exp != 777 {
		t.Errorf("experiencia = %d, queria 777", s.Avatar.Exp)
	}
	if s.Base.VisibleMobs.Contains(mobdata.FirstMobClientID) {
		t.Error("o mob morto deveria ter saido da lista")
	}
}

// O multiplicador do .ini vale.
func TestMultiplicadorDeExperiencia(t *testing.T) {
	sp := especie("Bicho", 1, 1, 1000, 1000)
	sp.Exp = 100
	h, _ := handlerComMobs(t, tabelaDeMobs(sp))
	h.ExpMultiplier = 3
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 50
	s.Avatar.Exp = 0
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0)))
	if s.Avatar.Exp != 300 {
		t.Errorf("experiencia = %d, queria 300", s.Avatar.Exp)
	}
}

// Multiplicador zero no .ini nao pode zerar a experiencia de todo mob.
func TestMultiplicadorZeroUsaUm(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	h.ExpMultiplier = 0
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)

	if got := h.addExp(e.conn, s, 50, ExpTypeMob); got != 50 {
		t.Errorf("ganho = %d, queria 50", got)
	}
}

// ---- RespawnTick

func TestRespawnTickDesenhaDeVolta(t *testing.T) {
	sp := especie("Bicho", 1, 1, 1000, 1000)
	sp.RespawnSeconds = 5
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(mobdata.FirstMobClientID)

	h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0)))
	// A morte manda dano, nivel e a remocao do mob; o teste espera so o
	// spawn de volta, entao a fila e drenada antes.
	e.drena(150 * time.Millisecond)

	if n := h.RespawnTick(); n != 0 {
		t.Errorf("%d voltaram antes do tempo", n)
	}
	*agora = agora.Add(10 * time.Second)
	if n := h.RespawnTick(); n != 1 {
		t.Fatalf("%d voltaram, queria 1", n)
	}
	if got := e.espera(t); got != CodeSpawnMob {
		t.Errorf("recebeu %#x, queria o spawn %#x", got, CodeSpawnMob)
	}
	if !s.Base.VisibleMobs.Contains(mobdata.FirstMobClientID) {
		t.Error("o mob de volta deveria estar visivel")
	}
}

// Sem mundo de mob o tick nao quebra.
func TestRespawnTickSemMobs(t *testing.T) {
	h := testHandler()
	if n := h.RespawnTick(); n != 0 {
		t.Errorf("= %d", n)
	}
}

// ---- experiencia e nivel

func TestSubirDeNivel(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 10
	s.Avatar.Exp = 0
	h.Chan.Join(e.conn, s)

	// A tabela do teste pede 1000 x nivel, e a origem sobe so quando a
	// experiencia PASSA do valor do nivel ("Exp > ExpList[Level]"), nao
	// quando empata.
	h.addExp(e.conn, s, 30000, ExpTypeNormal)
	if s.Avatar.Level <= 10 {
		t.Errorf("nivel = %d, deveria ter subido", s.Avatar.Level)
	}
	// Subir enche a vida.
	if s.Avatar.CurHP != s.Avatar.MaxHP {
		t.Errorf("HP = %d, maximo %d", s.Avatar.CurHP, s.Avatar.MaxHP)
	}
}

// O nivel para no teto.
func TestNivelParaNoTeto(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = LevelCap - 1
	s.Avatar.Exp = 0
	h.Chan.Join(e.conn, s)

	h.addExp(e.conn, s, 1<<40, ExpTypeNormal)
	if s.Avatar.Level != LevelCap {
		t.Errorf("nivel = %d, queria o teto %d", s.Avatar.Level, LevelCap)
	}
	// No teto a experiencia para no valor do ultimo nivel.
	quer, _ := expFixa{}.ExpForLevel(LevelCap)
	if s.Avatar.Exp != int64(quer) {
		t.Errorf("experiencia = %d, queria %d", s.Avatar.Exp, quer)
	}
}

// A classe avanca uma casa ao passar do nivel 50.
func TestClasseAvancaNoCinquentaEUm(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 50
	s.Avatar.ClassInfo = 1
	s.Avatar.Exp = 0
	h.Chan.Join(e.conn, s)

	h.addExp(e.conn, s, 60000, ExpTypeNormal)
	if s.Avatar.Level < 51 {
		t.Fatalf("nivel = %d", s.Avatar.Level)
	}
	if s.Avatar.ClassInfo != 2 {
		t.Errorf("ClassInfo = %d, queria 2", s.Avatar.ClassInfo)
	}
}

// Os pontos extras dos niveis 51, 61, 71 e 81.
func TestPontosExtrasNosNiveisChave(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 50
	s.Avatar.SkillPoint = 0
	s.Avatar.StatusPoints = 0

	h.addLevel(s) // 51
	if s.Avatar.SkillPoint != SkillPointPerLevel+SkillPointX {
		t.Errorf("skill = %d", s.Avatar.SkillPoint)
	}
	if s.Avatar.StatusPoints != StatusPointPerLevel+StatusPointX {
		t.Errorf("status = %d", s.Avatar.StatusPoints)
	}
}

// Os pontos nao dao a volta no WORD: a origem usava Inc.
func TestPontosNaoDaoAVolta(t *testing.T) {
	if got := addPoints(65535, 10); got != 65535 {
		t.Errorf("= %d, queria 65535", got)
	}
	if got := addPoints(0, -10); got != 0 {
		t.Errorf("= %d, queria 0", got)
	}
}

// Nivel fora da tabela nao le fora do vetor: a origem fazia ExpList[Level]
// sem checar o tamanho.
func TestNivelForaDaTabelaNaoSobe(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	h.Exp = expCurta{}
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 50
	h.Chan.Join(e.conn, s)

	h.addExp(e.conn, s, 1<<40, ExpTypeNormal)
	if s.Avatar.Level != 50 {
		t.Errorf("nivel = %d, nao deveria ter subido sem tabela", s.Avatar.Level)
	}
}

// expCurta so conhece os niveis ate 10.
type expCurta struct{}

func (expCurta) ExpForLevel(level uint16) (uint64, bool) {
	if level > 10 {
		return 0, false
	}
	return uint64(level) * 100, true
}

// Sem tabela nenhuma a experiencia soma e o nivel nao mexe.
func TestSemTabelaDeExperiencia(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	h.Exp = nil
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 10
	h.Chan.Join(e.conn, s)

	h.addExp(e.conn, s, 5000, ExpTypeNormal)
	if s.Avatar.Exp != 5000 {
		t.Errorf("experiencia = %d", s.Avatar.Exp)
	}
	if s.Avatar.Level != 10 {
		t.Errorf("nivel = %d", s.Avatar.Level)
	}
}

// O $108 leva "nivel - 1", com piso em zero.
func TestPacoteDeNivel(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.Level = 0
	h.Chan.Join(e.conn, s)

	h.sendRefreshLevel(e.conn, s)
	vis := e.proximo(t)
	if vis.code != CodeCurrentLevel {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.SendCurrentLevel
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.Level != 0 {
		t.Errorf("nivel no pacote = %d, queria 0", pkt.Level)
	}
	s.Avatar.Level = 25
	h.sendRefreshLevel(e.conn, s)
	vis = e.proximo(t)
	netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt)
	if pkt.Level != 24 {
		t.Errorf("nivel no pacote = %d, queria 24", pkt.Level)
	}
}

// Experiencia zero ou negativa nao mexe em nada.
func TestExperienciaNaoPositiva(t *testing.T) {
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Avatar.Exp = 100

	for _, v := range []int64{0, -1, -1000} {
		if got := h.addExp(e.conn, s, v, ExpTypeMob); got != 0 {
			t.Errorf("ganho de %d = %d, queria 0", v, got)
		}
	}
	if s.Avatar.Exp != 100 {
		t.Errorf("experiencia = %d, queria 100", s.Avatar.Exp)
	}
}
