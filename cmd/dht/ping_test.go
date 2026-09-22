package main

import (
	"net"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
)

// A response without an id is a successful KRPC reply. SenderID is nil, and ping must not
// dereference it. PingQueryInput already skips a nil id.
func TestPingResponseWithoutID(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		b := make([]byte, 1500)
		n, addr, err := pc.ReadFrom(b)
		if err != nil {
			return
		}
		var q krpc.Msg
		if err := bencode.Unmarshal(b[:n], &q); err != nil {
			return
		}
		rb, err := bencode.Marshal(krpc.Msg{Y: krpc.YResponse, T: q.T})
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(rb, addr)
	}()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:             conn,
		NoSecurity:       true,
		QueryResendDelay: func() time.Duration { return time.Hour },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	err = ping(pingArgs{
		Network: "udp",
		Nodes:   []string{pc.LocalAddr().String()},
	}, s)
	if err != nil {
		t.Fatal(err)
	}
}
