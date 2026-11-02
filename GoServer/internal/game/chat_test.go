package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// chatPacket monta um $F86.
func chatPacket(tipo uint16, nick, fala string) []byte {
	var p packets.ChatPacket
	p.TypeChat = tipo
	netsrv.PutCString(p.Nick[:], nick)
	netsrv.PutCString(p.Fala[:], fala)
	buf, err := netsrv.Marshal(CodeClientSay, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// leFala devolve o tipo, o apelido e o texto de um $F86 recebido.
func leFala(t *testing.T, corpo []byte) (uint16, string, string) {
	t.Helper()
	var p packets.ChatPacket
	if err := netsrv.Unmarshal(asPacote(t, corpo), &p); err != nil {
		t.Fatal(err)
	}
	return p.TypeChat, netsrv.CString(p.Nick[:]), netsrv.CString(p.Fala[:])
}

// A fala normal chega ao autor e a quem esta a vista.
func TestFalaNormalChegaAoAutorEAosVisiveis(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(100, 100)
	b := sessaoEmJogo(101, 100)
	a.Avatar.Name = "Alfa"
	b.Avatar.Name = "Beta"
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatNormal, "", "ola mundo")))
	if err != nil {
		t.Fatal(err)
	}

	for nome, e := range map[string]*espiao{"autor": ea, "vizinho": eb} {
		vis := e.proximo(t)
		if vis.code != CodeClientSay {
			t.Fatalf("%s recebeu %#x", nome, vis.code)
		}
		tipo, nick, fala := leFala(t, vis.corpo)
		if tipo != ChatNormal {
			t.Errorf("%s: tipo = %d", nome, tipo)
		}
		if nick != "Alfa" {
			t.Errorf("%s: apelido = %q, queria Alfa", nome, nick)
		}
		if fala != "ola mundo" {
			t.Errorf("%s: fala = %q", nome, fala)
		}
	}
}

// O apelido vai com o nome do autor, nao com o que o cliente mandou: a origem
// repassava o pacote como chegou e dava para falar no nome de outra pessoa.
func TestFalaNaoAceitaApelidoDoCliente(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Verdadeiro"
	h.Chan.Join(e.conn, a)

	err := h.clientSay(context.Background(), e.conn, a,
		asPacket(t, chatPacket(ChatNormal, "Impostor", "oi")))
	if err != nil {
		t.Fatal(err)
	}
	_, nick, _ := leFala(t, e.proximo(t).corpo)
	if nick != "Verdadeiro" {
		t.Errorf("apelido = %q, queria Verdadeiro", nick)
	}
}

// O cabecalho vai com o id do autor.
func TestFalaNaoAceitaIndexDoCliente(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(e.conn, a)

	var p packets.ChatPacket
	p.Header.Index = 999
	p.TypeChat = ChatNormal
	netsrv.PutCString(p.Fala[:], "oi")
	raw, err := netsrv.Marshal(CodeClientSay, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.clientSay(context.Background(), e.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}

	var recebido packets.ChatPacket
	if err := netsrv.Unmarshal(asPacote(t, e.proximo(t).corpo), &recebido); err != nil {
		t.Fatal(err)
	}
	if recebido.Header.Index != a.ClientID {
		t.Errorf("Index = %d, queria %d", recebido.Header.Index, a.ClientID)
	}
}

// Fala vazia nao vira pacote.
func TestFalaVaziaNaoViraPacote(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	h.Chan.Join(e.conn, a)

	for _, texto := range []string{"", "   "} {
		if err := h.clientSay(context.Background(), e.conn, a,
			asPacket(t, chatPacket(ChatNormal, "", texto))); err != nil {
			t.Fatal(err)
		}
	}
	e.nada(t, 150*time.Millisecond)
}

// Texto maior que o campo e cortado, nao copiado para cima dos vizinhos.
func TestFalaLongaECortada(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(e.conn, a)

	longa := strings.Repeat("x", 300)
	if err := h.clientSay(context.Background(), e.conn, a,
		asPacket(t, chatPacket(ChatNormal, "", longa))); err != nil {
		t.Fatal(err)
	}
	_, _, fala := leFala(t, e.proximo(t).corpo)
	if len(fala) != MaxChatRunes {
		t.Errorf("fala com %d bytes, queria %d", len(fala), MaxChatRunes)
	}
}

// O comando `ping responde o $F93B e nao e repassado como fala.
func TestComandoPingRespondeSemFalar(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	b := sessaoEmJogo(1, 0)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatNormal, "", "`ping"))); err != nil {
		t.Fatal(err)
	}
	if got := ea.espera(t); got != CodePingReply {
		t.Errorf("autor recebeu %#x, queria %#x", got, CodePingReply)
	}
	eb.nada(t, 150*time.Millisecond)
	if a.LastPing.IsZero() {
		t.Error("LastPing deveria ter sido marcado")
	}
}

