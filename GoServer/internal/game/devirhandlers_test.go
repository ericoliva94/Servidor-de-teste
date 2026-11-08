package game

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aika/internal/devir"
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/stats"
	"aika/internal/store"
)

const (
	relicaA    = 6400 // tesouro sagrado de nivel 20 (EF 7 = +15)
	relicaB    = 6401 // outro, tambem tipo 40
	itemPremio = 8544
)

type fakeDevirDB struct {
	mu     sync.Mutex
	salvos int
	ultimo []devir.Row
	err    error
}

func (f *fakeDevirDB) Load(context.Context, uint8) ([]devir.Row, error) { return nil, nil }

func (f *fakeDevirDB) Save(_ context.Context, _ uint8, rows []devir.Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.salvos, f.ultimo = f.salvos+1, rows
	return nil
}

func prepDevir(t *testing.T) (*Handler, *fakeDevirDB, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	for _, idx := range []int{relicaA, relicaB} {
		var it gamedata.ItemFromList
		it.ItemType, it.Level = 40, 20
		it.EF[0], it.EFV[0] = 7, 15
		tab.poeItem(idx, it)
	}
	tab.poeItem(itemPremio, gamedata.ItemFromList{CanAgroup: 1})
	f := &fakeDevirDB{}
	h.Devir, h.DevirDB = devir.New(1), f
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Name, s.Avatar.Nation = "Alfa", 1
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.OpenedDevir, s.OpenedTemple = 0, 0
	return h, f, s, e
}

func moveRel(t *testing.T, h *Handler, s *Session, e *espiao, npc uint32, slot uint16) {
	t.Helper()
	if err := h.moveToReliquary(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeMoveToReliquary,
		packets.MoveItemToReliquare{DevirClientID: npc, SrcSlot: slot}))); err != nil {
		t.Fatal(err)
	}
}

// ---- $B52

func TestInfoDosTemplosTemNacaoSlotsENomes(t *testing.T) {
	h, _, s, e := prepDevir(t)
	h.Devir.Insert(2, 1, relicaA, relicaA, devir.Effect{Index: 7, Value: 15}, "Capturador", h.now())

	if err := h.requestDevirInfo(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeRequestDevirInfo, packets.RequestDevirInfoPacket{Channel: 2}))); err != nil {
		t.Fatal(err)
	}
	r := decodifica[packets.DevirTimeRelikInfoPacket](t, proximoDe(t, e, CodeDevirInfo))
	if r.Nation != 1 || r.Header.Index != WorldIndex {
		t.Errorf("nacao=%d index=%d", r.Nation, r.Header.Index)
	}
	sl := r.DevirCah.Slots[1] // templo 2 = Cah
	if sl.ItemID != relicaA || sl.APP != relicaA || sl.UnkByte1 != 2 || sl.UnkByte2 != 1 ||
		sl.TimeToEstabilish != uint32(h.now().Add(5*time.Hour).Unix()) {
		t.Errorf("slot do templo = %+v", sl)
	}
	info := r.DevirCahInfo.Slots[1]
	if info.ItemID != relicaA || netsrv.CString(info.NameCapped[:]) != "Capturador" || info.TimeCapped != uint32(h.now().Add(3*time.Hour).Unix()) {
		t.Errorf("info = %+v", info)
	}
	for d, di := range []model.DevirInfoForPacket{r.DevirAmkInfo, r.DevirSigInfo, r.DevirCahInfo, r.DevirMirInfo, r.DevirZelInfo} {
		for i, s := range di.Slots {
			if s.IsActive != 1 {
				t.Errorf("templo %d slot %d: IsActive = %d (slot habilitado vai ativo mesmo vazio)", d, i, s.IsActive)
			}
		}
	}
}

// ---- entregar

