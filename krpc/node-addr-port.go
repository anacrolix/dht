package krpc

import (
	"net"
	"net/netip"
)

// This is a comparable replacement for NodeAddr.
type NodeAddrPort struct {
	netip.AddrPort
}

func (me NodeAddrPort) ToNodeAddr() NodeAddr {
	return NodeAddr{me.Addr().AsSlice(), int(me.Port())}
}

func (me NodeAddrPort) UDP() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   me.Addr().AsSlice(),
		Port: int(me.Port()),
	}
}

func (me NodeAddrPort) IP() net.IP {
	return me.Addr().AsSlice()
}

// Orders by address, then port.
func (me NodeAddrPort) Compare(r NodeAddrPort) int {
	return me.AddrPort.Compare(r.AddrPort)
}
