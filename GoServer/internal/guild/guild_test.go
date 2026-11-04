package guild

import (
	"strings"
	"testing"
	"time"
)

func membro(id uint32, nome string) Member {
	return Member{CharIndex: id, Name: nome, Level: 50, ClassInfo: 1}
}

// ---- capacidade

// A origem fazia "MaxGuildPlayersOfLevel[Self.Level]" com Level DWORD e o
// vetor ARRAY [0..9]: nivel acima de 9 lia fora do vetor e o limite de
// membros saia de um byte qualquer da memoria.
func TestCapacidadePorNivel(t *testing.T) {
	for nivel := uint8(0); nivel <= MaxLevel; nivel++ {
		g := &Guild{Level: nivel}
		if got := g.Capacity(); got != int(MaxPlayersOfLevel[nivel]) {
			t.Errorf("nivel %d: capacidade %d, queria %d",
				nivel, got, MaxPlayersOfLevel[nivel])
		}
	}
	// Nivel fora da faixa usa o teto, em vez de ler fora do vetor.
	for _, nivel := range []uint8{10, 50, 255} {
		g := &Guild{Level: nivel}
		if got := g.Capacity(); got != int(MaxPlayersOfLevel[MaxLevel]) {
			t.Errorf("nivel %d: capacidade %d, queria %d",
				nivel, got, MaxPlayersOfLevel[MaxLevel])
		}
	}
	var nulo *Guild
	if nulo.Capacity() != 0 {
		t.Error("guilda nil deveria ter capacidade zero")
	}
}

// O limite do nivel vale de verdade.
func TestLimiteDeMembrosPorNivel(t *testing.T) {
	g := &Guild{Level: 0} // 32 membros
	for i := uint32(1); i <= 32; i++ {
		if _, err := g.AddMember(membro(i, "M")); err != nil {
			t.Fatalf("membro %d: %v", i, err)
		}
	}
	if _, err := g.AddMember(membro(33, "M")); err != ErrGuildFull {
		t.Errorf("erro = %v, queria ErrGuildFull", err)
	}
	if g.TotalMembers() != 32 {
		t.Errorf("%d membros", g.TotalMembers())
	}
	// Subir de nivel abre espaco.
	g.Level = 1 // 48
	if _, err := g.AddMember(membro(33, "M")); err != nil {
		t.Errorf("deveria caber no nivel 1: %v", err)
	}
}

// ---- permissoes de rank

// A origem fazia "Self.RanksConfig[Rank]" com Rank Integer e o vetor
// ARRAY [0..4]: rank fora da faixa lia fora do vetor.
func TestRankConfigForaDaFaixa(t *testing.T) {
	g := &Guild{}
	g.RanksConfig[0] = PermInvite | PermKick
	if cfg := g.RankConfigOf(0); !cfg.Invite || !cfg.Kick || cfg.UseChest {
		t.Errorf("rank 0 = %+v", cfg)
	}
	for _, rank := range []uint8{MaxRanks, 10, 255} {
		if cfg := g.RankConfigOf(rank); cfg != (RankConfig{}) {
			t.Errorf("rank %d deveria dar permissao vazia: %+v", rank, cfg)
		}
	}
}

func TestBitsDePermissao(t *testing.T) {
	g := &Guild{}
	g.RanksConfig[1] = PermInvite | PermUseGuildSkill | PermOpenChest
	cfg := g.RankConfigOf(1)
	if !cfg.Invite || !cfg.UseGuildSkill || !cfg.OpenChest {
		t.Errorf("= %+v", cfg)
	}
	if cfg.Kick || cfg.EditNotices || cfg.UseChest {
		t.Errorf("permissao que nao foi dada: %+v", cfg)
	}
}

// O mestre pode tudo, independente da configuracao de rank.
func TestMestrePodeTudo(t *testing.T) {
	g := &Guild{LeaderID: 1}
	g.Members[0] = membro(1, "Mestre")
	g.RanksConfig[0] = 0 // nenhuma permissao configurada

	cfg := g.CanOf(1)
	if !cfg.Invite || !cfg.Kick || !cfg.EditNotices || !cfg.UseChest {
		t.Errorf("o mestre deveria poder tudo: %+v", cfg)
	}
	// Quem nao e da guilda nao pode nada.
	if cfg := g.CanOf(999); cfg != (RankConfig{}) {
		t.Errorf("de fora = %+v", cfg)
	}
}