func TestEntregarReliquiaGravaDaHonraEPremio(t *testing.T) {
	h, f, s, e := prepDevir(t)
	s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}

	moveRel(t, h, s, e, devirTempleNPC, 4)

	sl := h.Devir.Snapshot()[0].Slots[0]
	if sl.ItemID != relicaA || sl.Name != "Alfa" {
		t.Errorf("slot = %+v", sl)
	}
	if !s.Inventory.Slots[4].IsEmpty() {
		t.Error("o tesouro deveria ter saido do inventario")
	}
	if f.salvos != 1 {
		t.Errorf("gravacoes = %d", f.salvos)
	}
	if want := uint32(20+1) * 250; s.Avatar.Honor != want {
		t.Errorf("honra = %d, queria %d", s.Avatar.Honor, want)
	}
	if contaItem(s, itemPremio) != 1 {
		t.Error("faltou o item de recompensa")
	}
	if h.Devir.EffectValue(7) != 15 {
		t.Errorf("efeito = %d", h.Devir.EffectValue(7))
	}
	proximoDe(t, e, CodeSignal10F)
}

func TestEntregarDividePremioComOGrupoPorPerto(t *testing.T) {
	h, _, s, e := prepDevir(t)
	s.Position = model.Position{X: 3400, Y: 600}
	perto, pe := novoEspiao(t), novoEspiao(t)
	_ = perto
	pertoS := sessaoComInventario()
	pertoS.Avatar.Nation = 1
	pertoS.Position = model.Position{X: 3405, Y: 600}
	longeS := sessaoComInventario()
	longeS.Avatar.Nation = 1
	longeS.Position = model.Position{X: 3600, Y: 600}
	h.Chan.Join(pe.conn, pertoS)
	le := novoEspiao(t)
	h.Chan.Join(le.conn, longeS)
	h.Parties = NewPartyRegistry()
	g, _ := h.Parties.Create(s.ClientID)
	h.Parties.Add(g, pertoS.ClientID)
	h.Parties.Add(g, longeS.ClientID)
	s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}

	moveRel(t, h, s, e, devirTempleNPC, 4)

	honra := uint32(21 * 250)
	if s.Avatar.Honor != honra || pertoS.Avatar.Honor != honra {
		t.Errorf("honra lider=%d perto=%d, queria %d", s.Avatar.Honor, pertoS.Avatar.Honor, honra)
	}
	if longeS.Avatar.Honor != 0 || contaItem(longeS, itemPremio) != 0 {
		t.Error("quem estava longe nao recebe")
	}
	if contaItem(pertoS, itemPremio) != 1 {
		t.Error("o membro por perto deveria receber o premio")
	}
}

func TestEntregarRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(h *Handler, s *Session)
		msg   string
	}{
		{"nao e tesouro", func(h *Handler, s *Session) { s.Inventory.Slots[4].Index = 100 }, "Este item nao e um tesouro sagrado."},
		{"slot vazio", func(h *Handler, s *Session) { s.Inventory.Slots[4] = model.Item{} }, "Slot invalido."},
		{"outra nacao", func(h *Handler, s *Session) { s.Avatar.Nation = 2 }, "Para computar a reliquia, e necessario ter nacionalidade no pais."},
		{"sem nacao", func(h *Handler, s *Session) { s.Avatar.Nation = 0 }, "Para computar a reliquia, e necessario ter nacionalidade no pais."},
		{"nenhum templo aberto", func(h *Handler, s *Session) { s.OpenedDevir = NoDevir }, "Nenhum templo aberto."},
		{"sem espaco", func(h *Handler, s *Session) {
			for i := 0; i < devir.SlotsPerTemple; i++ {
				h.Devir.Insert(0, i, relicaB, relicaB, devir.Effect{}, "x", h.now())
			}
		}, "Todos os espacos sagrados disponiveis estao lotados."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, f, s, e := prepDevir(t)
			s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}
			c.setup(h, s)
			antes := f.salvos
			moveRel(t, h, s, e, devirTempleNPC, 4)
			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if f.salvos != antes || s.Avatar.Honor != 0 {
				t.Error("recusa gravou ou deu honra")
			}
		})
	}
}

func TestEntregarDesfazSeABaseFalha(t *testing.T) {
	h, f, s, e := prepDevir(t)
	f.err = errors.New("mysql caiu")
	s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}
	moveRel(t, h, s, e, devirTempleNPC, 4)
	if got := mensagemDe(t, e); got != "Falha ao salvar reliquia no banco de dados." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[4].Index != relicaA {
		t.Error("o jogador perdeu o tesouro apesar da falha")
	}
	if h.Devir.Snapshot()[0].Slots[0].ItemID != 0 || h.Devir.EffectValue(7) != 0 {
		t.Error("o templo ficou com a reliquia apesar da falha")
	}
	if s.Avatar.Honor != 0 {
		t.Error("deu honra apesar da falha")
	}
}

