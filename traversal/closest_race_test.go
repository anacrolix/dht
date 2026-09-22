package traversal

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/anacrolix/generics"

	"github.com/anacrolix/dht/v2/int160"
	k_nearest_nodes "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/types"
)

// Closest hands out the live struct. A query response replaces it while a caller ranges it.
func TestClosestDoesNotRaceQuery(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var responder int160.T
	responder.SetBit(3, true)
	op := Start(OperationInput{
		Alpha: 1,
		K:     8,
		DoQuery: func(context.Context, krpc.NodeAddr) QueryResult {
			close(started)
			<-release
			ni := krpc.NodeInfo{
				ID:   responder.AsByteArray(),
				Addr: krpc.NodeAddr{IP: netip.MustParseAddr("1.2.3.4").AsSlice(), Port: 1},
			}
			return QueryResult{ResponseFrom: &ni, ClosestData: "token"}
		},
	})
	var id int160.T
	id.SetBit(4, true)
	op.AddNodes([]types.AddrMaybeId{{
		Addr: krpc.NodeAddrPort{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1)},
		Id:   generics.Some(id),
	}})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("query did not start")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 20000 {
			op.Closest().Range(func(k_nearest_nodes.Elem) {})
		}
	}()
	close(release)
	<-done
	op.Stop()
	<-op.Stopped()
}
