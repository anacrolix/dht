package getput

import (
	"context"
	"crypto/sha1"
	"errors"
	"log"
	"math"
	"sync"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	k_nearest_nodes "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
	"github.com/davecgh/go-spew/spew"
)

type PutGetResult struct {
	GetV           interface{}
	GetBytes       []byte
	TraversalStats *traversal.Stats
	SuccessfulPuts []krpc.NodeAddr
}

type GetResult struct {
	Seq     int64
	V       interface{}
	Mutable bool
}

func Get(
	ctx context.Context, target bep44.Target, s *dht.Server, seq int64, salt []byte,
) (
	ret GetResult, stats *traversal.Stats, err error,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	vChan := make(chan GetResult)
	op := traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.Get(ctx, dht.NewAddr(addr.UDP()), target, seq, dht.QueryRateLimiting{})
			err := res.ToError()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, dht.TransactionTimeout) {
				log.Printf("error querying %v: %v", addr, err)
			}
			if r := res.Reply.R; r != nil {
				rv := r.V
				bv := bencode.MustMarshal(rv)
				spew.Dump(r)
				if sha1.Sum(bv) == target {
					select {
					case vChan <- GetResult{
						V:       rv,
						Mutable: false,
					}:
					case <-ctx.Done():
					}
				} else if sha1.Sum(append(r.K[:], salt...)) == target && bep44.Verify(r.K[:], salt, r.Seq, bv, r.Sig[:]) {
					select {
					case vChan <- GetResult{
						Seq:     r.Seq,
						V:       rv,
						Mutable: true,
					}:
					case <-ctx.Done():
					}
				} else {
					log.Printf("get response item didn't match target: %q", rv)
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
	ret.Seq = math.MinInt64
	gotValue := false
receiveResults:
	select {
	case <-op.Stalled():
		if !gotValue {
			err = errors.New("value not found")
		}
	case v := <-vChan:
		spew.Dump("got result", v)
		gotValue = true
		if !v.Mutable {
			ret = v
			break
		}
		if v.Seq >= ret.Seq {
			ret = v
		}
		goto receiveResults
	case <-ctx.Done():
		err = ctx.Err()
	}
	op.Stop()
	stats = op.Stats()
	return
}

func Put(
	ctx context.Context, target krpc.ID, s *dht.Server, put bep44.Put,
) (
	stats *traversal.Stats, err error,
) {
	op := traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.Get(ctx, dht.NewAddr(addr.UDP()), target, 1, dht.QueryRateLimiting{})
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
	case <-ctx.Done():
		err = ctx.Err()
	}
	op.Stop()
	var wg sync.WaitGroup
	op.Closest().Range(func(elem k_nearest_nodes.Elem) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token := elem.Data.(string)
			res := s.Put(ctx, dht.NewAddr(elem.Addr.UDP()), put, token, dht.QueryRateLimiting{})
			err := res.ToError()
			if err != nil {
				log.Printf("error putting to %v [token=%q]: %v", elem.Addr, token, err)
			} else {
				log.Printf("put to %v [token=%q]", elem.Addr, token)
			}
		}()
	})
	wg.Wait()
	stats = op.Stats()
	return
}
