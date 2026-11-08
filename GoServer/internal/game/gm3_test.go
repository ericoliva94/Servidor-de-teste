package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

type fakeFounders struct {
	lista    []store.Founder
	criados  []store.Founder
	produtos []int64
	validou  []uint32
	resets   []uint32
	semPran  bool
}

func (f *fakeFounders) GMIDByUsername(_ context.Context, u string) (uint32, bool, error) {
	return uint32(len(u)), u != "", nil
}

func (f *fakeFounders) FoundersByTransaction(_ context.Context, id string) ([]store.Founder, error) {
	var out []store.Founder
	for _, x := range f.lista {
		if x.TransactionID == id {
			out = append(out, x)
		}
	}
	return out, nil
}

func (f *fakeFounders) FoundersByName(_ context.Context, nome string) ([]store.Founder, error) {
	var out []store.Founder
	for _, x := range f.lista {
		if len(nome) > 0 && x.Name[:min(len(nome), len(x.Name))] == nome {
			out = append(out, x)
		}
	}
	return out, nil
}

func (f *fakeFounders) CreateFounder(_ context.Context, x store.Founder, _ uint32, prod int64, _ time.Time) (bool, error) {
	for _, y := range f.lista {
		if y.TransactionID == x.TransactionID {
			return false, nil
		}
	}
	f.lista = append(f.lista, x)
	f.criados = append(f.criados, x)
	f.produtos = append(f.produtos, prod)
	return true, nil
}

func (f *fakeFounders) ValidateFounder(_ context.Context, id, _ uint32) (bool, bool, error) {
	for i := range f.lista {
		if f.lista[i].ID == id {
			if f.lista[i].Validated {
				return true, true, nil
			}
			f.lista[i].Validated = true
			f.validou = append(f.validou, id)
			return true, false, nil
		}
	}
	return false, false, nil
}

func (f *fakeFounders) ResetPrans(_ context.Context, acc uint32) (bool, error) {
	if f.semPran {
		return false, nil
	}
	f.resets = append(f.resets, acc)
	return true, nil
}

func painel(t *testing.T, e *espiao) string {
	t.Helper()
	m := decodifica[packets.MessageToPainelPacket](t, proximoDe(t, e, CodeGMPanelMessage))
	return netsrv.CString(m.Msg[:])
}

func TestFoundersDoPainel(t *testing.T) {
	c := novoCenarioGM(t)
	ff := &fakeFounders{lista: []store.Founder{{ID: 1, TransactionID: "T1", Name: "Ana Silva", Value: 50},
		{ID: 2, TransactionID: "T2", Name: "Ana Souza", Value: 10}}}
	c.h.GMFounders = ff

	porID := func(id string) {
		p := packets.SearchComprovantByID{}
		netsrv.PutCString(p.TransactionID[:], id)
		c.manda(t, c.h.gmFounderByID, CodeGMFounderByID, p)
	}
	porNome := func(n string) {
		p := packets.SearchComprovantByName{}
		netsrv.PutCString(p.NameOfComprovant[:], n)
		c.manda(t, c.h.gmFounderByName, CodeGMFounderByName, p)
	}
	c.entra(t, "mod")
	porID("T1")
	c.eg.nada(t, 30*time.Millisecond)
	c.entra(t, "gm")
	porID("T1")
	r := decodifica[packets.ComprovantReceive](t, proximoDe(t, c.eg, CodeGMFounderRow))
	if r.ComprovantDBID != 1 || netsrv.CString(r.NameOfComprovant[:]) != "Ana Silva" || r.ValueOfComprovant != 50 {
		t.Errorf("linha = %d %q %v", r.ComprovantDBID, netsrv.CString(r.NameOfComprovant[:]), r.ValueOfComprovant)
	}
	porID("NAO")
	if got := painel(t, c.eg); got != "O comprovante em questão não foi encontrado." {
		t.Errorf("mensagem = %q", got)
	}
	porNome("Ana")
	proximoDe(t, c.eg, CodeGMFounderRow)
	proximoDe(t, c.eg, CodeGMFounderRow)

	// Validar: uma vez so.
	val := func(id int32) {
		c.manda(t, c.h.gmFounderValidate, CodeGMFounderOK, packets.ComprovantValidate{ComprovantDBID: id})
	}
	val(1)
	if len(ff.validou) != 1 {
		t.Fatal("deveria validar")
	}
	painel(t, c.eg)
	val(1)
	if got := painel(t, c.eg); got != "Solicitação já foi validada." || len(ff.validou) != 1 {
		t.Errorf("segunda validacao: %q", got)
	}
	val(99)
	if got := painel(t, c.eg); got != "Solicitação não encontrada." {
		t.Errorf("inexistente: %q", got)
	}
}

