package mail

import (
	"strings"
	"testing"
	"time"

	"aika/internal/model"
)

func carta(index uint64, de string) Mail {
	return Mail{Index: index, Nick: de, Title: "oi", Text: "texto"}
}

// ---- caixa

// O bug central da origem: AddMail chamava GetMails (que nao le nada e
// devolve True), deixava MailCount em zero, fazia SetLength(Mails, 1) e
// gravava o arquivo com UMA carta. Cada carta recebida apagava a caixa.
func TestAddNaoApagaAsAnteriores(t *testing.T) {
	var b Box
	for i := uint64(1); i <= 5; i++ {
		if err := b.Add(carta(i, "Alfa")); err != nil {
			t.Fatalf("carta %d: %v", i, err)
		}
	}
	if b.Count() != 5 {
		t.Fatalf("%d cartas, queria 5", b.Count())
	}
	// Todas as cinco seguem achaveis.
	for i := uint64(1); i <= 5; i++ {
		if b.Get(i) == nil {
			t.Errorf("carta %d deveria estar na caixa", i)
		}
	}
}

// Na origem GetMail varria um vetor que GetMails nao preencheu: ler correio
// nao funcionava de jeito nenhum.
func TestGetAchaACarta(t *testing.T) {
	var b Box
	b.Add(carta(10, "Alfa"))
	b.Add(carta(20, "Beta"))

	if m := b.Get(20); m == nil || m.Nick != "Beta" {
		t.Errorf("= %v", m)
	}
	if b.Get(99) != nil {
		t.Error("carta inexistente deveria dar nil")
	}
	if b.Get(0) != nil {
		t.Error("indice zero deveria dar nil")
	}
}

// Caixa vazia nao quebra: na origem o laco
// "for i := Length(Mails) - 1 downto 0" com i BYTE partia de -1, que num BYTE
// e 255, e indexava um vetor nulo.
func TestCaixaVaziaNaoQuebra(t *testing.T) {
	var b Box
	if b.Count() != 0 || b.Unread() != 0 {
		t.Error("caixa nova deveria estar vazia")
	}
	if b.Get(1) != nil {
		t.Error("nao deveria achar nada")
	}
	if b.Remove(1) {
		t.Error("nao deveria remover nada")
	}
	if b.Expired(time.Now()) != nil {
		t.Error("nada vencido")
	}
	if b.Latest(32) != nil && len(b.Latest(32)) != 0 {
		t.Error("nenhuma carta recente")
	}
	var nulo *Box
	if nulo.Count() != 0 || nulo.Unread() != 0 || nulo.Get(1) != nil {
		t.Error("caixa nil deveria responder vazio")
	}
	if nulo.Remove(1) || nulo.Expired(time.Now()) != nil || nulo.Latest(5) != nil {
		t.Error("caixa nil deveria responder vazio")
	}
	if err := nulo.Add(carta(1, "A")); err == nil {
		t.Error("Add em nil deveria falhar")
	}
}

func TestRemove(t *testing.T) {
	var b Box
	b.Add(carta(1, "A"))
	b.Add(carta(2, "B"))
	b.Add(carta(3, "C"))

	if !b.Remove(2) {
		t.Error("deveria ter removido")
	}
	if b.Count() != 2 {
		t.Errorf("%d cartas", b.Count())
	}
	if b.Get(2) != nil {
		t.Error("a carta removida nao deveria aparecer")
	}
	if b.Get(1) == nil || b.Get(3) == nil {
		t.Error("as outras deveriam ter ficado")
	}
	if b.Remove(2) {
		t.Error("nao deveria remover duas vezes")
	}
}

func TestUnread(t *testing.T) {
	var b Box
	b.Add(carta(1, "A"))
	m2 := carta(2, "B")
	m2.Checked = true
	b.Add(m2)
	b.Add(carta(3, "C"))

	if got := b.Unread(); got != 2 {
		t.Errorf("= %d, queria 2", got)
	}
}

func TestTetoDaCaixa(t *testing.T) {
	b := &Box{Mails: make([]Mail, MaxMails)}
	if err := b.Add(carta(1, "A")); err != ErrMailboxFull {
		t.Errorf("erro = %v, queria ErrMailboxFull", err)
	}
}

