package dht

// get_peers and announce_peers.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/log"

	"github.com/anacrolix/dht/v2/int160"
	dhtutil "github.com/anacrolix/dht/v2/k-nearest-nodes"
	"github.com/anacrolix/dht/v2/krpc"
)

// Maintains state for an ongoing Announce operation. An Announce is started by calling
// Server.Announce.
type Announce[T krpc.CompactNodeInfo] struct {
	Peers chan PeersValues[T]

	values chan PeersValues[T] // Responses are pushed to this channel.

	server   *Server[T]
	infoHash int160.T // Target

	announcePeerOpts *AnnouncePeerOpts[T]
	scrape           bool

	traversal *traversal.Operation
}

func (a *Announce[T]) String() string {
	return fmt.Sprintf("%[1]T %[1]p of %v on %v", a, a.infoHash, a.server)
}

// Returns the number of distinct remote addresses the announce has queried.
func (a *Announce[T]) NumContacted() uint32 {
	return atomic.LoadUint32(&a.traversal.Stats().NumAddrsTried)
}

// Server.Announce option
type AnnounceOpt[T krpc.CompactNodeInfo] func(a *Announce[T])

// Scrape BEP 33 bloom filters in queries.
func Scrape[T krpc.CompactNodeInfo]() AnnounceOpt[T] {
	return func(a *Announce[T]) {
		a.scrape = true
	}
}

// Arguments for announce_peer from a Server.Announce.
type AnnouncePeerOpts[T krpc.CompactNodeInfo] struct {
	// The peer port that we're announcing.
	Port int
	// The peer port should be determined by the receiver to be the source port of the query packet.
	ImpliedPort bool
}

// Finish an Announce get_peers traversal with an announce of a local peer.
func AnnouncePeer[T krpc.CompactNodeInfo](opts AnnouncePeerOpts[T]) AnnounceOpt[T] {
	return func(a *Announce[T]) {
		a.announcePeerOpts = &opts
	}
}

// Deprecated: Use Server.AnnounceTraversal.
// Traverses the DHT graph toward nodes that store peers for the infohash, streaming them to the
// caller, and announcing the local node to each responding node if port is non-zero or impliedPort
// is true.
func (s *Server[T]) Announce(infoHash [20]byte, port int, impliedPort bool, opts ...AnnounceOpt[T]) (_ *Announce[T], err error) {
	if port != 0 || impliedPort {
		ap := AnnouncePeer(AnnouncePeerOpts[T]{
			Port:        port,
			ImpliedPort: impliedPort,
		})
		opts = append([]AnnounceOpt[T]{ap}, opts...)
	}
	return s.AnnounceTraversal(infoHash, opts...)
}

// Traverses the DHT graph toward nodes that store peers for the infohash, streaming them to the
// caller.
func (s *Server[T]) AnnounceTraversal(infoHash [20]byte, opts ...AnnounceOpt[T]) (_ *Announce[T], err error) {
	infoHashInt160 := int160.FromByteArray(infoHash)
	a := &Announce[T]{
		Peers:    make(chan PeersValues[T]),
		values:   make(chan PeersValues[T]),
		server:   s,
		infoHash: infoHashInt160,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.traversal = traversal.Start(traversal.OperationInput{
		Target:     infoHash,
		DoQuery:    a.getPeers,
		NodeFilter: s.TraversalNodeFilter,
	})
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
		a.traversal.Stop()
		return
	}
	a.traversal.AddNodes(nodes)
	// Function ferries from values to Peers until discovery is halted.
	go func() {
		defer close(a.Peers)
		for {
			select {
			case psv := <-a.values:
				select {
				case a.Peers <- psv:
				case <-a.traversal.Stopped():
					return
				}
			case <-a.traversal.Stopped():
				return
			}
		}
	}()
	go func() {
		<-a.traversal.Stalled()
		a.traversal.Stop()
		<-a.traversal.Stopped()
		if a.announcePeerOpts != nil {
			a.announceClosest()
		}
		close(a.values)
	}()
	return a, nil
}

func (a *Announce[T]) announceClosest() {
	var wg sync.WaitGroup
	a.traversal.Closest().Range(func(elem dhtutil.Elem) {
		wg.Add(1)
		go func() {
			a.logger().WithLevel(log.Debug).Printf(
				"announce_peer to %v: %v",
				elem, a.announcePeer(elem),
			)
			wg.Done()
		}()
	})
	wg.Wait()
}

func (a *Announce[T]) announcePeer(peer dhtutil.Elem) error {
	return a.server.announcePeer(
		NewAddr(peer.Addr.UDP()),
		a.infoHash,
		a.announcePeerOpts.Port,
		peer.Data.(string),
		a.announcePeerOpts.ImpliedPort,
		QueryRateLimiting{},
	).Err
}

func (a *Announce[T]) getPeers(ctx context.Context, addr krpc.NodeAddr) (tqr traversal.QueryResult) {
	res := a.server.GetPeers(ctx, NewAddr(addr.UDP()), a.infoHash, a.scrape, QueryRateLimiting{})
	if r := res.Reply.R; r != nil {
		peersValues := PeersValues[T]{
			Peers: r.Values,
			NodeInfo: krpc.NodeInfo{
				Addr: addr,
				ID:   r.ID,
			},
			Return: *r,
		}
		select {
		case a.values <- peersValues:
		case <-a.traversal.Stopped():
		}
		if r.Token != nil {
			tqr.ClosestData = *r.Token
			tqr.ResponseFrom = &krpc.NodeInfo{
				ID:   r.ID,
				Addr: addr,
			}
		}
		tqr.Nodes = r.Nodes
		tqr.Nodes6 = r.Nodes6
	}
	return
}

// Corresponds to the "values" key in a get_peers KRPC response. A list of
// peers that a node has reported as being in the swarm for a queried info
// hash.
type PeersValues[T krpc.CompactNodeInfo] struct {
	Peers         []Peer // Peers given in get_peers response.
	krpc.NodeInfo        // The node that gave the response.
	krpc.Return[T]
}

// Stop the announce.
func (a *Announce[T]) Close() {
	a.traversal.Stop()
}

func (a *Announce[T]) logger() log.Logger {
	return a.server.logger()
}
