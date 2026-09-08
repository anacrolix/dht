package dht

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"

	"github.com/go-quicktest/qt"
)

func TestSaveLoadNodesFile(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	qt.Assert(t, qt.IsNil(err))
	defer os.Remove(f.Name())
	f.Close()
	ns := []krpc.NodeInfo{krpc.RandomNodeInfo(4), krpc.RandomNodeInfo(16)}
	qt.Assert(t, qt.IsNil(WriteNodesToFile(ns, f.Name())))
	_ns, err := ReadNodesFromFile(f.Name())
	qt.Check(t, qt.IsNil(err))
	_ns[0].Addr.IP = _ns[0].Addr.IP.To4()
	qt.Check(t, qt.DeepEquals(_ns, ns))
}
