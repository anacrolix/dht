package dht

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/log"
	"github.com/go-quicktest/qt"
)

// Counts goroutines currently running a traversal operation loop.
func numTraversalGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), "traversal.(*Operation).run(")
		}
		buf = make([]byte, 2*len(buf))
	}
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
