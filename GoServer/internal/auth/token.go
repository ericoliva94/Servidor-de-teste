// Package auth trata o login por token no servidor de login (porta 8831).
//
// Porte de TAuthHandlers.CheckToken (AuthHandlers.pas) e do despacho de
// LoginSocket.pas, que so trata o codigo $81.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
)

// Codigos de pacote do servidor de login.
const (
	CodeCheckToken    = 0x81 // cliente -> servidor
	CodeResponseLogin = 0x82 // servidor -> cliente
)

// TokenLifetime e a validade do token. No original era
// "(Now - CreationTime) < (300.0 / 86400.0)", ou seja 300 segundos.
const TokenLifetime = 300 * time.Second

// StatusBanned e o AccountStatus que barra o login.
const StatusBanned = 8

// Account e o que o login precisa saber da conta.
type Account struct {
	ID           uint32
	Username     string
	Token        string
	TokenCreated time.Time
	Status       uint8
	Nation       uint8
}

// Accounts busca contas. Implementado por internal/store contra o MySQL e
// por um dublê nos testes.
type Accounts interface {
	// ByUsername devolve a conta. found=false quando nao existe.
	ByUsername(ctx context.Context, username string) (acc Account, found bool, err error)
}

// Erros de recusa. Todos levam ao mesmo resultado para o cliente (conexao
// encerrada), mas distinguem a causa no log.
var (
	ErrPacketTooShort = errors.New("auth: pacote de login curto")
	ErrNoSuchAccount  = errors.New("auth: conta inexistente")
	ErrBadToken       = errors.New("auth: token incorreto")
	ErrTokenExpired   = errors.New("auth: token expirado")
	ErrBanned         = errors.New("auth: conta suspensa")
)

// Uptime devolve o valor do campo Time da resposta. No original era
// TimeGetTime (ms desde o boot do Windows); o cliente nao interpreta.
type Uptime func() uint32

// Handler trata o pacote $81.
type Handler struct {
	Accounts Accounts
	Now      func() time.Time
	Uptime   Uptime
}

// minLoginPacket e a checagem de tamanho do original ("if Length(Buffer) < 76").
const minLoginPacket = 76

// Offsets dos campos dentro do pacote, como o original os lia a mao.
// Coincidem com a struct CheckTokenPacket: Username em 12, Token em 44.
const (
	offUsername = 12
	offToken    = 44
	// O original tentava um segundo offset quando o token vinha vazio, para
	// dar conta de uma variante do pacote do cliente.
	offTokenAlt    = 76
	fieldSize      = 32
	altMinimumSize = 108
)

// HandlePacket atende o $81 e responde o $82.
//
// Ao contrario do original, que chamava Connection.Destroy dentro de cada
// ramo de recusa (zerando o registro da conexao com ZeroMemory enquanto outro
// trecho ainda podia estar lendo dele), aqui a conexao e apenas marcada para
// encerrar e o servidor fecha depois de enviar o que falta.
func (h *Handler) HandlePacket(ctx context.Context, c *netsrv.Conn, p protocol.Packet) error {
	if p.Code != CodeCheckToken {
		// O login so conhece o $81. Pacote desconhecido fecha a conexao, como
		// no original, onde o case sem ramo caia no fim do PacketControl e a
		// conexao seguia sem resposta.
		c.Close()
		return nil
	}

	acc, err := h.check(ctx, p)
	c.Close()
	if err != nil {
		return err
	}

	reply := packets.ResponseLoginPacket{
		Index:  acc.ID,
		Time:   h.uptime(),
		Nation: acc.Nation,
	}
	return c.SendStruct(CodeResponseLogin, reply)
}

// check valida o pacote e devolve a conta autenticada.
func (h *Handler) check(ctx context.Context, p protocol.Packet) (Account, error) {
	if p.Size < minLoginPacket {
		return Account{}, ErrPacketTooShort
	}

	username := normalize(field(p, offUsername))
	token := normalize(field(p, offToken))
	if token == "" && p.Size >= altMinimumSize {
		token = normalize(field(p, offTokenAlt))
	}
	if username == "" {
		return Account{}, ErrNoSuchAccount
	}

	acc, found, err := h.Accounts.ByUsername(ctx, username)
	if err != nil {
		return Account{}, err
	}
	if !found {
		return Account{}, ErrNoSuchAccount
	}

	// Comparacao de token em tempo constante nao e necessaria aqui: o token e
	// de uso unico e expira em 300 s, e o original comparava texto direto.
	if token == "" || normalize(acc.Token) != token {
		return Account{}, ErrBadToken
	}

	age := h.now().Sub(acc.TokenCreated)
	// Token com data no futuro tambem nao vale: no original a conta
	// "(Now - CreationTime) < 300s" aceitava qualquer valor negativo, entao um
	// CreationTime adiantado no banco deixava o token valido para sempre.
	if age < 0 || age >= TokenLifetime {
		return Account{}, ErrTokenExpired
	}
	if acc.Status == StatusBanned {
		return Account{}, ErrBanned
	}
	return acc, nil
}

// field le um campo de texto de tamanho fixo do pacote.
func field(p protocol.Packet, off int) string {
	if off < 0 || off+fieldSize > len(p.Data) {
		return ""
	}
	return netsrv.CString(p.Data[off : off+fieldSize])
}

// normalize aplica o mesmo tratamento do original: minusculas e sem espacos
// nas pontas.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) uptime() uint32 {
	if h.Uptime != nil {
		return h.Uptime()
	}
	return uint32(time.Since(processStart).Milliseconds())
}

var processStart = time.Now()
