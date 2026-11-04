package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"aika/internal/auth"
	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// handlerComComandos devolve um handler com canal, itens e tabela de exp.
func handlerComComandos(t *testing.T) *Handler {
	h := handlerComGuildas(t)
	itens := novaTabelaDeItens()
	itens.poeItem(100, gamedata.ItemFromList{})
	itens.poeItem(20038, gamedata.ItemFromList{})
	h.Data = itens
	h.Exp = tabelaExp{}
	h.Durations = duracoesFixas{}
	h.MapObjs = NewMapObjects()
	return h
}

// tabelaExp e uma tabela de exp simples: nivel n exige n*1000.
type tabelaExp struct{}

func (tabelaExp) ExpForLevel(level uint16) (uint64, bool) {
	if level > 150 {
		return 0, false
	}
	return uint64(level) * 1000, true
}

// duracoesFixas da 60 s para todo buff de 1 a 100.
type duracoesFixas struct{}

func (duracoesFixas) BuffSeconds(index uint16) int {
	if index >= 1 && index <= 100 {
		return 60
	}
	return 0
}

// gm poe uma sessao em jogo com o nivel de conta dado.
func gm(t *testing.T, h *Handler, nome string, charID uint32, tipo uint8) (*Session, *espiao) {
	s, e := entraComPersonagem(t, h, nome, charID)
	s.Account.Type = tipo
	s.Position = model.Position{X: 1000, Y: 1000}
	s.Avatar.Level = 10
	s.Avatar.Exp = 0
	e.drena(50 * time.Millisecond)
	return s, e
}

// roda executa um comando pelo chat, como GM.
func roda(h *Handler, s *Session, e *espiao, linha string) string {
	campos := strings.Fields(linha)
	nome, args := campos[0], campos[1:]
	cc := &CommandContext{Caller: s, CallerConn: e.conn, Origem: OrigemChat, Args: args}
	return h.RunCommand(context.Background(), cc, nome)
}

// rodaConsole executa pelo console, sem jogador por tras.
func rodaConsole(h *Handler, linha string) string {
	campos := strings.Fields(linha)
	return h.RunConsole(context.Background(), campos[0], campos[1:])
}

// ---- permissao

// Na origem ProcessCommands nao conferia permissao: o gate estava em quem
// chamava, e o console chamava sem gate nenhum.
func TestComandoExigeNivelDeConta(t *testing.T) {
	h := handlerComComandos(t)
	jogador, e := gm(t, h, "Ze", 100, auth.AccPlayer)

	if resp := roda(h, jogador, e, "additem 100"); resp != "Sem permissao." {
		t.Errorf("jogador comum: %q", resp)
	}
	// Moderador ainda nao basta para additem (exige GameMaster).
	jogador.Account.Type = auth.AccModerator
	if resp := roda(h, jogador, e, "additem 100"); resp != "Sem permissao." {
		t.Errorf("moderador: %q", resp)
	}
	jogador.Account.Type = auth.AccGameMaster
	if resp := roda(h, jogador, e, "additem 100"); strings.Contains(resp, "permissao") {
		t.Errorf("gm: %q", resp)
	}
}

// setrank exige Admin, nao GameMaster: na origem qualquer GM se promovia.
func TestSetrankExigeAdmin(t *testing.T) {
	h := handlerComComandos(t)
	a, ea := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	gm(t, h, "Beta", 200, auth.AccPlayer)

	if resp := roda(h, a, ea, "setrank Beta 5"); resp != "Sem permissao." {
		t.Errorf("gm: %q", resp)
	}
	a.Account.Type = auth.AccAdmin
	if resp := roda(h, a, ea, "setrank Beta 4"); strings.Contains(resp, "permissao") {
		t.Errorf("admin: %q", resp)
	}
}

