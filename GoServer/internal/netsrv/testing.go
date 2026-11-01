package netsrv

import (
	"log/slog"
	"net"
	"time"

	"aika/internal/protocol"
)

// NewConnForTest monta uma Conn sobre um net.Conn qualquer.
//
// Serve para testar handlers sem subir um servidor inteiro: um net.Pipe do
// lado do teste e suficiente para conferir o que o handler envia.
func NewConnForTest(nc net.Conn) *Conn {
	return &Conn{
		nc:           nc,
		framer:       protocol.NewFramer(),
		ip:           remoteIP(nc),
		writeTimeout: 5 * time.Second,
		log:          slog.Default(),
	}
}
