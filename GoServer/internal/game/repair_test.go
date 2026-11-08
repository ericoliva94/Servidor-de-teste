package game

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

const (
	itemReparavel = 800 // SellPrince 100000: 25 de ouro por ponto
	itemRankTres  = 801 // Rank 3: multiplica por 2
	itemBarato    = 802 // SellPrince 10000: 2,5 por ponto
)

func prepReparo(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(itemReparavel, gamedata.ItemFromList{ItemType: 3, SellPrince: 100000})
	tab.poeItem(itemRankTres, gamedata.ItemFromList{ItemType: 3, SellPrince: 100000, Rank: 3})
	tab.poeItem(itemBarato, gamedata.ItemFromList{ItemType: 3, SellPrince: 10000})
	e := novoEspiao(t)
	s := sessaoComInventario()
	s.Avatar.Gold = 100_000
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, s, e
}

func repairPacket(modo uint32, slots, tipos []uint8) []byte {
	p := packets.PacketRepareItens{Unk: modo}
	for i := range p.ItensToRepareSlotType {
		p.ItensToRepareSlotType[i] = repairSlotNone
	}
	copy(p.ItensToRepareSlot[:], slots)
	copy(p.ItensToRepareSlotType[:], tipos)
	buf, err := netsrv.Marshal(CodeRepairItems, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func repara(t *testing.T, h *Handler, s *Session, e *espiao, modo uint32, slots, tipos []uint8) {
	t.Helper()
	if err := h.repairItems(context.Background(), e.conn, s,
		asPacket(t, repairPacket(modo, slots, tipos))); err != nil {
		t.Fatal(err)
	}
}

func TestReparoSelecionadoCobraEReparaInventarioEEquipamento(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[3] = model.Item{Index: itemReparavel, Min: 60, Max: 100} // 40 * 25 = 1000
	s.Equip.Slots[3] = model.Item{Index: itemReparavel, Min: 90, Max: 100}     // 10 * 25 = 250

	repara(t, h, s, e, repairSelected, []uint8{3, 3}, []uint8{repairSlotInv, repairSlotEquip})

	if s.Inventory.Slots[3].Min != 100 || s.Equip.Slots[3].Min != 100 {
		t.Errorf("Min = %d e %d, queria 100", s.Inventory.Slots[3].Min, s.Equip.Slots[3].Min)
	}
	if s.Avatar.Gold != 100_000-1250 {
		t.Errorf("ouro = %d, queria %d", s.Avatar.Gold, 100_000-1250)
	}
	proximoDe(t, e, CodeRefreshMoney)
	proximoDe(t, e, CodeRefreshItem)
	proximoDe(t, e, CodeRefreshItem)
	eco := decodifica[packets.PacketRepareItens](t, proximoDe(t, e, CodeRepairItems))
	if eco.Header.Index != s.ClientID {
		t.Errorf("eco com Index %d", eco.Header.Index)
	}
}

func TestReparoRankMultiplicaPorRankMenos1(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[0] = model.Item{Index: itemRankTres, Min: 80, Max: 100} // 20*25*2 = 1000
	repara(t, h, s, e, repairSelected, []uint8{0}, []uint8{repairSlotInv})
	if got := 100_000 - s.Avatar.Gold; got != 1000 {
		t.Errorf("custo = %d, queria 1000", got)
	}
}

// O total e arredondado para o par (Round do Delphi): 2,5 -> 2 e 7,5 -> 8.
func TestReparoArredondaParaOPar(t *testing.T) {
	for _, c := range []struct{ perdido, want int }{{1, 2}, {3, 8}} {
		h, s, e := prepReparo(t)
		s.Inventory.Slots[0] = model.Item{Index: itemBarato, Min: uint8(100 - c.perdido), Max: 100}
		repara(t, h, s, e, repairSelected, []uint8{0}, []uint8{repairSlotInv})
		if got := int(100_000 - s.Avatar.Gold); got != c.want {
			t.Errorf("perdido %d: custo %d, queria %d", c.perdido, got, c.want)
		}
	}
}

func TestReparoSemOuroNaoMudaNada(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Avatar.Gold = 999
	s.Inventory.Slots[3] = model.Item{Index: itemReparavel, Min: 60, Max: 100}
	repara(t, h, s, e, repairSelected, []uint8{3}, []uint8{repairSlotInv})
	if got := mensagemDe(t, e); got != "Voce necessita de 1000 de gold para realizar essa acao." {
		t.Errorf("mensagem = %q", got)
	}
	if s.Inventory.Slots[3].Min != 60 || s.Avatar.Gold != 999 {
		t.Error("reparo sem ouro alterou algo")
	}
}

// Defeito da origem: Min > Max dava durabilidade perdida negativa, o total
// ficava negativo e DecGold pagava o jogador.
func TestReparoDeItemComMinMaiorQueMaxNaoPagaOJogador(t *testing.T) {
	for _, modo := range []uint32{repairSelected, repairAll} {
		h, s, e := prepReparo(t)
		s.Inventory.Slots[0] = model.Item{Index: itemReparavel, Min: 200, Max: 10}
		s.Equip.Slots[3] = model.Item{Index: itemReparavel, Min: 60, Max: 100}
		repara(t, h, s, e, modo, []uint8{0, 3}, []uint8{repairSlotInv, repairSlotEquip})
		if s.Avatar.Gold > 100_000-1000 {
			t.Errorf("modo %d: ouro %d, o jogador nao pode ganhar com o reparo", modo, s.Avatar.Gold)
		}
		if s.Inventory.Slots[0].Min != 200 {
			t.Errorf("modo %d: item com Min > Max foi alterado", modo)
		}
	}
}

func TestReparoComSlotRepetidoCobraUmaVez(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[3] = model.Item{Index: itemReparavel, Min: 60, Max: 100}
	repara(t, h, s, e, repairSelected, []uint8{3, 3, 3}, []uint8{1, 1, 1})
	if got := 100_000 - s.Avatar.Gold; got != 1000 {
		t.Errorf("custo = %d, queria 1000 (uma vez)", got)
	}
}

func TestReparoIgnoraSlotsInvalidos(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[20] = model.Item{Index: itemReparavel, Min: 60, Max: 100}
	s.Inventory.Slots[61] = model.Item{} // sem bolsa para o slot 20
	slots := []uint8{250, 64, 20, 200, 16}
	tipos := []uint8{1, 1, 1, 0, 0}
	repara(t, h, s, e, repairSelected, slots, tipos)
	if s.Inventory.Slots[20].Min != 60 || s.Avatar.Gold != 100_000 {
		t.Error("slot invalido ou sem bolsa foi reparado")
	}
}

func TestReparoDeTudoPegaInventarioEEquipamento2a7(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[0] = model.Item{Index: itemReparavel, Min: 90, Max: 100}  // 250
	s.Inventory.Slots[59] = model.Item{Index: itemReparavel, Min: 90, Max: 100} // 250
	s.Equip.Slots[2] = model.Item{Index: itemReparavel, Min: 90, Max: 100}      // 250
	s.Equip.Slots[7] = model.Item{Index: itemReparavel, Min: 90, Max: 100}      // 250
	s.Equip.Slots[8] = model.Item{Index: itemReparavel, Min: 90, Max: 100}      // fora do modo
	s.Inventory.Slots[1] = model.Item{Index: itemReparavel, Min: 100, Max: 100} // inteiro
	s.Inventory.Slots[2] = model.Item{Index: itemReparavel}                     // Max 0

	repara(t, h, s, e, repairAll, nil, nil)

	if got := 100_000 - s.Avatar.Gold; got != 1000 {
		t.Errorf("custo = %d, queria 1000", got)
	}
	if s.Equip.Slots[8].Min != 90 {
		t.Error("equipamento fora de 2..7 foi reparado")
	}
	for _, it := range []model.Item{s.Inventory.Slots[0], s.Inventory.Slots[59], s.Equip.Slots[2], s.Equip.Slots[7]} {
		if it.Min != 100 {
			t.Errorf("item nao reparado: %+v", it)
		}
	}
}

func TestReparoSemNadaParaRepararNaoCobra(t *testing.T) {
	h, s, e := prepReparo(t)
	repara(t, h, s, e, repairAll, nil, nil)
	if s.Avatar.Gold != 100_000 {
		t.Errorf("ouro = %d", s.Avatar.Gold)
	}
}

func TestReparoModoDesconhecidoNaoFazNada(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Inventory.Slots[3] = model.Item{Index: itemReparavel, Min: 60, Max: 100}
	repara(t, h, s, e, 7, []uint8{3}, []uint8{1})
	if s.Inventory.Slots[3].Min != 60 {
		t.Error("modo desconhecido reparou")
	}
	e.nada(t, 20*time.Millisecond)
}

func TestReparoForaDoJogoETamanhoDoPacote(t *testing.T) {
	h, s, e := prepReparo(t)
	s.Status = CharList
	if err := h.repairItems(context.Background(), e.conn, s,
		asPacket(t, repairPacket(0, nil, nil))); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
	if n := binary.Size(packets.PacketRepareItens{}); n != 36 {
		t.Errorf("PacketRepareItens = %d, o Delphi tem 36", n)
	}
}
