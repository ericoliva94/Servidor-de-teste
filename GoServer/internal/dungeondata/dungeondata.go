// Package dungeondata le as tabelas das dungeons: Dungeons.csv, os
// MobInfo_*/MobsPosition_* de cada dungeon e dificuldade, e as listas de drop
// por coroa.
//
// Porte de TServerSocket.InitDungeons (ServerSocket.pas:906-2198).
//
// Tres coisas da origem nao foram portadas ao pe da letra, e as duas primeiras
// sao o motivo pelo qual a dungeon nao funciona na origem:
//
//  1. Os cinco vetores de dungeon sao "Array [0..0] of TDungeon"
//     (ServerSocket.pas:63-67) e o laco de carga e "for i := 0 to 2 do"
//     (linha 935): a carga escreve DUAS fichas inteiras de dungeon fora do
//     vetor. TDungeon tem 45 especies de 50 pontos cada, entao sao megabytes
//     gravados por cima de Parties, Devires, SecureAreas, CastleObjects e dos
//     ponteiros de thread que vem depois no objeto. Aqui cada dungeon guarda
//     as tres dificuldades num slice indexado com checagem.
//
//  2. Por isso mesmo, a chamada "Self.InitDungeons" esta comentada
//     (ServerSocket.pas:493): no binario de hoje nenhuma dungeon e carregada.
//     Todo DGUrsula[x] lido em jogo devolve zeros, e entrar numa dungeon
//     teleportava o grupo para a posicao (0,0).
//
//  3. A coluna 7 do MobInfo, que a origem le como nivel do mob, vale de 1288 a
//     1346 nos arquivos reais — a mesma faixa de lixo que 25 linhas do
//     AllMobsInfo.csv do mundo tem. Nao existe coluna de nivel aproveitavel
//     nesses arquivos. O valor e lido como na origem e so serve para a cor do
//     nome no cliente ("for a name collor on client", MOB.pas:322); quem
//     precisa de nivel para valer usa LevelMin da dungeon.
package dungeondata

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aika/internal/model"
)

// Ids de dungeon, de GlobalDefs.pas.
const (
	ZantorianCitadel = 1 // Ursula
	MarauderHold     = 2 // Evg Inf
	MarauderCabin    = 3 // Evg Sup
	LostMines        = 4 // Mines 1
	KinaryAviary     = 5 // Kinary
)

// Dificuldades. A origem carrega 0, 1 e 2 e so a 0 cabe no vetor.
const (
	Normal = 0
	Hard   = 1
	Elite  = 2
)

// Difficulties e quantas dificuldades cada dungeon tem.
const Difficulties = 3

// Limites dos vetores da origem.
//
//	MobsP: Array [0 .. 49]   -> 50 pontos por especie
//	TMobS: Array [0 .. 44]   -> 45 especies por dungeon
//	Drops: Array [0 .. 41]   -> 42 itens por lista de drop
//	MOBS:  Array [1 .. 450]  -> 450 mobs por instancia
//
// A origem indexava os quatro sem checagem nenhuma. Os dados de hoje passam
// raspando: Kinary tem 44 especies e 414 pontos, e 603.txt tem exatamente 42
// linhas. Uma linha a mais em qualquer um desses arquivos corrompia memoria.
const (
	MaxSpawnsPerSpecies  = 50
	MaxSpeciesPerDungeon = 45
	MaxDropsPerTier      = 42
	MaxMobsPerInstance   = 450
)

// FirstDungeonMobIndex e o 2600 de "IndexGeneric := CountMob + 2600".
const FirstDungeonMobIndex = 2600

// Elevacao, cabeca e perna que a origem fixa para todo mob de dungeon.
const (
	FixedElevation = 7
	FixedCabeca    = 119
	FixedPerna     = 119
	FixedMoveSpeed = 25
)

// FixedAttackRange e o 20 que a origem punha em InitAttackRange e
// DestAttackRange de todo ponto de dungeon.
const FixedAttackRange = 20

// Crown sao as tres listas de drop de cada dungeon e dificuldade. A origem
// chamava SemCoroa, CoroaPrata e CoroaDourada, e escolhia pelo ultimo digito
// do DungeonDropIndex da especie.
type Crown int

const (
	NoCrown Crown = iota
	SilverCrown
	GoldCrown
)

