package game

import (
	"context"
	"testing"

	"aika/internal/model"
	"aika/internal/packets"
)

type fakeNacao struct {
	escolhas []uint8
	jaTem    bool
}

func (f *fakeNacao) Choose(_ context.Context, _ uint32, n uint8) (bool, error) {
	if f.jaTem {
		return false, nil
	}
	f.escolhas = append(f.escolhas, n)
	f.jaTem = true
	return true, nil
}

func cenarioItemNacao(t *testing.T, nacaoDoCanal uint8) (*Handler, *espiao, *Session, *fakeNacao) {
	t.Helper()
	h, tab, _ := handlerParaUso(t)
	f := &fakeNacao{}
	h.Nation, h.AccountNation = nacaoDoCanal, f
	h.TitleTable = tabelaDeTitulosComMeta(t, titleNationChosen, 1, 1, "Cidadao")
	poeConsumivel(tab, 9000, ItemTypeSetNation, itemEffectSetNation, "Escolha de Nacao")
	poeConsumivel(tab, 9001, ItemTypeSetNation, 5, "Outro efeito")
	poeConsumivel(tab, 10766, 0, 0, "Closed Beta")
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	s.Account.ID, s.Account.Nation = 7, 0
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	return h, e, s, f
}

func TestItemDeNacaoUsaANacaoDoCanal(t *testing.T) {
	for _, nacao := range []uint8{1, 2, 3} {
		h, e, s, f := cenarioItemNacao(t, nacao)
		s.Inventory.Slots[0] = model.Item{Index: 9000, Refi: 1}
		usa(t, h, e, s, 0)
		if len(f.escolhas) != 1 || f.escolhas[0] != nacao || s.Account.Nation != nacao || s.Avatar.Nation != uint16(nacao) {
			t.Errorf("canal da nacao %d: escolhas=%v conta=%d avatar=%d", nacao, f.escolhas, s.Account.Nation, s.Avatar.Nation)
		}
		if !s.Inventory.Get(0).IsEmpty() {
			t.Error("o item deveria ser gasto")
		}
		info := decodifica[packets.RefreshPlayerInfosPacket](t, proximoDe(t, e, CodeRefreshPlayerInfos))
		if info.Nation != uint16(nacao) {
			t.Errorf("$130 nacao = %d", info.Nation)
		}
		if titleSlot(s, titleNationChosen) < 0 {
			t.Error("deveria ganhar o titulo 18")
		}
	}
}

func TestItemDeNacaoNaoGastaQuemJaTemNacaoOuEfeitoErrado(t *testing.T) {
	h, e, s, f := cenarioItemNacao(t, 2)
	s.Account.Nation = 1
	s.Inventory.Slots[0] = model.Item{Index: 9000, Refi: 1}
	usa(t, h, e, s, 0)
	if len(f.escolhas) != 0 || s.Inventory.Get(0).IsEmpty() || s.Account.Nation != 1 {
		t.Error("quem ja tem nacao nao troca nem gasta o item")
	}
	s.Account.Nation = 0
	s.Inventory.Slots[1] = model.Item{Index: 9001, Refi: 1}
	usa(t, h, e, s, 1)
	if len(f.escolhas) != 0 || s.Inventory.Get(1).IsEmpty() {
		t.Error("efeito diferente de 99 nao escolhe nacao")
	}
	// Canal sem nacao.
	h.Nation = 0
	usa(t, h, e, s, 0)
	if len(f.escolhas) != 0 {
		t.Error("canal sem nacao nao escolhe")
	}
	// Banco recusa (outra sessao ja escolheu): nada muda.
	h.Nation = 2
	f.jaTem = true
	usa(t, h, e, s, 0)
	if s.Account.Nation != 0 || s.Inventory.Get(0).IsEmpty() {
		t.Error("escolha recusada pelo banco nao pode mudar a sessao")
	}
}

func TestItemQueDaTituloGastaEDa(t *testing.T) {
	h, e, s, _ := cenarioItemNacao(t, 1)
	h.TitleTable = tabelaDeTitulosComMeta(t, 78, 1, 1, "Beta")
	s.Inventory.Slots[0] = model.Item{Index: 10766, Refi: 1}
	usa(t, h, e, s, 0)
	if titleSlot(s, 78) < 0 || !s.Inventory.Get(0).IsEmpty() {
		t.Errorf("titulo=%d item=%+v", titleSlot(s, 78), s.Inventory.Get(0))
	}
	// Segunda vez: o titulo ja existe, o item nao e gasto.
	s.Inventory.Slots[1] = model.Item{Index: 10766, Refi: 1}
	usa(t, h, e, s, 1)
	if s.Inventory.Get(1).IsEmpty() {
		t.Error("titulo repetido nao deveria gastar o item")
	}
}