// ---- puxar

func prepRoubo(t *testing.T) (*Handler, *fakeDevirDB, *Session, *espiao) {
	h, f, s, e := prepDevir(t)
	s.Avatar.Nation = 2                                                                                          // o templo e da nacao 1
	h.Devir.Insert(0, 1, relicaA, relicaA, devir.Effect{Index: 7, Value: 15}, "Dono", h.now().Add(-3*time.Hour)) // ja liberada
	return h, f, s, e
}

func TestPuxarReliquiaDoTemploInimigo(t *testing.T) {
	h, f, s, e := prepRoubo(t)
	moveRel(t, h, s, e, devirRelicNPC, 1)

	if contaItem(s, relicaA) != 1 {
		t.Error("a reliquia deveria estar no inventario")
	}
	sl := h.Devir.Snapshot()[0].Slots[1]
	if sl.ItemID != 0 || !sl.Furthed || sl.ItemFurthed != relicaA {
		t.Errorf("slot depois do roubo = %+v", sl)
	}
	if h.Devir.EffectValue(7) != 0 {
		t.Errorf("efeito = %d, queria 0", h.Devir.EffectValue(7))
	}
	if s.OpenedDevir != NoDevir || s.OpenedTemple != NoDevir {
		t.Error("o templo deveria ter sido fechado para o jogador")
	}
	if f.salvos != 1 {
		t.Errorf("gravacoes = %d", f.salvos)
	}
	proximoDe(t, e, CodeMoveToReliquary) // eco
}

func TestPuxarRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(h *Handler, s *Session)
		slot  uint16
		msg   string
	}{
		{"templo da propria nacao", func(h *Handler, s *Session) { s.Avatar.Nation = 1 }, 1, "Voce nao pode pegar seus proprios tesouros."},
		{"ja carrega uma", func(h *Handler, s *Session) { s.Inventory.Slots[3] = model.Item{Index: relicaB} }, 1, "Voce nao pode carregar mais de uma reliquia por vez."},
		{"inventario cheio", func(h *Handler, s *Session) {
			for i := 0; i < MovableInventorySlots; i++ {
				s.Inventory.Slots[i] = model.Item{Index: 100}
			}
		}, 1, "Inventario cheio."},
		{"ainda nao liberada", func(h *Handler, s *Session) {
			h.Devir.Insert(0, 2, relicaB, relicaB, devir.Effect{}, "Novo", h.now())
		}, 2, "Reliquia nao liberada ainda."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, f, s, e := prepRoubo(t)
			c.setup(h, s)
			antes := f.salvos
			moveRel(t, h, s, e, devirRelicNPC, c.slot)
			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if f.salvos != antes || h.Devir.Snapshot()[0].Slots[1].ItemID != relicaA {
				t.Error("recusa mexeu no templo")
			}
		})
	}
}

// Defeitos da origem: SrcSlot (WORD) sem faixa e puxar slot vazio dava o item 0.
func TestPuxarSlotVazioOuForaDaFaixaNaoFazNada(t *testing.T) {
	h, f, s, e := prepRoubo(t)
	for _, slot := range []uint16{0, 4, 5, 255, 65535} {
		moveRel(t, h, s, e, devirRelicNPC, slot)
	}
	if f.salvos != 0 || len(s.Inventory.Slots) == 0 {
		t.Error("pedido invalido gravou")
	}
	for i := 0; i < MovableInventorySlots; i++ {
		if !s.Inventory.Slots[i].IsEmpty() && s.Inventory.Slots[i].Index < 4 {
			t.Fatalf("slot %d recebeu item invalido %d", i, s.Inventory.Slots[i].Index)
		}
	}
	e.nada(t, 20*time.Millisecond)
}

