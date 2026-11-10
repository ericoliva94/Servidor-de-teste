// Package siege tem as regras do cerco de castelo: quem esta cadastrado, quem
// pode entrar e por onde, as orbes, o selo do marechal e o fim da guerra.
//
// Porte de TNPCHandlers.SignInCastle e EnterInCastle, de TCastleSiege e de
// TCastleSiegeThread (UpdateThreads.pas). O pacote nao sabe de jogadores nem de
// rede: o chamador diz quem segura o que e recebe de volta o que aconteceu.
package siege

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Limites da guerra.
const (
	// MaxAttackers e o numero de aliancas atacantes (Atacantes[0..3]).
	MaxAttackers = 4
	// WarDuration e o tempo maximo da guerra: passou disto sem vencedor, a
	// defesa sustenta o poder.
	WarDuration = 60 * time.Minute
	// SealDuration e o tempo que o lider da alianca segura o selo para vencer.
	SealDuration = 120 * time.Second
	// Orbs e o numero de orbes (agua, vento e fogo).
	Orbs = 3
)

// Posicoes do castelo (X, Y) e a tolerancia, em celulas, de cada uma.
var (
	OrbPos    = [Orbs][2]float32{{3551, 2759}, {3616, 2759}, {3584, 2860}}
	SealPos   = [2]float32{3584, 2804.75}
	OrbRange  = float32(1)
	SealRange = float32(3)
)

// Pontos de entrada (a defesa nasce perto do que protege) e a "nacao" que o
// cliente usa para pintar o jogador no castelo (NationForCastle).
type Entry struct {
	X, Y   float32
	Nation uint8
}

var (
	// AttackEntry e o ponto de cada slot de atacante.
	AttackEntry = [MaxAttackers]Entry{{3827, 2883, 7}, {3341, 2883, 8}, {3434, 2598, 9}, {3734, 2598, 10}}
	// DefenseEntry e o ponto de cada guilda da defesa, por cargo.
	DefenseMarshal   = Entry{3583, 2804, 6}
	DefenseTactician = Entry{3606, 2900, 6}
	DefenseJudge     = Entry{3659, 2742, 6}
	DefenseTreasurer = Entry{3548, 2714, 6}
)

// Erros de cadastro e de entrada.
var (
	ErrNotAllyLeader = errors.New("siege: so o lider da alianca cadastra")
	ErrRegistered    = errors.New("siege: alianca ja cadastrada na guerra")
	ErrIsDefender    = errors.New("siege: alianca ja cadastrada como defesa")
	ErrNoSlot        = errors.New("siege: sem espaco para novas aliancas atacantes")
	ErrWarRunning    = errors.New("siege: guerra em andamento")
	ErrNotRegistered = errors.New("siege: guilda nao cadastrada na guerra")
	ErrNotStarted    = errors.New("siege: a guerra ainda nao comecou")
)

// Alliance e uma alianca no cerco: os nomes das quatro guildas (o primeiro e o
// da lider). Vazio = sem guilda no cargo. Porte de TGuildsAlly.
type Alliance struct {
	Marshal, Tactician, Judge, Treasurer string
}

// Empty diz se a alianca nao tem ninguem.
func (a Alliance) Empty() bool {
	return a.Marshal == "" && a.Tactician == "" && a.Judge == "" && a.Treasurer == ""
}

