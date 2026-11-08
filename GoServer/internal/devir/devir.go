// Package devir guarda os templos (devires) de uma nacao e as reliquias nos
// slots deles: quem capturou, quando, e os efeitos que cada reliquia soma.
//
// Porte do estado de TDevir/TDevirSlot (MiscData.pas), de
// TServerSocket.GetFreeTempleSpace, do carregamento em Nation.pas e das
// regras de TPacketHandlers.MoveItemToReliquare. Nao faz I/O de banco nem de
// rede. A abertura do templo (pedras, guardas, NPCs) NAO esta aqui.
package devir

import (
	"errors"
	"sync"
	"time"
)

// Tamanhos e tempos da origem.
const (
	TemplesPerNation = 5
	SlotsPerTemple   = 5
	// EstHours e RELIQ_EST_TIME: horas entre a captura e a reliquia fixar.
	EstHours = 2
	// CaptureDelay sao as 3 horas que a origem soma a captura (IncHour(Now, 3)).
	CaptureDelay = 3 * time.Hour
	// MinValidUnix: timecap abaixo disso e lixo (ver "Docs bugs/Relics SQL.txt").
	MinValidUnix = 1_000_000_000
)

// Erros das operacoes.
var (
	ErrBadSlot     = errors.New("devir: templo ou slot invalido")
	ErrSlotBusy    = errors.New("devir: slot ocupado ou desabilitado")
	ErrSlotEmpty   = errors.New("devir: slot sem reliquia")
	ErrNotReleased = errors.New("devir: reliquia ainda nao liberada")
)

// Slot e um espaco de reliquia (TDevirSlot).
type Slot struct {
	ItemID          uint16
	APP             uint16
	Able            bool
	TimeCapped      time.Time
	TimeToEstablish time.Time
	Name            string
	TimeFurthed     time.Time
	ItemFurthed     uint16
	Furthed         bool
}

// Temple e um devir (TDevir, so o que se usa aqui).
type Temple struct {
	DevirID   int
	Slots     [SlotsPerTemple]Slot
	Collected bool
}

// Row e a linha da tabela devires.
type Row struct {
	DevirID int
	Slots   [SlotsPerTemple]RowSlot
}

// RowSlot sao as quatro colunas de cada slot.
type RowSlot struct {
	ItemID  int
	Name    string
	TimeCap int64
	Able    bool
}

// Effect e o efeito de uma reliquia: EF[0] e EFV[0] do item.
type Effect struct {
	Index uint16
	Value int
}

// Registry e o estado dos 5 templos de uma nacao e dos efeitos somados.
type Registry struct {
	mu      sync.RWMutex
	Nation  uint8
	temples [TemplesPerNation]Temple
	effect  map[uint16]int
}

// New cria o registro de uma nacao.
func New(nation uint8) *Registry {
	r := &Registry{Nation: nation, effect: map[uint16]int{}}
	for i := range r.temples {
		r.temples[i].DevirID = FirstDevirID(nation) + i
		for j := range r.temples[i].Slots {
			r.temples[i].Slots[j].Able = true
		}
	}
	return r
}

// FirstDevirID e o devir_id do primeiro templo da nacao (nacao 1 -> 1..5,
// nacao 2 -> 6..10, ...), como no comentario do schema.
func FirstDevirID(nation uint8) int {
	if nation == 0 {
		return 1
	}
	return (int(nation)-1)*TemplesPerNation + 1
}

func validIdx(d, s int) bool {
	return d >= 0 && d < TemplesPerNation && s >= 0 && s < SlotsPerTemple
}

// Load carrega as linhas do banco, como Nation.pas: timecap invalido (abaixo
// de MinValidUnix) vale "sem captura". effectOf devolve o efeito de um item;
// o efeito so entra para slot com reliquia e captura valida.
func (r *Registry) Load(rows []Row, effectOf func(item uint16) (Effect, bool)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.effect = map[uint16]int{}
	first := FirstDevirID(r.Nation)
	for _, row := range rows {
		d := row.DevirID - first
		if d < 0 || d >= TemplesPerNation {
			continue
		}
		t := &r.temples[d]
		t.DevirID = row.DevirID
		for i, rs := range row.Slots {
			s := &t.Slots[i]
			*s = Slot{Able: rs.Able, Name: rs.Name}
			if rs.ItemID <= 0 || rs.ItemID > 0xFFFF {
				continue
			}
			s.ItemID, s.APP = uint16(rs.ItemID), uint16(rs.ItemID)
			if rs.TimeCap >= MinValidUnix {
				s.TimeCapped = time.Unix(rs.TimeCap, 0)
				s.TimeToEstablish = s.TimeCapped.Add(EstHours * time.Hour)
				if e, ok := effectOf(s.ItemID); ok {
					r.effect[e.Index] += e.Value
				}
			}
		}
	}
}

