package game

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	"aika/internal/netsrv"
	"aika/internal/packets"
	"aika/internal/protocol"
	"aika/internal/store"
)

// memNumerics registra as gravacoes de numerica.
type memNumerics struct {
	numeric map[uint32]string
	errs    map[uint32]int
	erroGrv error
}

func novoNumerics() *memNumerics {
	return &memNumerics{numeric: map[uint32]string{}, errs: map[uint32]int{}}
}

func (m *memNumerics) SaveNumeric(_ context.Context, id uint32, n string) error {
	if m.erroGrv != nil {
		return m.erroGrv
	}
	m.numeric[id] = n
	return nil
}

func (m *memNumerics) SaveNumericErrors(_ context.Context, id uint32, n int) error {
	if m.erroGrv != nil {
		return m.erroGrv
	}
	m.errs[id] = n
	return nil
}

// numericPacket monta um $F02.
func numericPacket(slot, mode uint32, n1, n2 string) []byte {
	var p packets.NumericTokenPacket
	p.Slot = slot
	p.RequestChange = mode
	copy(p.Numeric_1[:], n1)
	copy(p.Numeric_2[:], n2)
	buf, err := netsrv.Marshal(CodeNumericToken, p)
	if err != nil {
		panic(err)
	}
	return buf
}

// sessaoAutenticada devolve uma sessao pronta com os personagens de teste.
func sessaoAutenticada() *Session {
	s := NewSession(0)
	s.Authenticated = true
	s.Username = "teste"
	s.Account = contaValida()
	s.Characters = personagens()[7]
	s.Status = CharList
	return s
}

// connDescartavel devolve uma Conn cujo envio e jogado fora.
func connDescartavel(t *testing.T) *netsrv.Conn {
	t.Helper()
	esq, dir := net.Pipe()
	t.Cleanup(func() { esq.Close(); dir.Close() })
	go io.Copy(io.Discard, dir)
	return netsrv.NewConnForTest(esq)
}

func handlerComNumerics(num NumericStore) *Handler {
	h := testHandler()
	h.Numerics = num
	return h
}

// O bug central: Packet.Slot vinha do cliente como DWORD e indexava
// NumError[Slot] e NumericToken[Slot], vetores de 3 posicoes, ANTES de
// qualquer checagem. Slot fora da faixa lia e escrevia fora do vetor.
func TestNumericTokenRejectsSlotOutOfRange(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)

	for _, slot := range []uint32{3, 4, 255, 1 << 16, 0xFFFFFFFF} {
		s := sessaoAutenticada()
		conn := connDescartavel(t)
		err := h.numericToken(context.Background(), conn, s,
			asPacket(t, numericPacket(slot, NumericVerify, "1234", "")))
		if !errors.Is(err, ErrSlotOutOfRange) {
			t.Errorf("slot %d: erro = %v, queria ErrSlotOutOfRange", slot, err)
		}
	}
	if len(num.numeric) != 0 || len(num.errs) != 0 {
		t.Error("slot invalido nao deveria gravar nada")
	}
}

// Slot valido mas vazio tambem nao pode entrar no mundo.
func TestNumericTokenRejectsEmptySlot(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := sessaoAutenticada() // o slot 1 nao tem personagem
	err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(1, NumericVerify, "1234", "")))
	if !errors.Is(err, ErrEmptySlot) {
		t.Errorf("erro = %v, queria ErrEmptySlot", err)
	}
}

func TestNumericTokenRequiresAuthentication(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := NewSession(0) // nao autenticada
	err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericVerify, "1234", "")))
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("erro = %v, queria ErrNotAuthenticated", err)
	}
}

// Numerica correta entra no mundo e zera o contador de erros.
func TestNumericTokenVerifyEntersWorld(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)
	s := sessaoAutenticada()
	s.Characters[0].NumericErrors = 3

	if err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericVerify, "1234", ""))); err != nil {
		t.Fatalf("numerica correta deveria entrar: %v", err)
	}
	if s.Status != Playing {
		t.Errorf("estado = %v, queria Playing", s.Status)
	}
	if s.SelectedSlot != 0 {
		t.Errorf("slot escolhido = %d", s.SelectedSlot)
	}
	if num.errs[100] != 0 {
		t.Errorf("contador de erros = %d, queria 0", num.errs[100])
	}
	if s.Characters[0].NumericErrors != 0 {
		t.Errorf("erros na sessao = %d", s.Characters[0].NumericErrors)
	}
}

