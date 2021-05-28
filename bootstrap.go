package dht

import (
	"errors"
	"time"

	"github.com/anacrolix/stm"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/types"
	dhtutil "github.com/anacrolix/dht/v2/util"
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
	nearestNodes := stm.NewVar(dhtutil.NewKNearestNodes(s.id))
	numResponses := stm.NewBuiltinEqVar(0)
	t, err := s.NewTraversal(NewTraversalInput{
		Target: s.id.AsByteArray(),
		Query: func(addr Addr) QueryResult {
			res := s.FindNode(addr, s.id, QueryRateLimiting{NotFirst: true})
			if res.Err == nil && res.Reply.R != nil {
				stm.AtomicModify(numResponses, func(i int) int { return i + 1 })
				senderId := int160.FromByteArray([20]byte(res.Reply.R.ID))
				stm.AtomicModify(nearestNodes, func(in dhtutil.KNearestNodes) dhtutil.KNearestNodes {
					return in.Push(dhtutil.KNearestNodesElem{
						AddrMaybeId: types.AddrMaybeId{
							Addr: addr.KRPC(),
							Id:   &senderId,
						},
					})
				})
			}
			return res
		},
		QueryConnectionTrackingReason: "dht bootstrap find_node",
		StopTraversal: func(tx *stm.Tx, next addrMaybeId) bool {
			nn := tx.Get(nearestNodes).(dhtutil.KNearestNodes)
			if !nn.Full() {
				return false
			}
			f, ok := nn.Farthest()
			if !ok {
				panic("k is zero?")
			}
			return f.CloserThan(next, s.id)
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
