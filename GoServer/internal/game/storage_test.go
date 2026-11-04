package game

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"aika/internal/guild"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// ---- montagem

// sessaoComArmazem devolve uma sessao com o armazem pronto e as expansoes
// postas nos slots 80..83, que e o que a origem exigia.
func sessaoComArmazem(nome string, charID uint32) *Session {
	s := lutador(nome, 100, 100)
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: charID, Name: nome}
	s.Account.ID = 7000
	s.Storage = store.ItemSet{Slots: make([]model.Item, store.StorageSlots)}
	for _, b := range []int{80, 81, 82, 83} {
		s.Storage.Slots[b] = model.Item{Index: 9100}
	}
	return s
}

func openNPCPacket(index, tipo1 uint32) []byte {
	var p packets.OpenNPCPacket
	p.Index = index
	p.Type1 = tipo1
	buf, err := netsrv.Marshal(CodeOpenNPC, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func chestGoldPacket(tipo uint32, valor int64) []byte {
	var p packets.ChangeChestGoldPacket
	p.ChestType = tipo
	p.Value = valor
	buf, err := netsrv.Marshal(CodeChangeChestGold, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func signalPacket(code uint16, data uint32) []byte {
	var p packets.SignalData
	p.Data = data
	buf, err := netsrv.Marshal(code, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// handlerComArmazem devolve um handler com canal, guildas e itens falsos.
func handlerComArmazem(t *testing.T) (*Handler, *tabelaDeItens, *itensFalsos) {
	t.Helper()
	tab := novaTabelaDeItens()
	itens := &itensFalsos{gravados: map[string]store.ItemSet{}}
	h := testHandler()
	h.Chan = NewChannel(0, 20)
	h.Data = tab
	h.Items = itens
	h.Guilds = guild.NewRegistry()
	h.Parties = NewPartyRegistry()
	return h, tab, itens
}

// abreArmazem poe a sessao no canal e abre a janela do armazem.
func abreArmazem(t *testing.T, h *Handler, s *Session, e *espiao) {
	t.Helper()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	if err := h.openNPC(context.Background(), e.conn, s,
		asPacket(t, openNPCPacket(2500, NPCOptionStorage))); err != nil {
		t.Fatal(err)
	}
	if s.OpenedOption != NPCOptionStorage || s.OpenedNPC != 2500 {
		t.Fatalf("janela do armazem nao abriu: npc=%d opcao=%d",
			s.OpenedNPC, s.OpenedOption)
	}
}

// ---- janela do NPC

// A origem punha "OpennedNPC := Packet.Index; OpennedOption := Type1" no fim
// da funcao, e o ramo do armazem ($7) nao passava por la: a janela nunca era
// marcada como aberta, o que e exatamente por que a guarda do deposito esta
// comentada no codigo da origem.
func TestOpcaoDeArmazemMarcaAJanelaAberta(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
}

// Packet.Index e DWORD e ia direto para "Servers[].NPCs[Packet.Index]".
func TestIndiceDeNPCAcimaDoWordEhRecusado(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	for _, idx := range []uint32{0x10000, 0xFFFFFFFF} {
		if err := h.openNPC(context.Background(), e.conn, s,
			asPacket(t, openNPCPacket(idx, NPCOptionStorage))); err != nil {
			t.Fatal(err)
		}
		if s.OpenedNPC != 0 || s.OpenedOption != 0 {
			t.Fatalf("indice %d abriu janela: npc=%d", idx, s.OpenedNPC)
		}
	}
}

func TestOpcaoDesconhecidaFechaAJanela(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)

	if err := h.openNPC(context.Background(), e.conn, s,
		asPacket(t, openNPCPacket(2500, 0x5))); err != nil { // loja, nao portada
		t.Fatal(err)
	}
	if s.OpenedNPC != 0 || s.OpenedOption != 0 {
		t.Errorf("janela ficou aberta: npc=%d opcao=%d", s.OpenedNPC, s.OpenedOption)
	}
}

// "if (Packet.Data = 0) then Exit" e "if not(OpennedNPC >= 2048) then Exit".
func TestCloseNPCOptionRespeitaAsGuardasDaOrigem(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)

	// Data zero nao fecha.
	if err := h.closeNPCOption(context.Background(), e.conn, s,
		asPacket(t, signalPacket(CodeCloseNPCOption, 0))); err != nil {
		t.Fatal(err)
	}
	if s.OpenedNPC == 0 {
		t.Error("Data zero fechou a janela")
	}
	// NPC abaixo de 2048 nao fecha.
	s.OpenedNPC = 100
	if err := h.closeNPCOption(context.Background(), e.conn, s,
		asPacket(t, signalPacket(CodeCloseNPCOption, 1))); err != nil {
		t.Fatal(err)
	}
	if s.OpenedNPC != 100 {
		t.Error("NPC abaixo de 2048 fechou a janela")
	}
	// Com os dois em ordem, fecha.
	s.OpenedNPC = 2500
	if err := h.closeNPCOption(context.Background(), e.conn, s,
		asPacket(t, signalPacket(CodeCloseNPCOption, 1))); err != nil {
		t.Fatal(err)
	}
	if s.OpenedNPC != 0 || s.OpenedOption != 0 {
		t.Errorf("janela nao fechou: npc=%d opcao=%d", s.OpenedNPC, s.OpenedOption)
	}
}

// ---- itens do armazem

func TestStorageBagSlot(t *testing.T) {
	casos := map[int]int{
		0: 80, 19: 80, 20: 81, 39: 81, 40: 82, 59: 82, 60: 83, 79: 83,
		80: -1, 84: -1, 86: -1, 1000: -1,
	}
	for slot, quer := range casos {
		if got := storageBagSlot(slot); got != quer {
			t.Errorf("storageBagSlot(%d) = %d, queria %d", slot, got, quer)
		}
	}
}

// A guarda da janela existia so no DESTINO na origem (e comentada na
// origem do arrasto): tirar item do armazem funcionava de qualquer lugar.
func TestArmazemFechadoRecusaOsDoisLados(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Storage.Slots[0] = model.Item{Index: 300}
	s.Inventory.Slots[0] = model.Item{Index: 400}

	// Sem abrir a janela, nos dois sentidos.
	for _, mov := range [][4]uint16{
		{store.SlotTypeStorage, 0, store.SlotTypeInventory, 1},
		{store.SlotTypeInventory, 0, store.SlotTypeStorage, 1},
	} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(mov[0], mov[1], mov[2], mov[3]))); err != nil {
			t.Fatal(err)
		}
	}
	if s.Storage.Get(0).Index != 300 || s.Inventory.Get(0).Index != 400 {
		t.Error("o movimento passou com o armazem fechado")
	}
}

func TestArmazemExigeExpansaoDoGrupo(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Inventory.Slots[0] = model.Item{Index: 400}
	// Tira a expansao do segundo grupo (slots 20..39).
	s.Storage.Slots[81] = model.Item{}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeStorage, 25))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(0).Index != 400 || !s.Storage.Get(25).IsEmpty() {
		t.Error("o movimento passou sem a expansao")
	}
}

