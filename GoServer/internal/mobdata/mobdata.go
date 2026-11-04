// Package mobdata le as duas tabelas CSV de mob do servidor.
//
// Porte de TServerSocket.LoadMobs (ServerSocket.pas:585-860), que lia
// Data/Mobs/MonsterListCSV.csv (um ponto de spawn por linha) e
// Data/Mobs/AllMobsInfo.csv (uma especie por linha).
package mobdata

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aika/internal/model"
)

// Limites dos vetores da origem, de MOB.pas.
//
//	MobsP: Array [0 .. 49] of TMobSPosition
//	TMobS: Array [0 .. 449] of TMobSa
//
// A origem indexava os dois sem nenhuma checagem: uma especie com mais de 49
// pontos de spawn, ou a 451a especie no arquivo, escrevia fora do vetor. Os
// dados de hoje chegam exatamente no limite (372 especies, 49 pontos na maior
// delas), entao bastava um ponto a mais em qualquer especie para corromper
// memoria em silencio. Aqui o excedente e recusado e reportado.
const (
	MaxSpawnsPerSpecies = 49
	MaxSpecies          = 450
)

// FirstMobClientID e o 3048 de "MobsP[idGen].index := Count + 3048".
const FirstMobClientID = 3048

// DefaultHP e o 3500 que a origem punha quando a coluna de HP vinha zerada.
const DefaultHP = 3500

// Species e uma especie de mob: o que vale para todos os seus pontos.
type Species struct {
	// Index e a posicao da especie na tabela, o IndexGeneric da origem.
	Index int
	// Name e o nome com underscore, como vem nos dois arquivos.
	Name string
	// IntName e o nome numerico que o cliente usa para desenhar.
	IntName uint16

	Equip [13]uint16

	HP        uint32
	Rotation  uint32
	Level     uint16
	Elevation uint8
	Cabeca    uint8
	Perna     uint8
	MobType   uint16
	SpawnType uint8
	IsService bool

	AtkFis uint32
	AtkMag uint32
	DefFis uint32
	DefMag uint32

	RespawnSeconds uint16
	Skills         [5]uint16
	Exp            uint32
	DropIndex      uint16
	ActiveToSpawn  bool

	// MoveSpeed e o 22 fixo da origem.
	MoveSpeed uint16

	// Mutant marca quem nao anda. A origem decidia pelo nome: "Mutante" nao
	// anda e e marcado, "Crenon" nao anda e nao e marcado.
	Mutant bool

	// Spawns sao os pontos em que esta especie nasce.
	Spawns []Spawn
}

// Spawn e um ponto de nascimento de mob.
type Spawn struct {
	// ClientID e o id que vai nos pacotes, 3048 + a ordem da linha.
	ClientID uint16
	// SecondIndex e o indice do ponto dentro da especie (o idGen da origem,
	// que comeca em 1).
	SecondIndex int

	InitPos model.Position
	DestPos model.Position
	// FirstDestPos e o destino sem o deslocamento de 5 celulas que a origem
	// somava a DestPos.
	FirstDestPos model.Position

	InitAttackRange int
	InitMoveWait    int
	DestAttackRange int
	DestMoveWait    int
}

// Table e o resultado da carga.
type Table struct {
	// Species esta na ordem em que as especies aparecem no MonsterList.
	Species []Species
	// ByName acha a especie pelo nome com underscore.
	ByName map[string]int
	// ByClientID acha a especie e o ponto de um ClientID de mob.
	ByClientID map[uint16]Ref
	// Avisos descreve o que foi recusado.
	Avisos []string
}

// Ref aponta para um ponto de spawn.
type Ref struct {
	Species int
	Spawn   int
}

// ErrNotFound indica arquivo ausente. Como em Load.pas, isso e aviso e nao
// erro fatal: o servidor sobe sem mobs.
var ErrNotFound = errors.New("mobdata: arquivo nao encontrado")

// Load le as duas tabelas da pasta Data/Mobs.
func Load(dataDir string) (*Table, error) {
	mobs := filepath.Join(dataDir, "Mobs")
	t, err := LoadSpawns(filepath.Join(mobs, "MonsterListCSV.csv"))
	if err != nil {
		return t, err
	}
	if err := t.LoadInfo(filepath.Join(mobs, "AllMobsInfo.csv")); err != nil {
		return t, err
	}
	return t, nil
}