// A origem fazia "TAccountType(Rank)" sem conferir a faixa.
func TestSetrankConfereAFaixa(t *testing.T) {
	h := handlerComComandos(t)
	a, ea := gm(t, h, "Alfa", 100, auth.AccAdmin)
	b, _ := gm(t, h, "Beta", 200, auth.AccPlayer)

	for _, r := range []string{"6", "99", "255", "-1", "1000000"} {
		resp := roda(h, a, ea, "setrank Beta "+r)
		if !strings.Contains(resp, "faixa") {
			t.Errorf("rank %s aceito: %q", r, resp)
		}
	}
	if b.Account.Type != auth.AccPlayer {
		t.Errorf("o rank mudou para %d", b.Account.Type)
	}
	// E exige o nome, que a origem deixava opcional (aplicava em si mesmo).
	if resp := roda(h, a, ea, "setrank 5"); !strings.Contains(resp, "Uso:") {
		t.Errorf("sem nome: %q", resp)
	}
	if a.Account.Type != auth.AccAdmin {
		t.Error("o proprio rank mudou")
	}
}

// ---- comandos sem parametro

// Na origem /dmg sem parametro aplicava dano zero por um "finally" e deixava
// a excecao subir; vindo do console, matava a thread do console.
func TestComandosSemParametroNaoEstouram(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccAdmin)
	hpAntes := s.Avatar.CurHP

	for _, cmd := range []string{"dmg", "additem", "addgold", "addexp",
		"addlevel", "move", "dc", "addbuff", "removebuff", "setrank"} {
		resp := roda(h, s, e, cmd)
		if resp == "" {
			t.Errorf("%s: resposta vazia", cmd)
		}
		if !strings.Contains(resp, "Uso:") && !strings.Contains(resp, "obrigatorio") {
			t.Errorf("%s: %q", cmd, resp)
		}
	}
	if s.Avatar.CurHP != hpAntes {
		t.Errorf("HP mudou de %d para %d sem parametro", hpAntes, s.Avatar.CurHP)
	}
}

func TestComandoDesconhecido(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccAdmin)
	for _, cmd := range []string{"naoexiste", "/", "//", "   x   "} {
		cc := &CommandContext{Caller: s, CallerConn: e.conn, Origem: OrigemChat}
		if resp := h.RunCommand(context.Background(), cc, cmd); resp != "Comando invalido." {
			t.Errorf("%q: %q", cmd, resp)
		}
	}
}

func TestAjudaListaOsComandos(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccAdmin)
	resp := roda(h, s, e, "help")
	for _, esperado := range []string{"additem", "addgold", "dmg", "who"} {
		if !strings.Contains(resp, esperado) {
			t.Errorf("a ajuda nao lista %s: %q", esperado, resp)
		}
	}
}

// ---- additem

func TestAdditem(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Inventory = sessaoComInventario().Inventory

	if resp := roda(h, s, e, "additem 100 5"); !strings.Contains(resp, "slot") {
		t.Fatalf("resposta = %q", resp)
	}
	it := s.Inventory.Get(0)
	if it.Index != 100 {
		t.Errorf("item = %+v", it)
	}
}

// A origem recusava 20038..20041 com "Itens banidos."; mantido.
func TestAdditemRecusaItemBanido(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Inventory = sessaoComInventario().Inventory

	for _, id := range []string{"20038", "20039", "20040", "20041"} {
		if resp := roda(h, s, e, "additem "+id); resp != "Itens banidos." {
			t.Errorf("item %s: %q", id, resp)
		}
	}
	if !s.Inventory.Get(0).IsEmpty() {
		t.Error("o item banido entrou no inventario")
	}
}

func TestAdditemComNomeDeJogador(t *testing.T) {
	h := handlerComComandos(t)
	a, ea := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	b, _ := gm(t, h, "Beta", 200, auth.AccPlayer)
	a.Inventory = sessaoComInventario().Inventory
	b.Inventory = sessaoComInventario().Inventory

	if resp := roda(h, a, ea, "additem Beta 100 3"); !strings.Contains(resp, "Beta") {
		t.Fatalf("resposta = %q", resp)
	}
	if b.Inventory.Get(0).Index != 100 {
		t.Error("o item nao foi para o Beta")
	}
	if !a.Inventory.Get(0).IsEmpty() {
		t.Error("o item foi tambem para quem digitou")
	}
}

