package game

import (
	"context"
	"testing"

	"aika/internal/auth"
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// ---- montagem

// handlerParaUso devolve um handler com tabela de item, de skill, de exp e de
// duracao de buff, mais o canal e o registro de guildas.
func handlerParaUso(t *testing.T) (*Handler, *tabelaDeItens, *tabelaDeSkills) {
	t.Helper()
	h, tabItens, _ := handlerComArmazem(t)
	tabSkills := novaTabelaDeSkills()
	h.SkillTable = tabSkills
	h.Exp = tabelaExp{}
	h.Durations = duracoesFixas{}
	return h, tabItens, tabSkills
}

func useItemPacket(tipoSlot, slot, tipo1 uint32) []byte {
	var p packets.UseItemPacket
	p.TypeSlot = tipoSlot
	p.Slot = slot
	p.Type1 = tipo1
	buf, err := netsrv.Marshal(CodeUseItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func useBuffItemPacket(slot uint32) []byte {
	var p packets.UseBuffItemPacket
	p.Slot = slot
	buf, err := netsrv.Marshal(CodeUseBuffItem, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// poeConsumivel registra um item da tabela com tipo e efeito.
func poeConsumivel(tab *tabelaDeItens, index int, tipo, efeito uint16, nome string) {
	it := gamedata.ItemFromList{
		ItemType:  tipo,
		UseEffect: efeito,
		CanAgroup: 1,
	}
	copy(it.Name[:], nome)
	tab.poeItem(index, it)
}

// usa dispara o $31D no slot do inventario.
func usa(t *testing.T, h *Handler, e *espiao, s *Session, slot uint32) {
	t.Helper()
	if err := h.useItem(context.Background(), e.conn, s,
		asPacket(t, useItemPacket(store.SlotTypeInventory, slot, 0))); err != nil {
		t.Fatal(err)
	}
}

// ---- guardas de entrada

// A origem conferia "Packet.Slot > 63" e seguia SEM olhar se o slot tinha
// item: para um slot vazio ela lia ItemList[0] e aplicava o efeito daquela
// linha da tabela.
func TestUsarSlotVazioNaoAplicaOItemZero(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// Item 0 da tabela: uma moeda de ouro rica, o pior caso.
	poeConsumivel(tab, 0, ItemTypeUseRichGoldCoin, 0, "Linha zero")
	s.Avatar.Gold = 0

	usa(t, h, e, s, 5) // slot vazio
	if s.Avatar.Gold != 0 {
		t.Errorf("ouro = %d: o slot vazio aplicou ItemList[0]", s.Avatar.Gold)
	}
}

func TestUsarSlotForaDaFaixa(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 100, "Pocao")
	s.Avatar.CurHP = 1

	for _, slot := range []uint32{64, 100, 70000, 0xFFFFFFFF} {
		usa(t, h, e, s, slot)
	}
	if s.Avatar.CurHP != 1 {
		t.Errorf("HP = %d: um slot fora da faixa curou", s.Avatar.CurHP)
	}
}

// "ItemList[item.Index]" era indexado com um WORD do banco sem checar a faixa.
func TestItemForaDaTabelaEhRecusado(t *testing.T) {
	h, _, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Inventory.Slots[0] = model.Item{Index: 65535, Refi: 1}
	s.Avatar.CurHP = 1

	usa(t, h, e, s, 0)
	if s.Inventory.Get(0).Index != 65535 {
		t.Error("o item fora da tabela foi consumido")
	}
}

func TestUsarItemMortoEhRecusado(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 100, "Pocao")
	s.Inventory.Slots[0] = model.Item{Index: 500, Refi: 5}
	s.Avatar.CurHP = 1
	s.Dead = true

	usa(t, h, e, s, 0)
	if s.Avatar.CurHP != 1 || s.Inventory.Get(0).Refi != 5 {
		t.Errorf("morto curou: HP=%d quantidade=%d",
			s.Avatar.CurHP, s.Inventory.Get(0).Refi)
	}
}

// A origem conferia o selo em UseBuffItem mas nao em UseItem: um item selado
// dava o efeito e seguia selado.
func TestItemSeladoNaoFunciona(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 100, "Pocao")
	it := model.Item{Index: 500, Refi: 5}
	it.SetSealed(true, ItemTypeHPPotion, agora, 0, agora)
	s.Inventory.Slots[0] = it
	s.Avatar.CurHP = 1

	usa(t, h, e, s, 0)
	if s.Avatar.CurHP != 1 {
		t.Errorf("HP = %d: o item selado curou", s.Avatar.CurHP)
	}
}

// "if (not Player.IsGM) and (Level < ItemList[].Level)". IsGM na origem e
// AccountType = 5, so Admin.
func TestNivelDoItemEhExigidoMenosParaAdmin(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	it := gamedata.ItemFromList{ItemType: ItemTypeHPPotion, UseEffect: 100, Level: 90}
	copy(it.Name[:], "Pocao alta")
	tab.poeItem(500, it)

	for _, caso := range []struct {
		nome  string
		tipo  uint8
		curou bool
	}{
		{"jogador", auth.AccPlayer, false},
		{"gm", auth.AccGameMaster, false}, // IsGM na origem e so Admin
		{"admin", auth.AccAdmin, true},
	} {
		e := novoEspiao(t)
		s := sessaoComArmazem(caso.nome, 11)
		s.Account.Type = caso.tipo
		if _, err := h.Chan.Join(e.conn, s); err != nil {
			t.Fatal(err)
		}
		s.Avatar.Level = 50
		s.Avatar.MaxHP = 1000
		s.Avatar.CurHP = 1
		s.Inventory.Slots[0] = model.Item{Index: 500, Refi: 5}

		usa(t, h, e, s, 0)
		curou := s.Avatar.CurHP > 1
		if curou != caso.curou {
			t.Errorf("%s: curou=%v, queria %v", caso.nome, curou, caso.curou)
		}
		h.Chan.Leave(s.ClientID)
	}
}

// ---- pocoes

// A origem fazia "Inc(CurrentScore.CurHP, UseEffect)" sem teto: beber com a
// vida cheia empurrava CurHP acima de MaxHP e o personagem era gravado assim.
func TestPocaoNaoPassaDoMaximo(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 5000, "Pocao de HP")
	s.Avatar.MaxHP = 1000
	s.Avatar.CurHP = 900
	s.Inventory.Slots[0] = model.Item{Index: 500, Refi: 10}

	usa(t, h, e, s, 0)
	if s.Avatar.CurHP != 1000 {
		t.Errorf("HP = %d, queria o maximo 1000", s.Avatar.CurHP)
	}
	// Beber de novo com a vida cheia nao estoura.
	usa(t, h, e, s, 0)
	if s.Avatar.CurHP != 1000 {
		t.Errorf("HP = %d depois da segunda pocao", s.Avatar.CurHP)
	}
}

func TestPocaoDeMPEDeLagrimas(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 501, ItemTypeMPPotion, 200, "Pocao de MP")
	poeConsumivel(tab, 502, ItemTypeHPMPLagrimas, 300, "Lagrimas")
	poeConsumivel(tab, 503, ItemTypeHPMPPotion, 50, "Pocao dupla")
	s.Avatar.MaxHP, s.Avatar.MaxMP = 1000, 1000
	s.Avatar.CurHP, s.Avatar.CurMP = 100, 100
	s.Inventory.Slots[0] = model.Item{Index: 501, Refi: 1}
	s.Inventory.Slots[1] = model.Item{Index: 502, Refi: 1}
	s.Inventory.Slots[2] = model.Item{Index: 503, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Avatar.CurHP != 100 || s.Avatar.CurMP != 300 {
		t.Errorf("pocao de MP: HP=%d MP=%d", s.Avatar.CurHP, s.Avatar.CurMP)
	}
	usa(t, h, e, s, 1)
	if s.Avatar.CurHP != 400 || s.Avatar.CurMP != 600 {
		t.Errorf("lagrimas: HP=%d MP=%d", s.Avatar.CurHP, s.Avatar.CurMP)
	}
	usa(t, h, e, s, 2)
	if s.Avatar.CurHP != 450 || s.Avatar.CurMP != 650 {
		t.Errorf("pocao dupla: HP=%d MP=%d", s.Avatar.CurHP, s.Avatar.CurMP)
	}
}

// ---- consumo

func TestItemEhConsumidoUmaUnidadePorUso(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 10, "Pocao")
	s.Avatar.MaxHP, s.Avatar.CurHP = 1000, 1
	s.Inventory.Slots[0] = model.Item{Index: 500, Refi: 3}

	usa(t, h, e, s, 0)
	if got := s.Inventory.Get(0).Refi; got != 2 {
		t.Fatalf("quantidade = %d, queria 2", got)
	}
	usa(t, h, e, s, 0)
	usa(t, h, e, s, 0)
	if !s.Inventory.Get(0).IsEmpty() {
		t.Errorf("o slot deveria estar vazio: %+v", s.Inventory.Get(0))
	}
}

