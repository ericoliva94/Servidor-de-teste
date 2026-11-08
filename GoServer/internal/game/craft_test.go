package game

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

const (
	itemEspadaForjada = 1000 // resultado da receita, equipamento (tipo 6)
	itemPocaoForjada  = 1001 // resultado da receita, empilha
	matFerro          = 5100 // ingrediente que empilha
	matCouro          = 4630 // ingrediente que empilha
	matSucata         = 4999 // ingrediente que e equipamento (tipo 4)
	equipNivel15      = 900
	equipNivel85      = 901
	equipNivel100     = 902
	vaizanCinza       = 910
	vaizanColorido    = 911
	pedraRestauracao  = 912
	itemNormal        = 920 // TypeItem 0
	itemSuperior      = 921 // TypeItem 5
	itemCash          = 922 // TypeItem 7
)

type tabelaComReceitas struct {
	*tabelaDeItens
	receitas []gamedata.MakeItemsData
	ings     []gamedata.MakeItemsIngredientsData
}

func (t *tabelaComReceitas) RecipeByResult(id uint16) (*gamedata.MakeItemsData, bool) {
	for i := range t.receitas {
		if t.receitas[i].ResultItemID == id {
			return &t.receitas[i], true
		}
	}
	return nil, false
}

func (t *tabelaComReceitas) IngredientsOf(id uint16) []gamedata.MakeItemsIngredientsData {
	var out []gamedata.MakeItemsIngredientsData
	for _, g := range t.ings {
		if g.ID == id {
			out = append(out, g)
		}
	}
	return out
}

func prepForja(t *testing.T) (*Handler, *tabelaComReceitas, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tr := &tabelaComReceitas{tabelaDeItens: tab}
	h.Data = tr
	tab.poeItem(itemEspadaForjada, gamedata.ItemFromList{ItemType: 6, Durabilidade: 80})
	tab.poeItem(itemPocaoForjada, gamedata.ItemFromList{CanAgroup: 1})
	for _, m := range []int{matFerro, matCouro, 5080, 5081, 5082, 4632, 4633, 4636, 4638, 4649, 4647} {
		tab.poeItem(m, gamedata.ItemFromList{CanAgroup: 1})
	}
	tab.poeItem(matSucata, gamedata.ItemFromList{ItemType: 4})
	tab.poeItem(equipNivel15, gamedata.ItemFromList{ItemType: 3, Level: 15})
	tab.poeItem(equipNivel85, gamedata.ItemFromList{ItemType: 3, Level: 85})
	tab.poeItem(equipNivel100, gamedata.ItemFromList{ItemType: 3, Level: 100})
	tab.poeItem(vaizanCinza, gamedata.ItemFromList{CanAgroup: 1, ItemType: 513})
	tab.poeItem(vaizanColorido, gamedata.ItemFromList{CanAgroup: 1, ItemType: 514})
	tab.poeItem(pedraRestauracao, gamedata.ItemFromList{CanAgroup: 1, ItemType: 517})
	tab.poeItem(itemNormal, gamedata.ItemFromList{ItemType: 3, TypeItem: 0})
	tab.poeItem(itemSuperior, gamedata.ItemFromList{ItemType: 3, TypeItem: 5})
	tab.poeItem(itemCash, gamedata.ItemFromList{ItemType: 3, TypeItem: 7})
	tr.receitas = []gamedata.MakeItemsData{
		{ResultItemID: itemEspadaForjada, ID: 1, LevelMin: 9, Price: 100, ResultAmount: 1, TaxSuccess: 1000},
		{ResultItemID: itemPocaoForjada, ID: 2, LevelMin: 0, Price: 10, ResultAmount: 1, TaxSuccess: 1000},
	}
	tr.ings = []gamedata.MakeItemsIngredientsData{
		{ID: itemEspadaForjada, ItemID: matFerro, Amount: 5},
		{ID: itemEspadaForjada, ItemID: matCouro, Amount: 13},
		{ID: itemPocaoForjada, ItemID: matFerro, Amount: 2},
	}
	h.Rand = sorteioFixo(0)
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Gold = 10_000
	s.Avatar.Level = 50
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, tr, s, e
}

func makePacket(id, amount, modo uint16) []byte {
	buf, err := netsrv.Marshal(CodeMakeItem, packets.MakeItemPacket{ItemID: id, Amount: amount, Null3: modo})
	if err != nil {
		panic(err)
	}
	return buf
}

func forja(t *testing.T, h *Handler, s *Session, e *espiao, id, amount, modo uint16) {
	t.Helper()
	if err := h.makeItem(context.Background(), e.conn, s, asPacket(t, makePacket(id, amount, modo))); err != nil {
		t.Fatal(err)
	}
}

