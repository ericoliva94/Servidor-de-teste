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

// especiePatrulha devolve uma especie com um ponto que anda de (x,y) ate
// (x+dx, y), com as duas esperas dadas.
func especiePatrulha(nome string, x, y, dx float32, esperaIda, esperaVolta int) mobdata.Species {
	return mobdata.Species{
		Name: nome, IntName: 7, HP: 100000, Level: 10,
		AtkFis: 1000, DefFis: 50, Exp: 10,
		RespawnSeconds: 30, ActiveToSpawn: true, MoveSpeed: 22,
		Spawns: []mobdata.Spawn{{
			InitPos:      model.Position{X: x, Y: y},
			DestPos:      model.Position{X: x + dx, Y: y},
			FirstDestPos: model.Position{X: x + dx, Y: y},
			InitMoveWait: esperaIda,
			DestMoveWait: esperaVolta,
		}},
	}
}

// ---- passo por eixo

func TestStepTowardAndaSoQuandoPassaDoLimite(t *testing.T) {
	de := model.Position{X: 100, Y: 100}
	// Diferenca de 1 em X nao chega ao limite de 2: o eixo nao anda.
	got := stepToward(de, model.Position{X: 101, Y: 100}, 1.5, 2)
	if got.X != 100 {
		t.Errorf("X = %v, nao deveria ter andado", got.X)
	}
	// Diferenca de 2 anda 1.5 na direcao do destino.
	got = stepToward(de, model.Position{X: 102, Y: 100}, 1.5, 2)
	if got.X != 101.5 {
		t.Errorf("X = %v, queria 101.5", got.X)
	}
	// Para tras tambem.
	got = stepToward(de, model.Position{X: 90, Y: 100}, 1.5, 2)
	if got.X != 98.5 {
		t.Errorf("X = %v, queria 98.5", got.X)
	}
	// Os dois eixos ao mesmo tempo.
	got = stepToward(de, model.Position{X: 110, Y: 110}, 1.5, 2)
	if got.X != 101.5 || got.Y != 101.5 {
		t.Errorf("= %v", got)
	}
}

// Passo que levaria o mob para fora do mapa e descartado: a origem nao
// validava a posicao do mob em nenhum momento.
func TestStepTowardRecusaSairDoMapa(t *testing.T) {
	de := model.Position{X: MapLimit - 1, Y: 100}
	got := stepToward(de, model.Position{X: MapLimit + 1000, Y: 100}, 1.5, 2)
	if got != de {
		t.Errorf("= %v, deveria ter ficado em %v", got, de)
	}
	de2 := model.Position{X: 1, Y: 100}
	got = stepToward(de2, model.Position{X: -1000, Y: 100}, 1.5, 2)
	if got != de2 {
		t.Errorf("= %v, deveria ter ficado em %v", got, de2)
	}
}

func TestAbsF(t *testing.T) {
	if absF(-3.5) != 3.5 || absF(3.5) != 3.5 || absF(0) != 0 {
		t.Error("absF errado")
	}
}

// ---- patrulha

func TestMobPatrulhaAndaEEnviaPacote(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	inicio := m.Position

	// O pacote de movimento sai a cada dois passos.
	if h.mobMoveStep(m, *agora) {
		t.Error("o primeiro passo nao envia pacote")
	}
	if m.Position == inicio {
		t.Error("o mob deveria ter andado no primeiro passo")
	}
	if !h.mobMoveStep(m, *agora) {
		t.Error("o segundo passo deveria enviar o pacote")
	}
	vis := e.proximo(t)
	if vis.code != CodeMovement {
		t.Fatalf("recebeu %#x, queria %#x", vis.code, CodeMovement)
	}
	var pkt packets.MobMovimentPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.Header.Index != m.ClientID {
		t.Errorf("Index = %d, queria %d", pkt.Header.Index, m.ClientID)
	}
	// A velocidade do pacote e a da especie, nao o 25 que MobMove recebia.
	if pkt.Speed != 22 {
		t.Errorf("velocidade = %d, queria 22", pkt.Speed)
	}
}

