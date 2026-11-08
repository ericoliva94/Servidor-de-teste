package game

import (
	"context"
	"sync"
	"testing"

	"aika/internal/guild"
	"aika/internal/nation"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// fakeNations e um NationStore em memoria que guarda o que foi gravado.
type fakeNations struct {
	mu     sync.Mutex
	salvos []nation.State
	grades []nation.State
}

func (f *fakeNations) Load(context.Context) ([]nation.State, error) { return nil, nil }

func (f *fakeNations) SaveTaxes(_ context.Context, st nation.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.salvos = append(f.salvos, st)
	return nil
}

// cenarioNacao monta a nacao 1 com a guilda marechal "Alfa" (lider 11), um
// membro comum (12), e um jogador da nacao 2 (13).
type cenarioNacao struct {
	h               *Handler
	db              *fakeNations
	lider, membro   *Session
	outro           *Session
	eLider, eMembro *espiao
	eOutro          *espiao
	alfa            *guild.Guild
}

func novoCenarioNacao(t *testing.T) *cenarioNacao {
	t.Helper()
	h := handlerComGuildas(t)
	h.Nations = nation.New()
	db := &fakeNations{}
	h.NationDB = db
	c := &cenarioNacao{h: h, db: db}
	c.lider, c.eLider = entraComPersonagem(t, h, "Lider", 11)
	c.membro, c.eMembro = entraComPersonagem(t, h, "Membro", 12)
	c.outro, c.eOutro = entraComPersonagem(t, h, "Outro", 13)
	for _, s := range []*Session{c.lider, c.membro, c.outro} {
		s.Status = Playing
	}
	c.lider.Avatar.Nation, c.membro.Avatar.Nation, c.outro.Avatar.Nation = 1, 1, 2

	g, err := h.Guilds.Create("Alfa", 1, guild.Member{CharIndex: 11, Name: "Lider", Logged: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Guilds.Join(g, guild.Member{CharIndex: 12, Name: "Membro", Rank: 1, Logged: true}); err != nil {
		t.Fatal(err)
	}
	c.alfa = g
	c.lider.Avatar.GuildIndex, c.membro.Avatar.GuildIndex = g.Index, g.Index
	h.Nations.Update(1, func(s *nation.State) {
		s.MarshalGuild = g.Index
		s.CitizenTax, s.VisitorTax = 7, 12
		s.Gold = 777
	})
	h.Nations.Update(2, func(s *nation.State) { s.Gold = 5 })
	return c
}

func taxasPacket(cidadao, visitante uint16) []byte {
	return miscPacket(CodeUpdateNationTaxes, packets.UpdateNationTaxes{CitizenTax: cidadao, VisitorTax: visitante})
}

func (c *cenarioNacao) mudaTaxas(t *testing.T, s *Session, e *espiao, cid, vis uint16) {
	t.Helper()
	if err := c.h.updateNationTaxes(context.Background(), e.conn, s, asPacket(t, taxasPacket(cid, vis))); err != nil {
		t.Fatal(err)
	}
}

func TestTesouroDaNacaoVoltaAoCliente(t *testing.T) {
	c := novoCenarioNacao(t)
	pedido := asPacket(t, miscPacket(CodeNationGold, packets.UpdateNationTreasure{}))
	if err := c.h.updateNationGold(c.eLider.conn, c.lider, pedido); err != nil {
		t.Fatal(err)
	}
	got := decodifica[packets.UpdateNationTreasure](t, proximoDe(t, c.eLider, CodeNationGold))
	if got.Gold != 777 {
		t.Errorf("tesouro da nacao 1 = %d, queria 777", got.Gold)
	}
	// Outra nacao ve o seu proprio tesouro.
	if err := c.h.updateNationGold(c.eOutro.conn, c.outro, pedido); err != nil {
		t.Fatal(err)
	}
	if got := decodifica[packets.UpdateNationTreasure](t, proximoDe(t, c.eOutro, CodeNationGold)); got.Gold != 5 {
		t.Errorf("tesouro da nacao 2 = %d, queria 5", got.Gold)
	}
	// Sem nacao: zero.
	c.membro.Avatar.Nation = 0
	if err := c.h.updateNationGold(c.eMembro.conn, c.membro, pedido); err != nil {
		t.Fatal(err)
	}
	if got := decodifica[packets.UpdateNationTreasure](t, proximoDe(t, c.eMembro, CodeNationGold)); got.Gold != 0 {
		t.Errorf("sem nacao = %d, queria 0", got.Gold)
	}
}

func TestMarechalMudaTaxasESalva(t *testing.T) {
	c := novoCenarioNacao(t)
	c.mudaTaxas(t, c.lider, c.eLider, 10, 15)
	st, _ := c.h.Nations.Get(1)
	if st.CitizenTax != 10 || st.VisitorTax != 15 {
		t.Fatalf("taxas = %d/%d, queria 10/15", st.CitizenTax, st.VisitorTax)
	}
	if len(c.db.salvos) != 1 || c.db.salvos[0].CitizenTax != 10 || c.db.salvos[0].Gold != 777 {
		t.Errorf("gravado = %+v", c.db.salvos)
	}
	// Todo jogador da nacao recebe o aviso e a informacao nova; o de outra nacao nao.
	want := "A taxa de sua nação foi setada em 10% para cidadãos e 15% para não-cidadãos."
	if got := mensagemDe(t, c.eMembro); got != want {
		t.Errorf("aviso = %q", got)
	}
	grade := decodifica[packets.GuildsGradePacket](t, proximoDe(t, c.eMembro, CodeNationGrade))
	if grade.CitizenTax != 10 || grade.NoCitizenTax != 15 || grade.Nation != 1 {
		t.Errorf("grade = %+v", grade)
	}
	if netsrv.CString(grade.Guilds.LordMarechal[:]) != "Alfa" {
		t.Errorf("lord marechal = %q", netsrv.CString(grade.Guilds.LordMarechal[:]))
	}
	c.eOutro.nada(t, 50e6)
}

func TestSoOMarechalMudaTaxas(t *testing.T) {
	c := novoCenarioNacao(t)
	// Membro comum da guilda marechal.
	c.mudaTaxas(t, c.membro, c.eMembro, 10, 15)
	// Lider de uma guilda que nao e a marechal.
	c.h.Nations.Update(1, func(s *nation.State) { s.MarshalGuild = 999 })
	c.mudaTaxas(t, c.lider, c.eLider, 10, 15)
	// Sem guilda.
	c.outro.Avatar.Nation = 1
	c.mudaTaxas(t, c.outro, c.eOutro, 10, 15)
	st, _ := c.h.Nations.Get(1)
	if st.CitizenTax != 7 || st.VisitorTax != 12 || len(c.db.salvos) != 0 {
		t.Errorf("taxas mudaram sem ser marechal: %d/%d gravados=%d", st.CitizenTax, st.VisitorTax, len(c.db.salvos))
	}
}

func TestTaxasForaDaFaixaSaoRecusadas(t *testing.T) {
	c := novoCenarioNacao(t)
	for _, par := range [][2]uint16{{4, 15}, {16, 15}, {10, 9}, {10, 21}, {0, 0},
		// A origem truncava para BYTE: 266 virava 10 e passava.
		{266, 15}, {10, 266}} {
		c.mudaTaxas(t, c.lider, c.eLider, par[0], par[1])
	}
	st, _ := c.h.Nations.Get(1)
	if st.CitizenTax != 7 || st.VisitorTax != 12 || len(c.db.salvos) != 0 {
		t.Errorf("taxa invalida foi aceita: %d/%d gravados=%d", st.CitizenTax, st.VisitorTax, len(c.db.salvos))
	}
	// Os limites exatos valem.
	for _, par := range [][2]uint16{{5, 10}, {15, 20}} {
		c.mudaTaxas(t, c.lider, c.eLider, par[0], par[1])
	}
	if st, _ := c.h.Nations.Get(1); st.CitizenTax != 15 || st.VisitorTax != 20 {
		t.Errorf("limites: %d/%d", st.CitizenTax, st.VisitorTax)
	}
}

func TestInformacaoDeNacaoLevaOsQuatroPacotes(t *testing.T) {
	c := novoCenarioNacao(t)
	c.h.sendNationInformation(c.eLider.conn, c.lider)
	for _, code := range []uint16{CodeNationGrade, CodeNationSiege, CodeNationAlly, CodeNationGold} {
		proximoDe(t, c.eLider, code)
	}
}

func TestBuffsDeMarechalEDeMembro(t *testing.T) {
	c := novoCenarioNacao(t)
	c.h.sendNationInformation(c.eLider.conn, c.lider)
	c.h.sendNationInformation(c.eMembro.conn, c.membro)
	if !c.lider.Base.Buffs.Has(buffMarshal) || c.lider.Base.Buffs.Has(buffMarshalGuild) {
		t.Error("o marechal deveria ter so o buff 6600")
	}
	if c.membro.Base.Buffs.Has(buffMarshal) || !c.membro.Base.Buffs.Has(buffMarshalGuild) {
		t.Error("o membro deveria ter so o buff 6601")
	}
	// Perde o cargo: os buffs saem.
	c.h.Nations.Update(1, func(s *nation.State) { s.MarshalGuild = 0 })
	c.h.sendNationInformation(c.eLider.conn, c.lider)
	if c.lider.Base.Buffs.Has(buffMarshal) {
		t.Error("o buff do marechal deveria ter saido")
	}
}

func TestBuffsDeArchon(t *testing.T) {
	c := novoCenarioNacao(t)
	c.h.Nations.Update(1, func(s *nation.State) { s.MarshalGuild = 0; s.TacticianGuild = c.alfa.Index })
	c.h.sendNationInformation(c.eLider.conn, c.lider)
	c.h.sendNationInformation(c.eMembro.conn, c.membro)
	if !c.lider.Base.Buffs.Has(buffArchon) || c.lider.Base.Buffs.Has(buffArchonGuild) {
		t.Error("o lider da guilda archon deveria ter so o buff 6602")
	}
	if c.membro.Base.Buffs.Has(buffArchon) || !c.membro.Base.Buffs.Has(buffArchonGuild) {
		t.Error("o membro da guilda archon deveria ter so o buff 6648")
	}
}

func TestNacaoSemRegistroNaoQuebra(t *testing.T) {
	c := novoCenarioNacao(t)
	c.h.Nations = nil
	c.mudaTaxas(t, c.lider, c.eLider, 10, 15)
	c.h.sendNationInformation(c.eLider.conn, c.lider)
	if err := c.h.updateNationGold(c.eLider.conn, c.lider,
		asPacket(t, miscPacket(CodeNationGold, packets.UpdateNationTreasure{}))); err != nil {
		t.Fatal(err)
	}
	if got := decodifica[packets.UpdateNationTreasure](t, proximoDe(t, c.eLider, CodeNationGold)); got.Gold != 0 {
		t.Errorf("tesouro = %d, queria 0", got.Gold)
	}
}

func (f *fakeNations) SaveGrade(_ context.Context, st nation.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grades = append(f.grades, st)
	return nil
}