// Defeito da origem: se a gravacao do roubo falhava, o slot era restaurado mas
// o item ja estava no inventario do jogador (duplicava).
func TestPuxarDesfazSeABaseFalhaSemDarOItem(t *testing.T) {
	h, f, s, e := prepRoubo(t)
	f.err = errors.New("mysql caiu")
	moveRel(t, h, s, e, devirRelicNPC, 1)
	if got := mensagemDe(t, e); got != "Falha ao salvar roubo no banco de dados." {
		t.Errorf("mensagem = %q", got)
	}
	if contaItem(s, relicaA) != 0 {
		t.Error("o jogador recebeu a reliquia apesar da falha")
	}
	if h.Devir.Snapshot()[0].Slots[1].ItemID != relicaA || h.Devir.EffectValue(7) != 15 {
		t.Error("o templo nao voltou ao que era")
	}
}

func TestReliquiasSemRegistroOuForaDoJogo(t *testing.T) {
	h, _, s, e := prepDevir(t)
	h.Devir = nil
	moveRel(t, h, s, e, devirTempleNPC, 0)
	if got := mensagemDe(t, e); !strings.Contains(got, "Reliquias nao estao disponiveis") {
		t.Errorf("mensagem = %q", got)
	}
	s.Status = CharList
	if err := h.requestDevirInfo(context.Background(), e.conn, s, asPacket(t,
		miscPacket(CodeRequestDevirInfo, packets.RequestDevirInfoPacket{}))); !errors.Is(err, ErrNotPlaying) {
		t.Errorf("erro = %v", err)
	}
	// Outro NPC que nao e do templo nem da reliquia: ignorado.
	h2, _, s2, e2 := prepDevir(t)
	moveRel(t, h2, s2, e2, 5, 0)
	e2.nada(t, 20*time.Millisecond)
}

func TestTamanhosDosPacotesDeReliquia(t *testing.T) {
	if n := binary.Size(packets.MoveItemToReliquare{}); n != 24 {
		t.Errorf("MoveItemToReliquare = %d, o Delphi tem 24", n)
	}
	if n := binary.Size(packets.RequestDevirInfoPacket{}); n != 16 {
		t.Errorf("RequestDevirInfoPacket = %d, o Delphi tem 16", n)
	}
}

// ---- $136 e o reenvio a todos

func observador(t *testing.T, h *Handler, nacao uint16) (*Session, *espiao) {
	t.Helper()
	s, e := entraJogadorSimples(t, h)
	s.Avatar.Nation = nacao
	return s, e
}

func TestEntregarReliquiaAtualizaTodosOsJogadores(t *testing.T) {
	h, _, s, e := prepDevir(t)
	mesma, me := observador(t, h, 1)
	outra, oe := observador(t, h, 2)
	_, _ = mesma, outra
	s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}

	moveRel(t, h, s, e, devirTempleNPC, 4)

	got := decodifica[packets.SendReliques](t, proximoDe(t, me, CodeReliques))
	if got.ReliquesItemID[0] != relicaA {
		t.Errorf("lista da mesma nacao = %v", got.ReliquesItemID)
	}
	if got.Header.Index != shopHeaderIndex {
		t.Errorf("Header.Index = %#x", got.Header.Index)
	}
	proximoDe(t, me, CodeRefreshPoint)
	// Outra nacao recebe a lista vazia, como a origem (canal diferente).
	vazio := decodifica[packets.SendReliques](t, proximoDe(t, oe, CodeReliques))
	for i, id := range vazio.ReliquesItemID {
		if id != 0 {
			t.Errorf("outra nacao viu a reliquia %d no indice %d", id, i)
		}
	}
	// Quem entregou tambem recebe.
	proximoDe(t, e, CodeReliques)
}

func TestPuxarReliquiaAtualizaTodosOsJogadores(t *testing.T) {
	h, _, s, e := prepRoubo(t)
	dono, de := observador(t, h, 1)
	_ = dono
	moveRel(t, h, s, e, devirRelicNPC, 1)
	got := decodifica[packets.SendReliques](t, proximoDe(t, de, CodeReliques))
	if got.ReliquesItemID[1] != 0 {
		t.Errorf("a reliquia roubada ainda aparece: %v", got.ReliquesItemID)
	}
}