// LoadSpawns le o MonsterListCSV.csv.
//
// O formato tem as colunas uteis em 4 (nome), 9 e 10 (posicao inicial), 11 e
// 12 (alcance e espera na inicial), 14 e 15 (destino), 16 e 17 (alcance e
// espera no destino).
func LoadSpawns(path string) (*Table, error) {
	t := &Table{
		ByName:     make(map[string]int, 400),
		ByClientID: make(map[uint16]Ref, 6000),
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return t, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return t, fmt.Errorf("mobdata: abrir %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	linha := 0
	contador := 0 // o "Count" da origem, que nunca reinicia
	for sc.Scan() {
		linha++
		texto := strings.TrimSpace(sc.Text())
		if texto == "" {
			continue
		}
		col := strings.Split(texto, ",")
		if len(col) < 18 {
			// A origem chamava FileStrings[17] sem checar quantas colunas
			// vieram: uma linha curta levantava excecao no meio da carga e o
			// servidor subia sem o resto dos mobs.
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: %d colunas, precisa de 18", filepath.Base(path), linha, len(col)))
			continue
		}

		nome := strings.TrimSpace(col[4])
		if nome == "" {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: nome vazio", filepath.Base(path), linha))
			continue
		}

		idx, existe := t.ByName[nome]
		if !existe {
			if len(t.Species) >= MaxSpecies {
				t.Avisos = append(t.Avisos, fmt.Sprintf(
					"%s:%d: especie %q passa do limite de %d, ignorada",
					filepath.Base(path), linha, nome, MaxSpecies))
				contador++
				continue
			}
			idx = len(t.Species)
			t.Species = append(t.Species, Species{
				Index: idx, Name: nome, MoveSpeed: 22,
				Mutant: strings.Contains(nome, "Mutante"),
			})
			t.ByName[nome] = idx
		}
		sp := &t.Species[idx]

		if len(sp.Spawns) >= MaxSpawnsPerSpecies {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: especie %q passa de %d pontos de spawn, ponto ignorado",
				filepath.Base(path), linha, nome, MaxSpawnsPerSpecies))
			contador++
			continue
		}

		clientID := FirstMobClientID + contador
		contador++
		if clientID > 65535 {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: ClientID %d nao cabe num WORD, ponto ignorado",
				filepath.Base(path), linha, clientID))
			continue
		}

		s := Spawn{
			ClientID:    uint16(clientID),
			SecondIndex: len(sp.Spawns) + 1, // o idGen da origem comeca em 1
			InitPos: model.Position{
				X: parseFloat(col[9]), Y: parseFloat(col[10])},
			InitAttackRange: parseInt(col[11]),
			InitMoveWait:    parseInt(col[12]),
			DestAttackRange: parseInt(col[16]),
			DestMoveWait:    parseInt(col[17]),
		}

		// Mutante e Crenon ficam parados: o destino e a propria posicao.
		// A origem decidia isso pelo nome, e so o Mutante recebia a marca.
		parado := strings.Contains(nome, "Mutante") || strings.Contains(nome, "Crenon")
		if parado {
			s.DestPos = s.InitPos
			s.FirstDestPos = s.InitPos
		} else {
			s.FirstDestPos = model.Position{
				X: parseFloat(col[14]), Y: parseFloat(col[15])}
			// A origem somava 5 celulas ao destino.
			s.DestPos = model.Position{
				X: s.FirstDestPos.X + 5, Y: s.FirstDestPos.Y + 5}
		}

		sp.Spawns = append(sp.Spawns, s)
		t.ByClientID[s.ClientID] = Ref{Species: idx, Spawn: len(sp.Spawns) - 1}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return t, fmt.Errorf("mobdata: ler %s: %w", path, err)
	}
	return t, nil
}