// Latest devolve as ultimas, da mais nova para a mais antiga, como a janela
// de SendRefreshMails.
func TestLatest(t *testing.T) {
	var b Box
	for i := uint64(1); i <= 40; i++ {
		b.Add(carta(i, "A"))
	}
	ultimas := b.Latest(MailsPerPacket)
	if len(ultimas) != MailsPerPacket {
		t.Fatalf("%d cartas, queria %d", len(ultimas), MailsPerPacket)
	}
	// A primeira da lista e a mais nova.
	if ultimas[0].Index != 40 {
		t.Errorf("primeira = %d, queria 40", ultimas[0].Index)
	}
	if ultimas[31].Index != 9 {
		t.Errorf("ultima = %d, queria 9", ultimas[31].Index)
	}
	// Caixa menor que a janela devolve tudo.
	var b2 Box
	b2.Add(carta(1, "A"))
	if n := len(b2.Latest(32)); n != 1 {
		t.Errorf("%d cartas, queria 1", n)
	}
	if b.Latest(0) != nil || b.Latest(-1) != nil {
		t.Error("janela nao positiva deveria dar nil")
	}
}

// ---- anexos

func TestAnexos(t *testing.T) {
	var m Mail
	if m.HasItems() || m.ItemCount() != 0 || !m.Empty() {
		t.Error("carta nova nao tem anexo")
	}
	m.Items[0] = model.Item{Index: 100}
	m.Items[3] = model.Item{Index: 200}
	if !m.HasItems() || m.ItemCount() != 2 {
		t.Errorf("anexos = %d", m.ItemCount())
	}
	if m.Empty() {
		t.Error("carta com anexo nao esta vazia")
	}
	// So ouro tambem nao e vazia.
	var m2 Mail
	m2.Gold = 100
	if m2.Empty() {
		t.Error("carta com ouro nao esta vazia")
	}
	var nulo *Mail
	if nulo.HasItems() || nulo.ItemCount() != 0 || !nulo.Empty() {
		t.Error("carta nil deveria responder vazio")
	}
}

// ---- prazo

func TestExpired(t *testing.T) {
	var b Box
	t0 := time.Now()
	m1 := carta(1, "A")
	m1.ReturnAt = t0.Add(-time.Hour)
	b.Add(m1)
	m2 := carta(2, "B")
	m2.ReturnAt = t0.Add(time.Hour)
	b.Add(m2)
	// Carta sem prazo nao vence.
	b.Add(carta(3, "C"))

	venceram := b.Expired(t0)
	if len(venceram) != 1 || venceram[0] != 1 {
		t.Errorf("= %v", venceram)
	}
}

// ---- carta nova

func TestNewCortaOsCampos(t *testing.T) {
	var conteudo model.MailContent
	copy(conteudo.Titulo[:], strings.Repeat("t", 100))
	copy(conteudo.Texto[:], strings.Repeat("x", 600))
	conteudo.Gold = 1234

	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := New(7, strings.Repeat("n", 50), conteudo, t0)

	if m.Index != 7 {
		t.Errorf("indice = %d", m.Index)
	}
	if len(m.Nick) != NickLen-1 {
		t.Errorf("nick com %d caracteres, queria %d", len(m.Nick), NickLen-1)
	}
	if len(m.Title) != TitleLen-1 {
		t.Errorf("titulo com %d caracteres, queria %d", len(m.Title), TitleLen-1)
	}
	if len(m.Text) != TextLen-1 {
		t.Errorf("texto com %d caracteres, queria %d", len(m.Text), TextLen-1)
	}
	if m.Gold != 1234 {
		t.Errorf("ouro = %d", m.Gold)
	}
	if !m.SentAt.Equal(t0) {
		t.Errorf("envio = %v", m.SentAt)
	}
	// O prazo de retorno e 15 dias, como IncDay(Now, 15).
	quer := t0.AddDate(0, 0, ReturnDays)
	if !m.ReturnAt.Equal(quer) {
		t.Errorf("retorno = %v, queria %v", m.ReturnAt, quer)
	}
}

// Campo terminado em zero antes do fim e lido ate o zero.
func TestNewLeAteOZero(t *testing.T) {
	var conteudo model.MailContent
	copy(conteudo.Titulo[:], "curto\x00lixo depois")
	m := New(1, "Alfa", conteudo, time.Now())
	if m.Title != "curto" {
		t.Errorf("titulo = %q", m.Title)
	}
}

func TestCustoEPrazoSaoOsDaOrigem(t *testing.T) {
	if SendCost != 500 {
		t.Errorf("custo = %d, a origem cobra 500", SendCost)
	}
	if ReturnDays != 15 {
		t.Errorf("prazo = %d dias, a origem usa 15", ReturnDays)
	}
	if MailsPerPacket != 32 {
		t.Errorf("janela = %d, o pacote leva 32", MailsPerPacket)
	}
	if MaxItems != 5 {
		t.Errorf("anexos = %d, o pacote leva 5", MaxItems)
	}
}