func TestSendReliquesSemNacaoVeAsDoCanalESemRegistroNaoQuebra(t *testing.T) {
	h, _, _, _ := prepDevir(t)
	h.Devir.Insert(0, 2, relicaB, relicaB, devir.Effect{}, "X", h.now())
	s, e := observador(t, h, 0)
	h.sendReliques(e.conn, s)
	got := decodifica[packets.SendReliques](t, proximoDe(t, e, CodeReliques))
	if got.ReliquesItemID[2] != relicaB {
		t.Errorf("lista = %v", got.ReliquesItemID)
	}
	h.Devir = nil
	h.sendReliques(e.conn, s)      // nao pode quebrar
	h.updateReliquaresForAll()     // nem este
	e.nada(t, 30*time.Millisecond) // e nada e enviado
}

// ---- efeitos da reliquia nos atributos

func TestEfeitosDeReliquiaSoValemParaANacaoDoCanal(t *testing.T) {
	reg := devir.New(1)
	reg.Insert(0, 0, relicaA, relicaA, devir.Effect{Index: efRelicCritical, Value: 7}, "X", time.Now())
	reg.Insert(0, 1, relicaB, relicaB, devir.Effect{Index: efRelicDouble, Value: 5}, "X", time.Now())
	dono := relicEffectsFor(reg, 1)
	if dono.Critical != 7 || dono.Double != 5 {
		t.Errorf("da nacao = %+v", dono)
	}
	// Outra nacao e quem nao tem nacao: so o duplo, que a origem nao filtra.
	for _, n := range []uint16{0, 2} {
		fx := relicEffectsFor(reg, n)
		if fx.Critical != 0 || fx.Double != 5 {
			t.Errorf("nacao %d = %+v", n, fx)
		}
	}
	if (relicEffectsFor(nil, 1) != RelicEffects{}) {
		t.Error("sem registro deveria ser zero")
	}
}

func TestEntregarReliquiaMudaOsAtributosDeQuemEDaNacao(t *testing.T) {
	h, _, s, e := prepDevir(t)
	// relicaA: EF[0]=7, EFV[0]=15. Troca para HP% (146) e depois para critico.
	h.Data.(*tabelaDeItens).itens[relicaA] = gamedata.ItemFromList{ItemType: 40, Level: 20,
		EF: [3]uint16{efRelicPerHP}, EFV: [3]uint16{20}}
	s.Equip = store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}
	s.Avatar.Level, s.Avatar.ClassInfo, s.Avatar.Cons = 50, 1, 100
	s.Avatar.Recalc(stats.Bonus{})
	antes := s.Avatar.MaxHP
	s.Inventory.Slots[4] = model.Item{Index: relicaA, APP: relicaA}

	moveRel(t, h, s, e, devirTempleNPC, 4)

	if s.Avatar.Relic.PerHP != 20 {
		t.Fatalf("Relic = %+v", s.Avatar.Relic)
	}
	if want := antes + antes/100*20; s.Avatar.MaxHP < want-uint32(20) || s.Avatar.MaxHP <= antes {
		t.Errorf("MaxHP = %d, antes %d, esperado perto de %d", s.Avatar.MaxHP, antes, want)
	}
	proximoDe(t, e, CodeRefreshPoint)
}

func TestSubirDeNivelMantemOBonusDeReliquia(t *testing.T) {
	h, _, s, e := prepDevir(t)
	s.Equip = store.ItemSet{Slots: make([]model.Item, store.EquipSlots)}
	s.Avatar.ClassInfo, s.Avatar.Level = 1, 10
	h.Devir.Insert(0, 0, relicaA, relicaA, devir.Effect{Index: efRelicPerHP, Value: 10}, "X", time.Now())
	h.refreshRelics(s)
	semNivel := s.Avatar.MaxHP
	if s.Avatar.Relic.PerHP != 10 {
		t.Fatal("reliquia nao aplicada")
	}
	s.Avatar.Level = 11
	s.Avatar.ApplyEquipBonus(h.data(), s.Equip)
	if s.Avatar.MaxHP <= semNivel {
		t.Errorf("MaxHP nao subiu: %d -> %d", semNivel, s.Avatar.MaxHP)
	}
	_ = e
}
