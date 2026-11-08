package game

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

const (
	itemArmadura  = 700 // tipo 3 (peitoral), UseEffect 1, empilhavel nao
	itemArma      = 701 // tipo 6
	itemHira      = 702 // material, empilha
	itemExtratoN  = 703 // tipo 63, empilha
	itemExtratoE  = 704 // tipo 64, empilha
	itemAparencia = 705 // equipamento da mesma classe
	itemAthlon    = athlonItem
	itemBloqueado = 706
	itemRankAlto  = 707
)

// tabelaComCusto acrescenta a consulta de custo de refino.
type tabelaComCusto struct {
	*tabelaDeItens
	custoArmadura, custoArma uint32
	semLinha                 bool
}

func (t *tabelaComCusto) ReinforceCost(weapon bool, useEffect int) (uint32, bool) {
	if t.semLinha {
		return 0, false
	}
	if weapon {
		return t.custoArma, true
	}
	return t.custoArmadura, true
}

// sorteioFixo devolve sempre o mesmo valor (limitado ao intervalo).
type sorteioFixo int

func (f sorteioFixo) IntN(n int) int {
	if int(f) >= n {
		return n - 1
	}
	return int(f)
}

func prepRefino(t *testing.T) (*Handler, *tabelaComCusto, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tc := &tabelaComCusto{tabelaDeItens: tab, custoArmadura: 1000, custoArma: 2000}
	h.Data = tc
	tab.poeItem(itemArmadura, gamedata.ItemFromList{ItemType: 3, UseEffect: 1, Classe: 1})
	tab.poeItem(itemArma, gamedata.ItemFromList{ItemType: 6, UseEffect: 1, Classe: 1})
	tab.poeItem(itemHira, gamedata.ItemFromList{CanAgroup: 1})
	tab.poeItem(itemExtratoN, gamedata.ItemFromList{CanAgroup: 1, ItemType: 63})
	tab.poeItem(itemExtratoE, gamedata.ItemFromList{CanAgroup: 1, ItemType: 64})
	tab.poeItem(itemAparencia, gamedata.ItemFromList{ItemType: 3, Classe: 2})
	tab.poeItem(itemAthlon, gamedata.ItemFromList{CanAgroup: 1})
	tab.poeItem(itemBloqueado, gamedata.ItemFromList{ItemType: 3, UseEffect: 1, Fortification: 1})
	tab.poeItem(itemRankAlto, gamedata.ItemFromList{ItemType: 3, UseEffect: 1, Rank: 3})
	tab.poeItem(300, gamedata.ItemFromList{CanAgroup: 1}) // pocao
	h.Rand = sorteioFixo(0)
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Gold = 10_000
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, tc, s, e
}

func changePacket(tipo, item, item2, item3 uint32) []byte {
	buf, err := netsrv.Marshal(CodeChangeItemAttribute,
		packets.ChangeItemAttPacket{ChangeType: tipo, ItemSlot: item, Item2: item2, Item3: item3})
	if err != nil {
		panic(err)
	}
	return buf
}

func change(t *testing.T, h *Handler, s *Session, e *espiao, tipo, item, item2, item3 uint32) {
	t.Helper()
	if err := h.changeItemAttribute(context.Background(), e.conn, s,
		asPacket(t, changePacket(tipo, item, item2, item3))); err != nil {
		t.Fatal(err)
	}
}

func resposta(t *testing.T, e *espiao) packets.ReinforceResponse {
	t.Helper()
	return decodifica[packets.ReinforceResponse](t, proximoDe(t, e, CodeChangeItemResponse))
}

func refina(t *testing.T, h *Handler, s *Session, e *espiao, item, hira, extrato uint32) {
	t.Helper()
	change(t, h, s, e, ChangeReinforce, item, hira, extrato)
}

// ---- refino