// ---- moedas

func TestMoedaDeOuroRespeitaOTeto(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	it := gamedata.ItemFromList{ItemType: ItemTypeUseGoldCoin, SellPrince: 1_000_000}
	copy(it.Name[:], "Moeda")
	tab.poeItem(600, it)
	s.Inventory.Slots[0] = model.Item{Index: 600, Refi: 2}
	s.Avatar.Gold = MaxGold - 10

	usa(t, h, e, s, 0)
	if s.Avatar.Gold != MaxGold {
		t.Errorf("ouro = %d, queria o teto %d", s.Avatar.Gold, MaxGold)
	}
}

func TestMoedaRicaDaOValorChumbado(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 601, ItemTypeUseRichGoldCoin, 0, "Moeda rica")
	s.Inventory.Slots[0] = model.Item{Index: 601, Refi: 1}
	s.Avatar.Gold = 0

	usa(t, h, e, s, 0)
	if s.Avatar.Gold != GoldCoinRichValue {
		t.Errorf("ouro = %d, queria %d", s.Avatar.Gold, GoldCoinRichValue)
	}
}

// ---- experiencia

// A origem calculava "ExpList[Level + 49] + 1" num vetor dinamico sem checar
// a faixa; acima do fim caia no except e usava High(ExpList), o ultimo
// INDICE, e "LevelExp - UInt64(Exp)" dava a volta no UInt64: o personagem
// recebia cerca de 1,8e19 de experiencia.
func TestItemDeNivelNaoDaVoltaNoUInt64(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// UseEffect 1 e o ramo dos 50 niveis de uma vez.
	poeConsumivel(tab, 700, ItemTypeUseToUpLevel, 1, "Pedra de nivel")
	s.Inventory.Slots[0] = model.Item{Index: 700, Refi: 1}
	// tabelaExp vai so ate o nivel 150: 149 + 49 passa do fim.
	s.Avatar.Level = 149
	s.Avatar.Exp = 149_000
	expAntes := s.Avatar.Exp

	usa(t, h, e, s, 0)
	if s.Avatar.Exp != expAntes {
		t.Errorf("exp = %d, queria intacta em %d", s.Avatar.Exp, expAntes)
	}
	if s.Inventory.Get(0).IsEmpty() {
		t.Error("a pedra foi consumida sem efeito")
	}
}