// Species e uma especie de mob de dungeon: uma linha do MobInfo.
type Species struct {
	// Index e a ordem no arquivo, e IndexGeneric e Index + 2600.
	Index        int
	IndexGeneric int

	IntName uint16
	Name    string

	Equip [13]uint16

	HP       uint32
	Rotation uint32
	// Level e a coluna 7, que nao e nivel nenhum nos arquivos reais. Ver o
	// comentario do pacote.
	Level     uint16
	MobType   uint16
	SpawnType uint8

	AtkFis uint32
	AtkMag uint32
	DefFis uint32
	DefMag uint32

	Exp uint32

	// DropIndex e a coluna 18: o nome do arquivo de drop (11, 402, 603...).
	DropIndex uint16

	Spawns []Spawn
}

// Spawn e um ponto de nascimento dentro da dungeon: uma linha do MobsPosition.
type Spawn struct {
	// ClientID e a coluna 0, o id que vai nos pacotes.
	ClientID uint16
	// SecondIndex e a ordem do ponto dentro da especie.
	SecondIndex int

	HP      uint32
	MP      uint32
	InitPos model.Position
}

// Definition e uma dungeon numa dificuldade.
type Definition struct {
	Index    uint32
	Name     string
	Dificult uint8
	LevelMin uint8

	EntranceNPCID uint16
	// EntrancePosition e a posicao do portal, que a origem guardava "pra
	// calcular se todos os players estao la" e nunca usou.
	EntrancePosition       model.Position
	SpawnInDungeonPosition model.Position

	Species []Species

	// Drops[c] e a lista da coroa c. Vazia quando o arquivo nao existe.
	Drops [3][]uint16
}

// SpawnCount soma os pontos de todas as especies.
func (d *Definition) SpawnCount() int {
	n := 0
	for i := range d.Species {
		n += len(d.Species[i].Spawns)
	}
	return n
}

// CrownOf devolve a coroa de um DungeonDropIndex. A origem decidia com
//
//	case DungeonDropIndex of
//	  11, 21, 31: SemCoroa; 12, 22, 32: CoroaPrata; 13, 23, 33: CoroaDourada
//
// em TODOS os cinco blocos de CreateDungeonInstance — os numeros do Zantorian.
// Mina usa 101..303, Marauder 41..93 e Kinary 401..603, e nenhum deles casa
// com esse case: a lista de drop dessas quatro dungeons ficava zerada e o mob
// morria sem dropar nada. Aqui a coroa sai do ultimo digito, que e o que os
// nomes dos arquivos querem dizer.
func CrownOf(dropIndex uint16) (Crown, bool) {
	switch dropIndex % 10 {
	case 1:
		return NoCrown, true
	case 2:
		return SilverCrown, true
	case 3:
		return GoldCrown, true
	}
	return NoCrown, false
}

// DropsFor devolve a lista de drop da especie.
func (d *Definition) DropsFor(sp *Species) []uint16 {
	c, ok := CrownOf(sp.DropIndex)
	if !ok {
		return nil
	}
	return d.Drops[c]
}

// Table e o resultado da carga: as cinco dungeons em tres dificuldades.
type Table struct {
	// Defs[dungeonID][dificuldade]. dungeonID vai de 1 a 5.
	Defs map[uint32][Difficulties]*Definition
	// Avisos reune o que faltou nos arquivos, para o log.
	Avisos []string
}

// Get devolve a dungeon na dificuldade pedida. Index e dificuldade vindos do
// cliente passam por aqui: na origem "DGUrsula[Packet.Dificult]" indexava um
// vetor de um elemento com um DWORD do cliente, sem checagem nenhuma
// (PacketHandlers.pas:9489).
func (t *Table) Get(dungeonID uint32, dificult uint32) (*Definition, bool) {
	if t == nil {
		return nil, false
	}
	porDif, ok := t.Defs[dungeonID]
	if !ok {
		return nil, false
	}
	if dificult >= Difficulties {
		return nil, false
	}
	d := porDif[dificult]
	if d == nil {
		return nil, false
	}
	return d, true
}

// ByEntranceNPC acha a dungeon pelo NPC do portal.
func (t *Table) ByEntranceNPC(npcID uint16, dificult uint32) (*Definition, bool) {
	if t == nil {
		return nil, false
	}
	for id := range t.Defs {
		d, ok := t.Get(id, dificult)
		if ok && d.EntranceNPCID == npcID {
			return d, true
		}
	}
	return nil, false
}

// nomeArquivo tem o nome que cada dungeon usa nos arquivos de mob e a pasta
// de drop, na ordem das dificuldades.
type nomeArquivo struct {
	mobs     string
	dropDir  string
	dropBase [Difficulties]int
	sufixo   [Difficulties]string
}

