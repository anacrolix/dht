package main

import (
	"net"
	"testing"
	"time"
)

// A missing table file used to return after NewServer without Close. The deferred socket close
// then made serveUntilClosed panic: read on a closed connection while the server was still open.
func TestTableLoadErrorDoesNotPanic(t *testing.T) {
	flags.TableFile = "no-such-dht-table-file"
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = initServer(conn)
	if err == nil {
		t.Fatal("expected missing table file")
	}
	time.Sleep(300 * time.Millisecond)
}
