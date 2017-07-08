package dht

type table struct {
	rootID  int160
	buckets [160][]*node
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
		if n.id == id && n.addr == addr {
			return n
		}
	}
	return nil
}

func (tbl *table) closestNodes(k int, target int160, filter func(*node) bool) []*node {
	nodes := tbl.buckets[tbl.bucketIndex(target)]
	if len(nodes) > k {
		nodes = nodes[:k]
	}
	return nodes
}
