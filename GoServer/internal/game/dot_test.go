package game

import (
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/model"
)

// prepDot: autor (a) lanca no inimigo (b); a skill idx entra na tabela como
// dano em um alvo (SuccessRate 1), com duracao.
func prepDot(t *testing.T, idx int, d gamedata.T_SkillData) (*Handler, *Session, *espiao, *Session, *time.Time) {
	t.Helper()
	d.SuccessRate, d.DamageRange, d.Classe = 1, 20, 1
	h, a, ae, b, _ := prepArea(t, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(idx, d)
	a.Skills.Others[0] = model.SkillFromList{Index: uint16(idx), Level: 5}
	a.Avatar.AtkFis, a.Avatar.AtkMag = 400, 600
	b.Avatar.CurHP = b.Avatar.MaxHP
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	return h, a, ae, b, &agora
}

func TestDotCausaDanoACadaSegundoENaoMata(t *testing.T) {
	h, a, ae, b, agora := prepDot(t, skSacrificio, gamedata.T_SkillData{Duration: 5, EFV: [4]int32{20}})
	lanca(t, h, a, ae, skSacrificio, uint32(b.ClientID), a.Position)
	st, ok := b.dots[skSacrificio]
	if !ok || st.dmg <= 0 {
		t.Fatalf("dot = %+v ok=%v", st, ok)
	}
	b.Avatar.CurHP = b.Avatar.MaxHP
	h.BuffTick()
	h.BuffTick()
	if want := b.Avatar.MaxHP - 2*uint32(st.dmg); b.Avatar.CurHP != want {
		t.Errorf("HP = %d, queria %d", b.Avatar.CurHP, want)
	}
	// Nunca mata: com pouca vida fica em 1.
	b.Avatar.CurHP = 3
	h.BuffTick()
	if b.Avatar.CurHP != 1 || b.Dead {
		t.Errorf("HP = %d dead=%v, deveria ficar em 1", b.Avatar.CurHP, b.Dead)
	}
	// Acaba com o buff.
	*agora = agora.Add(6 * time.Second)
	h.BuffTick()
	if len(b.dots) != 0 {
		t.Error("o dano continuo deveria acabar com o buff")
	}
	b.Avatar.CurHP = 500
	h.BuffTick()
	if b.Avatar.CurHP != 500 {
		t.Error("causou dano depois do buff")
	}
}

func TestVenenoDividePelaDuracao(t *testing.T) {
	h, a, ae, b, _ := prepDot(t, skVeneno55, gamedata.T_SkillData{Duration: 10, EFV: [4]int32{100}})
	lanca(t, h, a, ae, skVeneno55, uint32(b.ClientID), a.Position)
	// vida cheia: sorteia 60..119; randFixo(50) -> 110; (100+110)/10 = 21
	if got := b.dots[skVeneno55].dmg; got != 21 {
		t.Errorf("dano por segundo = %d, queria 21", got)
	}
}

func TestRaioSolarUsaMetadeDoEFVMaisSorteio(t *testing.T) {
	h, a, ae, b, _ := prepDot(t, skRaioSolar, gamedata.T_SkillData{Duration: 10, EFV: [4]int32{100}})
	lanca(t, h, a, ae, skRaioSolar, uint32(b.ClientID), a.Position)
	// vida cheia: 30..58, randFixo(50) -> 30+IntN(29)=30+28=58 (teto); 100/2+58
	if got := b.dots[skRaioSolar].dmg; got < 50+30 || got > 50+58 {
		t.Errorf("dano = %d, fora de [80,108]", got)
	}
	antes := b.Avatar.CurHP
	if antes != b.Avatar.MaxHP {
		t.Error("o proprio golpe do Raio Solar nao causa dano")
	}
}

func TestRecastarDotNaoAcumulaEDoisDotsConvivem(t *testing.T) {
	h, a, ae, b, _ := prepDot(t, skSacrificio, gamedata.T_SkillData{Duration: 5, EFV: [4]int32{20}})
	h.SkillTable.(*tabelaDeSkills).poe(skVeneno55, gamedata.T_SkillData{Classe: 1, SuccessRate: 1,
		DamageRange: 20, Duration: 5, EFV: [4]int32{50}})
	a.Skills.Others[1] = model.SkillFromList{Index: skVeneno55, Level: 5}
	lanca(t, h, a, ae, skSacrificio, uint32(b.ClientID), a.Position)
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skSacrificio, uint32(b.ClientID), a.Position)
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, skVeneno55, uint32(b.ClientID), a.Position)
	if len(b.dots) != 2 {
		t.Errorf("dots = %d, queria 2 (a origem guardava um so por autor)", len(b.dots))
	}
}

