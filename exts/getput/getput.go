package getput

import (
	"context"
	"crypto/sha1"
	"errors"
	"math"
	"sync"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	k_nearest_nodes "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
)

type GetResult struct {
	Seq     int64
	V       bencode.Bytes
	Sig     [64]byte
	Mutable bool
}

// Returns the item carried by r if it is the immutable item for target, or a correctly signed
// mutable item for target and salt.
func verifiedResult(r *krpc.Return, target bep44.Target, salt []byte) (GetResult, bool) {
	if sha1.Sum(r.V) == target {
		return GetResult{V: r.V, Sig: r.Sig}, true
	}
	if r.Seq != nil &&
		bep44.MakeMutableTarget(r.K, salt) == target &&
		bep44.Verify(r.K[:], salt, *r.Seq, r.V, r.Sig[:]) {
		return GetResult{Seq: *r.Seq, V: r.V, Sig: r.Sig, Mutable: true}, true
	}
	return GetResult{}, false
}

func startGetTraversal(
	target bep44.Target, s *dht.Server, seq *int64, salt []byte,
) (
	vChan chan GetResult, op *traversal.Operation, err error,
) {
	vChan = make(chan GetResult)
	op = traversal.Start(traversal.OperationInput{
		Alpha:  15,
		Target: target,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			logger := log.ContextLogger(ctx)
			res := s.Get(ctx, dht.NewAddr(addr.UDP()), target, seq, dht.QueryRateLimiting{})
			err := res.ToError()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, dht.TransactionTimeout) {
				logger.Levelf(log.Debug, "error querying %v: %v", addr, err)
			}
			if r := res.Reply.R; r != nil {
				if v, ok := verifiedResult(r, target, salt); ok {
					select {
					case vChan <- v:
					case <-ctx.Done():
					}
				} else if r.V != nil {
					logger.Levelf(log.Debug, "get response item hash didn't match target: %q", r.V)
				}
			}
			return res.TraversalQueryResult(addr)
		},
		NodeFilter: s.TraversalNodeFilter,
		// Only nodes that gave us a token can be put to.
		DataFilter: func(data any) bool {
			_, ok := data.(string)
			return ok
		},
	})
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
		op.Stop()
		return nil, nil, err
	}
	op.AddNodes(nodes)
	return
}

func Get(
	ctx context.Context, target bep44.Target, s *dht.Server, seq *int64, salt []byte,
) (
	ret GetResult, stats *traversal.Stats, err error,
) {
	vChan, op, err := startGetTraversal(target, s, seq, salt)
	if err != nil {
		return
	}
	ret.Seq = math.MinInt64
	gotValue := false
receive:
	for {
		select {
		case <-op.Stalled():
			if !gotValue {
				err = errors.New("value not found")
			}
			break receive
		case v := <-vChan:
			log.ContextLogger(ctx).Levelf(log.Debug, "received %#v", v)
			gotValue = true
			if !v.Mutable {
				ret = v
				break receive
			}
			if v.Seq >= ret.Seq {
				ret = v
			}
		case <-ctx.Done():
			err = ctx.Err()
			break receive
		}
	}
	op.Stop()
	<-op.Stopped()
	stats = op.Stats()
	return
}

type SeqToPut func(seq int64) bep44.Put

func Put(
	ctx context.Context, target krpc.ID, s *dht.Server, salt []byte, seqToPut SeqToPut,
) (
	stats *traversal.Stats, err error,
) {
	logger := log.ContextLogger(ctx)
	// The seq filter is irrelevant for a put, but the salt is needed to verify responses for the
	// automatic sequence number.
	vChan, op, err := startGetTraversal(target, s, nil, salt)
	if err != nil {
		return
	}
	var autoSeq int64
receive:
	for {
		select {
		case v := <-vChan:
			// TODO: Set CAS automatically, and republish the existing seq if the content already
			// matches.
			if v.Mutable && v.Seq > autoSeq {
				autoSeq = v.Seq
			}
		case <-op.Stalled():
			break receive
		case <-ctx.Done():
			err = ctx.Err()
			break receive
		}
	}
	op.Stop()
	<-op.Stopped()
	stats = op.Stats()
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	put := seqToPut(autoSeq)
	op.Closest().Range(func(elem k_nearest_nodes.Elem) {
		wg.Go(func() {
			// This is enforced by the DataFilter in startGetTraversal.
			token := elem.Data.(string)
			res := s.Put(ctx, dht.NewAddr(elem.Addr.UDP()), put, token, dht.QueryRateLimiting{})
			if err := res.ToError(); err != nil {
				logger.Levelf(log.Warning, "error putting to %v [token=%q]: %v", elem.Addr, token, err)
			} else {
				logger.Levelf(log.Debug, "put to %v [token=%q]", elem.Addr, token)
			}
		})
	})
	wg.Wait()
	return
}
