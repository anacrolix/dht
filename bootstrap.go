package dht

import (
	"context"
	"errors"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
)

type TraversalStats = traversal.Stats

// See BootstrapContext.
func (s *Server) Bootstrap() (TraversalStats, error) {
	return s.BootstrapContext(context.Background())
}

// Populates the node table.
func (s *Server) BootstrapContext(ctx context.Context) (_ TraversalStats, err error) {
	s.mu.Lock()
	if s.bootstrappingNow {
		s.mu.Unlock()
		err = errors.New("already bootstrapping")
		return
	}
	s.bootstrappingNow = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.bootstrappingNow = false
	}()
	nodes, err := s.TraversalStartingNodes()
	if err != nil {
		return
	}
	t := traversal.Start(traversal.OperationInput{
		Target: s.id.AsByteArray(),
		K:      16,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			return s.findNode(ctx, NewAddr(addr.UDP()), s.id, QueryRateLimiting{}).TraversalQueryResult(addr)
		},
		NodeFilter: s.TraversalNodeFilter,
	})
	t.AddNodes(nodes)
	s.mu.Lock()
	s.lastBootstrap = time.Now()
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-t.Stalled():
	}
	// Stopping cancels outstanding queries, so this doesn't wait for them to time out.
	t.Stop()
	<-t.Stopped()
	return t.LoadStats(), err
}
