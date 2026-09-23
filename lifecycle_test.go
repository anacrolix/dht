package dht

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/log"
	"github.com/go-quicktest/qt"
	"golang.org/x/time/rate"
)

// Counts goroutines with a frame in the traversal package: operation loops and their queries.
func numTraversalGoroutines() (num int) {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(g, "dht/v2/traversal.") {
			num++
		}
	}
	return
}

// Waits briefly for the traversal goroutine count to settle back to want.
func assertTraversalGoroutines(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for numTraversalGoroutines() != want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	qt.Assert(t, qt.Equals(numTraversalGoroutines(), want))
}

func newServerWithoutStartingNodes(t *testing.T) *Server {
	cfg := NewDefaultServerConfig()
	cfg.Conn = mustListen("localhost:0")
	cfg.Logger = log.Default.WithNames(t.Name())
	cfg.StartingNodes = func() ([]Addr, error) { return nil, errors.New("no starting nodes") }
	s, err := NewServer(cfg)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(s.Close)
	return s
}

// A traversal must not outlive a failure to get starting nodes.
func TestTraversalStartingNodesErrorDoesNotLeak(t *testing.T) {
	s := newServerWithoutStartingNodes(t)
	before := numTraversalGoroutines()

	_, err := s.Bootstrap()
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)

	_, err = s.AnnounceTraversal([20]byte{1})
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)
}

// Cancelling a bootstrap must stop its outstanding queries before returning, rather than leaving
// them to run until they time out.
func TestBootstrapContextCancelWaitsForTraversal(t *testing.T) {
	silent := mustListen("127.0.0.1:0")
	t.Cleanup(func() { _ = silent.Close() })
	cfg := NewDefaultServerConfig()
	cfg.Conn = mustListen("127.0.0.1:0")
	cfg.Logger = log.Default.WithNames(t.Name())
	cfg.SendLimiter = rate.NewLimiter(rate.Inf, 0)
	cfg.QueryResendDelay = func() time.Duration { return time.Hour }
	cfg.StartingNodes = addrResolver(silent.LocalAddr().String())
	s, err := NewServer(cfg)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(s.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	type result struct {
		tried uint32
		err   error
	}
	done := make(chan result, 1)
	go func() {
		stats, err := s.BootstrapContext(ctx)
		done <- result{stats.NumAddrsTried, err}
	}()
	qt.Assert(t, qt.IsNil(silent.SetReadDeadline(time.Now().Add(2*time.Second))))
	var packet [1500]byte
	_, _, err = silent.ReadFrom(packet[:])
	qt.Assert(t, qt.IsNil(err))
	cancel()
	select {
	case got := <-done:
		qt.Assert(t, qt.ErrorIs(got.err, context.Canceled))
		qt.Assert(t, qt.Equals(got.tried, uint32(1)))
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not stop its active query after cancellation")
	}
	qt.Assert(t, qt.Equals(s.Stats().OutstandingTransactions, 0))
}

// The traversal goroutine must be gone when refreshBucket returns. This does not observe the
// stats-before-Stopped ordering; that return is assigned only after Stopped in refreshBucket.
func TestRefreshBucketStopsBeforeStats(t *testing.T) {
	s := newServerWithoutStartingNodes(t)
	before := numTraversalGoroutines()
	stats := s.refreshBucket(0)
	qt.Assert(t, qt.IsNotNil(stats))
	assertTraversalGoroutines(t, before)
}