func TestArmazemGuardaERetiraItem(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Inventory.Slots[0] = model.Item{Index: 400, Refi: 3}

	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeStorage, 5))); err != nil {
		t.Fatal(err)
	}
	if s.Storage.Get(5).Index != 400 || !s.Inventory.Get(0).IsEmpty() {
		t.Fatalf("guardar falhou: armazem=%d inventario=%d",
			s.Storage.Get(5).Index, s.Inventory.Get(0).Index)
	}
	if err := h.moveItem(context.Background(), e.conn, s,
		asPacket(t, moveItemPacket(store.SlotTypeStorage, 5,
			store.SlotTypeInventory, 2))); err != nil {
		t.Fatal(err)
	}
	if s.Inventory.Get(2).Index != 400 || !s.Storage.Get(5).IsEmpty() {
		t.Errorf("retirar falhou: inventario=%d armazem=%d",
			s.Inventory.Get(2).Index, s.Storage.Get(5).Index)
	}
}

// Os slots 84 e 85 sao a central de pran, que nao esta portada; 80..83 sao
// as expansoes, que a origem nao deixava arrastar.
func TestSlotsDeExpansaoEPranNaoSeMovem(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Storage.Slots[84] = model.Item{Index: 777}

	for _, slot := range []uint16{80, 81, 82, 83, 84, 85, 86, 1000, 65535} {
		if err := h.moveItem(context.Background(), e.conn, s,
			asPacket(t, moveItemPacket(store.SlotTypeStorage, slot,
				store.SlotTypeInventory, 1))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		if !s.Inventory.Get(1).IsEmpty() {
			t.Fatalf("slot %d foi movido para o inventario", slot)
		}
	}
	if s.Storage.Get(80).Index != 9100 || s.Storage.Get(84).Index != 777 {
		t.Error("os slots de expansao ou de pran mudaram")
	}
}

// ---- ouro do armazem

func TestOuroDoArmazemIdaEVolta(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Avatar.Gold = 1000
	s.StorageGold = 0

	if err := h.changeChestGold(context.Background(), e.conn, s,
		asPacket(t, chestGoldPacket(store.SlotTypeStorage, 400))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 600 || s.StorageGold != 400 {
		t.Fatalf("deposito: bolsa=%d armazem=%d", s.Avatar.Gold, s.StorageGold)
	}
	if err := h.changeChestGold(context.Background(), e.conn, s,
		asPacket(t, chestGoldPacket(store.SlotTypeStorage, -150))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 750 || s.StorageGold != 250 {
		t.Errorf("retirada: bolsa=%d armazem=%d", s.Avatar.Gold, s.StorageGold)
	}
}

func TestOuroDoArmazemSemSaldoNaoAnda(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Avatar.Gold = 100
	s.StorageGold = 50

	for _, v := range []int64{500, -500} {
		if err := h.changeChestGold(context.Background(), e.conn, s,
			asPacket(t, chestGoldPacket(store.SlotTypeStorage, v))); err != nil {
			t.Fatal(err)
		}
	}
	if s.Avatar.Gold != 100 || s.StorageGold != 50 {
		t.Errorf("o ouro andou: bolsa=%d armazem=%d", s.Avatar.Gold, s.StorageGold)
	}
}

// A origem chamava abs(Packet.Value) sem recusar o minimo do Int64, cujo
// modulo nao cabe no tipo e volta negativo: as duas guardas de saldo passavam
// por estarem comparando com um numero negativo.
func TestMinimoDoInt64NaoMoveOuro(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Avatar.Gold = 1000
	s.StorageGold = 1000

	if err := h.changeChestGold(context.Background(), e.conn, s,
		asPacket(t, chestGoldPacket(store.SlotTypeStorage, -9223372036854775808))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 1000 || s.StorageGold != 1000 {
		t.Errorf("bolsa=%d armazem=%d, queria os dois intactos",
			s.Avatar.Gold, s.StorageGold)
	}
}

func TestOuroDoArmazemRespeitaOTeto(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.StorageGold = MaxStorageGold - 10
	s.Avatar.Gold = 1000

	if err := h.changeChestGold(context.Background(), e.conn, s,
		asPacket(t, chestGoldPacket(store.SlotTypeStorage, 100))); err != nil {
		t.Fatal(err)
	}
	if s.StorageGold != MaxStorageGold-10 {
		t.Errorf("armazem = %d, queria intacto", s.StorageGold)
	}
}

func TestTipoDeBauDesconhecidoNaoMoveOuro(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Avatar.Gold = 1000

	for _, tipo := range []uint32{0, 1, 4, 10, 999} {
		if err := h.changeChestGold(context.Background(), e.conn, s,
			asPacket(t, chestGoldPacket(tipo, 100))); err != nil {
			t.Fatalf("tipo %d: %v", tipo, err)
		}
	}
	if s.Avatar.Gold != 1000 {
		t.Errorf("bolsa = %d, queria 1000", s.Avatar.Gold)
	}
}

func TestOuroDoArmazemFechadoNaoAnda(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Avatar.Gold = 1000

	if err := h.changeChestGold(context.Background(), e.conn, s,
		asPacket(t, chestGoldPacket(store.SlotTypeStorage, 100))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 1000 || s.StorageGold != 0 {
		t.Errorf("bolsa=%d armazem=%d com a janela fechada",
			s.Avatar.Gold, s.StorageGold)
	}
}

// ---- bau de guilda

// guildaParaBau cria a guilda, poe os dois na mesma e devolve as sessoes.
func guildaParaBau(t *testing.T, h *Handler) (*guild.Guild, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	sA := sessaoComArmazem("Lider", 11)
	eA := novoEspiao(t)
	if _, err := h.Chan.Join(eA.conn, sA); err != nil {
		t.Fatal(err)
	}
	sB := sessaoComArmazem("Membro", 22)
	sB.Account.ID = 7001
	eB := novoEspiao(t)
	if _, err := h.Chan.Join(eB.conn, sB); err != nil {
		t.Fatal(err)
	}

	g, err := h.Guilds.Create("Alfa", 1, guild.Member{CharIndex: 11, Name: "Lider"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddMember(guild.Member{CharIndex: 22, Name: "Membro", Rank: 1}); err != nil {
		t.Fatal(err)
	}
	h.Guilds.Put(g)
	g.RanksConfig[1] = guild.PermOpenChest | guild.PermUseChest
	sA.Avatar.GuildIndex = g.Index
	sB.Avatar.GuildIndex = g.Index
	return g, sA, eA, sB, eB
}

// abreBau abre o bau pela opcao $B do NPC.
func abreBau(t *testing.T, h *Handler, s *Session, e *espiao) {
	t.Helper()
	if err := h.openNPC(context.Background(), e.conn, s,
		asPacket(t, openNPCPacket(2600, NPCOptionGuildChest))); err != nil {
		t.Fatal(err)
	}
}

// O bau nao abria na origem: OpenChest recebia Player.Character.Index, um
// campo que nunca e atribuido em lugar nenhum do servidor e valia sempre
// zero, entao GetPlayerByCharIndex(0) falhava e o ramo saia.
func TestBauDeGuildaAbreEEnviaOsTresPacotes(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)

	abreBau(t, h, sA, eA)
	if sA.OpenedOption != NPCOptionGuildChest || sA.OpenedNPC != 2600 {
		t.Fatalf("janela do bau: npc=%d opcao=%d", sA.OpenedNPC, sA.OpenedOption)
	}
	if !g.Chest.IsOpenFor(11) {
		t.Fatal("o bau nao ficou reservado para quem abriu")
	}
	vistos := map[uint16]bool{}
	for range 3 {
		vistos[eA.proximo(t).code] = true
	}
	for _, code := range []uint16{CodeGuildChestPermission, CodeGuildChest,
		CodeRefreshGuildChestGold} {
		if !vistos[code] {
			t.Errorf("pacote %#x nao foi enviado", code)
		}
	}
}

// A origem lia "Members[FindMemberFromCharIndex(...)].Rank" sem conferir o
// retorno: para quem nao era membro isso e Members[-1].
func TestBauRecusaQuemNaoEhMembro(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, _, _, _, _ := guildaParaBau(t, h)

	sC := sessaoComArmazem("Fora", 33)
	eC := novoEspiao(t)
	if _, err := h.Chan.Join(eC.conn, sC); err != nil {
		t.Fatal(err)
	}
	// Ele diz ter a guilda, mas nao esta na lista de membros.
	sC.Avatar.GuildIndex = g.Index

	abreBau(t, h, sC, eC)
	if sC.OpenedOption == NPCOptionGuildChest {
		t.Error("um nao-membro abriu o bau")
	}
	if g.Chest.Holder() != 0 {
		t.Errorf("o bau ficou com o personagem %d", g.Chest.Holder())
	}
}

func TestBauRecusaRankSemPermissao(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, _, _, sB, eB := guildaParaBau(t, h)
	g.RanksConfig[1] = 0 // sem OpenChest nem UseChest

	abreBau(t, h, sB, eB)
	if sB.OpenedOption == NPCOptionGuildChest || g.Chest.Holder() != 0 {
		t.Error("rank sem permissao abriu o bau")
	}
}

func TestBauRecusaSegundoMembroEnquantoOPrimeiroUsa(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, sB, eB := guildaParaBau(t, h)

	abreBau(t, h, sA, eA)
	abreBau(t, h, sB, eB)
	if sB.OpenedOption == NPCOptionGuildChest {
		t.Error("o segundo membro abriu o bau ocupado")
	}
	if g.Chest.Holder() != 11 {
		t.Errorf("o bau mudou de dono: %d", g.Chest.Holder())
	}
	_ = sA
}

func TestItemVaiDoInventarioParaOBauEVolta(t *testing.T) {
	h, _, itens := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	sA.Inventory.Slots[0] = model.Item{Index: 555, Refi: 2}

	if err := h.moveItem(context.Background(), eA.conn, sA,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeGuildChest, 3))); err != nil {
		t.Fatal(err)
	}
	if it, _ := g.Chest.At(3); it.Index != 555 {
		t.Fatalf("bau slot 3 = %d, queria 555", it.Index)
	}
	if !sA.Inventory.Get(0).IsEmpty() {
		t.Fatal("o item ficou tambem no inventario")
	}
	if _, ok := itens.gravados["bau:1"]; !ok {
		t.Error("o bau nao foi gravado depois do movimento")
	}

	if err := h.moveItem(context.Background(), eA.conn, sA,
		asPacket(t, moveItemPacket(store.SlotTypeGuildChest, 3,
			store.SlotTypeInventory, 1))); err != nil {
		t.Fatal(err)
	}
	if sA.Inventory.Get(1).Index != 555 {
		t.Errorf("inventario slot 1 = %d", sA.Inventory.Get(1).Index)
	}
	if it, _ := g.Chest.At(3); !it.IsEmpty() {
		t.Errorf("o item ficou no bau: %d", it.Index)
	}
}

// A origem fazia "if (Packet.srcSlot < 50)" mas chegava em
// "@Guild.Chest.Items[slot]" sem guilda validada; o slot acima de 49 nunca
// deve ser tocado.
func TestSlotDoBauForaDaFaixa(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	_, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	sA.Inventory.Slots[0] = model.Item{Index: 555}

	for _, slot := range []uint16{50, 51, 100, 1000, 65535} {
		if err := h.moveItem(context.Background(), eA.conn, sA,
			asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
				store.SlotTypeGuildChest, slot))); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
	}
	if sA.Inventory.Get(0).Index != 555 {
		t.Error("o item saiu do inventario para um slot invalido")
	}
}

// Sem a janela aberta nenhum lado do bau pode ser tocado.
func TestBauFechadoRecusaMovimento(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	g.Chest.Do(func(itens []model.Item) { itens[0] = model.Item{Index: 900} })
	sA.Inventory.Slots[0] = model.Item{Index: 555}

	for _, mov := range [][4]uint16{
		{store.SlotTypeGuildChest, 0, store.SlotTypeInventory, 1},
		{store.SlotTypeInventory, 0, store.SlotTypeGuildChest, 1},
	} {
		if err := h.moveItem(context.Background(), eA.conn, sA,
			asPacket(t, moveItemPacket(mov[0], mov[1], mov[2], mov[3]))); err != nil {
			t.Fatal(err)
		}
	}
	if it, _ := g.Chest.At(0); it.Index != 900 {
		t.Error("o bau mudou com a janela fechada")
	}
	if sA.Inventory.Get(0).Index != 555 {
		t.Error("o inventario mudou com a janela fechada")
	}
}

// Quem nao tem o bau aberto nao mexe nele, mesmo sendo membro: na origem a
// comparacao de posse era entre dois -1 e passava.
func TestOutroMembroNaoMexeNoBauAbertoPorTerceiro(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, sB, eB := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	// B forja a janela aberta sem ter passado pelo OpenChest.
	sB.OpenedNPC = 2600
	sB.OpenedOption = NPCOptionGuildChest
	sB.Inventory.Slots[0] = model.Item{Index: 555}

	if err := h.moveItem(context.Background(), eB.conn, sB,
		asPacket(t, moveItemPacket(store.SlotTypeInventory, 0,
			store.SlotTypeGuildChest, 5))); err != nil {
		t.Fatal(err)
	}
	if it, _ := g.Chest.At(5); !it.IsEmpty() {
		t.Errorf("o membro sem posse mexeu no bau: %d", it.Index)
	}
	if sB.Inventory.Get(0).Index != 555 {
		t.Error("o item saiu do inventario dele")
	}
}

func TestEquipamentoNaoTrocaDiretoComOBau(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	sA.Equip.Slots[2] = model.Item{Index: 200}

	for _, mov := range [][4]uint16{
		{store.SlotTypeEquip, 2, store.SlotTypeGuildChest, 0},
		{store.SlotTypeGuildChest, 0, store.SlotTypeEquip, 2},
	} {
		if err := h.moveItem(context.Background(), eA.conn, sA,
			asPacket(t, moveItemPacket(mov[0], mov[1], mov[2], mov[3]))); err != nil {
			t.Fatal(err)
		}
	}
	if sA.Equip.Get(2).Index != 200 {
		t.Error("o equipamento foi para o bau")
	}
	if it, _ := g.Chest.At(0); !it.IsEmpty() {
		t.Errorf("o bau recebeu equipamento: %d", it.Index)
	}
}

func TestItemTrocaDeSlotDentroDoBau(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	g.Chest.Do(func(itens []model.Item) { itens[2] = model.Item{Index: 901} })

	if err := h.moveItem(context.Background(), eA.conn, sA,
		asPacket(t, moveItemPacket(store.SlotTypeGuildChest, 2,
			store.SlotTypeGuildChest, 7))); err != nil {
		t.Fatal(err)
	}
	if it, _ := g.Chest.At(7); it.Index != 901 {
		t.Errorf("slot 7 = %d", it.Index)
	}
	if it, _ := g.Chest.At(2); !it.IsEmpty() {
		t.Errorf("slot 2 = %d", it.Index)
	}
}

// ---- ouro do bau

func TestOuroDoBauIdaEVolta(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	sA.Avatar.Gold = 1000

	if err := h.changeChestGold(context.Background(), eA.conn, sA,
		asPacket(t, chestGoldPacket(store.SlotTypeGuildChest, 600))); err != nil {
		t.Fatal(err)
	}
	if sA.Avatar.Gold != 400 || g.Chest.Gold() != 600 {
		t.Fatalf("deposito: bolsa=%d bau=%d", sA.Avatar.Gold, g.Chest.Gold())
	}
	if err := h.changeChestGold(context.Background(), eA.conn, sA,
		asPacket(t, chestGoldPacket(store.SlotTypeGuildChest, -200))); err != nil {
		t.Fatal(err)
	}
	if sA.Avatar.Gold != 600 || g.Chest.Gold() != 400 {
		t.Errorf("retirada: bolsa=%d bau=%d", sA.Avatar.Gold, g.Chest.Gold())
	}
}

// "Chest.Gold := Chest.Gold + UInt64(Packet.Value)" com a guarda comparando
// UInt64 com Int64: retirar mais do que ha nao pode dar a volta.
func TestRetirarAcimaDoSaldoDoBauNaoDaAVolta(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	g.Chest.SetGold(100)
	sA.Avatar.Gold = 0

	if err := h.changeChestGold(context.Background(), eA.conn, sA,
		asPacket(t, chestGoldPacket(store.SlotTypeGuildChest, -5000))); err != nil {
		t.Fatal(err)
	}
	if g.Chest.Gold() != 100 || sA.Avatar.Gold != 0 {
		t.Errorf("bau=%d bolsa=%d, queria os dois intactos",
			g.Chest.Gold(), sA.Avatar.Gold)
	}
}

func TestOuroDoBauSemPermissaoDeUso(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	sA.Avatar.Gold = 1000
	// Tira a permissao de uso depois de abrir: o lider tem tudo, entao o
	// teste usa um membro de rank 1.
	_ = g

	gB := h.Guilds.Of(22)
	gB.RanksConfig[1] = guild.PermOpenChest // abre mas nao usa
	sB := sessaoComArmazem("Membro", 22)
	sB.Avatar.GuildIndex = gB.Index
	eB := novoEspiao(t)
	if _, err := h.Chan.Join(eB.conn, sB); err != nil {
		t.Fatal(err)
	}
	// O lider solta o bau para o membro poder abrir.
	gB.Chest.Close(11)
	abreBau(t, h, sB, eB)
	sB.Avatar.Gold = 1000

	if err := h.changeChestGold(context.Background(), eB.conn, sB,
		asPacket(t, chestGoldPacket(store.SlotTypeGuildChest, 100))); err != nil {
		t.Fatal(err)
	}
	if gB.Chest.Gold() != 0 || sB.Avatar.Gold != 1000 {
		t.Errorf("bau=%d bolsa=%d sem permissao de uso",
			gB.Chest.Gold(), sB.Avatar.Gold)
	}
}

// ---- fechamento

func TestFecharOBauPeloPacoteDoCliente(t *testing.T) {
	h, _, itens := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)

	if err := h.closeGuildChest(context.Background(), eA.conn, sA,
		asPacket(t, signalPacket(CodeCloseGuildChest, 1))); err != nil {
		t.Fatal(err)
	}
	if g.Chest.Holder() != 0 {
		t.Error("o bau continuou reservado")
	}
	if sA.OpenedNPC != 0 || sA.OpenedOption != 0 {
		t.Error("a janela continuou aberta")
	}
	if _, ok := itens.gravados["bau:1"]; !ok {
		t.Error("o bau nao foi gravado no fechamento")
	}
}

// Nada no caminho de desconexao da origem chamava CloseChest: o bau ficava
// trancado para a guilda inteira ate o servidor reiniciar.
func TestSairDoJogoSoltaOBau(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)

	h.chestOnDisconnect(sA)
	if g.Chest.Holder() != 0 {
		t.Fatal("o bau continuou trancado depois da saida")
	}
	// E o outro membro consegue abrir em seguida.
	sB := sessaoComArmazem("Membro", 22)
	sB.Avatar.GuildIndex = g.Index
	eB := novoEspiao(t)
	if _, err := h.Chan.Join(eB.conn, sB); err != nil {
		t.Fatal(err)
	}
	abreBau(t, h, sB, eB)
	if g.Chest.Holder() != 22 {
		t.Errorf("o bau ficou com %d, queria 22", g.Chest.Holder())
	}
}

