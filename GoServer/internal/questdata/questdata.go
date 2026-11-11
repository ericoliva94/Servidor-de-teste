// Package questdata le as quests do servidor, Data/Quest/Quests.csv.
//
// Porte de TLoad.InitQuests. Cada linha tem 34 campos separados por virgula:
// NPC, id, tipo, marca, 6 recompensas, 5 requisitos, 5 tipos de requisito, 5
// quantidades, 3 itens a retirar, 3 quantidades, ouro, exp e nivel minimo.
package questdata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Tipos de requisito (RequirimentsType).
const (
	ReqKillMob = 1 // matar mobs: Requiriments[j] e o IntName do mob
)

// MarkAutoDone e a marca (QuestMark) 11: abandonar a quest a conclui em vez de
// apaga-la.
const MarkAutoDone = 11

// MaxPlayerQuests e o tamanho da lista de quests do personagem (PlayerQuests).
const MaxPlayerQuests = 50

// fields e o numero de colunas de uma linha.
const fields = 34

// Quest e uma linha do Quests.csv (TQuestMisc).
type Quest struct {
	NPCID     uint32
	QuestID   uint32
	QuestType uint8
	QuestMark uint8
	Rewards   [6]uint32

	Requirements       [5]uint32
	RequirementsType   [5]uint8
	RequirementsAmount [5]uint16

	DeletesItem   [3]uint32
	DeletesAmount [3]uint16

	Gold     int64
	Exp      int64
	LevelMin uint8
}

// Table guarda as quests por id.
type Table struct {
	byID map[uint32]*Quest
}

// Get devolve a quest do id, ou nil.
func (t *Table) Get(id uint32) *Quest {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// Len conta as quests.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.byID)
}

// Parse le o CSV. Linha vazia e ignorada; linha com menos de 34 campos ou com
// campo que nao e numero vai para avisos; id 0 e enchimento e some sem aviso (a origem estourava excecao e perdia o
// arquivo inteiro). Id repetido: vale a primeira linha.
func Parse(r *bufio.Scanner) (*Table, []string) {
	t := &Table{byID: map[uint32]*Quest{}}
	var avisos []string
	for n := 1; r.Scan(); n++ {
		linha := strings.TrimSpace(r.Text())
		if linha == "" {
			continue
		}
		parts := strings.Split(linha, ",")
		if len(parts) < fields {
			avisos = append(avisos, fmt.Sprintf("linha %d: %d campos, queria %d", n, len(parts), fields))
			continue
		}
		v := make([]int64, fields)
		ok := true
		for i := 0; i < fields; i++ {
			x, err := strconv.ParseInt(strings.TrimSpace(parts[i]), 10, 64)
			if err != nil {
				avisos = append(avisos, fmt.Sprintf("linha %d: campo %d invalido", n, i))
				ok = false
				break
			}
			v[i] = x
		}
		if !ok {
			continue
		}
		q := &Quest{
			NPCID: uint32(v[0]), QuestID: uint32(v[1]),
			QuestType: uint8(v[2]), QuestMark: uint8(v[3]),
			Gold: v[31], Exp: v[32], LevelMin: uint8(v[33]),
		}
		for i := 0; i < 6; i++ {
			q.Rewards[i] = uint32(v[4+i])
		}
		for i := 0; i < 5; i++ {
			q.Requirements[i] = uint32(v[10+i])
			q.RequirementsType[i] = uint8(v[15+i])
			q.RequirementsAmount[i] = uint16(v[20+i])
		}
		for i := 0; i < 3; i++ {
			q.DeletesItem[i] = uint32(v[25+i])
			q.DeletesAmount[i] = uint16(v[28+i])
		}
		if q.QuestID == 0 {
			continue // linha de enchimento (o arquivo termina em linhas zeradas)
		}
		if _, dup := t.byID[q.QuestID]; dup {
			avisos = append(avisos, fmt.Sprintf("linha %d: quest %d repetida, ignorada", n, q.QuestID))
			continue
		}
		t.byID[q.QuestID] = q
	}
	return t, avisos
}

// Load le Quest/Quests.csv da pasta Data. Arquivo ausente devolve tabela vazia
// e o erro, como a origem (que so avisava e seguia).
func Load(dataDir string) (*Table, []string, error) {
	f, err := os.Open(filepath.Join(dataDir, "Quest", "Quests.csv"))
	if err != nil {
		return &Table{byID: map[uint32]*Quest{}}, nil, err
	}
	defer f.Close()
	t, avisos := Parse(bufio.NewScanner(f))
	return t, avisos, nil
}