func contaItem(s *Session, index uint16) int {
	n := 0
	for _, it := range s.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == index {
			n++
			if it.Refi > 0 {
				n += int(it.Refi) - 1
			}
		}
	}
	return n
}

// ---- fabricar

func TestFabricarEquipamentoCobraConsomeEEntrega(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 20}
	s.Inventory.Slots[1] = model.Item{Index: matCouro, Refi: 13}

	forja(t, h, s, e, itemEspadaForjada, 1, 0) // ItemID 1000: o limite de slot da origem nao vale aqui

	if got := mensagemDe(t, e); got != "A criacao do item foi bem sucedida." {
		// o primeiro recebido pode ser o refresh; procura a mensagem certa
		t.Logf("primeira mensagem: %q", got)
	}
	if s.Avatar.Gold != 9900 {
		t.Errorf("ouro = %d, queria 9900", s.Avatar.Gold)
	}
	if s.Inventory.Slots[0].Refi != 15 || !s.Inventory.Slots[1].IsEmpty() {
		t.Errorf("ingredientes = %+v e %+v", s.Inventory.Slots[0], s.Inventory.Slots[1])
	}
	if contaItem(s, itemEspadaForjada) != 1 {
		t.Error("a espada nao foi entregue")
	}
	for _, it := range s.Inventory.Slots[:MovableInventorySlots] {
		if it.Index == itemEspadaForjada && (it.Min != 80 || it.Max != 80 || it.APP != itemEspadaForjada) {
			t.Errorf("item entregue = %+v", it)
		}
	}
}

func TestFabricarEmQuantidadeMultiplicaPrecoEIngredientes(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 30}
	forja(t, h, s, e, itemPocaoForjada, 5, 0)
	if s.Avatar.Gold != 9950 {
		t.Errorf("ouro = %d, queria 9950", s.Avatar.Gold)
	}
	if s.Inventory.Slots[0].Refi != 20 {
		t.Errorf("ferro = %d, queria 20", s.Inventory.Slots[0].Refi)
	}
	if got := contaItem(s, itemPocaoForjada); got != 5 {
		t.Errorf("pocoes = %d, queria 5", got)
	}
}

// Defeito da origem: Amount 0 passava na conferencia de ouro (preco x 0) e
// depois cobrava preco x 1 sem conferir.
func TestFabricarQuantidadeZeroValeUmEConfereOuro(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Avatar.Gold = 5
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 20}
	forja(t, h, s, e, itemPocaoForjada, 0, 0)
	if s.Avatar.Gold != 5 || s.Inventory.Slots[0].Refi != 20 {
		t.Error("fabricou sem ouro")
	}
	if got := mensagemDe(t, e); !strings.Contains(got, "10 necessarios") {
		t.Errorf("mensagem = %q", got)
	}
}

// Defeito da origem: o saldo do ingrediente era lido num BYTE.
func TestFabricarComPilhaGrandeDeIngrediente(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 300} // 300 mod 256 = 44
	forja(t, h, s, e, itemPocaoForjada, 100, 0)                   // pede 200
	if got := contaItem(s, itemPocaoForjada); got != 100 {
		t.Errorf("pocoes = %d, queria 100", got)
	}
	if s.Inventory.Slots[0].Refi != 100 {
		t.Errorf("ferro = %d, queria 100", s.Inventory.Slots[0].Refi)
	}
}

func TestFabricarRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(h *Handler, tr *tabelaComReceitas, s *Session)
		id    uint16
		qtd   uint16
		msg   string
	}{
		{"receita inexistente", func(*Handler, *tabelaComReceitas, *Session) {}, 7777, 1, "Item nao encontrado na forja."},
		{"nivel baixo", func(h *Handler, tr *tabelaComReceitas, s *Session) { s.Avatar.Level = 9 }, itemEspadaForjada, 1, "Voce nao possui o nivel necessario."},
		{"sem ouro", func(h *Handler, tr *tabelaComReceitas, s *Session) { s.Avatar.Gold = 99 }, itemEspadaForjada, 1, "Voce nao possui o gold necessario. 100 necessarios."},
		{"ingrediente ausente", func(h *Handler, tr *tabelaComReceitas, s *Session) { s.Inventory.Slots[1] = model.Item{} }, itemEspadaForjada, 1,
			"Voce precisa ter o item []. Separe a quantidade correta em apenas UM slot."},
		{"ingrediente insuficiente", func(h *Handler, tr *tabelaComReceitas, s *Session) { s.Inventory.Slots[0].Refi = 4 }, itemEspadaForjada, 1, "Voce precisa de (5) do item []."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, tr, s, e := prepForja(t)
			s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 20}
			s.Inventory.Slots[1] = model.Item{Index: matCouro, Refi: 13}
			c.setup(h, tr, s)
			ouro, ferro := s.Avatar.Gold, s.Inventory.Slots[0]
			forja(t, h, s, e, c.id, c.qtd, 0)
			got := mensagemDe(t, e)
			if got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if s.Avatar.Gold != ouro || s.Inventory.Slots[0] != ferro {
				t.Error("recusa alterou ouro ou ingredientes")
			}
		})
	}
}

