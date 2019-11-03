package dht

// get_peers and announce_peers.

import (
	"context"
	"net"

	"github.com/anacrolix/missinggo/v2/conntrack"
	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/lukechampine/stm"
	"github.com/lukechampine/stm/stmutil"
	"github.com/willf/bloom"

	"github.com/anacrolix/dht/v2/krpc"
)

// Maintains state for an ongoing Announce operation. An Announce is started by calling
// Server.Announce.
type Announce struct {
	Peers chan PeersValues

	values chan PeersValues // Responses are pushed to this channel.

	// These only exist to support routines relying on channels for synchronization.
	done    <-chan struct{}
	doneVar *stm.Var
	cancel  func()

	triedAddrs *stm.Var // Settish of krpc.NodeAddr.String

	pending  *stm.Var // How many transactions are still ongoing (int).
	server   *Server
	infoHash int160 // Target
	// Count of (probably) distinct addresses we've sent get_peers requests to.
	numContacted *stm.Var
	// The torrent port that we're announcing.
	announcePort int
	// The torrent port should be determined by the receiver in case we're
	// being NATed.
	announcePortImplied bool

	nodesPendingContact *stm.Var // Settish of addrMaybeId sorted by distance from the target
}

// Returns the number of distinct remote addresses the announce has queried.
func (a *Announce) NumContacted() int {
	return stm.AtomicGet(a.numContacted).(int)
}

func newBloomFilterForTraversal() *bloom.BloomFilter {
	return bloom.NewWithEstimates(10000, 0.5)
}

// Traverses the DHT graph toward nodes that store peers for the infohash, streaming them to the
// caller, and announcing the local node to each responding node if port is non-zero or impliedPort
// is true.
func (s *Server) Announce(infoHash [20]byte, port int, impliedPort bool) (*Announce, error) {
	startAddrs, err := s.traversalStartingNodes()
	if err != nil {
		return nil, err
	}
	infoHashInt160 := int160FromByteArray(infoHash)
	a := &Announce{
		Peers:               make(chan PeersValues, 100),
		values:              make(chan PeersValues),
		triedAddrs:          stm.NewVar(stmutil.NewSet()),
		server:              s,
		infoHash:            infoHashInt160,
		announcePort:        port,
		announcePortImplied: impliedPort,
		nodesPendingContact: stm.NewVar(nodesByDistance(infoHashInt160)),
		pending:             stm.NewVar(0),
		numContacted:        stm.NewVar(0),
	}
	var ctx context.Context
	ctx, a.cancel = context.WithCancel(context.Background())
	a.done = ctx.Done()
	a.doneVar, _ = stmutil.ContextDoneVar(ctx)
	// Function ferries from values to Peers until discovery is halted.
	go func() {
		defer close(a.Peers)
		for {
			select {
			case psv := <-a.values:
				select {
				case a.Peers <- psv:
				case <-a.done:
					return
				}
			case <-a.done:
				return
			}
		}
	}()
	for _, n := range startAddrs {
		stm.Atomically(func(tx *stm.Tx) {
			a.pendContact(n, tx)
		})
	}
	go a.closer()
	go a.nodeContactor()
	return a, nil
}

func (a *Announce) closer() {
	defer a.cancel()
	stm.Atomically(func(tx *stm.Tx) {
		if tx.Get(a.doneVar).(bool) {
			return
		}
		tx.Assert(tx.Get(a.pending).(int) == 0)
		tx.Assert(tx.Get(a.nodesPendingContact).(stmutil.Lenner).Len() == 0)
	})
}

func validNodeAddr(addr net.Addr) bool {
	// At least for UDP addresses, we know what doesn't work.
	ua := addr.(*net.UDPAddr)
	if ua.Port == 0 {
		return false
	}
	if ip4 := ua.IP.To4(); ip4 != nil && ip4[0] == 0 {
		// Why?
		return false
	}
	return true
}

func (a *Announce) shouldContact(addr krpc.NodeAddr, tx *stm.Tx) bool {
	if !validNodeAddr(addr.UDP()) {
		return false
	}
	if tx.Get(a.triedAddrs).(stmutil.Settish).Contains(addr.String()) {
		return false
	}
	if a.server.ipBlocked(addr.IP) {
		return false
	}
	return true
}

