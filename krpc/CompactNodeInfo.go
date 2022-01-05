package krpc

import "github.com/anacrolix/missinggo/slices"

type (
	CompactNodeInfo []NodeInfo
)

func (CompactNodeInfo) ElemSize() int {
	return 20 + nodeAddrNumBytes
}

// func (me *CompactIPv4NodeInfo) Scrub() {
// 	slices.FilterInPlace(me, func(ni *NodeInfo) bool {
// 		ni.Addr.IP = ni.Addr.IP.To4()
// 		return ni.Addr.IP != nil
// 	})
// }

func (me CompactNodeInfo) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(slices.Map(func(ni NodeInfo) NodeInfo {
		ni.Addr.IP = ni.Addr.IP.To4()
		return ni
	}, me).(CompactNodeInfo))
}

func (me CompactNodeInfo) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactNodeInfo) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactNodeInfo) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
