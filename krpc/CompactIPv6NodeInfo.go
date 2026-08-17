package krpc

type (
	CompactIPv6NodeInfo []NodeInfo
)

func (CompactIPv6NodeInfo) ElemSize() int {
	return 38
}

func (me CompactIPv6NodeInfo) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(mapSlice(me, func(ni NodeInfo) NodeInfo {
		ni.Addr.IP = ni.Addr.IP.To16()
		return ni
	}))
}

func (me CompactIPv6NodeInfo) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactIPv6NodeInfo) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactIPv6NodeInfo) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