// A origem fazia "Move(RanksConfig, Self.RanksConfig, 4)": copiava 4 bytes
// num vetor de 5, entao a configuracao do rank 4 NUNCA era atualizada.
func TestSetRanksConfigCobreOsCincoRanks(t *testing.T) {
	g := &Guild{}
	g.SetRanksConfig([]uint8{1, 2, 3, 4, 5})
	for i, quer := range []uint8{1, 2, 3, 4, 5} {
		if g.RanksConfig[i] != quer {
			t.Errorf("rank %d = %d, queria %d", i, g.RanksConfig[i], quer)
		}
	}
	// Lista curta so mexe no que veio.
	g.SetRanksConfig([]uint8{9})
	if g.RanksConfig[0] != 9 || g.RanksConfig[4] != 5 {
		t.Errorf("= %v", g.RanksConfig)
	}
	// Lista maior nao estoura.
	g.SetRanksConfig([]uint8{1, 2, 3, 4, 5, 6, 7, 8})
	if g.RanksConfig[4] != 5 {
		t.Errorf("rank 4 = %d, queria 5", g.RanksConfig[4])
	}
}

// ---- membros

// A origem fazia "Self.Members[Self.FindMemberFromCharIndex(CharIndex)]"
// direto, e FindMemberFromCharIndex devolve -1 quando nao acha: aquilo lia
// Members[-1], antes do inicio do vetor.
func TestMemberAtForaDaFaixa(t *testing.T) {
	g := &Guild{}
	g.Members[0] = membro(1, "Alfa")

	if m := g.MemberAt(0); m == nil || m.CharIndex != 1 {
		t.Error("slot 0 deveria ter o membro")
	}
	for _, slot := range []int{-1, -100, MaxMembers, 1000} {
		if m := g.MemberAt(slot); m != nil {
			t.Errorf("slot %d deveria ser nil", slot)
		}
	}
	// Slot vazio tambem e nil.
	if m := g.MemberAt(5); m != nil {
		t.Error("slot vazio deveria ser nil")
	}
	// E o caminho que a origem usava: FindMember devolve -1 e MemberAt o
	// recusa em vez de ler antes do vetor.
	if m := g.MemberAt(g.FindMember(999)); m != nil {
		t.Error("membro inexistente deveria dar nil")
	}
}

func TestFindMember(t *testing.T) {
	g := &Guild{}
	g.Members[3] = membro(42, "Alfa")
	if got := g.FindMember(42); got != 3 {
		t.Errorf("= %d, queria 3", got)
	}
	if got := g.FindMember(0); got != -1 {
		t.Errorf("CharIndex zero = %d, queria -1", got)
	}
	if got := g.FindMember(999); got != -1 {
		t.Errorf("= %d, queria -1", got)
	}
}

// Nome longo e cortado: a origem usava Move com o tamanho do texto de
// ORIGEM, e um nome de mais de 20 caracteres passava por cima de Rank,
// ClassInfo, Level e Logged no registro do membro.
func TestNomeLongoECortado(t *testing.T) {
	g := &Guild{Level: 0}
	longo := strings.Repeat("x", 50)
	slot, err := g.AddMember(membro(1, longo))
	if err != nil {
		t.Fatal(err)
	}
	m := g.MemberAt(slot)
	if len(m.Name) != MemberNameLen-1 {
		t.Errorf("nome com %d caracteres, queria %d", len(m.Name), MemberNameLen-1)
	}
	// O rank nao foi tocado.
	if m.Rank != 0 || m.Level != 50 || m.ClassInfo != 1 {
		t.Errorf("campos vizinhos mexidos: %+v", m)
	}
}

