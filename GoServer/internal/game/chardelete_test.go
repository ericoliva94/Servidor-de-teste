package game

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"aika/internal/packets"
	"aika/internal/store"
)

type fakeDeleter struct {
	mu        sync.Mutex
	agendas   []bool
	quando    []time.Time
	apagados  []uint32
	falhaDel  error
	falhaMark error
}

func (f *fakeDeleter) SetDeleteSchedule(_ context.Context, _, _ uint32, d bool, w time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falhaMark != nil {
		return f.falhaMark
	}
	f.agendas, f.quando = append(f.agendas, d), append(f.quando, w)
	return nil
}

func (f *fakeDeleter) Delete(_ context.Context, _, charID uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falhaDel != nil {
		return f.falhaDel
	}
	f.apagados = append(f.apagados, charID)
	return nil
}

func prepExclusao(t *testing.T) (*Handler, *fakeDeleter, *Session, *espiao) {
	t.Helper()
	h, s, e := prepMisc(t)
	f := &fakeDeleter{}
	h.CharDelete = f
	s.Status = CharList
	s.Authenticated = true
	s.Account.ID = 7
	s.Characters[0] = store.Character{ID: 42, Name: "Alfa", NumericToken: "1234"}
	return h, f, s, e
}

func numeric4(s string) [4]byte {
	var n [4]byte
	copy(n[:], s)
	return n
}

func pedeExclusao(t *testing.T, h *Handler, s *Session, e *espiao, slot uint32, apagar uint32, num string) {
	t.Helper()
	if err := h.requestDeleteChar(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeRequestDeleteChar,
		packets.DeleteCharacterRequestPacket{SlotIndex: slot, Delete: apagar, Numeric: numeric4(num)}))); err != nil {
		t.Fatal(err)
	}
}

func apagaDeVez(t *testing.T, h *Handler, s *Session, e *espiao, slot uint32, num string) {
	t.Helper()
	if err := h.deleteChar(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeDeleteChar,
		packets.DeleteChar{Slot: slot, Numeric: numeric4(num)}))); err != nil {
		t.Fatal(err)
	}
}

// ---- pedir e cancelar

func TestPedirExclusaoMarcaParaDaquiAUmDia(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	pedeExclusao(t, h, s, e, 0, 0, "1234")

	ch := s.Characters[0]
	if !ch.Deleted || !ch.DeleteTime.Equal(h.now().AddDate(0, 0, 1)) {
		t.Errorf("personagem = deleted %v em %v", ch.Deleted, ch.DeleteTime)
	}
	if len(f.agendas) != 1 || !f.agendas[0] {
		t.Errorf("banco = %v", f.agendas)
	}
	proximoDe(t, e, CodeCharList)
}

func TestCancelarExclusao(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(time.Hour)

	pedeExclusao(t, h, s, e, 0, 1, "1234")

	if s.Characters[0].Deleted || !s.Characters[0].DeleteTime.IsZero() {
		t.Error("a exclusao deveria ter sido cancelada")
	}
	if len(f.agendas) != 1 || f.agendas[0] {
		t.Errorf("banco = %v", f.agendas)
	}
	if got := mensagemDe(t, e); got != "Exclusao de personagem cancelada!" {
		t.Errorf("mensagem = %q", got)
	}
}

func TestPedirExclusaoComNumericaErradaContaErroENaoMarca(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	pedeExclusao(t, h, s, e, 0, 0, "9999")

	if s.Characters[0].Deleted || len(f.agendas) != 0 {
		t.Error("marcou com a numerica errada")
	}
	if s.Characters[0].NumericErrors != 1 {
		t.Errorf("erros = %d, queria 1", s.Characters[0].NumericErrors)
	}
	if got := mensagemDe(t, e); got != "Numerica nao correspondente!" {
		t.Errorf("mensagem = %q", got)
	}
}

// Defeito da origem: o pedido de exclusao deixava testar as numericas sem
// limite. Agora soma no contador da entrada e tranca.
func TestPedirExclusaoTrancaDepoisDeMuitosErros(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	s.Characters[0].NumericErrors = MaxNumericErrors
	pedeExclusao(t, h, s, e, 0, 0, "1234") // ate a numerica certa e recusada
	if s.Characters[0].Deleted || len(f.agendas) != 0 {
		t.Error("personagem trancado foi marcado para exclusao")
	}
}

func TestPedirExclusaoRecusas(t *testing.T) {
	h, f, s, e := prepExclusao(t)

	s.Characters[1] = store.Character{} // slot vazio
	pedeExclusao(t, h, s, e, 1, 0, "1234")
	if got := mensagemDe(t, e); got != "Parece que esse personagem nao existe!" {
		t.Errorf("slot vazio: %q", got)
	}

	s.Characters[2] = store.Character{ID: 43, Name: "Beta"} // sem numerica
	pedeExclusao(t, h, s, e, 2, 0, "1234")
	if got := mensagemDe(t, e); got != "Voce nao possui uma numerica. Crie uma." {
		t.Errorf("sem numerica: %q", got)
	}

	// Slot fora de 0..2 (a origem indexava o vetor com o DWORD do cliente).
	pedeExclusao(t, h, s, e, 3, 0, "1234")
	pedeExclusao(t, h, s, e, 0xFFFFFFFF, 0, "1234")
	e.nada(t, 20*time.Millisecond)
	if len(f.agendas) != 0 {
		t.Error("recusa chegou ao banco")
	}
}