func (a *Announce) completeContact() {
	stm.Atomically(func(tx *stm.Tx) {
		tx.Set(a.pending, tx.Get(a.pending).(int)-1)
	})
}

func (a *Announce) responseNode(node krpc.NodeInfo) {
	i := int160FromByteArray(node.ID)
	stm.Atomically(func(tx *stm.Tx) {
		a.pendContact(addrMaybeId{node.Addr, &i}, tx)
	})
}

// Announce to a peer, if appropriate.
func (a *Announce) maybeAnnouncePeer(to Addr, token *string, peerId *krpc.ID) {
	if token == nil {
		return
	}
	if !a.server.config.NoSecurity && (peerId == nil || !NodeIdSecure(*peerId, to.IP())) {
		return
	}
	a.server.mu.Lock()
	defer a.server.mu.Unlock()
	a.server.announcePeer(to, a.infoHash, a.announcePort, *token, a.announcePortImplied, nil)
}

func (a *Announce) getPeers(addr Addr, cteh *conntrack.EntryHandle) {
	// log.Printf("sending get_peers to %v", node)
	m, writes, _ := a.server.getPeers(context.TODO(), addr, a.infoHash)
	if writes == 0 {
		cteh.Forget()
	} else {
		cteh.Done()
	}
	// log.Print(err)
	// log.Printf("get_peers response error from %v: %v", node, err)
	// Register suggested nodes closer to the target info-hash.
	if m.R != nil && m.SenderID() != nil {
		expvars.Add("announce get_peers response nodes values", int64(len(m.R.Nodes)))
		expvars.Add("announce get_peers response nodes6 values", int64(len(m.R.Nodes6)))
		m.R.ForAllNodes(a.responseNode)
		select {
		case a.values <- PeersValues{
			Peers: m.R.Values,
			NodeInfo: krpc.NodeInfo{
				Addr: addr.KRPC(),
				ID:   *m.SenderID(),
			},
		}:
		case <-a.done:
		}
		a.maybeAnnouncePeer(addr, m.R.Token, m.SenderID())
	}
	a.completeContact()
}

// Corresponds to the "values" key in a get_peers KRPC response. A list of
// peers that a node has reported as being in the swarm for a queried info
// hash.
type PeersValues struct {
	Peers         []Peer // Peers given in get_peers response.
	krpc.NodeInfo        // The node that gave the response.
}

// Stop the announce.
func (a *Announce) Close() {
	a.close()
}

func (a *Announce) close() {
	a.cancel()
}

func (a *Announce) pendContact(node addrMaybeId, tx *stm.Tx) {
	if !a.shouldContact(node.Addr, tx) {
		// log.Printf("shouldn't contact (pend): %v", node)
		return
	}
	tx.Set(a.nodesPendingContact, tx.Get(a.nodesPendingContact).(stmutil.Settish).Add(node))
}

func (a *Announce) nodeContactor() {
	for {
		type txResT struct {
			done    bool
			contact bool
			addr    Addr
			cteh    *conntrack.EntryHandle
		}
		txRes := stm.Atomically(func(tx *stm.Tx) {
			if tx.Get(a.doneVar).(bool) {
				tx.Return(txResT{done: true})
			}
			npc := tx.Get(a.nodesPendingContact).(stmutil.Settish)
			first, ok := iter.First(npc.Iter)
			tx.Assert(ok)
			addr := first.(addrMaybeId).Addr
			tx.Set(a.nodesPendingContact, npc.Delete(first))
			if !a.shouldContact(addr, tx) {
				tx.Return(txResT{})
			}
			cteh := a.server.config.ConnectionTracking.Allow(tx, a.server.connTrackEntryForAddr(NewAddr(addr.UDP())), "announce get_peers", -1)
			tx.Assert(cteh != nil)
			tx.Assert(a.server.sendLimit.AllowStm(tx))
			tx.Set(a.numContacted, tx.Get(a.numContacted).(int)+1)
			tx.Set(a.pending, tx.Get(a.pending).(int)+1)
			tx.Set(a.triedAddrs, tx.Get(a.triedAddrs).(stmutil.Settish).Add(addr.String()))
			tx.Return(txResT{addr: NewAddr(addr.UDP()), cteh: cteh, contact: true})
		}).(txResT)
		if txRes.done {
			break
		}
		if txRes.contact {
			go a.getPeers(txRes.addr, txRes.cteh)
		}
	}
}
