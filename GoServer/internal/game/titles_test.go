package game

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
	"aika/internal/titledata"
)

// fakeTitles guarda os titulos em memoria.
type fakeTitles struct {
	mu     sync.Mutex
	carga  []model.TitleData
	salvos [][]model.TitleData
}

func (f *fakeTitles) Load(_ context.Context, _ uint32) ([]model.TitleData, error) {
	return f.carga, nil
}

func (f *fakeTitles) Save(_ context.Context, _ uint32, l []model.TitleData) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.salvos = append(f.salvos, append([]model.TitleData(nil), l...))
	return nil
}

// tabelaDeTitulos monta um Title.bin com os niveis dados: [indice][nivel-1].
type nivelTitulo struct {
	tipo     uint8
	idx      uint8
	ef, efv  [3]uint16
	nivelNum int
}

func tabelaDeTitulos(t *testing.T, defs map[int]map[int]nivelTitulo) *titledata.Table {
	t.Helper()
	b := make([]byte, titledata.TableBytes)
	for i, niveis := range defs {
		for l, n := range niveis {
			off := (i*titledata.Levels + l - 1) * 56
			r := b[off : off+56]
			r[1] = n.tipo
			for k := 0; k < 3; k++ {
				binary.LittleEndian.PutUint16(r[4+2*k:], n.ef[k])
				binary.LittleEndian.PutUint16(r[10+2*k:], n.efv[k])
			}
			r[49] = n.idx
		}
	}
	tb, err := titledata.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

type cenarioTitulo struct {
	h    *Handler
	s    *Session
	e    *espiao
	viz  *Session
	eViz *espiao
	db   *fakeTitles
}

func novoCenarioTitulo(t *testing.T) *cenarioTitulo {
	t.Helper()
	h, _ := handlerComItens(t)
	c := &cenarioTitulo{h: h, db: &fakeTitles{}}
	h.TitleDB = c.db
	h.TitleTable = tabelaDeTitulos(t, map[int]map[int]nivelTitulo{
		1: {1: {ef: [3]uint16{182, 0, 0}, efv: [3]uint16{9, 0, 0}},
			2: {ef: [3]uint16{182, 183, 0}, efv: [3]uint16{20, 4, 0}}},
		2: {1: {ef: [3]uint16{183, 0, 0}, efv: [3]uint16{5, 0, 0}}},
	})
	c.s, c.e = entraJogadorSimples(t, h)
	c.s.Status = Playing
	c.viz, c.eViz = entraJogadorSimples(t, h)
	c.viz.Status = Playing
	c.s.Base.VisiblePlayers.Add(c.viz.ClientID)
	c.viz.Base.VisiblePlayers.Add(c.s.ClientID)
	return c
}

func (c *cenarioTitulo) pede(t *testing.T, idx uint32) {
	t.Helper()
	buf, err := netsrv.Marshal(CodeUpdateActiveTitle, packets.SignalData{Data: idx})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.h.updateActiveTitle(context.Background(), c.e.conn, c.s, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
}

func TestMapaDeNiveisDoTitulo(t *testing.T) {
	casos := []struct {
		slot, nivel uint8
		quer        uint32
	}{
		{0, 0, 0}, {0, 1, 1}, {0, 2, 3}, {0, 4, 15}, {1, 1, 16}, {1, 3, 16 + 32 + 64},
		{7, 4, 0xF0000000}, {7, 1, 1 << 28},
	}
	for _, k := range casos {
		if got := titleLevelValue(k.slot, k.nivel); got != k.quer {
			t.Errorf("titleLevelValue(%d,%d) = %#x, queria %#x", k.slot, k.nivel, got, k.quer)
		}
	}
}

func TestPacoteDeEntradaLevaNiveisEProgressoDosTitulos(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.h.TitleTable = tabelaDeTitulos(t, map[int]map[int]nivelTitulo{
		9:  {2: {tipo: 8, idx: 3}},
		17: {1: {tipo: 4}},
		30: {1: {tipo: 16, idx: 99}}, // indice fora do vetor do tipo: ignorado
	})
	c.s.Titles[0] = model.TitleData{Index: 9, Level: 2, Progress: 7}
	c.s.Titles[1] = model.TitleData{Index: 17, Level: 1, Progress: 55}
	c.s.Titles[2] = model.TitleData{Index: 30, Level: 1, Progress: 1}
	c.s.Titles[3] = model.TitleData{Index: 5, Level: 0, Progress: 1} // nivel 0 nao conta
	c.s.Titles[4] = model.TitleData{Index: 200, Level: 1}            // categoria 25: fora do vetor de 12
	c.s.Titles[5] = model.TitleData{Index: 96, Level: 1}             // categoria 12: a primeira fora
	var ch model.Character
	c.h.fillTitleProgress(c.s, &ch)
	// Titulo 9: categoria 1, slot 1, nivel 2 -> bits 4 e 5.
	if ch.TitleCategoryLevel[1] != 16+32 || ch.TitleProgressType8[2] != 7 {
		t.Errorf("cat1=%#x progresso8=%d", ch.TitleCategoryLevel[1], ch.TitleProgressType8[2])
	}
	// Titulo 17: categoria 2, slot 1, nivel 1; tipo 4 e escalar.
	if ch.TitleCategoryLevel[2] != 16 || ch.TitleProgressType4 != 55 {
		t.Errorf("cat2=%#x progresso4=%d", ch.TitleCategoryLevel[2], ch.TitleProgressType4)
	}
	if ch.TitleCategoryLevel[0] != 0 {
		t.Errorf("titulo de nivel 0 nao deve contar: cat0=%#x", ch.TitleCategoryLevel[0])
	}
}

func TestCarregarTitulosAchaONivelDoAtivo(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.db.carga = []model.TitleData{{Index: 2, Level: 1, Progress: 4}, {Index: 1, Level: 2, Progress: 9}}
	c.h.loadTitles(context.Background(), c.s, 5, 1)
	if c.s.ActiveTitle != (model.TitleData{Index: 1, Level: 2}) {
		t.Errorf("ativo = %+v", c.s.ActiveTitle)
	}
	if c.s.Avatar.TitleID != 1 || c.s.Avatar.TitleLevel != 2 {
		t.Errorf("avatar = %d/%d", c.s.Avatar.TitleID, c.s.Avatar.TitleLevel)
	}
	if c.s.Titles[1].Progress != 9 {
		t.Errorf("lista = %+v", c.s.Titles[:2])
	}
	// Ativo que o jogador nao tem mais: fica so o indice, sem nivel e sem efeito.
	c.h.loadTitles(context.Background(), c.s, 5, 77)
	if c.s.ActiveTitle != (model.TitleData{Index: 77}) {
		t.Errorf("ativo sem titulo = %+v", c.s.ActiveTitle)
	}
}

func TestAtivarTituloSomaOEfeitoEAvisaOsVizinhos(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 2}
	c.pede(t, 1)
	if c.s.ActiveTitle.Index != 1 || c.s.ActiveTitle.Level != 2 {
		t.Fatalf("ativo = %+v", c.s.ActiveTitle)
	}
	if c.s.Avatar.Ability.get(efPranDamage1) != 20 || c.s.Avatar.Ability.get(efPranDamage2) != 4 {
		t.Errorf("efeitos = %v", nonZero(&c.s.Avatar.Ability))
	}
	proprio := decodifica[packets.UpdateActiveTitlePacket](t, proximoDe(t, c.e, CodeUpdateActiveTitle))
	vizinho := decodifica[packets.UpdateActiveTitlePacket](t, proximoDe(t, c.eViz, CodeUpdateActiveTitle))
	if proprio.TitleIndex != 1 || proprio.TitleLevel != 1 ||
		vizinho.TitleIndex != proprio.TitleIndex || vizinho.TitleLevel != proprio.TitleLevel ||
		vizinho.Header.Index != c.s.ClientID {
		t.Errorf("proprio=%+v vizinho=%+v", proprio, vizinho)
	}
	if c.s.Avatar.TitleID != 1 || c.s.Avatar.TitleLevel != 2 {
		t.Errorf("o avatar deve refletir o titulo: %d/%d", c.s.Avatar.TitleID, c.s.Avatar.TitleLevel)
	}
}

func TestTrocarDeTituloTiraOEfeitoDoAntigo(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 2}
	c.s.Titles[1] = model.TitleData{Index: 2, Level: 1}
	c.pede(t, 1)
	c.pede(t, 2)
	got := nonZero(&c.s.Avatar.Ability)
	if got[efPranDamage1] != 0 || got[efPranDamage2] != 5 {
		t.Errorf("efeitos depois da troca = %v", got)
	}
	c.pede(t, 0)
	if c.s.ActiveTitle != (model.TitleData{}) || len(nonZero(&c.s.Avatar.Ability)) != 0 {
		t.Errorf("ativo=%+v efeitos=%v", c.s.ActiveTitle, nonZero(&c.s.Avatar.Ability))
	}
	c.e.drena(20 * time.Millisecond)
}

