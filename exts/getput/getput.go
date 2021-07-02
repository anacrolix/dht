package getput

import (
	"context"
	"crypto/sha1"
	"errors"
	"log"
	"sync"

	"github.com/anacrolix/dht/v2"
	k_nearest_nodes "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
)

type PutGetResult struct {
	GetV           interface{}
	GetBytes       []byte
	TraversalStats *traversal.Stats
	SuccessfulPuts []krpc.NodeAddr
}

func Get(
	ctx context.Context, target krpc.ID, s *dht.Server,
) (
	v interface{}, stats *traversal.Stats, err error,
) {
	vChan := make(chan interface{}, 1)
	op := traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.Query(ctx, dht.NewAddr(addr.UDP()), "get", dht.QueryInput{
				MsgArgs: krpc.MsgArgs{
					Target: target,
				},
			})
			err := res.ToError()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, dht.TransactionTimeout) {
				//log.Printf("error querying %v: %v", addr, err)
			}
			if r := res.Reply.R; r != nil {
				rv := r.V
				if rv != nil {
					b, err := bencode.Marshal(rv)
					if err != nil {
						log.Printf("re-marshalling v: %v", err)
					}
					h := sha1.Sum(b)
					if h == target {
						log.Printf("got %v from %v", target, addr)
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
			res := s.Query(ctx, dht.NewAddr(addr.UDP()), "get", dht.QueryInput{
				MsgArgs: krpc.MsgArgs{
					Target: target,
				},
			})
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
		var wg sync.WaitGroup
		op.Closest().Range(func(elem k_nearest_nodes.Elem) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := s.Query(ctx, dht.NewAddr(elem.Addr.UDP()), "put", dht.QueryInput{
					MsgArgs: krpc.MsgArgs{
						Token: elem.Data.(string),
						V:     put,
					},
				})
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
