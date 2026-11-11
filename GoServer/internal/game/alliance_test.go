package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/guild"
	"aika/internal/nation"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// legiaoDeTeste e uma guilda com o lider em jogo.
type guildaDeTeste struct {
	g  *guild.Guild
	s  *Session
	e  *espiao
	id uint32
}

func (c *cenarioNacao) novaGuilda(t *testing.T, nome string, nacao uint16, charID uint32) *guildaDeTeste {
	t.Helper()
	s, e := entraComPersonagem(t, c.h, "L"+nome, charID)
	s.Status = Playing
	s.Avatar.Nation = nacao
	g, err := c.h.Guilds.Create(nome, uint8(nacao), guild.Member{CharIndex: charID, Name: "L" + nome, Logged: true})
	if err != nil {
		t.Fatal(err)
	}
	s.Avatar.GuildIndex = g.Index
	return &guildaDeTeste{g: g, s: s, e: e, id: charID}
}

func (c *cenarioNacao) pedeAlianca(t *testing.T, de *Session, e *espiao, nome string) {
	t.Helper()
	var p packets.GuildRequestAllyPacket
	netsrv.PutCString(p.GuildName[:], nome)
	if err := c.h.requestGuildAlly(context.Background(), e.conn, de,
		asPacket(t, miscPacket(CodeGuildAllyRequest, p))); err != nil {
		t.Fatal(err)
	}
}

func (c *cenarioNacao) respondeAlianca(t *testing.T, s *Session, e *espiao, aceita bool) {
	t.Helper()
	acc := int32(BoolRefuse)
	if aceita {
		acc = BoolAccept
	}
	if err := c.h.acceptRaid(context.Background(), e.conn, s,
		asPacket(t, miscPacket(CodeAcceptRaid, packets.InviteToRaidResponse{Accept: acc}))); err != nil {
		t.Fatal(err)
	}
}

// aliada monta Alfa (a guilda do cenario, lider 11) e Beta (nacao 1, lider 21).
func (c *cenarioNacao) alianca(t *testing.T) *guildaDeTeste {
	t.Helper()
	return c.novaGuilda(t, "Beta", 1, 21)
}

func TestPedidoDeAliancaConvidaOLiderDaOutraGuilda(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	c.eLider.drena(5 * time.Millisecond)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")

	inv := decodifica[packets.SendInviteToRaid](t, proximoDe(t, beta.e, CodeSendRaid))
	if inv.SendTo != uint32(c.lider.ClientID) {
		t.Errorf("convite vem de %d, queria %d", inv.SendTo, c.lider.ClientID)
	}
	if !beta.s.AllyByLegion || beta.s.AllyRequester != c.lider.ClientID {
		t.Error("o lider convidado deveria ficar com o convite pendente")
	}
}

func TestPedidoDeAliancaRecusas(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	gama := c.novaGuilda(t, "Gama", 2, 31)
	_ = gama

	casos := []struct {
		nome   string
		quem   *Session
		e      *espiao
		alvo   string
		espera string
		antes  func()
	}{
		{"membro comum", c.membro, c.eMembro, "Beta", "Você não é o líder da guild.", nil},
		{"guilda inexistente", c.lider, c.eLider, "Zeta", "Erro de busca por guild para inclusão na aliança. Ticket pro suporte.", nil},
		{"outra nacao", c.lider, c.eLider, "Gama", "A guild escolhida é de outra nação.", nil},
		{"mesma guilda", c.lider, c.eLider, "Alfa", "Você não pode adicionar a própia legião na aliança.", nil},
		{"lider offline", c.lider, c.eLider, "Beta", "O lider da guild escolhida está offline.", func() { beta.s.Status = CharList }},
	}
	for _, k := range casos {
		if k.antes != nil {
			k.antes()
		}
		k.e.drena(5 * time.Millisecond)
		c.pedeAlianca(t, k.quem, k.e, k.alvo)
		if got := mensagemDe(t, k.e); got != k.espera {
			t.Errorf("%s: mensagem = %q, queria %q", k.nome, got, k.espera)
		}
	}
	if beta.s.AllyByLegion {
		t.Error("nenhuma recusa deveria deixar convite pendente")
	}
}

func TestSoOLiderDaAliancaConvida(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	gama := c.novaGuilda(t, "Gama", 1, 31)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	c.respondeAlianca(t, beta.s, beta.e, true)

	// Beta e aliada, nao lider: nao pode chamar a Gama.
	beta.e.drena(5 * time.Millisecond)
	c.pedeAlianca(t, beta.s, beta.e, "Gama")
	if got := mensagemDe(t, beta.e); got != "Você não é o líder da aliança" {
		t.Errorf("mensagem = %q", got)
	}
	if gama.s.AllyByLegion {
		t.Error("a Gama nao deveria ter convite")
	}
}

