package peer_store

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

type InMemory struct {
	// This is used for sorting infohashes by distance in WriteDebug.
	RootId int160.T
	mu     sync.RWMutex
	index  map[InfoHash]indexValue
}

// Keys are raw IP bytes used only for uniqueness. NodeAndTime values are
// authoritative for endpoint and timestamp data.
type indexValue = map[string]NodeAndTime

var _ interface {
	Interface
	WriteDebug(w io.Writer)
} = (*InMemory)(nil)

func (me *InMemory) GetPeers(ih InfoHash) (ret []krpc.NodeAddr) {
	me.mu.RLock()
	defer me.mu.RUnlock()
	nodes := me.index[ih]
	if len(nodes) == 0 {
		return
	}
	ret = make([]krpc.NodeAddr, 0, len(nodes))
	for _, v := range nodes {
		ret = append(ret, v.NodeAddr)
	}
	return
}

func (me *InMemory) AddPeer(ih InfoHash, na krpc.NodeAddr) {
	key := string(na.IP)
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.index == nil {
		me.index = make(map[InfoHash]indexValue)
	}
	nodes := me.index[ih]
	if nodes == nil {
		nodes = make(indexValue)
		me.index[ih] = nodes
	}
	nodes[key] = NodeAndTime{na, time.Now()}
}

type NodeAndTime struct {
	krpc.NodeAddr
	time.Time
}

func (me *InMemory) GetAll() (ret map[InfoHash][]NodeAndTime) {
	me.mu.RLock()
	defer me.mu.RUnlock()
	ret = make(map[InfoHash][]NodeAndTime, len(me.index))
	for ih, nodes := range me.index {
		ret[ih] = slices.Collect(maps.Values(nodes))
	}
	return
}

func (me *InMemory) WriteDebug(w io.Writer) {
	all := me.GetAll()
	var totalCount int
	for _, addrs := range all {
		totalCount += len(addrs)
	}
	fmt.Fprintf(w, "total count: %v\n\n", totalCount)
	infoHashes := slices.SortedFunc(maps.Keys(all), func(l, r InfoHash) int {
		return int160.Distance(int160.FromByteArray(l), me.RootId).Cmp(
			int160.Distance(int160.FromByteArray(r), me.RootId))
	})
	for _, ih := range infoHashes {
		addrs := all[ih]
		fmt.Fprintf(w, "%v (count %v):\n", ih, len(addrs))
		// By IP, then most recent first, then port.
		slices.SortFunc(addrs, func(l, r NodeAndTime) int {
			return cmp.Or(
				bytes.Compare(l.IP, r.IP),
				r.Time.Compare(l.Time),
				cmp.Compare(l.Port, r.Port),
			)
		})
		for _, na := range addrs {
			fmt.Fprintf(w, "\t%v (age: %v)\n", na.NodeAddr, time.Since(na.Time))
		}
	}
	fmt.Fprintln(w)
}