// LoadInfo le o AllMobsInfo.csv e completa as especies ja carregadas.
//
// A origem varria as 450 posicoes do vetor comparando o nome a cada linha —
// 450 x 373 comparacoes. Aqui a busca e por indice.
//
// Especie que esta num arquivo e nao no outro: a origem simplesmente nao
// casava e seguia, e a especie ficava com InitHP zero (o piso de 3500 estava
// dentro do ramo que casou), isto e, nascia morta. Aqui o caso e reportado.
func (t *Table) LoadInfo(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return fmt.Errorf("mobdata: abrir %s: %w", path, err)
	}
	defer f.Close()

	preenchidas := make(map[int]bool, len(t.Species))
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	linha := 0
	for sc.Scan() {
		linha++
		texto := strings.TrimSpace(sc.Text())
		if texto == "" {
			continue
		}
		col := strings.Split(texto, ",")
		if len(col) < 27 {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: %d colunas, precisa de 27", filepath.Base(path), linha, len(col)))
			continue
		}

		// A origem trocava espaco por underscore para casar com o outro
		// arquivo, que ja vem com underscore.
		nome := strings.ReplaceAll(strings.TrimSpace(col[1]), " ", "_")
		idx, ok := t.ByName[nome]
		if !ok {
			t.Avisos = append(t.Avisos, fmt.Sprintf(
				"%s:%d: especie %q tem atributos mas nao tem ponto de spawn",
				filepath.Base(path), linha, nome))
			continue
		}
		sp := &t.Species[idx]
		preenchidas[idx] = true

		sp.IntName = uint16(parseInt(col[0]))
		sp.Equip[0] = uint16(parseInt(col[2]))
		sp.Equip[1] = uint16(parseInt(col[3]))
		sp.Equip[6] = uint16(parseInt(col[4]))

		sp.HP = uint32(parseInt(col[5]))
		if sp.HP == 0 {
			sp.HP = DefaultHP
		}
		sp.Rotation = uint32(parseInt(col[6]))
		sp.Level = uint16(parseInt(col[7]))
		sp.Elevation = uint8(parseInt(col[8]))
		sp.Cabeca = uint8(parseInt(col[9]))
		sp.Perna = uint8(parseInt(col[10]))
		sp.MobType = uint16(parseInt(col[11]))
		sp.SpawnType = uint8(parseInt(col[12]))
		sp.IsService = parseInt(col[13]) != 0

		sp.AtkFis = uint32(parseInt(col[14]))
		sp.AtkMag = uint32(parseInt(col[15]))
		sp.DefFis = uint32(parseInt(col[16]))
		sp.DefMag = uint32(parseInt(col[17]))

		sp.RespawnSeconds = uint16(parseInt(col[18]))
		for i := range sp.Skills {
			sp.Skills[i] = uint16(parseInt(col[19+i]))
		}
		sp.Exp = uint32(parseInt(col[24]))
		sp.DropIndex = uint16(parseInt(col[25]))
		sp.ActiveToSpawn = parseInt(col[26]) != 0
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("mobdata: ler %s: %w", path, err)
	}

	// Especie com ponto de spawn e sem atributos nasceria com HP zero.
	for i := range t.Species {
		if preenchidas[i] {
			continue
		}
		t.Avisos = append(t.Avisos, fmt.Sprintf(
			"especie %q tem %d pontos de spawn e nenhum atributo: HP vai como %d",
			t.Species[i].Name, len(t.Species[i].Spawns), DefaultHP))
		if t.Species[i].HP == 0 {
			t.Species[i].HP = DefaultHP
		}
	}
	return nil
}

// SpawnCount conta os pontos de spawn carregados.
func (t *Table) SpawnCount() int {
	n := 0
	for i := range t.Species {
		n += len(t.Species[i].Spawns)
	}
	return n
}

// Lookup devolve a especie e o ponto de um ClientID de mob.
func (t *Table) Lookup(clientID uint16) (*Species, *Spawn, bool) {
	if t == nil {
		return nil, nil, false
	}
	r, ok := t.ByClientID[clientID]
	if !ok {
		return nil, nil, false
	}
	if r.Species < 0 || r.Species >= len(t.Species) {
		return nil, nil, false
	}
	sp := &t.Species[r.Species]
	if r.Spawn < 0 || r.Spawn >= len(sp.Spawns) {
		return nil, nil, false
	}
	return sp, &sp.Spawns[r.Spawn], true
}

// parseInt le um inteiro tolerando espaco e campo vazio.
//
// A origem usava ToInteger(), que levanta excecao em campo invalido e
// derrubava a carga inteira. Aqui campo invalido vale zero.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(v)
}

// parseFloat le um Single tolerando espaco e campo vazio.
func parseFloat(s string) float32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0
	}
	return float32(v)
}
