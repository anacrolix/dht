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
}