func TestTituloQueOJogadorNaoTemNaoMudaNada(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 2}
	c.s.Titles[1] = model.TitleData{Index: 2, Level: 0} // nivel 0: ignorado
	c.pede(t, 1)
	antes := nonZero(&c.s.Avatar.Ability)
	for _, idx := range []uint32{2, 3, 300, 1 << 20} {
		c.pede(t, idx)
	}
	if c.s.ActiveTitle.Index != 1 {
		t.Errorf("o ativo mudou: %+v", c.s.ActiveTitle)
	}
	depois := nonZero(&c.s.Avatar.Ability)
	if len(antes) != len(depois) || depois[efPranDamage1] != antes[efPranDamage1] {
		t.Errorf("efeitos antes=%v depois=%v", antes, depois)
	}
}

func TestTituloExigeEstarEmJogo(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Status = CharList
	buf, _ := netsrv.Marshal(CodeUpdateActiveTitle, packets.SignalData{Data: 1})
	if err := c.h.updateActiveTitle(context.Background(), c.e.conn, c.s, asPacket(t, buf)); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
}

func TestEntrarComTituloAtivoMandaOPacoteELigaOEfeito(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.db.carga = []model.TitleData{{Index: 1, Level: 1}}
	c.h.loadTitles(context.Background(), c.s, 5, 1)
	c.h.enterTitle(c.e.conn, c.s)
	pkt := decodifica[packets.UpdateActiveTitlePacket](t, proximoDe(t, c.e, CodeUpdateActiveTitle))
	if pkt.TitleIndex != 1 || pkt.TitleLevel != 0 {
		t.Errorf("pacote = %+v", pkt)
	}
	if c.s.Avatar.Ability.get(efPranDamage1) != 9 {
		t.Errorf("efeitos = %v", nonZero(&c.s.Avatar.Ability))
	}
}

