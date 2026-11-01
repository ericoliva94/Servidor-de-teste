package protocol

import (
	"encoding/binary"
	"errors"
)

// Framer separa o fluxo TCP em pacotes inteiros.
//
// Porte de TPlayerThread.DispatchPackets / IsHello (PlayerThread.pas). O
// binario original tratava apenas o primeiro pacote de cada recv() e
// descartava o resto, alem de processar pela metade os pacotes partidos entre
// dois segmentos TCP; era isso que dessincronizava o cliente e derrubava a
// conexao, e o motivo de existir o AikaProxy em .NET. Aqui o enquadramento e
// feito pelo tamanho do cabecalho, como o protocolo manda.
type Framer struct {
	pending      []byte
	scratch      []byte
	helloChecked bool
}

var (
	// ErrInvalidPacketSize indica tamanho fora de [HeaderSize, MaxPacketSize].
	// O buffer pendente e descartado, como no Delphi.
	ErrInvalidPacketSize = errors.New("protocol: tamanho de pacote invalido")
	// ErrPendingOverflow indica que o cliente acumulou mais bytes do que um
	// pacote maximo poderia justificar.
	ErrPendingOverflow = errors.New("protocol: buffer pendente estourou")
)

// maxPending da folga para um pacote maximo partido entre dois segmentos.
const maxPending = 2 * MaxPacketSize

// NewFramer devolve um Framer pronto para uma conexao.
func NewFramer() *Framer {
	return &Framer{
		pending: make([]byte, 0, 4096),
		scratch: make([]byte, MaxPacketSize),
	}
}

// Feed acumula bytes recem-lidos do socket.
func (f *Framer) Feed(p []byte) error {
	if len(f.pending)+len(p) > maxPending {
		f.pending = f.pending[:0]
		return ErrPendingOverflow
	}
	f.pending = append(f.pending, p...)
	return nil
}

// Packet e um pacote inteiro, ja decifrado.
//
// Data tem sempre MaxPacketSize bytes com a cauda zerada, para que um handler
// possa ler uma struct de tamanho fixo sem estourar o buffer quando o cliente
// manda um pacote mais curto do que a struct — era o que o Delphi fazia com
// ZeroMemory e e o que evita indice fora de faixa aqui.
//
// Data aponta para um buffer reaproveitado pelo Framer: vale ate a proxima
// chamada de Next. Quem precisa guardar o conteudo tem que copiar.
type Packet struct {
	Size       int
	Code       uint16
	Data       []byte
	ChecksumOK bool
}

// Next devolve o proximo pacote completo. Quando faltam bytes, devolve
// ok=false sem erro: e o caso normal de pacote partido, basta ler mais do
// socket e chamar de novo.
func (f *Framer) Next() (pkt Packet, ok bool, err error) {
	if !f.helloChecked {
		if len(f.pending) < 4 {
			return Packet{}, false, nil
		}
		f.helloChecked = true
		if f.isHello() {
			f.pending = f.pending[4:]
		}
	}

	if len(f.pending) < HeaderSize {
		return Packet{}, false, nil
	}

	size := int(binary.LittleEndian.Uint16(f.pending[offSize:]))
	if size < HeaderSize || size > MaxPacketSize {
		f.pending = f.pending[:0]
		return Packet{}, false, ErrInvalidPacketSize
	}
	if len(f.pending) < size {
		return Packet{}, false, nil
	}

	copy(f.scratch, f.pending[:size])
	clear(f.scratch[size:])
	f.pending = f.pending[size:]
	f.compact()

	// Cada pacote traz a sua propria chave, entao decifra so os bytes dele.
	checksumOK, err := Decrypt(f.scratch, size)
	if err != nil {
		return Packet{}, false, err
	}

	return Packet{
		Size:       size,
		Code:       binary.LittleEndian.Uint16(f.scratch[6:]),
		Data:       f.scratch,
		ChecksumOK: checksumOK,
	}, true, nil
}

// compact devolve a capacidade do slice ao inicio quando o resto pendente ja
// e pequeno, para o append de Feed nao crescer sem limite.
func (f *Framer) compact() {
	if len(f.pending) == 0 {
		f.pending = f.pending[:0:cap(f.pending)]
		return
	}
	if cap(f.pending) < 4096 {
		return
	}
	rest := make([]byte, len(f.pending), max(4096, 2*len(f.pending)))
	copy(rest, f.pending)
	f.pending = rest
}

// isHello reconhece os 4 bytes de "hello" que o cliente manda ao conectar, as
// vezes sozinhos, as vezes colados no primeiro pacote.
//
// Difere de proposito de TPlayerThread.IsHello. O original tinha dois palpites
// que erram quando o primeiro segmento TCP chega curto:
//
//   - "if (FPendingLen = 4) then Exit(True)": qualquer pacote cujos 4
//     primeiros bytes chegassem sozinhos era tomado por hello, e os 4 bytes
//     eram descartados — a conexao dessincronizava na primeira leitura.
//   - o palpite do tamanho no offset 4 compara contra o que ja chegou, nao
//     contra o pacote inteiro, entao num pacote partido ele acerta por acaso
//     e come outros 4 bytes.
//
// Aqui vale so a assinatura documentada no proprio comentario do Delphi
// ("?? ?? 11 1F") mais a variante "11 F3" que ele tambem reconhecia. Isso e
// deterministico e nao depende de como o TCP fatiou o fluxo. O caso que resta
// e um primeiro pacote cujo checksum seja 0x11 e cuja chave seja 0x1F (1 em
// 65536, custo de uma reconexao) — contra desincronizar sempre que o cliente
// manda os 4 primeiros bytes em segmento proprio.
func (f *Framer) isHello() bool {
	p := f.pending
	if len(p) < 4 {
		return false
	}
	if p[0] == 0x11 && p[1] == 0xF3 {
		return true
	}
	return p[2] == 0x11 && p[3] == 0x1F
}
