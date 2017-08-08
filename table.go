package dht

type table struct {
	rootID  int160
	k       int
	buckets [160]bucket
	addrs   map[Addr]*node
}

func (tbl *table) numNodes() (num int) {
	for _, b := range tbl.buckets {
		num += b.Len()
	}
	return
}

func (tbl *table) bucketIndex(id int160) int {
	if id == tbl.rootID {
		panic("nobody puts the root ID in a bucket")
	}
	var a int160
	a.Xor(&tbl.rootID, &id)
	index := 160 - a.BitLen()
	return index
}

func (tbl *table) forNodes(f func(*node) bool) bool {
	for _, b := range tbl.buckets {
		if !b.EachNode(f) {
			return false
		}
	}
	return true
}

func (tbl *table) getNode(addr Addr, id int160) *node {
	if id == tbl.rootID {
		return nil
	}
	return tbl.buckets[tbl.bucketIndex(id)].GetNode(addr, id)
}

func (tbl *table) closestNodes(k int, target int160, filter func(*node) bool) (ret []*node) {
	for bi := func() int {
		if target == tbl.rootID {
			return len(tbl.buckets) - 1
		} else {
			return tbl.bucketIndex(target)
		}
	}(); bi >= 0 && len(ret) < k; bi-- {
		for n := range tbl.buckets[bi].nodes {
			ret = append(ret, n)
		}
	}
	// TODO: Keep only the closest.
	if len(ret) > k {
		ret = ret[:k]
	}
	return
}

func (tbl *table) addNode(n *node) {
	if n.id == tbl.rootID {
		return
	}
	b := &tbl.buckets[tbl.bucketIndex(n.id)]
	if _, ok := b.nodes[n]; ok {
		return
	}
	if b.Len() < tbl.k {
		b.AddNode(n, tbl.k)
	}
}
