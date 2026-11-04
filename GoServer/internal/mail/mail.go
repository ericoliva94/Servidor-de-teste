// Package mail tem o correio entre personagens.
//
// Porte de Server/Src/Mail/MailFunctions.pas e CharacterMail.pas.
//
// A camada de armazenamento da origem era arquivo por personagem em
// Data/Mails, e estava NAO FUNCIONAL: TMailFunctions.GetMails tem o corpo
// esvaziado — e so "Result := True" com a leitura comentada — entao
//
//   - toda carta recebida APAGAVA a caixa inteira do destinatario:
//     AddMail chama GetMails (que nao le nada e devolve True), deixa
//     MailCount em zero, faz SetLength(Mails, 1) e grava o arquivo com UMA
//     carta so. A carta anterior, com os itens anexados, sumia do disco;
//   - nenhuma carta podia ser lida de volta: GetMail e GetMailSlot varrem o
//     vetor que GetMails nao preencheu;
//   - os lacos "for i := Length(Mails) - 1 downto 0" com i declarado BYTE ou
//     DWORD partem de -1 num vetor vazio, o que como inteiro sem sinal vira
//     255 ou 4294967295 e indexa um vetor nulo.
//
// Portar isso ao pe da letra seria portar perda de dado. Aqui a caixa fica no
// MySQL, como o resto do estado de jogo, e os pacotes seguem os mesmos.
package mail

import (
	"errors"
	"strings"
	"time"

	"aika/internal/model"
)

// Limites dos campos de TMail.
const (
	// NickLen e "Nick: ARRAY [0..15]".
	NickLen = 16
	// TitleLen e "Titulo: ARRAY [0..31]".
	TitleLen = 32
	// TextLen e "Texto: ARRAY [0..511]".
	TextLen = 512
	// MaxItems e "Item: ARRAY [0..6]", mas o pacote de leitura leva 5 e o
	// envio percorre 0..4: cinco e o numero util.
	MaxItems = 5
	// MaxItemsRecord e o tamanho do vetor no registro da origem.
	MaxItemsRecord = 7
	// MailsPerPacket e "Cartas: ARRAY [0..31]" do $3F17.
	MailsPerPacket = 32
	// MaxMails e o teto que AddMail testava.
	MaxMails = 32000
)

// Custos e prazos, da origem.
const (
	// SendCost e o "if not(Gold >= 500)" de SendCharacterMail.
	SendCost = 500
	// ReturnDays e o "IncDay(Now, 15)" de ParseMail.
	ReturnDays = 15
)

// Erros do correio.
var (
	ErrNotEnoughGold = errors.New("mail: ouro insuficiente")
	ErrMailboxFull   = errors.New("mail: caixa cheia")
	ErrMailNotFound  = errors.New("mail: carta nao encontrada")
	ErrNoAttachment  = errors.New("mail: carta sem anexo")
	ErrInventoryFull = errors.New("mail: inventario cheio")
	ErrInvalidTarget = errors.New("mail: destinatario invalido")
)

// Mail e uma carta. Porte de TMail.
type Mail struct {
	Index     uint64
	CharIndex uint32
	// Nick e quem enviou.
	Nick  string
	Title string
	Text  string
	Gold  uint32
	Items [MaxItems]model.Item

	SentAt   time.Time
	ReturnAt time.Time

	Checked   bool
	Return    bool
	CheckItem bool
	Auction   bool
}

// HasItems diz se a carta tem anexo.
func (m *Mail) HasItems() bool {
	if m == nil {
		return false
	}
	for i := range m.Items {
		if !m.Items[i].IsEmpty() {
			return true
		}
	}
	return false
}

// ItemCount conta os anexos. Porte de TMailFunctions.MailItemsCount.
func (m *Mail) ItemCount() int {
	if m == nil {
		return 0
	}
	n := 0
	for i := range m.Items {
		if !m.Items[i].IsEmpty() {
			n++
		}
	}
	return n
}

// Empty diz se a carta nao tem ouro nem item: nada a retirar.
func (m *Mail) Empty() bool {
	return m == nil || (m.Gold == 0 && !m.HasItems())
}