func TestAceitarEntraNaAliancaECopiaAListaParaTodas(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	gama := c.novaGuilda(t, "Gama", 1, 31)

	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	c.eLider.drena(5 * time.Millisecond)
	c.respondeAlianca(t, beta.s, beta.e, true)

	if c.alfa.Ally.Guilds[1].Index != beta.g.Index || c.alfa.Ally.Guilds[1].Name != "Beta" {
		t.Fatalf("Alfa.Ally = %+v", c.alfa.Ally)
	}
	if beta.g.Ally != c.alfa.Ally || beta.g.Ally.Leader != c.alfa.Index {
		t.Errorf("a Beta deveria ter a copia da aliança: %+v", beta.g.Ally)
	}
	// Os membros logados das duas guildas recebem o aviso e a guilda com a lista.
	if got := mensagemDe(t, c.eLider); got != "Sua aliança teve mudanças." {
		t.Errorf("aviso da lider = %q", got)
	}
	info := decodifica[packets.GuildInfoPacket](t, proximoDe(t, beta.e, CodeGuildInfo))
	if info.GuildsAlly[0].Index != c.alfa.Index || info.GuildsAlly[1].Index != beta.g.Index ||
		netsrv.CString(info.GuildsAlly[1].Name[:]) != "Beta" {
		t.Errorf("GuildsAlly = %+v", info.GuildsAlly)
	}

	// Terceira guilda: as tres ficam com a mesma lista.
	c.pedeAlianca(t, c.lider, c.eLider, "Gama")
	c.respondeAlianca(t, gama.s, gama.e, true)
	if c.alfa.Ally.Guilds[2].Index != gama.g.Index {
		t.Fatalf("Alfa.Ally = %+v", c.alfa.Ally)
	}
	if beta.g.Ally != c.alfa.Ally || gama.g.Ally != c.alfa.Ally {
		t.Error("as tres guildas deveriam ter a mesma lista")
	}
	if got := mensagemDe(t, beta.e); got != "Sua aliança teve mudanças." {
		t.Errorf("a Beta deveria saber da Gama: %q", got)
	}
}

func TestRecusarAliancaAvisaOLiderENaoMuda(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	c.eLider.drena(5 * time.Millisecond)
	c.respondeAlianca(t, beta.s, beta.e, false)
	if got := mensagemDe(t, c.eLider); got != "O lider da guild escolhida recusou-se a entrar na aliança." {
		t.Errorf("mensagem = %q", got)
	}
	if c.alfa.AllyCount() != 1 || beta.g.AllyCount() != 1 || beta.s.AllyByLegion {
		t.Error("recusar nao deveria mudar nada")
	}
}

