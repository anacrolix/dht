package dht

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"golang.org/x/time/rate"
)

func TestCloseCancelsRateLimitedReply(t *testing.T) {
	limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	if !limiter.Allow() {
		t.Fatal("could not consume initial token")
	}
	s, err := NewServer(&ServerConfig{Conn: mustListen("127.0.0.1:0"), NoSecurity: true, WaitToReply: true, SendLimiter: limiter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.reply(NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}), "test", krpc.Return{})
	deadline := time.Now().Add(time.Second)
	for limiter.Tokens() >= 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if limiter.Tokens() >= 0 {
		t.Fatal("reply did not reserve a rate-limit token")
	}
	s.Close()
	deadline = time.Now().Add(time.Second)
	for limiter.Tokens() < 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if limiter.Tokens() < 0 {
		t.Fatal("Close left reply waiting on its rate-limit reservation")
	}
}

type countingWriteConn struct {
	net.PacketConn
	writes atomic.Int32
}

func (c *countingWriteConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.writes.Add(1)
	return c.PacketConn.WriteTo(b, addr)
}

func TestBlocklistClosePreventsWrite(t *testing.T) {
	conn := &countingWriteConn{PacketConn: mustListen("127.0.0.1:0")}
	var s *Server
	cfg := NewDefaultServerConfig()
	cfg.Conn = conn
	cfg.IPBlocklist = &callbackTestRanger{lookup: func(net.IP) { s.Close() }}
	var err error
	s, err = NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	wrote, err := s.writeToNode(context.Background(), []byte("query"), NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}), false, false)
	if wrote || err == nil {
		t.Fatalf("write after callback Close: wrote=%v err=%v", wrote, err)
	}
	if got := conn.writes.Load(); got != 0 {
		t.Fatalf("WriteTo called %d times after callback closed server", got)
	}
}
