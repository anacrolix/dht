package krpc

import "github.com/anacrolix/missinggo/slices"

const (
	ipv4AddrSize = 6
	i2pAddrSize  = 34
)

var nodeAddrNumBytes = ipv4AddrSize

type CompactNodeAddrs []NodeAddr

func (CompactNodeAddrs) ElemSize() int { return nodeAddrNumBytes }

func (me CompactNodeAddrs) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(slices.Map(func(addr NodeAddr) NodeAddr {
		return addr.Compacted()
	}, me).(CompactNodeAddrs))
}

func (me CompactNodeAddrs) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactNodeAddrs) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactNodeAddrs) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}

func (me CompactNodeAddrs) NodeAddrs() []NodeAddr {
	return me
}

func (me CompactNodeAddrs) Index(x NodeAddr) int {
	return addrIndex(me, x)
}