func TestPedirExclusaoComPrazoVencidoMantemOAvisoDaOrigem(t *testing.T) {
	h, _, s, e := prepExclusao(t)
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(-time.Hour)
	pedeExclusao(t, h, s, e, 0, 0, "1234")
	if got := mensagemDe(t, e); got != "Voce ainda nao pode excluir este personagem!" {
		t.Errorf("mensagem = %q", got)
	}
}

func TestFalhaDoBancoAoMarcarNaoMudaASessao(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	f.falhaMark = errors.New("mysql caiu")
	pedeExclusao(t, h, s, e, 0, 0, "1234")
	if s.Characters[0].Deleted {
		t.Error("a sessao ficou marcada apesar da falha do banco")
	}
}

// ---- excluir de vez

func TestExcluirDeVezDepoisDoPrazo(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(-time.Minute)

	apagaDeVez(t, h, s, e, 0, "1234")

	if len(f.apagados) != 1 || f.apagados[0] != 42 {
		t.Errorf("apagados = %v", f.apagados)
	}
	if s.Characters[0].ID != 0 {
		t.Error("o slot deveria ter sido esvaziado")
	}
	proximoDe(t, e, CodeCharList)
	if got := mensagemDe(t, e); got != "Personagem deletado com sucesso!" {
		t.Errorf("mensagem = %q", got)
	}
}

func TestExcluirDeVezRecusas(t *testing.T) {
	casos := []struct {
		nome  string
		setup func(h *Handler, s *Session)
		slot  uint32
		num   string
		msg   string
	}{
		{"antes do prazo", func(h *Handler, s *Session) {
			s.Characters[0].Deleted = true
			s.Characters[0].DeleteTime = h.now().Add(time.Hour)
		}, 0, "1234", "Voce ainda nao pode excluir este personagem!"},
		{"sem data de exclusao", func(h *Handler, s *Session) { s.Characters[0].Deleted = true }, 0, "1234",
			"Voce ainda nao pode excluir este personagem!"},
		{"nao marcado", func(h *Handler, s *Session) {}, 0, "1234", "Voce nao pode deletar esse personagem!"},
		{"slot vazio", func(h *Handler, s *Session) {}, 1, "1234", "Voce nao pode deletar esse personagem!"},
		{"numerica errada", func(h *Handler, s *Session) {
			s.Characters[0].Deleted = true
			s.Characters[0].DeleteTime = h.now().Add(-time.Minute)
		}, 0, "0000", "Numerica nao correspondente!"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, f, s, e := prepExclusao(t)
			c.setup(h, s)
			apagaDeVez(t, h, s, e, c.slot, c.num)
			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if len(f.apagados) != 0 || s.Characters[0].ID != 42 {
				t.Error("recusa apagou algo")
			}
		})
	}
}

// Defeito da origem: BYTE(Slot) transformava o slot 256 em 0 e apagava o
// primeiro personagem.
func TestExcluirDeVezComSlotQueEstouraOByteNaoApagaOSlotZero(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(-time.Minute)
	apagaDeVez(t, h, s, e, 256, "1234")
	apagaDeVez(t, h, s, e, 0xFFFFFF00, "1234")
	apagaDeVez(t, h, s, e, 3, "1234")
	if len(f.apagados) != 0 || s.Characters[0].ID != 42 {
		t.Error("slot invalido apagou o personagem 0")
	}
}

func TestExcluirDeVezSemNumericaDefinidaNaoPedeNumerica(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	s.Characters[0].NumericToken = ""
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(-time.Minute)
	apagaDeVez(t, h, s, e, 0, "")
	if len(f.apagados) != 1 {
		t.Error("como na origem, sem numerica a conferencia e pulada")
	}
}

func TestFalhaDoBancoAoExcluirMantemOPersonagem(t *testing.T) {
	h, f, s, e := prepExclusao(t)
	f.falhaDel = errors.New("mysql caiu")
	s.Characters[0].Deleted = true
	s.Characters[0].DeleteTime = h.now().Add(-time.Minute)
	apagaDeVez(t, h, s, e, 0, "1234")
	if s.Characters[0].ID != 42 {
		t.Error("o personagem sumiu da sessao apesar da falha do banco")
	}
	if got := mensagemDe(t, e); got != "Nao foi possivel excluir o personagem." {
		t.Errorf("mensagem = %q", got)
	}
}

func TestExclusaoExigeSessaoAutenticadaForaDoJogo(t *testing.T) {
	h, _, s, e := prepExclusao(t)
	s.Authenticated = false
	err := h.requestDeleteChar(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeRequestDeleteChar,
		packets.DeleteCharacterRequestPacket{})))
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("erro = %v", err)
	}
	s.Authenticated, s.Status = true, Playing
	err = h.deleteChar(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeDeleteChar, packets.DeleteChar{})))
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("em jogo: erro = %v", err)
	}
}

func TestTamanhosDosPacotesDeExclusao(t *testing.T) {
	if n := binary.Size(packets.DeleteCharacterRequestPacket{}); n != 28 {
		t.Errorf("DeleteCharacterRequestPacket = %d, o Delphi tem 28", n)
	}
	if n := binary.Size(packets.DeleteChar{}); n != 56 {
		t.Errorf("DeleteChar = %d, o Delphi tem 56", n)
	}
}
