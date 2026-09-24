package dht

import (
	"bytes"
	"net"
	"slices"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/anacrolix/dht/v2/int160"
)

func TestTable(t *testing.T) {
	tbl := table{k: 8}
	var maxFar int160.T
	maxFar.SetMax()
	qt.Check(t, qt.Equals(tbl.bucketIndex(maxFar), 0))
	qt.Check(t, qt.PanicMatches(func() { tbl.bucketIndex(tbl.rootID) }, ".*"))

	qt.Check(t, qt.IsNotNil(tbl.addNode(&node{})))
	qt.Check(t, qt.Equals(tbl.buckets[0].Len(), 0))

	id0 := int160.FromByteString("\x2f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	id1 := int160.FromByteString("\x2e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	n0 := &node{nodeKey: nodeKey{
		Id:   id0,
		Addr: NewAddr(&net.UDPAddr{}),
	}}
	n1 := &node{nodeKey: nodeKey{
		Id:   id1,
		Addr: NewAddr(&net.UDPAddr{}),
	}}

	qt.Check(t, qt.IsNil(tbl.addNode(n0)))
	qt.Check(t, qt.Equals(tbl.buckets[2].Len(), 1))

	qt.Check(t, qt.IsNotNil(tbl.addNode(n0)))
	qt.Check(t, qt.Equals(tbl.buckets[2].Len(), 1))
	qt.Check(t, qt.Equals(tbl.numNodes(), 1))

	qt.Check(t, qt.IsNil(tbl.addNode(n1)))
	qt.Check(t, qt.Equals(tbl.buckets[2].Len(), 2))
	qt.Check(t, qt.Equals(tbl.numNodes(), 2))

	tbl.dropNode(n0)
	qt.Check(t, qt.Equals(tbl.buckets[2].Len(), 1))
	qt.Check(t, qt.Equals(tbl.numNodes(), 1))

	tbl.dropNode(n1)
	qt.Check(t, qt.Equals(tbl.buckets[2].Len(), 0))
	qt.Check(t, qt.Equals(tbl.numNodes(), 0))
}

func TestRandomIdInBucket(t *testing.T) {
	tbl := table{
		rootID: int160.FromByteArray(RandomNodeID()),
	}
	t.Logf("%v: table root id", tbl.rootID)
	for i := range tbl.buckets {
		id := tbl.randomIdForBucket(i)
		t.Logf("%v: random id for bucket index %v", id, i)
		qt.Assert(t, qt.Equals(tbl.bucketIndex(id), i))
	}
}

// Check exact nearest-K membership against a full-table oracle, independently of result order.
func TestClosestNodesKeepsNearestInBucket(t *testing.T) {
	var root int160.T
	tbl := table{rootID: root, k: 8}
	var target int160.T
	target.SetBit(10, true)
	var candidates []*node

	add := func(bucket, extra, port int) {
		t.Helper()
		var id int160.T
		id.SetBit(bucket, true)
		if extra >= 0 {
			id.SetBit(extra, true)
		}
		if got := tbl.bucketIndex(id); got != bucket {
			t.Fatalf("bit %d extra %d is bucket %d, want %d", bucket, extra, got, bucket)
		}
		n := &node{nodeKey: nodeKey{
			Id:   id,
			Addr: NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}),
		}}
		if err := tbl.addNode(n); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, n)
	}
	// Three nodes in the target's bucket. Every one of them is closer than every node in the next
	// bucket, so all three must be kept.
	for i, bit := range []int{20, 30, 40} {
		add(10, bit, 1000+i)
	}
	// Eight nodes in the next bucket. Only the five closest fit in the remaining slots.
	for _, bit := range []int{20, 30, 40, 50, 60, 70, 80, 90} {
		add(9, bit, 2000+bit)
	}
	// Include buckets above the target bucket too: those were skipped by the old walk.
	add(11, 120, 3011)
	add(159, -1, 3159)
	for _, tc := range []struct {
		name   string
		target int160.T
		k      int
		filter func(*node) bool
	}{
		{"partial bucket", target, 8, func(*node) bool { return true }},
		{"filtered candidates", target, 8, func(n *node) bool { return n.Addr.Port()%2 != 0 }},
		{"local target", root, 8, func(*node) bool { return true }},
		{"all eligible", target, len(candidates) + 1, func(*node) bool { return true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Bytewise XOR is independent of the production int160 distance calculation.
			distance := func(n *node) [20]byte {
				d := n.Id.AsByteArray()
				targetBytes := tc.target.AsByteArray()
				for i := range d {
					d[i] ^= targetBytes[i]
				}
				return d
			}
			var want []*node
			for _, n := range candidates {
				if tc.filter(n) {
					want = append(want, n)
				}
			}
			slices.SortFunc(want, func(a, b *node) int {
				ad, bd := distance(a), distance(b)
				return bytes.Compare(ad[:], bd[:])
			})
			if len(want) > tc.k {
				want = want[:tc.k]
			}
			got := tbl.closestNodes(tc.k, tc.target, tc.filter)
			qt.Assert(t, qt.Equals(len(got), len(want)))
			type identity struct {
				id   [20]byte
				addr string
			}
			remaining := make(map[identity]bool, len(want))
			for _, n := range want {
				remaining[identity{n.Id.AsByteArray(), n.Addr.String()}] = true
			}
			for _, n := range got {
				key := identity{n.Id.AsByteArray(), n.Addr.String()}
				if !remaining[key] {
					t.Errorf("unexpected or duplicate nearest node: id=%x addr=%s", key.id, key.addr)
				}
				delete(remaining, key)
			}
			for key := range remaining {
				t.Errorf("missing nearest node: id=%x addr=%s", key.id, key.addr)
			}
			// Keep ordering independent: a sorted but incorrect subset must still fail above.
			for i := 1; i < len(got); i++ {
				prev, next := distance(got[i-1]), distance(got[i])
				if bytes.Compare(prev[:], next[:]) > 0 {
					t.Errorf("XOR distance decreased at index %d: %x > %x", i, prev, next)
				}
			}
		})
	}
}

func TestClosestNodesAcrossRootBuckets(t *testing.T) {
	tbl := table{k: 8}
	var target int160.T
	target.SetBit(10, true)
	makeNode := func(bits ...int) *node {
		var id int160.T
		for _, bit := range bits {
			id.SetBit(bit, true)
		}
		n := &node{nodeKey: nodeKey{Id: id, Addr: NewAddr(&net.UDPAddr{
			IP: net.IPv4(127, 0, 0, 1), Port: bits[0] + 1,
		})}}
		qt.Assert(t, qt.IsNil(tbl.addNode(n)))
		return n
	}
	near := makeNode(10, 20)
	middle := makeNode(11)
	far := makeNode(9)
	for _, tc := range []struct {
		name     string
		k        int
		excluded *node
		want     []*node
	}{
		{"target bucket", 1, nil, []*node{near}},
		{"higher root bucket", 1, near, []*node{middle}},
		{"all buckets", 8, nil, []*node{near, middle, far}},
		{"zero limit", 0, nil, nil},
		{"negative limit", -1, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tbl.closestNodes(tc.k, target, func(n *node) bool { return n != tc.excluded })
			qt.Assert(t, qt.Equals(len(got), len(tc.want)))
			for i := range got {
				qt.Assert(t, qt.Equals(got[i], tc.want[i]))
			}
		})
	}
}
