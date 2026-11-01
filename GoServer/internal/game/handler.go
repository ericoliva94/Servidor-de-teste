package game

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aika/internal/auth"
	"aika/internal/model"
	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// Codigos de pacote do canal.
const (
	CodeCheckLogin = 0x685 // cliente -> servidor
	CodeCharList   = 0x901 // servidor -> cliente
	CodeClientMsg  = 0x100 // servidor -> cliente (mensagem de texto)
)

// Offsets dos campos do $685 depois de decifrado, como CheckLogin os le a mao.
const (
	offLoginUsername    = 16
	offLoginVersion     = 66
	offLoginTokenLegacy = 72
	offLoginTokenModern = 227
	loginFieldSize      = 32
	minCheckLoginPacket = 104
)

// Characters busca os personagens de uma conta.
type Characters interface {
	ByAccount(ctx context.Context, accountID uint32) ([store.MaxCharacterSlots]store.Character, []string, error)
}

// Handler atende os pacotes do canal.
type Handler struct {
	Channel  uint8
	Accounts auth.Accounts
	Chars    Characters
	Numerics NumericStore
	Now      func() time.Time
	Log      *slog.Logger
	// ServerVersion e o SERVER_VERSION do .ini, so para registro: o original
	// aceita qualquer versao e apenas anota a diferenca.
	ServerVersion uint16
}

// Erros de recusa do canal.
var (
	ErrShortLoginPacket = errors.New("game: pacote de login curto")
	ErrAccountNotFound  = errors.New("game: conta nao encontrada")
	ErrAccountBlocked   = errors.New("game: conta bloqueada")
	ErrInvalidToken     = errors.New("game: token invalido")
)

// HandlePacket despacha pelo codigo do pacote.
func (h *Handler) HandlePacket(ctx context.Context, c *netsrv.Conn, p protocol.Packet) error {
	s, _ := c.Session.(*Session)
	if s == nil {
		s = NewSession(h.Channel)
		c.Session = s
	}

	switch p.Code {
	case CodeCheckLogin:
		return h.checkLogin(ctx, c, s, p)
	case CodeNumericToken:
		return h.numericToken(ctx, c, s, p)
	default:
		// O original caia no fim do case sem ramo e seguia sem responder. Aqui
		// o codigo desconhecido e registrado, o que torna visivel o que falta
		// portar em vez de o cliente travar calado.
		h.log().Debug("codigo de pacote sem handler",
			"codigo", fmt.Sprintf("%#x", p.Code), "ip", c.IP(),
			"tamanho", p.Size, "estado", s.Status.String())
		return nil
	}
}