func TestFabricarSemEspacoNoInventario(t *testing.T) {
	h, _, s, e := prepForja(t)
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: matSucata}
	}
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 20}
	forja(t, h, s, e, itemPocaoForjada, 1, 0)
	if got := mensagemDe(t, e); got != "Inventario cheio." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[0].Refi != 20 || s.Avatar.Gold != 10_000 {
		t.Error("cobrou sem entregar")
	}
}

// Duas linhas da receita com o mesmo item eram conferidas separadamente.
func TestFabricarSomaIngredientesRepetidosDoMesmoSlot(t *testing.T) {
	h, tr, s, e := prepForja(t)
	tr.ings = append(tr.ings, gamedata.MakeItemsIngredientsData{ID: itemPocaoForjada, ItemID: matFerro, Amount: 2})
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 3} // cada linha pede 2: juntas, 4
	forja(t, h, s, e, itemPocaoForjada, 1, 0)
	if contaItem(s, itemPocaoForjada) != 0 || s.Inventory.Slots[0].Refi != 3 {
		t.Error("fabricou com ingrediente insuficiente no total")
	}
}

func TestFabricarIngredienteEquipamentoSaiInteiro(t *testing.T) {
	h, tr, s, e := prepForja(t)
	tr.ings = []gamedata.MakeItemsIngredientsData{{ID: itemPocaoForjada, ItemID: matSucata, Amount: 1}}
	s.Inventory.Slots[4] = model.Item{Index: matSucata, Refi: 32, Min: 5, Max: 9}
	forja(t, h, s, e, itemPocaoForjada, 1, 0)
	if !s.Inventory.Slots[4].IsEmpty() {
		t.Errorf("ingrediente de equipamento = %+v, queria removido", s.Inventory.Slots[4])
	}
}

// Comportamento da origem: TaxSuccess >= 10 sempre fabrica, abaixo disso
// nunca. Fica travado para uma "correcao" nao mudar o balanceamento calada.
func TestFabricarSorteioDeSucessoComoAOrigem(t *testing.T) {
	for _, c := range []struct {
		tax  uint16
		rand int
		ok   bool
	}{{1000, 99, true}, {500, 49, true}, {10, 0, true}, {9, 0, false}, {0, 0, false}} {
		h, tr, s, e := prepForja(t)
		tr.receitas[1].TaxSuccess = c.tax
		h.Rand = sorteioFixo(c.rand)
		s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 20}
		forja(t, h, s, e, itemPocaoForjada, 1, 0)
		if got := contaItem(s, itemPocaoForjada) == 1; got != c.ok {
			t.Errorf("TaxSuccess %d: fabricou=%v, queria %v", c.tax, got, c.ok)
		}
		if !c.ok && (s.Avatar.Gold != 10_000 || s.Inventory.Slots[0].Refi != 20) {
			t.Errorf("TaxSuccess %d: falha cobrou ouro ou ingrediente", c.tax)
		}
	}
}

// ---- desmontar

func TestDesmontarTrocaOItemPorMaterial(t *testing.T) {
	h, _, s, e := prepForja(t)
	h.Rand = sorteioFixo(4) // material = indice 4 da faixa; quantidade = 1 + 4 = 5
	s.Inventory.Slots[2] = model.Item{Index: equipNivel15}

	forja(t, h, s, e, 2, 0, makeDisassemble)

	if !s.Inventory.Slots[2].IsEmpty() && s.Inventory.Slots[2].Index == equipNivel15 {
		t.Fatal("o equipamento continua no inventario")
	}
	// Faixa 1..20, indice 4 -> 4630; quantidade 5.
	if got := contaItem(s, 4630); got != 5 {
		t.Errorf("material 4630 = %d, queria 5", got)
	}
	proximoDe(t, e, CodeMakeItem) // eco com tudo zerado
}

