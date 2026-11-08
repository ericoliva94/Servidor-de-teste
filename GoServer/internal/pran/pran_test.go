package pran

import "testing"

func TestPersonalidadeUltimaQueAtingeADevocao(t *testing.T) {
	p := &Pran{Devotion: 10, Cute: 3, Smart: 12, Sexy: 4, Energetic: 11, Tough: 1, Corrupt: 2}
	if got := p.Personality(); got != 3 { // Smart(1) e Energetic(3) passam; a ultima ganha
		t.Errorf("personalidade = %d, queria 3", got)
	}
	p = &Pran{Devotion: 10}
	if got := p.Personality(); got != 0 {
		t.Errorf("nenhuma passa = %d, queria 0", got)
	}
	p = &Pran{Devotion: 0} // tudo >= 0: a ultima
	if got := p.Personality(); got != 5 {
		t.Errorf("devocao 0 = %d, queria 5", got)
	}
}

func TestNiveisDeHabilidadeEmpacotados(t *testing.T) {
	var p Pran
	p.Skills[0] = Skill{Index: 1, Level: 3} // l=7, 1 byte
	p.Skills[1] = Skill{Index: 1, Level: 2} // l=3, a=4 -> 12
	p.Skills[2] = Skill{Index: 1, Level: 8} // l=255, a=16 -> 4080 (2 bytes: F0 0F)
	p.Skills[3] = Skill{Index: 1, Level: 1} // l=1, a=81 -> 81, reescreve o 0F
	got := p.PackSkills()
	want := [16]byte{7, 12, 0xF0, 81}
	if got != want {
		t.Errorf("empacotado = %v, queria %v", got, want)
	}
	// Habilidade sem nivel nao escreve.
	if (&Pran{}).PackSkills() != [16]byte{} {
		t.Error("pran sem habilidades deveria dar zeros")
	}
}

func TestFada(t *testing.T) {
	for _, c := range []uint8{61, 71, 81} {
		if !IsFairy(c) {
			t.Errorf("classe %d e fada", c)
		}
	}
	if IsFairy(62) || IsFairy(0) {
		t.Error("62 e 0 nao sao fada")
	}
}

func TestIDsDePran(t *testing.T) {
	r := NewIDs()
	a, ok := r.Alloc(5)
	if !ok || a != FirstClientID {
		t.Fatalf("primeiro id = %d ok=%v", a, ok)
	}
	b, _ := r.Alloc(6)
	if b != FirstClientID+1 {
		t.Errorf("segundo id = %d", b)
	}
	r.Free(a, 6) // outro dono nao libera
	if o, ok := r.Owner(a); !ok || o != 5 {
		t.Error("so o dono libera")
	}
	r.Free(a, 5)
	if c, _ := r.Alloc(7); c != a {
		t.Errorf("id liberado deveria voltar: %d", c)
	}
	if _, ok := r.Alloc(0); ok {
		t.Error("dono zero nao aloca")
	}
	// Esgota a faixa.
	for i := 0; i < LastClientID-FirstClientID+2; i++ {
		if _, ok := r.Alloc(9); !ok {
			return
		}
	}
	t.Error("a faixa deveria esgotar")
}
