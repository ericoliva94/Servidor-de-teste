package game

import (
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// prepWar: autor (a, nacao 1) contra inimigo (b, nacao 2), perto, com a skill 300
// na tabela. d.Index sera o indice real da skill de guerreiro/templario.
func prepWar(t *testing.T, idx int, d gamedata.T_SkillData) (*Handler, *Session, *espiao, *Session, *espiao) {
	t.Helper()
	d.SuccessRate, d.DamageRange = 1, 20
	if d.Classe == 0 {
		d.Classe = 1
	}
	h, a, ae, b, be := prepArea(t, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(idx, d)
	a.Skills.Others[0] = model.SkillFromList{Index: uint16(idx), Level: 5}
	a.Skills.Others[1] = model.SkillFromList{}
	return h, a, ae, b, be
}

func TestPancadaSomaVidaDoAutor(t *testing.T) {
	d := &gamedata.T_SkillData{Index: skPancada, Adicional: 10}
	a := &Session{}
	a.Avatar.CurHP = 5000
	if got := warriorAdjust(a, d, 100); got != 100+500 {
		t.Errorf("dano = %d, queria 600", got)
	}
	if got := warriorAdjust(a, &gamedata.T_SkillData{Index: skAtaquePoderoso}, 100); got != 100 {
		t.Errorf("skill comum mudou o dano: %d", got)
	}
}

func TestEstocadaNaoCausaDanoMasAplicaOEfeito(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skEstocada, gamedata.T_SkillData{Damage: 900, Duration: 30,
		EF: [4]int32{efHit}, EFV: [4]int32{-5}})
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skEstocada, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != antes {
		t.Errorf("Estocada causou dano: %d -> %d", antes, b.Avatar.CurHP)
	}
	if !b.Base.Buffs.Has(skEstocada) {
		t.Error("o efeito deveria ter entrado")
	}
}

func TestAvancoPoderosoStunEAvanco(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skAvancoPoderoso, gamedata.T_SkillData{Damage: 10, Duration: 5})
	b.Position = model.Position{X: 1010, Y: 1000}
	a.Avatar.Resistence = 0
	lanca(t, h, a, ae, skAvancoPoderoso, uint32(b.ClientID), a.Position)
	if !b.Base.Buffs.Has(skAvancoPoderoso) {
		t.Error("o stun deveria ter entrado")
	}
	if a.Position != b.Position {
		t.Errorf("autor em %+v, queria %+v (avanco)", a.Position, b.Position)
	}
}

func TestStunResistidoNaoEntraENaoAvanca(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skAvancoPoderoso, gamedata.T_SkillData{Damage: 10, Duration: 5})
	b.Position = model.Position{X: 1010, Y: 1000}
	b.Avatar.Ability[efImSkillStun] = 300 // imune: o sorteio (1..254) nunca passa
	antes := a.Position
	lanca(t, h, a, ae, skAvancoPoderoso, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(skAvancoPoderoso) {
		t.Error("stun resistido entrou")
	}
	if a.Position != antes {
		t.Error("avancou sobre quem resistiu")
	}
}

func TestFeridaMortalSilencioResistido(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skFeridaMortal, gamedata.T_SkillData{Damage: 10, Duration: 5})
	b.Avatar.Ability[efImSilence] = 300
	lanca(t, h, a, ae, skFeridaMortal, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(skFeridaMortal) {
		t.Error("silencio resistido entrou")
	}
	h2, a2, ae2, b2, _ := prepWar(t, skFeridaMortal, gamedata.T_SkillData{Damage: 10, Duration: 5})
	lanca(t, h2, a2, ae2, skFeridaMortal, uint32(b2.ClientID), a2.Position)
	if !b2.Base.Buffs.Has(skFeridaMortal) {
		t.Error("sem imunidade o silencio deveria entrar")
	}
}

func TestCargaDivinaLongeNaoAvanca(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skCargaDivina, gamedata.T_SkillData{Damage: 10})
	b.Position = model.Position{X: 1019, Y: 1000} // 19 > 18
	a.Position = model.Position{X: 1000, Y: 1000}
	h.dashTo(a, b.Position, skCargaDivina)
	if a.Position.X != 1000 {
		t.Errorf("avancou %v celulas, o limite e 18", a.Position.X-1000)
	}
	_ = ae
}

// ---- escudos com contador

func TestBolhaAnulaOGolpeDeSkillEGastaPontos(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skNemesis, gamedata.T_SkillData{Damage: 10, Duration: 5})
	h.SkillTable.(*tabelaDeSkills).poe(skBolha, gamedata.T_SkillData{Duration: 60, TargetType: 1})
	h.addBuff(b, skBolha)
	b.BolhaPoints = 2
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skNemesis, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != antes || b.BolhaPoints != 1 || !b.Base.Buffs.Has(skBolha) {
		t.Fatalf("1o golpe: HP %d->%d pontos=%d bolha=%v", antes, b.Avatar.CurHP, b.BolhaPoints, b.Base.Buffs.Has(skBolha))
	}
	if b.Base.Buffs.Has(skNemesis) {
		t.Error("o efeito do golpe anulado nao pode entrar")
	}
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skNemesis, uint32(b.ClientID), a.Position)
	if b.BolhaPoints != 0 || b.Base.Buffs.Has(skBolha) {
		t.Errorf("2o golpe: pontos=%d bolha=%v (deveria acabar)", b.BolhaPoints, b.Base.Buffs.Has(skBolha))
	}
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skNemesis, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP >= antes {
		t.Error("sem a bolha o golpe deveria causar dano")
	}
}

