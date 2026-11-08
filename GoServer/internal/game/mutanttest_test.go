package game

import (
	"context"
	"testing"

	"aika/internal/dropdata"
	"aika/internal/gamedata"
	"aika/internal/mobdata"
)

const (
	aneis      = 1583 // ItemType 11
	brinco     = 1584 // ItemType 12
	bracelete  = 1585 // ItemType 13
	colar      = 1586 // ItemType 14
	espadaLend = 2515 // ItemType 1001: arma, nao acessorio
)

func itensDoTesteMutante() *tabelaDeItens {
	t := novaTabelaDeItens()
	t.poeItem(100, gamedata.ItemFromList{})
	t.poeItem(aneis, gamedata.ItemFromList{ItemType: 11})
	t.poeItem(brinco, gamedata.ItemFromList{ItemType: 12})
	t.poeItem(bracelete, gamedata.ItemFromList{ItemType: 13})
	t.poeItem(colar, gamedata.ItemFromList{ItemType: 14})
	t.poeItem(espadaLend, gamedata.ItemFromList{ItemType: 1001})
	return t
}

// tabelaMutantes monta uma tabela com os seis mutantes, um mob comum e a defesa alta.
func tabelaMutantes() *mobdata.Table {
	nomes := []struct {
		nome string
		drop uint16
	}{
		{"Planta_Carnivora_Mutante", 9}, {"Cao_Mutante_Croshu", 6}, {"Buto_Mutante", 5},
		{"Croshu_Mutante_das_Cinzas", 7}, {"Penza_Poderosa_Mutante", 8}, {"Verit_Mutante", 10},
		{"Mob_Comum", 0},
	}
	var sps []mobdata.Species
	for _, n := range nomes {
		sp := especie(n.nome, 1000, 1, 1000, 1000)
		sp.DropIndex = n.drop
		sp.DefFis, sp.DefMag = 15000, 9000
		sps = append(sps, sp)
	}
	t := tabelaDeMobs(sps...)
	for i := range t.Species {
		t.ByName[t.Species[i].Name] = i
	}
	return t
}

func dropsDosMutantes() *dropdata.Table {
	d := &dropdata.Table{}
	d.Categories[9].Items[dropdata.Legendary] = []uint16{aneis, brinco, espadaLend}
	d.Categories[10].Items[dropdata.Legendary] = []uint16{bracelete, colar}
	d.Categories[5].Items[dropdata.Legendary] = []uint16{espadaLend}
	d.Categories[0].Items[dropdata.Normal] = []uint16{100}
	return d
}

func TestModoDeTesteReduzADefesaSoDosAlvos(t *testing.T) {
	mobs := tabelaMutantes()
	mt := NewMutantTest(mobs, dropsDosMutantes(), itensDoTesteMutante(), 700)

	for _, nome := range mutantTestTargets {
		sp := mobs.Species[mobs.ByName[nome]]
		if sp.DefFis != 700 || sp.DefMag != 700 {
			t.Errorf("%s: defesa %d/%d, queria o teto 700", nome, sp.DefFis, sp.DefMag)
		}
		if !mt.Species[sp.Index] {
			t.Errorf("%s deveria ser alvo", nome)
		}
	}
	// Penza, Verit e o mob comum nao sao alvo e mantem a defesa.
	for _, nome := range []string{"Penza_Poderosa_Mutante", "Verit_Mutante", "Mob_Comum"} {
		sp := mobs.Species[mobs.ByName[nome]]
		if sp.DefFis != 15000 || sp.DefMag != 9000 || mt.Species[sp.Index] {
			t.Errorf("%s foi alterado: %+v", nome, sp)
		}
	}
}

