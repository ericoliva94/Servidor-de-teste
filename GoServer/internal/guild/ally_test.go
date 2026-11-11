package guild

import (
	"errors"
	"testing"
)

func novaGuildaDeTeste(t *testing.T, r *Registry, nome string, nacao uint8, lider uint32) *Guild {
	t.Helper()
	g, err := r.Create(nome, nacao, Member{CharIndex: lider, Name: "L" + nome})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGuildaNovaFicaSozinhaNaPropriaAlianca(t *testing.T) {
	r := NewRegistry()
	g := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	if g.Ally.Leader != g.Index || g.Ally.Guilds[0].Index != g.Index || g.Ally.Guilds[0].Name != "Alfa" {
		t.Errorf("Ally = %+v", g.Ally)
	}
	if g.AllyCount() != 1 || g.InAlliance() {
		t.Error("guilda nova nao esta em aliança")
	}
}

func TestGuildaDoBancoSemAliancaVoltaSozinha(t *testing.T) {
	r := NewRegistry()
	g := &Guild{Index: 9, Name: "Velha", Nation: 1}
	r.Put(g)
	if g.Ally.Leader != 9 || g.Ally.Guilds[0].Index != 9 {
		t.Errorf("Ally = %+v", g.Ally)
	}
	// Quem veio do banco com aliança mantem a sua.
	h := &Guild{Index: 10, Name: "Com", Nation: 1}
	h.Ally.Leader = 3
	h.Ally.Guilds[0].Index = 3
	r.Put(h)
	if h.Ally.Leader != 3 {
		t.Errorf("aliança do banco perdida: %+v", h.Ally)
	}
}

func TestEntradaNaAliancaRegras(t *testing.T) {
	r := NewRegistry()
	a := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	b := novaGuildaDeTeste(t, r, "Beta", 1, 2)
	c := novaGuildaDeTeste(t, r, "Gama", 2, 3)
	d := novaGuildaDeTeste(t, r, "Delta", 1, 4)

	if _, err := r.JoinAlliance(a, a); !errors.Is(err, ErrAllySelf) {
		t.Errorf("a si mesma: %v", err)
	}
	if _, err := r.JoinAlliance(a, c); !errors.Is(err, ErrAllyNation) {
		t.Errorf("outra nacao: %v", err)
	}
	if _, err := r.JoinAlliance(a, b); err != nil {
		t.Fatal(err)
	}
	// Beta e aliada: nao lidera, nao pode ser chamada de novo nem chamar.
	if _, err := r.JoinAlliance(b, d); !errors.Is(err, ErrNotAllyLeader) {
		t.Errorf("aliada convidando: %v", err)
	}
	if _, err := r.JoinAlliance(d, b); !errors.Is(err, ErrAlreadyAllied) {
		t.Errorf("ja aliada: %v", err)
	}
}

func TestAliancaTemNoMaximoQuatroGuildas(t *testing.T) {
	r := NewRegistry()
	a := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	var outras []*Guild
	for i, n := range []string{"B", "C", "D", "E"} {
		outras = append(outras, novaGuildaDeTeste(t, r, n, 1, uint32(10+i)))
	}
	for _, o := range outras[:3] {
		if _, err := r.JoinAlliance(a, o); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.JoinAlliance(a, outras[3]); !errors.Is(err, ErrAllyFull) {
		t.Errorf("quinta guilda: %v", err)
	}
	if a.AllyCount() != 4 || outras[3].InAlliance() {
		t.Error("a quinta nao deveria entrar")
	}
}

func TestListaCopiadaParaTodasAsGuildas(t *testing.T) {
	r := NewRegistry()
	a := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	b := novaGuildaDeTeste(t, r, "Beta", 1, 2)
	c := novaGuildaDeTeste(t, r, "Gama", 1, 3)
	if _, err := r.JoinAlliance(a, b); err != nil {
		t.Fatal(err)
	}
	mudou, err := r.JoinAlliance(a, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(mudou) != 3 {
		t.Errorf("guildas alteradas = %d, queria 3", len(mudou))
	}
	if b.Ally != a.Ally || c.Ally != a.Ally {
		t.Error("todas deveriam ter a lista da lider")
	}
}

func TestGuildaDesfeitaSaiDaAlianca(t *testing.T) {
	r := NewRegistry()
	a := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	b := novaGuildaDeTeste(t, r, "Beta", 1, 2)
	c := novaGuildaDeTeste(t, r, "Gama", 1, 3)
	_, _ = r.JoinAlliance(a, b)
	_, _ = r.JoinAlliance(a, c)

	// Aliada some: o slot dela e limpo em todas as copias.
	r.Destroy(b)
	for _, g := range []*Guild{a, c} {
		for _, it := range g.Ally.Guilds {
			if it.Index == b.Index {
				t.Errorf("%s ainda lista a guilda desfeita", g.Name)
			}
		}
	}
	if a.AllyCount() != 2 {
		t.Errorf("aliança = %d guildas, queria 2", a.AllyCount())
	}
	// Lider some: a aliança acaba.
	r.Destroy(a)
	if c.InAlliance() || c.Ally.Leader != c.Index {
		t.Errorf("a Gama deveria ficar sozinha: %+v", c.Ally)
	}
}

func aliancaDeTres(t *testing.T) (*Registry, *Guild, *Guild, *Guild) {
	t.Helper()
	r := NewRegistry()
	a := novaGuildaDeTeste(t, r, "Alfa", 1, 1)
	b := novaGuildaDeTeste(t, r, "Beta", 1, 2)
	c := novaGuildaDeTeste(t, r, "Gama", 1, 3)
	if _, err := r.JoinAlliance(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.JoinAlliance(a, c); err != nil {
		t.Fatal(err)
	}
	return r, a, b, c
}

func TestDesfazerAlianca(t *testing.T) {
	r, a, b, c := aliancaDeTres(t)
	if _, err := r.DisbandAlliance(b); !errors.Is(err, ErrNotAllyLeader) {
		t.Errorf("aliada desfazendo: %v", err)
	}
	mudou, err := r.DisbandAlliance(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(mudou) != 3 {
		t.Errorf("alteradas = %d, queria 3", len(mudou))
	}
	for _, g := range []*Guild{a, b, c} {
		if g.InAlliance() || g.AllyCount() != 1 || g.Ally.Leader != g.Index {
			t.Errorf("%s deveria estar sozinha: %+v", g.Name, g.Ally)
		}
	}
	if _, err := r.DisbandAlliance(a); !errors.Is(err, ErrNotAllied) {
		t.Errorf("desfazer sem aliança: %v", err)
	}
}

func TestRetirarAliadaDeixaAsOutrasComListaNova(t *testing.T) {
	r, a, b, c := aliancaDeTres(t)
	tirada, mudou, err := r.KickAlly(a, 1) // slot 1 = Beta
	if err != nil {
		t.Fatal(err)
	}
	if tirada != b || b.InAlliance() {
		t.Error("Beta deveria sair e ficar sozinha")
	}
	if len(mudou) != 3 {
		t.Errorf("alteradas = %d, queria 3 (lider, retirada, restante)", len(mudou))
	}
	if a.AllyCount() != 2 || c.Ally != a.Ally {
		t.Errorf("a lista nova deveria ir para Gama: %+v / %+v", a.Ally, c.Ally)
	}
	for _, it := range c.Ally.Guilds {
		if it.Index == b.Index {
			t.Error("Gama ainda lista Beta")
		}
	}
	if _, _, err := r.KickAlly(a, 1); !errors.Is(err, ErrAllySlot) {
		t.Errorf("slot vazio: %v", err)
	}
	if _, _, err := r.KickAlly(a, 0); !errors.Is(err, ErrAllySlot) {
		t.Errorf("slot 0 (a lider): %v", err)
	}
	if _, _, err := r.KickAlly(c, 1); !errors.Is(err, ErrNotAllyLeader) {
		t.Errorf("aliada retirando: %v", err)
	}
}

func TestSairDaAlianca(t *testing.T) {
	r, a, b, c := aliancaDeTres(t)
	if _, err := r.LeaveAlliance(a); !errors.Is(err, ErrIsAllyLeader) {
		t.Errorf("lider saindo: %v", err)
	}
	mudou, err := r.LeaveAlliance(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(mudou) != 3 || b.InAlliance() {
		t.Errorf("alteradas = %d, Beta sozinha = %v", len(mudou), !b.InAlliance())
	}
	for _, g := range []*Guild{a, c} {
		for _, it := range g.Ally.Guilds {
			if it.Index == b.Index {
				t.Errorf("%s ainda lista Beta", g.Name)
			}
		}
	}
	if a.AllyCount() != 2 {
		t.Errorf("a aliança deveria ter 2 guildas, tem %d", a.AllyCount())
	}
	if _, err := r.LeaveAlliance(b); !errors.Is(err, ErrNotAllied) {
		t.Errorf("sair sem estar: %v", err)
	}
}
