package dht

import (
	"errors"
	"time"

	"github.com/anacrolix/stm"

	dhtutil "github.com/anacrolix/dht/v2/k-nearest-nodes"
)

// Populates the node table.
func (s *Server) Bootstrap() (_ TraversalStats, err error) {
	s.mu.Lock()
	if s.bootstrappingNow {
		s.mu.Unlock()
		err = errors.New("already bootstrapping")
		return
	}
	s.bootstrappingNow = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.bootstrappingNow = false
	}()
	// Track number of responses, for STM use. (It's available via atomic in TraversalStats but that
	// won't let wake up STM transactions that are observing the value.)
	nearestNodes := stm.NewVar(dhtutil.New(s.id, 16))
	numResponses := stm.NewBuiltinEqVar(0)
	t, err := s.NewTraversal(NewTraversalInput{
		Target: s.id.AsByteArray(),
		Query: func(addr Addr) QueryResult {
			res := s.FindNode(addr, s.id, QueryRateLimiting{NotFirst: true})
			if res.Err == nil && res.Reply.R != nil {
				stm.AtomicModify(numResponses, func(i int) int { return i + 1 })
				stm.AtomicModify(nearestNodes, func(in dhtutil.Type) dhtutil.Type {
					return in.Push(dhtutil.Elem{
						Key: dhtutil.Key{
							Addr: addr.KRPC(),
							ID:   res.Reply.R.ID,
						},
					})
				})
			}
			return res
		},
		QueryConnectionTrackingReason: "dht bootstrap find_node",
		StopTraversal: func(tx *stm.Tx, next addrMaybeId) bool {
			nn := tx.Get(nearestNodes).(dhtutil.Type)
			if !nn.Full() {
				return false
			}
			if next.Id == nil {
				return false
			}
			f := nn.Farthest()
			return f.ID.Int160().Distance(s.id).Cmp(next.Id.Distance(s.id)) < 0
		},
	})
	if err != nil {
		return
	}
	s.mu.Lock()
	s.lastBootstrap = time.Now()
	s.mu.Unlock()
	t.doneVar = stm.NewVar(false)
	t.Run()
	return t.stats, nil
}