func TestAdditemJogadorInexistente(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	if resp := roda(h, s, e, "additem Ninguem 100 1"); resp != "Jogador nao encontrado." {
		t.Errorf("resposta = %q", resp)
	}
}

func TestAdditemInventarioCheio(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Inventory = sessaoComInventario().Inventory
	for i := 0; i < MovableInventorySlots; i++ {
		s.Inventory.Slots[i] = model.Item{Index: 100}
	}
	if resp := roda(h, s, e, "additem 100"); resp != "Inventario cheio." {
		t.Errorf("resposta = %q", resp)
	}
}

// ---- addgold

func TestAddgold(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Avatar.Gold = 100

	roda(h, s, e, "addgold 500")
	if s.Avatar.Gold != 600 {
		t.Errorf("ouro = %d", s.Avatar.Gold)
	}
	// Valor negativo desconta, com piso em zero.
	roda(h, s, e, "addgold -1000")
	if s.Avatar.Gold != 0 {
		t.Errorf("ouro = %d", s.Avatar.Gold)
	}
}

// A origem fazia "Inc(Gold, Value)" sem teto: dois comandos com o maximo do
// Int64 davam a volta e deixavam o ouro negativo.
func TestAddgoldNaoDaAVolta(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Avatar.Gold = 0

	for i := 0; i < 3; i++ {
		roda(h, s, e, "addgold 9223372036854775807")
		if s.Avatar.Gold < 0 {
			t.Fatalf("ouro negativo: %d", s.Avatar.Gold)
		}
		if s.Avatar.Gold > MaxGold {
			t.Fatalf("ouro acima do teto: %d", s.Avatar.Gold)
		}
	}
	if s.Avatar.Gold != MaxGold {
		t.Errorf("ouro = %d, queria o teto %d", s.Avatar.Gold, MaxGold)
	}
}

// ---- addlevel

// A origem lia o nivel de QUEM DIGITOU para calcular a exp alvo, mesmo com o
// alvo sendo outro jogador.
func TestAddlevelUsaONivelDoAlvo(t *testing.T) {
	h := handlerComComandos(t)
	a, ea := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	b, _ := gm(t, h, "Beta", 200, auth.AccPlayer)
	a.Avatar.Level = 100
	b.Avatar.Level = 10
	b.Avatar.Exp = 0

	roda(h, a, ea, "addlevel Beta 1")
	// Nivel 11 exige 11000 na tabela de teste: a exp do Beta tem de chegar
	// la, nao a soma do nivel do Alfa.
	if b.Avatar.Exp != 11000 {
		t.Errorf("exp do Beta = %d, queria 11000", b.Avatar.Exp)
	}
	if a.Avatar.Exp != 0 {
		t.Errorf("a exp de quem digitou mudou: %d", a.Avatar.Exp)
	}
}

// "/addlevel 0" na origem dava 65535 no indice do vetor, caia no except, e o
// fallback deixava a exp dar a volta num UInt64.
func TestAddlevelZeroOuNegativo(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Avatar.Exp = 5000

	for _, n := range []string{"0", "-1", "-100"} {
		resp := roda(h, s, e, "addlevel "+n)
		if !strings.Contains(resp, "positivo") {
			t.Errorf("addlevel %s: %q", n, resp)
		}
	}
	if s.Avatar.Exp != 5000 {
		t.Errorf("a exp mudou para %d", s.Avatar.Exp)
	}
}

func TestAddlevelAcimaDaTabela(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	resp := roda(h, s, e, "addlevel 9999")
	if !strings.Contains(resp, "tabela") {
		t.Errorf("resposta = %q", resp)
	}
}

// ---- dmg, move, dc