var arquivos = map[uint32]nomeArquivo{
	ZantorianCitadel: {
		mobs: "Zantorian Citadel", dropDir: "zantorian",
		dropBase: [3]int{10, 20, 30},
		sufixo:   [3]string{"Normal", "Dificil", "Elite"},
	},
	MarauderHold: {
		mobs: "Marauder Hold", dropDir: "marauder_hold",
		dropBase: [3]int{40, 50, 60},
		sufixo:   [3]string{"Normal", "Dificil", "Elite"},
	},
	MarauderCabin: {
		mobs: "Marauder Cabin", dropDir: "marauder_cabin",
		dropBase: [3]int{70, 80, 90},
		sufixo:   [3]string{"Normal", "Dificil", "Elite"},
	},
	LostMines: {
		mobs: "Lost Mine", dropDir: "mines1",
		dropBase: [3]int{100, 200, 300},
		sufixo:   [3]string{"Normal", "Dificil", "Elite"},
	},
	KinaryAviary: {
		mobs: "Kynari Aviary", dropDir: "kinary",
		dropBase: [3]int{400, 500, 600},
		sufixo:   [3]string{"Normal", "Dificil", "Elite"},
	},
}

// Load le Dungeons.csv e, para cada linha, as tres dificuldades.
func Load(dataDir string) (*Table, error) {
	path := filepath.Join(dataDir, "Dungeons.csv")
	linhas, err := lerLinhas(path)
	if err != nil {
		return nil, fmt.Errorf("dungeondata: %w", err)
	}
	t := &Table{Defs: map[uint32][Difficulties]*Definition{}}

	for n, linha := range linhas {
		col := strings.Split(linha, ",")
		if len(col) < 10 {
			t.avisar("Dungeons.csv linha %d: %d colunas, esperava 10", n+1, len(col))
			continue
		}
		id := uint32(parseInt(col[0]))
		nomes, ok := arquivos[id]
		if !ok {
			t.avisar("Dungeons.csv linha %d: dungeon %d desconhecida", n+1, id)
			continue
		}
		var porDif [Difficulties]*Definition
		for dif := 0; dif < Difficulties; dif++ {
			d := &Definition{
				Index:         id,
				Name:          strings.TrimSpace(col[1]),
				Dificult:      uint8(dif),
				EntranceNPCID: uint16(parseInt(col[2])),
				EntrancePosition: model.Position{
					X: parseFloat(col[3]), Y: parseFloat(col[4])},
				SpawnInDungeonPosition: model.Position{
					X: parseFloat(col[5]), Y: parseFloat(col[6])},
				// As colunas 7, 8 e 9 sao o nivel minimo de cada dificuldade.
				LevelMin: uint8(parseInt(col[7+dif])),
			}
			if err := t.carregarMobs(dataDir, d, nomes, dif); err != nil {
				return nil, err
			}
			t.carregarDrops(dataDir, d, nomes, dif)
			porDif[dif] = d
		}
		t.Defs[id] = porDif
	}
	if len(t.Defs) == 0 {
		return nil, errors.New("dungeondata: nenhuma dungeon carregada")
	}
	return t, nil
}

func (t *Table) avisar(f string, a ...any) {
	t.Avisos = append(t.Avisos, fmt.Sprintf(f, a...))
}

