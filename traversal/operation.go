package traversal

import (
	"context"
	"sort"
	"sync/atomic"

	"github.com/anacrolix/multiless"
	"github.com/anacrolix/sync"

	"github.com/anacrolix/chansync"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/types"
)

type QueryResult struct {
	ResponseFrom *krpc.NodeInfo
	Nodes        []krpc.NodeInfo
}

type OperationInput struct {
	Target     [20]byte
	Alpha      int
	K          int
	DoQuery    func(context.Context, krpc.NodeAddr) QueryResult
	NodeFilter func(krpc.NodeInfo) bool
}

type defaultsAppliedOperationInput OperationInput

func Start(input OperationInput) *Operation {
	herp := defaultsAppliedOperationInput(input)
	if herp.Alpha == 0 {
		herp.Alpha = 3
	}
	if herp.NodeFilter == nil {
		herp.NodeFilter = func(krpc.NodeInfo) bool {
			return true
		}
	}
	op := &Operation{
		targetInt160: int160.FromByteArray(input.Target),
		input:        herp,
		queried:      make(map[string]struct{}),
	}
	go op.run()
	return op
}

type Operation struct {
	stats        Stats
	mu           sync.Mutex
	unqueried    []types.AddrMaybeId
	queried      map[string]struct{}
	closest      []krpc.NodeInfo
	targetInt160 int160.T
	input        defaultsAppliedOperationInput
	outstanding  int
	cond         chansync.BroadcastCond
	stalled      chansync.LevelTrigger
	stopping     chansync.SetOnce
	stopped      chansync.SetOnce
}

func (op *Operation) Stats() *Stats {
	return &op.stats
}

func (op *Operation) Stop() {
	if op.stopping.Set() {
		go func() {
			defer op.stopped.Set()
			op.mu.Lock()
			defer op.mu.Unlock()
			for {
				if op.outstanding == 0 {
					break
				}
				cond := op.cond.Signaled()
				op.mu.Unlock()
				<-cond
				op.mu.Lock()
			}
		}()
	}
}

func (op *Operation) Stopped() chansync.Done {
	return op.stopped.Done()
}

func (op *Operation) Stalled() chansync.Active {
	return op.stalled.Active()
}

func (op *Operation) AddNodes(nodes []types.AddrMaybeId) {
	op.mu.Lock()
	defer op.mu.Unlock()
	for _, n := range nodes {
		if _, ok := op.queried[n.Addr.String()]; ok {
			continue
		}
		if ni := n.TryIntoNodeInfo(); ni != nil && !op.input.NodeFilter(*ni) {
			continue
		}
		op.unqueried = append(op.unqueried, n)
	}
	op.cond.Broadcast()
}

func (op *Operation) markQueried(addr krpc.NodeAddr) {
	op.queried[addr.String()] = struct{}{}
}

func (op *Operation) closestUnqueriedIndex() int {
	closest := 0
	for i, a := range op.unqueried {
		if a.CloserThan(op.unqueried[closest], op.targetInt160) {
			closest = i
		}
	}
	return closest

}

func (op *Operation) closestUnqueried() (ret types.AddrMaybeId) {
	//defer func() {
	//	spew.Dump("closest unqueried", ret)
	//}()
	return op.unqueried[op.closestUnqueriedIndex()]
}

func (op *Operation) popClosestUnqueried() types.AddrMaybeId {
	i := op.closestUnqueriedIndex()
	ret := op.unqueried[i]
	op.unqueried = append(op.unqueried[:i], op.unqueried[i+1:]...)
	return ret
}

func (op *Operation) farthestClosest() (ret types.AddrMaybeId) {
	ret.FromNodeInfo(op.closest[len(op.closest)-1])
	return
}

func (op *Operation) haveQuery() bool {
	if len(op.unqueried) == 0 {
		return false
	}
	if len(op.closest) < op.input.K {
		return true
	}
	closestUnqueried := op.closestUnqueried()
	if closestUnqueried.Id == nil {
		return true
	}
	return closestUnqueried.CloserThan(op.farthestClosest(), op.targetInt160)
}

func (op *Operation) run() {
	op.mu.Lock()
	defer op.mu.Unlock()
	for {
		if op.stopping.IsSet() {
			return
		}
		for op.outstanding < op.input.Alpha && op.haveQuery() {
			op.startQuery()
		}
		var stalled chansync.Signal
		if (!op.haveQuery() || op.input.Alpha == 0) && op.outstanding == 0 {
			stalled = op.stalled.Signal()
		}
		queryCondSignaled := op.cond.Signaled()
		op.mu.Unlock()
		select {
		case stalled <- struct{}{}:
		case <-op.stopping.Done():
		case <-queryCondSignaled:
		}
		op.mu.Lock()
	}
}

func (op *Operation) sortNodesByClosest(nodes []krpc.NodeInfo) {
	sort.Slice(nodes, func(i, j int) bool {
		return multiless.New().Cmp(
			int160.Distance(int160.FromByteArray(nodes[i].ID), op.targetInt160).Cmp(
				int160.Distance(int160.FromByteArray(nodes[j].ID), op.targetInt160)),
		).Less()
	})
}

func (op *Operation) addClosest(node krpc.NodeInfo) {
	if !op.input.NodeFilter(node) {
		return
	}
	op.closest = append(op.closest, node)
	op.sortNodesByClosest(op.closest)
	if len(op.closest) > op.input.K {
		op.closest = op.closest[:op.input.K]
	}
}

func (op *Operation) startQuery() {
	a := op.popClosestUnqueried()
	op.markQueried(a.Addr)
	op.outstanding++
	go func() {
		defer func() {
			op.mu.Lock()
			defer op.mu.Unlock()
			op.outstanding--
			op.cond.Broadcast()
		}()
		//log.Printf("traversal querying %v", a)
		atomic.AddInt64(&op.stats.NumAddrsTried, 1)
		res := op.input.DoQuery(context.TODO(), a.Addr)
		if res.ResponseFrom != nil {
			func() {
				op.mu.Lock()
				defer op.mu.Unlock()
				atomic.AddInt64(&op.stats.NumResponses, 1)
				op.addClosest(*res.ResponseFrom)
			}()
		}
		op.AddNodes(func() (ret []types.AddrMaybeId) {
			for _, ni := range res.Nodes {
				id := int160.FromByteArray(ni.ID)
				ret = append(ret, types.AddrMaybeId{
					Addr: ni.Addr,
					Id:   &id,
				})
			}
			return
		}())
	}()
}