func TestItemDeNivelSobeDentroDaTabela(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 701, ItemTypeUseToUpLevel, 5, "Pedra de 5 niveis")
	s.Inventory.Slots[0] = model.Item{Index: 701, Refi: 1}
	s.Avatar.Level = 10
	s.Avatar.Exp = 10_000

	usa(t, h, e, s, 0)
	if s.Avatar.Exp <= 10_000 {
		t.Errorf("exp = %d, queria acima de 10000", s.Avatar.Exp)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("a pedra nao foi consumida")
	}
}

// "Perc := Value / 1000": o UseEffect e por mil da faixa do nivel.
func TestItemDeExpPercentualUsaAFaixaDoNivel(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// tabelaExp: nivel n vale n*1000, entao a faixa de um nivel e 1000.
	// 500 por mil = metade da faixa = 500 de exp.
	poeConsumivel(tab, 702, ItemTypeAddExpPerc, 500, "Pergaminho de exp")
	s.Inventory.Slots[0] = model.Item{Index: 702, Refi: 1}
	s.Avatar.Level = 20
	s.Avatar.Exp = 20_000

	usa(t, h, e, s, 0)
	if s.Avatar.Exp != 20_500 {
		t.Errorf("exp = %d, queria 20500", s.Avatar.Exp)
	}
}