func TestCriarComprovante(t *testing.T) {
	c := novoCenarioGM(t)
	ff := &fakeFounders{}
	c.h.GMFounders = ff
	cria := func(id, nome string, valor float32, data float64) {
		p := packets.ComprovantCreate{DateOfComprovant: data, ValueOfComprovant: valor}
		netsrv.PutCString(p.TransactionID[:], id)
		netsrv.PutCString(p.NameOfComprovant[:], nome)
		c.manda(t, c.h.gmFounderCreate, CodeGMFounderCreate, p)
	}
	c.entra(t, "gm")
	cria("A", "Bia", 50, 45000)
	if len(ff.criados) != 0 {
		t.Fatal("game master nao cadastra comprovante")
	}
	c.entra(t, "admin")
	cria("A", "Bia", 50, 45000)
	if len(ff.criados) != 1 || ff.produtos[0] != 14135 || ff.criados[0].Coupon == "" {
		t.Fatalf("criados=%+v produtos=%v", ff.criados, ff.produtos)
	}
	if got := painel(t, c.eg); len(got) < 30 {
		t.Errorf("mensagem = %q", got)
	}
	cria("A", "Bia", 50, 45000) // transacao repetida
	if got := painel(t, c.eg); got != "Esse ID de transação já existe." {
		t.Errorf("repetida: %q", got)
	}
	cria("B", "Caio", 1000, 45000) // valor fora das faixas: sem cupom
	if len(ff.criados) != 2 || ff.criados[1].Coupon != "" || ff.produtos[1] != 0 {
		t.Errorf("sem faixa: %+v", ff.criados)
	}
	for _, ruim := range []struct {
		id, nome string
		v        float32
		d        float64
	}{{"", "x", 10, 1}, {"C", "", 10, 1}, {"C", "x", 0, 1}, {"C", "x", -5, 1}, {"C", "x", 10, 0}} {
		cria(ruim.id, ruim.nome, ruim.v, ruim.d)
	}
	if len(ff.criados) != 2 {
		t.Errorf("dados invalidos entraram: %d", len(ff.criados))
	}
	for v, want := range map[float32]int64{1: 14134, 30: 14134, 31: 14135, 59: 14135, 60: 14136, 228: 14136, 229: 14137, 330: 14137, 331: 0, 0.2: 0} {
		if got := founderProduct(v); got != want {
			t.Errorf("faixa %v = %d, queria %d", v, got, want)
		}
	}
}

func TestZerarPransDoPainel(t *testing.T) {
	c := novoCenarioGM(t)
	ff := &fakeFounders{}
	c.h.GMFounders = ff
	zera := func(acc int32) {
		c.manda(t, c.h.gmResetPrans, CodeGMResetPrans, packets.PacketSendDeletePrans{AccountIndex: acc})
	}
	c.entra(t, "mod")
	zera(7)
	if len(ff.resets) != 0 {
		t.Fatal("moderador nao zera prans")
	}
	c.entra(t, "gm")
	zera(0)
	zera(-3)
	zera(7)
	if len(ff.resets) != 1 || ff.resets[0] != 7 {
		t.Fatalf("resets = %v", ff.resets)
	}
	painel(t, c.eg)
	ff.semPran = true
	zera(8)
	if got := painel(t, c.eg); got != "Não existem prans no banco de dados linkados a conta." {
		t.Errorf("sem pran: %q", got)
	}
}

func TestModeradorNaoValidaComprovante(t *testing.T) {
	c := novoCenarioGM(t)
	ff := &fakeFounders{lista: []store.Founder{{ID: 1, TransactionID: "T1", Name: "Ana"}}}
	c.h.GMFounders = ff
	c.entra(t, "mod")
	c.manda(t, c.h.gmFounderValidate, CodeGMFounderOK, packets.ComprovantValidate{ComprovantDBID: 1})
	if len(ff.validou) != 0 {
		t.Error("moderador nao valida comprovante")
	}
}
