package protocol

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"testing"
)

func TestKeyTableIsIntact(t *testing.T) {
	// Primeiros e ultimos bytes conferidos contra EncDec.pas.
	want := []byte{0x52, 0xB3, 0xF7, 0x20, 0x34, 0xFB, 0x8E, 0x4C}
	if !bytes.Equal(encDecKeys[:8], want) {
		t.Fatalf("inicio da tabela divergiu: %x", encDecKeys[:8])
	}
	wantTail := []byte{0x03, 0x6B, 0x8B, 0xD4, 0xEC, 0xF6, 0x57, 0x6C}
	if !bytes.Equal(encDecKeys[504:], wantTail) {
		t.Fatalf("fim da tabela divergiu: %x", encDecKeys[504:])
	}
}

// O caminho que importa: o que Encrypt grava, Decrypt tem que devolver igual,
// com o checksum fechando. Era aqui que as duas sobrecargas do Delphi
// divergiam (uma trocava os bytes 2 e 3).
func TestEncryptDecryptRoundTrip(t *testing.T) {
	for _, n := range []int{12, 16, 20, 24, 60, 100, 1024, 22000} {
		for _, key := range []byte{0, 1, 7, 128, 254} {
			plain := make([]byte, n)
			for i := range plain {
				plain[i] = byte(i * 7 % 251)
			}
			buf := append([]byte(nil), plain...)

			if err := encryptWith(buf, n, key, 0x12345678); err != nil {
				t.Fatalf("n=%d key=%d Encrypt: %v", n, key, err)
			}
			if n > HeaderSize+4 && bytes.Equal(buf[offBody:], plain[offBody:]) {
				t.Fatalf("n=%d key=%d: corpo nao foi cifrado", n, key)
			}

			ok, err := Decrypt(buf, n)
			if err != nil {
				t.Fatalf("n=%d key=%d Decrypt: %v", n, key, err)
			}
			if !ok {
				t.Errorf("n=%d key=%d: checksum nao fechou", n, key)
			}
			// Encrypt sobrescreve [8..11] com o uptime antes de cifrar, entao
			// o round-trip devolve o uptime ali, nao o conteudo original.
			if !bytes.Equal(buf[offBody:8], plain[offBody:8]) {
				t.Errorf("n=%d key=%d: bytes 4..7 nao voltaram", n, key)
			}
			if got := binary.LittleEndian.Uint32(buf[8:]); got != 0x12345678 {
				t.Errorf("n=%d key=%d: uptime voltou %#x", n, key, got)
			}
			if n > 12 && !bytes.Equal(buf[12:n], plain[12:n]) {
				t.Errorf("n=%d key=%d: corpo apos o cabecalho nao voltou", n, key)
			}
			if got := binary.LittleEndian.Uint16(buf[offSize:]); int(got) != n {
				t.Errorf("n=%d: tamanho gravado %d", n, got)
			}
			if buf[offKeyIndex] != key {
				t.Errorf("n=%d: chave gravada %d, queria %d", n, buf[offKeyIndex], key)
			}
		}
	}
}

// O checksum deste protocolo detecta troca do indice de chave.
func TestDecryptDetectsWrongKeyIndex(t *testing.T) {
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = byte(i)
	}
	if err := encryptWith(buf, len(buf), 42, 0); err != nil {
		t.Fatal(err)
	}
	buf[offKeyIndex] = 43

	ok, err := Decrypt(buf, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("checksum fechou com o indice de chave trocado")
	}
}

// ...e NAO detecta corrupcao do corpo: um byte alterado desloca enc_v e dec_v
// na mesma medida, e o checksum e a diferenca entre as duas somas. Limitacao
// do algoritmo original, fixada aqui para ninguem "corrigir" o porte achando
// que o checksum vale como integridade. Quem precisa validar conteudo tem que
// validar o pacote no handler.
func TestChecksumIsBlindToBodyCorruption(t *testing.T) {
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = byte(i)
	}
	if err := encryptWith(buf, len(buf), 42, 0); err != nil {
		t.Fatal(err)
	}
	buf[20] ^= 0xFF

	ok, err := Decrypt(buf, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("comportamento mudou: o checksum passou a detectar corpo corrompido")
	}
}

// Encrypt no Delphi gravava o uptime em BufferDest[8] sem checar o tamanho:
// um pacote de 8 bytes estourava o buffer. Aqui tem que virar erro.
func TestRejectsInvalidSizes(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		n    int
		want error
	}{
		{"menor que o cabecalho", make([]byte, 64), 8, ErrPacketTooSmall},
		{"tamanho zero", make([]byte, 64), 0, ErrPacketTooSmall},
		{"acima do maximo", make([]byte, 64), MaxPacketSize + 1, ErrPacketTooLarge},
		{"buffer curto", make([]byte, 16), 32, ErrShortBuffer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Encrypt(c.buf, c.n); err != c.want {
				t.Errorf("Encrypt: %v, queria %v", err, c.want)
			}
			if _, err := Decrypt(c.buf, c.n); err != c.want {
				t.Errorf("Decrypt: %v, queria %v", err, c.want)
			}
		})
	}
}

// Nenhuma entrada, por mais torta que seja, pode derrubar o servidor.
func TestNeverPanics(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 20000; i++ {
		n := r.IntN(MaxPacketSize + 64)
		bufLen := r.IntN(MaxPacketSize + 64)
		buf := make([]byte, bufLen)
		for j := range buf {
			buf[j] = byte(r.UintN(256))
		}
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("Encrypt entrou em panico (n=%d len=%d): %v", n, bufLen, p)
				}
			}()
			_ = Encrypt(buf, n)
		}()
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("Decrypt entrou em panico (n=%d len=%d): %v", n, bufLen, p)
				}
			}()
			_, _ = Decrypt(buf, n)
		}()
	}
}

// Tamanho nao multiplo de 4: o laco usa n>>2, entao os bytes sobrando ficam em
// claro. Comportamento do Delphi, preservado de proposito.
func TestNonMultipleOfFourLeavesTailPlain(t *testing.T) {
	const n = 23
	plain := make([]byte, n)
	for i := range plain {
		plain[i] = byte(0xA0 + i)
	}
	buf := append([]byte(nil), plain...)
	if err := encryptWith(buf, n, 9, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[20:23], plain[20:23]) {
		t.Errorf("cauda deveria ficar em claro: %x vs %x", buf[20:23], plain[20:23])
	}
	ok, err := Decrypt(buf, n)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("checksum nao fechou")
	}
	if !bytes.Equal(buf[offBody:8], plain[offBody:8]) {
		t.Error("bytes 4..7 nao voltaram")
	}
	if !bytes.Equal(buf[12:n], plain[12:n]) {
		t.Error("corpo apos o cabecalho nao voltou")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	buf := make([]byte, 1024)
	for b.Loop() {
		_ = Encrypt(buf, len(buf))
	}
}
