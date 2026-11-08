package netsrv

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"

	"aika/internal/protocol"
)

// traceOn liga o registro de todo pacote enviado e recebido (opcao -trace do
// servidor). Serve para achar qual pacote o cliente nao aceita quando ele
// trava; fica desligado por padrao porque o volume e alto.
var traceOn atomic.Bool

// SetTrace liga ou desliga o registro de pacotes.
func SetTrace(on bool) { traceOn.Store(on) }

// traceTx registra um pacote ja cifrado que sai para o cliente.
func traceTx(log *slog.Logger, ip string, wire []byte) {
	if !traceOn.Load() || log == nil || len(wire) < protocol.HeaderSize {
		return
	}
	cp := append([]byte(nil), wire...)
	if _, err := protocol.Decrypt(cp, len(cp)); err != nil {
		return
	}
	log.Info("tx", "ip", ip, "codigo", fmt.Sprintf("%#x", binary.LittleEndian.Uint16(cp[6:])),
		"tam", len(cp), "idx", binary.LittleEndian.Uint16(cp[4:]))
}

// traceRx registra um pacote que chegou do cliente.
func traceRx(log *slog.Logger, ip string, code uint16, size int, data []byte) {
	if !traceOn.Load() || log == nil {
		return
	}
	// O corpo (decifrado, com o cabecalho de 12 bytes) vai em hexa, ate 64 bytes:
	// e o que mostra, por exemplo, tipo de slot e slot de um movimento de item.
	n := min(len(data), 64)
	log.Info("rx", "ip", ip, "codigo", fmt.Sprintf("%#x", code), "tam", size,
		"corpo", hex.EncodeToString(data[:n]))
}