// Numerica errada conta o erro, volta para a lista e nao entra.
func TestNumericTokenWrongCountsError(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)
	s := sessaoAutenticada()

	if err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericVerify, "9999", ""))); err != nil {
		t.Fatalf("a recusa nao deveria virar erro de conexao: %v", err)
	}
	if s.Status == Playing {
		t.Error("nao deveria entrar no mundo com numerica errada")
	}
	if s.SubStatus != Waiting {
		t.Errorf("SubStatus = %v, queria Waiting", s.SubStatus)
	}
	if num.errs[100] != 3 {
		t.Errorf("contador gravado = %d, queria 3 (2 anteriores + 1)", num.errs[100])
	}
}

// Cinco erros bloqueiam o slot.
func TestNumericTokenLocksAfterFiveErrors(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := sessaoAutenticada()
	s.Characters[0].NumericErrors = MaxNumericErrors

	err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericVerify, "1234", "")))
	if !errors.Is(err, ErrNumericLocked) {
		t.Errorf("erro = %v, queria ErrNumericLocked", err)
	}
}

// O contador de erros nao pode estourar o byte.
func TestNumericErrorCounterDoesNotOverflow(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := sessaoAutenticada()
	s.Characters[0].NumericErrors = 255
	s.Characters[0].NumericToken = "1234"

	// Com 255 o slot ja esta bloqueado, entao o contador nem e tocado.
	if err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericVerify, "9999", ""))); !errors.Is(err, ErrNumericLocked) {
		t.Fatalf("erro = %v", err)
	}
	if s.Characters[0].NumericErrors != 255 {
		t.Errorf("contador = %d, nao deveria passar de 255", s.Characters[0].NumericErrors)
	}
}

// Cadastro so vale com a numerica vazia; com uma ja cadastrada a origem
// ignorava o pacote em silencio e o cliente ficava esperando.
func TestNumericTokenRegisterOnEmptyOnly(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)

	// Slot 2 nao tem numerica.
	s := sessaoAutenticada()
	if err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(2, NumericRegister, "4321", ""))); err != nil {
		t.Fatalf("cadastro deveria funcionar: %v", err)
	}
	if num.numeric[102] != "4321" {
		t.Errorf("numerica gravada = %q", num.numeric[102])
	}
	if s.Status != Playing {
		t.Errorf("deveria entrar no mundo depois de cadastrar")
	}

	// Slot 0 ja tem numerica: recusa explicita.
	s2 := sessaoAutenticada()
	if err := h.numericToken(context.Background(), connDescartavel(t), s2,
		asPacket(t, numericPacket(0, NumericRegister, "5555", ""))); err != nil {
		t.Fatalf("a recusa nao deveria virar erro de conexao: %v", err)
	}
	if s2.Status == Playing {
		t.Error("nao deveria entrar: a numerica ja existia")
	}
}

// Troca exige a numerica atual em Numeric_2.
func TestNumericTokenChange(t *testing.T) {
	num := novoNumerics()
	h := handlerComNumerics(num)

	s := sessaoAutenticada()
	if err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(0, NumericChange, "7777", "1234"))); err != nil {
		t.Fatalf("troca deveria funcionar: %v", err)
	}
	if num.numeric[100] != "7777" {
		t.Errorf("numerica gravada = %q, queria 7777", num.numeric[100])
	}

	// Com a atual errada, recusa.
	s2 := sessaoAutenticada()
	if err := h.numericToken(context.Background(), connDescartavel(t), s2,
		asPacket(t, numericPacket(0, NumericChange, "7777", "errada"))); err != nil {
		t.Fatalf("%v", err)
	}
	if s2.Status == Playing {
		t.Error("nao deveria entrar com a numerica atual errada")
	}
}

// Modo desconhecido nao entra no mundo nem derruba a conexao.
func TestNumericTokenUnknownMode(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := sessaoAutenticada()
	conn := connDescartavel(t)
	if err := h.numericToken(context.Background(), conn, s,
		asPacket(t, numericPacket(0, 99, "1234", ""))); err != nil {
		t.Errorf("modo desconhecido deveria ser ignorado: %v", err)
	}
	if s.Status == Playing {
		t.Error("modo desconhecido nao deveria entrar no mundo")
	}
	if conn.Closing() {
		t.Error("a conexao nao deveria ser encerrada")
	}
}

