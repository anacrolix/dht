package getput

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/go-quicktest/qt"
)

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

func assertTraversalGoroutines(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for numTraversalGoroutines() != want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	qt.Assert(t, qt.Equals(numTraversalGoroutines(), want))
}

// A traversal must not outlive a failure to get starting nodes.
func TestStartingNodesErrorDoesNotLeak(t *testing.T) {
	conn, err := net.ListenPacket("udp", "localhost:0")
	qt.Assert(t, qt.IsNil(err))
	cfg := dht.NewDefaultServerConfig()
	cfg.Conn = conn
	cfg.StartingNodes = func() ([]dht.Addr, error) { return nil, errors.New("no starting nodes") }
	s, err := dht.NewServer(cfg)
	qt.Assert(t, qt.IsNil(err))
	defer s.Close()
	before := numTraversalGoroutines()

	_, _, err = Get(context.Background(), bep44.Target{1}, s, nil, nil)
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)

	_, err = Put(context.Background(), krpc.ID{1}, s, nil, func(int64) bep44.Put { return bep44.Put{} })
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)
}
