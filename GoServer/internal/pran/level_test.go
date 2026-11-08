package pran

import "testing"

func TestSubirDeNivelDaVidaEMana(t *testing.T) {
	p := &Pran{Level: 3, MaxHP: 1000, MaxMP: 2000, CurHP: 1, CurMP: 2}
	if !p.LevelUp() || p.Level != 4 || p.MaxHP != 1209 || p.MaxMP != 2356 || p.CurHP != 1209 || p.CurMP != 2356 {
		t.Errorf("pran = %+v", p)
	}
	p.Level = MaxLevel
	if p.LevelUp() || p.Level != MaxLevel {
		t.Error("no teto nao sobe")
	}
	if (&Pran{Level: 7}).DisplayLevel() != 8 {
		t.Error("o cliente ve Level+1")
	}
}

func TestMarcoDeNivelSobeHabilidadesEAvancaABarra(t *testing.T) {
	p := &Pran{Level: 3} // sobe para 4, mostra 5
	p.Skills[0] = Skill{Index: 500, Level: 1}
	p.Skills[1] = Skill{Index: 600, Level: 1}
	p.Skills[2] = Skill{Index: 0, Level: 1} // indice 0: nunca "esta na barra"
	p.Skills[3] = Skill{Index: 700, Level: 1}
	p.ItemBar = [BarSlots]uint32{0, 600, 500} // a barra vazia (0) nao pode casar com o indice 0
	p.LevelUp()
	bar := p.ApplyMilestone()
	if p.Skills[0].Level != 2 || p.Skills[1].Level != 2 || p.Skills[2].Level != 2 || p.Skills[3].Level != 1 {
		t.Errorf("niveis = %+v", p.Skills[:4])
	}
	if len(bar) != 2 || bar[0] != (BarChange{Bar: 2, NewIndex: 501}) || bar[1] != (BarChange{Bar: 1, NewIndex: 601}) {
		t.Errorf("barra = %+v", bar)
	}
	if p.Skills[2].Index != 0 {
		t.Error("indice 0 nao deve avancar")
	}
	// Nivel sem marco nao muda nada.
	p.LevelUp()
	if b := p.ApplyMilestone(); b != nil || p.Skills[0].Level != 2 {
		t.Error("nivel 6 nao e marco")
	}
}

func TestMarcos(t *testing.T) {
	for _, lv := range []int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50} {
		p2 := &Pran{Level: uint8(lv - 1)}
		for i := range p2.Skills {
			p2.Skills[i].Level = 1
		}
		p2.ApplyMilestone()
		n := 0
		for _, s := range p2.Skills {
			if s.Level == 2 {
				n++
			}
		}
		want := map[int]int{5: 3, 10: 4, 15: 5, 20: 6, 25: 7, 30: 8, 35: 9, 40: 9, 45: 9, 50: 8}[lv]
		if n != want {
			t.Errorf("marco %d subiu %d habilidades, queria %d", lv, n, want)
		}
	}
	// O slot 3 nunca sobe, e o 0 deixa de subir no 50.
	p := &Pran{Level: 49}
	p.Skills[0].Level, p.Skills[3].Level = 1, 1
	p.ApplyMilestone()
	if p.Skills[0].Level != 1 || p.Skills[3].Level != 1 {
		t.Error("no 50 os slots 0 e 3 ficam")
	}
}

func TestFronteirasDeEvolucao(t *testing.T) {
	casos := []struct {
		lvl   uint8
		class uint8
		want  bool
	}{
		{4, 61, true}, {4, 62, false}, {19, 72, true}, {19, 61, false},
		{49, 83, true}, {49, 63, true}, {20, 83, false}, {3, 61, false},
	}
	for _, k := range casos {
		if got := (&Pran{Level: k.lvl, Class: k.class}).ClassNeedsEvolution(); got != k.want {
			t.Errorf("nivel %d classe %d = %v", k.lvl, k.class, got)
		}
	}
}

func TestComidaDePersonalidadeEDevocao(t *testing.T) {
	p := &Pran{Cute: 5, Smart: 2, Sexy: 10, Energetic: 3, Tough: 4, Corrupt: 9, Devotion: 10, Food: 100}
	if !p.Feed(8106) { // perfait de cereja: sexy
		t.Fatal("deveria comer")
	}
	if p.Sexy != 12 || p.Cute != 2 || p.Smart != 0 || p.Energetic != 0 || p.Tough != 1 || p.Corrupt != 6 {
		t.Errorf("personalidade = %+v", p)
	}
	if p.Devotion != 11 || p.Food != 115 {
		t.Errorf("devocao %d comida %d", p.Devotion, p.Food)
	}
	p.Feed(8110)
	if p.Food != MaxFood {
		t.Errorf("comida passa do teto: %d", p.Food)
	}
	if p.Feed(8105) || p.Food != MaxFood {
		t.Error("cheia nao come")
	}
	// Comida comum (fora de 8105..8110) so enche.
	q := &Pran{Food: 10, Devotion: 5, Cute: 7}
	q.Feed(8000)
	if q.Food != 25 || q.Devotion != 5 || q.Cute != 7 {
		t.Errorf("comida comum mexeu na personalidade: %+v", q)
	}
	// Devocao para em 226.
	d := &Pran{Devotion: 226}
	d.Feed(8105)
	if d.Devotion != 226 {
		t.Errorf("devocao = %d", d.Devotion)
	}
}

func TestDigestivo(t *testing.T) {
	p := &Pran{Food: 13}
	if p.Digest() {
		t.Error("com 13 ou menos nao pode")
	}
	p.Food = 61
	if !p.Digest() || p.Food != 30 {
		t.Errorf("comida = %d", p.Food)
	}
}
