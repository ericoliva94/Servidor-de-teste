package game

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"aika/internal/devir"
	"aika/internal/netsrv"
	"aika/internal/packets"
)

// leAte le pacotes do socket ate achar o codigo, e devolve o corpo decifrado.
func leAte(t *testing.T, c net.Conn, code uint16) []byte {
	t.Helper()
	for i := 0; i < 400; i++ {
		got, buf, err := lerPacote(c)
		if err != nil {
			t.Fatalf("esperando %#x: %v", code, err)
		}
		if got == code {
			return buf
		}
	}
	t.Fatalf("pacote %#x nao chegou", code)
	return nil
}

// Troca de canal de ponta a ponta, por socket de verdade, entre dois canais de
// nacoes diferentes (como o SL.bin: canal 0 = nacao 1, canal 1 = nacao 2).
func TestTrocaDeCanalEntreNacoesPorSocket(t *testing.T) {
	h0, h1, _ := doisCanais(t)
	num := novoNumerics()
	h0.Numerics, h1.Numerics = num, num
	h0.Devir, h1.Devir = devir.New(1), devir.New(2)
	if h0.Devir.Nation == h1.Devir.Nation {
		t.Fatal("os canais deveriam ter nacoes diferentes")
	}
	addr0, stop0 := serve(t, h0)
	defer stop0()
	addr1, stop1 := serve(t, h1)
	defer stop1()

	// Canal 0: login, numerica e entrada no mundo.
	c0, err := net.Dial("tcp", addr0)
	if err != nil {
		t.Fatal(err)
	}
	defer c0.Close()
	if _, err := c0.Write(loginPacket("teste", "tok123", 305)); err != nil {
		t.Fatal(err)
	}
	leAte(t, c0, CodeCharList)
	if _, err := c0.Write(numericPacket(0, NumericVerify, "1234", "")); err != nil {
		t.Fatal(err)
	}
	leAte(t, c0, CodeSendToWorld)
	if h0.Chan.Count() != 1 {
		t.Fatalf("jogadores no canal 0: %d", h0.Chan.Count())
	}

	// Pede o canal 1.
	if _, err := c0.Write(pedidoDeCanal(t, 1)); err != nil {
		t.Fatal(err)
	}
	corpo := leAte(t, c0, CodeChangeChannel)
	ordem := decodificaCru[packets.ChangeChannelPacket](t, corpo)
	if ordem.Info1 != 7 || ordem.TypeChanel != 1 {
		t.Fatalf("ordem = %+v, queria conta 7 e canal 1", ordem)
	}
	// O servidor solta a conexao do canal velho.
	c0.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := lerPacote(c0); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatalf("esperava a conexao fechada: %v", err)
			}
			break
		}
	}

	// Canal 1: apresenta-se com a conta e o usuario.
	c1, err := net.Dial("tcp", addr1)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	var f06 packets.ChannelSendInfoPacket
	f06.Serial = ordem.Info1
	netsrv.PutCString(f06.Username[:], "teste")
	raw, err := netsrv.Marshal(CodeLoginIntoChannel, f06)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c1.Write(raw); err != nil {
		t.Fatal(err)
	}
	mundo := decodificaCru[packets.SendToWorldPacket](t, leAte(t, c1, CodeSendToWorld))
	if got := netsrv.CString(mundo.Character.Name[:]); got != "Guerreiro" {
		t.Errorf("personagem no canal novo = %q", got)
	}
	if mundo.Character.Nation != 2 {
		t.Errorf("nacao do personagem = %d (a da conta)", mundo.Character.Nation)
	}
	// Cada canal tem os proprios jogadores e os proprios templos.
	deadline := time.Now().Add(2 * time.Second)
	for h0.Chan.Count() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h0.Chan.Count() != 0 || h1.Chan.Count() != 1 {
		t.Errorf("jogadores: canal0=%d canal1=%d, queria 0 e 1", h0.Chan.Count(), h1.Chan.Count())
	}
}

func decodificaCru[T any](t *testing.T, corpo []byte) T {
	t.Helper()
	return decodifica[T](t, pacoteVisto{corpo: corpo})
}
