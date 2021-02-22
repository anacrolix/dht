package dht

import (
	"fmt"
	"hash/fnv"
	"net"

	"github.com/anacrolix/missinggo/v2"
	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/anacrolix/stm/stmutil"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

func mustListen(addr string) net.PacketConn {
	ret, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	return ret
}

func addrResolver(addr string) func() ([]Addr, error) {
	return func() ([]Addr, error) {
		ua, err := net.ResolveUDPAddr("udp", addr)
		return []Addr{NewAddr(ua)}, err
	}
}

type addrMaybeId struct {
	Addr krpc.NodeAddr
	Id   *int160.T
}

func (me addrMaybeId) String() string {
	if me.Id == nil {
		return fmt.Sprintf("unknown id at %s", me.Addr)
	} else {
		return fmt.Sprintf("%v at %v", *me.Id, me.Addr)
	}
}

func (l addrMaybeId) closerThan(r addrMaybeId, target int160.T) bool {
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

func nodesByDistance(target int160.T) stmutil.Settish {
	return stmutil.NewSortedSet(func(l, r interface{}) bool {
		return l.(addrMaybeId).closerThan(r.(addrMaybeId), target)
	})
}

func randomIdInBucket(rootId int160.T, bucketIndex int) int160.T {
	id := int160.FromByteArray(RandomNodeID())
	for i := range iter.N(bucketIndex) {
		id.SetBit(i, rootId.GetBit(i))
	}
	id.SetBit(bucketIndex, !rootId.GetBit(bucketIndex))
	return id
}
