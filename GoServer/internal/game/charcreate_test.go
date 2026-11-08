package game

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	"aika/internal/charcreate"
	"aika/internal/model"
	"aika/internal/packets"
	"aika/internal/store"
)

type fakeCreator struct {
	mu      sync.Mutex
	criados []store.NewCharacter
	contas  []uint32
	bolsa   int
	err     error
	proxID  uint32
}

func (f *fakeCreator) Create(_ context.Context, acc uint32, n store.NewCharacter) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.criados, f.contas = append(f.criados, n), append(f.contas, acc)
	if f.proxID == 0 {
		f.proxID = 500
	}
	return f.proxID, nil
}

func (f *fakeCreator) EnsureStorageBag(context.Context, uint32, uint16, int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bolsa++
	return nil
}

// fakeSaveItems so grava equipamento e inventario; o resto da interface fica
// nil (o teste nao os chama).
type fakeSaveItems struct {
	ItemSource
	equip, inv store.ItemSet
	falhaInv   error
}

func (f *fakeSaveItems) SaveEquip(_ context.Context, _ uint32, s store.ItemSet) error {
	f.equip = s
	return nil
}

func (f *fakeSaveItems) SaveInventory(_ context.Context, _ uint32, s store.ItemSet) error {
	if f.falhaInv != nil {
		return f.falhaInv
	}
	f.inv = s
	return nil
}

type fakeSaveSkills struct {
	SkillStore
	lista model.SkillsList
	vezes int
}

func (f *fakeSaveSkills) Save(_ context.Context, _ uint32, l model.SkillsList) error {
	f.lista, f.vezes = l, f.vezes+1
	return nil
}

// charsDepois devolve, depois da criacao, a lista com o personagem novo no slot.
type charsDepois struct{ slot int }

func (c charsDepois) ByAccount(context.Context, uint32) ([store.MaxCharacterSlots]store.Character, []string, error) {
	var out [store.MaxCharacterSlots]store.Character
	out[c.slot] = store.Character{ID: 500, Name: "Novo", Slot: c.slot}
	return out, nil, nil
}

func prepCriacao(t *testing.T) (*Handler, *fakeCreator, *Session, *espiao) {
	t.Helper()
	h, s, e := prepMisc(t)
	f := &fakeCreator{}
	h.CharCreate = f
	h.CharDelete = &fakeDeleter{}
	h.Chars = charsDepois{slot: 1}
	h.Items = &fakeSaveItems{}
	h.SkillDB = &fakeSaveSkills{}
	h.ItemBarDB = &fakeItemBars{}
	for i := range h.Templates {
		h.Templates[i] = charcreate.Template{Loaded: true, ClassInfo: uint8(1 + 10*i), Str: 15, Agi: 9, Int: 5, Cons: 16, Level: 1}
	}
	s.Status = CharList
	s.Authenticated = true
	s.Account.ID = 7
	return h, f, s, e
}

func criar(t *testing.T, h *Handler, s *Session, e *espiao, nome string, slot uint32, classe, cabelo uint16, local uint32) {
	t.Helper()
	p := packets.CreateCharacterRequestPacket{SlotIndex: slot, ClassIndex: classe, Cabelo: cabelo, Local: local}
	copy(p.Name[:], nome)
	if err := h.createCharacter(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeCreateCharacter, p))); err != nil {
		t.Fatal(err)
	}
}

func TestCriarPersonagemGravaTudoEReenviaALista(t *testing.T) {
	h, f, s, e := prepCriacao(t)
	criar(t, h, s, e, "Novo", 1, 30, 7710, 1) // atirador

	if len(f.criados) != 1 || f.contas[0] != 7 {
		t.Fatalf("criacoes = %d conta=%v", len(f.criados), f.contas)
	}
	n := f.criados[0]
	if n.Name != "Novo" || n.Slot != 1 || n.ClassInfo != 21 || n.Str != 15 || n.Level != 1 ||
		n.PosX != 3443 || n.PosY != 912 {
		t.Errorf("personagem = %+v", n)
	}
	its := h.Items.(*fakeSaveItems)
	if its.equip.Slots[0].Index != 30 || its.equip.Slots[1].Index != 7710 || its.equip.Slots[15].Index != charcreate.ItemBulletSniper {
		t.Errorf("equip = %+v", its.equip.Slots[:2])
	}
	if its.inv.Slots[60].Index != charcreate.ItemStarterBag || its.inv.Slots[5].Refi != 1000 {
		t.Errorf("inventario = bolsa %d bala %d", its.inv.Slots[60].Index, its.inv.Slots[5].Refi)
	}
	if sk := h.SkillDB.(*fakeSaveSkills); sk.vezes != 1 || sk.lista.Basics[0].Index == 0 {
		t.Errorf("skills = %+v", sk)
	}
	if f.bolsa != 1 {
		t.Errorf("expansao do armazem = %d vez(es)", f.bolsa)
	}
	if s.Characters[1].ID != 500 || s.Characters[1].Name != "Novo" {
		t.Errorf("lista da sessao = %+v", s.Characters[1])
	}
	proximoDe(t, e, CodeCharList)
}

