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

// prepBuff devolve um handler com tabela de skill e duracoes reais, e um jogador.
func prepBuff(t *testing.T) (*Handler, *tabelaDeSkills, *Session, *espiao, *time.Time) {
	t.Helper()
	h, tab, agora := handlerComSkills(t)
	h.Durations = skillfn.BuffSeconds{T: tab}
	e := novoEspiao(t)
	s := lutador("Alfa", 1000, 1000)
	s.Avatar.ClassInfo = 1
	s.Equip.Slots[WeaponSlot] = model.Item{Index: 1000, Min: 100, Max: 100, Refi: 16}
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	e.drena(10 * time.Millisecond)
	return h, tab, s, e, agora
}

func TestBuffSomaOsEfeitosDaTabelaNosAtributos(t *testing.T) {
	h, tab, s, e, _ := prepBuff(t)
	// EF 2 = EF_DAMAGE1 (dano fisico plano), EF 50 = critico, EF 53 = acerto.
	tab.poe(500, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efDamage1, efCritical, efHit}, EFV: [4]int32{70, 20, 12}})
	atk, crit, hit := s.Avatar.AtkFis, s.Avatar.Critical, s.Avatar.Acerto

	if !h.addBuff(s, 500) {
		t.Fatal("addBuff recusou")
	}
	if s.Avatar.AtkFis != atk+70 {
		t.Errorf("AtkFis = %d, queria %d", s.Avatar.AtkFis, atk+70)
	}
	if s.Avatar.Critical != crit+20 || s.Avatar.Acerto != hit+12 {
		t.Errorf("critico=%d acerto=%d", s.Avatar.Critical, s.Avatar.Acerto)
	}
	// O cliente recebe o aviso do buff novo, a lista e os numeros.
	got := decodifica[packets.UpdateBuffPacket](t, proximoDe(t, e, CodeUpdateBuff))
	if got.Buff != 500 || got.EndTime == 0 {
		t.Errorf("$16F = %+v", got)
	}
	proximoDe(t, e, CodeRefreshBuffs)

	h.removeBuffAndNotify(s, 500)
	if s.Avatar.AtkFis != atk || s.Avatar.Critical != crit || s.Avatar.Acerto != hit {
		t.Errorf("depois de remover: atk=%d crit=%d hit=%d", s.Avatar.AtkFis, s.Avatar.Critical, s.Avatar.Acerto)
	}
}

func TestRecastarBuffNaoAcumulaOEfeito(t *testing.T) {
	h, tab, s, _, agora := prepBuff(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efDamage1}, EFV: [4]int32{70}})
	base := s.Avatar.AtkFis
	h.addBuff(s, 500)
	*agora = agora.Add(10 * time.Second)
	h.addBuff(s, 500)
	if s.Avatar.AtkFis != base+70 {
		t.Errorf("AtkFis = %d, queria %d", s.Avatar.AtkFis, base+70)
	}
	if s.Base.Buffs.Count() != 1 {
		t.Errorf("buffs = %d", s.Base.Buffs.Count())
	}
	if ini, _ := s.Base.Buffs.StartedAt(500); !ini.Equal(*agora) {
		t.Error("o prazo deveria ter sido renovado")
	}
}

// O bug principal: buff nunca expirava.
func TestBuffVencidoSaiEDesfazOEfeito(t *testing.T) {
	h, tab, s, e, agora := prepBuff(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efDamage1}, EFV: [4]int32{70}})
	base := s.Avatar.AtkFis
	h.addBuff(s, 500)
	e.drena(10 * time.Millisecond)

	*agora = agora.Add(30 * time.Second)
	if n := h.BuffTick(); n != 0 || !s.Base.Buffs.Has(500) {
		t.Fatalf("saiu cedo: n=%d", n)
	}
	*agora = agora.Add(31 * time.Second)
	if n := h.BuffTick(); n != 1 {
		t.Fatalf("removidos = %d, queria 1", n)
	}
	if s.Base.Buffs.Has(500) || s.Avatar.AtkFis != base {
		t.Errorf("buff=%v atk=%d (base %d)", s.Base.Buffs.Has(500), s.Avatar.AtkFis, base)
	}
	proximoDe(t, e, CodeRefreshBuffs)
}