// ---- grito

func TestGritoChegaAoCanalInteiro(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb, ec := novoEspiao(t), novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	b := sessaoEmJogo(5000, 5000) // longe, nem visivel
	c := sessaoEmJogo(9000, 9000)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	h.Chan.Join(ec.conn, c)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatShout, "", "eh nois"))); err != nil {
		t.Fatal(err)
	}
	for nome, e := range map[string]*espiao{"a": ea, "b": eb, "c": ec} {
		if got := e.espera(t); got != CodeClientSay {
			t.Errorf("%s recebeu %#x", nome, got)
		}
	}
}

// O grito respeita o intervalo de 5 s da origem.
func TestGritoTemIntervalo(t *testing.T) {
	agora := time.Now()
	h := testHandler()
	h.Now = func() time.Time { return agora }
	h.Chan = NewChannel(0, 10)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(e.conn, a)

	grito := func() {
		if err := h.clientSay(context.Background(), e.conn, a,
			asPacket(t, chatPacket(ChatShout, "", "ai"))); err != nil {
			t.Fatal(err)
		}
	}

	grito()
	if got := e.espera(t); got != CodeClientSay {
		t.Fatalf("primeiro grito deu %#x", got)
	}

	// Logo em seguida: recusado com mensagem, nao com fala.
	grito()
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("segundo grito deu %#x, queria a mensagem %#x", got, CodeClientMsg)
	}

	// Passado o intervalo: liberado.
	agora = agora.Add(ShoutCooldown + time.Second)
	grito()
	if got := e.espera(t); got != CodeClientSay {
		t.Errorf("terceiro grito deu %#x", got)
	}
}

// O grito so sai da nacao do autor quando ele tem nacao.
func TestGritoFiltraPorNacao(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb, ec := novoEspiao(t), novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	b := sessaoEmJogo(0, 0)
	c := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	a.Avatar.Nation = 1
	b.Avatar.Nation = 1
	c.Avatar.Nation = 2
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	h.Chan.Join(ec.conn, c)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatShout, "", "nacao 1"))); err != nil {
		t.Fatal(err)
	}
	if got := ea.espera(t); got != CodeClientSay {
		t.Errorf("autor recebeu %#x", got)
	}
	if got := eb.espera(t); got != CodeClientSay {
		t.Errorf("mesma nacao recebeu %#x", got)
	}
	ec.nada(t, 150*time.Millisecond)
}

// Canais de conversa que ainda nao existem avisam em vez de ficar calados.
func TestCanaisNaoPortadosAvisam(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(e.conn, a)

	for _, tipo := range []uint16{ChatParty, ChatGuild, ChatAlly, ChatNation, ChatMegafone} {
		if err := h.clientSay(context.Background(), e.conn, a,
			asPacket(t, chatPacket(tipo, "", "oi"))); err != nil {
			t.Fatal(err)
		}
		if got := e.espera(t); got != CodeClientMsg {
			t.Errorf("tipo %d deu %#x, queria a mensagem", tipo, got)
		}
	}
}

// Tipo desconhecido e ignorado, como na origem.
func TestTipoDeFalaDesconhecidoEIgnorado(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	a.Avatar.Name = "Alfa"
	h.Chan.Join(e.conn, a)

	if err := h.clientSay(context.Background(), e.conn, a,
		asPacket(t, chatPacket(777, "", "oi"))); err != nil {
		t.Fatal(err)
	}
	e.nada(t, 150*time.Millisecond)
}

// Falar antes de entrar no mundo e recusado.
func TestFalarExigeEstarEmJogo(t *testing.T) {
	h := testHandler()
	s := sessaoAutenticada()
	err := h.clientSay(context.Background(), connDescartavel(t), s,
		asPacket(t, chatPacket(ChatNormal, "", "oi")))
	if err != ErrNotPlaying {
		t.Errorf("erro = %v, queria ErrNotPlaying", err)
	}
}

// ---- rotacao

