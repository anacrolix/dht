package dht

import (
	"sync"

	"github.com/anacrolix/dht/v2/krpc"
)

// Populates the node table.
func (s *Server) Bootstrap() (ts TraversalStats, err error) {
	initialAddrs, err := s.traversalStartingNodes()
	if err != nil {
		return
	}
	var outstanding sync.WaitGroup
	triedAddrs := newBloomFilterForTraversal()
	var onAddr func(addr Addr)
	onAddr = func(addr Addr) {
		if triedAddrs.Test([]byte(addr.String())) {
			return
		}
		ts.NumAddrsTried++
		outstanding.Add(1)
		triedAddrs.AddString(addr.String())
		s.findNode(addr, s.id, func(m krpc.Msg, err error) {
			defer outstanding.Done()
			s.mu.Lock()
			defer s.mu.Unlock()
			if err != nil {
				return
			}
			ts.NumResponses++
			if r := m.R; r != nil {
				r.ForAllNodes(func(ni krpc.NodeInfo) {
					onAddr(NewAddr(ni.Addr.UDP()))
				})
			}
		})
	}
	s.mu.Lock()
	for _, addr := range initialAddrs {
		onAddr(NewAddr(addr.Addr.UDP()))
	}
	s.mu.Unlock()
	outstanding.Wait()
	return
}
