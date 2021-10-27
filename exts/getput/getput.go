package getput

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	k_nearest_nodes "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
)

type PutGetResult struct {
	GetV           interface{}
	GetBytes       []byte
	TraversalStats *traversal.Stats
	SuccessfulPuts []krpc.NodeAddr
}

func Get(
	ctx context.Context, target bep44.Target, s *dht.Server,
) (
	v interface{}, stats *traversal.Stats, err error,
) {
	vChan := make(chan interface{}, 1)
	op := traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.Get(ctx, dht.NewAddr(addr.UDP()), target, 0, dht.QueryRateLimiting{})
			err := res.ToError()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, dht.TransactionTimeout) {
				//log.Printf("error querying %v: %v", addr, err)
			}
			if r := res.Reply.R; r != nil {
				rv := r.V
				if rv != nil {
					i, err := bep44.NewItem(rv, nil, 0, 0, nil)
					if err != nil {
						log.Printf("re-marshalling v: %v", err)
					}
					h := i.Target()
					if h == target {
						log.Printf("got %x from %v", target, addr)
						select {
						case vChan <- rv:
						default:
						}
					} else {
						log.Printf("returned v failed hash check: %x", h)
					}
				}
			}
			return res.TraversalQueryResult(addr)
		},
		NodeFilter: s.TraversalNodeFilter,
	})
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
		return
	}
	op.AddNodes(nodes)
	select {
	case <-op.Stalled():
		err = errors.New("value not found")
	case v = <-vChan:
	case <-ctx.Done():
		err = ctx.Err()
	}
	op.Stop()
	stats = op.Stats()
	return
}

func Put(
	ctx context.Context, target krpc.ID, s *dht.Server, put interface{},
) (
	stats *traversal.Stats, err error,
) {
	op := traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.Get(ctx, dht.NewAddr(addr.UDP()), target, 0, dht.QueryRateLimiting{})
			err := res.ToError()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, dht.TransactionTimeout) {
				//log.Printf("error querying %v: %v", addr, err)
			}
			tqr := res.TraversalQueryResult(addr)
			if tqr.ClosestData == nil {
				tqr.ResponseFrom = nil
			}
			return tqr
		},
		NodeFilter: s.TraversalNodeFilter,
	})
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
		return
	}
	op.AddNodes(nodes)
	select {
	case <-op.Stalled():
		if put == nil {
			err = errors.New("value not found")
		}
	case <-ctx.Done():
		err = ctx.Err()
	}
	op.Stop()
	if put != nil {
		item := bep44.Put{V: put}
		var wg sync.WaitGroup
		op.Closest().Range(func(elem k_nearest_nodes.Elem) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := s.Put(ctx, dht.NewAddr(elem.Addr.UDP()), item, elem.Data.(string), dht.QueryRateLimiting{})
				err := res.ToError()
				if err != nil {
					log.Printf("error putting to %v: %v", elem.Addr, err)
				} else {
					log.Printf("put to %v", elem.Addr)
				}
			}()
		})
		wg.Wait()
	}
	stats = op.Stats()
	return
}
