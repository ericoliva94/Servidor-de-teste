package netsrv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"aika/internal/protocol"
)

// Conn e uma conexao de cliente.
//
// O envio e serializado por um mutex: no original varias threads escreviam no
// mesmo socket ao mesmo tempo e os bytes de pacotes diferentes se misturavam
// no fluxo, o que dessincronizava o cliente.
type Conn struct {
	nc     net.Conn
	framer *protocol.Framer
	ip     string

	writeMu      sync.Mutex
	writeTimeout time.Duration
	log          *slog.Logger

	closing atomic.Bool

	// Session guarda o estado que o handler quiser pendurar na conexao.
	Session any
}

// IP devolve o endereco do cliente.
func (c *Conn) IP() string { return c.ip }

// RemoteAddr devolve o endereco completo.
func (c *Conn) RemoteAddr() net.Addr {
	if c.nc == nil {
		return nil
	}
	return c.nc.RemoteAddr()
}

// Close marca a conexao para encerrar depois de enviar o que falta.
//
// Equivale ao Connection.Destroy do original, mas sem fechar o socket no meio
// do tratamento: ali o Destroy zerava o registro inteiro com ZeroMemory
// enquanto outro trecho ainda podia estar lendo dele.
func (c *Conn) Close() { c.closing.Store(true) }

// Closing diz se a conexao esta marcada para encerrar.
func (c *Conn) Closing() bool { return c.closing.Load() }

// Send cifra e envia um pacote.
//
// O tamanho e gravado no cabecalho pelo Encrypt, entao basta passar o buffer
// com o pacote montado. Envio parcial e tratado: no original o retorno do
// send era ignorado e um envio parcial perdia o resto do pacote.
func (c *Conn) Send(packet []byte) error {
	if len(packet) < protocol.HeaderSize {
		return fmt.Errorf("netsrv: pacote de %d bytes, o cabecalho tem %d",
			len(packet), protocol.HeaderSize)
	}
	buf := make([]byte, len(packet))
	copy(buf, packet)
	if err := protocol.Encrypt(buf, len(buf)); err != nil {
		return fmt.Errorf("netsrv: cifrar: %w", err)
	}
	return c.sendRaw(buf)
}

// SendStruct monta, cifra e envia um pacote a partir de uma struct de
// internal/packets. O tamanho e o do proprio tipo.
func (c *Conn) SendStruct(code uint16, v any) error {
	buf, err := Marshal(code, v)
	if err != nil {
		return err
	}
	return c.sendRaw(buf)
}

// Marshal serializa a struct, grava o codigo e cifra.
func Marshal(code uint16, v any) ([]byte, error) {
	size := binary.Size(v)
	if size < protocol.HeaderSize {
		return nil, fmt.Errorf("netsrv: %T tem %d bytes, o cabecalho tem %d",
			v, size, protocol.HeaderSize)
	}
	buf := make([]byte, size)
	if err := encodeInto(buf, v); err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint16(buf[6:], code)
	if err := protocol.Encrypt(buf, size); err != nil {
		return nil, fmt.Errorf("netsrv: cifrar: %w", err)
	}
	return buf, nil
}

// ErrNoSocket indica Conn sem socket associado.
var ErrNoSocket = errors.New("netsrv: conexao sem socket")

// SendRaw escreve bytes JA CIFRADOS. Serve para reenviar a varios
// destinatarios o mesmo pacote, cifrando uma vez so — e o que o
// SendToVisible do original fazia passando o mesmo buffer adiante.
func (c *Conn) SendRaw(packet []byte) error { return c.sendRaw(packet) }

// sendRaw escreve bytes ja cifrados, com trava por conexao.
func (c *Conn) sendRaw(buf []byte) error {
	if c == nil || c.nc == nil {
		// Conn de valor zero (em teste, ou depois de a conexao ser recolhida)
		// nao pode entrar em panico no envio.
		return ErrNoSocket
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	traceTx(c.log, c.ip, buf)

	if c.writeTimeout > 0 {
		if err := c.nc.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return err
		}
	}
	// net.Conn.Write ja escreve tudo ou devolve erro, ao contrario do send()
	// cru, cujo retorno parcial o original ignorava.
	if _, err := c.nc.Write(buf); err != nil {
		return fmt.Errorf("netsrv: enviar para %s: %w", c.ip, err)
	}
	return nil
}