// Names devolve os nomes nao vazios.
func (a Alliance) Names() []string {
	var out []string
	for _, n := range [4]string{a.Marshal, a.Tactician, a.Judge, a.Treasurer} {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Has diz se a guilda esta na alianca.
func (a Alliance) Has(name string) bool {
	for _, n := range a.Names() {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// Cerco e o cadastro de uma nacao: a defesa (a alianca do marechal) e ate
// quatro aliancas atacantes. Porte de TNationCerco.
type Cerco struct {
	Defenders Alliance
	Attackers [MaxAttackers]Alliance
}

// Status e a fase da guerra (TSiegeStatus).
type Status int

const (
	StatusNone     Status = iota
	StatusRunning         // guerra aberta, orbes por tomar
	StatusOrbsHeld        // as tres orbes estao seguras; falta o selo
	StatusSealing         // o lider da alianca segura o selo
)

// OutcomeKind diz como a guerra acabou.
type OutcomeKind int

const (
	NoOutcome    OutcomeKind = iota
	NoWinner                 // passou de 60 min: a defesa se mantem
	AttackersWin             // o selo foi mantido por 120 s
)

// Outcome e o fim da guerra.
type Outcome struct {
	Kind OutcomeKind
	// Winner e a alianca vencedora (com AttackersWin): quem segurou o selo.
	Winner Alliance
}

// War e a guerra de um canal. Seguro para uso concorrente.
type War struct {
	mu sync.Mutex

	cerco Cerco

	inProgress bool
	started    time.Time
	status     Status

	orb          [Orbs]uint16 // ClientID de quem segura cada orbe; 0 = ninguem
	sealHolder   uint16
	sealSince    time.Time
	sealAlliance Alliance // alianca de quem segura o selo
}

// Cerco devolve uma copia do cadastro.
func (w *War) Cerco() Cerco {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cerco
}

// SetCerco troca o cadastro (carga do banco).
func (w *War) SetCerco(c Cerco) {
	w.mu.Lock()
	w.cerco = c
	w.mu.Unlock()
}

// InProgress diz se ha guerra.
func (w *War) InProgress() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.inProgress
}

// Status devolve a fase.
func (w *War) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// RegisterAttacker cadastra uma alianca atacante. Porte de SignInCastle.
// names sao as guildas da alianca (lider primeiro, ate quatro). O dia da semana
// e do chamador (de segunda a sexta).
//
// Regras: a alianca nao pode ja estar entre as atacantes nem ser a defesa, tem
// de haver slot livre e a guerra nao pode estar em andamento.
func (w *War) RegisterAttacker(a Alliance) (slot int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	slot = -1
	for i := range w.cerco.Attackers {
		cur := w.cerco.Attackers[i]
		if cur.Marshal != "" {
			if strings.EqualFold(cur.Marshal, a.Marshal) {
				return -1, ErrRegistered
			}
		} else {
			slot = i
		}
		if d := w.cerco.Defenders.Marshal; d != "" && strings.EqualFold(d, a.Marshal) {
			return -1, ErrIsDefender
		}
	}
	if slot < 0 {
		return -1, ErrNoSlot
	}
	if w.inProgress {
		return -1, ErrWarRunning
	}
	w.cerco.Attackers[slot] = a
	return slot, nil
}

// Entry resolve o direito de entrar da guilda e o ponto de nascimento. Porte de
// EnterInCastle: atacante entra pelo ponto do slot dela, defesa pelo ponto do
// cargo. Sem cadastro: ErrNotRegistered.
func (w *War) Entry(guild string) (Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, a := range w.cerco.Attackers {
		if a.Has(guild) {
			return AttackEntry[i], nil
		}
	}
	d := w.cerco.Defenders
	switch {
	case strings.EqualFold(d.Marshal, guild) && d.Marshal != "":
		return DefenseMarshal, nil
	case strings.EqualFold(d.Tactician, guild) && d.Tactician != "":
		return DefenseTactician, nil
	case strings.EqualFold(d.Judge, guild) && d.Judge != "":
		return DefenseJudge, nil
	case strings.EqualFold(d.Treasurer, guild) && d.Treasurer != "":
		return DefenseTreasurer, nil
	}
	return Entry{}, ErrNotRegistered
}

// Registered diz se a guilda esta no cadastro, como atacante ou defesa.
func (w *War) Registered(guild string) bool {
	_, err := w.Entry(guild)
	return err == nil
}

// Start abre a guerra. Falso se ja estava aberta.
func (w *War) Start(now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inProgress {
		return false
	}
	w.inProgress = true
	w.started = now
	w.status = StatusRunning
	w.orb = [Orbs]uint16{}
	w.sealHolder = 0
	return true
}

// HoldOrb poe o jogador na orbe (0 a 2). Falso fora da guerra ou fora da faixa.
func (w *War) HoldOrb(i int, clientID uint16) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.inProgress || i < 0 || i >= Orbs || clientID == 0 {
		return false
	}
	w.orb[i] = clientID
	w.recount()
	return true
}

// OrbHolder devolve quem segura a orbe, ou 0.
func (w *War) OrbHolder(i int) uint16 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if i < 0 || i >= Orbs {
		return 0
	}
	return w.orb[i]
}

// OrbsHeld conta as orbes seguras.
func (w *War) OrbsHeld() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.orbsHeld()
}

