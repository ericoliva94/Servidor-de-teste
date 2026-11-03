package game

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aika/internal/gamedata"
	"aika/internal/mail"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// correioFalso guarda o correio em memoria.
type correioFalso struct {
	mu       sync.Mutex
	caixas   map[uint32]*mail.Box
	nomes    map[string]uint32
	proximo  uint64
	erroAdd  error
	erroSave error
	erroBox  error
}

func novoCorreio() *correioFalso {
	return &correioFalso{
		caixas:  map[uint32]*mail.Box{},
		nomes:   map[string]uint32{},
		proximo: 0,
	}
}

func (f *correioFalso) poeNome(nome string, id uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nomes[strings.ToLower(nome)] = id
}

func (f *correioFalso) Box(_ context.Context, charID uint32) (*mail.Box, error) {
	if f.erroBox != nil {
		return nil, f.erroBox
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.caixas[charID]
	if !ok {
		b = &mail.Box{}
		f.caixas[charID] = b
	}
	return b, nil
}

func (f *correioFalso) Add(_ context.Context, nome string, m mail.Mail) error {
	if f.erroAdd != nil {
		return f.erroAdd
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.nomes[strings.ToLower(nome)]
	if !ok {
		return mail.ErrInvalidTarget
	}
	b, ok := f.caixas[id]
	if !ok {
		b = &mail.Box{}
		f.caixas[id] = b
	}
	return b.Add(m)
}

func (f *correioFalso) Save(_ context.Context, charID uint32, m mail.Mail) error {
	if f.erroSave != nil {
		return f.erroSave
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.caixas[charID]
	if b == nil {
		return mail.ErrMailNotFound
	}
	for i := range b.Mails {
		if b.Mails[i].Index == m.Index {
			b.Mails[i] = m
			return nil
		}
	}
	return mail.ErrMailNotFound
}

func (f *correioFalso) Remove(_ context.Context, charID uint32, index uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.caixas[charID]; b != nil {
		b.Remove(index)
	}
	return nil
}

func (f *correioFalso) MarkRead(_ context.Context, charID uint32, index uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.caixas[charID]; b != nil {
		if m := b.Get(index); m != nil {
			m.Checked = true
		}
	}
	return nil
}

func (f *correioFalso) NextIndex(_ context.Context) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proximo++
	return f.proximo, nil
}

func (f *correioFalso) CharacterExists(_ context.Context, nome string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.nomes[strings.ToLower(nome)]
	return ok, nil
}

// handlerComCorreio devolve um handler com correio em memoria.
func handlerComCorreio(t *testing.T) (*Handler, *correioFalso) {
	h := handlerComGuildas(t)
	f := novoCorreio()
	h.MailDB = f
	itens := novaTabelaDeItens()
	itens.poeItem(100, gamedata.ItemFromList{})
	itens.poeItem(9000, gamedata.ItemFromList{})
	h.Data = itens
	return h, f
}

func confirmMailPacket(nome string) []byte {
	var p packets.SendMailPacket
	netsrv.PutCString(p.Nick[:], nome)
	buf, err := netsrv.Marshal(CodeMailConfirm, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func sendMailPacket(para, titulo, texto string, ouro uint32, slots ...uint8) []byte {
	var p packets.ContentMailPacket
	netsrv.PutCString(p.Content.Nick[:], para)
	netsrv.PutCString(p.Content.Titulo[:], titulo)
	netsrv.PutCString(p.Content.Texto[:], texto)
	p.Content.Gold = ouro
	for i := range p.Content.ItemSlot {
		p.Content.ItemSlot[i] = 0xFF
	}
	for i, s := range slots {
		if i < len(p.Content.ItemSlot) {
			p.Content.ItemSlot[i] = s
		}
	}
	buf, err := netsrv.Marshal(CodeMailSend, p)
	if err != nil {
		panic(err)
	}
	return buf
}

func openMailPacket(code uint16, index uint64, tipo uint16) []byte {
	var p packets.RecvOpenMailPacket
	p.Index = index
	p.OpenType = tipo
	buf, err := netsrv.Marshal(code, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// ---- conferir destinatario

func TestConferirDestinatario(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Beta", 200)

	if err := h.confirmMail(context.Background(), ea.conn, a,
		asPacket(t, confirmMailPacket("Beta"))); err != nil {
		t.Fatal(err)
	}
	if a.CanSendMailTo != "Beta" {
		t.Errorf("CanSendMailTo = %q", a.CanSendMailTo)
	}
	vis := ea.proximo(t)
	if vis.code != CodeMailSend {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.SignalData
	netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt)
	if pkt.Data != BoolAccept {
		t.Errorf("resultado = %d, queria aceite", pkt.Data)
	}
}

// Destinatario que nao existe e recusado: a origem so guardava o nome e
// conferia depois, na gravacao, que criava o arquivo do destinatario mesmo
// inexistente.
func TestDestinatarioInexistente(t *testing.T) {
	h, _ := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)

	if err := h.confirmMail(context.Background(), ea.conn, a,
		asPacket(t, confirmMailPacket("NaoExiste"))); err != nil {
		t.Fatal(err)
	}
	if a.CanSendMailTo != "" {
		t.Errorf("CanSendMailTo = %q", a.CanSendMailTo)
	}
}

func TestNaoMandaCartaParaSiMesmo(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Alfa", 100)

	if err := h.confirmMail(context.Background(), ea.conn, a,
		asPacket(t, confirmMailPacket("alfa"))); err != nil {
		t.Fatal(err)
	}
	if a.CanSendMailTo != "" {
		t.Errorf("CanSendMailTo = %q", a.CanSendMailTo)
	}
}

func TestConferirSemOuro(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Avatar.Gold = 10
	f.poeNome("Beta", 200)

	if err := h.confirmMail(context.Background(), ea.conn, a,
		asPacket(t, confirmMailPacket("Beta"))); err != nil {
		t.Fatal(err)
	}
	if a.CanSendMailTo != "" {
		t.Errorf("CanSendMailTo = %q", a.CanSendMailTo)
	}
}

// ---- enviar

func TestEnviarCarta(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Beta", 200)
	a.CanSendMailTo = "Beta"
	ouroAntes := a.Avatar.Gold

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "assunto", "corpo", 5000))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 200)
	if caixa.Count() != 1 {
		t.Fatalf("%d cartas na caixa de Beta", caixa.Count())
	}
	m := caixa.Mails[0]
	if m.Nick != "Alfa" || m.Title != "assunto" || m.Text != "corpo" {
		t.Errorf("= %+v", m)
	}
	if m.Gold != 5000 {
		t.Errorf("ouro = %d", m.Gold)
	}
	// Custa 500 mais o ouro enviado.
	if a.Avatar.Gold != ouroAntes-500-5000 {
		t.Errorf("ouro do remetente = %d", a.Avatar.Gold)
	}
	// E o nome fica liberado.
	if a.CanSendMailTo != "" {
		t.Errorf("CanSendMailTo = %q", a.CanSendMailTo)
	}
}

// Duas cartas para o mesmo destinatario: a origem apagava a primeira.
func TestSegundaCartaNaoApagaAPrimeira(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Beta", 200)
	ea.drena(100 * time.Millisecond)

	for i, titulo := range []string{"primeira", "segunda", "terceira"} {
		a.CanSendMailTo = "Beta"
		if err := h.sendMail(context.Background(), ea.conn, a,
			asPacket(t, sendMailPacket("Beta", titulo, "corpo", 0))); err != nil {
			t.Fatalf("carta %d: %v", i, err)
		}
	}
	caixa, _ := f.Box(context.Background(), 200)
	if caixa.Count() != 3 {
		t.Fatalf("%d cartas, queria 3: a origem deixava 1", caixa.Count())
	}
	for i, titulo := range []string{"primeira", "segunda", "terceira"} {
		if caixa.Mails[i].Title != titulo {
			t.Errorf("carta %d = %q, queria %q", i, caixa.Mails[i].Title, titulo)
		}
	}
}

// Sem passar pelo $3F16 a carta nao sai.
func TestEnviarSemConferir(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Beta", 200)
	a.CanSendMailTo = ""

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 0))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 200)
	if caixa.Count() != 0 {
		t.Error("nao deveria ter enviado")
	}
}