// "ExpList[Level] - ExpList[Level - 1]" com Level zero lia ExpList[-1].
func TestItemDeExpPercentualComNivelZero(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 702, ItemTypeAddExpPerc, 500, "Pergaminho de exp")
	s.Inventory.Slots[0] = model.Item{Index: 702, Refi: 1}
	s.Avatar.Level = 0
	s.Avatar.Exp = 0

	usa(t, h, e, s, 0)
	if s.Avatar.Exp != 0 {
		t.Errorf("exp = %d, queria 0", s.Avatar.Exp)
	}
}

// ---- buffs

func TestBuffDeItemEntraEConsome(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	skills.poe(50, gamedata.T_SkillData{})
	poeConsumivel(tab, 800, ItemTypePotionBuff, 50, "Pocao de forca")
	s.Inventory.Slots[0] = model.Item{Index: 800, Refi: 2}

	usa(t, h, e, s, 0)
	if !s.Base.Buffs.Has(50) {
		t.Fatal("o buff nao entrou")
	}
	if got := s.Inventory.Get(0).Refi; got != 1 {
		t.Errorf("quantidade = %d, queria 1", got)
	}
}

// A origem repetia cinco ramos quase identicos para as pocoes 493 a 497.
func TestBuffDeItemRecusaConflito(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// As duracoes do teste cobrem 1..100, entao os buffs de conflito usam
	// indices dentro dessa faixa, com o Index da skill no valor em conflito.
	skills.m[60] = gamedata.T_SkillData{Index: 493}
	skills.m[61] = gamedata.T_SkillData{Index: 494}
	poeConsumivel(tab, 801, ItemTypePotionBuff, 60, "Valor de batalha A")
	poeConsumivel(tab, 802, ItemTypePotionBuff, 61, "Valor de batalha B")
	s.Inventory.Slots[0] = model.Item{Index: 801, Refi: 1}
	s.Inventory.Slots[1] = model.Item{Index: 802, Refi: 1}

	// O primeiro entra: chave de buff 60, cuja linha tem Index 493.
	usa(t, h, e, s, 0)
	if !s.Base.Buffs.Has(60) {
		t.Fatal("o primeiro buff nao entrou")
	}
	// O segundo tem Index 494, que conflita com 493. O conflito e por indice
	// de SKILL: a origem passava cada chave ativa pela tabela
	// ("SkillData[i].Index = j"), nao comparava a chave crua.
	usa(t, h, e, s, 1)
	if s.Base.Buffs.Has(61) {
		t.Error("o segundo buff entrou apesar do conflito 493/494")
	}
	if s.Inventory.Get(1).IsEmpty() {
		t.Error("o item recusado foi consumido")
	}
}

// O conflito e resolvido pelo campo Index da linha de skill, nao pela chave
// do buff: uma chave 65 cuja linha tem Index 251 conflita com 251.
func TestConflitoDeBuffUsaOIndiceDaSkillENaoAChave(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// Chave 65, linha com Index 251. A chave 251 nao esta ativa em momento
	// nenhum, entao uma comparacao pela chave crua nao acharia conflito.
	skills.m[65] = gamedata.T_SkillData{Index: 251}
	skills.m[66] = gamedata.T_SkillData{Index: 251}
	poeConsumivel(tab, 804, ItemTypePotionBuff, 65, "Pocao A")
	poeConsumivel(tab, 805, ItemTypePotionBuff, 66, "Pocao B")
	s.Inventory.Slots[0] = model.Item{Index: 804, Refi: 1}
	s.Inventory.Slots[1] = model.Item{Index: 805, Refi: 1}

	usa(t, h, e, s, 0)
	if !s.Base.Buffs.Has(65) {
		t.Fatal("o primeiro buff nao entrou")
	}
	usa(t, h, e, s, 1)
	if s.Base.Buffs.Has(66) {
		t.Error("o segundo entrou: o conflito nao foi resolvido pela tabela")
	}
}