func TestConviteDeAliancaVelhoNaoValeMaisNada(t *testing.T) {
	c := novoCenarioNacao(t)
	beta := c.alianca(t)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	// O convidador perde a lideranca antes da resposta.
	c.alfa.LeaderID = 12
	c.eLider.drena(5 * time.Millisecond)
	c.respondeAlianca(t, beta.s, beta.e, true)
	if c.alfa.AllyCount() != 1 || beta.g.InAlliance() {
		t.Error("o convite envelhecido nao deveria criar aliança")
	}
	if got := mensagemDe(t, c.eLider); got != "Não foi possível criar a aliança." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestAliancaCheiaRecusa(t *testing.T) {
	c := novoCenarioNacao(t)
	var gs []*guildaDeTeste
	for i, n := range []string{"B1", "B2", "B3", "B4"} {
		gs = append(gs, c.novaGuilda(t, n, 1, uint32(40+i)))
	}
	for _, b := range gs[:3] {
		c.pedeAlianca(t, c.lider, c.eLider, b.g.Name)
		c.respondeAlianca(t, b.s, b.e, true)
	}
	if c.alfa.AllyCount() != 4 {
		t.Fatalf("aliança = %d guildas", c.alfa.AllyCount())
	}
	c.eLider.drena(10 * time.Millisecond)
	c.pedeAlianca(t, c.lider, c.eLider, "B4")
	if got := mensagemDe(t, c.eLider); got != "Não há mais espaço na aliança." {
		t.Errorf("mensagem = %q", got)
	}
	// Guilda que ja esta em outra aliança tambem e recusada.
	c.eLider.drena(10 * time.Millisecond)
	c.pedeAlianca(t, c.lider, c.eLider, "B1")
	if got := mensagemDe(t, c.eLider); got != "A guild já está em uma aliança." && got != "Não há mais espaço na aliança." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestAliancaDoMarechalPreencheAGrade(t *testing.T) {
	c := novoCenarioNacao(t) // Alfa e a guilda marechal da nacao 1
	nomes := []string{"B1", "B2", "B3"}
	var gs []*guildaDeTeste
	for i, n := range nomes {
		gs = append(gs, c.novaGuilda(t, n, 1, uint32(50+i)))
	}
	for _, b := range gs {
		c.pedeAlianca(t, c.lider, c.eLider, b.g.Name)
		c.respondeAlianca(t, b.s, b.e, true)
	}
	st, _ := c.h.Nations.Get(1)
	if st.TacticianGuild != gs[0].g.Index || st.JudgeGuild != gs[1].g.Index || st.TreasurerGuild != gs[2].g.Index {
		t.Errorf("grade = %d/%d/%d", st.TacticianGuild, st.JudgeGuild, st.TreasurerGuild)
	}
	if len(c.db.grades) != 3 {
		t.Errorf("a grade deveria ser gravada a cada cargo novo: %d gravacoes", len(c.db.grades))
	}
	// O lider da guilda nova ganha o buff de archon e recebe a nacao.
	if !gs[0].s.Base.Buffs.Has(buffArchon) {
		t.Error("o lider da guilda estrategista deveria ter o buff 6602")
	}
	proximoDe(t, gs[0].e, CodeNationGrade)
}

func TestAliancaForaDoMarechalNaoMexeNaGrade(t *testing.T) {
	c := novoCenarioNacao(t)
	c.h.Nations.Update(1, func(s *nation.State) { s.MarshalGuild = 999 })
	beta := c.alianca(t)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	c.respondeAlianca(t, beta.s, beta.e, true)
	if st, _ := c.h.Nations.Get(1); st.TacticianGuild != 0 || len(c.db.grades) != 0 {
		t.Error("so a aliança do marechal preenche a grade")
	}
}

// ---- desfazer, retirar e sair

func (c *cenarioNacao) aliancaDeTres(t *testing.T) (*guildaDeTeste, *guildaDeTeste) {
	t.Helper()
	beta := c.alianca(t)
	gama := c.novaGuilda(t, "Gama", 1, 31)
	c.pedeAlianca(t, c.lider, c.eLider, "Beta")
	c.respondeAlianca(t, beta.s, beta.e, true)
	c.pedeAlianca(t, c.lider, c.eLider, "Gama")
	c.respondeAlianca(t, gama.s, gama.e, true)
	for _, e := range []*espiao{c.eLider, beta.e, gama.e} {
		e.drena(5 * time.Millisecond)
	}
	return beta, gama
}

func TestDesfazerAliancaSoPeloLider(t *testing.T) {
	c := novoCenarioNacao(t)
	beta, gama := c.aliancaDeTres(t)
	ctx := context.Background()
	if c.h.destroyAlliance(ctx, c.membro) {
		t.Error("membro comum nao desfaz")
	}
	if c.h.destroyAlliance(ctx, beta.s) {
		t.Error("lider de uma aliada nao desfaz a aliança dos outros")
	}
	beta.e.drena(5 * time.Millisecond)
	if !c.h.destroyAlliance(ctx, c.lider) {
		t.Fatal("a lider deveria conseguir")
	}
	for _, g := range []*guild.Guild{c.alfa, beta.g, gama.g} {
		if g.InAlliance() {
			t.Errorf("%s deveria estar sozinha", g.Name)
		}
	}
	if got := mensagemDe(t, beta.e); got != "Sua aliança teve mudanças." {
		t.Errorf("aviso da Beta = %q", got)
	}
}

func TestRetirarAliadaTiraOCargoDeNacao(t *testing.T) {
	c := novoCenarioNacao(t)
	beta, gama := c.aliancaDeTres(t)
	c.h.Nations.Update(1, func(n *nation.State) { n.JudgeGuild = beta.g.Index })

	if !c.h.kickAlly(context.Background(), c.lider, 1) {
		t.Fatal("a lider deveria conseguir retirar")
	}
	if beta.g.InAlliance() || c.alfa.AllyCount() != 2 || gama.g.Ally != c.alfa.Ally {
		t.Errorf("estado da aliança: alfa=%+v gama=%+v", c.alfa.Ally, gama.g.Ally)
	}
	if st, _ := c.h.Nations.Get(1); st.JudgeGuild != 0 {
		t.Error("a Beta retirada deveria perder o cargo de juiz")
	}
	if c.h.kickAlly(context.Background(), c.lider, 1) {
		t.Error("slot ja vazio")
	}
}

func TestSairDaAliancaERecusaALider(t *testing.T) {
	c := novoCenarioNacao(t)
	beta, _ := c.aliancaDeTres(t)
	ctx := context.Background()
	if c.h.exitAlliance(ctx, c.lider) {
		t.Error("a lider nao sai: ela desfaz")
	}
	if got := mensagemDe(t, c.eLider); got != msgAllyLeaderOut {
		t.Errorf("mensagem = %q", got)
	}
	if !c.h.exitAlliance(ctx, beta.s) {
		t.Fatal("a aliada deveria conseguir sair")
	}
	if beta.g.InAlliance() || c.alfa.AllyCount() != 2 {
		t.Errorf("Beta=%v alfa=%d", beta.g.InAlliance(), c.alfa.AllyCount())
	}
}