// Destinatario diferente do conferido nao vale.
func TestEnviarParaOutroDestinatario(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	f.poeNome("Beta", 200)
	f.poeNome("Gama", 300)
	a.CanSendMailTo = "Beta"

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Gama", "x", "y", 0))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 300)
	if caixa.Count() != 0 {
		t.Error("nao deveria ter enviado para Gama")
	}
}

func TestEnviarComAnexo(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 100, Refi: 5}
	a.Inventory.Slots[1] = model.Item{Index: 200}
	f.poeNome("Beta", 200)
	a.CanSendMailTo = "Beta"

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 0, 0, 1))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 200)
	m := caixa.Mails[0]
	if m.Items[0].Index != 100 || m.Items[1].Index != 200 {
		t.Errorf("anexos = %d/%d", m.Items[0].Index, m.Items[1].Index)
	}
	if !m.CheckItem || !m.Return {
		t.Error("a carta deveria estar marcada com anexo")
	}
	// Os itens sairam do inventario.
	if !a.Inventory.Get(0).IsEmpty() || !a.Inventory.Get(1).IsEmpty() {
		t.Error("os itens deveriam ter saido do inventario")
	}
}

// Slot de anexo fora da faixa e ignorado.
func TestAnexoComSlotInvalido(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	f.poeNome("Beta", 200)
	a.CanSendMailTo = "Beta"

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 0, 200, 99, 64))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 200)
	if caixa.Count() != 1 {
		t.Fatalf("%d cartas", caixa.Count())
	}
	if caixa.Mails[0].HasItems() {
		t.Error("nenhum anexo deveria ter entrado")
	}
}

