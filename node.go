package dht

import (
	"time"

	"github.com/anacrolix/dht/krpc"
)

type node struct {
	addr          Addr
	id            int160
	announceToken string

	lastGotQuery    time.Time
	lastGotResponse time.Time
	lastSentQuery   time.Time
}

func (n *node) IsSecure() bool {
	return NodeIdSecure(n.id.AsByteArray(), n.addr.UDPAddr().IP)
}

func (n *node) idString() string {
	return n.id.ByteString()
}

func (n *node) NodeInfo() (ret krpc.NodeInfo) {
	ret.Addr = n.addr.UDPAddr()
	if n := copy(ret.ID[:], n.idString()); n != 20 {
		panic(n)
	}
	return
}

// TODO: Match this to the spec.
func (n *node) DefinitelyGood() bool {
	if n.id.IsZero() {
		return false
	}
	// No reason to think ill of them if they've never been queried.
	if n.lastSentQuery.IsZero() {
		return true
	}
	// They answered our last query.
	if n.lastSentQuery.Before(n.lastGotResponse) {
		return true
	}
	return true
}