// Chegando ao destino, o mob vira e volta.
func TestMobPatrulhaVolta(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 6, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	if m.State != MobAtInit {
		t.Fatalf("estado inicial = %d", m.State)
	}
	// Anda ate chegar; o passo e 1.5 e a distancia 6. O relogio avanca um
	// segundo por passo porque a espera entre passos e comparada com ">",
	// como na origem: com o relogio parado e espera zero o mob nao sai do
	// lugar depois de marcar a chegada.
	for range 20 {
		*agora = agora.Add(time.Second)
		h.mobMoveStep(m, *agora)
		if m.State == MobAtDest {
			break
		}
	}
	if m.State != MobAtDest {
		t.Fatalf("nao chegou ao destino: estado %d, posicao %v", m.State, m.Position)
	}
	if m.Position.X != 1006 {
		t.Errorf("posicao = %v, queria X 1006", m.Position)
	}
	// E volta.
	for range 20 {
		*agora = agora.Add(time.Second)
		h.mobMoveStep(m, *agora)
		if m.State == MobAtInit {
			break
		}
	}
	if m.State != MobAtInit {
		t.Errorf("nao voltou: estado %d, posicao %v", m.State, m.Position)
	}
	if m.Position.X != 1000 {
		t.Errorf("posicao de volta = %v, queria X 1000", m.Position)
	}
}

// A espera de cada sentido vem da sua coluna. A origem usava InitMoveWait nos
// dois sentidos e nunca lia DestMoveWait, que ela carregava do arquivo.
func TestEsperaDeCadaSentidoUsaASuaColuna(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 6, 1, 30))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	// Vai ate o destino; chegar marca LastMove.
	for range 20 {
		*agora = agora.Add(2 * time.Second)
		h.mobMoveStep(m, *agora)
		if m.State == MobAtDest {
			break
		}
	}
	if m.State != MobAtDest {
		t.Fatal("nao chegou ao destino")
	}
	pos := m.Position

	// A volta espera 30 s: passados 2 s ainda nao anda.
	*agora = agora.Add(2 * time.Second)
	h.mobMoveStep(m, *agora)
	if m.Position != pos {
		t.Errorf("andou antes da espera de volta: %v", m.Position)
	}
	// Passados 31 s, anda.
	*agora = agora.Add(31 * time.Second)
	h.mobMoveStep(m, *agora)
	if m.Position == pos {
		t.Error("deveria ter andado depois da espera")
	}
}

// Mutante nao anda.
func TestMutanteNaoAnda(t *testing.T) {
	sp := especiePatrulha("Lobo_Mutante", 1000, 1000, 20, 0, 0)
	sp.Mutant = true
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	pos := m.Position
	for range 5 {
		h.mobMoveStep(m, *agora)
	}
	if m.Position != pos {
		t.Errorf("mutante andou para %v", m.Position)
	}
}

// Ponto cujo destino e a propria posicao tambem fica parado.
func TestPontoSemDestinoNaoAnda(t *testing.T) {
	sp := especiePatrulha("Bicho", 1000, 1000, 0, 0, 0)
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	pos := m.Position
	h.mobMoveStep(m, *agora)
	if m.Position != pos {
		t.Errorf("andou para %v", m.Position)
	}
}

// Especie sem nome numerico nao mexe: "if(IntName = 0) then Exit" da origem.
func TestEspecieSemIntNameNaoMexe(t *testing.T) {
	sp := especiePatrulha("Bicho", 1000, 1000, 20, 0, 0)
	sp.IntName = 0
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	pos := m.Position
	if h.mobMoveStep(m, *agora) {
		t.Error("nao deveria ter andado")
	}
	if m.Position != pos {
		t.Errorf("andou para %v", m.Position)
	}
}

// ---- perseguicao

func TestMobPerseguAlvo(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1010, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)

	antes := m.Position.Distance(s.Position)
	if !h.mobMoveStep(m, *agora) {
		t.Fatal("deveria ter perseguido")
	}
	if depois := m.Position.Distance(s.Position); depois >= antes {
		t.Errorf("distancia = %d, antes %d: nao se aproximou", depois, antes)
	}
	// A perseguicao manda pacote a cada passo.
	if got := e.espera(t); got != CodeMovement {
		t.Errorf("recebeu %#x", got)
	}
}

// No alcance de bater o mob para de andar.
func TestMobNoAlcanceNaoAnda(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)
	pos := m.Position
	if h.mobMoveStep(m, *agora) {
		t.Error("nao deveria andar estando no alcance")
	}
	if m.Position != pos {
		t.Errorf("andou para %v", m.Position)
	}
}

// Alvo longe demais: o mob desiste e volta a patrulhar. A origem nao tinha
// esse limite e o mob perseguia sem fim.
func TestMobDesisteDeAlvoLonge(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)
	if !m.Attacked {
		t.Fatal("deveria estar irritado")
	}
	s.Position = model.Position{X: 2000, Y: 2000}
	h.mobMoveStep(m, *agora)
	if m.Attacked || m.AttackerID != 0 {
		t.Error("o mob deveria ter desistido do alvo")
	}
}

