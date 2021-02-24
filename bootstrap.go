package dht

import (
	"github.com/anacrolix/stm"
)

// Populates the node table.
func (s *Server) Bootstrap() (_ TraversalStats, err error) {
	t, err := s.newTraversal(s.id)
	if err != nil {
		return
	}
	t.reason = "dht bootstrap find_node"
	t.doneVar = stm.NewVar(false)
	// Track number of responses, for STM use. (It's available via atomic in TraversalStats but that
	// won't let wake up STM transactions that are observing the value.)
	numResponseStm := stm.NewBuiltinEqVar(0)
	t.stopTraversal = func(tx *stm.Tx, _ addrMaybeId) bool {
		return tx.Get(numResponseStm).(int) >= 100
	}
	t.query = func(addr Addr) QueryResult {
		res := s.FindNode(addr, s.id, QueryRateLimiting{NotFirst: true})
		if res.Err == nil {
			stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
				tx.Set(numResponseStm, tx.Get(numResponseStm).(int)+1)
			}))
		}
		return res
	}
	t.run()
	return t.stats, nil
}
