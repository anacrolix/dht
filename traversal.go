package dht

import (
	"fmt"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/anacrolix/stm"
	"github.com/anacrolix/stm/stmutil"
)

type TraversalStats struct {
	// Count of (probably) distinct addresses we've sent traversal queries to. Accessed with atomic.
	NumAddrsTried int64
	// Number of responses we received to queries related to this traversal. Accessed with atomic.
	NumResponses int64
}

func (me TraversalStats) String() string {
	return fmt.Sprintf("%#v", me)
}

// Prioritizes addrs to try by distance from target, disallowing repeat contacts.
type traversal struct {
	targetInfohash      int160.T
	triedAddrs          *stm.Var // Settish of krpc.NodeAddr.String
	nodesPendingContact *stm.Var // Settish of addrMaybeId sorted by distance from the target
	addrBestIds         *stm.Var // Mappish Addr to best
	pending             *stm.Var
	doneVar             *stm.Var
	stopTraversal       func(_ *stm.Tx, next addrMaybeId) bool
	reason              string
	shouldContact       func(krpc.NodeAddr, *stm.Tx) bool
	// User-specified traversal query
	query func(Addr) QueryResult
	// A hook to a begin a query on the server, that expects to receive the number of writes back.
	serverBeginQuery func(Addr, string, func() numWrites) stm.Operation
	stats            TraversalStats
}

func newTraversal(targetInfohash int160.T) traversal {
	return traversal{
		targetInfohash:      targetInfohash,
		triedAddrs:          stm.NewVar(stmutil.NewSet()),
		nodesPendingContact: stm.NewVar(nodesByDistance(targetInfohash)),
		addrBestIds:         stm.NewVar(stmutil.NewMap()),
		pending:             stm.NewVar(0),
	}
}

func (t *traversal) waitFinished(tx *stm.Tx) {
	tx.Assert(tx.Get(t.nodesPendingContact).(stmutil.Lenner).Len() == 0)
}

func (t *traversal) pendContact(node addrMaybeId) stm.Operation {
	return stm.VoidOperation(func(tx *stm.Tx) {
		if !t.shouldContact(node.Addr, tx) {
			return
		}
		nodeAddrString := node.Addr.String()
		if tx.Get(t.triedAddrs).(stmutil.Settish).Contains(nodeAddrString) {
			return
		}
		addrBestIds := tx.Get(t.addrBestIds).(stmutil.Mappish)
		nodesPendingContact := tx.Get(t.nodesPendingContact).(stmutil.Settish)
		if _best, ok := addrBestIds.Get(nodeAddrString); ok {
			if node.Id == nil {
				return
			}
			best := _best.(*int160.T)
			if best != nil && int160.Distance(*best, t.targetInfohash).Cmp(int160.Distance(*node.Id, t.targetInfohash)) <= 0 {
				return
			}
			nodesPendingContact = nodesPendingContact.Delete(addrMaybeId{
				Addr: node.Addr,
				Id:   best,
			})
		}
		tx.Set(t.addrBestIds, addrBestIds.Set(nodeAddrString, node.Id))
		nodesPendingContact = nodesPendingContact.Add(node)
		tx.Set(t.nodesPendingContact, nodesPendingContact)
	})
}

func (a *traversal) nextContact(tx *stm.Tx) (ret addrMaybeId, ok bool) {
	npc := tx.Get(a.nodesPendingContact).(stmutil.Settish)
	first, ok := iter.First(npc.Iter)
	if !ok {
		return
	}
	ret = first.(addrMaybeId)
	return
}

func (a *traversal) popNextContact(tx *stm.Tx) (ret addrMaybeId, ok bool) {
	ret, ok = a.nextContact(tx)
	if !ok {
		return
	}
	addrString := ret.Addr.String()
	tx.Set(a.nodesPendingContact, tx.Get(a.nodesPendingContact).(stmutil.Settish).Delete(ret))
	tx.Set(a.addrBestIds, tx.Get(a.addrBestIds).(stmutil.Mappish).Delete(addrString))
	tx.Set(a.triedAddrs, tx.Get(a.triedAddrs).(stmutil.Settish).Add(addrString))
	return
}