func TestDebuffComValorNegativoEDesfeitoSemSobra(t *testing.T) {
	h, tab, s, _, _ := prepBuff(t)
	tab.poe(501, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efHit}, EFV: [4]int32{-5}})
	hit := s.Avatar.Acerto
	h.addBuff(s, 501)
	if s.Avatar.Ability.get(efHit) != -5 {
		t.Fatalf("ability = %d", s.Avatar.Ability.get(efHit))
	}
	h.removeBuffAndNotify(s, 501)
	if s.Avatar.Ability.get(efHit) != 0 || s.Avatar.Acerto != hit {
		t.Errorf("ability=%d acerto=%d (base %d)", s.Avatar.Ability.get(efHit), s.Avatar.Acerto, hit)
	}
}

func TestBuffDeHPSobeOMaximo(t *testing.T) {
	h, tab, s, _, _ := prepBuff(t)
	tab.poe(502, gamedata.T_SkillData{Duration: 60, EF: [4]int32{efHP, efPerHP}, EFV: [4]int32{1000, 10}})
	antes := s.Avatar.MaxHP
	h.addBuff(s, 502)
	if s.Avatar.MaxHP <= antes+1000 {
		t.Errorf("MaxHP = %d, antes %d", s.Avatar.MaxHP, antes)
	}
	h.removeBuffAndNotify(s, 502)
	if s.Avatar.MaxHP != antes {
		t.Errorf("MaxHP depois = %d, queria %d", s.Avatar.MaxHP, antes)
	}
}

func TestLimiteDeBuffsAvisaOJogador(t *testing.T) {
	h, tab, s, e, _ := prepBuff(t)
	for i := 1; i <= 61; i++ {
		tab.poe(1000+i, gamedata.T_SkillData{Duration: 60})
	}
	for i := 1; i <= 60; i++ { // direto na lista: sem 60 rodadas de pacotes
		if !s.Base.Buffs.Set(uint16(1000+i), h.now()) {
			t.Fatalf("buff %d recusado", i)
		}
	}
	if h.addBuff(s, 1061) {
		t.Error("passou do limite de 60")
	}
	if got := mensagemDe(t, e); got != "Nao foi possivel adicionar novos buffs. Limite: 60 Buffs." {
		t.Errorf("mensagem = %q", got)
	}
	// Recastar um que ja existe continua valendo com a lista cheia.
	if !h.addBuff(s, 1001) {
		t.Error("recastar com a lista cheia deveria funcionar")
	}
}

func TestIndicesQueAOrigemRecusa(t *testing.T) {
	h, tab, s, _, _ := prepBuff(t)
	tab.poe(7257, gamedata.T_SkillData{Duration: 60})
	tab.poe(9133, gamedata.T_SkillData{Duration: 60})
	if h.addBuff(s, 7257) || h.addBuff(s, 9133) || s.Base.Buffs.Count() != 0 {
		t.Error("buff recusado pela origem entrou")
	}
}

func TestBuffDeClasseUsadoPorSkillChegaAoClienteComEfeito(t *testing.T) {
	h, tab, s, e, _ := prepBuff(t)
	tab.poe(500, gamedata.T_SkillData{Duration: 60, Classe: 1, MP: 10,
		EF: [4]int32{efDamage1}, EFV: [4]int32{70}})
	s.Skills.Others[0] = model.SkillFromList{Index: 500, Level: 5}
	base := s.Avatar.AtkFis
	if err := h.useSkill(context.Background(), e.conn, s, asPacket(t, useSkillPacket(500, 0))); err != nil {
		t.Fatal(err)
	}
	if s.Avatar.AtkFis != base+70 {
		t.Errorf("AtkFis = %d, queria %d", s.Avatar.AtkFis, base+70)
	}
	proximoDe(t, e, CodeUpdateBuff)
}

// Trocar de personagem na mesma conexao nao herda buffs nem efeitos.
func TestAbilityZeraNoAvatarNovo(t *testing.T) {
	var ab Ability
	ab.increase(efDamage1, 9)
	ab.decrease(efDamage1, 100) // piso em zero (decInt)
	if ab.get(efDamage1) != 0 {
		t.Errorf("ability = %d", ab.get(efDamage1))
	}
	ab.increase(-1, 5)
	ab.increase(AbilitySize, 5)
	ab.increase(0, 5) // EF_NONE nao acumula
	if ab.get(-1) != 0 || ab.get(AbilitySize) != 0 || ab.get(0) != 0 {
		t.Error("indice invalido acumulou")
	}
}
