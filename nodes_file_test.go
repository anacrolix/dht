package dht

import (
	"path/filepath"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"

	"github.com/go-quicktest/qt"
)

func TestSaveLoadNodesFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "nodes")
	ns := []krpc.NodeInfo{krpc.RandomNodeInfo(4), krpc.RandomNodeInfo(16)}
	qt.Assert(t, qt.IsNil(WriteNodesToFile(ns, name)))
	_ns, err := ReadNodesFromFile(name)
	qt.Check(t, qt.IsNil(err))
	_ns[0].Addr.IP = _ns[0].Addr.IP.To4()
	qt.Check(t, qt.DeepEquals(_ns, ns))
}