// Conflito consigo mesmo: 251, 285 e 305 na origem.
func TestBuffDeItemNaoAcumulaConsigoMesmo(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// Buff 251 com Index 251: a tabela de conflitos diz 251 -> {251}.
	skills.m[251] = gamedata.T_SkillData{Index: 251}
	poeConsumivel(tab, 803, ItemTypePotionBuff, 251, "Pocao unica")
	s.Inventory.Slots[0] = model.Item{Index: 803, Refi: 3}
	// A duracao do teste cobre 1..100; 251 esta fora, entao e preciso uma
	// duracao propria para este caso.
	h.Durations = duracoesAmplas{}

	usa(t, h, e, s, 0)
	if !s.Base.Buffs.Has(251) {
		t.Fatal("o buff nao entrou")
	}
	if got := s.Inventory.Get(0).Refi; got != 2 {
		t.Fatalf("quantidade = %d, queria 2", got)
	}
	// O segundo uso e recusado e NAO consome.
	usa(t, h, e, s, 0)
	if got := s.Inventory.Get(0).Refi; got != 2 {
		t.Errorf("quantidade = %d: o uso recusado consumiu o item", got)
	}
}

// "Copy(String(ItemList[].Name), 0, 4) = 'Sopa'": a categoria sai do nome.
func TestSopaNaoAcumulaComOutraSopa(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	skills.poe(70, gamedata.T_SkillData{Name: nomeSkill("Sopa de carne")})
	skills.poe(71, gamedata.T_SkillData{Name: nomeSkill("Sopa de peixe")})
	poeConsumivel(tab, 810, ItemTypePotionBuff, 70, "Sopa de carne")
	poeConsumivel(tab, 811, ItemTypePotionBuff, 71, "Sopa de peixe")
	s.Inventory.Slots[0] = model.Item{Index: 810, Refi: 1}
	s.Inventory.Slots[1] = model.Item{Index: 811, Refi: 1}

	usa(t, h, e, s, 0)
	if !s.Base.Buffs.Has(70) {
		t.Fatal("a primeira sopa nao entrou")
	}
	usa(t, h, e, s, 1)
	if s.Base.Buffs.Has(71) {
		t.Error("a segunda sopa entrou junto da primeira")
	}
	if s.Inventory.Get(1).IsEmpty() {
		t.Error("a segunda sopa foi consumida sem efeito")
	}
}

// A origem indexava SkillData com o UseEffect da tabela de itens sem checar a
// faixa, ate seis vezes no mesmo ramo.
func TestBuffComSkillForaDaTabelaEhRecusado(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 820, ItemTypePotionBuff, 9999, "Pocao quebrada")
	s.Inventory.Slots[0] = model.Item{Index: 820, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Inventory.Get(0).IsEmpty() {
		t.Error("o item foi consumido apontando para skill inexistente")
	}
}