// Falha na gravacao nao pode tirar o item do jogador: na origem os itens
// saiam do inventario ANTES do AddMail, e um AddMail que falhasse devolvia
// False sem repor nada.
func TestFalhaAoGravarNaoPerdeOItem(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	a.Inventory.Slots[0] = model.Item{Index: 100, Refi: 5}
	f.poeNome("Beta", 200)
	f.erroAdd = errors.New("falha de teste")
	a.CanSendMailTo = "Beta"
	ouroAntes := a.Avatar.Gold

	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 1000, 0))); err != nil {
		t.Fatal(err)
	}
	if a.Inventory.Get(0).Index != 100 {
		t.Error("o item deveria ter ficado no inventario")
	}
	if a.Avatar.Gold != ouroAntes {
		t.Errorf("ouro = %d, nao deveria ter sido cobrado", a.Avatar.Gold)
	}
}

func TestEnviarSemOuroSuficiente(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Avatar.Gold = 600
	f.poeNome("Beta", 200)
	a.CanSendMailTo = "Beta"

	// 500 de taxa mais 1000 de anexo: nao cabe.
	if err := h.sendMail(context.Background(), ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 1000))); err != nil {
		t.Fatal(err)
	}
	caixa, _ := f.Box(context.Background(), 200)
	if caixa.Count() != 0 {
		t.Error("nao deveria ter enviado")
	}
	if a.Avatar.Gold != 600 {
		t.Errorf("ouro = %d", a.Avatar.Gold)
	}
}

// ---- ler

func TestLerCarta(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	caixa, _ := f.Box(context.Background(), 100)
	caixa.Add(mail.Mail{Index: 7, Nick: "Beta", Title: "assunto", Text: "corpo"})
	ea.drena(100 * time.Millisecond)

	if err := h.openMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailOpen, 7, MailOpenRead))); err != nil {
		t.Fatal(err)
	}
	vis := ea.proximo(t)
	if vis.code != CodeMailOpen {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.RecvOpenMailPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.Index != 7 || pkt.Index2 != 7 {
		t.Errorf("indices = %d/%d", pkt.Index, pkt.Index2)
	}
	if got := netsrv.CString(pkt.Nick[:]); got != "Beta" {
		t.Errorf("remetente = %q", got)
	}
	if got := netsrv.CString(pkt.Titulo[:]); got != "assunto" {
		t.Errorf("titulo = %q", got)
	}
	if got := netsrv.CString(pkt.Texto[:]); got != "corpo" {
		t.Errorf("texto = %q", got)
	}
	// E fica marcada como lida.
	if m := caixa.Get(7); m == nil || !m.Checked {
		t.Error("a carta deveria estar marcada como lida")
	}
}

func TestLerCartaInexistente(t *testing.T) {
	h, _ := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	ea.drena(100 * time.Millisecond)

	for _, idx := range []uint64{0, 1, 999, 1 << 40} {
		if err := h.openMail(context.Background(), ea.conn, a,
			asPacket(t, openMailPacket(CodeMailOpen, idx, MailOpenRead))); err != nil {
			t.Fatalf("indice %d: %v", idx, err)
		}
	}
	ea.nada(t, 150*time.Millisecond)
}

// ---- apagar

func TestApagarCartaVazia(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	caixa, _ := f.Box(context.Background(), 100)
	caixa.Add(mail.Mail{Index: 7, Nick: "Beta"})
	ea.drena(100 * time.Millisecond)

	if err := h.openMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailOpen, 7, MailOpenDelete))); err != nil {
		t.Fatal(err)
	}
	if caixa.Count() != 0 {
		t.Errorf("%d cartas, deveria ter apagado", caixa.Count())
	}
}