// New monta uma carta nova a partir do conteudo que o cliente mandou.
//
// Porte de ParseMail(MailContent): titulo e texto cortados no tamanho do
// campo, data de envio agora e prazo de retorno em 15 dias.
func New(index uint64, de string, conteudo model.MailContent, agora time.Time) Mail {
	return Mail{
		Index:    index,
		Nick:     truncate(de, NickLen-1),
		Title:    truncate(cstring(conteudo.Titulo[:]), TitleLen-1),
		Text:     truncate(cstring(conteudo.Texto[:]), TextLen-1),
		Gold:     conteudo.Gold,
		SentAt:   agora,
		ReturnAt: agora.AddDate(0, 0, ReturnDays),
	}
}

// Box e a caixa de correio de um personagem.
//
// A origem trabalhava com um vetor dinamico por personagem, lido e gravado
// inteiro a cada operacao; aqui a caixa e uma lista ordenada e cada operacao
// diz o que mudou, para a gravacao ser por carta.
type Box struct {
	// Mails esta da mais antiga para a mais nova, como o arquivo da origem.
	Mails []Mail
}

// Count conta as cartas.
func (b *Box) Count() int {
	if b == nil {
		return 0
	}
	return len(b.Mails)
}

// Unread conta as cartas nao lidas. Porte de GetUnreadMails.
func (b *Box) Unread() int {
	if b == nil {
		return 0
	}
	n := 0
	for i := range b.Mails {
		if !b.Mails[i].Checked {
			n++
		}
	}
	return n
}

// Get devolve a carta de um indice, ou nil.
//
// A origem fazia "for i := Length(Mails) - 1 downto 0" com i BYTE: num vetor
// vazio o inicio e -1, que num BYTE e 255, e o laco indexava um vetor nulo.
func (b *Box) Get(index uint64) *Mail {
	if b == nil || index == 0 {
		return nil
	}
	for i := range b.Mails {
		if b.Mails[i].Index == index {
			return &b.Mails[i]
		}
	}
	return nil
}

// Add poe uma carta na caixa.
//
// Esta e a operacao que a origem destruia: ali ela gravava o arquivo com uma
// carta so, apagando as anteriores.
func (b *Box) Add(m Mail) error {
	if b == nil {
		return ErrMailNotFound
	}
	if len(b.Mails) >= MaxMails {
		return ErrMailboxFull
	}
	b.Mails = append(b.Mails, m)
	return nil
}

// Remove tira a carta da caixa e diz se ela estava la.
func (b *Box) Remove(index uint64) bool {
	if b == nil {
		return false
	}
	for i := range b.Mails {
		if b.Mails[i].Index == index {
			b.Mails = append(b.Mails[:i], b.Mails[i+1:]...)
			return true
		}
	}
	return false
}

// Expired lista as cartas cujo prazo de retorno passou. Porte de
// RemoveExpiredMails.
func (b *Box) Expired(agora time.Time) []uint64 {
	if b == nil {
		return nil
	}
	var out []uint64
	for i := range b.Mails {
		m := &b.Mails[i]
		if m.ReturnAt.IsZero() {
			continue
		}
		if agora.After(m.ReturnAt) {
			out = append(out, m.Index)
		}
	}
	return out
}

// Latest devolve as ultimas n cartas, da mais nova para a mais antiga.
//
// Porte da janela de SendRefreshMails, que pegava as 32 ultimas e as
// percorria de tras para frente.
func (b *Box) Latest(n int) []*Mail {
	if b == nil || n <= 0 {
		return nil
	}
	inicio := len(b.Mails) - n
	if inicio < 0 {
		inicio = 0
	}
	out := make([]*Mail, 0, len(b.Mails)-inicio)
	for i := len(b.Mails) - 1; i >= inicio; i-- {
		out = append(out, &b.Mails[i])
	}
	return out
}

// truncate corta o texto no numero de runas.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// cstring le uma cadeia terminada em zero de um campo de tamanho fixo.
func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
