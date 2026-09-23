package krpc

import "fmt"

type CompactIPv4NodeAddrs []NodeAddr

func (CompactIPv4NodeAddrs) ElemSize() int { return 6 }

func (me CompactIPv4NodeAddrs) MarshalBinary() ([]byte, error) {
	converted := make(CompactIPv4NodeAddrs, len(me))
	for i, addr := range me {
		ip := addr.IP.To4()
		if ip == nil {
			return nil, fmt.Errorf("marshal compact IPv4 address from %v", addr.IP)
		}
		addr.IP = ip
		converted[i] = addr
	}
	return marshalBinarySlice(converted)
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
