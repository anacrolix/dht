package krpc

type CompactIPv6NodeAddrs []NodeAddr

func (CompactIPv6NodeAddrs) ElemSize() int { return 6 }

func (me CompactIPv6NodeAddrs) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(me)
}

func (me CompactIPv6NodeAddrs) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactIPv6NodeAddrs) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactIPv6NodeAddrs) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