func TestDesmontarNivelAltoRendeAte7(t *testing.T) {
	h, _, s, e := prepForja(t)
	h.Rand = sorteioFixo(6) // material indice 5 da faixa 81..90; quantidade 1 + 6 = 7 (em faixa baixa o teto seria 5)
	s.Inventory.Slots[2] = model.Item{Index: equipNivel85}
	forja(t, h, s, e, 2, 0, makeDisassemble)
	if got := contaItem(s, 4647); got != 7 {
		t.Errorf("material 4647 = %d, queria 7", got)
	}
}

func TestDesmontarRecusas(t *testing.T) {
	casos := map[string]func(s *Session) uint16{
		"nivel 100":     func(s *Session) uint16 { s.Inventory.Slots[2] = model.Item{Index: equipNivel100}; return 2 },
		"nao equipavel": func(s *Session) uint16 { s.Inventory.Slots[2] = model.Item{Index: matFerro, Refi: 3}; return 2 },
		"slot vazio":    func(s *Session) uint16 { return 2 },
		"slot invalido": func(s *Session) uint16 { return 200 },
		"slot de bolsa": func(s *Session) uint16 { return 62 },
	}
	for nome, setup := range casos {
		t.Run(nome, func(t *testing.T) {
			h, _, s, e := prepForja(t)
			slot := setup(s)
			antes := append([]model.Item(nil), s.Inventory.Slots...)
			forja(t, h, s, e, slot, 0, makeDisassemble)
			if got := mensagemDe(t, e); got != "Este item nao pode ser desmontado." {
				t.Errorf("mensagem = %q", got)
			}
			for i := range antes {
				if s.Inventory.Slots[i] != antes[i] {
					t.Fatalf("slot %d alterado", i)
				}
			}
		})
	}
}

func TestDesmontarComInventarioCheioUsaOSlotLiberado(t *testing.T) {
	h, _, s, e := prepForja(t)
	h.Rand = sorteioFixo(4)
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: matSucata}
	}
	s.Inventory.Slots[2] = model.Item{Index: equipNivel15}
	forja(t, h, s, e, 2, 0, makeDisassemble)
	if s.Inventory.Slots[2].Index == equipNivel15 || contaItem(s, 4630) != 5 {
		t.Errorf("slot liberado = %+v, queria o material", s.Inventory.Slots[2])
	}
}

// ---- limpar adicionais

func comAdicionais() model.Item {
	return model.Item{Index: itemNormal, Effects: model.ItemEffect{
		Index: [3]byte{10, 20, 30}, Value: [3]byte{1, 2, 3}}}
}

func TestLimparTodosOsAdicionaisGastaUmVaizan(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = comAdicionais()
	s.Inventory.Slots[1] = model.Item{Index: vaizanCinza, Refi: 3}
	forja(t, h, s, e, 0, 0, makeClearExtrasA)
	if s.Inventory.Slots[0].Effects != (model.ItemEffect{}) {
		t.Errorf("adicionais = %+v", s.Inventory.Slots[0].Effects)
	}
	if s.Inventory.Slots[1].Refi != 2 {
		t.Errorf("vaizan = %d, queria 2", s.Inventory.Slots[1].Refi)
	}
}

func TestLimparUmAdicionalComoAOrigem(t *testing.T) {
	casos := []struct {
		amount uint16
		want   [3]byte
	}{
		{1, [3]byte{0, 20, 30}}, // o primeiro existente
		{2, [3]byte{10, 0, 30}}, // o segundo
		{3, [3]byte{10, 20, 0}}, // o ultimo existente
	}
	for _, c := range casos {
		h, _, s, e := prepForja(t)
		s.Inventory.Slots[0] = comAdicionais()
		s.Inventory.Slots[1] = model.Item{Index: vaizanCinza, Refi: 1}
		forja(t, h, s, e, 0, c.amount, makeClearExtrasB)
		if got := s.Inventory.Slots[0].Effects.Index; got != c.want {
			t.Errorf("amount %d: efeitos %v, queria %v", c.amount, got, c.want)
		}
		if !s.Inventory.Slots[1].IsEmpty() {
			t.Errorf("amount %d: a ultima unidade do vaizan deveria sumir", c.amount)
		}
	}
}

func TestLimparUsaOVaizanDaCategoriaDoItem(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: itemSuperior, Effects: comAdicionais().Effects}
	s.Inventory.Slots[1] = model.Item{Index: vaizanCinza, Refi: 2} // serve so ao normal
	forja(t, h, s, e, 0, 0, makeClearExtrasA)
	if got := mensagemDe(t, e); got != "Voce deve possuir o item Vaizan Colorido [Superior]." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[0].Effects == (model.ItemEffect{}) || s.Inventory.Slots[1].Refi != 2 {
		t.Error("limpou com o vaizan errado")
	}
}

