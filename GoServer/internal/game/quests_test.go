package game

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/mobdata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/questdata"
)

func scanner(s string) *bufio.Scanner { return bufio.NewScanner(strings.NewReader(s)) }

type questsFalsas struct {
	porChar map[uint32][]model.QuestProgress
}

func (f *questsFalsas) Load(_ context.Context, id uint32) ([]model.QuestProgress, error) {
	return f.porChar[id], nil
}
func (f *questsFalsas) Save(_ context.Context, id uint32, l []model.QuestProgress) error {
	if f.porChar == nil {
		f.porChar = map[uint32][]model.QuestProgress{}
	}
	f.porChar[id] = append([]model.QuestProgress(nil), l...)
	return nil
}

// questTable monta duas quests: a 14 pede 3 mobs de IntName 7; a 20 tem marca 11.
func questHandler(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, _ := handlerComMobs(t, tabelaDeMobs(especie("Bicho", 100, 1, 1000, 1000)))
	h.QuestTable, _ = questdata.Parse(scanner(
		"2048,14,1,8,5300,0,0,0,0,0,7,0,0,0,0,1,0,0,0,0,3,0,0,0,0,0,0,0,0,0,0,500,30000,1\n" +
			"2048,20,1,11,5300,0,0,0,0,0,7,0,0,0,0,1,0,0,0,0,2,0,0,0,0,0,0,0,0,0,0,500,30000,1\n"))
	h.QuestList = make([]gamedata.QuestFromList, 30)
	copy(h.QuestList[14].Titulo[:], "Cacada")
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.drena(10 * time.Millisecond)
	return h, s, e
}

func TestQuestKillProgressAndCompletion(t *testing.T) {
	h, s, e := questHandler(t)
	if !h.startQuest(e.conn, s, 14) {
		t.Fatal("startQuest recusou")
	}
	e.drena(10 * time.Millisecond)
	for i := 0; i < 2; i++ {
		h.questOnKill(e.conn, s, 7)
	}
	if got := s.Quests[0].Complete[0]; got != 2 {
		t.Fatalf("progresso = %d, queria 2", got)
	}
	h.questOnKill(e.conn, s, 8) // outro mob: nao conta
	e.drena(10 * time.Millisecond)
	h.questOnKill(e.conn, s, 7)
	if s.Quests[0].Complete[0] != 3 {
		t.Errorf("progresso = %d, queria 3", s.Quests[0].Complete[0])
	}
	var sawMsg bool
	for i := 0; i < 2; i++ {
		v := e.proximo(t)
		if v.code == CodeClientMsg {
			sawMsg = true
		}
	}
	if !sawMsg {
		t.Error("deveria avisar que a quest foi concluida")
	}
	// Passou da quantidade: nao conta mais.
	h.questOnKill(e.conn, s, 7)
	if s.Quests[0].Complete[0] != 3 {
		t.Errorf("depois de completa = %d, queria 3", s.Quests[0].Complete[0])
	}
}

func TestQuestProgressSentToClient(t *testing.T) {
	h, s, e := questHandler(t)
	h.startQuest(e.conn, s, 14)
	e.drena(10 * time.Millisecond)
	h.questOnKill(e.conn, s, 7)
	v := e.proximo(t)
	if v.code != CodeQuestInfo {
		t.Fatalf("codigo %#x, queria %#x", v.code, CodeQuestInfo)
	}
	var pkt packets.SendQuestInfo
	if err := netsrv.Unmarshal(asPacote(t, v.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.QuestID != 14 || pkt.RequirimentComplete[0] != 1 || pkt.RequirimentAmount[0] != 3 || pkt.RequirimentItem[0] != 7 {
		t.Errorf("pacote = %+v", pkt)
	}
}

func TestStartQuestRules(t *testing.T) {
	h, s, e := questHandler(t)
	if h.startQuest(e.conn, s, 999) {
		t.Error("quest inexistente")
	}
	h.startQuest(e.conn, s, 14)
	if h.startQuest(e.conn, s, 14) {
		t.Error("quest repetida")
	}
	for len(s.Quests) < questdata.MaxPlayerQuests {
		s.Quests = append(s.Quests, model.QuestProgress{QuestID: uint32(1000 + len(s.Quests))})
	}
	if h.startQuest(e.conn, s, 20) {
		t.Error("lista cheia")
	}
}

func abandonPacket(t *testing.T, id uint32) []byte {
	t.Helper()
	b, err := netsrv.Marshal(CodeAbandonQuest, packets.AbandonQuestPacket{QuestID: id})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAbandonQuest(t *testing.T) {
	h, s, e := questHandler(t)
	h.startQuest(e.conn, s, 14)
	h.startQuest(e.conn, s, 20)
	e.drena(10 * time.Millisecond)
	ctx := context.Background()

	if err := h.abandonQuest(ctx, e.conn, s, asPacket(t, abandonPacket(t, 14))); err != nil {
		t.Fatal(err)
	}
	if questSlot(s, 14) >= 0 {
		t.Error("a quest 14 deveria sair da lista")
	}
	if v := e.proximo(t); v.code != CodeAbandonQuest {
		t.Errorf("eco = %#x", v.code)
	}
	// Marca 11: fica, concluida.
	h.abandonQuest(ctx, e.conn, s, asPacket(t, abandonPacket(t, 20)))
	if i := questSlot(s, 20); i < 0 || !s.Quests[i].Done {
		t.Error("a quest de marca 11 deveria ficar concluida")
	}
	// Quest que nao existe nao responde.
	e.drena(10 * time.Millisecond)
	h.abandonQuest(ctx, e.conn, s, asPacket(t, abandonPacket(t, 555)))
	e.nada(t, 20*time.Millisecond)
}

func TestMobDeathAdvancesQuest(t *testing.T) {
	h, s, e := questHandler(t)
	h.startQuest(e.conn, s, 14)
	h.onMobDeath(e.conn, s, &Mob{ClientID: 100}, &mobdata.Species{Exp: 10, Level: 50, IntName: 7})
	if s.Quests[0].Complete[0] != 1 {
		t.Errorf("a morte do mob deveria somar 1: %d", s.Quests[0].Complete[0])
	}
}

func TestQuestsLoadedFromStore(t *testing.T) {
	h, s, _ := questHandler(t)
	h.QuestDB = &questsFalsas{porChar: map[uint32][]model.QuestProgress{
		7: {{QuestID: 14, Complete: [5]uint8{2}}},
	}}
	h.loadQuests(context.Background(), s, 7)
	if len(s.Quests) != 1 || s.Quests[0].Complete[0] != 2 {
		t.Errorf("quests = %+v", s.Quests)
	}
	h.loadQuests(context.Background(), s, 8) // outro personagem: nada herdado
	if len(s.Quests) != 0 {
		t.Error("a lista do personagem anterior nao pode ficar")
	}
}
