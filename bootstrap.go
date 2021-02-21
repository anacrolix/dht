package dht

import (
	"sync/atomic"

	"github.com/anacrolix/stm"
	"github.com/bradfitz/iter"

	"github.com/anacrolix/dht/v2/krpc"
)

func randomIdInBucket(rootId int160, bucketIndex int) int160 {
	id := int160FromByteArray(RandomNodeID())
	for i := range iter.N(bucketIndex) {
		id.SetBit(i, rootId.GetBit(i))
	}
	id.SetBit(bucketIndex, !rootId.GetBit(bucketIndex))
	return id
}

//
//func (s *Server) refreshBucket(b *bucket) {
//	targetId = RandomNodeID()
//}

// Populates the node table.
func (s *Server) Bootstrap() (ts TraversalStats, err error) {
	traversal, err := s.newTraversal(s.id)
	if err != nil {
		return
	}
	outstanding := stm.NewVar(0)
	for {
		type txResT struct {
			done bool
			io   func()
		}
		txRes := stm.Atomically(stm.Select(
			func(tx *stm.Tx) interface{} {
				addr, ok := traversal.popNextContact(tx)
				tx.Assert(ok)
				dhtAddr := NewAddr(addr.Addr.UDP())
				tx.Set(outstanding, tx.Get(outstanding).(int)+1)
				return txResT{
					io: s.beginQuery(dhtAddr, "dht bootstrap find_node", func() numWrites {
						atomic.AddInt64(&ts.NumAddrsTried, 1)
						res := s.FindNode(dhtAddr, s.id, QueryRateLimiting{NotFirst: true})
						if res.Err == nil {
							atomic.AddInt64(&ts.NumResponses, 1)
						}
						if r := res.Reply.R; r != nil {
							r.ForAllNodes(func(ni krpc.NodeInfo) {
								id := int160FromByteArray(ni.ID)
								stm.Atomically(traversal.pendContact(addrMaybeId{
									Addr: ni.Addr,
									Id:   &id,
								}))
							})
						}
						stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
							tx.Set(outstanding, tx.Get(outstanding).(int)-1)
						}))
						return res.writes
					})(tx).(func()),
				}
			},
			func(tx *stm.Tx) interface{} {
				tx.Assert(tx.Get(outstanding).(int) == 0)
				return txResT{done: true}
			},
		)).(txResT)
		if txRes.done {
			break
		}
		go txRes.io()
	}
	return
}