// Carta com anexo nao se apaga: o item ficaria perdido.
func TestApagarCartaComAnexoERecusado(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	caixa, _ := f.Box(context.Background(), 100)
	m := mail.Mail{Index: 7, Nick: "Beta"}
	m.Items[0] = model.Item{Index: 100}
	caixa.Add(m)
	ea.drena(100 * time.Millisecond)

	if err := h.openMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailOpen, 7, MailOpenDelete))); err != nil {
		t.Fatal(err)
	}
	if caixa.Count() != 1 {
		t.Error("a carta com anexo deveria ter ficado")
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// ---- retirar anexo

func TestRetirarAnexo(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	caixa, _ := f.Box(context.Background(), 100)
	m := mail.Mail{Index: 7, Nick: "Beta", Gold: 2500, CheckItem: true}
	m.Items[0] = model.Item{Index: 100, Refi: 3}
	m.Items[1] = model.Item{Index: 200}
	caixa.Add(m)
	ouroAntes := a.Avatar.Gold
	ea.drena(100 * time.Millisecond)

	if err := h.withdrawMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailWithdraw, 7, 0))); err != nil {
		t.Fatal(err)
	}
	// Os itens entraram no inventario.
	achados := 0
	for i := 0; i < MovableInventorySlots; i++ {
		switch a.Inventory.Get(i).Index {
		case 100, 200:
			achados++
		}
	}
	if achados != 2 {
		t.Errorf("%d itens no inventario, queria 2", achados)
	}
	// O ouro entrou.
	if a.Avatar.Gold != ouroAntes+2500 {
		t.Errorf("ouro = %d, queria %d", a.Avatar.Gold, ouroAntes+2500)
	}
	// A carta ficou sem anexo.
	guardada := caixa.Get(7)
	if guardada == nil {
		t.Fatal("a carta deveria ter ficado")
	}
	if !guardada.Empty() {
		t.Errorf("a carta deveria estar vazia: %+v", guardada)
	}
}

// Inventario cheio nao retira nada: nem item nem ouro.
func TestRetirarComInventarioCheio(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	for i := 0; i < MovableInventorySlots; i++ {
		a.Inventory.Slots[i] = model.Item{Index: 9000}
	}
	caixa, _ := f.Box(context.Background(), 100)
	m := mail.Mail{Index: 7, Gold: 100, CheckItem: true}
	m.Items[0] = model.Item{Index: 100}
	caixa.Add(m)
	ouroAntes := a.Avatar.Gold
	ea.drena(100 * time.Millisecond)

	if err := h.withdrawMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailWithdraw, 7, 0))); err != nil {
		t.Fatal(err)
	}
	if a.Avatar.Gold != ouroAntes {
		t.Errorf("ouro = %d, nao deveria ter entrado", a.Avatar.Gold)
	}
	if guardada := caixa.Get(7); guardada == nil || !guardada.HasItems() {
		t.Error("o anexo deveria ter ficado na carta")
	}
	if got := ea.espera(t); got != CodeClientMsg {
		t.Errorf("recebeu %#x", got)
	}
}

// Falha na gravacao devolve a carta ao estado anterior e nada entra.
func TestFalhaAoGravarRetiradaNaoEntrega(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	caixa, _ := f.Box(context.Background(), 100)
	m := mail.Mail{Index: 7, Gold: 500, CheckItem: true}
	m.Items[0] = model.Item{Index: 100}
	caixa.Add(m)
	ouroAntes := a.Avatar.Gold
	f.erroSave = errors.New("falha de teste")
	ea.drena(100 * time.Millisecond)

	if err := h.withdrawMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailWithdraw, 7, 0))); err != nil {
		t.Fatal(err)
	}
	if a.Avatar.Gold != ouroAntes {
		t.Errorf("ouro = %d", a.Avatar.Gold)
	}
	guardada := caixa.Get(7)
	if guardada == nil || guardada.Items[0].Index != 100 || guardada.Gold != 500 {
		t.Errorf("a carta deveria ter voltado ao estado anterior: %+v", guardada)
	}
}

// Carta sem anexo nao tem o que retirar.
func TestRetirarDeCartaVazia(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	a.Inventory = sessaoComInventario().Inventory
	caixa, _ := f.Box(context.Background(), 100)
	caixa.Add(mail.Mail{Index: 7})
	ea.drena(100 * time.Millisecond)

	if err := h.withdrawMail(context.Background(), ea.conn, a,
		asPacket(t, openMailPacket(CodeMailWithdraw, 7, 0))); err != nil {
		t.Fatal(err)
	}
	ea.nada(t, 150*time.Millisecond)
}