// Alvo que sai do canal nao prende o mob: na origem o
// "if(Players[AttackerID].Status < Playing) then Exit" so saia do
// procedimento e o mob ficava preso num alvo que foi embora.
func TestMobEsqueceAlvoQueSaiu(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	idAlvo := s.ClientID
	h.Mobs.Hit(m, 10, idAlvo, *agora)
	h.Chan.Leave(idAlvo)

	if h.mobAttackStep(m, *agora) {
		t.Error("nao deveria ter atacado")
	}
	if m.Attacked || m.AttackerID != 0 {
		t.Error("o mob deveria ter esquecido o alvo")
	}
}

// ---- ataque do mob

func TestMobAtacaOJogador(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	h.Rand = randFixo{v: 50} // cai no ramo "normal"
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)

	antes := s.Avatar.CurHP
	if !h.mobAttackStep(m, *agora) {
		t.Fatal("o mob deveria ter atacado")
	}
	if s.Avatar.CurHP >= antes {
		t.Errorf("vida = %d, antes %d", s.Avatar.CurHP, antes)
	}
	vis := e.proximo(t)
	if vis.code != CodeRecvDamage {
		t.Fatalf("recebeu %#x, queria %#x", vis.code, CodeRecvDamage)
	}
	var pkt packets.RecvDamagePacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.AttackerID != m.ClientID {
		t.Errorf("atacante no pacote = %d, queria o mob %d", pkt.AttackerID, m.ClientID)
	}
	if pkt.TargetID != s.ClientID {
		t.Errorf("alvo no pacote = %d, queria %d", pkt.TargetID, s.ClientID)
	}
}

// O intervalo de 3 s entre golpes do mob.
func TestMobRespeitaOIntervaloDeGolpe(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)

	if !h.mobAttackStep(m, *agora) {
		t.Fatal("o primeiro golpe deveria sair")
	}
	if h.mobAttackStep(m, *agora) {
		t.Error("o segundo golpe imediato nao deveria sair")
	}
	*agora = agora.Add(MobAttackCooldown + time.Second)
	if !h.mobAttackStep(m, *agora) {
		t.Error("passado o intervalo o golpe deveria sair")
	}
}

// Mob que nao foi atacado nao bate em ninguem.
func TestMobPacificoNaoAtaca(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	if h.mobAttackStep(m, *agora) {
		t.Error("mob que nao foi atacado nao deveria bater")
	}
	if s.Avatar.CurHP != s.Avatar.MaxHP {
		t.Error("o jogador nao deveria ter levado dano")
	}
}

// Mob longe nao alcanca o alvo.
func TestMobLongeNaoAlcanca(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1010, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)
	if h.mobAttackStep(m, *agora) {
		t.Error("mob a 10 celulas nao deveria alcancar")
	}
}

// Mob morto nao ataca.
func TestMobMortoNaoAtaca(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)

	m, ok := h.Mobs.Get(mobdata.FirstMobClientID)
	if !ok {
		t.Fatal("o mob deveria estar no mundo")
	}
	h.Mobs.Hit(m, 1<<20, s.ClientID, *agora)
	if !m.Dead {
		t.Fatal("deveria estar morto")
	}
	if h.mobAttackStep(m, *agora) {
		t.Error("mob morto nao deveria bater")
	}
}

// O golpe do mob pode matar o jogador, e o mob esquece o alvo.
func TestMobPodeMatarOJogador(t *testing.T) {
	sp := especiePatrulha("Bicho", 1000, 1000, 20, 0, 0)
	sp.AtkFis = 1000000
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	s.Avatar.CurHP = 10
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)
	h.mobAttackStep(m, *agora)

	if !s.Dead || s.Avatar.CurHP != 0 {
		t.Errorf("morto = %v, HP = %d", s.Dead, s.Avatar.CurHP)
	}
	if m.Attacked {
		t.Error("o mob deveria ter esquecido o alvo morto")
	}
}

// O buff 19 bloqueia o golpe e e consumido.
func TestBuffDeBloqueioZeraOGolpeDoMob(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)
	s.Base.Buffs.Set(19, *agora)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	h.Mobs.Hit(m, 10, s.ClientID, *agora)
	antes := s.Avatar.CurHP
	h.mobAttackStep(m, *agora)

	if s.Avatar.CurHP != antes {
		t.Errorf("vida = %d, antes %d: o bloqueio deveria zerar o golpe",
			s.Avatar.CurHP, antes)
	}
	if s.Base.Buffs.Has(19) {
		t.Error("o buff de bloqueio deveria ter sido consumido")
	}
}

// ---- tick do servidor

