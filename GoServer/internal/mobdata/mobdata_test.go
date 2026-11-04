package mobdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escreve cria um CSV temporario.
func escreve(t *testing.T, nome, conteudo string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, nome)
	if err := os.WriteFile(p, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// linhaSpawn monta uma linha de MonsterListCSV com as colunas uteis.
func linhaSpawn(nome string, initX, initY, destX, destY int) string {
	col := make([]string, 18)
	for i := range col {
		col[i] = "0"
	}
	col[4] = nome
	col[9] = itoa(initX)
	col[10] = itoa(initY)
	col[11] = "11"
	col[12] = "8"
	col[14] = itoa(destX)
	col[15] = itoa(destY)
	col[16] = "12"
	col[17] = "9"
	return strings.Join(col, ",")
}

// linhaInfo monta uma linha de AllMobsInfo.
func linhaInfo(id int, nome string, hp int) string {
	col := make([]string, 27)
	for i := range col {
		col[i] = "0"
	}
	col[0] = itoa(id)
	col[1] = nome
	col[5] = itoa(hp)
	col[7] = "10"    // nivel
	col[14] = "500"  // AtkFis
	col[16] = "200"  // DefFis
	col[18] = "30"   // respawn
	col[24] = "1234" // exp
	col[26] = "1"    // ativo
	return strings.Join(col, ",")
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func TestCarregaSpawns(t *testing.T) {
	p := escreve(t, "MonsterListCSV.csv", strings.Join([]string{
		linhaSpawn("Max_Filhote", 100, 200, 110, 210),
		linhaSpawn("Max_Filhote", 101, 201, 111, 211),
		linhaSpawn("Merkbee", 300, 400, 310, 410),
	}, "\n"))

	tb, err := LoadSpawns(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Species) != 2 {
		t.Fatalf("%d especies, queria 2", len(tb.Species))
	}
	if tb.SpawnCount() != 3 {
		t.Errorf("%d pontos, queria 3", tb.SpawnCount())
	}

	// Os ClientID saem de 3048 na ordem das linhas e nao reiniciam por
	// especie, como na origem.
	esperado := []uint16{FirstMobClientID, FirstMobClientID + 1, FirstMobClientID + 2}
	got := []uint16{
		tb.Species[0].Spawns[0].ClientID,
		tb.Species[0].Spawns[1].ClientID,
		tb.Species[1].Spawns[0].ClientID,
	}
	for i := range esperado {
		if got[i] != esperado[i] {
			t.Errorf("ClientID[%d] = %d, queria %d", i, got[i], esperado[i])
		}
	}

	// SecondIndex comeca em 1 por especie, como o idGen da origem.
	if tb.Species[0].Spawns[0].SecondIndex != 1 ||
		tb.Species[0].Spawns[1].SecondIndex != 2 ||
		tb.Species[1].Spawns[0].SecondIndex != 1 {
		t.Error("SecondIndex deveria comecar em 1 por especie")
	}
}

// O destino recebe o deslocamento de 5 celulas da origem.
func TestDestinoRecebeDeslocamentoDeCinco(t *testing.T) {
	p := escreve(t, "MonsterListCSV.csv", linhaSpawn("Bicho", 100, 200, 300, 400))
	tb, err := LoadSpawns(p)
	if err != nil {
		t.Fatal(err)
	}
	s := tb.Species[0].Spawns[0]
	if s.FirstDestPos.X != 300 || s.FirstDestPos.Y != 400 {
		t.Errorf("FirstDestPos = %v", s.FirstDestPos)
	}
	if s.DestPos.X != 305 || s.DestPos.Y != 405 {
		t.Errorf("DestPos = %v, queria (305,405)", s.DestPos)
	}
	if s.InitAttackRange != 11 || s.InitMoveWait != 8 ||
		s.DestAttackRange != 12 || s.DestMoveWait != 9 {
		t.Errorf("alcances e esperas errados: %+v", s)
	}
}

// Mutante e Crenon ficam parados; so o Mutante recebe a marca, como na origem.
func TestMutanteECrenonFicamParados(t *testing.T) {
	p := escreve(t, "MonsterListCSV.csv", strings.Join([]string{
		linhaSpawn("Lobo_Mutante", 100, 200, 300, 400),
		linhaSpawn("Crenon_Guarda", 500, 600, 700, 800),
		linhaSpawn("Bicho_Comum", 10, 20, 30, 40),
	}, "\n"))
	tb, err := LoadSpawns(p)
	if err != nil {
		t.Fatal(err)
	}

	mut := tb.Species[tb.ByName["Lobo_Mutante"]]
	if !mut.Mutant {
		t.Error("Mutante deveria ter a marca")
	}
	if mut.Spawns[0].DestPos != mut.Spawns[0].InitPos {
		t.Errorf("Mutante deveria ficar parado: %v", mut.Spawns[0])
	}

	cre := tb.Species[tb.ByName["Crenon_Guarda"]]
	if cre.Mutant {
		t.Error("Crenon nao recebia a marca na origem")
	}
	if cre.Spawns[0].DestPos != cre.Spawns[0].InitPos {
		t.Errorf("Crenon deveria ficar parado: %v", cre.Spawns[0])
	}

	com := tb.Species[tb.ByName["Bicho_Comum"]]
	if com.Spawns[0].DestPos == com.Spawns[0].InitPos {
		t.Error("bicho comum deveria ter destino diferente")
	}
}

// O bug do vetor: a origem indexava MobsP[0..49] e TMobS[0..449] sem checar.
// Os dados de hoje chegam exatamente no limite.
func TestLimiteDePontosPorEspecie(t *testing.T) {
	var linhas []string
	for i := 0; i < MaxSpawnsPerSpecies+5; i++ {
		linhas = append(linhas, linhaSpawn("Muitos", 100+i, 200, 300, 400))
	}
	tb, err := LoadSpawns(escreve(t, "MonsterListCSV.csv", strings.Join(linhas, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tb.Species[0].Spawns); n != MaxSpawnsPerSpecies {
		t.Errorf("%d pontos, queria o teto de %d", n, MaxSpawnsPerSpecies)
	}
	if len(tb.Avisos) != 5 {
		t.Errorf("%d avisos, queria 5: %v", len(tb.Avisos), tb.Avisos)
	}
}

func TestLimiteDeEspecies(t *testing.T) {
	var linhas []string
	for i := 0; i < MaxSpecies+3; i++ {
		linhas = append(linhas, linhaSpawn("Bicho"+itoa(i), 1, 2, 3, 4))
	}
	tb, err := LoadSpawns(escreve(t, "MonsterListCSV.csv", strings.Join(linhas, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tb.Species); n != MaxSpecies {
		t.Errorf("%d especies, queria o teto de %d", n, MaxSpecies)
	}
	if len(tb.Avisos) != 3 {
		t.Errorf("%d avisos, queria 3", len(tb.Avisos))
	}
}

// Linha curta nao derruba a carga: a origem chamava FileStrings[17] e
// levantava excecao, deixando o servidor sem o resto dos mobs.
func TestLinhaCurtaNaoDerrubaACarga(t *testing.T) {
	p := escreve(t, "MonsterListCSV.csv", strings.Join([]string{
		linhaSpawn("Bom", 1, 2, 3, 4),
		"1,2,3",
		"",
		linhaSpawn("Outro", 5, 6, 7, 8),
	}, "\n"))
	tb, err := LoadSpawns(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Species) != 2 {
		t.Errorf("%d especies, queria 2", len(tb.Species))
	}
	if len(tb.Avisos) != 1 {
		t.Errorf("avisos = %v", tb.Avisos)
	}
}

// Campo invalido vale zero em vez de derrubar a carga: a origem usava
// ToInteger(), que levanta excecao.
func TestCampoInvalidoValeZero(t *testing.T) {
	col := make([]string, 18)
	for i := range col {
		col[i] = "lixo"
	}
	col[4] = "Bicho"
	tb, err := LoadSpawns(escreve(t, "MonsterListCSV.csv", strings.Join(col, ",")))
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Species) != 1 {
		t.Fatalf("%d especies", len(tb.Species))
	}
	s := tb.Species[0].Spawns[0]
	if s.InitPos.X != 0 || s.InitAttackRange != 0 {
		t.Errorf("campos invalidos deveriam valer zero: %+v", s)
	}
}

func TestCarregaInfo(t *testing.T) {
	dir := t.TempDir()
	mobs := filepath.Join(dir, "Mobs")
	if err := os.MkdirAll(mobs, 0o700); err != nil {
		t.Fatal(err)
	}
	escrevaEm := func(nome, conteudo string) {
		if err := os.WriteFile(filepath.Join(mobs, nome), []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	escrevaEm("MonsterListCSV.csv", strings.Join([]string{
		linhaSpawn("Max_Filhote", 100, 200, 110, 210),
		linhaSpawn("Max_Filhote", 101, 201, 111, 211),
	}, "\n"))
	// O AllMobsInfo traz o nome com espaco; a origem trocava por underscore.
	escrevaEm("AllMobsInfo.csv", linhaInfo(1, "Max Filhote", 5000))

	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := tb.Species[0]
	if s.IntName != 1 {
		t.Errorf("IntName = %d, queria 1", s.IntName)
	}
	if s.HP != 5000 {
		t.Errorf("HP = %d, queria 5000", s.HP)
	}
	if s.Level != 10 || s.AtkFis != 500 || s.DefFis != 200 {
		t.Errorf("atributos errados: %+v", s)
	}
	if s.Exp != 1234 || s.RespawnSeconds != 30 || !s.ActiveToSpawn {
		t.Errorf("exp/respawn/ativo errados: %+v", s)
	}
	if len(tb.Avisos) != 0 {
		t.Errorf("avisos = %v", tb.Avisos)
	}
}

// HP zero vira o piso de 3500, como na origem.
func TestHPZeroViraOPiso(t *testing.T) {
	dir := t.TempDir()
	mobs := filepath.Join(dir, "Mobs")
	os.MkdirAll(mobs, 0o700)
	os.WriteFile(filepath.Join(mobs, "MonsterListCSV.csv"),
		[]byte(linhaSpawn("Bicho", 1, 2, 3, 4)), 0o600)
	os.WriteFile(filepath.Join(mobs, "AllMobsInfo.csv"),
		[]byte(linhaInfo(7, "Bicho", 0)), 0o600)

	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tb.Species[0].HP != DefaultHP {
		t.Errorf("HP = %d, queria %d", tb.Species[0].HP, DefaultHP)
	}
}

// Especie com ponto de spawn e sem atributos nasceria com HP zero na origem:
// o piso de 3500 estava dentro do ramo que casou o nome.
func TestEspecieSemAtributosRecebeOPisoEAviso(t *testing.T) {
	dir := t.TempDir()
	mobs := filepath.Join(dir, "Mobs")
	os.MkdirAll(mobs, 0o700)
	os.WriteFile(filepath.Join(mobs, "MonsterListCSV.csv"),
		[]byte(linhaSpawn("Orfao", 1, 2, 3, 4)), 0o600)
	os.WriteFile(filepath.Join(mobs, "AllMobsInfo.csv"),
		[]byte(linhaInfo(1, "Outro", 100)), 0o600)

	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tb.Species[0].HP != DefaultHP {
		t.Errorf("HP = %d, queria o piso %d", tb.Species[0].HP, DefaultHP)
	}
	if len(tb.Avisos) < 2 {
		t.Errorf("deveria avisar dos dois lados: %v", tb.Avisos)
	}
}

// Arquivo ausente e aviso, nao erro fatal, como em Load.pas.
func TestArquivoAusente(t *testing.T) {
	_, err := LoadSpawns(filepath.Join(t.TempDir(), "naoexiste.csv"))
	if err == nil || !strings.Contains(err.Error(), "nao encontrado") {
		t.Errorf("erro = %v", err)
	}
}

func TestLookup(t *testing.T) {
	p := escreve(t, "MonsterListCSV.csv", strings.Join([]string{
		linhaSpawn("A", 1, 2, 3, 4),
		linhaSpawn("B", 5, 6, 7, 8),
	}, "\n"))
	tb, err := LoadSpawns(p)
	if err != nil {
		t.Fatal(err)
	}
	sp, pt, ok := tb.Lookup(FirstMobClientID + 1)
	if !ok {
		t.Fatal("deveria achar")
	}
	if sp.Name != "B" || pt.ClientID != FirstMobClientID+1 {
		t.Errorf("achou %q / %d", sp.Name, pt.ClientID)
	}
	// Id que nao existe nao quebra.
	for _, id := range []uint16{0, 1, 3047, 60000} {
		if _, _, ok := tb.Lookup(id); ok {
			t.Errorf("id %d nao deveria existir", id)
		}
	}
	var nulo *Table
	if _, _, ok := nulo.Lookup(FirstMobClientID); ok {
		t.Error("tabela nil nao deveria achar nada")
	}
}

// ---- dados reais

// Confere o carregador contra os dois CSV do servidor, quando estao presentes.
func TestDadosReais(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "Server", "Bin", "Data")
	if _, err := os.Stat(filepath.Join(dir, "Mobs", "MonsterListCSV.csv")); err != nil {
		t.Skip("CSV de mob nao esta neste checkout")
	}
	tb, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("especies=%d pontos=%d avisos=%d",
		len(tb.Species), tb.SpawnCount(), len(tb.Avisos))

	if len(tb.Species) == 0 || tb.SpawnCount() == 0 {
		t.Fatal("nao carregou nada")
	}
	// Os dois arquivos batem: nenhum aviso.
	if len(tb.Avisos) != 0 {
		t.Errorf("avisos nos dados reais: %v", tb.Avisos[:min(3, len(tb.Avisos))])
	}
	// Todo mob carregado tem HP e nome numerico.
	for i := range tb.Species {
		s := &tb.Species[i]
		if s.HP == 0 {
			t.Errorf("especie %q com HP zero", s.Name)
		}
		if s.IntName == 0 {
			t.Errorf("especie %q sem nome numerico", s.Name)
		}
		if len(s.Spawns) == 0 {
			t.Errorf("especie %q sem ponto de spawn", s.Name)
		}
	}
	// Nenhum ClientID repetido.
	if len(tb.ByClientID) != tb.SpawnCount() {
		t.Errorf("%d ids para %d pontos: ha id repetido",
			len(tb.ByClientID), tb.SpawnCount())
	}
	// A primeira especie do arquivo e o Max_Filhote, com IntName 1.
	if tb.Species[0].Name != "Max_Filhote" {
		t.Errorf("primeira especie = %q", tb.Species[0].Name)
	}
	if tb.Species[0].IntName != 1 {
		t.Errorf("IntName do Max_Filhote = %d, queria 1", tb.Species[0].IntName)
	}
}
