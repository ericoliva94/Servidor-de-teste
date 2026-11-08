package game

import (
	"testing"
	"time"

	"aika/internal/combat"
	"aika/internal/gamedata"
	"aika/internal/model"
)

// prepMisc2: autor (a, classe 1) com uma skill "em si mesmo" (SuccessRate 0).
func prepSelf(t *testing.T, idx int, d gamedata.T_SkillData) (*Handler, *Session, *espiao, *Session) {
	t.Helper()
	d.SuccessRate, d.TargetType, d.Classe, d.DamageRange = 0, 1, 1, 20
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(idx, d)
	a.Skills.Others[0] = model.SkillFromList{Index: uint16(idx), Level: 5}
	a.Avatar.ClassInfo = 1
	return h, a, ae, b
}

func TestTempoExtraDeBuffPeloEfeitoATIME6(t *testing.T) {
	h, a, ae, _ := prepSelf(t, 124, gamedata.T_SkillData{Duration: 100})
	a.Avatar.Ability[efSkillATime6] = 2 // 2 min
	lanca(t, h, a, ae, 124, 0, a.Position)
	ini, ok := a.Base.Buffs.StartedAt(124)
	if !ok || ini.Sub(h.now()) != 120*time.Second {
		t.Errorf("inicio adiantado em %v, queria 120s", ini.Sub(h.now()))
	}
}

func TestFluxoDeManaRecuperaMana(t *testing.T) {
	h, a, ae, _ := prepSelf(t, skFluxoMana, gamedata.T_SkillData{})
	a.Avatar.CurMP = 0
	lanca(t, h, a, ae, skFluxoMana, 0, a.Position)
	if min := a.Avatar.MaxMP / 2; a.Avatar.CurMP < min {
		t.Errorf("MP = %d, queria pelo menos %d", a.Avatar.CurMP, min)
	}
}

func TestEfeito5RecuperaMPouHP(t *testing.T) {
	h, a, ae, _ := prepSelf(t, skEfeito5, gamedata.T_SkillData{Damage: 200, Duration: 10})
	a.Avatar.CurMP = 0
	lanca(t, h, a, ae, skEfeito5, 0, a.Position)
	if a.Avatar.CurMP != a.Avatar.MaxMP/100*15 || !a.Base.Buffs.Has(skEfeito5) {
		t.Errorf("MP=%d buff=%v", a.Avatar.CurMP, a.Base.Buffs.Has(skEfeito5))
	}
	h2, a2, ae2, _ := prepSelf(t, skEfeito5, gamedata.T_SkillData{Damage: 300, Duration: 10})
	a2.Avatar.CurHP = 1
	lanca(t, h2, a2, ae2, skEfeito5, 0, a2.Position)
	if a2.Avatar.CurHP != 1+a2.Avatar.MaxHP/100*15 || a2.Base.Buffs.Has(skEfeito5) {
		t.Errorf("HP=%d buff=%v (300 so cura)", a2.Avatar.CurHP, a2.Base.Buffs.Has(skEfeito5))
	}
}

func TestDisparosRapidosZeraRecargas(t *testing.T) {
	h, a, ae, _ := prepSelf(t, skDisparoRapido, gamedata.T_SkillData{Duration: 10})
	a.Base.Cooldowns.Use(500, h.now())
	lanca(t, h, a, ae, skDisparoRapido, 0, a.Position)
	if !a.Base.Cooldowns.Ready(500, h.now(), h.cooldowns()) && h.cooldowns().CooldownMillis(500) > 0 {
		t.Error("as recargas deveriam ter sido zeradas")
	}
	if !a.Base.Buffs.Has(skDisparoRapido) {
		t.Error("falta o buff")
	}
}

func TestTeleporteVaiAoPontoFixoERecusaComReliquia(t *testing.T) {
	h, a, ae, _ := prepSelf(t, skTeleporte, gamedata.T_SkillData{})
	h.Data.(*tabelaDeItens).poeItem(900, gamedata.ItemFromList{ItemType: 40})
	a.Avatar.Nation = 0
	a.Inventory.Slots[3] = model.Item{Index: 900}
	lanca(t, h, a, ae, skTeleporte, 0, a.Position)
	if a.Position.X == teleportDestX {
		t.Fatal("teleportou carregando reliquia")
	}
	a.Inventory.Slots[3] = model.Item{}
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skTeleporte, 0, a.Position)
	if a.Position.X != teleportDestX || a.Position.Y != teleportDestY {
		t.Errorf("posicao = %+v", a.Position)
	}
}