// checkLogin e o porte de TPacketHandlers.CheckLogin ($685).
func (h *Handler) checkLogin(ctx context.Context, c *netsrv.Conn, s *Session, p protocol.Packet) error {
	if p.Size < minCheckLoginPacket {
		c.Close()
		return ErrShortLoginPacket
	}

	username := strings.TrimSpace(loginField(p, offLoginUsername))
	legacy := strings.TrimSpace(loginField(p, offLoginTokenLegacy))
	modern := strings.TrimSpace(loginField(p, offLoginTokenModern))
	version := uint16(0)
	if offLoginVersion+2 <= len(p.Data) {
		version = binary.LittleEndian.Uint16(p.Data[offLoginVersion:])
	}

	// O original aceita qualquer versao e so anota a diferenca; mantido.
	if version != h.ServerVersion {
		h.log().Info("versao de cliente diferente da configurada",
			"cliente", version, "configurada", h.ServerVersion, "usuario", username)
	}

	acc, found, err := h.Accounts.ByUsername(ctx, strings.ToLower(username))
	if err != nil {
		c.Close()
		return err
	}
	if !found {
		h.message(c, "Conta nao encontrada.")
		c.Close()
		return ErrAccountNotFound
	}
	if acc.Status == auth.StatusBanned {
		h.message(c, "Conta bloqueada. Entre em contato com o suporte.")
		c.Close()
		return ErrAccountBlocked
	}

	// Prefere o token do cliente atual quando ele casa com o guardado; o
	// offset 72 continua valendo para o layout antigo.
	saved := strings.TrimSpace(acc.Token)
	token := legacy
	if modern != "" && strings.EqualFold(modern, saved) {
		token = modern
	}
	if saved == "" || !strings.EqualFold(strings.TrimSpace(token), saved) {
		h.message(c, "Token invalido.")
		c.Close()
		return ErrInvalidToken
	}
	// Mesma validade do login (300 s). Data no futuro tambem nao vale: no
	// original "(Now - CreationTime) >= 300s" era falso para diferenca
	// negativa, entao uma data adiantada no banco valia para sempre.
	age := h.now().Sub(acc.TokenCreated)
	if age < 0 || age >= auth.TokenLifetime {
		h.message(c, "Token invalido.")
		c.Close()
		return ErrInvalidToken
	}

	chars, avisos, err := h.Chars.ByAccount(ctx, acc.ID)
	if err != nil {
		c.Close()
		return err
	}
	for _, a := range avisos {
		h.log().Warn("lista de personagens", "aviso", a)
	}

	s.Authenticated = true
	s.Username = username
	s.Account = acc
	s.Characters = chars

	h.log().Info("usuario conectado",
		"usuario", username, "conta", acc.ID, "canal", h.Channel, "ip", c.IP())

	return h.sendCharList(c, s)
}

// sendCharList monta e envia o $901. Porte de TPlayer.SendCharList.
func (h *Handler) sendCharList(c *netsrv.Conn, s *Session) error {
	var pkt packets.SendToCharListPacket
	pkt.Header.Index = s.ClientID
	pkt.AcountID = s.Account.ID

	for i := range pkt.CharactersData {
		src := s.Characters[i]
		dst := &pkt.CharactersData[i]

		copy(dst.Name[:], src.Name)
		if n := len(src.Name); n < len(dst.Name) {
			dst.Name[n] = 0
		}
		dst.Nation = uint16(s.Account.Nation)
		dst.ClassInfo = src.ClassInfo

		// O original faz "Level - 1" num WORD: com Level 0 (slot vazio) isso
		// estoura para 65535. O cliente mostra o valor como esta, entao slot
		// vazio ia com nivel 65535. Aqui o piso e zero.
		if src.Level > 0 {
			dst.Level = src.Level - 1
		}

		dst.Exp = src.Exp
		dst.Gold = src.Gold
		dst.Attributes = model.Attributes{
			Str:  src.Strength,
			Agi:  src.Agility,
			Int:  src.Intelligence,
			Cons: src.Constitution,
			Luck: src.Luck,
		}
		dst.Size = model.Sizes{
			Altura: src.Altura,
			Tronco: src.Tronco,
			Perna:  src.Perna,
			Corpo:  src.Corpo,
		}
		dst.NumericError = src.NumericErrors
		if src.NumericToken != "" {
			dst.NumRegister = 1
		}
		if src.Deleted && !src.DeleteTime.IsZero() {
			dst.DeleteTime = uint32(src.DeleteTime.Unix())
		}
	}

	if s.Status != CharList {
		s.Status = CharList
		s.SubStatus = Senha2
	}
	return c.SendStruct(CodeCharList, pkt)
}

// message envia uma mensagem de texto ao cliente.
func (h *Handler) message(c *netsrv.Conn, msg string) {
	var pkt packets.ClientMessagePacket
	netsrv.PutCString(pkt.Msg[:], msg)
	if err := c.SendStruct(CodeClientMsg, pkt); err != nil {
		h.log().Debug("falha ao enviar mensagem ao cliente",
			"ip", c.IP(), "erro", err)
	}
}

// loginField le um campo de texto de tamanho fixo do pacote de login.
func loginField(p protocol.Packet, off int) string {
	if off < 0 || off+loginFieldSize > len(p.Data) {
		return ""
	}
	return netsrv.CString(p.Data[off : off+loginFieldSize])
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}
