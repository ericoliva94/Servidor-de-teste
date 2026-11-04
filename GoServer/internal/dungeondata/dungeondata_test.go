package dungeondata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dadosReais acha a pasta Data do servidor, se ela estiver no repositorio.
func dadosReais(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(filepath.Join(dir, "Dungeons.csv")); err != nil {
		t.Skip("Server/Bin/Data nao esta aqui")
	}
	return dir
}

func TestCarregaDadosReais(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Defs) != 5 {
		t.Fatalf("%d dungeons, queria 5", len(tb.Defs))
	}
	// Toda dungeon tem as tres dificuldades, com mobs e pontos.
	for id := uint32(1); id <= 5; id++ {
		for dif := uint32(0); dif < Difficulties; dif++ {
			d, ok := tb.Get(id, dif)
			if !ok {
				t.Fatalf("dungeon %d dif %d nao carregou", id, dif)
			}
			if len(d.Species) == 0 {
				t.Errorf("dungeon %d dif %d sem especies", id, dif)
			}
			if d.SpawnCount() == 0 {
				t.Errorf("dungeon %d dif %d sem pontos", id, dif)
			}
			if d.SpawnInDungeonPosition.X == 0 || d.SpawnInDungeonPosition.Y == 0 {
				t.Errorf("dungeon %d dif %d sem posicao de entrada: %+v",
					id, dif, d.SpawnInDungeonPosition)
			}
			if d.LevelMin == 0 {
				t.Errorf("dungeon %d dif %d com nivel minimo zero", id, dif)
			}
		}
	}
}

// Os numeros que o Dungeons.csv real tem hoje.
func TestNumerosDosDadosReais(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		id       uint32
		dif      uint32
		nome     string
		npc      uint16
		levelMin uint8
		especies int
		pontos   int
	}{
		{ZantorianCitadel, Normal, "Zantorian Citadel", 2109, 1, 11, 84},
		{ZantorianCitadel, Hard, "Zantorian Citadel", 2109, 20, 11, 84},
		{ZantorianCitadel, Elite, "Zantorian Citadel", 2109, 50, 11, 157},
		{MarauderHold, Normal, "Marauder Hold", 2095, 1, 19, 134},
		{MarauderCabin, Normal, "Marauder Cabin", 2103, 1, 17, 164},
		{LostMines, Normal, "Lost Mine 1", 2197, 30, 19, 137},
		{KinaryAviary, Normal, "Kinary Aviary", 2258, 40, 44, 414},
	}
	for _, c := range casos {
		d, ok := tb.Get(c.id, c.dif)
		if !ok {
			t.Fatalf("dungeon %d dif %d", c.id, c.dif)
		}
		if d.Name != c.nome || d.EntranceNPCID != c.npc || d.LevelMin != c.levelMin {
			t.Errorf("dungeon %d dif %d = %q/%d/%d, queria %q/%d/%d",
				c.id, c.dif, d.Name, d.EntranceNPCID, d.LevelMin,
				c.nome, c.npc, c.levelMin)
		}
		if len(d.Species) != c.especies || d.SpawnCount() != c.pontos {
			t.Errorf("dungeon %d dif %d = %d especies / %d pontos, queria %d / %d",
				c.id, c.dif, len(d.Species), d.SpawnCount(), c.especies, c.pontos)
		}
	}
}

// Os dados reais ESTOURAM o vetor de pontos da origem: nas tres dificuldades
// do Marauder Hold uma especie tem 93 pontos de spawn, e MobsP e
// "Array [0 .. 49]". A origem escrevia
//
//	Self.DGEvgInf[i].MOBS.TMobS[j].MobsP[CountMob] := ...
//
// com CountMob = cntControl, sem checagem nenhuma (ServerSocket.pas:1332): os
// 43 pontos excedentes iam por cima do registro da especie seguinte, e cada
// TMobSPosition carrega um TBaseMob inteiro. Aqui o excedente e recusado e o
// aviso fica no log.
func TestDadosReaisEstouramOVetorDaOrigem(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	var avisoDePontos int
	for _, a := range tb.Avisos {
		if strings.Contains(a, "pontos foram recusados") {
			avisoDePontos++
			t.Log(a)
		}
	}
	if avisoDePontos != 3 {
		t.Errorf("%d avisos de estouro de pontos, queria 3 (as tres "+
			"dificuldades do Marauder Hold)", avisoDePontos)
	}
	// Mesmo com o estouro, nada passa dos limites depois da carga.
	maiorPontos, maiorEspecies, maiorDrop := 0, 0, 0
	for id := uint32(1); id <= 5; id++ {
		for dif := uint32(0); dif < Difficulties; dif++ {
			d, _ := tb.Get(id, dif)
			if n := d.SpawnCount(); n > maiorPontos {
				maiorPontos = n
			}
			if n := len(d.Species); n > maiorEspecies {
				maiorEspecies = n
			}
			for c := range d.Drops {
				if n := len(d.Drops[c]); n > maiorDrop {
					maiorDrop = n
				}
			}
			for i := range d.Species {
				if n := len(d.Species[i].Spawns); n > MaxSpawnsPerSpecies {
					t.Errorf("dungeon %d dif %d especie %d com %d pontos",
						id, dif, d.Species[i].IntName, n)
				}
			}
		}
	}
	if maiorPontos > MaxMobsPerInstance {
		t.Errorf("%d pontos na maior dungeon, acima de %d", maiorPontos, MaxMobsPerInstance)
	}
	if maiorEspecies > MaxSpeciesPerDungeon {
		t.Errorf("%d especies, acima de %d", maiorEspecies, MaxSpeciesPerDungeon)
	}
	if maiorDrop > MaxDropsPerTier {
		t.Errorf("%d itens de drop, acima de %d", maiorDrop, MaxDropsPerTier)
	}
	t.Logf("maior depois da carga: %d pontos, %d especies, %d itens de drop",
		maiorPontos, maiorEspecies, maiorDrop)
}

