package krpc

import "github.com/anacrolix/missinggo/slices"

/*
type CompactNodeAddr interface {
	ElemSize() int
	MarshalBinary() ([]byte, error)
	MarshalBencode() ([]byte, error)
	NodeAddrs() []NodeAddr
	Index(x NodeAddr) int
}
*/

type CompactIPv4NodeAddrs []NodeAddr

func (CompactIPv4NodeAddrs) ElemSize() int { return 6 }

func (me CompactIPv4NodeAddrs) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(slices.Map(func(addr NodeAddr) NodeAddr {
		return addr.Compacted()
	}, me).(CompactIPv4NodeAddrs))
}

func (me CompactIPv4NodeAddrs) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactIPv4NodeAddrs) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactIPv4NodeAddrs) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}

func (me CompactIPv4NodeAddrs) NodeAddrs() []NodeAddr {
	return me
}

func (me CompactIPv4NodeAddrs) Index(x NodeAddr) int {
	return addrIndex(me, x)
}
