package dht

import (
	"context"
	"crypto/rand"
	"net"
	"regexp"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/go-quicktest/qt"
)

func TestAnnounceNoStartingNodes(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		Conn:       mustListen(":0"),
		NoSecurity: true,
	})
	qt.Assert(t, qt.IsNil(err))
	defer s.Close()
	var ih [20]byte
	copy(ih[:], "blah")
	_, err = s.Announce(ih, 0, true)
	qt.Assert(t, qt.ErrorMatches(err, regexp.QuoteMeta("no initial nodes")))
}

func randomInfohash() (ih [20]byte) {
	rand.Read(ih[:])
	return
}

func TestAnnounceStopsNoPending(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		Conn: mustListen(":0"),
		StartingNodes: func() ([]Addr, error) {
			return []Addr{NewAddr(&net.TCPAddr{})}, nil
		},
	})
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(s.Close)
	a, err := s.Announce(randomInfohash(), 0, true)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(a.Close)
	select {
	case <-a.Finished():
	case <-time.After(2 * time.Second):
		t.Fatal("empty traversal did not finish")
	}
}

// Assert that rate.Limiter won't wake-up waiters once they have determined a
// delay. This means we can't use it to cancel reservations for queries that
// are successful.
func TestRateLimiterInadequate(t *testing.T) {
	rl := rate.NewLimiter(rate.Every(time.Hour), 1)
	qt.Check(t, qt.IsNil(rl.Wait(context.Background())))
	time.AfterFunc(time.Millisecond, func() { rl.AllowN(time.Now(), -1) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(2*time.Millisecond, cancel)
	qt.Check(t, qt.Equals(rl.Wait(ctx), context.Canceled))
}

// Stats can be read while a query is still outstanding.
func TestTraversalStatsDuringQuery(t *testing.T) {
	silent := mustListen("localhost:0")
	defer silent.Close()
	s, err := NewServer(&ServerConfig{
		Conn:             mustListen("localhost:0"),
		NoSecurity:       true,
		SendLimiter:      rate.NewLimiter(rate.Inf, 0),
		QueryResendDelay: func() time.Duration { return time.Hour },
		StartingNodes:    addrResolver(silent.LocalAddr().String()),
	})
	qt.Assert(t, qt.IsNil(err))
	defer s.Close()
	a, err := s.AnnounceTraversal([20]byte{9})
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(a.Close)
	qt.Assert(t, qt.IsNil(silent.SetReadDeadline(time.Now().Add(2*time.Second))))
	var packet [1500]byte
	_, _, err = silent.ReadFrom(packet[:])
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(a.TraversalStats().NumAddrsTried, uint32(1)))
	qt.Assert(t, qt.Equals(a.NumContacted(), uint32(1)))
	a.Close()
	select {
	case <-a.Finished():
	case <-time.After(2 * time.Second):
		t.Fatal("stats traversal did not finish")
	}
}