func TestLaminaAcertaOsInimigosACadaSegundo(t *testing.T) {
	h, a, ae, b := prepSelf(t, skLamina, gamedata.T_SkillData{Duration: 5, EFV: [4]int32{301}})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(301, gamedata.T_SkillData{Classe: 1, SuccessRate: 1, Range: 10, Damage: 100, DamageRange: 20})
	b.Avatar.CurHP = b.Avatar.MaxHP
	lanca(t, h, a, ae, skLamina, 0, a.Position)
	antes := b.Avatar.CurHP
	h.BuffTick()
	if b.Avatar.CurHP >= antes {
		t.Fatal("a lamina deveria ferir quem esta em volta")
	}
	// Acaba com o buff.
	h.removeBuffAndNotify(a, skLamina)
	meio := b.Avatar.CurHP
	h.BuffTick()
	if b.Avatar.CurHP != meio {
		t.Error("continuou ferindo depois do buff")
	}
}

func TestRequiemSomaODobroDoEfeito(t *testing.T) {
	h, a, _, b := prepSelf(t, 1, gamedata.T_SkillData{})
	b.Avatar.Ability[efAddDamage1] = 30
	if got := h.reactiveHit(a, b, 100, combat.Normal, true); got != 160 {
		t.Errorf("dano = %d, queria 160", got)
	}
}

func TestPedraDaAlmaRevive(t *testing.T) {
	h, a, _, b := prepSelf(t, 1, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(buffPedraDaAlma, gamedata.T_SkillData{Duration: 60})
	h.SkillTable.(*tabelaDeSkills).poe(601, gamedata.T_SkillData{Duration: 60, BuffDebuff: 3})
	h.addBuff(b, buffPedraDaAlma)
	h.addBuff(b, 601)
	b.Avatar.CurHP = 10
	h.reactiveHit(a, b, 500, combat.Normal, true)
	if b.Base.Buffs.Has(buffPedraDaAlma) || b.Base.Buffs.Has(601) {
		t.Error("a pedra e os debuffs deveriam sair")
	}
	if b.Avatar.CurHP != b.Avatar.MaxHP/100*30 || b.Avatar.CurMP != b.Avatar.MaxMP/100*25 {
		t.Errorf("HP=%d MP=%d", b.Avatar.CurHP, b.Avatar.CurMP)
	}
	// Sem o buff, nada acontece.
	b.Avatar.CurHP = 10
	h.reactiveHit(a, b, 500, combat.Normal, true)
	if b.Avatar.CurHP != 10 {
		t.Error("reviveu sem o buff")
	}
}

func TestEstripadorDisparaBuffNoCritico(t *testing.T) {
	h, a, _, b := prepSelf(t, 1, gamedata.T_SkillData{})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(buffEstripador, gamedata.T_SkillData{Duration: 60})
	tab.poe(buffEstripadorEf, gamedata.T_SkillData{Duration: 5})
	h.addBuff(b, buffEstripador)
	h.reactiveHit(a, b, 100, combat.Normal, true)
	if b.Base.Buffs.Has(buffEstripadorEf) {
		t.Error("golpe normal nao dispara")
	}
	h.reactiveHit(a, b, 100, combat.Critical, true)
	if !b.Base.Buffs.Has(buffEstripadorEf) {
		t.Error("critico deveria disparar o buff")
	}
}

func TestResolutoMarcaOAlvoPorOitoSegundos(t *testing.T) {
	h, a, _, b := prepSelf(t, 1, gamedata.T_SkillData{})
	tab := h.SkillTable.(*tabelaDeSkills)
	tab.poe(buffResolutoMark, gamedata.T_SkillData{Duration: 5})
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	h.markResoluto(b, &gamedata.T_SkillData{Index: skResoluto, Damage: 2})
	if b.ResolutoPoints != 2 {
		t.Fatalf("pontos = %d", b.ResolutoPoints)
	}
	h.reactiveHit(a, b, 10, combat.Normal, true)
	if b.ResolutoPoints != 1 {
		t.Errorf("pontos = %d, queria 1", b.ResolutoPoints)
	}
	agora = agora.Add(9 * time.Second)
	h.reactiveHit(a, b, 10, combat.Normal, true)
	if b.ResolutoPoints != 0 {
		t.Error("a marca deveria expirar depois de 8 s")
	}
}

func TestInvisibilidadeLargaAsReliquias(t *testing.T) {
	h, a, ae, _ := prepSelf(t, buffInvAtirador, gamedata.T_SkillData{Duration: 30})
	h.Data.(*tabelaDeItens).poeItem(900, gamedata.ItemFromList{ItemType: 40})
	h.MapObjs = NewMapObjects()
	a.Inventory.Slots[3] = model.Item{Index: 900}
	lanca(t, h, a, ae, buffInvAtirador, 0, a.Position)
	if !a.Inventory.Slots[3].IsEmpty() {
		t.Error("a reliquia deveria ter ido para o chao")
	}
	if h.MapObjs.Count() != 1 {
		t.Errorf("objetos no chao = %d, queria 1", h.MapObjs.Count())
	}
	if !a.Base.Buffs.Has(buffInvAtirador) {
		t.Error("falta o buff de invisibilidade")
	}
}