func TestProtecaoCastGuardaTresPontosEReduzODano(t *testing.T) {
	h, a, ae, b, be := prepWar(t, skAtaquePoderoso, gamedata.T_SkillData{Damage: 10})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(skProtecao, gamedata.T_SkillData{Classe: 1, TargetType: 1, Duration: 60})
	b.Skills.Others[0] = model.SkillFromList{Index: skProtecao, Level: 5}
	b.Avatar.ClassInfo = 1
	lanca(t, h, b, be, skProtecao, 0, b.Position)
	if b.DefesaPoints != 3 || !b.Base.Buffs.Has(skProtecao) {
		t.Fatalf("pontos=%d buff=%v", b.DefesaPoints, b.Base.Buffs.Has(skProtecao))
	}
	b.Avatar.Ability[155] = 50 // EF_POINT_DEFENCE: 50% menos
	b.Avatar.CurHP = b.Avatar.MaxHP
	var danos []uint32
	for i := 0; i < 4; i++ {
		a.Base.Cooldowns.Clear()
		antes := b.Avatar.CurHP
		b.Avatar.Ability[155] = 50
		lanca(t, h, a, ae, skAtaquePoderoso, uint32(b.ClientID), a.Position)
		danos = append(danos, antes-b.Avatar.CurHP)
	}
	if b.Base.Buffs.Has(skProtecao) {
		t.Error("a Protecao deveria sair depois de 3 golpes")
	}
	if !(danos[0] < danos[3]) {
		t.Errorf("danos = %v: com Protecao deveria ser menor que sem", danos)
	}
}

func TestUniaoDivinaDesviaDanoParaQuemLancou(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skAtaquePoderoso, gamedata.T_SkillData{Damage: 10})
	h.SkillTable.(*tabelaDeSkills).poe(skUniaoDivina, gamedata.T_SkillData{Duration: 60})
	// O aliado de b que lancou a uniao e o proprio "a"? Usa um terceiro.
	templar := lutador("Templar", 1004, 1000)
	templar.Avatar.Nation = 2
	te := novoEspiao(t)
	h.Chan.Join(te.conn, templar)
	h.Chan.RegisterCharacter(templar.ClientID, "Templar")
	h.addBuff(b, skUniaoDivina)
	b.UniaoDivina = "Templar"
	b.Avatar.Ability[efTransfer] = 50
	b.Avatar.Ability[efTransferLimit] = 100000
	b.Avatar.CurHP = b.Avatar.MaxHP
	hpT := templar.Avatar.CurHP
	lanca(t, h, a, ae, skAtaquePoderoso, uint32(b.ClientID), a.Position)
	if templar.Avatar.CurHP >= hpT {
		t.Error("o lancador da uniao deveria sofrer parte do dano")
	}
	// Sem o lancador no jogo, o buff cai.
	h.Chan.Leave(templar.ClientID)
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skAtaquePoderoso, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(skUniaoDivina) || b.UniaoDivina != "" {
		t.Error("o buff deveria cair sem o lancador")
	}
}

func TestElTyciaCuraTudoEZeraAManaDoAutor(t *testing.T) {
	h, a, ae, b, _ := prepWar(t, skElTycia, gamedata.T_SkillData{})
	d := h.SkillTable.(*tabelaDeSkills)
	d.poe(skElTycia, gamedata.T_SkillData{Classe: 1, SuccessRate: 0, DamageRange: 20})
	b.Avatar.CurHP = 5
	a.Avatar.CurMP = a.Avatar.MaxMP
	lanca(t, h, a, ae, skElTycia, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != b.Avatar.MaxHP || a.Avatar.CurMP != 0 {
		t.Errorf("HP alvo=%d/%d MP autor=%d", b.Avatar.CurHP, b.Avatar.MaxHP, a.Avatar.CurMP)
	}
}

func TestRevitalizarRegeneraPorcentoDaVidaPorSegundo(t *testing.T) {
	h, a, ae, _, _ := prepWar(t, skRevitalizar, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(skRevitalizar, gamedata.T_SkillData{Classe: 1, SuccessRate: 0, TargetType: 1, Duration: 20})
	a.Avatar.CurHP = 10
	lanca(t, h, a, ae, skRevitalizar, 0, a.Position)
	h.BuffTick()
	h.BuffTick()
	if want := 10 + 2*(a.Avatar.MaxHP/100); a.Avatar.CurHP != want {
		t.Errorf("HP = %d, queria %d", a.Avatar.CurHP, want)
	}
	_ = time.Second
}
