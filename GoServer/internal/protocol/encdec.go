package protocol

import (
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"time"
)

// Layout do cabecalho na rede, conforme EncDec.pas:
//
//	[0..1] tamanho total do pacote (uint16, little endian)
//	[2]    checksum
//	[3]    indice da chave aleatoria
//	[4..]  corpo, cifrado em blocos de 4 bytes
//
// Atencao: TPacketHeader em Packets.pas declara os campos 2 e 3 como
// "Key" e "ChkSum" nessa ordem, ou seja, com os nomes trocados em relacao
// ao que Encrypt/Decrypt realmente gravam. Os nomes estao errados, o
// formato na rede e o descrito acima. Aqui seguimos o formato real.
const (
	offSize     = 0
	offChecksum = 2
	offKeyIndex = 3
	offBody     = 4
)

// HeaderSize e o sizeof(TPacketHeader) do Delphi: 12 bytes.
const HeaderSize = 12

// MaxPacketSize e o MAX_RECV_PACKET_SIZE de PlayerThread.pas.
const MaxPacketSize = 22000

var (
	// ErrShortBuffer indica buf menor que o tamanho declarado.
	ErrShortBuffer = errors.New("protocol: buffer menor que o pacote")
	// ErrPacketTooSmall indica pacote menor que o cabecalho.
	ErrPacketTooSmall = errors.New("protocol: pacote menor que o cabecalho")
	// ErrPacketTooLarge indica pacote acima de MaxPacketSize.
	ErrPacketTooLarge = errors.New("protocol: pacote acima do tamanho maximo")
)

var processStart = time.Now()

// uptimeMillis substitui TimeGetTime (ms desde o boot do Windows). O cliente
// nao interpreta esse campo, so precisa estar presente.
func uptimeMillis() uint32 {
	return uint32(time.Since(processStart).Milliseconds())
}

// validate confere o tamanho antes de qualquer acesso ao buffer. O Delphi
// nao fazia isso: Encrypt gravava o uptime em BufferDest[8] sem checar se o
// pacote tinha 12 bytes, estourando o bloco alocado em pacotes curtos.
func validate(buf []byte, n int) error {
	if n < HeaderSize {
		return ErrPacketTooSmall
	}
	if n > MaxPacketSize {
		return ErrPacketTooLarge
	}
	if len(buf) < n {
		return ErrShortBuffer
	}
	return nil
}

// Encrypt cifra buf[:n] no lugar, gravando tamanho, checksum e chave no
// cabecalho. Porte de TEncDec.Encrypt(src: pointer; Len: Word) de EncDec.pas,
// que e a sobrecarga usada pelo servidor (Player.pas:4601 e
// LoginSocket.pas:265). A outra sobrecarga, que recebe array, inverte os
// bytes 2 e 3 e por isso e incompativel com Decrypt; ela nunca e chamada e
// nao foi portada.
func Encrypt(buf []byte, n int) error {
	return encryptWith(buf, n, byte(rand.IntN(255)), uptimeMillis())
}

// encryptWith e o nucleo deterministico de Encrypt, com a chave aleatoria e o
// uptime injetados para permitir teste.
func encryptWith(buf []byte, n int, randKey byte, uptime uint32) error {
	if err := validate(buf, n); err != nil {
		return err
	}

	binary.LittleEndian.PutUint16(buf[offSize:], uint16(n))
	buf[offKeyIndex] = randKey
	binary.LittleEndian.PutUint32(buf[8:], uptime)

	var sum1, sum2 int32
	pos := int(encDecKeys[int(randKey)*2])

	for i, p := 1, offBody; i < n>>2; i, p = i+1, p+4 {
		key := int32(encDecKeys[((pos&0xFF)*2)+1])
		v := int32(binary.LittleEndian.Uint32(buf[p:]))
		sum1 += v

		switch i & 3 {
		case 0:
			key *= 4
			v += key
		case 1:
			key >>= 1
			v -= key
		case 2:
			key += key
			v += key
		case 3:
			key >>= 2
			v -= key
		}

		binary.LittleEndian.PutUint32(buf[p:], uint32(v))
		sum2 += v
		pos++
	}

	sum1 &= 0xFF
	sum2 &= 0xFF
	buf[offChecksum] = byte((sum2 - sum1) & 0xFF)
	return nil
}

// Decrypt decifra buf[:n] no lugar e informa se o checksum fecha. Porte de
// TEncDec.Decrypt de EncDec.pas.
//
// O Delphi retornava o valor invertido (False quando o pacote era valido) e
// nenhum chamador olhava o resultado. Aqui o retorno tem o sentido obvio:
// true quando o checksum confere. O conteudo decifrado e identico.
func Decrypt(buf []byte, n int) (checksumOK bool, err error) {
	if err := validate(buf, n); err != nil {
		return false, err
	}

	var sum1, sum2 int32
	pos := int(encDecKeys[int(buf[offKeyIndex])*2])

	for i, p := 1, offBody; i < n>>2; i, p = i+1, p+4 {
		key := int32(encDecKeys[((pos&0xFF)*2)+1])
		v := int32(binary.LittleEndian.Uint32(buf[p:]))
		sum1 += v

		switch i & 3 {
		case 0:
			key *= 4
			v -= key
		case 1:
			key >>= 1
			v += key
		case 2:
			key += key
			v -= key
		case 3:
			key >>= 2
			v += key
		}

		binary.LittleEndian.PutUint32(buf[p:], uint32(v))
		sum2 += v
		pos++
	}

	sum1 &= 0xFF
	sum2 &= 0xFF
	return buf[offChecksum] == byte((sum1-sum2)&0xFF), nil
}
