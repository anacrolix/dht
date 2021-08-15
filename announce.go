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
type Announce struct {
	Peers chan PeersValues

	values chan PeersValues // Responses are pushed to this channel.

	server   *Server
	infoHash int160.T // Target
	// The torrent port that we're announcing.
	announcePort int
	// The torrent port should be determined by the receiver in case we're
	// being NATed.
	announcePortImplied bool
	scrape              bool

	traversal *traversal.Operation
}

func (a *Announce) String() string {
	return fmt.Sprintf("%[1]T %[1]p of %v on %v", a, a.infoHash, a.server)
}

// Returns the number of distinct remote addresses the announce has queried.
func (a *Announce) NumContacted() int64 {
	return atomic.LoadInt64(&a.traversal.Stats().NumAddrsTried)
}

type AnnounceOpt *struct{}

var scrape = AnnounceOpt(&struct{}{})

func Scrape() AnnounceOpt { return scrape }

// Traverses the DHT graph toward nodes that store peers for the infohash, streaming them to the
// caller, and announcing the local node to each responding node if port is non-zero or impliedPort
// is true.
func (s *Server) Announce(infoHash [20]byte, port int, impliedPort bool, opts ...AnnounceOpt) (_ *Announce, err error) {
	infoHashInt160 := int160.FromByteArray(infoHash)
	a := &Announce{
		Peers:               make(chan PeersValues),
		values:              make(chan PeersValues),
		server:              s,
		infoHash:            infoHashInt160,
		announcePort:        port,
		announcePortImplied: impliedPort,
	}
	for _, opt := range opts {
		if opt == scrape {
			a.scrape = true
		}
	}
	a.traversal = traversal.Start(traversal.OperationInput{
		Target:     infoHash,
		DoQuery:    a.getPeers,
		NodeFilter: s.TraversalNodeFilter,
	})
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
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
		a.announceClosest()
		close(a.values)
	}()
	return a, nil
}

func (a *Announce) announceClosest() {
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

func (a *Announce) announcePeer(peer dhtutil.Elem) error {
	return a.server.announcePeer(
		NewAddr(peer.Addr.UDP()), a.infoHash, a.announcePort, peer.Data.(string),
		a.announcePortImplied, QueryRateLimiting{}).Err
}

func (a *Announce) getPeers(ctx context.Context, addr krpc.NodeAddr) (tqr traversal.QueryResult) {
	res := a.server.GetPeers(ctx, NewAddr(addr.UDP()), a.infoHash, a.scrape, QueryRateLimiting{})
	if r := res.Reply.R; r != nil {
		peersValues := PeersValues{
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
type PeersValues struct {
	Peers         []Peer // Peers given in get_peers response.
	krpc.NodeInfo        // The node that gave the response.
	krpc.Return
}

// Stop the announce.
func (a *Announce) Close() {
	a.traversal.Stop()
}

func (a *Announce) logger() log.Logger {
	return a.server.logger()
}
