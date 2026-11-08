package game

import (
	"context"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/skillfn"
)

// prepArea monta um autor (guerreiro, nacao 1) e um inimigo (nacao 2) perto.
func prepArea(t *testing.T, d gamedata.T_SkillData) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	h, tab, _ := handlerComSkills(t)
	h.Durations = skillfn.BuffSeconds{T: tab}
	d.Classe = 1
	tab.poe(300, d)
	h.Rand = randFixo{v: 50}
	ae, be := novoEspiao(t), novoEspiao(t)
	a := lutador("Alfa", 1000, 1000)
	a.Avatar.ClassInfo, a.Avatar.Nation = 1, 1
	a.Skills.Others[0] = model.SkillFromList{Index: 300, Level: 5}
	b := lutador("Beta", 1003, 1000)
	b.Avatar.Nation = 2
	if _, err := h.Chan.Join(ae.conn, a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Chan.Join(be.conn, b); err != nil {
		t.Fatal(err)
	}
	a.Base.VisiblePlayers.Add(b.ClientID)
	b.Base.VisiblePlayers.Add(a.ClientID)
	ae.drena(10 * time.Millisecond)
	be.drena(10 * time.Millisecond)
	return h, a, ae, b, be
}

func lanca(t *testing.T, h *Handler, s *Session, e *espiao, skill, alvo uint32, pos model.Position) {
	t.Helper()
	var p packets.SendSkillUse
	p.Skill, p.Index, p.Pos = skill, alvo, pos
	if err := h.useSkill(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeUseSkill, p))); err != nil {
		t.Fatal(err)
	}
}

func TestSkillDeAreaAcertaInimigoPerto(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5, Damage: 500, DamageRange: 10})
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, 300, 0, a.Position)
	if b.Avatar.CurHP >= antes {
		t.Errorf("HP do inimigo = %d, antes %d", b.Avatar.CurHP, antes)
	}
	if a.Avatar.CurHP != a.Avatar.MaxHP {
		t.Error("o autor nao pode se ferir")
	}
}

func TestSkillDeAreaPoupaAliados(t *testing.T) {
	casos := map[string]func(a, b *Session, h *Handler){
		"mesma guilda": func(a, b *Session, h *Handler) { a.Avatar.GuildIndex, b.Avatar.GuildIndex = 7, 7 },
		"mesmo grupo": func(a, b *Session, h *Handler) {
			g, _ := h.Parties.Create(a.ClientID)
			_ = h.Parties.Add(g, b.ClientID)
		},
		"mesma nacao sem PK": func(a, b *Session, h *Handler) { b.Avatar.Nation = a.Avatar.Nation },
		"zona segura":        func(a, b *Session, h *Handler) { b.Position = model.Position{X: 2947, Y: 1664} },
		"morto":              func(a, b *Session, h *Handler) { b.Dead = true },
	}
	for nome, prep := range casos {
		t.Run(nome, func(t *testing.T) {
			h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5000, Damage: 500, DamageRange: 20000})
			prep(a, b, h)
			if nome == "zona segura" { // o centro precisa alcancar o alvo
				a.Position = b.Position
			}
			antes := b.Avatar.CurHP
			lanca(t, h, a, ae, 300, 0, a.Position)
			if b.Avatar.CurHP != antes {
				t.Errorf("o alvo protegido levou dano: %d -> %d", antes, b.Avatar.CurHP)
			}
		})
	}
}

func TestSkillDeAreaMesmaNacaoComPKAcerta(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5, Damage: 500, DamageRange: 10})
	b.Avatar.Nation, a.Avatar.PlayerKill = a.Avatar.Nation, true
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, 300, 0, a.Position)
	if b.Avatar.CurHP >= antes {
		t.Error("com PK ligado o aliado de nacao deveria apanhar")
	}
}

