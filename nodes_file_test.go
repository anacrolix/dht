package dht

import (
	"io/ioutil"
	"os"

	"testing"

	"github.com/anacrolix/dht/krpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLoadNodesFile(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.Close()
	ns := []krpc.NodeInfo{krpc.RandomNodeInfo(4), krpc.RandomNodeInfo(16)}
	require.NoError(t, WriteNodesToFile(ns, f.Name()))
	_ns, err := ReadNodesFromFile(f.Name())
	_ns[0].Addr.IP = _ns[0].Addr.IP.To4()
	assert.EqualValues(t, ns, _ns)
}