// O case de coroa da origem so cobria 11..33 (Zantorian). Com os dados reais,
// as outras quatro dungeons ficavam sem drop nenhum.
func TestTodaEspecieRealTemListaDeDrop(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	semDrop := 0
	for id := uint32(1); id <= 5; id++ {
		for dif := uint32(0); dif < Difficulties; dif++ {
			d, _ := tb.Get(id, dif)
			for i := range d.Species {
				sp := &d.Species[i]
				if _, ok := CrownOf(sp.DropIndex); !ok {
					t.Errorf("dungeon %d dif %d especie %d: DropIndex %d sem coroa",
						id, dif, sp.IntName, sp.DropIndex)
					continue
				}
				if len(d.DropsFor(sp)) == 0 {
					semDrop++
					t.Errorf("dungeon %d dif %d especie %d: lista de drop vazia",
						id, dif, sp.IntName)
				}
			}
		}
	}
	if semDrop > 0 {
		t.Errorf("%d especies sem drop", semDrop)
	}
}

// Com o case da origem (apenas 11..33), so o Zantorian teria drop.
func TestCaseDaOrigemDeixariaQuatroDungeonsSemDrop(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	daOrigem := func(idx uint16) bool {
		switch idx {
		case 11, 21, 31, 12, 22, 32, 13, 23, 33:
			return true
		}
		return false
	}
	type conta struct{ com, sem int }
	por := map[uint32]*conta{}
	for id := uint32(1); id <= 5; id++ {
		por[id] = &conta{}
		for dif := uint32(0); dif < Difficulties; dif++ {
			d, _ := tb.Get(id, dif)
			for i := range d.Species {
				if daOrigem(d.Species[i].DropIndex) {
					por[id].com++
				} else {
					por[id].sem++
				}
			}
		}
	}
	if por[ZantorianCitadel].sem != 0 {
		t.Errorf("Zantorian: %d especies fora do case da origem", por[ZantorianCitadel].sem)
	}
	for _, id := range []uint32{MarauderHold, MarauderCabin, LostMines, KinaryAviary} {
		if por[id].com != 0 {
			t.Errorf("dungeon %d: %d especies casariam com o case da origem", id, por[id].com)
		}
		if por[id].sem == 0 {
			t.Errorf("dungeon %d: nenhuma especie", id)
		}
	}
	t.Logf("fora do case da origem: hold=%d cabin=%d mines=%d kinary=%d",
		por[MarauderHold].sem, por[MarauderCabin].sem,
		por[LostMines].sem, por[KinaryAviary].sem)
}

// Dificuldade vinda do cliente e um DWORD. A origem indexava um vetor de um
// elemento com ele.
func TestDificuldadeForaDaFaixa(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, dif := range []uint32{3, 4, 255, 1 << 16, 0xFFFFFFFF} {
		if _, ok := tb.Get(ZantorianCitadel, dif); ok {
			t.Errorf("dificuldade %d foi aceita", dif)
		}
	}
	for _, id := range []uint32{0, 6, 99, 0xFFFFFFFF} {
		if _, ok := tb.Get(id, Normal); ok {
			t.Errorf("dungeon %d foi aceita", id)
		}
	}
}

func TestTabelaNilNaoEstoura(t *testing.T) {
	var tb *Table
	if _, ok := tb.Get(1, 0); ok {
		t.Error("tabela nil devolveu dungeon")
	}
	if _, ok := tb.ByEntranceNPC(2109, 0); ok {
		t.Error("tabela nil achou NPC")
	}
}

func TestPorNPCDeEntrada(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := tb.ByEntranceNPC(2258, Elite)
	if !ok {
		t.Fatal("NPC 2258 nao achou dungeon")
	}
	if d.Index != KinaryAviary || d.Dificult != Elite {
		t.Errorf("= dungeon %d dif %d", d.Index, d.Dificult)
	}
	if _, ok := tb.ByEntranceNPC(9999, Normal); ok {
		t.Error("NPC inexistente achou dungeon")
	}
}

