package krpc

import "github.com/anacrolix/missinggo/slices"

type CompactI2PNodeInfo []NodeInfo

func (CompactI2PNodeInfo) ElemSize() int {
	return 26
}

// func (me *CompactI2PNodeInfo) Scrub() {
// 	slices.FilterInPlace(me, func(ni *NodeInfo) bool {
// 		ni.Addr.IP = ni.Addr.IP.To4()
// 		return ni.Addr.IP != nil
// 	})
// }

func (me CompactI2PNodeInfo) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(slices.Map(func(ni NodeInfo) NodeInfo {
		ni.Addr.IP = ni.Addr.IP.To4()
		return ni
	}, me).(CompactI2PNodeInfo))
}

func (me CompactI2PNodeInfo) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactI2PNodeInfo) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactI2PNodeInfo) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