func TestRefinoComSucessoSobeUmNivelCobraEConsome(t *testing.T) {
	h, _, s, e := prepRefino(t)
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 32} // +2
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 5}

	refina(t, h, s, e, 0, 1, noSlot)

	if got := s.Inventory.Slots[0].Refi; got != 48 {
		t.Errorf("Refi = %d, queria 48 (+3)", got)
	}
	if got := s.Inventory.Slots[1].Refi; got != 4 {
		t.Errorf("material = %d, queria 4", got)
	}
	if got := s.Avatar.Gold; got != 9000 {
		t.Errorf("ouro = %d, queria 9000", got)
	}
	r := resposta(t, e)
	if r.ReinforceResult != refineSuccess || r.Unk1 != ChangeReinforce {
		t.Errorf("resposta = %+v", r)
	}
}

func TestRefinoDeArmaCobraDaTabelaDeArmas(t *testing.T) {
	h, _, s, e := prepRefino(t)
	s.Inventory.Slots[0] = model.Item{Index: itemArma}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 1}
	refina(t, h, s, e, 0, 1, noSlot)
	if got := s.Avatar.Gold; got != 8000 {
		t.Errorf("ouro = %d, queria 8000", got)
	}
	if !s.Inventory.Slots[1].IsEmpty() {
		t.Error("material de uma unidade deveria ter acabado")
	}
}

func TestRefinoFalhaSemExtratoDe7EmDiante_Quebra(t *testing.T) {
	h, _, s, e := prepRefino(t)
	h.Rand = sorteioFixo(99) // rolagem 100: falha em qualquer chance < 100
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 7 * 16}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 2}

	refina(t, h, s, e, 0, 1, noSlot)

	if !s.Inventory.Slots[0].IsEmpty() {
		t.Error("o item deveria ter quebrado")
	}
	if r := resposta(t, e); r.ReinforceResult != refineBreak {
		t.Errorf("resultado = %d", r.ReinforceResult)
	}
}

func TestRefinoFalhaCom4a6SoPerdeNivel(t *testing.T) {
	h, _, s, e := prepRefino(t)
	h.Rand = sorteioFixo(99)
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 4 * 16}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 2}
	refina(t, h, s, e, 0, 1, noSlot)
	if got := s.Inventory.Slots[0].Refi; got != 3*16 {
		t.Errorf("Refi = %d, queria 48", got)
	}
}

func TestExtratoNormalEvitaQuebra_EnriquecidoNaoMudaNada(t *testing.T) {
	h, _, s, e := prepRefino(t)
	h.Rand = sorteioFixo(99)

	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 7 * 16}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 2}
	s.Inventory.Slots[2] = model.Item{Index: itemExtratoN, Refi: 2}
	refina(t, h, s, e, 0, 1, 2)
	if got := s.Inventory.Slots[0].Refi; got != 6*16 {
		t.Errorf("normal: Refi = %d, queria 96", got)
	}
	if s.Inventory.Slots[2].Refi != 1 {
		t.Error("extrato nao foi consumido")
	}

	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 7 * 16}
	s.Inventory.Slots[2] = model.Item{Index: itemExtratoE, Refi: 2}
	e.drena(20 * time.Millisecond)
	refina(t, h, s, e, 0, 1, 2)
	if got := s.Inventory.Slots[0].Refi; got != 7*16 {
		t.Errorf("enriquecido: Refi = %d, queria inalterado", got)
	}
	if r := resposta(t, e); r.ReinforceResult != refineBlocked {
		t.Errorf("resultado = %d, queria %d", r.ReinforceResult, refineBlocked)
	}
}

func TestSorteioMultiplicadoPeloRank(t *testing.T) {
	// +4 -> chance 70. Rolagem 40 com rank 2 vira 80: falha. Rolagem 30 vira 60: passa.
	if got := refineOutcome(sorteioFixo(39), 2, 0, false, 4); got != refineDown {
		t.Errorf("rank 2, rolagem 40 = %d, queria falha", got)
	}
	if got := refineOutcome(sorteioFixo(29), 2, 0, false, 4); got != refineSuccess {
		t.Errorf("rank 2, rolagem 30 = %d, queria sucesso", got)
	}
}