// Rows devolve o estado em linhas, para gravar.
func (r *Registry) Rows() []Row {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Row, TemplesPerNation)
	for d := range r.temples {
		out[d].DevirID = r.temples[d].DevirID
		for i, s := range r.temples[d].Slots {
			rs := RowSlot{ItemID: int(s.ItemID), Name: s.Name, Able: s.Able}
			if s.ItemID != 0 && !s.TimeCapped.IsZero() {
				rs.TimeCap = s.TimeCapped.Unix()
			}
			out[d].Slots[i] = rs
		}
	}
	return out
}

// Snapshot copia os templos.
func (r *Registry) Snapshot() [TemplesPerNation]Temple {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.temples
}

// EffectValue devolve o efeito somado de um indice.
func (r *Registry) EffectValue(idx uint16) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effect[idx]
}

// FreeSpace e GetFreeTempleSpace: o primeiro slot habilitado e vazio.
func (r *Registry) FreeSpace(d int) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if d < 0 || d >= TemplesPerNation {
		return 0, false
	}
	for i, s := range r.temples[d].Slots {
		if s.Able && s.ItemID == 0 {
			return i, true
		}
	}
	return 0, false
}

func (r *Registry) addEffect(e Effect) { r.effect[e.Index] += e.Value }

// subEffect desconta com piso em zero, como DecInt.
func (r *Registry) subEffect(e Effect) {
	v := r.effect[e.Index] - e.Value
	if v < 0 {
		v = 0
	}
	r.effect[e.Index] = v
}

// Insert poe a reliquia no slot. Devolve o conteudo anterior, para desfazer
// se a gravacao falhar.
func (r *Registry) Insert(d, s int, item, app uint16, eff Effect, name string, now time.Time) (Slot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !validIdx(d, s) {
		return Slot{}, ErrBadSlot
	}
	slot := &r.temples[d].Slots[s]
	if !slot.Able || slot.ItemID != 0 {
		return Slot{}, ErrSlotBusy
	}
	prev := *slot
	// A reliquia que o jogador traz deixa de estar "roubada" em qualquer templo
	// (a origem varria todos os templos de todos os servidores).
	for i := range r.temples {
		for j := range r.temples[i].Slots {
			if f := &r.temples[i].Slots[j]; f.Furthed && f.ItemFurthed == item {
				f.Furthed, f.ItemFurthed = false, 0
			}
		}
	}
	slot.ItemID, slot.APP = item, app
	slot.TimeCapped = now.Add(CaptureDelay)
	slot.TimeToEstablish = now.Add(CaptureDelay + EstHours*time.Hour)
	slot.Name = name
	slot.Furthed, slot.ItemFurthed = false, 0
	r.addEffect(eff)
	return prev, nil
}

// UndoInsert desfaz Insert.
func (r *Registry) UndoInsert(d, s int, prev Slot, eff Effect) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !validIdx(d, s) {
		return
	}
	r.temples[d].Slots[s] = prev
	r.subEffect(eff)
}

// Pull tira a reliquia do slot (roubo). So vale quando ja foi liberada: a
// liberacao e TimeToEstablish menos as 3 horas de captura.
func (r *Registry) Pull(d, s int, eff func(item uint16) Effect, now time.Time) (taken Slot, undo func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !validIdx(d, s) {
		return Slot{}, nil, ErrBadSlot
	}
	slot := &r.temples[d].Slots[s]
	if slot.ItemID == 0 {
		return Slot{}, nil, ErrSlotEmpty
	}
	if slot.TimeToEstablish.Add(-CaptureDelay).After(now) {
		return Slot{}, nil, ErrNotReleased
	}
	prev := *slot
	e := eff(slot.ItemID)
	slot.ItemFurthed = slot.ItemID
	slot.ItemID, slot.APP = 0, 0
	slot.TimeToEstablish = now.Add(CaptureDelay)
	slot.TimeFurthed = now.Add(CaptureDelay)
	slot.Name = ""
	slot.Furthed = true
	r.temples[d].Collected = true
	r.subEffect(e)
	return prev, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.temples[d].Slots[s] = prev
		r.addEffect(e)
	}, nil
}
