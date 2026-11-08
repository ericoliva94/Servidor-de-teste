package game

import (
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
	"aika/internal/skillfn"
)

// prepCleric poe uma cleriga (a) e um aliado ferido (b) do mesmo grupo, perto
// um do outro, com a tabela de skill pronta para o teste preencher.
func prepCleric(t *testing.T) (*Handler, *tabelaDeSkills, *Session, *espiao, *Session, *espiao, *time.Time) {
	t.Helper()
	h, tab, agora := handlerComSkills(t)
	h.Durations = skillfn.BuffSeconds{T: tab}
	h.Rand = randFixo{v: 50} // cura aleatoria = (50+20)/2+35 = 70
	ae, be := novoEspiao(t), novoEspiao(t)
	a := lutador("Clerica", 1000, 1000)
	a.Avatar.ClassInfo, a.Avatar.Nation = 1, 1
	a.Avatar.AtkMag = 400
	b := lutador("Aliado", 1002, 1000)
	b.Avatar.Nation = 1
	b.Avatar.CureTax = 0
	if _, err := h.Chan.Join(ae.conn, a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Chan.Join(be.conn, b); err != nil {
		t.Fatal(err)
	}
	a.Base.VisiblePlayers.Add(b.ClientID)
	b.Base.VisiblePlayers.Add(a.ClientID)
	g, _ := h.Parties.Create(a.ClientID)
	_ = h.Parties.Add(g, b.ClientID)
	b.Avatar.CurHP = 100
	ae.drena(10 * time.Millisecond)
	be.drena(10 * time.Millisecond)
	return h, tab, a, ae, b, be, agora
}

func aprende(a *Session, idx int) {
	a.Skills.Others[0] = model.SkillFromList{Index: uint16(idx), Level: 5}
}

func TestCuraRecuperaVidaDoAliadoEAvisa(t *testing.T) {
	h, tab, a, ae, b, be, _ := prepCleric(t)
	tab.poe(skCura, gamedata.T_SkillData{Classe: 1, Damage: 100, DamageRange: 10})
	aprende(a, skCura)
	lanca(t, h, a, ae, skCura, uint32(b.ClientID), a.Position)
	// 400/2 + 100 = 300, mais 70 do sorteio fixo.
	if want := uint32(100 + 370); b.Avatar.CurHP != want {
		t.Errorf("HP = %d, queria %d", b.Avatar.CurHP, want)
	}
	if got := mensagemDe(t, be); got != "Seu HP foi restaurado em 370 por [Clerica]." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestCuraNaoPassaDoMaximoENaoCuraMorto(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	tab.poe(skCura, gamedata.T_SkillData{Classe: 1, Damage: 100000, DamageRange: 10})
	aprende(a, skCura)
	lanca(t, h, a, ae, skCura, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != b.Avatar.MaxHP {
		t.Errorf("HP = %d, maximo %d", b.Avatar.CurHP, b.Avatar.MaxHP)
	}
	b.Avatar.CurHP, b.Dead = 0, true
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skCura, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP != 0 {
		t.Error("curou um jogador morto")
	}
}

func TestCuraUsaOCureTaxEODecure(t *testing.T) {
	h, tab, a, _, b, _, _ := prepCleric(t)
	tab.poe(skCura, gamedata.T_SkillData{Classe: 1, Damage: 100})
	d := skillfn.Data(h.SkillTable, skCura)
	base := h.cureValue(a, b, 0, d.Damage, true)
	b.Avatar.CureTax = 50
	if com := h.cureValue(a, b, 0, d.Damage, true); com <= base {
		t.Errorf("CureTax nao somou: %d vs %d", com, base)
	}
	b.Avatar.CureTax = 0
	b.Avatar.Ability[efDecure] = 50
	if menos := h.cureValue(a, b, 0, d.Damage, true); menos >= base {
		t.Errorf("EF_DECURE nao reduziu: %d vs %d", menos, base)
	}
}

func TestAnticuraTrocaACuraPorDanoPorTresVezes(t *testing.T) {
	h, tab, a, ae, b, be, _ := prepCleric(t)
	tab.poe(skCura, gamedata.T_SkillData{Classe: 1, Damage: 100, DamageRange: 10})
	tab.poe(buffAnticure, gamedata.T_SkillData{Duration: 60})
	aprende(a, skCura)
	b.Avatar.CurHP = 1000
	h.addBuff(b, buffAnticure)
	b.Avatar.Ability[efAnticure] = 20
	be.drena(10 * time.Millisecond)
	for i := 0; i < 3; i++ {
		a.Base.Cooldowns.Clear()
		antes := b.Avatar.CurHP
		lanca(t, h, a, ae, skCura, uint32(b.ClientID), a.Position)
		if b.Avatar.CurHP >= antes {
			t.Fatalf("cura %d: HP %d -> %d, devia cair", i+1, antes, b.Avatar.CurHP)
		}
	}
	if b.Base.Buffs.Has(buffAnticure) {
		t.Error("o buff 88 deveria sair depois de tres curas")
	}
}

func TestLimparERecuperacaoTiramDebuffs(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	tab.poe(600, gamedata.T_SkillData{Duration: 60, BuffDebuff: 3})
	tab.poe(601, gamedata.T_SkillData{Duration: 60, BuffDebuff: 4})
	tab.poe(602, gamedata.T_SkillData{Duration: 60, BuffDebuff: 1})
	tab.poe(skLimpar, gamedata.T_SkillData{Classe: 1, DamageRange: 10})
	tab.poe(skRecuperacao, gamedata.T_SkillData{Classe: 1, Damage: 10, DamageRange: 10})
	a.Skills.Others[0] = model.SkillFromList{Index: skLimpar, Level: 5}
	a.Skills.Others[1] = model.SkillFromList{Index: skRecuperacao, Level: 5}
	for _, id := range []uint16{600, 601, 602} {
		h.addBuff(b, id)
	}
	lanca(t, h, a, ae, skLimpar, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(600) || !b.Base.Buffs.Has(601) || !b.Base.Buffs.Has(602) {
		t.Errorf("Limpar tira so um debuff: 600=%v 601=%v 602=%v",
			b.Base.Buffs.Has(600), b.Base.Buffs.Has(601), b.Base.Buffs.Has(602))
	}
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skRecuperacao, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Has(601) || !b.Base.Buffs.Has(602) {
		t.Error("Recuperacao tira todos os debuffs e preserva os buffs")
	}
	if b.Avatar.CurHP <= antes {
		t.Error("Recuperacao tambem cura")
	}
}

func TestMaoDeCuraRegeneraACadaSegundoEPara(t *testing.T) {
	h, tab, a, ae, b, _, agora := prepCleric(t)
	tab.poe(skMaoDeCura, gamedata.T_SkillData{Classe: 1, Damage: 0, DamageRange: 10, Duration: 10,
		EFV: [4]int32{300}})
	aprende(a, skMaoDeCura)
	b.Avatar.CurHP = 100
	lanca(t, h, a, ae, skMaoDeCura, uint32(b.ClientID), a.Position)
	r, ok := b.regen[skMaoDeCura]
	if !ok || r.hp <= 0 {
		t.Fatalf("regen = %+v ok=%v", r, ok)
	}
	antes := b.Avatar.CurHP
	h.BuffTick()
	h.BuffTick()
	if b.Avatar.CurHP != antes+2*uint32(r.hp) {
		t.Errorf("HP = %d, queria %d", b.Avatar.CurHP, antes+2*uint32(r.hp))
	}
	*agora = agora.Add(11 * time.Second)
	h.BuffTick()
	if _, ok := b.regen[skMaoDeCura]; ok || b.Base.Buffs.Has(skMaoDeCura) {
		t.Error("o buff venceu mas a regeneracao continua")
	}
	parado := b.Avatar.CurHP
	h.BuffTick()
	if b.Avatar.CurHP != parado {
		t.Error("regenerou sem buff")
	}
}

func TestLibertacaoDeManaRegeneraMP(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	tab.poe(skLibertacaoMana, gamedata.T_SkillData{Classe: 1, DamageRange: 10, Duration: 30,
		EFV: [4]int32{40}})
	aprende(a, skLibertacaoMana)
	b.Avatar.CurMP = 0
	lanca(t, h, a, ae, skLibertacaoMana, uint32(b.ClientID), a.Position)
	h.BuffTick()
	if b.Avatar.CurMP == 0 {
		t.Error("MP nao regenerou")
	}
}

func TestCuraPreventivaCuraQuandoOBuffAcaba(t *testing.T) {
	h, tab, a, _, _, _, agora := prepCleric(t)
	tab.poe(skCuraPreventiva, gamedata.T_SkillData{Duration: 5, Damage: 100})
	a.Avatar.CurHP = 10
	h.addBuff(a, skCuraPreventiva)
	if a.Avatar.CurHP != 10 {
		t.Fatal("curou antes da hora")
	}
	*agora = agora.Add(6 * time.Second)
	h.BuffTick()
	if a.Avatar.CurHP <= 10 {
		t.Error("o buff acabou sem curar")
	}
}

func TestRessurreicaoLevantaMembroDoGrupo(t *testing.T) {
	h, tab, a, ae, b, be, _ := prepCleric(t)
	tab.poe(skRessurreicao, gamedata.T_SkillData{Classe: 1, Damage: 50, DamageRange: 10})
	aprende(a, skRessurreicao)
	b.Dead, b.Avatar.CurHP = true, 0
	lanca(t, h, a, ae, skRessurreicao, uint32(b.ClientID), a.Position)
	if b.Dead || b.Avatar.CurHP != b.Avatar.MaxHP/100*50 {
		t.Errorf("dead=%v HP=%d (queria %d)", b.Dead, b.Avatar.CurHP, b.Avatar.MaxHP/100*50)
	}
	if got := mensagemDe(t, be); got != "Voce foi ressuscitado pelo jogador Clerica." {
		t.Errorf("mensagem = %q", got)
	}

	// Fora do grupo nao levanta.
	h.Parties.Remove(b.ClientID)
	b.Dead, b.Avatar.CurHP = true, 0
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skRessurreicao, uint32(b.ClientID), a.Position)
	if !b.Dead {
		t.Error("ressuscitou quem nao e do grupo")
	}
}

func TestAureaCuraEDaBuff(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	tab.poe(skAureaEsplendor, gamedata.T_SkillData{Classe: 1, Damage: 50, DamageRange: 10, Duration: 30,
		EF: [4]int32{efDamage1}, EFV: [4]int32{20}})
	aprende(a, skAureaEsplendor)
	antes := b.Avatar.CurHP
	lanca(t, h, a, ae, skAureaEsplendor, uint32(b.ClientID), a.Position)
	if b.Avatar.CurHP <= antes || !b.Base.Buffs.Has(skAureaEsplendor) {
		t.Errorf("HP %d->%d buff=%v", antes, b.Avatar.CurHP, b.Base.Buffs.Has(skAureaEsplendor))
	}
}

func TestCuraEmAreaChegaAoGrupoPerto(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	tab.poe(skFloresGloria, gamedata.T_SkillData{Classe: 1, Range: 20, Damage: 50})
	aprende(a, skFloresGloria)
	a.Avatar.CurHP = 10
	longe := lutador("Longe", 5000, 5000)
	longe.Avatar.CurHP = 5
	le := novoEspiao(t)
	h.Chan.Join(le.conn, longe)
	g := h.Parties.Of(a.ClientID)
	_ = h.Parties.Add(g, longe.ClientID)
	a.Base.VisiblePlayers.Add(longe.ClientID)
	lanca(t, h, a, ae, skFloresGloria, 0, a.Position)
	if a.Avatar.CurHP <= 10 || b.Avatar.CurHP <= 100 {
		t.Errorf("autor=%d aliado=%d deveriam ter sido curados", a.Avatar.CurHP, b.Avatar.CurHP)
	}
	if longe.Avatar.CurHP != 5 {
		t.Error("membro fora do raio foi curado")
	}
}

func TestTodosDebuffsEmSiMesmo(t *testing.T) {
	h, tab, a, ae, _, _, _ := prepCleric(t)
	tab.poe(600, gamedata.T_SkillData{Duration: 60, BuffDebuff: 3})
	tab.poe(skTodosDebuffs, gamedata.T_SkillData{Classe: 1, TargetType: 1})
	aprende(a, skTodosDebuffs)
	h.addBuff(a, 600)
	lanca(t, h, a, ae, skTodosDebuffs, 0, a.Position)
	if a.Base.Buffs.Has(600) {
		t.Error("o debuff deveria ter sido removido")
	}
}

func TestAjustesDeDanoDaClerica(t *testing.T) {
	flecha := &gamedata.T_SkillData{Index: skFlecha, Adicional: 25}
	if got := clericAdjust(flecha, 1000); got != 1250 {
		t.Errorf("Flecha Sagrada = %d, queria 1250", got)
	}
	retorno := &gamedata.T_SkillData{Index: skRetornoMagica, Adicional: 70}
	if got := clericAdjust(retorno, 1000); got != 1070 {
		t.Errorf("Retorno de Magia = %d", got)
	}
	if got := clericAdjust(&gamedata.T_SkillData{Index: 1}, 1000); got != 1000 {
		t.Errorf("skill comum mudou o dano: %d", got)
	}
}

func TestRetornoDeMagiaTiraBuffsDoAlvo(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	b.Avatar.Nation = 2
	tab.poe(602, gamedata.T_SkillData{Duration: 60, BuffDebuff: 1})
	tab.poe(603, gamedata.T_SkillData{Duration: 60, BuffDebuff: 1})
	tab.poe(skRetornoMagica, gamedata.T_SkillData{Classe: 1, SuccessRate: 1, Damage: 1, Adicional: 50, DamageRange: 10})
	aprende(a, skRetornoMagica)
	h.addBuff(b, 602)
	h.addBuff(b, 603)
	h.Parties.Remove(b.ClientID)
	lanca(t, h, a, ae, skRetornoMagica, uint32(b.ClientID), a.Position)
	if b.Base.Buffs.Count() != 1 {
		t.Errorf("buffs restantes = %d, queria 1 (Damage=1 tira um)", b.Base.Buffs.Count())
	}
}

func TestSensorMagicoNaoCausaDanoERevelaInvisivel(t *testing.T) {
	h, tab, a, ae, b, _, _ := prepCleric(t)
	b.Avatar.Nation = 2
	tab.poe(buffInvAtirador, gamedata.T_SkillData{Duration: 60})
	tab.poe(skSensorMagico, gamedata.T_SkillData{Classe: 1, SuccessRate: 1, Range: 20, Damage: 900, DamageRange: 10})
	aprende(a, skSensorMagico)
	h.Parties.Remove(b.ClientID)
	h.addBuff(b, buffInvAtirador)
	b.Avatar.CurHP = b.Avatar.MaxHP
	lanca(t, h, a, ae, skSensorMagico, 0, a.Position)
	if b.Avatar.CurHP != b.Avatar.MaxHP {
		t.Errorf("Sensor Magico causou dano: %d", b.Avatar.CurHP)
	}
	if b.Base.Buffs.Has(buffInvAtirador) {
		t.Error("a invisibilidade deveria ter sido revelada")
	}
}

// Reviver tira os buffs desfazendo os efeitos.
func TestLimparBuffsDesfazOsEfeitos(t *testing.T) {
	h, tab, a, _, _, _, _ := prepCleric(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efDamage1}, EFV: [4]int32{70}})
	h.addBuff(a, 500)
	h.clearBuffs(a)
	if a.Base.Buffs.Count() != 0 || a.Avatar.Ability.get(efDamage1) != 0 {
		t.Errorf("buffs=%d ability=%d", a.Base.Buffs.Count(), a.Avatar.Ability.get(efDamage1))
	}
}
