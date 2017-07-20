package dht

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := table{}
	var maxFar int160
	maxFar.SetMax()
	assert.Equal(t, 0, tbl.bucketIndex(maxFar))
	assert.Equal(t, 160, tbl.bucketIndex(tbl.rootID))
	tbl.addNode(&node{})
	tbl.addNode(&node{id: int160FromByteString("\x2f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")})
	assert.Len(t, tbl.buckets[2], 1)
	assert.Len(t, tbl.buckets[0], 0)
	assert.Equal(t, 1, tbl.numNodes())
}