func TestAddMemberRecusas(t *testing.T) {
	g := &Guild{Level: 0}
	if _, err := g.AddMember(membro(0, "Zero")); err != ErrMemberNotFound {
		t.Errorf("CharIndex zero: erro = %v", err)
	}
	g.AddMember(membro(1, "Alfa"))
	if _, err := g.AddMember(membro(1, "Alfa")); err != ErrAlreadyInGuild {
		t.Errorf("repetido: erro = %v", err)
	}
	var nulo *Guild
	if _, err := nulo.AddMember(membro(1, "Alfa")); err != ErrNoGuild {
		t.Errorf("guilda nil: erro = %v", err)
	}
}

func TestRemoveMember(t *testing.T) {
	g := &Guild{Level: 0}
	g.AddMember(membro(1, "Alfa"))
	g.AddMember(membro(2, "Beta"))

	if !g.RemoveMember(1) {
		t.Error("deveria ter removido")
	}
	if g.RemoveMember(1) {
		t.Error("nao deveria remover duas vezes")
	}
	if g.TotalMembers() != 1 {
		t.Errorf("%d membros", g.TotalMembers())
	}
	// O slot liberado e reusado.
	slot, err := g.AddMember(membro(3, "Gama"))
	if err != nil || slot != 0 {
		t.Errorf("slot = %d, err = %v", slot, err)
	}
}

// Rank fora da faixa e recusado: gravado no registro, ele indexaria
// RanksConfig fora do vetor na proxima checagem.
func TestSetRankForaDaFaixa(t *testing.T) {
	g := &Guild{Level: 0}
	g.AddMember(membro(1, "Alfa"))

	if err := g.SetRank(1, 3); err != nil {
		t.Fatal(err)
	}
	if g.MemberAt(0).Rank != 3 {
		t.Errorf("rank = %d", g.MemberAt(0).Rank)
	}
	for _, rank := range []uint8{MaxRanks, 10, 255} {
		if err := g.SetRank(1, rank); err == nil {
			t.Errorf("rank %d deveria ser recusado", rank)
		}
	}
	if g.MemberAt(0).Rank != 3 {
		t.Errorf("o rank mudou para %d", g.MemberAt(0).Rank)
	}
	if err := g.SetRank(999, 1); err != ErrMemberNotFound {
		t.Errorf("membro inexistente: erro = %v", err)
	}
}

func TestSetLeader(t *testing.T) {
	g := &Guild{Level: 0, LeaderID: 1}
	g.AddMember(membro(1, "Mestre"))
	g.AddMember(membro(2, "Beta"))

	if err := g.SetLeader(2); err != nil {
		t.Fatal(err)
	}
	if g.LeaderID != 2 {
		t.Errorf("mestre = %d", g.LeaderID)
	}
	if g.MemberAt(g.FindMember(2)).Rank != RankLeader {
		t.Error("o mestre novo deveria estar no rank 0")
	}
	// O antigo mestre fica na guilda, no rank abaixo.
	antigo := g.MemberAt(g.FindMember(1))
	if antigo == nil {
		t.Fatal("o antigo mestre deveria ter ficado")
	}
	if antigo.Rank != 1 {
		t.Errorf("rank do antigo = %d, queria 1", antigo.Rank)
	}
	if err := g.SetLeader(999); err != ErrMemberNotFound {
		t.Errorf("erro = %v", err)
	}
}

func TestAvisosESiteSaoCortados(t *testing.T) {
	g := &Guild{}
	g.SetNotices(strings.Repeat("a", 100), "curto", strings.Repeat("b", 100))
	if len(g.Notices[0]) != NoticeLen-1 {
		t.Errorf("aviso 0 com %d", len(g.Notices[0]))
	}
	if g.Notices[1] != "curto" {
		t.Errorf("aviso 1 = %q", g.Notices[1])
	}
	g.SetSite(strings.Repeat("c", 100))
	if len(g.Site) != SiteLen-1 {
		t.Errorf("site com %d", len(g.Site))
	}
	// Lista curta nao mexe no resto.
	g.SetNotices("novo")
	if g.Notices[1] != "curto" {
		t.Errorf("aviso 1 mudou para %q", g.Notices[1])
	}
}

