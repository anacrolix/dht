package krpc

type Infohash [20]byte

type CompactInfohashes [][20]byte

func (CompactInfohashes) ElemSize() int { return 20 }

func (me CompactInfohashes) MarshalBinary() ([]byte, error) {
	ret := make([]byte, 0, len(me)*me.ElemSize())
	for _, ih := range me {
		ret = append(ret, ih[:]...)
	}
	return ret, nil
}

func (me CompactInfohashes) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactInfohashes) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactInfohashes) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
