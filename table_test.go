package dht

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := table{k: 8}
	var maxFar int160
	maxFar.SetMax()
	assert.Equal(t, 0, tbl.bucketIndex(maxFar))
	assert.Panics(t, func() { tbl.bucketIndex(tbl.rootID) })

	assert.Error(t, tbl.addNode(&node{}))
	assert.Equal(t, 0, tbl.buckets[0].Len())

	id0 := int160FromByteString("\x2f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	id1 := int160FromByteString("\x2e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	n0 := &node{nodeKey: nodeKey{
		id:   id0,
		addr: NewAddr(&net.UDPAddr{}),
	}}
	n1 := &node{nodeKey: nodeKey{
		id:   id1,
		addr: NewAddr(&net.UDPAddr{}),
	}}

	assert.NoError(t, tbl.addNode(n0))
	assert.Equal(t, 1, tbl.buckets[2].Len())

	assert.Error(t, tbl.addNode(n0))
	assert.Equal(t, 1, tbl.buckets[2].Len())
	assert.Equal(t, 1, tbl.numNodes())

	assert.NoError(t, tbl.addNode(n1))
	assert.Equal(t, 2, tbl.buckets[2].Len())
	assert.Equal(t, 2, tbl.numNodes())

	tbl.dropNode(n0)
	assert.Equal(t, 1, tbl.buckets[2].Len())
	assert.Equal(t, 1, tbl.numNodes())

	tbl.dropNode(n1)
	assert.Equal(t, 0, tbl.buckets[2].Len())
	assert.Equal(t, 0, tbl.numNodes())
}
