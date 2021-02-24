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
	t.stopTraversal = func(*stm.Tx, addrMaybeId) bool {
		return false
	}
	t.query = func(addr Addr) QueryResult {
		return s.FindNode(addr, s.id, QueryRateLimiting{NotFirst: true})
	}
	t.run()
	return t.stats, nil
}
