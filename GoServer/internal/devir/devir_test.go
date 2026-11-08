package devir

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Unix(1_700_000_000, 0)

func efeito(item uint16) (Effect, bool) { return Effect{Index: 7, Value: 15}, item != 0 }

func linha(id int, slots ...RowSlot) Row {
	r := Row{DevirID: id}
	for i := range r.Slots {
		r.Slots[i].Able = true
	}
	copy(r.Slots[:], slots)
	return r
}

func TestFirstDevirIDPorNacao(t *testing.T) {
	for nation, want := range map[uint8]int{0: 1, 1: 1, 2: 6, 3: 11, 5: 21} {
		if got := FirstDevirID(nation); got != want {
			t.Errorf("nacao %d = %d, queria %d", nation, got, want)
		}
	}
}

func TestLoadSomaEfeitoSoDeSlotComCapturaValida(t *testing.T) {
	r := New(2)
	r.Load([]Row{
		linha(6,
			RowSlot{ItemID: 6400, Name: "Alfa", TimeCap: 1_700_000_000, Able: true}, // valido
			RowSlot{ItemID: 6401, Name: "Beta", TimeCap: 5, Able: true},             // timecap lixo
			RowSlot{ItemID: 0, Able: true},
		),
		linha(10, RowSlot{ItemID: 6402, TimeCap: 1_700_000_100, Able: true}),
		linha(99, RowSlot{ItemID: 6403, TimeCap: 1_700_000_100, Able: true}), // fora da nacao
	}, efeito)

	snap := r.Snapshot()
	s := snap[0].Slots[0]
	if s.ItemID != 6400 || s.APP != 6400 || s.Name != "Alfa" || !s.TimeCapped.Equal(time.Unix(1_700_000_000, 0)) {
		t.Errorf("slot 0 = %+v", s)
	}
	if want := s.TimeCapped.Add(2 * time.Hour); !s.TimeToEstablish.Equal(want) {
		t.Errorf("TimeToEstablish = %v, queria captura + 2h", s.TimeToEstablish)
	}
	if bad := snap[0].Slots[1]; bad.ItemID != 6401 || !bad.TimeCapped.IsZero() {
		t.Errorf("timecap lixo deveria ficar sem captura: %+v", bad)
	}
	if got := r.EffectValue(7); got != 30 { // 6400 e 6402; 6401 (lixo) e 6403 (fora) nao entram
		t.Errorf("efeito = %d, queria 30", got)
	}
	if snap[4].DevirID != 10 {
		t.Errorf("devir_id do ultimo templo = %d, queria 10", snap[4].DevirID)
	}
}

func TestRowsIdaEVolta(t *testing.T) {
	r := New(1)
	r.Load([]Row{linha(1, RowSlot{ItemID: 6400, Name: "Alfa", TimeCap: 1_700_000_000, Able: true})}, efeito)
	rows := r.Rows()
	if len(rows) != TemplesPerNation || rows[0].Slots[0].ItemID != 6400 || rows[0].Slots[0].TimeCap != 1_700_000_000 || rows[0].Slots[0].Name != "Alfa" {
		t.Errorf("linhas = %+v", rows[0])
	}
	if rows[0].Slots[1].TimeCap != 0 {
		t.Error("slot vazio nao deve gravar timecap")
	}
}

func TestFreeSpacePulaSlotDesabilitadoEOcupado(t *testing.T) {
	r := New(1)
	rows := []Row{linha(1)}
	rows[0].Slots[0].ItemID = 6400
	rows[0].Slots[0].TimeCap = 1_700_000_000
	rows[0].Slots[1].Able = false
	r.Load(rows, efeito)
	if s, ok := r.FreeSpace(0); !ok || s != 2 {
		t.Errorf("FreeSpace = %d %v, queria o slot 2", s, ok)
	}
	for _, d := range []int{-1, 5, 255} {
		if _, ok := r.FreeSpace(d); ok {
			t.Errorf("templo %d nao existe", d)
		}
	}
}

func TestInsertGravaTemposNomeEEfeito(t *testing.T) {
	r := New(1)
	prev, err := r.Insert(0, 2, 6400, 6400, Effect{7, 15}, "Alfa", t0)
	if err != nil || prev.ItemID != 0 {
		t.Fatalf("Insert = %+v %v", prev, err)
	}
	s := r.Snapshot()[0].Slots[2]
	if s.ItemID != 6400 || s.Name != "Alfa" || !s.TimeCapped.Equal(t0.Add(3*time.Hour)) || !s.TimeToEstablish.Equal(t0.Add(5*time.Hour)) {
		t.Errorf("slot = %+v", s)
	}
	if r.EffectValue(7) != 15 {
		t.Errorf("efeito = %d", r.EffectValue(7))
	}
	if _, err := r.Insert(0, 2, 6401, 6401, Effect{7, 15}, "Beta", t0); !errors.Is(err, ErrSlotBusy) {
		t.Errorf("slot ocupado: %v", err)
	}
	for _, c := range [][2]int{{-1, 0}, {5, 0}, {0, 5}, {0, -1}, {255, 255}} {
		if _, err := r.Insert(c[0], c[1], 6400, 6400, Effect{}, "x", t0); !errors.Is(err, ErrBadSlot) {
			t.Errorf("Insert%v: %v", c, err)
		}
	}
}

