// Package serverlist le o SL.bin: a lista de canais (IP, nome e nacao) que a
// origem carregava em TFunctions.LoadSL e que o cliente tambem usa para achar
// cada canal.
package serverlist

import (
	"fmt"
	"os"

	"aika/internal/netsrv"
)

// Tamanho de uma entrada (TChannelFromList): IP[32], Unk_0, Name[24], Check,
// NationIndex e ChannelNationIndex.
const (
	EntryBytes = 72
	// ChannelPort e a porta de todos os canais; o que muda de um para outro e o
	// IP (Servers[I].StartServer fixava htons(8822)).
	ChannelPort = 8822
	// FileName e o nome do arquivo ao lado do executavel.
	FileName = "SL.bin"
)

// Channel e uma entrada da lista.
type Channel struct {
	IP            string
	Name          string
	Nation        uint32
	ChannelNation uint32
}

// Active diz se a entrada tem IP, a condicao com que a origem subia um canal
// ("if ServerList[I].IP <> ”").
func (c Channel) Active() bool { return c.IP != "" }

// Addr e o endereco em que o canal escuta.
func (c Channel) Addr() string { return fmt.Sprintf("%s:%d", c.IP, ChannelPort) }

// Decode desfaz o embaralhamento do arquivo (TFunctions.EncDecSL) e le as
// entradas. Cada byte perde (posicao mod 5). A origem percorria o buffer de 2
// em 2 (de 3 em 3 se o tamanho era impar) e, nesse caso, escrevia um byte alem
// do fim; aqui todo byte e tratado uma vez. Bytes que sobram depois da ultima
// entrada completa sao ignorados, como em "Trunc(FSize / 72)".
func Decode(raw []byte) ([]Channel, error) {
	if len(raw) < EntryBytes {
		return nil, fmt.Errorf("serverlist: arquivo com %d bytes, queria pelo menos %d", len(raw), EntryBytes)
	}
	b := append([]byte(nil), raw...)
	for j := range b {
		b[j] -= byte(j % 5)
	}
	n := len(b) / EntryBytes
	out := make([]Channel, n)
	for i := range out {
		e := b[i*EntryBytes : (i+1)*EntryBytes]
		out[i] = Channel{
			IP:            netsrv.CString(e[0:32]),
			Name:          netsrv.CString(e[36:60]),
			Nation:        leU32(e[64:68]),
			ChannelNation: leU32(e[68:72]),
		}
	}
	return out, nil
}

// Load le o SL.bin.
func Load(path string) ([]Channel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("serverlist: %w", err)
	}
	return Decode(raw)
}

func leU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
