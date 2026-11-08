package game

import "testing"

// Ao subir de nivel o cliente recebe, nesta ordem, o que TPlayer.AddLevel manda:
// HP/MP ($103), nivel ($108), pontos ($109) e o efeito de nivel. O $10A nao vai.
func TestSubirDeNivelMandaOsPacotesDaOrigemNaOrdem(t *testing.T) {
	h, _, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Avatar.Level = 3
	h.addExp(e.conn, s, 100000, ExpTypeNormal)
	if s.Avatar.Level <= 3 {
		t.Fatal("deveria subir de nivel")
	}
	want := []uint16{CodeCurrentHPMP, CodeCurrentLevel, CodeRefreshPoint, CodeSendEffect}
	for i, code := range want {
		if got := e.espera(t); got != code {
			t.Fatalf("pacote %d = %#x, queria %#x", i, got, code)
		}
	}
	e.nada(t, 50*1e6)
}

func TestGanharExpSemSubirSoMandaONivel(t *testing.T) {
	h, _, _ := handlerParaUso(t)
	e := novoEspiao(t)
	s := sessaoComArmazem("Alfa", 11)
	if _, err := h.Chan.Join(e.conn, s); err != nil {
		t.Fatal(err)
	}
	s.Avatar.Level = 3
	h.addExp(e.conn, s, 1, ExpTypeNormal)
	if got := e.espera(t); got != CodeCurrentLevel {
		t.Fatalf("pacote = %#x", got)
	}
	e.nada(t, 50*1e6)
}
