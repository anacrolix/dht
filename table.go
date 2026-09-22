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
	bi := len(tbl.buckets) - 1
	if target != tbl.rootID {
		bi = tbl.bucketIndex(target)
	}
	// Buckets are ordered by XOR magnitude: every node in an earlier bucket is closer than every
	// node in a later one. Inside a bucket the map order is not distance order, so a partial
	// bucket is sorted and only the nearest remaining slots are kept.
	for ; bi >= 0 && len(ret) < k; bi-- {
		var bucketNodes []*node
		for n := range tbl.buckets[bi].nodes {
			if filter(n) {
				bucketNodes = append(bucketNodes, n)
			}
		}
		slices.SortFunc(bucketNodes, func(a, b *node) int {
			return a.Id.Distance(target).Cmp(b.Id.Distance(target))
		})
		if need := k - len(ret); len(bucketNodes) > need {
			bucketNodes = bucketNodes[:need]
		}
		ret = append(ret, bucketNodes...)
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