func TestDmgAtingeOAlvoNomeado(t *testing.T) {
	h := handlerComComandos(t)
	a, ea := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	b, _ := gm(t, h, "Beta", 200, auth.AccPlayer)
	a.Avatar.CurHP = 1000
	b.Avatar.CurHP = 1000

	// A origem so sabia bater em quem digitou.
	roda(h, a, ea, "dmg Beta 300")
	if b.Avatar.CurHP != 700 {
		t.Errorf("HP do Beta = %d", b.Avatar.CurHP)
	}
	if a.Avatar.CurHP != 1000 {
		t.Errorf("HP de quem digitou = %d", a.Avatar.CurHP)
	}
}

func TestMove(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	roda(h, s, e, "move 2000 3000")
	if s.Position != (model.Position{X: 2000, Y: 3000}) {
		t.Errorf("posicao = %+v", s.Position)
	}
	// Fora do mapa e recusado: a origem nao validava nada.
	resp := roda(h, s, e, "move 999999 999999")
	if !strings.Contains(resp, "mapa") {
		t.Errorf("resposta = %q", resp)
	}
	if s.Position != (model.Position{X: 2000, Y: 3000}) {
		t.Errorf("a posicao mudou para %+v", s.Position)
	}
}

func TestDcExigeNome(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	if resp := roda(h, s, e, "dc"); !strings.Contains(resp, "Uso:") {
		t.Errorf("resposta = %q", resp)
	}
	if resp := roda(h, s, e, "dc Ninguem"); resp != "Jogador nao encontrado." {
		t.Errorf("resposta = %q", resp)
	}
}

// ---- buffs

func TestAddERemoveBuff(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)

	if resp := roda(h, s, e, "addbuff 7"); !strings.Contains(resp, "60s") {
		t.Fatalf("resposta = %q", resp)
	}
	if !s.Base.Buffs.Has(7) {
		t.Error("o buff nao entrou")
	}
	if resp := roda(h, s, e, "removebuff 7"); !strings.Contains(resp, "removido") {
		t.Errorf("resposta = %q", resp)
	}
	if s.Base.Buffs.Has(7) {
		t.Error("o buff ficou")
	}
	// Buff sem duracao na tabela e recusado.
	if resp := roda(h, s, e, "addbuff 9999"); !strings.Contains(resp, "duracao") {
		t.Errorf("resposta = %q", resp)
	}
	// Remover o que nao tem avisa.
	if resp := roda(h, s, e, "removebuff 7"); !strings.Contains(resp, "nao tem") {
		t.Errorf("resposta = %q", resp)
	}
}

// ---- console

// Pelo console o nome do jogador e obrigatorio: a origem rodava o comando
// como um Admin online sorteado, e um /addgold sem nome dava o ouro a ele.
func TestConsoleExigeNomeDeJogador(t *testing.T) {
	h := handlerComComandos(t)
	s, _ := gm(t, h, "Alfa", 100, auth.AccAdmin)
	s.Avatar.Gold = 100

	resp := rodaConsole(h, "addgold 1000000")
	if !strings.Contains(resp, "obrigatorio") {
		t.Errorf("resposta = %q", resp)
	}
	if s.Avatar.Gold != 100 {
		t.Errorf("o Admin online recebeu o ouro: %d", s.Avatar.Gold)
	}
}

func TestConsoleComNomeFunciona(t *testing.T) {
	h := handlerComComandos(t)
	s, _ := gm(t, h, "Alfa", 100, auth.AccPlayer)
	s.Avatar.Gold = 100

	resp := rodaConsole(h, "addgold Alfa 500")
	if !strings.Contains(resp, "600") {
		t.Errorf("resposta = %q", resp)
	}
	if s.Avatar.Gold != 600 {
		t.Errorf("ouro = %d", s.Avatar.Gold)
	}
}