// A origem nao conferia a duracao: um buff sem linha na tabela entrava e
// nunca saia.
func TestBuffSemDuracaoNaoEntra(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	// duracoesFixas da 60 s de 1 a 100; 500 fica sem duracao.
	skills.poe(500, gamedata.T_SkillData{})
	poeConsumivel(tab, 821, ItemTypePotionBuff, 500, "Pocao eterna")
	s.Inventory.Slots[0] = model.Item{Index: 821, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Base.Buffs.Has(500) {
		t.Error("um buff sem duracao entrou")
	}
	if s.Inventory.Get(0).IsEmpty() {
		t.Error("o item foi consumido sem efeito")
	}
}

// O defeito central do $21B: a origem nunca chamava DecreaseAmount nem
// SendRefreshItemSlot, entao o item de buff era infinito.
func TestItemDeBuffDoCodigo21BEhConsumido(t *testing.T) {
	h, tab, skills := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	skills.poe(80, gamedata.T_SkillData{})
	poeConsumivel(tab, 830, ItemTypeBuff, 80, "Buque de rosas")
	s.Inventory.Slots[0] = model.Item{Index: 830, Refi: 2}

	if err := h.useBuffItem(context.Background(), e.conn, s,
		asPacket(t, useBuffItemPacket(0))); err != nil {
		t.Fatal(err)
	}
	if !s.Base.Buffs.Has(80) {
		t.Fatal("o buff nao entrou")
	}
	if got := s.Inventory.Get(0).Refi; got != 1 {
		t.Errorf("quantidade = %d, queria 1: o item nao foi consumido", got)
	}
}

func TestItemDeBuffDoCodigo21BRecusaSlotForaDaFaixa(t *testing.T) {
	h, _, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []uint32{64, 1000, 0xFFFFFFFF} {
		if err := h.useBuffItem(context.Background(), e.conn, s,
			asPacket(t, useBuffItemPacket(slot))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
}

// ---- bolsas

// A origem copiava para Inventory[63] SEM conferir se ja havia bolsa ali:
// usar um segundo simbolo apagava o primeiro e os 15 slots que ele habilitava.
func TestBolsaDoInventarioNaoApagaAExistente(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 900, ItemTypeBagInv, 0, "Simbolo do Viajante")
	// sessaoComInventario ja poe bolsas em 60..63.
	bolsaAntes := s.Inventory.Get(InventoryBagLastSlot)
	s.Inventory.Slots[0] = model.Item{Index: 900, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Inventory.Get(InventoryBagLastSlot).Index != bolsaAntes.Index {
		t.Errorf("a bolsa do slot 63 mudou: %d -> %d",
			bolsaAntes.Index, s.Inventory.Get(InventoryBagLastSlot).Index)
	}
	if s.Inventory.Get(0).Index != 900 {
		t.Error("o simbolo foi consumido com o slot 63 ocupado")
	}
}

func TestBolsaDoInventarioEntraNoSlot63Vazio(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 900, ItemTypeBagInv, 0, "Simbolo do Viajante")
	s.Inventory.Slots[InventoryBagLastSlot] = model.Item{}
	s.Inventory.Slots[0] = model.Item{Index: 900, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Inventory.Get(InventoryBagLastSlot).Index != 900 {
		t.Errorf("slot 63 = %d, queria 900",
			s.Inventory.Get(InventoryBagLastSlot).Index)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("o simbolo ficou tambem no slot de origem")
	}
}

// O laco da origem era "for i := 1 to 3" sobre Storage.Itens[80 + i]: o slot
// 80 nunca era considerado, e sem Break ficava com o ULTIMO vazio.
func TestExpansaoDoArmazemUsaOSlot80(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 901, ItemTypeBagStorage, 0, "Simbolo da Determinacao")
	// sessaoComArmazem enche 80..83; esvazia todos para a expansao caber.
	for i := range StorageBagSlots {
		s.Storage.Slots[StorageBagFirstSlot+i] = model.Item{}
	}
	s.Inventory.Slots[0] = model.Item{Index: 901, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Storage.Get(StorageBagFirstSlot).Index != 901 {
		t.Errorf("slot 80 = %d, queria 901 (a origem comecava no 81)",
			s.Storage.Get(StorageBagFirstSlot).Index)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("o simbolo ficou no inventario")
	}
}

func TestExpansaoDoArmazemPegaOPrimeiroVazio(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 901, ItemTypeBagStorage, 0, "Simbolo da Determinacao")
	// 80 ocupado, 81 e 83 vazios: a origem pegaria o 83, o ultimo.
	s.Storage.Slots[81] = model.Item{}
	s.Storage.Slots[83] = model.Item{}
	s.Inventory.Slots[0] = model.Item{Index: 901, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Storage.Get(81).Index != 901 {
		t.Errorf("slot 81 = %d, queria 901", s.Storage.Get(81).Index)
	}
	if !s.Storage.Get(83).IsEmpty() {
		t.Error("a expansao foi para o ultimo vazio, nao o primeiro")
	}
}

func TestExpansaoDoArmazemCheiaEhRecusada(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 901, ItemTypeBagStorage, 0, "Simbolo da Determinacao")
	// sessaoComArmazem ja deixa 80..83 cheios.
	s.Inventory.Slots[0] = model.Item{Index: 901, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Inventory.Get(0).Index != 901 {
		t.Error("o simbolo foi consumido com o armazem sem espaco")
	}
}

// Agora que o armazem esta portado, o "Simbolo da Confianca" abre a janela.
func TestSimboloDaConfiancaAbreOArmazem(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 902, ItemTypeStorageOpen, 0, "Simbolo da Confianca")
	s.Inventory.Slots[0] = model.Item{Index: 902, Refi: 1}

	usa(t, h, e, s, 0)
	if s.OpenedOption != NPCOptionStorage {
		t.Errorf("opcao = %d, queria %d", s.OpenedOption, NPCOptionStorage)
	}
	if s.OpenedNPC != s.ClientID {
		t.Errorf("npc = %d, queria o proprio cliente %d", s.OpenedNPC, s.ClientID)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("o simbolo nao foi consumido")
	}
}

// ---- pedra de reparo

// O ramo 708 indexava Equip[Packet.Type1] com Type1 DWORD do cliente, num
// vetor ARRAY [0..15], e ESCREVIA ali. O ramo 709 tinha a checagem; o 708 nao.
func TestPedraDeReparoNaoEscreveForaDoEquipamento(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 950, ItemTypeRepairWeapon, 0, "Pedra de arma")
	s.Inventory.Slots[0] = model.Item{Index: 950, Refi: 5}

	// 16 ja e fora do equipamento; acima disso a origem tratava como
	// inventario com deslocamento, e um Type1 enorme estourava os dois.
	for _, tipo1 := range []uint32{16, 17, 80, 1000, 0xFFFFFFFF} {
		if err := h.repairWithItem(e.conn, s, 0, tipo1); err != nil {
			t.Fatalf("Type1 %d: %v", tipo1, err)
		}
	}
	if got := s.Inventory.Get(0).Refi; got != 5 {
		t.Errorf("quantidade = %d: a pedra foi consumida sem reparo", got)
	}
}

func TestPedraDeReparoConsertaAArmaEquipada(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 950, ItemTypeRepairWeapon, 0, "Pedra de arma")
	// Arma: EquipSlot 6 vem de ItemType 6 na tabela.
	poeConsumivel(tab, 1000, 6, 0, "Espada")
	s.Inventory.Slots[0] = model.Item{Index: 950, Refi: 2}
	s.Equip.Slots[WeaponSlot] = model.Item{Index: 1000, Min: 10, Max: 100}

	if err := h.repairWithItem(e.conn, s, 0, WeaponSlot); err != nil {
		t.Fatal(err)
	}
	if got := s.Equip.Get(WeaponSlot).Min; got != 100 {
		t.Errorf("durabilidade = %d, queria 100", got)
	}
	if got := s.Inventory.Get(0).Refi; got != 1 {
		t.Errorf("quantidade da pedra = %d, queria 1", got)
	}
}