func TestGravarTitulosEOTituloAtivo(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.SelectedSlot = 0
	c.s.Characters[0].ID = 5
	c.s.Account.ID = 1
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 2, Progress: 3}
	c.s.ActiveTitle = model.TitleData{Index: 1, Level: 2}
	c.h.saveTitles(c.s)
	if len(c.db.salvos) != 1 || c.db.salvos[0][0] != c.s.Titles[0] || len(c.db.salvos[0]) != store.MaxTitles {
		t.Fatalf("salvos = %+v", c.db.salvos)
	}
	if got := progressOf(c.s).ActiveTitle; got != 1 {
		t.Errorf("active_title gravado = %d, queria 1", got)
	}
}

func TestAdicionarTituloComAvisoUsaAMetaDoNivel(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.h.TitleTable = tabelaDeTitulosComMeta(t, 5, 2, 300, "Guardiao")
	if !c.h.addTitle(c.e.conn, c.s, 5, 2, true) {
		t.Fatal("deveria adicionar")
	}
	if c.s.Titles[0] != (model.TitleData{Index: 5, Level: 2, Progress: 300}) {
		t.Errorf("titulo = %+v", c.s.Titles[0])
	}
	up := decodifica[packets.UpdateTitleListPacket](t, proximoDe(t, c.e, CodeTitleUpdate))
	if up.IDAcquire != 5 || up.IDLeveled != 1 {
		t.Errorf("$17D = %+v", up)
	}
	if got := mensagemDe(t, c.e); got != "Você obteve um novo título [Guardiao]" {
		t.Errorf("mensagem = %q", got)
	}
	// Repetido, invalido, e sem aviso com progresso 1.
	if c.h.addTitle(c.e.conn, c.s, 5, 1, true) || c.h.addTitle(c.e.conn, c.s, 0, 1, false) ||
		c.h.addTitle(c.e.conn, c.s, 6, 9, true) || c.h.addTitle(c.e.conn, c.s, 6, 0, true) {
		t.Error("titulo repetido ou invalido nao entra")
	}
	if !c.h.addTitle(c.e.conn, c.s, 7, 0, false) || c.s.Titles[1] != (model.TitleData{Index: 7, Level: 0, Progress: 1}) {
		t.Errorf("sem aviso: %+v", c.s.Titles[1])
	}
}

