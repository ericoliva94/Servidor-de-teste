package serverlist

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// embaralha e o inverso de Decode (EncDecSL com Encrypt = True).
func embaralha(b []byte) []byte {
	out := append([]byte(nil), b...)
	for j := range out {
		out[j] += byte(j % 5)
	}
	return out
}

func entrada(ip, nome string, nacao, canal uint32) []byte {
	e := make([]byte, EntryBytes)
	copy(e[0:32], ip)
	copy(e[36:60], nome)
	binary.LittleEndian.PutUint32(e[60:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[64:], nacao)
	binary.LittleEndian.PutUint32(e[68:], canal)
	return e
}

func TestDecodeLeOsCanais(t *testing.T) {
	var raw []byte
	raw = append(raw, entrada("127.0.0.11", "Yggdrasil", 1, 0)...)
	raw = append(raw, entrada("", "vazio", 0, 0)...)
	raw = append(raw, entrada("10.0.0.7", "Mjolnir", 2, 1)...)
	lista, err := Decode(embaralha(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 3 {
		t.Fatalf("entradas = %d, queria 3", len(lista))
	}
	if lista[0] != (Channel{IP: "127.0.0.11", Name: "Yggdrasil", Nation: 1}) {
		t.Errorf("entrada 0 = %+v", lista[0])
	}
	if lista[1].Active() || !lista[2].Active() {
		t.Errorf("entrada sem IP nao e canal: %+v %+v", lista[1], lista[2])
	}
	if lista[2].Addr() != "10.0.0.7:8822" || lista[2].ChannelNation != 1 || lista[2].Nation != 2 {
		t.Errorf("entrada 2 = %+v (%s)", lista[2], lista[2].Addr())
	}
}

func TestDecodeIgnoraSobraEExigeUmaEntrada(t *testing.T) {
	raw := embaralha(append(entrada("1.2.3.4", "A", 1, 0), 0xAA, 0xBB, 0xCC)) // tamanho impar
	lista, err := Decode(raw)
	if err != nil || len(lista) != 1 {
		t.Fatalf("lista=%v err=%v", lista, err)
	}
	if _, err := Decode(make([]byte, EntryBytes-1)); err == nil {
		t.Error("arquivo curto deveria falhar")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "nao-existe.bin")); err == nil {
		t.Error("arquivo ausente deveria falhar")
	}
}

// O SL.bin do repositorio: os dois primeiros canais usam 127.0.0.11 e .12.
func TestSLBinDoRepositorio(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Server", "Bin", FileName)
	if _, err := os.Stat(p); err != nil {
		t.Skip("SL.bin ausente")
	}
	lista, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) < 2 || lista[0].IP != "127.0.0.11" || lista[1].IP != "127.0.0.12" {
		t.Errorf("canais = %+v", lista[:min(2, len(lista))])
	}
}