// Falha ao gravar nao pode deixar o jogador entrar com estado inconsistente.
func TestNumericTokenSaveFailureStopsEntry(t *testing.T) {
	num := novoNumerics()
	num.erroGrv = errors.New("sem escrita")
	h := handlerComNumerics(num)
	s := sessaoAutenticada()

	err := h.numericToken(context.Background(), connDescartavel(t), s,
		asPacket(t, numericPacket(2, NumericRegister, "4321", "")))
	if err == nil {
		t.Error("falha ao gravar deveria devolver erro")
	}
	if s.Status == Playing {
		t.Error("nao deveria entrar no mundo com a gravacao falhando")
	}
}

// Ponta a ponta: entra no mundo e confere o $925 decifrado.
func TestSendToWorldEndToEnd(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	addr, stop := serve(t, h)
	defer stop()

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Primeiro o login, que deixa a sessao autenticada.
	if _, err := c.Write(loginPacket("teste", "tok123", 305)); err != nil {
		t.Fatal(err)
	}
	if code, _ := readPacket(t, c); code != CodeCharList {
		t.Fatalf("esperava a lista, veio %#x", code)
	}

	// Depois a numerica do slot 0.
	if _, err := c.Write(numericPacket(0, NumericVerify, "1234", "")); err != nil {
		t.Fatal(err)
	}

	// O cliente recebe o $131 e em seguida o $925.
	if code, _ := readPacket(t, c); code != CodeP131 {
		t.Fatalf("esperava o $131, veio %#x", code)
	}
	code, raw := readPacket(t, c)
	if code != CodeSendToWorld {
		t.Fatalf("codigo = %#x, queria %#x", code, CodeSendToWorld)
	}
	if got := binary.LittleEndian.Uint16(raw[2:4]); got != 0 {
		_ = got // o byte 2 e o checksum, nao interessa aqui
	}

	var pkt packets.SendToWorldPacket
	if err := netsrv.Unmarshal(protocol.Packet{
		Data: append(raw, make([]byte, protocol.MaxPacketSize-len(raw))...),
		Size: len(raw)}, &pkt); err != nil {
		t.Fatal(err)
	}
	if pkt.AcountSerial != 7 {
		t.Errorf("AcountSerial = %d, queria 7", pkt.AcountSerial)
	}
	if got := netsrv.CString(pkt.Character.Name[:]); got != "Guerreiro" {
		t.Errorf("nome = %q", got)
	}
	if pkt.Character.Level != 42 {
		t.Errorf("nivel = %d, queria 42", pkt.Character.Level)
	}
	if pkt.Character.CharIndex != 100 {
		t.Errorf("CharIndex = %d", pkt.Character.CharIndex)
	}
	if pkt.Character.CurrentScore.Str != 50 {
		t.Errorf("forca = %d", pkt.Character.CurrentScore.Str)
	}
	if pkt.Character.Gold != 999 {
		t.Errorf("ouro = %d", pkt.Character.Gold)
	}
	if got := netsrv.CString(pkt.Character.Numeric[:]); got != "1234" {
		t.Errorf("numerica no pacote = %q", got)
	}
	if pkt.Header.Index != WorldIndex {
		t.Errorf("Header.Index = %#x, queria %#x", pkt.Header.Index, WorldIndex)
	}
}

// O SendToWorld do pacote $925 tem o tamanho que o cliente espera.
func TestSendToWorldPacketSize(t *testing.T) {
	if got := binary.Size(packets.SendToWorldPacket{}); got != 4232 {
		t.Errorf("SendToWorldPacket = %d bytes, o Delphi tem 4232", got)
	}
}

// Slot fora da faixa em sendToWorld tambem e recusado.
func TestSendToWorldGuardsSlot(t *testing.T) {
	h := handlerComNumerics(novoNumerics())
	s := sessaoAutenticada()
	for _, slot := range []int{-1, store.MaxCharacterSlots, 99} {
		if err := h.sendToWorld(context.Background(), connDescartavel(t), s, slot); !errors.Is(err, ErrSlotOutOfRange) {
			t.Errorf("slot %d: erro = %v", slot, err)
		}
	}
}
