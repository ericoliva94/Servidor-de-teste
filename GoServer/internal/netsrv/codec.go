package netsrv

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"aika/internal/protocol"
)

// encodeInto serializa v no inicio de buf, em little endian empacotado.
func encodeInto(buf []byte, v any) error {
	var b bytes.Buffer
	b.Grow(len(buf))
	if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
		return fmt.Errorf("netsrv: serializar %T: %w", v, err)
	}
	if b.Len() > len(buf) {
		return fmt.Errorf("netsrv: %T ocupa %d bytes, o buffer tem %d",
			v, b.Len(), len(buf))
	}
	copy(buf, b.Bytes())
	return nil
}

// Unmarshal le o pacote para dentro de dst.
//
// dst tem de ser ponteiro para uma struct de internal/packets. O pacote que o
// Framer entrega tem sempre MaxPacketSize bytes com a cauda zerada, entao uma
// struct maior que o que o cliente mandou le zeros no resto em vez de estourar
// o buffer — era o que o ZeroMemory do original garantia.
func Unmarshal(p protocol.Packet, dst any) error {
	size := binary.Size(dst)
	if size <= 0 {
		return fmt.Errorf("netsrv: %T nao tem tamanho binario fixo", dst)
	}
	if size > len(p.Data) {
		return fmt.Errorf("netsrv: %T precisa de %d bytes, o pacote tem espaco para %d",
			dst, size, len(p.Data))
	}
	if err := binary.Read(bytes.NewReader(p.Data[:size]), binary.LittleEndian, dst); err != nil {
		return fmt.Errorf("netsrv: ler %T: %w", dst, err)
	}
	return nil
}

// CString le um campo de texto de tamanho fixo ate o primeiro zero.
//
// Os campos de nome e token do protocolo sao ARRAY [0..N] OF AnsiChar com
// terminador zero quando sobra espaco; quando o campo esta cheio nao ha
// terminador, e por isso o limite e o tamanho do campo.
func CString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// PutCString grava s num campo de tamanho fixo, truncando se nao couber e
// zerando o resto.
func PutCString(dst []byte, s string) {
	clear(dst)
	n := copy(dst, s)
	if n < len(dst) {
		dst[n] = 0
	}
}