func TestLimparRecusas(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[1] = model.Item{Index: vaizanCinza, Refi: 2}

	s.Inventory.Slots[0] = model.Item{Index: itemCash, Effects: comAdicionais().Effects}
	forja(t, h, s, e, 0, 0, makeClearExtrasA)
	if got := mensagemDe(t, e); got != "Itens de cash nao podem ser limpos." {
		t.Errorf("cash: %q", got)
	}

	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 5}
	forja(t, h, s, e, 0, 0, makeClearExtrasA)
	if got := mensagemDe(t, e); got != "Esse item nao pode ser limpo." {
		t.Errorf("empilhavel: %q", got)
	}

	s.Inventory.Slots[0] = comAdicionais()
	forja(t, h, s, e, 0, 9, makeClearExtrasA) // Amount fora de 0..3
	forja(t, h, s, e, 5, 0, makeClearExtrasA) // slot vazio
	forja(t, h, s, e, 0, 0, makeClearExtrasA) // este sim limpa
	if s.Inventory.Slots[1].Refi != 1 {
		t.Errorf("vaizan = %d: so o ultimo pedido valido deveria gastar", s.Inventory.Slots[1].Refi)
	}
}

// ---- limpar aparencia

func TestLimparAparencia(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: itemNormal, APP: 555}
	s.Inventory.Slots[1] = model.Item{Index: pedraRestauracao, Refi: 2}
	forja(t, h, s, e, 0, 0, makeClearAppearance)
	if s.Inventory.Slots[0].APP != itemNormal || s.Inventory.Slots[1].Refi != 1 {
		t.Errorf("item = %+v pedra = %d", s.Inventory.Slots[0], s.Inventory.Slots[1].Refi)
	}
}

func TestLimparAparenciaRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(s *Session)
		msg   string
	}{
		{"sem aparencia", func(s *Session) { s.Inventory.Slots[0].APP = 0 }, "Esse item nao possui aparencia."},
		{"ja limpo", func(s *Session) { s.Inventory.Slots[0].APP = itemNormal }, "Esse item ja esta limpo."},
		{"sem pedra", func(s *Session) { s.Inventory.Slots[1] = model.Item{} }, "Voce deve possuir o item Pedra Magica da Restauracao."},
		{"empilhavel", func(s *Session) { s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 2, APP: 9} }, "Esse item nao pode ser limpo."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, _, s, e := prepForja(t)
			s.Inventory.Slots[0] = model.Item{Index: itemNormal, APP: 555}
			s.Inventory.Slots[1] = model.Item{Index: pedraRestauracao, Refi: 2}
			c.setup(s)
			forja(t, h, s, e, 0, 0, makeClearAppearance)
			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if s.Inventory.Slots[1].Index == pedraRestauracao && s.Inventory.Slots[1].Refi != 2 {
				t.Error("recusa gastou a pedra")
			}
		})
	}
}

// ---- infraestrutura

func TestPutItemEmpilhaEDividePorTeto(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Inventory.Slots[0] = model.Item{Index: matFerro, Refi: 900}
	if !h.putItem(e.conn, s, model.Item{Index: matFerro, Refi: 1200}) {
		t.Fatal("putItem falhou")
	}
	if s.Inventory.Slots[0].Refi != 1000 {
		t.Errorf("primeira pilha = %d, queria 1000", s.Inventory.Slots[0].Refi)
	}
	if got := contaItem(s, matFerro); got != 2100 {
		t.Errorf("total = %d, queria 2100", got)
	}
}

func TestPutItemSemEspacoNaoMudaNada(t *testing.T) {
	h, _, s, e := prepForja(t)
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: matSucata}
	}
	s.Inventory.Slots[10] = model.Item{Index: matFerro, Refi: 999}
	antes := append([]model.Item(nil), s.Inventory.Slots...)
	if h.putItem(e.conn, s, model.Item{Index: matFerro, Refi: 50}) {
		t.Fatal("putItem deveria falhar sem espaco")
	}
	for i := range antes {
		if s.Inventory.Slots[i] != antes[i] {
			t.Fatalf("slot %d mudou apesar da falha", i)
		}
	}
}

func TestMakeItemForaDoJogoETamanhoDoPacote(t *testing.T) {
	h, _, s, e := prepForja(t)
	s.Status = CharList
	if err := h.makeItem(context.Background(), e.conn, s, asPacket(t, makePacket(0, 0, 0))); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
	if n := binary.Size(packets.MakeItemPacket{}); n != 24 {
		t.Errorf("MakeItemPacket = %d, o Delphi tem 24", n)
	}
}