func TestInsertEmSlotDesabilitadoERecusado(t *testing.T) {
	r := New(1)
	rows := []Row{linha(1)}
	rows[0].Slots[0].Able = false
	r.Load(rows, efeito)
	if _, err := r.Insert(0, 0, 6400, 6400, Effect{}, "x", t0); !errors.Is(err, ErrSlotBusy) {
		t.Errorf("erro = %v", err)
	}
}

func TestInsertLimpaMarcaDeRoubadaDaMesmaReliquia(t *testing.T) {
	r := New(1)
	r.Insert(0, 0, 6400, 6400, Effect{7, 15}, "Alfa", t0)
	r.Pull(0, 0, func(uint16) Effect { return Effect{7, 15} }, t0.Add(3*time.Hour))
	if s := r.Snapshot()[0].Slots[0]; !s.Furthed || s.ItemFurthed != 6400 {
		t.Fatalf("slot apos roubo = %+v", s)
	}
	r.Insert(1, 0, 6400, 6400, Effect{7, 15}, "Beta", t0.Add(4*time.Hour)) // a reliquia volta
	if s := r.Snapshot()[0].Slots[0]; s.Furthed || s.ItemFurthed != 0 {
		t.Errorf("a marca de roubada deveria ter sido limpa: %+v", s)
	}
}

func TestUndoInsertRestauraComPisoNoEfeito(t *testing.T) {
	r := New(1)
	prev, _ := r.Insert(0, 0, 6400, 6400, Effect{7, 15}, "Alfa", t0)
	r.UndoInsert(0, 0, prev, Effect{7, 15})
	if r.Snapshot()[0].Slots[0].ItemID != 0 || r.EffectValue(7) != 0 {
		t.Error("o Insert nao foi desfeito")
	}
	r.UndoInsert(0, 0, prev, Effect{7, 999}) // desconto maior que o somado: piso 0
	if r.EffectValue(7) != 0 {
		t.Errorf("efeito = %d, queria piso 0", r.EffectValue(7))
	}
}

func TestPullSoDepoisDeLiberada(t *testing.T) {
	r := New(1)
	r.Insert(0, 0, 6400, 6400, Effect{7, 15}, "Alfa", t0)
	ef := func(uint16) Effect { return Effect{7, 15} }

	if _, _, err := r.Pull(0, 0, ef, t0.Add(2*time.Hour-time.Minute)); !errors.Is(err, ErrNotReleased) {
		t.Errorf("antes de liberar: %v", err)
	}
	taken, undo, err := r.Pull(0, 0, ef, t0.Add(2*time.Hour))
	if err != nil || taken.ItemID != 6400 {
		t.Fatalf("Pull = %+v %v", taken, err)
	}
	s := r.Snapshot()[0]
	if s.Slots[0].ItemID != 0 || !s.Slots[0].Furthed || s.Slots[0].ItemFurthed != 6400 || s.Slots[0].Name != "" || !s.Collected {
		t.Errorf("apos roubo = %+v", s.Slots[0])
	}
	if r.EffectValue(7) != 0 {
		t.Errorf("efeito = %d, queria 0", r.EffectValue(7))
	}
	undo()
	if r.Snapshot()[0].Slots[0].ItemID != 6400 || r.EffectValue(7) != 15 {
		t.Error("undo nao restaurou o slot e o efeito")
	}
}

// Defeito da origem: SrcSlot (WORD) indexava Slots sem faixa e puxar de slot
// vazio entregava o item 0.
func TestPullRecusaSlotVazioEForaDaFaixa(t *testing.T) {
	r := New(1)
	ef := func(uint16) Effect { return Effect{} }
	if _, _, err := r.Pull(0, 3, ef, t0); !errors.Is(err, ErrSlotEmpty) {
		t.Errorf("slot vazio: %v", err)
	}
	for _, c := range [][2]int{{0, 5}, {0, 65535}, {5, 0}, {-1, 0}} {
		if _, _, err := r.Pull(c[0], c[1], ef, t0); !errors.Is(err, ErrBadSlot) {
			t.Errorf("Pull%v: %v", c, err)
		}
	}
}

func TestRegistroEmParalelo(t *testing.T) {
	r := New(1)
	done := make(chan struct{})
	for g := 0; g < 4; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				r.FreeSpace(g)
				r.Snapshot()
				r.Rows()
				if _, err := r.Insert(g, i%SlotsPerTemple, 6400, 6400, Effect{7, 1}, "x", t0); err == nil {
					r.Pull(g, i%SlotsPerTemple, func(uint16) Effect { return Effect{7, 1} }, t0.Add(10*time.Hour))
				}
			}
		}(g)
	}
	for g := 0; g < 4; g++ {
		<-done
	}
}