// ---- lista

func TestListaDeCartas(t *testing.T) {
	h, f := handlerComCorreio(t)
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	caixa, _ := f.Box(context.Background(), 100)
	for i := uint64(1); i <= 40; i++ {
		caixa.Add(mail.Mail{Index: i, Nick: "Beta", Title: "t"})
	}
	ea.drena(100 * time.Millisecond)

	if err := h.sendMailList(context.Background(), ea.conn, a); err != nil {
		t.Fatal(err)
	}
	vis := ea.proximo(t)
	if vis.code != CodeMailList {
		t.Fatalf("codigo %#x", vis.code)
	}
	var pkt packets.PlayerMailsPacket
	if err := netsrv.Unmarshal(asPacote(t, vis.corpo), &pkt); err != nil {
		t.Fatal(err)
	}
	// A origem deixava MailsAmount comentado: o cliente nunca sabia o total.
	if pkt.MailsAmount != 40 {
		t.Errorf("total = %d, queria 40", pkt.MailsAmount)
	}
	// A primeira da lista e a mais nova.
	if pkt.Cartas[0].Index != 40 {
		t.Errorf("primeira = %d, queria 40", pkt.Cartas[0].Index)
	}
	if pkt.Cartas[31].Index != 9 {
		t.Errorf("ultima = %d, queria 9", pkt.Cartas[31].Index)
	}
	// E o $3F1B com as nao lidas vem junto.
	if got := ea.espera(t); got != CodeMailUnread {
		t.Errorf("recebeu %#x, queria %#x", got, CodeMailUnread)
	}
}

// ---- entrada no mundo

// Carta vencida sem anexo e apagada; com anexo fica, para o jogador nao
// perder item por prazo.
func TestCartaVencidaComAnexoFica(t *testing.T) {
	h, f := handlerComCorreio(t)
	agora := time.Now()
	h.Now = func() time.Time { return agora }
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	caixa, _ := f.Box(context.Background(), 100)

	vazia := mail.Mail{Index: 1, ReturnAt: agora.Add(-time.Hour)}
	caixa.Add(vazia)
	comAnexo := mail.Mail{Index: 2, ReturnAt: agora.Add(-time.Hour)}
	comAnexo.Items[0] = model.Item{Index: 100}
	caixa.Add(comAnexo)
	ea.drena(100 * time.Millisecond)

	h.enterMail(context.Background(), ea.conn, a)
	if caixa.Get(1) != nil {
		t.Error("a carta vencida e vazia deveria ter sido apagada")
	}
	if caixa.Get(2) == nil {
		t.Error("a carta vencida com anexo deveria ter ficado")
	}
}

// Sem correio configurado nada quebra.
func TestSemCorreio(t *testing.T) {
	h := handlerComGuildas(t)
	h.MailDB = nil
	a, ea := entraComPersonagem(t, h, "Alfa", 100)
	ctx := context.Background()

	if err := h.sendMail(ctx, ea.conn, a,
		asPacket(t, sendMailPacket("Beta", "x", "y", 0))); err != nil {
		t.Fatal(err)
	}
	if err := h.openMail(ctx, ea.conn, a,
		asPacket(t, openMailPacket(CodeMailOpen, 1, MailOpenRead))); err != nil {
		t.Fatal(err)
	}
	if err := h.withdrawMail(ctx, ea.conn, a,
		asPacket(t, openMailPacket(CodeMailWithdraw, 1, 0))); err != nil {
		t.Fatal(err)
	}
	h.enterMail(ctx, ea.conn, a)
}

// Fora de jogo os handlers recusam.
func TestCorreioExigeEstarEmJogo(t *testing.T) {
	h, _ := handlerComCorreio(t)
	s := sessaoAutenticada()
	c := connDescartavel(t)
	ctx := context.Background()
	casos := map[string]func() error{
		"conferir": func() error { return h.confirmMail(ctx, c, s, asPacket(t, confirmMailPacket("X"))) },
		"enviar":   func() error { return h.sendMail(ctx, c, s, asPacket(t, sendMailPacket("X", "a", "b", 0))) },
		"abrir":    func() error { return h.openMail(ctx, c, s, asPacket(t, openMailPacket(CodeMailOpen, 1, 0))) },
		"retirar":  func() error { return h.withdrawMail(ctx, c, s, asPacket(t, openMailPacket(CodeMailWithdraw, 1, 0))) },
	}
	for nome, f := range casos {
		if err := f(); err != ErrNotPlaying {
			t.Errorf("%s: erro = %v, queria ErrNotPlaying", nome, err)
		}
	}
}