func TestCriarPersonagemRecusasDeEntrada(t *testing.T) {
	casos := []struct {
		nome           string
		slot           uint32
		classe, cabelo uint16
		pers           string
		msg            string
	}{
		{"nome com espaco", 0, 10, 7700, "Al fa", "Voce so pode usar caracteres alfanumericos."},
		{"nome vazio", 0, 10, 7700, "", "Voce so pode usar caracteres alfanumericos."},
		{"nome com aspas", 0, 10, 7700, "a'b", "Voce so pode usar caracteres alfanumericos."},
		{"nome longo", 0, 10, 7700, "123456789012345", "Limitado a 14 caracteres apenas."},
		{"slot 3", 3, 10, 7700, "Alfa", "SLOT_ERROR"},
		{"slot enorme", 0xFFFFFFFF, 10, 7700, "Alfa", "SLOT_ERROR"},
		{"classe 9", 0, 9, 7700, "Alfa", "class_id error, try to create your toon again."},
		{"classe 70", 0, 70, 7700, "Alfa", "class_id error, try to create your toon again."},
		{"cabelo baixo", 0, 10, 7699, "Alfa", "Cabelo invalido."},
		{"cabelo alto", 0, 10, 7732, "Alfa", "Cabelo invalido."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h, f, s, e := prepCriacao(t)
			criar(t, h, s, e, c.pers, c.slot, c.classe, c.cabelo, 0)
			if got := mensagemDe(t, e); got != c.msg {
				t.Errorf("mensagem = %q, queria %q", got, c.msg)
			}
			if len(f.criados) != 0 {
				t.Error("recusa chegou ao banco")
			}
		})
	}
}

func TestCriarPersonagemErrosDoBanco(t *testing.T) {
	casos := []struct {
		err error
		msg string
	}{
		{store.ErrCharLimit, "Voce ja tem 3 personagens."},
		{store.ErrNameTaken, "Ja existe um personagem com esse nome."},
		{store.ErrSlotTaken, "Esse slot ja esta ocupado."},
		{errors.New("mysql caiu"), "Nao foi possivel criar o personagem."},
	}
	for _, c := range casos {
		h, f, s, e := prepCriacao(t)
		f.err = c.err
		criar(t, h, s, e, "Novo", 0, 10, 7700, 0)
		if got := mensagemDe(t, e); got != c.msg {
			t.Errorf("%v: mensagem = %q, queria %q", c.err, got, c.msg)
		}
		if s.Characters[1].ID == 500 {
			t.Error("a lista da sessao mudou apesar do erro")
		}
	}
}

// Se uma etapa depois do INSERT falha, o personagem nao pode ficar pela
// metade no banco: a origem deixava.
func TestCriarPersonagemDesfazSeAGravacaoDosItensFalha(t *testing.T) {
	h, f, s, e := prepCriacao(t)
	h.Items.(*fakeSaveItems).falhaInv = errors.New("mysql caiu")
	criar(t, h, s, e, "Novo", 0, 10, 7700, 0)
	if got := mensagemDe(t, e); got != "Nao foi possivel criar o personagem." {
		t.Errorf("mensagem = %q", got)
	}
	d := h.CharDelete.(*fakeDeleter)
	if len(d.apagados) != 1 || d.apagados[0] != 500 {
		t.Errorf("personagem incompleto nao foi apagado: %v", d.apagados)
	}
	if f.bolsa != 0 {
		t.Error("a expansao do armazem nao deve ser criada num personagem desfeito")
	}
}

func TestCriarPersonagemSemModeloOuSemBanco(t *testing.T) {
	h, f, s, e := prepCriacao(t)
	h.Templates[charcreate.ClassWarrior] = charcreate.Template{}
	criar(t, h, s, e, "Novo", 0, 10, 7700, 0)
	if got := mensagemDe(t, e); got != "Criacao de personagem indisponivel." {
		t.Errorf("sem modelo: %q", got)
	}
	h.CharCreate = nil
	criar(t, h, s, e, "Novo", 0, 20, 7700, 0)
	if got := mensagemDe(t, e); got != "Criacao de personagem indisponivel." {
		t.Errorf("sem banco: %q", got)
	}
	if len(f.criados) != 0 {
		t.Error("criou sem condicoes")
	}
}

func TestCriarPersonagemExigeSessaoNaListaDePersonagens(t *testing.T) {
	h, _, s, e := prepCriacao(t)
	s.Authenticated = false
	err := h.createCharacter(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeCreateCharacter, packets.CreateCharacterRequestPacket{})))
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("erro = %v", err)
	}
	s.Authenticated, s.Status = true, Playing
	err = h.createCharacter(context.Background(), e.conn, s, asPacket(t, miscPacket(CodeCreateCharacter, packets.CreateCharacterRequestPacket{})))
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("em jogo: erro = %v", err)
	}
}

func TestTamanhoDoPacoteDeCriacao(t *testing.T) {
	// Header 12 + AcountID 4 + SlotIndex 4 + Name 16 + Class 2 + Cabelo 2 +
	// Null 12 + Local 4 = 56.
	if n := binary.Size(packets.CreateCharacterRequestPacket{}); n != 56 {
		t.Errorf("CreateCharacterRequestPacket = %d, o Delphi tem 56", n)
	}
}
