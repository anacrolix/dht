package dht

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := table{k: 8}
	var maxFar int160
	maxFar.SetMax()
	assert.Equal(t, 0, tbl.bucketIndex(maxFar))
	assert.Panics(t, func() { tbl.bucketIndex(tbl.rootID) })
	tbl.addNode(&node{})
	tbl.addNode(&node{id: int160FromByteString("\x2f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")})
	assert.Equal(t, 1, tbl.buckets[2].Len())
	assert.Equal(t, 0, tbl.buckets[0].Len())
	assert.Equal(t, 1, tbl.numNodes())
}
