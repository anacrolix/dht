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

func TestCancelActiveTraversal(t *testing.T) {
	for _, operation := range []string{"get", "put"} {
		t.Run(operation, func(t *testing.T) {
			peer, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(func() { _ = peer.Close() })
			conn, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			s, err := dht.NewServer(&dht.ServerConfig{
				Conn: conn, NoSecurity: true,
				QueryResendDelay: func() time.Duration { return time.Hour },
				StartingNodes: func() ([]dht.Addr, error) {
					return []dht.Addr{dht.NewAddr(peer.LocalAddr())}, nil
				},
			})
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(s.Close)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				if operation == "get" {
					_, _, err := Get(ctx, bep44.Target{1}, s, nil, nil)
					done <- err
				} else {
					_, err := Put(ctx, krpc.ID{1}, s, nil, func(int64) bep44.Put { return bep44.Put{} })
					done <- err
				}
			}()
			qt.Assert(t, qt.IsNil(peer.SetReadDeadline(time.Now().Add(2*time.Second))))
			var packet [1500]byte
			_, _, err = peer.ReadFrom(packet[:])
			qt.Assert(t, qt.IsNil(err))
			cancel()
			select {
			case err := <-done:
				qt.Assert(t, qt.IsTrue(errors.Is(err, context.Canceled)))
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled traversal did not return")
			}
			qt.Assert(t, qt.Equals(s.Stats().OutstandingTransactions, 0))
		})
	}
}