func (t *Table) carregarMobs(dataDir string, d *Definition, nomes nomeArquivo, dif int) error {
	base := filepath.Join(dataDir, "MobsDungeon")
	info := filepath.Join(base, fmt.Sprintf("MobInfo_%s_%s.csv", nomes.mobs, nomes.sufixo[dif]))
	pos := filepath.Join(base, fmt.Sprintf("MobsPosition_%s_%s.csv", nomes.mobs, nomes.sufixo[dif]))

	linhas, err := lerLinhas(info)
	if err != nil {
		t.avisar("%s: %v", filepath.Base(info), err)
		return nil
	}
	porIntName := map[uint16]int{}
	for n, linha := range linhas {
		col := strings.Split(linha, ",")
		if len(col) < 19 {
			t.avisar("%s linha %d: %d colunas, esperava 19",
				filepath.Base(info), n+1, len(col))
			continue
		}
		if len(d.Species) >= MaxSpeciesPerDungeon {
			t.avisar("%s: mais de %d especies, o resto foi recusado",
				filepath.Base(info), MaxSpeciesPerDungeon)
			break
		}
		idx := len(d.Species)
		sp := Species{
			Index:        idx,
			IndexGeneric: idx + FirstDungeonMobIndex,
			IntName:      uint16(parseInt(col[0])),
			Name:         strings.TrimSpace(col[1]),
			HP:           uint32(parseInt(col[5])),
			Rotation:     uint32(parseInt(col[6])),
			Level:        uint16(parseInt(col[7])),
			MobType:      uint16(parseInt(col[11])),
			SpawnType:    uint8(parseInt(col[16])),
			DropIndex:    uint16(parseInt(col[18])),
		}
		sp.Equip[0] = uint16(parseInt(col[2]))
		sp.Equip[1] = uint16(parseInt(col[3]))
		sp.Equip[6] = uint16(parseInt(col[4]))
		// A origem: arma se houver, senao a roupa.
		if sp.Equip[6] > 0 {
			sp.AtkFis = uint32(sp.Equip[6])
		} else {
			sp.AtkFis = uint32(sp.Equip[0])
		}
		sp.AtkMag = sp.AtkFis
		sp.DefFis = sp.AtkFis * 2
		sp.DefMag = sp.DefFis
		// MobExp := Round(InitHP * 1.8), com o Round do Delphi.
		sp.Exp = uint32(roundHalfEven(float64(sp.HP) * 1.8))

		if sp.IntName == 0 {
			t.avisar("%s linha %d: IntName zero, linha ignorada",
				filepath.Base(info), n+1)
			continue
		}
		if _, repetido := porIntName[sp.IntName]; repetido {
			t.avisar("%s linha %d: IntName %d repetido",
				filepath.Base(info), n+1, sp.IntName)
			continue
		}
		porIntName[sp.IntName] = idx
		d.Species = append(d.Species, sp)
	}

	linhas, err = lerLinhas(pos)
	if err != nil {
		t.avisar("%s: %v", filepath.Base(pos), err)
		return nil
	}
	// Um aviso por especie, nao por linha recusada.
	recusados := map[uint16]int{}
	for n, linha := range linhas {
		col := strings.Split(linha, ",")
		if len(col) < 7 {
			t.avisar("%s linha %d: %d colunas, esperava 7",
				filepath.Base(pos), n+1, len(col))
			continue
		}
		intName := uint16(parseInt(col[6]))
		idx, ok := porIntName[intName]
		if !ok {
			t.avisar("%s linha %d: IntName %d nao esta no MobInfo",
				filepath.Base(pos), n+1, intName)
			continue
		}
		sp := &d.Species[idx]
		if len(sp.Spawns) >= MaxSpawnsPerSpecies {
			recusados[intName]++
			continue
		}
		hp := uint32(parseInt(col[2]))
		sp.Spawns = append(sp.Spawns, Spawn{
			ClientID:    uint16(parseInt(col[0])),
			SecondIndex: len(sp.Spawns),
			HP:          hp,
			MP:          hp,
			InitPos: model.Position{
				X: parseFloat(col[3]), Y: parseFloat(col[4])},
		})
	}
	for intName, n := range recusados {
		t.avisar("%s: especie %d tem %d pontos, acima dos %d que MobsP da "+
			"origem cabia; %d pontos foram recusados",
			filepath.Base(pos), intName, MaxSpawnsPerSpecies+n,
			MaxSpawnsPerSpecies, n)
	}
	if n := d.SpawnCount(); n > MaxMobsPerInstance {
		t.avisar("%s: %d pontos, acima dos %d que a instancia da origem tinha",
			filepath.Base(pos), n, MaxMobsPerInstance)
	}
	return nil
}

func (t *Table) carregarDrops(dataDir string, d *Definition, nomes nomeArquivo, dif int) {
	for c := 0; c < 3; c++ {
		nome := fmt.Sprintf("%d.txt", nomes.dropBase[dif]+c+1)
		path := filepath.Join(dataDir, "Drops", nomes.dropDir, nome)
		linhas, err := lerLinhas(path)
		if err != nil {
			t.avisar("Drops/%s/%s: %v", nomes.dropDir, nome, err)
			continue
		}
		var itens []uint16
		for _, linha := range linhas {
			if len(itens) >= MaxDropsPerTier {
				t.avisar("Drops/%s/%s: mais de %d itens, o resto foi recusado",
					nomes.dropDir, nome, MaxDropsPerTier)
				break
			}
			v := parseInt(strings.TrimSpace(linha))
			if v <= 0 {
				continue
			}
			itens = append(itens, uint16(v))
		}
		d.Drops[c] = itens
	}
}

func lerLinhas(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		linha := strings.TrimRight(sc.Text(), "\r\n \t")
		if linha == "" {
			continue
		}
		out = append(out, linha)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return int(v)
}

func parseFloat(s string) float32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return float32(v)
}

// roundHalfEven e o Round do Delphi: metade vai para o par.
func roundHalfEven(v float64) int64 {
	i := int64(v)
	f := v - float64(i)
	switch {
	case f > 0.5 || (f == 0.5 && i%2 != 0):
		return i + 1
	case f < -0.5 || (f == -0.5 && i%2 != 0):
		return i - 1
	}
	return i
}