func TestPedraDeArmaNaoConsertaArmadura(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 950, ItemTypeRepairWeapon, 0, "Pedra de arma")
	poeConsumivel(tab, 1001, 3, 0, "Armadura") // EquipSlot 3
	s.Inventory.Slots[0] = model.Item{Index: 950, Refi: 2}
	s.Equip.Slots[3] = model.Item{Index: 1001, Min: 10, Max: 100}

	if err := h.repairWithItem(e.conn, s, 0, 3); err != nil {
		t.Fatal(err)
	}
	if got := s.Equip.Get(3).Min; got != 10 {
		t.Errorf("durabilidade = %d, queria 10 intacta", got)
	}
	if got := s.Inventory.Get(0).Refi; got != 2 {
		t.Errorf("quantidade da pedra = %d: foi consumida sem reparar", got)
	}
}

func TestPedraDeArmaduraConsertaNoInventario(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 951, ItemTypeRepairArmor, 0, "Pedra de armadura")
	poeConsumivel(tab, 1001, 3, 0, "Armadura")
	s.Inventory.Slots[0] = model.Item{Index: 951, Refi: 1}
	s.Inventory.Slots[5] = model.Item{Index: 1001, Min: 1, Max: 80}

	// Alvo no inventario: Type1 vem com deslocamento de 16.
	if err := h.repairWithItem(e.conn, s, 0, uint32(5+store.EquipSlots)); err != nil {
		t.Fatal(err)
	}
	if got := s.Inventory.Get(5).Min; got != 80 {
		t.Errorf("durabilidade = %d, queria 80", got)
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("a pedra nao foi consumida")
	}
}