func TestTabelaDeChancesNosExtremos(t *testing.T) {
	if got := refineOutcome(sorteioFixo(0), 0, 0, false, 0); got != refineSuccess {
		t.Errorf("+0 = %d", got)
	}
	// Acima de +16 a chance e a ultima (0) e o resultado e perder nivel.
	if got := refineOutcome(sorteioFixo(0), 0, 0, false, 30); got != refineDown {
		t.Errorf("+30 = %d", got)
	}
}

// Defeito da origem: nada exigia que o alvo fosse equipamento. Uma pocao
// ganhava 16 unidades por refino com sucesso.
func TestRefinoDePilhaNaoDuplicaItem(t *testing.T) {
	h, _, s, e := prepRefino(t)
	s.Inventory.Slots[0] = model.Item{Index: 300, Refi: 10}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 3}
	refina(t, h, s, e, 0, 1, noSlot)
	if s.Inventory.Slots[0].Refi != 10 || s.Inventory.Slots[1].Refi != 3 || s.Avatar.Gold != 10_000 {
		t.Error("refino de empilhavel alterou algo")
	}
}

func TestRefinoRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(s *Session)
		args  [3]uint32
		msg   string
	}{
		{"mesmo slot", func(s *Session) {}, [3]uint32{0, 0, noSlot}, msgWrong},
		{"material e extrato no mesmo slot", func(s *Session) {}, [3]uint32{0, 1, 1}, msgWrong},
		{"slot fora da faixa", func(s *Session) {}, [3]uint32{0, 999, noSlot}, msgWrong},
		{"slot enorme", func(s *Session) {}, [3]uint32{0xFFFFFFFE, 1, noSlot}, msgWrong},
		{"slot de bolsa", func(s *Session) {}, [3]uint32{0, 63, noSlot}, msgWrong},
		{"material vazio", func(s *Session) { s.Inventory.Slots[1] = model.Item{} }, [3]uint32{0, 1, noSlot}, msgWrong},
		{"material que nao empilha", func(s *Session) { s.Inventory.Slots[1] = model.Item{Index: itemArma, Refi: 4} }, [3]uint32{0, 1, noSlot}, msgWrong},
		{"material de rank menor", func(s *Session) { s.Inventory.Slots[0] = model.Item{Index: itemRankAlto} }, [3]uint32{0, 1, noSlot}, msgWrong},
		{"sem ouro", func(s *Session) { s.Avatar.Gold = 999 }, [3]uint32{0, 1, noSlot}, "Voce nao possui gold suficiente."},
		{"nivel maximo", func(s *Session) { s.Inventory.Slots[0].Refi = 175 }, [3]uint32{0, 1, noSlot}, "Refinacao maxima permitida: +11"},
		{"extrato com quantidade zero", func(s *Session) { s.Inventory.Slots[2] = model.Item{Index: itemExtratoN} }, [3]uint32{0, 1, 2}, msgWrong},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, _, s, e := prepRefino(t)
			s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 32}
			s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 5}
			c.setup(s)
			ouro := s.Avatar.Gold
			antes := s.Inventory.Slots[1]

			refina(t, h, s, e, c.args[0], c.args[1], c.args[2])

			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if s.Avatar.Gold != ouro || s.Inventory.Slots[1] != antes {
				t.Error("pedido recusado consumiu algo")
			}
		})
	}
}

func TestRefinoDeItemBloqueadoNaoConsome(t *testing.T) {
	h, _, s, e := prepRefino(t)
	h.Data.(*tabelaComCusto).tabelaDeItens.poeItem(itemBloqueado, gamedata.ItemFromList{ItemType: 3, UseEffect: 1, Fortification: 1})
	s.Inventory.Slots[0] = model.Item{Index: itemBloqueado}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 3}
	refina(t, h, s, e, 0, 1, noSlot)
	if s.Inventory.Slots[1].Refi != 3 || s.Avatar.Gold != 10_000 {
		t.Error("consumiu em item que nao pode ser refinado")
	}
}

// Linha ausente na tabela de custos valia zero na origem: refino de graca.
func TestRefinoSemLinhaDeCustoERecusado(t *testing.T) {
	h, tc, s, e := prepRefino(t)
	tc.semLinha = true
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 3}
	refina(t, h, s, e, 0, 1, noSlot)
	if s.Inventory.Slots[0].Refi != 0 || s.Inventory.Slots[1].Refi != 3 {
		t.Error("refinou sem linha de custo")
	}
}

