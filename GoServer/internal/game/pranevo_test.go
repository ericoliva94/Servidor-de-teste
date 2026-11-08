package game

import (
	"testing"

	"aika/internal/packets"
	"aika/internal/pran"
	"aika/internal/store"
)

// cenarioEvolucao deixa a pran 1 invocada, no nivel e na classe pedidos.
func cenarioEvolucao(t *testing.T, nivel, classe uint8) *cenarioInvocar {
	t.Helper()
	c := novoCenarioInvocar(t)
	c.p1.Level, c.p1.Class = nivel, classe
	c.move(t, store.SlotTypeInventory, 0, store.SlotTypeEquip, 10)
	c.e.drena(20 * 1e6)
	c.eViz.drena(20 * 1e6)
	return c
}

func TestEvolucaoLv5FadaViraCrianca(t *testing.T) {
	c := cenarioEvolucao(t, 4, 71)
	c.p1.Skills[3].Level = 1
	if !c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) {
		t.Fatal("deveria evoluir")
	}
	if c.p1.Class != 72 || c.p1.Equip[0].Index != 104 || c.p1.Equip[0].APP != 104 ||
		c.p1.Equip[6].Index != 150 || c.p1.Skills[3].Level != 2 {
		t.Errorf("pran = classe %d equip0 %+v equip6 %+v skill3 %d", c.p1.Class, c.p1.Equip[0], c.p1.Equip[6], c.p1.Skills[3].Level)
	}
	if got := c.s.Equip.Get(10); got.Index != 104 || got.APP != 104 || got.Identific != 900 {
		t.Errorf("item de pran do dono = %+v", got)
	}
	ef := decodifica[packets.SendClientIndexPacket](t, proximoDe(t, c.e, CodeSendEffect))
	if ef.Effect != 0 {
		t.Errorf("efeito = %d, queria 0", ef.Effect)
	}
	dados := decodifica[packets.SendPranToWorld](t, proximoDe(t, c.e, CodePranToWorld))
	if dados.PranClass != 72 {
		t.Errorf("classe enviada = %d", dados.PranClass)
	}
	proximoDe(t, c.e, CodePranSpawn)
	r := decodifica[packets.RefreshItemPacket](t, proximoDe(t, c.e, CodeRefreshItem))
	if r.Slot != 10 || r.Item.Index != 104 {
		t.Errorf("slot atualizado = %+v", r)
	}
	// E a evolucao libera a experiencia: a fronteira do nivel 4 so vale para fada.
	if c.p1.ClassNeedsEvolution() {
		t.Error("depois de evoluir a pran volta a ganhar exp")
	}
}

func TestEvolucaoLv20CriancaViraAdolescente(t *testing.T) {
	c := cenarioEvolucao(t, 19, 82)
	if !c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv20) {
		t.Fatal("deveria evoluir")
	}
	if c.p1.Class != 83 || c.p1.Equip[0].Index != 105 || c.p1.Equip[6].Index != 0 || c.s.Equip.Get(10).Index != 105 {
		t.Errorf("pran = classe %d equip0 %+v item %+v", c.p1.Class, c.p1.Equip[0], c.s.Equip.Get(10))
	}
	// Sem o efeito zerado do 406.
	c.e.drena(1e6)
}

func TestEvolucaoRecusas(t *testing.T) {
	// Nivel errado: aborta em silencio.
	c := cenarioEvolucao(t, 5, 71)
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) || c.p1.Class != 71 {
		t.Error("nivel 5 nao evolui no 406")
	}
	// Classe errada para a quest.
	c = cenarioEvolucao(t, 4, 72)
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) {
		t.Error("classe 72 nao evolui no 406")
	}
	if got := mensagemDe(t, c.e); got != "Essa pran não pode ser upada de classe." {
		t.Errorf("mensagem = %q", got)
	}
	// Quest do 20 numa pran do 5.
	c = cenarioEvolucao(t, 4, 71)
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv20) {
		t.Error("nivel 4 nao evolui no 407")
	}
	// Quests sem evolucao.
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv50) || c.h.evolvePran(c.e.conn, c.s, 1) {
		t.Error("408 e quests comuns nao evoluem")
	}
}

func TestEvolucaoExigePranEquipada(t *testing.T) {
	c := novoCenarioInvocar(t) // nada equipado
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) {
		t.Fatal("sem pran equipada nao evolui")
	}
	if got := mensagemDe(t, c.e); got != "Você deve equipar uma pran para este procedimento." {
		t.Errorf("mensagem = %q", got)
	}
	// Item no slot, mas nenhuma pran invocada.
	c.s.Equip.Slots[10].Index = 7000
	c.s.SpawnedPran = -1
	if c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) {
		t.Error("sem pran invocada nao evolui")
	}
}

func TestEvolucaoRefazOsEfeitosDoEquipamentoNovo(t *testing.T) {
	c := cenarioEvolucao(t, 4, 61)
	_ = pran.MaxLevel
	antes := nonZero(&c.s.Avatar.Ability)
	if !c.h.evolvePran(c.e.conn, c.s, QuestPranEvolveLv5) {
		t.Fatal("deveria evoluir")
	}
	depois := nonZero(&c.s.Avatar.Ability)
	// O equipamento inicial do teste tinha Equip[0] = 1234; a forma nova troca
	// por 104 e o acumulador nao pode acumular os dois.
	c.h.removePranEffects(c.s)
	if got := nonZero(&c.s.Avatar.Ability); len(got) != 0 {
		t.Errorf("sobrou efeito depois de desligar: %v (antes %v, depois %v)", got, antes, depois)
	}
}
