package game

import (
	"context"
	"sync"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/pran"
	"aika/internal/store"
)

// fakePrans imita a tabela prans, com o mesmo SetName condicional do banco.
type fakePrans struct {
	mu     sync.Mutex
	prans  map[uint32][]store.PranName // por conta
	carga  []*pran.Pran
	salvas []pran.Pran
}

func (f *fakePrans) ByAccount(_ context.Context, acc uint32) ([]store.PranName, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.PranName(nil), f.prans[acc]...), nil
}

func (f *fakePrans) NameTaken(_ context.Context, nome string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.prans {
		for _, p := range l {
			if p.Name == nome {
				return true, nil
			}
		}
	}
	f.mu.Unlock()
	time.Sleep(20 * time.Millisecond) // abre a janela entre conferir e gravar
	f.mu.Lock()
	return false, nil
}

func (f *fakePrans) SetName(_ context.Context, id uint32, nome string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.prans {
		for i := range l {
			if l[i].ID == id && l[i].Name == "" {
				l[i].Name = nome
				return true, nil
			}
		}
	}
	return false, nil
}

func prepPran(t *testing.T, prans ...store.PranName) (*Handler, *fakePrans, *Session, *espiao) {
	t.Helper()
	h, s, e := prepMisc(t)
	f := &fakePrans{prans: map[uint32][]store.PranName{77: prans}}
	h.PranDB = f
	s.Account.ID = 77
	return h, f, s, e
}

func batiza(t *testing.T, h *Handler, s *Session, e *espiao, nome string) {
	t.Helper()
	var p packets.RenamePranPacket
	netsrv.PutCString(p.Name[:], nome)
	if err := h.renamePran(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeRenamePran, p))); err != nil {
		t.Fatal(err)
	}
}

func TestBatizaAPrimeiraPranSemNome(t *testing.T) {
	h, f, s, e := prepPran(t, store.PranName{ID: 1, Name: "Mimi"}, store.PranName{ID: 2})
	s.Storage.Slots = make([]model.Item, store.StorageSlots)
	s.Storage.Slots[84] = model.Item{Index: 100}
	batiza(t, h, s, e, "Kiki")
	if f.prans[77][1].Name != "Kiki" || f.prans[77][0].Name != "Mimi" {
		t.Errorf("prans = %+v", f.prans[77])
	}
	got := decodifica[packets.RenamePranPacket](t, proximoDe(t, e, CodeRenamePran))
	if got.AccountID != 77 || netsrv.CString(got.Name[:]) != "Kiki" {
		t.Errorf("resposta = %+v", got)
	}
}

func TestBatizaRecusas(t *testing.T) {
	h, f, s, e := prepPran(t, store.PranName{ID: 1, Name: "Mimi"})
	casos := []struct{ nome, msg string }{
		{"Ki ki", "O nome não pode conter esses caracteres."},
		{"Ki'ki", "O nome não pode conter esses caracteres."},
		{"", "O nome não pode conter esses caracteres."},
		{"Mimi", "O nome ja está em uso."},
		{"Kiki", "Todas suas prans já possuem nome."},
	}
	for _, k := range casos {
		batiza(t, h, s, e, k.nome)
		if got := mensagemDe(t, e); got != k.msg {
			t.Errorf("%q: mensagem = %q, queria %q", k.nome, got, k.msg)
		}
	}
	if f.prans[77][0].Name != "Mimi" {
		t.Error("nenhuma recusa deveria mudar o nome")
	}
}

func TestBatizaSemBancoESemPran(t *testing.T) {
	h, _, s, e := prepPran(t)
	batiza(t, h, s, e, "Kiki")
	if got := mensagemDe(t, e); got != "Todas suas prans já possuem nome." {
		t.Errorf("sem pran: %q", got)
	}
	h.PranDB = nil
	batiza(t, h, s, e, "Kiki")
	if got := mensagemDe(t, e); got != "Pran indisponivel." {
		t.Errorf("sem banco: %q", got)
	}
}

// Dois jogadores pedindo o mesmo nome ao mesmo tempo: so um fica com ele.
func TestBatizaConcorrenteMesmoNome(t *testing.T) {
	h, f, s, _ := prepPran(t, store.PranName{ID: 1})
	f.prans[88] = []store.PranName{{ID: 2}}
	s2, e2 := entraJogadorSimples(t, h)
	s2.Account.ID = 88
	s.Status, s2.Status = Playing, Playing
	e1 := novoEspiao(t)
	_, _ = h.Chan.Join(e1.conn, s)
	for _, e := range []*espiao{e1, e2} {
		e := e
		go func() {
			for range e.pacotes {
			}
		}()
	}
	var wg sync.WaitGroup
	for i, par := range []struct {
		s *Session
		e *espiao
	}{{s, e1}, {s2, e2}} {
		_ = i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var p packets.RenamePranPacket
			netsrv.PutCString(p.Name[:], "Kiki")
			_ = h.renamePran(context.Background(), par.e.conn, par.s, asPacket(t, miscPacket(CodeRenamePran, p)))
		}()
	}
	wg.Wait()
	n := 0
	for _, l := range f.prans {
		for _, p := range l {
			if p.Name == "Kiki" {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("pran com o nome = %d, queria 1", n)
	}
}

func (f *fakePrans) LoadAccount(context.Context, uint32) ([]*pran.Pran, error) { return f.carga, nil }

func (f *fakePrans) Save(_ context.Context, p *pran.Pran) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.salvas = append(f.salvas, *p)
	return nil
}

