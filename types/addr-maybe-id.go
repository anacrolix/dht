package types

import (
	"fmt"
	"hash/fnv"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

type AddrMaybeId struct {
	Addr krpc.NodeAddr
	Id   *int160.T
}

func (me AddrMaybeId) String() string {
	if me.Id == nil {
		return fmt.Sprintf("unknown id at %s", me.Addr)
	} else {
		return fmt.Sprintf("%v at %v", *me.Id, me.Addr)
	}
}

func (l AddrMaybeId) CloserThan(r AddrMaybeId, target int160.T) bool {
	var ml missinggo.MultiLess
	ml.NextBool(r.Id == nil, l.Id == nil)
	if l.Id != nil && r.Id != nil {
		d := int160.Distance(*l.Id, target).Cmp(int160.Distance(*r.Id, target))
		ml.StrictNext(d == 0, d < 0)
	}
	// TODO: Use bytes/hash when it's available (go1.14?), and have a unique seed for each
	// instance.
	hashString := func(s string) uint64 {
		h := fnv.New64a()
		h.Write([]byte(s))
		return h.Sum64()
	}
	lh := hashString(l.Addr.String())
	rh := hashString(r.Addr.String())
	ml.StrictNext(lh == rh, lh < rh)
	//ml.StrictNext(l.Addr.String() == r.Addr.String(), l.Addr.String() < r.Addr.String())
	return ml.Less()

}
