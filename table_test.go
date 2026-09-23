package dht

import (
	"net"
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

// BEP 5 replies must contain the k nodes closest to the target. closestNodes walks buckets and
// then cuts the list at k, so a partially included bucket keeps whichever nodes the map yields
// first rather than the nearest ones.
func TestClosestNodesKeepsNearestInBucket(t *testing.T) {
	var root int160.T
	tbl := table{rootID: root, k: 8}
	var target int160.T
	target.SetBit(10, true)

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
	want := map[int]bool{1000: true, 1001: true, 1002: true, 2090: true, 2080: true, 2070: true, 2060: true, 2050: true}

	for try := range 30 {
		got := tbl.closestNodes(8, target, func(*node) bool { return true })
		if len(got) != 8 {
			t.Fatalf("try %d: got %d nodes", try, len(got))
		}
		var prev int160.T
		seen := map[int]bool{}
		for i, n := range got {
			d := n.Id.Distance(target)
			if i > 0 && prev.Cmp(d) > 0 {
				t.Fatalf("try %d: result is not ordered by XOR distance", try)
			}
			prev = d
			seen[n.Addr.Port()] = true
		}
		if len(seen) != len(want) {
			t.Fatalf("try %d: ports %v, want %v", try, seen, want)
		}
		for port := range want {
			if !seen[port] {
				t.Fatalf("try %d: dropped closer node on port %d; got %v", try, port, seen)
			}
		}
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