func pranDeTeste() *pran.Pran {
	p := &pran.Pran{DBID: 5, ItemID: 900, Name: "Mimi", Class: 62, Level: 3,
		CurHP: 100, MaxHP: 200, CurMP: 10, MaxMP: 20, Food: 40, Devotion: 5,
		Cute: 9, Width: 3, Chest: 4, Leg: 5}
	p.Equip[0] = model.Item{Index: 1234}
	p.Skills[0] = pran.Skill{Index: 1, Level: 3}
	return p
}

func entraComPran(t *testing.T, p *pran.Pran, itemEquip int32) (*Handler, *Session, *espiao) {
	t.Helper()
	h, s, e := prepMisc(t)
	f := &fakePrans{carga: []*pran.Pran{p}}
	h.PranDB = f
	h.PranIDs = pran.NewIDs()
	s.Account.ID = 77
	s.Avatar.Name = "Dono"
	s.Equip.Slots = make([]model.Item, store.EquipSlots)
	s.Equip.Slots[10] = model.Item{Index: 7000, Identific: itemEquip}
	h.loadPrans(context.Background(), s)
	h.enterPran(e.conn, s)
	return h, s, e
}

func TestEntrarComPranEquipadaMandaSpawnEDados(t *testing.T) {
	_, s, e := entraComPran(t, pranDeTeste(), 900)
	spawn := decodifica[packets.SendCreatePranPacket](t, proximoDe(t, e, CodePranSpawn))
	if spawn.PranClientID != pran.FirstClientID || spawn.Header.Index != pran.FirstClientID ||
		netsrv.CString(spawn.Name[:]) != "Mimi" || spawn.Equip[0] != 1234 || spawn.MaxHP != 200 ||
		spawn.Altura != 3 || spawn.Tronco != 4 || spawn.Perna != 5 {
		t.Errorf("spawn = %+v", spawn)
	}
	if netsrv.CString(spawn.Title[:]) != "Dono 's Pran" {
		t.Errorf("titulo = %q", netsrv.CString(spawn.Title[:]))
	}
	dados := decodifica[packets.SendPranToWorld](t, proximoDe(t, e, CodePranToWorld))
	if netsrv.CString(dados.Name[:]) != "Mimi" || dados.PranClass != 62 || dados.Food != 40 ||
		dados.Devotion != 5 || dados.CurHP != 100 || dados.Unk[0] != 7 || dados.Equips[0].Index != 1234 {
		t.Errorf("dados = %+v", dados)
	}
	if s.SpawnedPran != 0 || !s.Prans[0].Spawned {
		t.Errorf("spawned = %d", s.SpawnedPran)
	}
}

func TestEntrarSemItemDePranOuComOutroNaoMandaNada(t *testing.T) {
	_, s, e := entraComPran(t, pranDeTeste(), 0)
	e.nada(t, 30*time.Millisecond)
	if s.SpawnedPran != -1 {
		t.Errorf("spawned = %d, queria -1", s.SpawnedPran)
	}
	_, _, e2 := entraComPran(t, pranDeTeste(), 901) // item que nao e desta pran
	e2.nada(t, 30*time.Millisecond)
}

func TestPranFadaNaoApareceNoMundo(t *testing.T) {
	p := pranDeTeste()
	p.Class = 61
	_, s, e := entraComPran(t, p, 900)
	if s.PranClientID != 0 {
		t.Error("fada nao ganha id de pran")
	}
	proximoDe(t, e, CodePranToWorld) // os dados continuam indo
}

func TestRenomearAtualizaOCacheEAGravacaoNaoApagaONome(t *testing.T) {
	p := pranDeTeste()
	p.Name = ""
	h, s, e := entraComPran(t, p, 900)
	f := h.PranDB.(*fakePrans)
	f.prans = map[uint32][]store.PranName{77: {{ID: 5}}}
	batiza(t, h, s, e, "Kiki")
	if s.Prans[0].Name != "Kiki" {
		t.Fatalf("cache = %q", s.Prans[0].Name)
	}
	h.savePrans(s)
	if len(f.salvas) != 1 || f.salvas[0].Name != "Kiki" {
		t.Errorf("gravado = %+v", f.salvas)
	}
}

func TestSairLiberaOIdDaPran(t *testing.T) {
	h, s, _ := entraComPran(t, pranDeTeste(), 900)
	h.pranOnDisconnect(s)
	if _, ok := h.PranIDs.Owner(pran.FirstClientID); ok || s.PranClientID != 0 {
		t.Error("o id deveria voltar ao alocador")
	}
}
