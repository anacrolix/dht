package dht

import (
	"errors"
	"fmt"
	"slices"

	"github.com/anacrolix/dht/v2/int160"
)

// Node table, with indexes on distance from root ID to bucket, and node addr.
type table struct {
	rootID  int160.T
	k       int
	buckets [160]bucket
	addrs   map[string]map[int160.T]struct{}
}

func (tbl *table) K() int {
	return tbl.k
}

func (tbl *table) randomIdForBucket(bucketIndex int) int160.T {
	randomId := randomIdInBucket(tbl.rootID, bucketIndex)
	if randomIdBucketIndex := tbl.bucketIndex(randomId); randomIdBucketIndex != bucketIndex {
		panic(fmt.Sprintf("bucket index for random id %v == %v not %v", randomId, randomIdBucketIndex, bucketIndex))
	}
	return randomId
}

func (tbl *table) dropNode(n *node) {
	as := n.Addr.String()
	if _, ok := tbl.addrs[as][n.Id]; !ok {
		panic("missing id for addr")
	}
	delete(tbl.addrs[as], n.Id)
	if len(tbl.addrs[as]) == 0 {
		delete(tbl.addrs, as)
	}
	b := tbl.bucketForID(n.Id)
	if _, ok := b.nodes[n]; !ok {
		panic("expected node in bucket")
	}
	delete(b.nodes, n)
}

func (tbl *table) bucketForID(id int160.T) *bucket {
	return &tbl.buckets[tbl.bucketIndex(id)]
}

func (tbl *table) numNodes() (num int) {
	for i := range tbl.buckets {
		num += tbl.buckets[i].Len()
	}
	return
}

func (tbl *table) bucketIndex(id int160.T) int {
	if id == tbl.rootID {
		panic("nobody puts the root ID in a bucket")
	}
	var a int160.T
	a.Xor(&tbl.rootID, &id)
	index := 160 - a.BitLen()
	return index
}

func (tbl *table) forNodes(f func(*node) bool) bool {
	for i := range tbl.buckets {
		if !tbl.buckets[i].EachNode(f) {
			return false
		}
	}
	return true
}

func (tbl *table) getNode(addr Addr, id int160.T) *node {
	if id == tbl.rootID {
		return nil
	}
	return tbl.buckets[tbl.bucketIndex(id)].GetNode(addr, id)
}

func (tbl *table) closestNodes(k int, target int160.T, filter func(*node) bool) (ret []*node) {
	if k <= 0 {
		return nil
	}
	// Buckets are relative to rootID, not target. Even a higher-index bucket can contain
	// a closer eligible node, so rank candidates from the entire table.
	tbl.forNodes(func(n *node) bool {
		if filter(n) {
			ret = append(ret, n)
		}
		return true
	})
	slices.SortFunc(ret, func(a, b *node) int {
		if d := a.Id.Distance(target).Cmp(b.Id.Distance(target)); d != 0 {
			return d
		}
		return a.Addr.KRPC().ToNodeAddrPort().Compare(b.Addr.KRPC().ToNodeAddrPort())
	})
	if len(ret) > k {
		ret = ret[:k]
	}
	return
}

func (tbl *table) addNode(n *node) error {
	if n.Id == tbl.rootID {
		return errors.New("is root id")
	}
	b := &tbl.buckets[tbl.bucketIndex(n.Id)]
	if b.GetNode(n.Addr, n.Id) != nil {
		return errors.New("already present")
	}
	if b.Len() >= tbl.k {
		return errors.New("bucket is full")
	}
	b.AddNode(n, tbl.k)
	if tbl.addrs == nil {
		tbl.addrs = make(map[string]map[int160.T]struct{}, 160*tbl.k)
	}
	as := n.Addr.String()
	if tbl.addrs[as] == nil {
		tbl.addrs[as] = make(map[int160.T]struct{}, 1)
	}
	tbl.addrs[as][n.Id] = struct{}{}
	return nil
}