func TestCancelarOBuffParaODot(t *testing.T) {
	h, a, ae, b, _ := prepDot(t, skSacrificio, gamedata.T_SkillData{Duration: 30, EFV: [4]int32{20}})
	lanca(t, h, a, ae, skSacrificio, uint32(b.ClientID), a.Position)
	h.removeBuffAndNotify(b, skSacrificio)
	if len(b.dots) != 0 {
		t.Error("dot continua depois de remover o buff")
	}
}

func TestDotEmMobAteAcabarENaoMata(t *testing.T) {
	h, a, ae, _, agora := prepDot(t, skSacrificio, gamedata.T_SkillData{Duration: 3, EFV: [4]int32{50}})
	h.Mobs = NewMobWorld(tabelaDeMobs(especie("Bicho", 100000, 1, 1000, 1000)))
	h.Exp = expFixa{}
	m, ok := h.Mobs.Get(3048)
	if !ok {
		t.Fatal("mob inexistente")
	}
	lanca(t, h, a, ae, skSacrificio, 3048, a.Position)
	antes := m.CurHP
	h.mobDotTick()
	h.mobDotTick()
	if m.CurHP >= antes {
		t.Fatalf("mob nao sofreu dano continuo: %d -> %d", antes, m.CurHP)
	}
	// Nunca mata.
	m.CurHP = 5
	h.mobDotTick()
	if m.CurHP != 1 || m.Dead {
		t.Errorf("HP do mob = %d dead=%v", m.CurHP, m.Dead)
	}
	*agora = agora.Add(4 * time.Second)
	h.mobDotTick()
	m.CurHP = 900
	h.mobDotTick()
	if m.CurHP != 900 {
		t.Error("o dano continuo no mob deveria ter acabado")
	}
}

func TestBuffQueFereAoAcabar(t *testing.T) {
	h, a, _, _, agora := prepDot(t, buffMjolnir, gamedata.T_SkillData{Duration: 5, EFV: [4]int32{40}})
	h.addBuff(a, buffMjolnir)
	a.Avatar.CurHP = a.Avatar.MaxHP
	*agora = agora.Add(6 * time.Second)
	h.BuffTick()
	perda := a.Avatar.MaxHP - a.Avatar.CurHP
	if perda < 15+40 || perda > 15+74+40 {
		t.Errorf("perda = %d, fora de [55, 129]", perda)
	}
	// Nao mata.
	h.addBuff(a, buffMjolnir)
	a.Avatar.CurHP = 2
	h.removeBuffAndNotify(a, buffMjolnir)
	if a.Avatar.CurHP != 1 || a.Dead {
		t.Errorf("HP = %d dead=%v", a.Avatar.CurHP, a.Dead)
	}
}

func TestAguaBentaCriaOItemENaoCabeSemEspaco(t *testing.T) {
	h, a, ae, _, _ := prepDot(t, 457, gamedata.T_SkillData{})
	h.SkillTable.(*tabelaDeSkills).poe(457, gamedata.T_SkillData{Classe: 1, TargetType: 1, Damage: 7777})
	lanca(t, h, a, ae, 457, 0, a.Position)
	if contaItem(a, 7777) != 1 {
		t.Fatalf("agua benta no inventario = %d", contaItem(a, 7777))
	}
	for i := 0; i < MovableInventorySlots; i++ {
		if a.Inventory.Slots[i].IsEmpty() {
			a.Inventory.Slots[i] = model.Item{Index: 5}
		}
	}
	a.Base.Cooldowns.Clear()
	lanca(t, h, a, ae, 457, 0, a.Position)
	if got := mensagemDe(t, ae); got != "Inventario cheio." {
		t.Errorf("mensagem = %q", got)
	}
}
