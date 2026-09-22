package dht

import (
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/iplist"

	"github.com/anacrolix/dht/v2/krpc"
)

// Close is documented to stop the server network activity. A ping that has already been written
// waits out QueryResendDelay unless Close cancels it. An hour-long delay must not stick.
func TestPingReturnsAfterClose(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	sent := make(chan struct{})
	go func() {
		b := make([]byte, 1500)
		if _, _, err := pc.ReadFrom(b); err == nil {
			close(sent)
		}
	}()
	s, err := NewServer(&ServerConfig{
		Conn:             mustListen("127.0.0.1:0"),
		NoSecurity:       true,
		QueryResendDelay: func() time.Duration { return time.Hour },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	done := make(chan QueryResult, 1)
	go func() {
		done <- s.Ping(pc.LocalAddr().(*net.UDPAddr))
	}()
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("ping was not written")
	}
	s.Close()
	select {
	case res := <-done:
		if res.Err == nil {
			t.Fatal("ping succeeded after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ping did not return after Close")
	}
}

type swapList struct{ n int }

func (swapList) Lookup(net.IP) (iplist.Range, bool) { return iplist.Range{}, false }
func (s swapList) NumRanges() int                   { return s.n }

// SetIPBlockList stores the list under s.mu, but IPBlocklist and TraversalNodeFilter read it
// without that lock. Concurrent replacement must not race.
func TestIPBlocklistConcurrentRead(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		Conn:       mustListen("127.0.0.1:0"),
		NoSecurity: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	node := addrMaybeId{Addr: krpc.NodeAddrPort{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("8.8.8.8"), 6881)}}
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 2000 {
			s.SetIPBlockList(swapList{1})
			s.SetIPBlockList(nil)
		}
	})
	wg.Go(func() {
		for range 2000 {
			_ = s.IPBlocklist()
			_ = s.TraversalNodeFilter(node)
		}
	})
	wg.Wait()
}
