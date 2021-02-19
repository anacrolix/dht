package peer_store

import (
	"io"
	"sync"

	debug_writer "github.com/anacrolix/confluence/debug-writer"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/davecgh/go-spew/spew"
)

type InMemory struct {
	mu    sync.RWMutex
	index map[InfoHash]map[string]struct{}
}

var _ interface {
	debug_writer.Interface
} = (*InMemory)(nil)

func (me *InMemory) GetPeers(ih InfoHash) (ret []krpc.NodeAddr) {
	me.mu.RLock()
	defer me.mu.RUnlock()
	for b := range me.index[ih] {
		var r krpc.NodeAddr
		err := r.UnmarshalBinary([]byte(b))
		if err != nil {
			panic(err)
		}
		ret = append(ret, r)
	}
	return
}

func (me *InMemory) AddPeer(ih InfoHash, na krpc.NodeAddr) {
	b, err := na.MarshalBinary()
	if err != nil {
		panic(err)
	}
	s := string(b)
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.index == nil {
		me.index = make(map[InfoHash]map[string]struct{})
	}
	nodes := me.index[ih]
	if nodes == nil {
		nodes = make(map[string]struct{})
		me.index[ih] = nodes
	}
	nodes[s] = struct{}{}
}

func (me *InMemory) GetAll() (ret map[InfoHash][]krpc.NodeAddr) {
	me.mu.RLock()
	defer me.mu.RUnlock()
	ret = make(map[InfoHash][]krpc.NodeAddr, len(me.index))
	for ih, nodes := range me.index {
		for b := range nodes {
			var r krpc.NodeAddr
			r.UnmarshalBinary([]byte(b))
			ret[ih] = append(ret[ih], r)
		}
	}
	return
}

func (me *InMemory) WriteDebug(w io.Writer) {
	spew.Fdump(w, me.GetAll())
}
