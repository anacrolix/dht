package dht

import (
	"testing"
)

func TestDefaultTraversalBloomFilterCharacteristics(t *testing.T) {
	bf := newBloomFilterForTraversal()
	t.Logf("%d bits with %d hashes per item", bf.Cap(), bf.K())
}