// O prazo da origem mandava o sinal mas nao fechava o bau.
func TestChestTickFechaEAvisaOCliente(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)
	for range 3 {
		eA.proximo(t) // permissao, bau, ouro
	}

	// Adianta o relogio do handler para depois do prazo.
	h.Now = func() time.Time { return agora.Add(guild.ChestTimeout + time.Minute) }
	h.ChestTick()

	if g.Chest.Holder() != 0 {
		t.Fatal("o bau continuou trancado depois do prazo")
	}
	if sA.OpenedOption != 0 || sA.OpenedNPC != 0 {
		t.Error("a janela continuou aberta depois do prazo")
	}
	if p := eA.proximo(t); p.code != CodeCloseGuildChest {
		t.Errorf("pacote %#x, queria %#x", p.code, CodeCloseGuildChest)
	}
}

// Fechar a janela do NPC com o bau aberto solta o bau: na origem os mais de
// quarenta pontos que zeravam OpennedOption nao soltavam.
func TestFecharAJanelaDoNPCSoltaOBau(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	abreBau(t, h, sA, eA)

	if err := h.openNPC(context.Background(), eA.conn, sA,
		asPacket(t, openNPCPacket(2600, NPCOptionCloseMenu))); err != nil {
		t.Fatal(err)
	}
	if g.Chest.Holder() != 0 {
		t.Error("fechar a janela nao soltou o bau")
	}
}