// A mensagem "Item reparado com sucesso." ficava fora do if na origem e era
// enviada mesmo sem reparo; e a pedra era consumida de qualquer forma.
func TestPedraNaoEhConsumidaComItemJaInteiro(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 950, ItemTypeRepairWeapon, 0, "Pedra de arma")
	poeConsumivel(tab, 1000, 6, 0, "Espada")
	s.Inventory.Slots[0] = model.Item{Index: 950, Refi: 3}
	s.Equip.Slots[WeaponSlot] = model.Item{Index: 1000, Min: 100, Max: 100}

	if err := h.repairWithItem(e.conn, s, 0, WeaponSlot); err != nil {
		t.Fatal(err)
	}
	if got := s.Inventory.Get(0).Refi; got != 3 {
		t.Errorf("quantidade = %d: a pedra foi gasta num item inteiro", got)
	}
}

func TestTipoDeSlotDesconhecidoNoUsoNaoQuebra(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 500, ItemTypeHPPotion, 100, "Pocao")
	s.Inventory.Slots[0] = model.Item{Index: 500, Refi: 3}
	s.Avatar.MaxHP, s.Avatar.CurHP = 1000, 1

	for _, tipo := range []uint32{0, 2, 3, 5, 6, 21, 999} {
		if err := h.useItem(context.Background(), e.conn, s,
			asPacket(t, useItemPacket(tipo, 0, 0))); err != nil {
			t.Fatalf("tipo %d: %v", tipo, err)
		}
	}
	if s.Avatar.CurHP != 1 || s.Inventory.Get(0).Refi != 3 {
		t.Errorf("HP=%d quantidade=%d: um tipo de slot errado teve efeito",
			s.Avatar.CurHP, s.Inventory.Get(0).Refi)
	}
}

// Tipo de item sem porte: a origem saia por "else Exit", sem mensagem, e o
// cliente ficava esperando.
func TestTipoDeItemSemPorteAvisaENaoConsome(t *testing.T) {
	h, tab, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	poeConsumivel(tab, 960, ItemTypeRecipe, 0, "Receita")
	s.Inventory.Slots[0] = model.Item{Index: 960, Refi: 1}

	usa(t, h, e, s, 0)
	if s.Inventory.Get(0).IsEmpty() {
		t.Error("o item sem porte foi consumido")
	}
	if p := e.proximo(t); p.code != CodeClientMsg {
		t.Errorf("codigo %#x, queria a mensagem ao cliente %#x", p.code, CodeClientMsg)
	}
}

// ---- auxiliares

// duracoesAmplas da duracao a qualquer buff, para os casos cujo indice fica
// fora da faixa de duracoesFixas.
type duracoesAmplas struct{}

func (duracoesAmplas) BuffSeconds(index uint16) int {
	if index == 0 {
		return 0
	}
	return 60
}

// nomeSkill monta o campo Name de T_SkillData.
func nomeSkill(s string) [64]byte {
	var out [64]byte
	copy(out[:], s)
	return out
}