func TestRefinoAnunciaDe9EmDiante(t *testing.T) {
	h, _, s, e := prepRefino(t)
	s.Avatar.Name = "Alfa"
	h.Chan.RegisterCharacter(s.ClientID, "Alfa")
	outro, oe := jogador(t, h, novoFakeFriends(), "Beta", 2)
	_ = outro
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura, Refi: 8 * 16}
	s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 3}

	refina(t, h, s, e, 0, 1, noSlot)

	if s.Inventory.Slots[0].Refi != 9*16 {
		t.Fatalf("Refi = %d", s.Inventory.Slots[0].Refi)
	}
	if got := mensagemDe(t, oe); !strings.Contains(got, "Alfa refinou com sucesso") || !strings.HasSuffix(got, "+9") {
		t.Errorf("anuncio = %q", got)
	}
}

// ---- troca de aparencia

func prepAparencia(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, _, s, e := prepRefino(t)
	s.Inventory.Slots[0] = model.Item{Index: itemArmadura}
	s.Inventory.Slots[1] = model.Item{Index: itemAparencia}
	s.Inventory.Slots[2] = model.Item{Index: itemAthlon, Refi: 2}
	return h, s, e
}

func TestTrocarAparenciaAplicaCobraEConsome(t *testing.T) {
	h, s, e := prepAparencia(t)
	change(t, h, s, e, ChangeApp, 0, 1, noSlot)

	if got := s.Inventory.Slots[0].APP; got != itemAparencia {
		t.Errorf("APP = %d, queria %d", got, itemAparencia)
	}
	if !s.Inventory.Slots[1].IsEmpty() {
		t.Error("item de aparencia deveria ter sido consumido")
	}
	if s.Inventory.Slots[2].Refi != 1 {
		t.Errorf("Athlon = %d, queria 1", s.Inventory.Slots[2].Refi)
	}
	if s.Avatar.Gold != 9500 {
		t.Errorf("ouro = %d, queria 9500", s.Avatar.Gold)
	}
	if r := resposta(t, e); r.ReinforceResult != 2 || r.Unk1 != ChangeApp {
		t.Errorf("resposta = %+v", r)
	}
}

// Defeito da origem: sem 500 de ouro o jogador perdia o Athlon e o item de
// aparencia e ficava com a aparencia mesmo assim.
func TestTrocarAparenciaSemOuroNaoMudaNada(t *testing.T) {
	h, s, e := prepAparencia(t)
	s.Avatar.Gold = 499
	change(t, h, s, e, ChangeApp, 0, 1, noSlot)

	if got := mensagemDe(t, e); got != "Voce nao tem o gold necessario!" {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[0].APP != 0 || s.Inventory.Slots[1].IsEmpty() || s.Inventory.Slots[2].Refi != 2 {
		t.Error("troca sem ouro alterou itens")
	}
}

func TestTrocarAparenciaRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(s *Session)
		args  [2]uint32
		msg   string
	}{
		{"sem Athlon", func(s *Session) { s.Inventory.Slots[2] = model.Item{} }, [2]uint32{0, 1}, "Voce nao possui Athlon!"},
		{"classe diferente", func(s *Session) { s.Inventory.Slots[1] = model.Item{Index: itemArma} }, [2]uint32{0, 1}, ""},
		{"ja tem aparencia", func(s *Session) { s.Inventory.Slots[0].APP = 999 }, [2]uint32{0, 1}, msgWrong},
		{"mesmo slot", func(s *Session) {}, [2]uint32{0, 0}, msgWrong},
		{"fora da faixa", func(s *Session) {}, [2]uint32{0, 999}, msgWrong},
		{"item de aparencia vazio", func(s *Session) { s.Inventory.Slots[1] = model.Item{} }, [2]uint32{0, 1}, msgWrong},
		{"empilhavel como aparencia", func(s *Session) { s.Inventory.Slots[1] = model.Item{Index: 300, Refi: 2} }, [2]uint32{0, 1}, "Este item nao pode ser colocado como aparencia!"},
		{"athlon e o proprio item", func(s *Session) {}, [2]uint32{2, 1}, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, s, e := prepAparencia(t)
			// Classe diferente: o item de aparencia e de outra classe (arqueiro).
			h.Data.(*tabelaComCusto).tabelaDeItens.poeItem(itemArma, gamedata.ItemFromList{ItemType: 6, UseEffect: 1, Classe: 21})
			c.setup(s)
			ouro := s.Avatar.Gold
			change(t, h, s, e, ChangeApp, c.args[0], c.args[1], noSlot)
			if c.msg != "" {
				if got := mensagemDe(t, e); got != c.msg {
					t.Errorf("mensagem = %q, queria %q", got, c.msg)
				}
			}
			if s.Avatar.Gold != ouro {
				t.Error("recusa cobrou ouro")
			}
			if s.Inventory.Slots[2].Index == itemAthlon && s.Inventory.Slots[2].Refi != 2 {
				t.Error("recusa gastou Athlon")
			}
		})
	}
}

