package dht

import (
	"sync/atomic"

	"github.com/anacrolix/stm"
)

// Populates the node table.
func (s *Server) Bootstrap() (ts TraversalStats, err error) {
	traversal, err := s.newTraversal(s.id)
	if err != nil {
		return
	}
	traversal.reason = "dht bootstrap find_node"
	traversal.doneVar = stm.NewVar(false)
	traversal.stopTraversal = func(*stm.Tx, addrMaybeId) bool {
		return false
	}
	traversal.query = func(addr Addr) QueryResult {
		atomic.AddInt64(&ts.NumAddrsTried, 1)
		res := s.FindNode(addr, s.id, QueryRateLimiting{NotFirst: true})
		if res.Err == nil {
			atomic.AddInt64(&ts.NumResponses, 1)
		}
		return res
	}
	traversal.run()
	return
}