func TestCoroaPeloUltimoDigito(t *testing.T) {
	casos := []struct {
		idx   uint16
		coroa Crown
		ok    bool
	}{
		{11, NoCrown, true}, {12, SilverCrown, true}, {13, GoldCrown, true},
		{21, NoCrown, true}, {33, GoldCrown, true},
		{401, NoCrown, true}, {402, SilverCrown, true}, {603, GoldCrown, true},
		{10, NoCrown, false}, {44, NoCrown, false}, {0, NoCrown, false},
	}
	for _, c := range casos {
		got, ok := CrownOf(c.idx)
		if ok != c.ok || (ok && got != c.coroa) {
			t.Errorf("CrownOf(%d) = %v,%v; queria %v,%v", c.idx, got, ok, c.coroa, c.ok)
		}
	}
}

// Exp = Round(HP * 1.8) com o arredondamento do Delphi.
func TestExpDaEspecie(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tb.Get(ZantorianCitadel, Normal)
	if len(d.Species) == 0 {
		t.Fatal("sem especies")
	}
	for i := range d.Species {
		sp := &d.Species[i]
		queria := uint32(roundHalfEven(float64(sp.HP) * 1.8))
		if sp.Exp != queria {
			t.Errorf("especie %d: exp %d, queria %d", sp.IntName, sp.Exp, queria)
		}
	}
	// 529 * 1.8 = 952.2 -> 952
	if got := roundHalfEven(529 * 1.8); got != 952 {
		t.Errorf("roundHalfEven(952.2) = %d", got)
	}
	// 5 * 1.8 = 9 exato
	if got := roundHalfEven(5 * 1.8); got != 9 {
		t.Errorf("roundHalfEven(9) = %d", got)
	}
	// metade para o par
	if got := roundHalfEven(2.5); got != 2 {
		t.Errorf("roundHalfEven(2.5) = %d, queria 2", got)
	}
	if got := roundHalfEven(3.5); got != 4 {
		t.Errorf("roundHalfEven(3.5) = %d, queria 4", got)
	}
}

// O ataque do mob sai da arma, ou da roupa quando nao ha arma.
func TestAtaqueDaEspecie(t *testing.T) {
	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tb.Get(ZantorianCitadel, Normal)
	for i := range d.Species {
		sp := &d.Species[i]
		queria := uint32(sp.Equip[0])
		if sp.Equip[6] > 0 {
			queria = uint32(sp.Equip[6])
		}
		if sp.AtkFis != queria || sp.AtkMag != queria {
			t.Errorf("especie %d: atk %d/%d, queria %d",
				sp.IntName, sp.AtkFis, sp.AtkMag, queria)
		}
		if sp.DefFis != queria*2 || sp.DefMag != queria*2 {
			t.Errorf("especie %d: def %d/%d, queria %d",
				sp.IntName, sp.DefFis, sp.DefMag, queria*2)
		}
	}
}

// A coluna 7 do MobInfo, que a origem le como nivel do mob, nao e nivel.
// Prova: no Zantorian, no Lost Mine e no Kinary TODO valor passa do nivel
// maximo do jogo (a ExpList.bin tem 152 niveis), e o valor anda poucas
// unidades de uma dificuldade para a outra, como um id sequencial. Onde ele
// cai numa faixa plausivel (28, 29 no Marauder) e so porque o id e baixo.
func TestColunaSeteNaoEhNivel(t *testing.T) {
	const nivelMaximo = 152

	tb, err := Load(dadosReais(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint32{ZantorianCitadel, LostMines, KinaryAviary} {
		for dif := uint32(0); dif < Difficulties; dif++ {
			d, _ := tb.Get(id, dif)
			for i := range d.Species {
				if d.Species[i].Level <= nivelMaximo {
					t.Errorf("dungeon %d dif %d especie %d: coluna 7 = %d, "+
						"abaixo do nivel maximo; se os arquivos mudaram vale "+
						"reavaliar a coluna", id, dif, d.Species[i].IntName,
						d.Species[i].Level)
				}
			}
		}
	}
	// O valor anda junto com a dificuldade para a mesma especie, o que um
	// nivel nao faria: o mesmo mob nao sobe de nivel por estar na lista da
	// dificuldade seguinte.
	normal, _ := tb.Get(ZantorianCitadel, Normal)
	dificil, _ := tb.Get(ZantorianCitadel, Hard)
	if len(normal.Species) == 0 || len(normal.Species) != len(dificil.Species) {
		t.Fatalf("%d e %d especies", len(normal.Species), len(dificil.Species))
	}
	iguais := 0
	for i := range normal.Species {
		if normal.Species[i].Name == dificil.Species[i].Name &&
			normal.Species[i].Level == dificil.Species[i].Level {
			iguais++
		}
	}
	if iguais != 0 {
		t.Errorf("%d especies com o mesmo nome e a mesma coluna 7 nas duas "+
			"dificuldades", iguais)
	}
}

func TestArquivoAusente(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Error("pasta vazia deveria dar erro")
	}
}

func TestDungeonsCsvComLinhaCurta(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dungeons.csv"),
		[]byte("1,Zantorian Citadel,2109\n9,Nao Existe,1,2,3,4,5,6,7,8\n"), 0o600)
	tb, err := Load(dir)
	if err == nil {
		t.Fatalf("deveria recusar: %+v", tb)
	}
}