// Defesa que ja e menor que o teto nao sobe.
func TestModoDeTesteNaoAumentaDefesaBaixa(t *testing.T) {
	mobs := tabelaMutantes()
	i := mobs.ByName["Buto_Mutante"]
	mobs.Species[i].DefFis, mobs.Species[i].DefMag = 120, 90
	NewMutantTest(mobs, dropsDosMutantes(), itensDoTesteMutante(), 700)
	if sp := mobs.Species[i]; sp.DefFis != 120 || sp.DefMag != 90 {
		t.Errorf("defesa = %d/%d, queria inalterada", sp.DefFis, sp.DefMag)
	}
}

// O sorteio so tem acessorios (ItemType 11 a 14): a espada lendaria, que tem
// ItemType 1001, nao entra.
func TestModoDeTesteSorteiaSoAcessorios(t *testing.T) {
	mt := NewMutantTest(tabelaMutantes(), dropsDosMutantes(), itensDoTesteMutante(), 700)
	got := map[uint16]bool{}
	for _, it := range mt.Pool {
		got[it] = true
	}
	if len(mt.Pool) != 4 || !got[aneis] || !got[brinco] || !got[bracelete] || !got[colar] || got[espadaLend] {
		t.Errorf("pool = %v, queria anel, brinco, bracelete e colar sem a espada", mt.Pool)
	}
}

func matarMutante(t *testing.T, nome string) (*Handler, uint16) {
	t.Helper()
	mobs := tabelaMutantes()
	h, _ := handlerComMobs(t, mobs)
	h.Drops = dropsDosMutantes()
	h.MapObjs = NewMapObjects()
	h.Data = itensDoTesteMutante()
	h.MutantTest = NewMutantTest(mobs, h.Drops, h.Data, 700)
	h.Rand = randFixo{v: 50}

	// O mob alvo e o unico spawn da especie escolhida.
	sp := mobs.Species[mobs.ByName[nome]]
	alvo := sp.Spawns[0].ClientID
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	h.Chan.Join(e.conn, s)
	s.Base.VisibleMobs.Add(alvo)
	s.Avatar.AtkFis = 60000 // o golpe tem de matar
	if err := h.attack(context.Background(), e.conn, s, asPacket(t, atkPacket(alvo, 0))); err != nil {
		t.Fatal(err)
	}
	return h, alvo
}

func TestMutanteAlvoDeixaAcessorioNoChao(t *testing.T) {
	for _, nome := range mutantTestTargets {
		h, _ := matarMutante(t, nome)
		obj, ok := h.MapObjs.Get(1)
		if !ok {
			t.Fatalf("%s: nada caiu no chao", nome)
		}
		if obj.ItemID < aneis || obj.ItemID > colar {
			t.Errorf("%s: caiu %d, queria um acessorio (%d a %d)", nome, obj.ItemID, aneis, colar)
		}
	}
}

// Quem nao e alvo segue o sorteio normal.
func TestMutanteForaDoTesteSegueOSorteioNormal(t *testing.T) {
	h, _ := matarMutante(t, "Verit_Mutante")
	obj, ok := h.MapObjs.Get(1)
	if !ok {
		t.Fatal("nada caiu no chao")
	}
	if obj.ItemID == aneis || obj.ItemID == brinco {
		t.Errorf("Verit_Mutante nao e alvo, mas deixou %d", obj.ItemID)
	}
}

func TestModoDeTesteComEspecieAusenteNaoQuebra(t *testing.T) {
	mt := NewMutantTest(tabelaDeMobs(especie("Outro", 100, 1, 1000, 1000)), &dropdata.Table{}, itensDoTesteMutante(), 700)
	if len(mt.Species) != 0 || len(mt.Pool) != 0 {
		t.Errorf("mt = %+v", mt)
	}
	if mt2 := NewMutantTest(nil, nil, itensDoTesteMutante(), 700); mt2 == nil || len(mt2.Species) != 0 {
		t.Error("tabela nula deveria dar um modo de teste vazio")
	}
	var h Handler
	if _, ok := h.mutantTestDrop(&Mob{}); ok {
		t.Error("sem MutantTest nao se forca nada")
	}
}