func TestMobTickSoMexeNoQueAlguemVe(t *testing.T) {
	// Dois mobs: um perto do jogador e um longe.
	perto := especiePatrulha("Perto", 1000, 1000, 20, 0, 0)
	longe := especiePatrulha("Longe", 9000, 9000, 20, 0, 0)
	h, agora := handlerComMobs(t, tabelaDeMobs(perto, longe))
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	ativos := h.activeMobs()
	if len(ativos) != 1 {
		t.Fatalf("%d mobs ativos, queria 1", len(ativos))
	}
	if ativos[0].ClientID != mobdata.FirstMobClientID {
		t.Errorf("mob ativo = %d", ativos[0].ClientID)
	}

	// O tick de ataque so mexe em quem foi atacado.
	if n, _ := h.MobTick(*agora); n != 0 {
		t.Errorf("%d atacaram sem ter sido atacados", n)
	}
	h.Mobs.Hit(ativos[0], 10, s.ClientID, *agora)
	h.Rand = randFixo{v: 50}
	if n, _ := h.MobTick(*agora); n != 1 {
		t.Errorf("%d atacaram, queria 1", n)
	}
}

func TestMobMoveTickAnda(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, agora := handlerComMobs(t, tab)
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)
	pos := m.Position
	h.MobMoveTickRun(*agora)
	if m.Position == pos {
		t.Error("o mob deveria ter andado no tick")
	}
}

// Sem mundo de mob ou sem canal os ticks nao quebram.
func TestTicksSemMundo(t *testing.T) {
	h := testHandler()
	if a, b := h.MobTick(time.Now()); a != 0 || b != 0 {
		t.Error("MobTick sem mundo deveria ser zero")
	}
	if n := h.MobMoveTickRun(time.Now()); n != 0 {
		t.Error("MobMoveTickRun sem mundo deveria ser zero")
	}
	h.Mobs = NewMobWorld(nil)
	if a, _ := h.MobTick(time.Now()); a != 0 {
		t.Error("MobTick sem canal deveria ser zero")
	}
}

// ---- mobTarget

// O mob nao recebe mensagem: na origem a linha equivalente era
// Players[mob.ClientID].SendClientMessage, com ClientID de mob acima de 1000
// num vetor de 1000.
func TestMobTargetNaoRecebeMensagem(t *testing.T) {
	tab := tabelaDeMobs(especiePatrulha("Bicho", 1000, 1000, 20, 0, 0))
	h, _ := handlerComMobs(t, tab)
	m, _ := h.Mobs.Get(mobdata.FirstMobClientID)

	alvo := &mobTarget{m: m, w: h.Mobs}
	alvo.Notify("nao deveria chegar a ninguem")
	if alvo.HasBuff(19) || alvo.Ability(1) != 0 {
		t.Error("mobTarget deveria responder vazio")
	}
	alvo.RemoveBuff(19)
	alvo.DecAbility(1, 1)

	// AddHP negativo tira vida do mob; positivo nao faz nada.
	antes := m.CurHP
	alvo.AddHP(100)
	if m.CurHP != antes {
		t.Errorf("AddHP positivo mexeu no mob: %d", m.CurHP)
	}
	alvo.AddHP(-500)
	if m.CurHP != antes-500 {
		t.Errorf("HP = %d, queria %d", m.CurHP, antes-500)
	}
}

// ---- integracao: o mob bate de volta depois de levar um golpe

func TestCicloCompletoDeBriga(t *testing.T) {
	sp := especiePatrulha("Bicho", 1000, 1000, 20, 0, 0)
	h, agora := handlerComMobs(t, tabelaDeMobs(sp))
	h.Rand = randFixo{v: 50}
	e := novoEspiao(t)
	s := lutador("Alfa", 1001, 1000)
	h.Chan.Join(e.conn, s)
	h.refreshVisibleMobs(s)
	e.drena(100 * time.Millisecond)

	// O jogador bate.
	if err := h.attack(context.Background(), e.conn, s,
		asPacket(t, atkPacket(mobdata.FirstMobClientID, 0))); err != nil {
		t.Fatal(err)
	}
	m, ok := h.Mobs.Get(mobdata.FirstMobClientID)
	if !ok {
		t.Fatal("o mob deveria estar no mundo")
	}
	if m.CurHP == m.MaxHP {
		t.Error("o mob deveria ter levado dano")
	}
	if m.AttackerID != s.ClientID {
		t.Errorf("o mob deveria ter marcado o atacante: %d", m.AttackerID)
	}

	// O mob bate de volta no tick.
	vidaAntes := s.Avatar.CurHP
	if n, _ := h.MobTick(*agora); n != 1 {
		t.Fatalf("%d mobs atacaram", n)
	}
	if s.Avatar.CurHP >= vidaAntes {
		t.Errorf("o jogador deveria ter levado dano: %d -> %d",
			vidaAntes, s.Avatar.CurHP)
	}
}
