package dht

type table struct {
	rootID  int160
	k       int
	buckets [160][]*node
	addrs   map[Addr]*node
}

func (tbl *table) numNodes() (num int) {
	for _, b := range tbl.buckets {
		num += len(b)
	}
	return
}

func (tbl *table) bucketIndex(id int160) int {
	var a int160
	a.Xor(&tbl.rootID, &id)
	index := 160 - a.BitLen()
	return index
}

func (tbl *table) forNodes(f func(*node) bool) bool {
	for _, b := range tbl.buckets {
		for _, n := range b {
			if !f(n) {
				return false
			}
		}
	}
	return true
}

func (tbl *table) getNode(addr Addr, id int160) *node {
	for _, n := range tbl.buckets[tbl.bucketIndex(id)] {
		if n.id == id && n.addr.String() == addr.String() {
			return n
		}
	}
	return nil
}

func (tbl *table) closestNodes(k int, target int160, filter func(*node) bool) (ret []*node) {
	for bi := func() int {
		if target == tbl.rootID {
			return len(tbl.buckets) - 1
		} else {
			return tbl.bucketIndex(target)
		}
	}(); bi >= 0 && len(ret) < k; bi-- {
		ret = append(ret, tbl.buckets[bi]...)
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
	bi := tbl.bucketIndex(n.id)
	b := tbl.buckets[bi]
	for _, n_ := range b {
		if n.addr.String() == n_.addr.String() && n.id.Cmp(n_.id) == 0 {
			return
		}
	}
	if len(b) < tbl.k {
		b = append(b, n)
	} else {
		for i, n_ := range b {
			if !n_.IsGood() {
				b[i] = n
				break
			}
		}
	}
	tbl.buckets[bi] = b
}
