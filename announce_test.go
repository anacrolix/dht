package dht

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnounceNoStartingNodes(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		Conn:       mustListen(":0"),
		NoSecurity: true,
	})
	require.NoError(t, err)
	defer s.Close()
	var ih [20]byte
	copy(ih[:], "blah")
	_, err = s.Announce(ih, 0, true)
	require.EqualError(t, err, "no initial nodes")
}

func TestDefaultTraversalBloomFilterCharacteristics(t *testing.T) {
	bf := newBloomFilterForTraversal()
	t.Logf("%d bits with %d hashes per item", bf.Cap(), bf.K())
}
