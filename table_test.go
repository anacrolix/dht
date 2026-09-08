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