func TestRotacaoChegaAosVisiveis(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	b := sessaoEmJogo(1, 0)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)

	var p packets.SignalData
	p.Data = 42
	raw, err := netsrv.Marshal(CodeUpdateRotation, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.updateRotation(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if a.Avatar.Rotation != 42 {
		t.Errorf("rotacao = %d, queria 42", a.Avatar.Rotation)
	}

	vis := eb.proximo(t)
	if vis.code != CodeUpdateRotation {
		t.Fatalf("vizinho recebeu %#x", vis.code)
	}
	var recebido packets.SignalData
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &recebido); err != nil {
		t.Fatal(err)
	}
	if recebido.Header.Index != a.ClientID {
		t.Errorf("Index = %d, queria %d", recebido.Header.Index, a.ClientID)
	}
	// O autor nao recebe de volta: a origem passava sendToSelf = False.
	ea.nada(t, 150*time.Millisecond)
}

// Giro repetido nao vira pacote, como na origem.
func TestRotacaoRepetidaNaoViraPacote(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	ea, eb := novoEspiao(t), novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	b := sessaoEmJogo(1, 0)
	h.Chan.Join(ea.conn, a)
	h.Chan.Join(eb.conn, b)
	a.Base.VisiblePlayers.Add(b.ClientID)
	a.Avatar.Rotation = 7

	var p packets.SignalData
	p.Data = 7
	raw, err := netsrv.Marshal(CodeUpdateRotation, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.updateRotation(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	eb.nada(t, 150*time.Millisecond)
}

// ---- informacao de personagem

// O bug central: "if (Packet.Index >= 0) or (Packet.Index <= MAX_CONNECTIONS)"
// com Index DWORD e sempre verdadeiro, e o ramo seguinte indexava um vetor de
// 1000 posicoes com um valor de ate 4 bilhoes.
func TestCharInfoRecusaIndiceForaDaFaixa(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	h.Chan.Join(e.conn, a)

	for _, idx := range []uint32{11, 1000, 65535, 4000000000} {
		var p packets.RequestCharInfoPacket
		p.Index = idx
		raw, err := netsrv.Marshal(CodeRequestCharInfo, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.requestCharInfo(context.Background(), e.conn, a,
			asPacket(t, raw)); err != nil {
			t.Fatalf("indice %d: %v", idx, err)
		}
	}
	e.nada(t, 150*time.Millisecond)
}

func TestCharInfoPorIndice(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	alvo := sessaoEmJogo(0, 0)
	alvo.Avatar.Name = "Alvo"
	alvo.Avatar.Level = 10
	alvo.Avatar.Honor = 1234
	alvo.Avatar.Str = 50
	h.Chan.Join(e.conn, a)
	h.Chan.Join(connDescartavel(t), alvo)

	var p packets.RequestCharInfoPacket
	p.Index = uint32(alvo.ClientID)
	raw, err := netsrv.Marshal(CodeRequestCharInfo, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.requestCharInfo(context.Background(), e.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}

	vis := e.proximo(t)
	if vis.code != CodeCharInfo {
		t.Fatalf("recebeu %#x, queria %#x", vis.code, CodeCharInfo)
	}
	var resp packets.CharInfoResponsePacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &resp); err != nil {
		t.Fatal(err)
	}
	if got := netsrv.CString(resp.Nick[:]); got != "Alvo" {
		t.Errorf("nome = %q", got)
	}
	// Nivel vai como "Level - 1", com piso em zero.
	if resp.Level != 9 {
		t.Errorf("nivel = %d, queria 9", resp.Level)
	}
	if resp.Honra != 1234 || resp.Str != 50 {
		t.Errorf("honra/forca = %d/%d", resp.Honra, resp.Str)
	}
	if got := netsrv.CString(resp.GuildName[:]); got != "Nenhuma" {
		t.Errorf("guilda = %q, queria Nenhuma", got)
	}
}

func TestCharInfoPorNome(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)

	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	alvo := sessaoEmJogo(0, 0)
	alvo.Avatar.Name = "Procurado"
	h.Chan.Join(e.conn, a)
	id, _ := h.Chan.Join(connDescartavel(t), alvo)
	h.Chan.RegisterCharacter(id, alvo.Avatar.Name)

	// Nome em caixa diferente tambem acha.
	var p packets.RequestCharInfoPacket
	netsrv.PutCString(p.Nick[:], "pROCURADO")
	raw, err := netsrv.Marshal(CodeRequestCharInfo, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.requestCharInfo(context.Background(), e.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if got := e.espera(t); got != CodeCharInfo {
		t.Errorf("recebeu %#x, queria %#x", got, CodeCharInfo)
	}
}

// Nome que nao esta em jogo avisa, como na origem.
func TestCharInfoNomeAusenteAvisa(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	a := sessaoEmJogo(0, 0)
	h.Chan.Join(e.conn, a)

	var p packets.RequestCharInfoPacket
	netsrv.PutCString(p.Nick[:], "NaoExiste")
	raw, err := netsrv.Marshal(CodeRequestCharInfo, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.requestCharInfo(context.Background(), e.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if got := e.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x, queria a mensagem", got)
	}
}

// O equipamento do alvo vai na resposta, nos dois vetores.
func TestCharInfoLevaOEquipamento(t *testing.T) {
	h := testHandler()
	alvo := sessaoEmJogo(0, 0)
	alvo.Avatar.Name = "Alvo"
	alvo.Equip = conjuntoEquip(map[int]model.Item{
		0:  {Index: 11},
		8:  {Index: 19},
		9:  {Index: 900, Identific: 7, Min: 5, Time: 3},
		11: {Index: 111},
		15: {Index: 115},
	})

	pkt := h.buildCharInfo(alvo)
	if pkt.Equips[0].Index != 11 || pkt.Equips[8].Index != 19 {
		t.Errorf("Equips = %d/%d", pkt.Equips[0].Index, pkt.Equips[8].Index)
	}
	if pkt.Equips2[0].Index != 111 || pkt.Equips2[4].Index != 115 {
		t.Errorf("Equips2 = %d/%d", pkt.Equips2[0].Index, pkt.Equips2[4].Index)
	}
	// Montaria vem do slot 9, com a aparencia igual ao indice.
	if pkt.MountEquip.Index != 900 || pkt.MountEquip.APP != 900 {
		t.Errorf("montaria = %d/%d", pkt.MountEquip.Index, pkt.MountEquip.APP)
	}
	if pkt.MountEquip.Identific != 7 || pkt.MountEquip.MIN != 5 || pkt.MountEquip.Time != 3 {
		t.Errorf("montaria mal copiada: %+v", pkt.MountEquip)
	}
}

// Nivel zero nao vira 65535 nesta resposta.
func TestCharInfoNivelZero(t *testing.T) {
	h := testHandler()
	alvo := sessaoEmJogo(0, 0)
	alvo.Avatar.Level = 0
	if pkt := h.buildCharInfo(alvo); pkt.Level != 0 {
		t.Errorf("nivel = %d, queria 0", pkt.Level)
	}
}

// ---- hora do servidor

func TestHoraDoServidor(t *testing.T) {
	quando := time.Unix(1700000000, 0)
	h := testHandler()
	h.Now = func() time.Time { return quando }
	e := novoEspiao(t)
	s := sessaoEmJogo(0, 0)

	var p packets.SignalData
	raw, err := netsrv.Marshal(CodeRequestTime, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.requestServerTime(context.Background(), e.conn, s, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	vis := e.proximo(t)
	if vis.code != CodeRequestTime {
		t.Fatalf("recebeu %#x", vis.code)
	}
	var resp packets.SignalData
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data != 1700000000 {
		t.Errorf("hora = %d, queria 1700000000", resp.Data)
	}
}

// ---- indice de nome no canal

func TestNomeSaiDoIndiceAoDesconectar(t *testing.T) {
	h := testHandler()
	h.Chan = NewChannel(0, 10)
	e := novoEspiao(t)
	s := sessaoEmJogo(0, 0)
	s.Avatar.Name = "Fulano"
	id, _ := h.Chan.Join(e.conn, s)
	h.Chan.RegisterCharacter(id, "Fulano")

	if _, ok := h.Chan.FindByCharacter("fulano"); !ok {
		t.Fatal("deveria achar antes de sair")
	}
	h.OnDisconnect(e.conn)
	if _, ok := h.Chan.FindByCharacter("fulano"); ok {
		t.Error("nao deveria achar depois de sair")
	}
}

// Registrar um id que nao esta no canal nao cria entrada fantasma.
func TestRegistrarNomeDeIDInexistente(t *testing.T) {
	ch := NewChannel(0, 10)
	ch.RegisterCharacter(999, "Fantasma")
	if _, ok := ch.FindByCharacter("Fantasma"); ok {
		t.Error("nao deveria ter registrado")
	}
}

func TestLimitarTexto(t *testing.T) {
	if got := limitarTexto("abc", 10); got != "abc" {
		t.Errorf("= %q", got)
	}
	if got := limitarTexto("abcdef", 3); got != "abc" {
		t.Errorf("= %q", got)
	}
	// Nao parte uma runa multibyte no meio.
	if got := limitarTexto("ação", 3); got != "açã" {
		t.Errorf("= %q", got)
	}
}
