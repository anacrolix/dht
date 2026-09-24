package int160

import (
	"bytes"
	"encoding/hex"
	"math"
	"math/bits"
)

type T struct {
	bits [20]uint8
}

func (me T) String() string {
	return hex.EncodeToString(me.bits[:])
}

func (me *T) AsByteArray() [20]byte {
	return me.bits
}

func (me *T) ByteString() string {
	return string(me.bits[:])
}

func (me *T) BitLen() int {
	for i, b := range me.bits {
		if b != 0 {
			return (len(me.bits)-i)*8 - bits.LeadingZeros8(b)
		}
	}
	return 0
}

func (me *T) SetBytes(b []byte) {
	n := copy(me.bits[:], b)
	if n != 20 {
		panic(n)
	}
}

func (me *T) SetBit(index int, val bool) {
	var orVal uint8
	if val {
		orVal = 1 << (7 - index%8)
	}
	var mask uint8 = ^(1 << (7 - index%8))
	me.bits[index/8] = me.bits[index/8]&mask | orVal
}

func (me *T) GetBit(index int) bool {
	return me.bits[index/8]>>(7-index%8)&1 == 1
}

func (me T) Bytes() []byte {
	return me.bits[:]
}

func (l T) Cmp(r T) int {
	return bytes.Compare(l.bits[:], r.bits[:])
}

func (me *T) SetMax() {
	for i := range me.bits {
		me.bits[i] = math.MaxUint8
	}
}

func (me *T) Xor(a, b *T) {
	for i := range me.bits {
		me.bits[i] = a.bits[i] ^ b.bits[i]
	}
}

func (me *T) IsZero() bool {
	return me.bits == [20]uint8{}
}

func FromBytes(b []byte) (ret T) {
	ret.SetBytes(b)
	return
}

func FromByteArray(b [20]byte) T {
	return T{bits: b}
}

func FromByteString(s string) (ret T) {
	ret.SetBytes([]byte(s))
	return
}

func Distance(a, b T) (ret T) {
	ret.Xor(&a, &b)
	return
}

func (a T) Distance(b T) (ret T) {
	ret.Xor(&a, &b)
	return
}
