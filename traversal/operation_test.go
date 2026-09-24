package traversal

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/types"
	"github.com/anacrolix/generics"
)

func TestQueryEachEndpointOnce(t *testing.T) {
	var mu sync.Mutex
	calls := make(map[string]int)
	started := make(chan struct{})
	var once sync.Once
	op := Start(OperationInput{
		Alpha: 3,
		DoQuery: func(_ context.Context, addr krpc.NodeAddr) QueryResult {
			mu.Lock()
			calls[addr.String()]++
			mu.Unlock()
			once.Do(func() { close(started) })
			return QueryResult{}
		},
	})
	t.Cleanup(func() {
		op.Stop()
		select {
		case <-op.Stopped():
		case <-time.After(2 * time.Second):
			t.Error("traversal did not stop")
		}
	})
	addr := krpc.NodeAddrPort{AddrPort: netip.MustParseAddrPort("127.0.0.1:6881")}
	other := krpc.NodeAddrPort{AddrPort: netip.MustParseAddrPort("127.0.0.1:6882")}
	var id1, id2 int160.T
	id1.SetBit(1, true)
	id2.SetBit(2, true)
	op.AddNodes([]types.AddrMaybeId{
		{Addr: addr},
		{Addr: addr, Id: generics.Some(id1)},
		{Addr: addr, Id: generics.Some(id2)},
		{Addr: other, Id: generics.Some(id1)},
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("query did not start")
	}
	select {
	case <-op.Stalled():
	case <-time.After(2 * time.Second):
		t.Fatal("traversal did not exhaust candidates")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, endpoint := range []krpc.NodeAddrPort{addr, other} {
		if got := calls[endpoint.String()]; got != 1 {
			t.Errorf("queries to %s = %d, want 1", endpoint.String(), got)
		}
	}
}