func TestSkillDeAreaRespeitaORaioEOLimiteDeAlvos(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 1, Damage: 500, DamageRange: 10})
	b.Position = model.Position{X: 1050, Y: 1000} // 50 de distancia, raio 1
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, 300, 0, a.Position)
	if b.Avatar.CurHP != antes {
		t.Error("alvo fora do raio levou dano")
	}

	// MaxTargets = 1: so o primeiro de dois alvos apanha.
	h2, a2, ae2, b2, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 50, Damage: 500, DamageRange: 10, MaxTargets: 1})
	ce := novoEspiao(t)
	c3 := lutador("Gama", 1004, 1000)
	c3.Avatar.Nation = 2
	h2.Chan.Join(ce.conn, c3)
	a2.Base.VisiblePlayers.Add(c3.ClientID)
	pb, pc := b2.Avatar.CurHP, c3.Avatar.CurHP
	lanca(t, h2, a2, ae2, 300, 0, a2.Position)
	feridos := 0
	if b2.Avatar.CurHP < pb {
		feridos++
	}
	if c3.Avatar.CurHP < pc {
		feridos++
	}
	if feridos != 1 {
		t.Errorf("feridos = %d, queria 1 (MaxTargets)", feridos)
	}
}

// A posicao do pacote longe demais do autor nao desloca a area para o outro lado do mapa.
func TestSkillDeAreaCentroLongeCaiNoAutor(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5, Damage: 500, DamageRange: 10})
	if got := h.skillCenter(a, model.Position{X: 9000, Y: 9000}, skillfn.Data(h.SkillTable, 300)); got != a.Position {
		t.Errorf("centro = %+v", got)
	}
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, 300, 0, model.Position{X: 1003, Y: 1000}) // pos valida
	if b.Avatar.CurHP >= antes {
		t.Error("centro valido deveria acertar")
	}
}

func TestSkillDeAreaAcertaMobs(t *testing.T) {
	h, a, ae, _, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5, Damage: 500, DamageRange: 10})
	h.Mobs = NewMobWorld(tabelaDeMobs(especie("Bicho", 100000, 1, 1000, 1000)))
	h.Exp = expFixa{}
	m, ok := h.Mobs.Get(3048)
	if !ok {
		t.Fatal("mob inexistente")
	}
	antes := m.CurHP
	lanca(t, h, a, ae, 300, 0, a.Position)
	if m.CurHP >= antes {
		t.Errorf("mob nao foi atingido: %d -> %d", antes, m.CurHP)
	}
}

// ---- buff em area e em si mesmo

func TestBuffEmAreaSemGrupoVaiSoNoAutor(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 0, Range: 20, Duration: 60,
		EF: [4]int32{efDamage1}, EFV: [4]int32{50}})
	base := a.Avatar.AtkFis
	lanca(t, h, a, ae, 300, 0, a.Position)
	if !a.Base.Buffs.Has(300) || a.Avatar.AtkFis != base+50 {
		t.Errorf("autor: buff=%v atk=%d (base %d)", a.Base.Buffs.Has(300), a.Avatar.AtkFis, base)
	}
	if b.Base.Buffs.Has(300) {
		t.Error("quem nao e do grupo nao recebe")
	}
}

func TestBuffEmAreaChegaAoGrupoPerto(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 0, Range: 20, Duration: 60})
	g, _ := h.Parties.Create(a.ClientID)
	_ = h.Parties.Add(g, b.ClientID)
	longe := lutador("Longe", 5000, 5000)
	le := novoEspiao(t)
	h.Chan.Join(le.conn, longe)
	_ = h.Parties.Add(g, longe.ClientID)
	a.Base.VisiblePlayers.Add(longe.ClientID)
	lanca(t, h, a, ae, 300, 0, a.Position)
	if !a.Base.Buffs.Has(300) || !b.Base.Buffs.Has(300) {
		t.Error("autor e membro perto deveriam ter o buff")
	}
	if longe.Base.Buffs.Has(300) {
		t.Error("membro fora do raio recebeu")
	}
}

func TestBuffTargetType1VaiSempreNoProprioAutor(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 0, TargetType: 1, Duration: 60, DamageRange: 10})
	lanca(t, h, a, ae, 300, uint32(b.ClientID), a.Position)
	if !a.Base.Buffs.Has(300) || b.Base.Buffs.Has(300) {
		t.Errorf("autor=%v alvo=%v", a.Base.Buffs.Has(300), b.Base.Buffs.Has(300))
	}
}

func TestSkillDeDanoComDuracaoAplicaOBuffNoAtingido(t *testing.T) {
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{SuccessRate: 1, Range: 5, Damage: 500, DamageRange: 10,
		Duration: 30, EF: [4]int32{efHit}, EFV: [4]int32{-8}})
	lanca(t, h, a, ae, 300, 0, a.Position)
	if !b.Base.Buffs.Has(300) {
		t.Error("o debuff deveria ter sido aplicado no atingido")
	}
}
