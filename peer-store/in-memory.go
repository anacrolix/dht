package peer_store

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	debug_writer "github.com/anacrolix/confluence/debug-writer"
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/multiless"
)

type InMemory struct {
	// This is used for sorting infohashes by distance in WriteDebug.
	RootId int160.T
	mu     sync.RWMutex
	index  map[InfoHash]indexValue
}

// Binary encoded krpc.NodeAddr to time of last add.
type indexValue = map[string]time.Time

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
		me.index = make(map[InfoHash]indexValue)
	}
	nodes := me.index[ih]
	if nodes == nil {
		nodes = make(indexValue)
		me.index[ih] = nodes
	}
	nodes[s] = time.Now()
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
		for b, t := range nodes {
			var r krpc.NodeAddr
			r.UnmarshalBinary([]byte(b))
			ret[ih] = append(ret[ih], NodeAndTime{r, t})
		}
	}
	return
}

func (me *InMemory) WriteDebug(w io.Writer) {
	all := me.GetAll()
	var totalCount int
	type sliceElem struct {
		InfoHash
		addrs []NodeAndTime
	}
	var allSlice []sliceElem
	for ih, addrs := range all {
		totalCount += len(addrs)
		allSlice = append(allSlice, sliceElem{ih, addrs})
	}
	fmt.Fprintf(w, "total count: %v\n\n", totalCount)
	sort.Slice(allSlice, func(i, j int) bool {
		return int160.Distance(int160.FromByteArray(allSlice[i].InfoHash), me.RootId).Cmp(
			int160.Distance(int160.FromByteArray(allSlice[j].InfoHash), me.RootId)) < 0
	})
	for _, elem := range allSlice {
		addrs := elem.addrs
		fmt.Fprintf(w, "%v (count %v):\n", elem.InfoHash, len(addrs))
		sort.Slice(addrs, func(i, j int) bool {
			return multiless.New().Cmp(
				bytes.Compare(addrs[i].IP, addrs[j].IP)).Int(
				addrs[i].Port, addrs[j].Port,
			).MustLess()
		})
		for _, na := range addrs {
			fmt.Fprintf(w, "\t%v (age: %v)\n", na.NodeAddr, time.Since(na.Time))
		}
	}
	fmt.Fprintln(w)
}
