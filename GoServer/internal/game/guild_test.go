package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"aika/internal/guild"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/store"
)

// handlerComGuildas devolve um handler com canal e registro de guildas.
func handlerComGuildas(t *testing.T) *Handler {
	h := testHandler()
	h.Chan = NewChannel(0, 20)
	h.Guilds = guild.NewRegistry()
	h.Parties = NewPartyRegistry()
	h.GuildCost = 1000
	return h
}

// entraComPersonagem poe uma sessao em jogo com um id de personagem proprio.
func entraComPersonagem(t *testing.T, h *Handler, nome string, charID uint32) (*Session, *espiao) {
	t.Helper()
	e := novoEspiao(t)
	s := lutador(nome, 1000, 1000)
	s.SelectedSlot = 0
	s.Characters[0] = store.Character{ID: charID, Name: nome}
	s.Avatar.Gold = 10_000_000
	id, err := h.Chan.Join(e.conn, s)
	if err != nil {
		t.Fatal(err)
	}
	h.Chan.RegisterCharacter(id, nome)
	return s, e
}

func createGuildPacket(nome string) []byte {
	var p packets.CreateGuildDialogPacket
	netsrv.PutCString(p.GuildName[:], nome)
	buf, err := netsrv.Marshal(CodeCreateGuild, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func inviteGuildPacket(nome string) []byte {
	var p packets.InviteToGuildPacket
	netsrv.PutCString(p.InviterNick[:], nome)
	buf, err := netsrv.Marshal(CodeInviteGuild, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func charIndexPacket(code uint16, charIndex uint32) []byte {
	var p packets.ChangeMasterGuild
	p.CharIndex = charIndex
	buf, err := netsrv.Marshal(code, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func rankPacket(charIndex uint32, rank int32) []byte {
	var p packets.ChangeGuildMemberRankPacket
	p.CharIndex = int32(charIndex)
	p.Rank = rank
	buf, err := netsrv.Marshal(CodeGuildRank, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// formaGuilda cria uma guilda com o lider e adiciona os outros.
func formaGuilda(t *testing.T, h *Handler, lider *Session, liderE *espiao,
	nome string, outros ...*Session) *guild.Guild {
	t.Helper()
	ctx := context.Background()
	if err := h.createGuild(ctx, liderE.conn, lider,
		asPacket(t, createGuildPacket(nome))); err != nil {
		t.Fatal(err)
	}
	g := h.Guilds.Of(lider.CharID())
	if g == nil {
		t.Fatal("a guilda deveria existir")
	}
	for _, o := range outros {
		_, oc, _ := h.Chan.Lookup(o.ClientID)
		if err := h.inviteGuild(ctx, liderE.conn, lider,
			asPacket(t, inviteGuildPacket(o.Avatar.Name))); err != nil {
			t.Fatal(err)
		}
		if err := h.acceptGuild(ctx, oc, o,
			asPacket(t, charIndexPacket(CodeInviteGuildAccept, 0))); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

// ---- criar

func TestCriarGuildaPeloJogo(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)

	if err := h.createGuild(context.Background(), ea.conn, a,
		asPacket(t, createGuildPacket("Irmandade"))); err != nil {
		t.Fatal(err)
	}
	g := h.Guilds.Of(100)
	if g == nil {
		t.Fatal("a guilda deveria existir")
	}
	if g.Name != "Irmandade" || g.LeaderID != 100 {
		t.Errorf("= %+v", g)
	}
	if a.Avatar.GuildIndex != g.Index {
		t.Errorf("GuildIndex = %d, queria %d", a.Avatar.GuildIndex, g.Index)
	}
	// O ouro foi descontado.
	if a.Avatar.Gold != 10_000_000-1000 {
		t.Errorf("ouro = %d", a.Avatar.Gold)
	}
	// Recebe os dados e a lista.
	codigos := map[uint16]bool{}
	for range 6 {
		select {
		case p := <-ea.pacotes:
			codigos[p.code] = true
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !codigos[CodeGuildInfo] {
		t.Error("deveria receber o $965")
	}
	if !codigos[CodeGuildPlayers] {
		t.Error("deveria receber o $97F")
	}
}

func TestCriarGuildaSemOuro(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Avatar.Gold = 10

	if err := h.createGuild(context.Background(), ea.conn, a,
		asPacket(t, createGuildPacket("Irmandade"))); err != nil {
		t.Fatal(err)
	}
	if h.Guilds.Count() != 0 {
		t.Error("nao deveria ter criado")
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

func TestCriarGuildaComNomeRepetido(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	formaGuilda(t, h, a, ea, "Irmandade")
	eb.drena(100 * time.Millisecond)

	if err := h.createGuild(context.Background(), eb.conn, b,
		asPacket(t, createGuildPacket("IRMANDADE"))); err != nil {
		t.Fatal(err)
	}
	if h.Guilds.Count() != 1 {
		t.Errorf("%d guildas", h.Guilds.Count())
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

func TestCriarGuildaComNomeVazio(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)

	for _, nome := range []string{"", "   "} {
		if err := h.createGuild(context.Background(), ea.conn, a,
			asPacket(t, createGuildPacket(nome))); err != nil {
			t.Fatal(err)
		}
	}
	if h.Guilds.Count() != 0 {
		t.Error("nao deveria ter criado")
	}
}

func TestQuemTemGuildaNaoCriaOutra(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	if err := h.createGuild(context.Background(), ea.conn, a,
		asPacket(t, createGuildPacket("Outra"))); err != nil {
		t.Fatal(err)
	}
	if h.Guilds.Count() != 1 {
		t.Errorf("%d guildas", h.Guilds.Count())
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- convite

func TestConviteEAceiteDeGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	if err := h.inviteGuild(context.Background(), ea.conn, a,
		asPacket(t, inviteGuildPacket("beta"))); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeInviteGuild {
		t.Errorf("recebeu %#x, queria %#x", got, CodeInviteGuild)
	}
	if b.GuildRequester != a.ClientID {
		t.Errorf("GuildRequester = %d", b.GuildRequester)
	}

	if err := h.acceptGuild(context.Background(), eb.conn, b,
		asPacket(t, charIndexPacket(CodeInviteGuildAccept, 0))); err != nil {
		t.Fatal(err)
	}
	if g.FindMember(200) < 0 {
		t.Error("Beta deveria estar na guilda")
	}
	if b.Avatar.GuildIndex != g.Index {
		t.Errorf("GuildIndex de Beta = %d", b.Avatar.GuildIndex)
	}
	// Entra no rank mais baixo.
	if m := g.MemberAt(g.FindMember(200)); m == nil || m.Rank != guild.MaxRanks-1 {
		t.Errorf("rank = %v", m)
	}
}

// O convite vale uma vez.
func TestConviteDeGuildaValeUmaVez(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	eb.drena(100 * time.Millisecond)

	if err := h.acceptGuild(context.Background(), eb.conn, b,
		asPacket(t, charIndexPacket(CodeInviteGuildAccept, 0))); err != nil {
		t.Fatal(err)
	}
	if g.TotalMembers() != 2 {
		t.Errorf("%d membros", g.TotalMembers())
	}
}

func TestRecusarConviteDeGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	formaGuilda(t, h, a, ea, "Irmandade")
	h.inviteGuild(context.Background(), ea.conn, a,
		asPacket(t, inviteGuildPacket("beta")))
	ea.drena(100 * time.Millisecond)
	eb.drena(100 * time.Millisecond)

	if err := h.denyGuild(context.Background(), eb.conn, b,
		asPacket(t, charIndexPacket(CodeInviteGuildDeny, 0))); err != nil {
		t.Fatal(err)
	}
	if h.Guilds.Of(200) != nil {
		t.Error("nao deveria ter entrado")
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("quem convidou recebeu %#x", got)
	}
}

// Sem permissao nao convida.
func TestConvidarSemPermissao(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	entraComPersonagem(t, h, "Gama", 300)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	// O rank de Beta nao tem a permissao de convidar.
	g.RanksConfig[guild.MaxRanks-1] = 0
	eb.drena(100 * time.Millisecond)

	if err := h.inviteGuild(context.Background(), eb.conn, b,
		asPacket(t, inviteGuildPacket("gama"))); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
	if h.Guilds.Of(300) != nil {
		t.Error("Gama nao deveria ter sido convidado")
	}
}

// Com a permissao, convida.
func TestConvidarComPermissao(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	_, ec := entraComPersonagem(t, h, "Gama", 300)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	g.RanksConfig[guild.MaxRanks-1] = guild.PermInvite
	eb.drena(100 * time.Millisecond)
	ec.drena(100 * time.Millisecond)

	if err := h.inviteGuild(context.Background(), eb.conn, b,
		asPacket(t, inviteGuildPacket("gama"))); err != nil {
		t.Fatal(err)
	}
	if got := ec.espera(t); got != CodeInviteGuild {
		t.Errorf("Gama recebeu %#x", got)
	}
}

// ---- expulsar e sair

func TestExpulsarDaGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, _ := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)

	if err := h.kickGuild(context.Background(), ea.conn, a,
		asPacket(t, charIndexPacket(CodeKickGuild, 200))); err != nil {
		t.Fatal(err)
	}
	if g.FindMember(200) >= 0 {
		t.Error("deveria ter sido expulso")
	}
	if b.Avatar.GuildIndex != 0 {
		t.Errorf("GuildIndex = %d, queria 0", b.Avatar.GuildIndex)
	}
}

// O mestre nao pode ser expulso.
func TestMestreNaoEExpulso(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	g.RanksConfig[guild.MaxRanks-1] = guild.PermKick
	eb.drena(100 * time.Millisecond)

	if err := h.kickGuild(context.Background(), eb.conn, b,
		asPacket(t, charIndexPacket(CodeKickGuild, 100))); err != nil {
		t.Fatal(err)
	}
	if g.FindMember(100) < 0 {
		t.Error("o mestre deveria ter ficado")
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

func TestSairDaGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	eb.drena(100 * time.Millisecond)

	if err := h.exitGuild(context.Background(), eb.conn, b,
		asPacket(t, charIndexPacket(CodeExitGuild, 0))); err != nil {
		t.Fatal(err)
	}
	if g.FindMember(200) >= 0 {
		t.Error("deveria ter saido")
	}
	if b.Avatar.GuildIndex != 0 {
		t.Errorf("GuildIndex = %d", b.Avatar.GuildIndex)
	}
}

// O mestre saindo passa a lideranca; o ultimo saindo desfaz a guilda.
func TestMestreSaiLiderancaPassa(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, _ := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)

	if err := h.exitGuild(context.Background(), ea.conn, a,
		asPacket(t, charIndexPacket(CodeExitGuild, 0))); err != nil {
		t.Fatal(err)
	}
	if g.LeaderID != 200 {
		t.Errorf("mestre = %d, queria 200", g.LeaderID)
	}
	if h.Guilds.Count() != 1 {
		t.Errorf("%d guildas", h.Guilds.Count())
	}
}

func TestUltimoMembroDesfazAGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	if err := h.exitGuild(context.Background(), ea.conn, a,
		asPacket(t, charIndexPacket(CodeExitGuild, 0))); err != nil {
		t.Fatal(err)
	}
	if h.Guilds.Count() != 0 {
		t.Errorf("%d guildas", h.Guilds.Count())
	}
}

// ---- rank e mestre

func TestMudarRank(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, _ := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)

	if err := h.guildRank(context.Background(), ea.conn, a,
		asPacket(t, rankPacket(200, 2))); err != nil {
		t.Fatal(err)
	}
	if m := g.MemberAt(g.FindMember(200)); m == nil || m.Rank != 2 {
		t.Errorf("rank = %v", m)
	}
}

// Rank fora da faixa e recusado: na origem ele ia para o registro e dali
// indexava RanksConfig, ARRAY [0..4], fora do vetor.
func TestRankForaDaFaixaERecusado(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, _ := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)
	antes := g.MemberAt(g.FindMember(200)).Rank

	for _, rank := range []int32{-1, guild.MaxRanks, 100, 1 << 20} {
		if err := h.guildRank(context.Background(), ea.conn, a,
			asPacket(t, rankPacket(200, rank))); err != nil {
			t.Fatalf("rank %d: %v", rank, err)
		}
	}
	if got := g.MemberAt(g.FindMember(200)).Rank; got != antes {
		t.Errorf("rank mudou para %d", got)
	}
}

// So o mestre muda rank.
func TestSoMestreMudaRank(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	formaGuilda(t, h, a, ea, "Irmandade", b)
	eb.drena(100 * time.Millisecond)

	if err := h.guildRank(context.Background(), eb.conn, b,
		asPacket(t, rankPacket(200, 1))); err != nil {
		t.Fatal(err)
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

func TestPassarMestre(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, _ := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)

	if err := h.guildMaster(context.Background(), ea.conn, a,
		asPacket(t, charIndexPacket(CodeGuildMaster, 200))); err != nil {
		t.Fatal(err)
	}
	if g.LeaderID != 200 {
		t.Errorf("mestre = %d", g.LeaderID)
	}
	// O antigo mestre fica na guilda.
	if g.FindMember(100) < 0 {
		t.Error("o antigo mestre deveria ter ficado")
	}
}

func TestPassarMestreParaDeFora(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	if err := h.guildMaster(context.Background(), ea.conn, a,
		asPacket(t, charIndexPacket(CodeGuildMaster, 999))); err != nil {
		t.Fatal(err)
	}
	if g.LeaderID != 100 {
		t.Errorf("mestre = %d", g.LeaderID)
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- avisos, site, permissoes

func TestAvisosDaGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	var pkt packets.UpdateGuildNoticesPacket
	netsrv.PutCString(pkt.Notices[0][:], "primeiro")
	netsrv.PutCString(pkt.Notices[1][:], "segundo")
	netsrv.PutCString(pkt.Notices[2][:], strings.Repeat("x", 100))
	raw, err := netsrv.Marshal(CodeGuildNotices, pkt)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.guildNotices(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if g.Notices[0] != "primeiro" || g.Notices[1] != "segundo" {
		t.Errorf("avisos = %v", g.Notices)
	}
	// O terceiro foi cortado no tamanho do campo.
	if len(g.Notices[2]) != guild.NoticeLen-1 {
		t.Errorf("aviso 2 com %d caracteres", len(g.Notices[2]))
	}
}

func TestSiteDaGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	var pkt packets.UpdateGuildMessagePacket
	netsrv.PutCString(pkt.Msg[:], "site.exemplo")
	raw, _ := netsrv.Marshal(CodeGuildSite, pkt)
	if err := h.guildSite(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if g.Site != "site.exemplo" {
		t.Errorf("site = %q", g.Site)
	}
}

// A origem copiava 4 bytes num vetor de 5: o rank 4 nunca era atualizado.
func TestPermissoesCobremOsCincoRanks(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	ea.drena(100 * time.Millisecond)

	var pkt packets.GuildInfoPacket
	pkt.RanksConfig = [5]uint8{11, 22, 33, 44, 55}
	raw, _ := netsrv.Marshal(CodeGuildRanksConfig, pkt)
	if err := h.guildRanksConfig(context.Background(), ea.conn, a, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	for i, quer := range []uint8{11, 22, 33, 44, 55} {
		if g.RanksConfig[i] != quer {
			t.Errorf("rank %d = %d, queria %d", i, g.RanksConfig[i], quer)
		}
	}
}

// Sem permissao de editar, os avisos nao mudam.
func TestAvisosSemPermissao(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	g := formaGuilda(t, h, a, ea, "Irmandade", b)
	g.RanksConfig[guild.MaxRanks-1] = 0
	g.SetNotices("original", "", "")
	eb.drena(100 * time.Millisecond)

	var pkt packets.UpdateGuildNoticesPacket
	netsrv.PutCString(pkt.Notices[0][:], "hackeado")
	raw, _ := netsrv.Marshal(CodeGuildNotices, pkt)
	if err := h.guildNotices(context.Background(), eb.conn, b, asPacket(t, raw)); err != nil {
		t.Fatal(err)
	}
	if g.Notices[0] != "original" {
		t.Errorf("aviso = %q", g.Notices[0])
	}
	if got := eb.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- conversa

func TestConversaDeGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	b, eb := entraComPersonagem(t, h, "Beta", 200)
	_, ec := entraComPersonagem(t, h, "Fora", 300)
	formaGuilda(t, h, a, ea, "Irmandade", b)
	ea.drena(100 * time.Millisecond)
	eb.drena(100 * time.Millisecond)
	ec.drena(100 * time.Millisecond)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatGuild, "", "ola guilda"))); err != nil {
		t.Fatal(err)
	}
	for nome, e := range map[string]*espiao{"autor": ea, "membro": eb} {
		vis := e.proximo(t)
		if vis.code != CodeClientSay {
			t.Errorf("%s recebeu %#x", nome, vis.code)
			continue
		}
		tipo, nick, fala := leFala(t, vis.corpo)
		if tipo != ChatGuild || nick != "Alfa" || fala != "ola guilda" {
			t.Errorf("%s: %d/%q/%q", nome, tipo, nick, fala)
		}
	}
	// Quem nao e da guilda nao recebe.
	ec.nada(t, 150*time.Millisecond)
}

func TestConversaDeGuildaSemGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)

	if err := h.clientSay(context.Background(), ea.conn, a,
		asPacket(t, chatPacket(ChatGuild, "", "alguem?"))); err != nil {
		t.Fatal(err)
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- pacotes

func TestPacoteDeMembrosEmLotesDeDez(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	// Enche a guilda com 25 membros a mao.
	for i := uint32(2); i <= 25; i++ {
		g.AddMember(guild.Member{CharIndex: i * 10, Name: "M", Level: 10})
	}
	ea.drena(100 * time.Millisecond)

	h.sendGuildPlayers(ea.conn, a, g)
	// 25 membros = 3 pacotes (10, 10, 5).
	lotes := 0
	for range 5 {
		select {
		case p := <-ea.pacotes:
			if p.code == CodeGuildPlayers {
				lotes++
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if lotes != 3 {
		t.Errorf("%d lotes, queria 3", lotes)
	}
}

func TestPacoteDeInfoDaGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	g.SetNotices("aviso um", "", "")
	g.SetSite("site.exemplo")
	g.Exp = 1234
	g.Level = 3
	ea.drena(100 * time.Millisecond)

	h.sendGuildInfo(ea.conn, a, g)
	vis := ea.proximo(t)
	if vis.code != CodeGuildInfo {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.GuildInfoPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if got := netsrv.CString(pkt.GuildName[:]); got != "Irmandade" {
		t.Errorf("nome = %q", got)
	}
	if pkt.GuildIndex != g.Index || pkt.GuildIndex_1 != g.Index {
		t.Errorf("indices = %d/%d", pkt.GuildIndex, pkt.GuildIndex_1)
	}
	if got := netsrv.CString(pkt.Notices[0].Text[:]); got != "aviso um" {
		t.Errorf("aviso = %q", got)
	}
	if got := netsrv.CString(pkt.Site[:]); got != "site.exemplo" {
		t.Errorf("site = %q", got)
	}
	if pkt.Exp != 1234 || pkt.Level != 3 {
		t.Errorf("exp/nivel = %d/%d", pkt.Exp, pkt.Level)
	}
}

// ---- entrada e saida

// Personagem com guildindex no banco e sem linha em guilds_players nao esta
// em guilda: a origem mandava o pacote com o indice gravado e o cliente
// mostrava uma guilda que nao existe.
func TestGuildaInexistenteELimpa(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Avatar.GuildIndex = 42

	h.enterGuild(ea.conn, a)
	if a.Avatar.GuildIndex != 0 {
		t.Errorf("GuildIndex = %d, queria 0", a.Avatar.GuildIndex)
	}
	ea.nada(t, 150*time.Millisecond)
}

func TestEntrarNoMundoRestauraAGuilda(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)

	// Guilda vinda do banco.
	g := &guild.Guild{Index: 7, Name: "DoBanco", Level: 0, LeaderID: 100}
	g.AddMember(guild.Member{CharIndex: 100, Name: "Alfa"})
	h.Guilds.Put(g)

	h.enterGuild(ea.conn, a)
	if a.Avatar.GuildIndex != 7 || a.Avatar.GuildName != "DoBanco" {
		t.Errorf("= %d/%q", a.Avatar.GuildIndex, a.Avatar.GuildName)
	}
	if m := g.MemberAt(g.FindMember(100)); m == nil || !m.Logged {
		t.Error("o membro deveria estar marcado como online")
	}
}

// A saida marca o membro como offline sem tirar da guilda.
func TestDesconexaoMarcaOffline(t *testing.T) {
	h := handlerComGuildas(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	g := formaGuilda(t, h, a, ea, "Irmandade")
	if m := g.MemberAt(g.FindMember(100)); m == nil || !m.Logged {
		t.Fatal("deveria estar online")
	}

	h.OnDisconnect(ea.conn)
	if g.FindMember(100) < 0 {
		t.Error("a desconexao nao tira da guilda")
	}
	if m := g.MemberAt(g.FindMember(100)); m == nil || m.Logged {
		t.Error("deveria estar offline")
	}
}

// Fora de jogo os handlers recusam.
func TestGuildaExigeEstarEmJogo(t *testing.T) {
	h := handlerComGuildas(t)
	s := sessaoAutenticada()
	c := connDescartavel(t)
	ctx := context.Background()
	casos := map[string]func() error{
		"criar":    func() error { return h.createGuild(ctx, c, s, asPacket(t, createGuildPacket("X"))) },
		"convidar": func() error { return h.inviteGuild(ctx, c, s, asPacket(t, inviteGuildPacket("X"))) },
		"aceitar":  func() error { return h.acceptGuild(ctx, c, s, asPacket(t, charIndexPacket(CodeInviteGuildAccept, 0))) },
		"expulsar": func() error { return h.kickGuild(ctx, c, s, asPacket(t, charIndexPacket(CodeKickGuild, 1))) },
		"sair":     func() error { return h.exitGuild(ctx, c, s, asPacket(t, charIndexPacket(CodeExitGuild, 0))) },
		"rank":     func() error { return h.guildRank(ctx, c, s, asPacket(t, rankPacket(1, 1))) },
		"mestre":   func() error { return h.guildMaster(ctx, c, s, asPacket(t, charIndexPacket(CodeGuildMaster, 1))) },
	}
	for nome, f := range casos {
		if err := f(); err != ErrNotPlaying {
			t.Errorf("%s: erro = %v, queria ErrNotPlaying", nome, err)
		}
	}
}