// ---- carga e gravacao

func TestBauEhLidoDoBancoNaPrimeiraAbertura(t *testing.T) {
	h, _, itens := handlerComArmazem(t)
	g, sA, eA, _, _ := guildaParaBau(t, h)
	// O banco responde com um item no slot 4 para a guilda 1.
	conj := store.ItemSet{Slots: make([]model.Item, store.GuildChestSlots)}
	conj.Slots[4] = model.Item{Index: 808}
	itens.bau = map[uint32]store.ItemSet{1: conj}

	abreBau(t, h, sA, eA)
	if it, _ := g.Chest.At(4); it.Index != 808 {
		t.Errorf("slot 4 = %d, queria 808 vindo do banco", it.Index)
	}
	if !g.Chest.IsLoaded() {
		t.Error("o bau nao ficou marcado como carregado")
	}
}

func TestArmazemEhGravadoNaSaida(t *testing.T) {
	h, _, itens := handlerComArmazem(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	abreArmazem(t, h, s, e)
	s.Storage.Slots[0] = model.Item{Index: 404}

	h.saveItems(s)
	got, ok := itens.gravados["armazem:7000"]
	if !ok {
		t.Fatalf("o armazem nao foi gravado: %v", itens.gravados)
	}
	if got.Get(0).Index != 404 {
		t.Errorf("gravado slot 0 = %d", got.Get(0).Index)
	}
}

func TestArmazemCurtoDoBancoEhCompletado(t *testing.T) {
	h, _, itens := handlerComArmazem(t)
	// O banco devolve menos slots do que o vetor da origem.
	itens.arm = map[uint32]store.ItemSet{
		7000: {Slots: make([]model.Item, 10)},
	}
	s := sessaoComArmazem("Alfa", 11)
	got := h.loadStorage(context.Background(), s)
	if len(got.Slots) != store.StorageSlots {
		t.Errorf("slots = %d, queria %d", len(got.Slots), store.StorageSlots)
	}
}

// ---- diálogo de dungeon pela janela do NPC

// A opcao 26 era a unica porta para o fluxo de dungeon ja portado
// (requestEnterDungeon exige OpennedOption = 26), e nada no porte marcava esse
// estado. A origem mandava o $119 com o nivel minimo da PRIMEIRA dificuldade,
// mesmo que o jogador fosse escolher outra.
func TestOpcaoDeDungeonMarcaAJanelaEMandaODialogo(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	tab := novaTabelaDungeon(2)
	h.DungeonTable = tab
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}

	if err := h.openNPC(context.Background(), e.conn, s,
		asPacket(t, openNPCPacket(2109, NPCOptionDungeon))); err != nil {
		t.Fatal(err)
	}
	if s.OpenedOption != DungeonNPCOption || s.OpenedNPC != 2109 {
		t.Fatalf("janela: npc=%d opcao=%d, queria 2109/%d",
			s.OpenedNPC, s.OpenedOption, DungeonNPCOption)
	}

	p := e.proximo(t)
	if p.code != CodeDungeonDialog {
		t.Fatalf("codigo %#x, queria %#x ($119)", p.code, CodeDungeonDialog)
	}
	// p.corpo e o pacote inteiro: o corpo comeca depois do cabecalho de 12
	// bytes, com Dungeon (DWORD), Entering (byte) e LevelMin (byte).
	const off = 12
	if len(p.corpo) < off+6 {
		t.Fatalf("pacote curto: %d bytes", len(p.corpo))
	}
	if got := binary.LittleEndian.Uint32(p.corpo[off:]); got != 1 {
		t.Errorf("dungeon = %d, queria 1", got)
	}
	if got := p.corpo[off+5]; got != 20 {
		t.Errorf("nivel minimo = %d, queria 20 (dificuldade 0)", got)
	}
}

// NPC que nao abre dungeon nenhuma nao quebra nada.
func TestOpcaoDeDungeonComNPCDesconhecido(t *testing.T) {
	h, _, _ := handlerComArmazem(t)
	h.DungeonTable = novaTabelaDungeon(1)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	if err := h.openNPC(context.Background(), e.conn, s,
		asPacket(t, openNPCPacket(9999, NPCOptionDungeon))); err != nil {
		t.Fatal(err)
	}
	// A janela abre (a origem tambem marcava), mas nenhum dialogo e enviado.
	if s.OpenedNPC != 9999 {
		t.Errorf("npc = %d", s.OpenedNPC)
	}
}