func TestEncantamentoAindaNaoPortadoNaoConsome(t *testing.T) {
	for _, tipo := range []uint32{ChangeEnchants, ChangeMountEnchants, ChangePranEnchants} {
		h, _, s, e := prepRefino(t)
		s.Inventory.Slots[0] = model.Item{Index: itemArmadura}
		s.Inventory.Slots[1] = model.Item{Index: itemHira, Refi: 3}
		change(t, h, s, e, tipo, 0, 1, noSlot)
		if got := mensagemDe(t, e); !strings.Contains(got, "Encantamento ainda nao") {
			t.Errorf("tipo %d mensagem = %q", tipo, got)
		}
		if s.Inventory.Slots[1].Refi != 3 || s.Avatar.Gold != 10_000 {
			t.Errorf("tipo %d consumiu algo", tipo)
		}
	}
}

// ---- mensagem ao cliente e pacotes

// O Delphi envia a mensagem com codigo $984 e Header.size 144.
func TestMensagemAoClienteTemCodigoETamanhoDaOrigem(t *testing.T) {
	h := testHandler()
	e := novoEspiao(t)
	h.message(e.conn, "ola")
	p := e.proximo(t)
	if p.code != 0x984 {
		t.Errorf("codigo = %#x, queria 0x984", p.code)
	}
	if len(p.corpo) != 144 {
		t.Errorf("tamanho = %d, queria 144", len(p.corpo))
	}
	m := decodifica[clientMessage](t, p)
	if m.Type1 != 16 || netsrv.CString(m.Msg[:]) != "ola" {
		t.Errorf("mensagem = tipo %d %q", m.Type1, netsrv.CString(m.Msg[:]))
	}
}

func TestMensagemLongaCabeComTerminador(t *testing.T) {
	h := testHandler()
	e := novoEspiao(t)
	h.message(e.conn, strings.Repeat("x", 500))
	m := decodifica[clientMessage](t, e.proximo(t))
	if n := len(netsrv.CString(m.Msg[:])); n != 127 {
		t.Errorf("texto com %d caracteres, queria 127", n)
	}
	if m.Msg[127] != 0 {
		t.Error("sem terminador")
	}
}

func TestTamanhosDosPacotesDeAlteracao(t *testing.T) {
	if n := binary.Size(packets.ChangeItemAttPacket{}); n != 28 {
		t.Errorf("ChangeItemAttPacket = %d, o Delphi tem 28", n)
	}
	if n := binary.Size(packets.ReinforceResponse{}); n != 20 {
		t.Errorf("ReinforceResponse = %d, o Delphi tem 20", n)
	}
	if n := binary.Size(clientMessage{}); n != 144 {
		t.Errorf("clientMessage = %d, o Delphi envia 144", n)
	}
}

func TestAlteracaoForaDoJogo(t *testing.T) {
	h, _, s, e := prepRefino(t)
	s.Status = CharList
	err := h.changeItemAttribute(context.Background(), e.conn, s, asPacket(t, changePacket(0, 0, 1, noSlot)))
	if err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
}