func (w *War) orbsHeld() int {
	n := 0
	for _, id := range w.orb {
		if id != 0 {
			n++
		}
	}
	return n
}

// recount refaz a fase depois de uma mudanca de orbes ou selo, como a soma
// de CountOrbsHolding, RemoveOrbHolder e RemoveSealHolder.
func (w *War) recount() {
	switch {
	case w.sealHolder != 0 && w.orbsHeld() == Orbs:
		w.status = StatusSealing
	case w.orbsHeld() == Orbs:
		w.status = StatusOrbsHeld
	default:
		w.status = StatusRunning
		// Sem as tres orbes o selo nao vale.
		w.sealHolder = 0
	}
}

// ReleaseOrb tira quem segurava a orbe. Devolve o ClientID que saiu (0 se nao
// havia). O chamador manda o sinal $33A ao jogador (cancela a coleta).
func (w *War) ReleaseOrb(i int) uint16 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if i < 0 || i >= Orbs {
		return 0
	}
	id := w.orb[i]
	w.orb[i] = 0
	w.recount()
	return id
}

// BeginSeal poe o lider da alianca no selo. Exige as tres orbes seguras; a
// alianca dele (nomes das guildas) e a que vence se ele segurar ate o fim.
func (w *War) BeginSeal(clientID uint16, a Alliance, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.inProgress || w.orbsHeld() < Orbs || clientID == 0 {
		return false
	}
	w.sealHolder, w.sealSince, w.sealAlliance = clientID, now, a
	w.recount()
	return true
}

// SealHolder devolve quem segura o selo, ou 0.
func (w *War) SealHolder() uint16 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sealHolder
}

// ReleaseSeal tira quem segurava o selo e devolve o ClientID que saiu.
func (w *War) ReleaseSeal() uint16 {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := w.sealHolder
	w.sealHolder = 0
	w.recount()
	return id
}

// SealRemaining devolve quantos segundos faltam para o selo valer, ou -1 sem
// selo. Serve ao aviso "Selo do Marechal: N seg. restantes".
func (w *War) SealRemaining(now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sealHolder == 0 {
		return -1
	}
	left := int(SealDuration.Seconds()) - int(now.Sub(w.sealSince).Seconds())
	return max(left, 0)
}

// Tick e UpdateSiegeStatus: confere o fim da guerra. Devolve o resultado quando
// ela acaba (e a fecha); NoOutcome enquanto segue.
func (w *War) Tick(now time.Time) Outcome {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.inProgress {
		return Outcome{}
	}
	if now.Sub(w.started) >= WarDuration {
		w.finishLocked(false)
		return Outcome{Kind: NoWinner}
	}
	if w.sealHolder != 0 && now.Sub(w.sealSince) >= SealDuration {
		winner := w.sealAlliance
		w.finishLocked(true)
		return Outcome{Kind: AttackersWin, Winner: winner}
	}
	return Outcome{}
}

// finishLocked encerra a guerra: orbes e selo soltos e atacantes limpos. Com
// vencedor, o cadastro da defesa vira o dele (o chamador faz o resto).
func (w *War) finishLocked(conquered bool) {
	w.inProgress = false
	w.status = StatusNone
	w.orb = [Orbs]uint16{}
	if conquered {
		w.cerco.Defenders = w.sealAlliance
	}
	w.sealHolder = 0
	w.cerco.Attackers = [MaxAttackers]Alliance{}
}