func TestLoggedMembers(t *testing.T) {
	g := &Guild{Level: 0}
	m1 := membro(1, "Alfa")
	m1.Logged = true
	g.AddMember(m1)
	g.AddMember(membro(2, "Beta"))

	online := g.LoggedMembers()
	if len(online) != 1 || online[0] != 1 {
		t.Errorf("= %v", online)
	}
}

// ---- registro

func TestCriarGuilda(t *testing.T) {
	r := NewRegistry()
	g, err := r.Create("Teste", 1, membro(10, "Mestre"))
	if err != nil {
		t.Fatal(err)
	}
	if g.Index == 0 {
		t.Error("o indice zero significa 'sem guilda'")
	}
	if g.LeaderID != 10 || g.Name != "Teste" || g.Nation != 1 {
		t.Errorf("= %+v", g)
	}
	if g.MemberAt(0).Rank != RankLeader {
		t.Error("o mestre deveria estar no rank 0")
	}
	// O mestre entra com todas as permissoes.
	if cfg := g.RankConfigOf(RankLeader); !cfg.Invite || !cfg.Kick {
		t.Errorf("permissoes do rank 0 = %+v", cfg)
	}
	if r.Of(10) != g || r.Get(g.Index) != g || r.ByName("teste") != g {
		t.Error("os indices do registro deveriam achar a guilda")
	}
}

func TestCriarRecusas(t *testing.T) {
	r := NewRegistry()
	r.Create("Teste", 1, membro(10, "Mestre"))

	if _, err := r.Create("TESTE", 1, membro(20, "Outro")); err != ErrNameTaken {
		t.Errorf("nome repetido: erro = %v", err)
	}
	if _, err := r.Create("Outra", 1, membro(10, "Mestre")); err != ErrAlreadyInGuild {
		t.Errorf("mestre em duas: erro = %v", err)
	}
	for _, nome := range []string{"", "   ", strings.Repeat("x", NameLen)} {
		if _, err := r.Create(nome, 1, membro(30, "X")); err != ErrInvalidName {
			t.Errorf("nome %q: erro = %v", nome, err)
		}
	}
	if _, err := r.Create("Nova", 1, membro(0, "Zero")); err != ErrMemberNotFound {
		t.Errorf("erro = %v", err)
	}
}

func TestJoinELeave(t *testing.T) {
	r := NewRegistry()
	g, _ := r.Create("Teste", 1, membro(10, "Mestre"))
	if _, err := r.Join(g, membro(20, "Beta")); err != nil {
		t.Fatal(err)
	}
	if r.Of(20) != g {
		t.Error("o membro novo deveria estar no indice")
	}
	// Entrar duas vezes nao.
	if _, err := r.Join(g, membro(20, "Beta")); err != ErrAlreadyInGuild {
		t.Errorf("erro = %v", err)
	}

	restante, desfeita := r.Leave(20)
	if desfeita || restante != g {
		t.Errorf("= %v/%v", restante, desfeita)
	}
	if r.Of(20) != nil {
		t.Error("quem saiu nao deveria estar no indice")
	}
}

// O mestre saindo passa a lideranca; o ultimo membro saindo desfaz a guilda.
func TestLiderancaPassaEGuildaSeDesfaz(t *testing.T) {
	r := NewRegistry()
	g, _ := r.Create("Teste", 1, membro(10, "Mestre"))
	r.Join(g, membro(20, "Beta"))

	restante, desfeita := r.Leave(10)
	if desfeita {
		t.Fatal("com um membro sobrando a guilda nao se desfaz")
	}
	if restante.LeaderID != 20 {
		t.Errorf("mestre novo = %d, queria 20", restante.LeaderID)
	}
	if restante.MemberAt(restante.FindMember(20)).Rank != RankLeader {
		t.Error("o mestre novo deveria estar no rank 0")
	}

	_, desfeita = r.Leave(20)
	if !desfeita {
		t.Error("o ultimo membro saindo deveria desfazer a guilda")
	}
	if r.Count() != 0 {
		t.Errorf("%d guildas", r.Count())
	}
	if r.ByName("Teste") != nil {
		t.Error("o nome deveria ter sido liberado")
	}
}

