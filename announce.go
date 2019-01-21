package dht

// get_peers and announce_peers.

import (
	"container/heap"
	"context"
	"net"
	"time"

	"github.com/anacrolix/dht/krpc"
	"github.com/anacrolix/sync"
	"github.com/willf/bloom"
)

// Maintains state for an ongoing Announce operation. An Announce is started
// by calling Server.Announce.
type Announce struct {
	mu    sync.Mutex
	Peers chan PeersValues
	// Inner chan is set to nil when on close.
	values     chan PeersValues
	ctx        context.Context
	cancel     func()
	stop       <-chan struct{}
	triedAddrs *bloom.BloomFilter
	// How many transactions are still ongoing.
	pending  int
	server   *Server
	infoHash int160
	// Count of (probably) distinct addresses we've sent get_peers requests
	// to.
	numContacted int
	// The torrent port that we're announcing.
	announcePort int
	// The torrent port should be determined by the receiver in case we're
	// being NATed.
	announcePortImplied bool

	nodesPendingContact nodesByDistance
	nodeContactorCond   sync.Cond
	contactRateLimiter  chan struct{}
}

// Returns the number of distinct remote addresses the announce has queried.
func (a *Announce) NumContacted() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.numContacted
}

func newBloomFilterForTraversal() *bloom.BloomFilter {
	return bloom.NewWithEstimates(10000, 0.5)
}

// This is kind of the main thing you want to do with DHT. It traverses the
// graph toward nodes that store peers for the infohash, streaming them to the
// caller, and announcing the local node to each node if allowed and
// specified.
func (s *Server) Announce(infoHash [20]byte, port int, impliedPort bool) (*Announce, error) {
	startAddrs, err := s.traversalStartingNodes()
	if err != nil {
		return nil, err
	}
	a := &Announce{
		Peers:               make(chan PeersValues, 100),
		values:              make(chan PeersValues),
		triedAddrs:          newBloomFilterForTraversal(),
		server:              s,
		infoHash:            int160FromByteArray(infoHash),
		announcePort:        port,
		announcePortImplied: impliedPort,
		contactRateLimiter:  make(chan struct{}, 10),
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.stop = a.ctx.Done()
	a.nodesPendingContact.target = int160FromByteArray(infoHash)
	a.nodeContactorCond.L = &a.mu
	go a.rateUnlimiter()
	// Function ferries from values to Values until discovery is halted.
	go func() {
		defer close(a.Peers)
		for {
			select {
			case psv := <-a.values:
				select {
				case a.Peers <- psv:
				case <-a.stop:
					return
				}
			case <-a.stop:
				return
			}
		}
	}()
	for _, n := range startAddrs {
		a.pendContact(n)
	}
	a.maybeClose()
	go a.nodeContactor()
	return a, nil
}

func (a *Announce) rateUnlimiter() {
	for {
		select {
		case a.contactRateLimiter <- struct{}{}:
		case <-a.ctx.Done():
			return
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-a.ctx.Done():
			return
		}
	}
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

func (a *Announce) shouldContact(addr krpc.NodeAddr) bool {
	if !validNodeAddr(addr.UDP()) {
		return false
	}
	if a.triedAddrs.TestString(addr.String()) {
		return false
	}
	if a.server.ipBlocked(addr.IP) {
		return false
	}
	return true
}

func (a *Announce) completeContact() {
	a.pending--
	a.maybeClose()
}

func (a *Announce) contact(node addrMaybeId) bool {
	if !a.shouldContact(node.Addr) {
		// log.Printf("shouldn't contact: %v", node)
		return false
	}
	a.numContacted++
	a.pending++
	a.triedAddrs.AddString(node.Addr.String())
	go a.getPeers(node)
	return true
}

func (a *Announce) maybeClose() {
	if a.nodesPendingContact.Len() == 0 && a.pending == 0 {
		a.close()
	}
}

func (a *Announce) responseNode(node krpc.NodeInfo) {
	i := int160FromByteArray(node.ID)
	a.pendContact(addrMaybeId{node.Addr, &i})
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

func (a *Announce) getPeers(node addrMaybeId) {
	addr := NewAddr(node.Addr.UDP())
	// log.Printf("sending get_peers to %v", node)
	m, err := a.server.getPeers(context.TODO(), addr, a.infoHash)
	// log.Print(err)
	// log.Printf("get_peers response error from %v: %v", node, err)
	if err == nil {
		select {
		case a.contactRateLimiter <- struct{}{}:
		default:
		}
	}
	// Register suggested nodes closer to the target info-hash.
	if m.R != nil && m.SenderID() != nil {
		expvars.Add("announce get_peers response nodes values", int64(len(m.R.Nodes)))
		expvars.Add("announce get_peers response nodes6 values", int64(len(m.R.Nodes6)))
		a.mu.Lock()
		m.R.ForAllNodes(a.responseNode)
		a.mu.Unlock()
		select {
		case a.values <- PeersValues{
			Peers: m.R.Values,
			NodeInfo: krpc.NodeInfo{
				Addr: addr.KRPC(),
				ID:   *m.SenderID(),
			},
		}:
		case <-a.stop:
		}
		a.maybeAnnouncePeer(addr, m.R.Token, m.SenderID())
	}
	a.mu.Lock()
	a.completeContact()
	a.mu.Unlock()
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
	a.mu.Lock()
	defer a.mu.Unlock()
	a.close()
}

func (a *Announce) close() {
	select {
	case <-a.stop:
	default:
		a.cancel()
		a.nodeContactorCond.Broadcast()
	}
}

func (a *Announce) pendContact(node addrMaybeId) {
	if !a.shouldContact(node.Addr) {
		// log.Printf("shouldn't contact (pend): %v", node)
		return
	}
	heap.Push(&a.nodesPendingContact, node)
	a.nodeContactorCond.Signal()
}

func (a *Announce) waitContactRateToken() bool {
	select {
	case <-a.ctx.Done():
		return false
	case <-a.contactRateLimiter:
		return true
	}
}

func (a *Announce) contactPendingNode() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for {
		if a.ctx.Err() != nil {
			return false
		}
		for a.nodesPendingContact.Len() > 0 {
			if a.contact(heap.Pop(&a.nodesPendingContact).(addrMaybeId)) {
				return true
			}
		}
		a.nodeContactorCond.Wait()
	}
}

func (a *Announce) nodeContactor() {
	for {
		if !a.waitContactRateToken() {
			return
		}
		if !a.contactPendingNode() {
			return
		}
	}
}
