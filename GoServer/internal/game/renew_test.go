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
	itemReparadorPran = 600
	itemLicencaMonta  = 601
	itemRoupaPran     = 602
	itemMontaria      = 603
	itemOutraCoisa    = 604
)

func prepRenew(t *testing.T) (*Handler, *Session, *espiao) {
	t.Helper()
	h, tab := handlerComItens(t)
	tab.poeItem(itemReparadorPran, gamedata.ItemFromList{CanAgroup: 1, ItemType: 35, UseEffect: 720})
	tab.poeItem(itemLicencaMonta, gamedata.ItemFromList{CanAgroup: 1, ItemType: 520, UseEffect: 240})
	tab.poeItem(itemRoupaPran, gamedata.ItemFromList{ItemType: 20, Classe: 101})
	tab.poeItem(itemMontaria, gamedata.ItemFromList{ItemType: 9})
	tab.poeItem(itemOutraCoisa, gamedata.ItemFromList{ItemType: 3, Classe: 1})
	e := novoEspiao(t)
	s := sessaoComInventario()
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, s, e
}

func renew(t *testing.T, h *Handler, s *Session, e *espiao, code uint16, renewer, alvo uint32) {
	t.Helper()
	buf, err := netsrv.Marshal(code, packets.PacketRenoveItem{SlotItem1: renewer, SlotItemToRenove: alvo})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.renewItem(context.Background(), e.conn, s, asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
}

func TestRenovarRoupaDePranGravaPrazoZeraDurabilidadeEConsome(t *testing.T) {
	for _, code := range []uint16{CodeRenewItem, CodeRenewItemPran} {
		h, s, e := prepRenew(t)
		s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 3}
		s.Inventory.Slots[5] = model.Item{Index: itemRoupaPran, Min: 9, Max: 9}

		renew(t, h, s, e, code, 2, 5)

		alvo := s.Inventory.Slots[5]
		if alvo.Min != 0 || alvo.Max != 0 {
			t.Errorf("durabilidade = %d/%d, queria 0/0", alvo.Min, alvo.Max)
		}
		want := h.now().Add(720 * time.Hour).Truncate(time.Second)
		got := alvo.Expire(20, baseDatetime)
		if d := want.Sub(got); d < 0 || d > 256*time.Second { // passo de 256 s do formato
			t.Errorf("prazo = %v, queria ~%v", got, want)
		}
		if s.Inventory.Slots[2].Refi != 2 {
			t.Errorf("renovador = %d, queria 2", s.Inventory.Slots[2].Refi)
		}
		if got := mensagemDe(t, e); !strings.Contains(got, "por mais 30 dias") {
			t.Errorf("mensagem = %q", got)
		}
	}
}

func TestRenovarMontariaGuardaDias(t *testing.T) {
	h, s, e := prepRenew(t)
	s.Inventory.Slots[0] = model.Item{Index: itemLicencaMonta, Refi: 1} // slot 0 vale
	s.Inventory.Slots[1] = model.Item{Index: itemMontaria}

	renew(t, h, s, e, CodeRenewItem, 0, 1)

	wantDias := uint16(h.now().Add(240*time.Hour).Sub(baseDatetime).Hours() / 24)
	if got := s.Inventory.Slots[1].Time; got != wantDias {
		t.Errorf("dias = %d, queria %d", got, wantDias)
	}
	if !s.Inventory.Slots[0].IsEmpty() {
		t.Error("licenca de uma unidade deveria ter sido consumida")
	}
	proximoDe(t, e, CodeRenewItem) // eco
}

func TestRenovarAlvoErradoNaoConsome(t *testing.T) {
	cases := map[string]model.Item{
		"item comum":       {Index: itemOutraCoisa},
		"slot vazio":       {},
		"indice na tabela": {Index: 60000},
	}
	for nome, alvo := range cases {
		t.Run(nome, func(t *testing.T) {
			h, s, e := prepRenew(t)
			s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 3}
			s.Inventory.Slots[5] = alvo
			renew(t, h, s, e, CodeRenewItem, 2, 5)
			if s.Inventory.Slots[2].Refi != 3 {
				t.Error("consumiu o renovador sem renovar")
			}
			e.nada(t, 20*time.Millisecond)
		})
	}
}

func TestRenovarMontariaComRenovadorDePranNaoServe(t *testing.T) {
	h, s, e := prepRenew(t)
	s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 1}
	s.Inventory.Slots[5] = model.Item{Index: itemMontaria}
	renew(t, h, s, e, CodeRenewItem, 2, 5)
	if s.Inventory.Slots[2].IsEmpty() {
		t.Error("renovador consumido por alvo que nao e roupa de pran")
	}
}

func TestRenovarSlotsInvalidos(t *testing.T) {
	h, s, e := prepRenew(t)
	s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 3}
	s.Inventory.Slots[5] = model.Item{Index: itemRoupaPran}
	for _, par := range [][2]uint32{{2, 2}, {2, 60}, {2, 63}, {2, 999}, {0xFFFFFFFF, 5}, {70, 5}} {
		renew(t, h, s, e, CodeRenewItem, par[0], par[1])
	}
	if s.Inventory.Slots[2].Refi != 3 {
		t.Error("pedido invalido consumiu o renovador")
	}
	e.nada(t, 20*time.Millisecond)
}

func TestRenovarSlotSemBolsaERecusado(t *testing.T) {
	h, s, e := prepRenew(t)
	s.Inventory.Slots[61] = model.Item{}
	s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 3}
	s.Inventory.Slots[20] = model.Item{Index: itemRoupaPran}
	renew(t, h, s, e, CodeRenewItem, 2, 20)
	if s.Inventory.Slots[2].Refi != 3 {
		t.Error("usou slot cuja bolsa nao esta no inventario")
	}
}

// UseEffect zero dava prazo "agora": o item expirava e a licenca era gasta.
func TestRenovarComUseEffectZeroERecusado(t *testing.T) {
	h, s, e := prepRenew(t)
	h.Data.(*tabelaDeItens).poeItem(itemReparadorPran, gamedata.ItemFromList{CanAgroup: 1, ItemType: 35})
	s.Inventory.Slots[2] = model.Item{Index: itemReparadorPran, Refi: 3}
	s.Inventory.Slots[5] = model.Item{Index: itemRoupaPran}
	renew(t, h, s, e, CodeRenewItem, 2, 5)
	if s.Inventory.Slots[2].Refi != 3 {
		t.Error("consumiu renovador sem prazo")
	}
}

func TestRenovarForaDoJogo(t *testing.T) {
	h, s, e := prepRenew(t)
	s.Status = CharList
	buf, _ := netsrv.Marshal(CodeRenewItem, packets.PacketRenoveItem{})
	if err := h.renewItem(context.Background(), e.conn, s, asPacket(t, buf)); err != ErrNotPlaying {
		t.Errorf("erro = %v", err)
	}
}

func TestTamanhoDoPacoteDeRenovar(t *testing.T) {
	if n := binary.Size(packets.PacketRenoveItem{}); n != 20 {
		t.Errorf("PacketRenoveItem = %d, o Delphi tem 20", n)
	}
}