func TestDestroy(t *testing.T) {
	r := NewRegistry()
	g, _ := r.Create("Teste", 1, membro(10, "Mestre"))
	r.Join(g, membro(20, "Beta"))

	soltos := r.Destroy(g)
	if len(soltos) != 2 {
		t.Errorf("%d soltos: %v", len(soltos), soltos)
	}
	if r.Count() != 0 || r.Of(10) != nil || r.Of(20) != nil {
		t.Error("o registro deveria estar limpo")
	}
	// Desfazer duas vezes nao quebra.
	if r.Destroy(g) != nil {
		t.Error("a segunda vez deveria devolver nil")
	}
}

func TestPutDoBanco(t *testing.T) {
	r := NewRegistry()
	g := &Guild{Index: 42, Name: "DoBanco", Level: 0, LeaderID: 7}
	g.Members[0] = membro(7, "Mestre")
	g.Members[1] = membro(8, "Beta")
	r.Put(g)

	if r.Get(42) != g || r.Of(7) != g || r.Of(8) != g || r.ByName("dobanco") != g {
		t.Error("Put deveria registrar a guilda e os membros")
	}
	// Uma guilda nova nao reaproveita o indice 42.
	nova, err := r.Create("Nova", 1, membro(9, "X"))
	if err != nil {
		t.Fatal(err)
	}
	if nova.Index == 42 {
		t.Error("o indice 42 nao deveria ser reusado")
	}
}

func TestRegistroNilNaoQuebra(t *testing.T) {
	var r *Registry
	if r.Count() != 0 || r.Get(1) != nil || r.Of(1) != nil || r.ByName("x") != nil {
		t.Error("registro nil deveria responder vazio")
	}
	if _, err := r.Create("X", 1, membro(1, "A")); err == nil {
		t.Error("Create em nil deveria falhar")
	}
	if _, err := r.Join(nil, membro(1, "A")); err == nil {
		t.Error("Join em nil deveria falhar")
	}
	if g, d := r.Leave(1); g != nil || d {
		t.Error("Leave em nil deveria responder vazio")
	}
	if r.Destroy(nil) != nil || r.All() != nil {
		t.Error("Destroy/All em nil deveriam ser vazios")
	}
	r.Put(nil)
}

func TestRegistroConcorrente(t *testing.T) {
	r := NewRegistry()
	const n = 20
	pronto := make(chan struct{}, n)
	for i := range n {
		go func(base uint32) {
			for j := range 20 {
				nome := "G" + string(rune('A'+base)) + string(rune('a'+j))
				g, err := r.Create(nome, 1, membro(base*1000+uint32(j)*10+1, "M"))
				if err != nil {
					continue
				}
				r.Join(g, membro(base*1000+uint32(j)*10+2, "B"))
				r.Of(base*1000 + uint32(j)*10 + 1)
				r.All()
				r.Leave(base*1000 + uint32(j)*10 + 2)
				r.Destroy(g)
			}
			pronto <- struct{}{}
		}(uint32(i))
	}
	for range n {
		<-pronto
	}
	if r.Count() != 0 {
		t.Errorf("%d guildas sobraram", r.Count())
	}
}

func TestTotalMembersContaOVetor(t *testing.T) {
	g := &Guild{Level: 0}
	if g.TotalMembers() != 0 {
		t.Error("guilda nova deveria estar vazia")
	}
	for i := uint32(1); i <= 5; i++ {
		g.AddMember(membro(i, "M"))
	}
	g.RemoveMember(3)
	if g.TotalMembers() != 4 {
		t.Errorf("%d membros, queria 4", g.TotalMembers())
	}
	var nulo *Guild
	if nulo.TotalMembers() != 0 {
		t.Error("nil deveria contar zero")
	}
}

func TestLastLoginGuardado(t *testing.T) {
	g := &Guild{Level: 0}
	quando := time.Unix(1700000000, 0)
	m := membro(1, "Alfa")
	m.LastLogin = quando
	g.AddMember(m)
	if !g.MemberAt(0).LastLogin.Equal(quando) {
		t.Errorf("= %v", g.MemberAt(0).LastLogin)
	}
}