// O console nao precisa de conta: a origem exigia um Admin online e nao fazia
// nada sem ele.
func TestConsoleFuncionaSemAdminOnline(t *testing.T) {
	h := handlerComComandos(t)
	s, _ := gm(t, h, "Ze", 100, auth.AccPlayer)
	s.Avatar.CurHP = 1000

	resp := rodaConsole(h, "dmg Ze 100")
	if strings.Contains(resp, "permissao") {
		t.Errorf("resposta = %q", resp)
	}
	if s.Avatar.CurHP != 900 {
		t.Errorf("HP = %d", s.Avatar.CurHP)
	}
}

func TestWho(t *testing.T) {
	h := handlerComComandos(t)
	gm(t, h, "Alfa", 100, auth.AccPlayer)
	gm(t, h, "Beta", 200, auth.AccPlayer)

	resp := rodaConsole(h, "who")
	if !strings.Contains(resp, "Alfa") || !strings.Contains(resp, "Beta") {
		t.Errorf("resposta = %q", resp)
	}
	if !strings.HasPrefix(resp, "2 em jogo") {
		t.Errorf("resposta = %q", resp)
	}
}

// Comando da origem que depende de sistema nao portado responde dizendo isso,
// em vez de nao fazer nada (corpo vazio de /addmob, "Ainda nao setado." de
// /equip).
func TestComandoNaoPortadoAvisa(t *testing.T) {
	h := handlerComComandos(t)
	for _, cmd := range []string{"addmob", "equip", "addcash", "pix",
		"startevent", "setmarechal", "npc", "walkto"} {
		resp := rodaConsole(h, cmd)
		if !strings.HasPrefix(resp, "Ainda nao portado") {
			t.Errorf("%s: %q", cmd, resp)
		}
	}
}

// 'npc' esta na lista de nomes da origem e nao tem ramo no case: caia no else
// com "Comando invalido.". Aqui ele existe e diz o que falta.
func TestNpcEstaNoRegistro(t *testing.T) {
	nomes := CommandNames()
	achou := false
	for _, n := range nomes {
		if n == "npc" {
			achou = true
		}
	}
	if !achou {
		t.Error("npc nao esta no registro")
	}
}

// ---- chat

// O nome do comando vem do campo Nick, que a origem sobrescrevia com o nome
// do autor antes de usar.
func TestComandoVemDoApelidoNaoDoNomeDoAutor(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Alfa", 100, auth.AccGameMaster)
	s.Avatar.Gold = 100

	var pkt packets.ChatPacket
	pkt.TypeChat = ChatWhisper
	netsrv.PutCString(pkt.Nick[:], "/addgold")
	netsrv.PutCString(pkt.Fala[:], "777")
	buf, err := netsrv.Marshal(CodeClientSay, pkt)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.clientSay(context.Background(), e.conn, s,
		asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 877 {
		t.Errorf("ouro = %d, queria 877: o comando nao foi lido do apelido",
			s.Avatar.Gold)
	}
}

// Jogador comum no sussurro com barra nao roda comando.
func TestSussurroDeJogadorComumNaoRodaComando(t *testing.T) {
	h := handlerComComandos(t)
	s, e := gm(t, h, "Ze", 100, auth.AccPlayer)
	s.Avatar.Gold = 100

	var pkt packets.ChatPacket
	pkt.TypeChat = ChatWhisper
	netsrv.PutCString(pkt.Nick[:], "/addgold")
	netsrv.PutCString(pkt.Fala[:], "999999")
	buf, _ := netsrv.Marshal(CodeClientSay, pkt)
	if err := h.clientSay(context.Background(), e.conn, s,
		asPacket(t, buf)); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.Gold != 100 {
		t.Errorf("jogador comum ganhou ouro: %d", s.Avatar.Gold)
	}
}

func TestChatSemCallerRecusa(t *testing.T) {
	h := handlerComComandos(t)
	cc := &CommandContext{Origem: OrigemChat, Args: []string{"100"}}
	if resp := h.RunCommand(context.Background(), cc, "addgold"); resp != "Comando invalido." {
		t.Errorf("resposta = %q", resp)
	}
}