func TestListaDeTitulosCheia(t *testing.T) {
	c := novoCenarioTitulo(t)
	for i := range c.s.Titles {
		c.s.Titles[i] = model.TitleData{Index: uint8(i + 1), Level: 1}
	}
	if c.h.addTitle(c.e.conn, c.s, 200, 1, false) {
		t.Fatal("lista cheia")
	}
	if got := mensagemDe(t, c.e); got != "Sua lista de títulos está cheia." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestSubirNivelDoTituloAtivoRefazOEfeito(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 1}
	c.pede(t, 1)
	if c.s.Avatar.Ability.get(efPranDamage1) != 9 {
		t.Fatalf("efeitos = %v", nonZero(&c.s.Avatar.Ability))
	}
	if !c.h.updateTitleLevel(c.e.conn, c.s, 1, 2, true) {
		t.Fatal("deveria subir")
	}
	if c.s.Titles[0].Level != 2 || c.s.ActiveTitle.Level != 2 || c.s.Avatar.TitleLevel != 2 {
		t.Errorf("lista=%+v ativo=%+v", c.s.Titles[0], c.s.ActiveTitle)
	}
	if got := nonZero(&c.s.Avatar.Ability); got[efPranDamage1] != 20 || got[efPranDamage2] != 4 {
		t.Errorf("efeitos = %v", got)
	}
	if c.h.updateTitleLevel(c.e.conn, c.s, 99, 2, true) || c.h.updateTitleLevel(c.e.conn, c.s, 1, 9, true) {
		t.Error("titulo inexistente ou nivel invalido")
	}
}

func TestRemoverTituloAtivoDesligaOEfeito(t *testing.T) {
	c := novoCenarioTitulo(t)
	c.s.Titles[0] = model.TitleData{Index: 1, Level: 2}
	c.pede(t, 1)
	c.e.drena(20 * time.Millisecond)
	if !c.h.removeTitle(c.e.conn, c.s, 1) {
		t.Fatal("deveria remover")
	}
	if c.s.Titles[0] != (model.TitleData{}) || c.s.ActiveTitle != (model.TitleData{}) || len(nonZero(&c.s.Avatar.Ability)) != 0 {
		t.Errorf("lista=%+v ativo=%+v efeitos=%v", c.s.Titles[0], c.s.ActiveTitle, nonZero(&c.s.Avatar.Ability))
	}
	up := decodifica[packets.UpdateTitleListPacket](t, proximoDe(t, c.e, CodeTitleUpdate))
	if up.IDAcquire != 1 || up.IDLeveled != 0 {
		t.Errorf("$17D = %+v", up)
	}
	if c.h.removeTitle(c.e.conn, c.s, 1) {
		t.Error("remover duas vezes")
	}
}

// tabelaDeTitulosComMeta monta uma tabela com um unico nivel com meta e nome.
func tabelaDeTitulosComMeta(t *testing.T, idx, nivel int, meta uint16, nome string) *titledata.Table {
	t.Helper()
	b := make([]byte, titledata.TableBytes)
	off := (idx*titledata.Levels + nivel - 1) * 56
	binary.LittleEndian.PutUint16(b[off+2:], meta)
	copy(b[off+16:off+48], nome)
	tb, err := titledata.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}
