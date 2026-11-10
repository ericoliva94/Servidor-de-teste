package questdata

import (
	"bufio"
	"strings"
	"testing"
)

func scan(s string) *bufio.Scanner { return bufio.NewScanner(strings.NewReader(s)) }

func TestParseRealLine(t *testing.T) {
	tab, avisos := Parse(scan("2048,14,1,8,5300,0,0,0,0,0,1,0,0,0,0,1,0,0,0,0,3,0,0,0,0,0,0,0,0,0,0,500,30000,1\n"))
	if len(avisos) != 0 {
		t.Fatalf("avisos: %v", avisos)
	}
	q := tab.Get(14)
	if q == nil {
		t.Fatal("quest 14 nao carregada")
	}
	if q.NPCID != 2048 || q.QuestMark != 8 || q.Rewards[0] != 5300 ||
		q.Requirements[0] != 1 || q.RequirementsType[0] != ReqKillMob || q.RequirementsAmount[0] != 3 ||
		q.Gold != 500 || q.Exp != 30000 || q.LevelMin != 1 {
		t.Errorf("quest = %+v", q)
	}
}

func TestParseSkipsBadLines(t *testing.T) {
	in := "1,2,3\n\n" + // curta
		"2048,x,1,8,5300,0,0,0,0,0,1,0,0,0,0,1,0,0,0,0,3,0,0,0,0,0,0,0,0,0,0,500,30000,1\n" + // nao numerica
		"2048,14,1,8,5300,0,0,0,0,0,1,0,0,0,0,1,0,0,0,0,3,0,0,0,0,0,0,0,0,0,0,500,30000,1\n" +
		"2048,14,1,8,5300,0,0,0,0,0,1,0,0,0,0,1,0,0,0,0,9,0,0,0,0,0,0,0,0,0,0,500,30000,1\n" // repetida
	tab, avisos := Parse(scan(in))
	if tab.Len() != 1 || len(avisos) != 3 {
		t.Errorf("quests=%d avisos=%v", tab.Len(), avisos)
	}
	if tab.Get(14).RequirementsAmount[0] != 3 {
		t.Error("vale a primeira linha repetida")
	}
}

func TestRealFileLoads(t *testing.T) {
	tab, avisos, err := Load("../../../Server/Bin/Data")
	if err != nil {
		t.Skip("Quests.csv fora do repositorio:", err)
	}
	if tab.Len() != 33 || len(avisos) != 0 {
		t.Errorf("quests=%d avisos=%v, queria 33 e nenhum", tab.Len(), avisos)
	}
	t.Logf("%d quests, avisos: %v", tab.Len(), avisos[:min(5, len(avisos))])
}
