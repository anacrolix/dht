package dht

import (
	"encoding/hex"
	"math"
	"math/big"
)

type int160 struct {
	bits [20]uint8
}

func (me int160) String() string {
	return hex.EncodeToString(me.bits[:])
}

func (me *int160) AsByteArray() [20]byte {
	return me.bits
}

func (me *int160) ByteString() string {
	return string(me.bits[:])
}

func (me *int160) BitLen() int {
	var a big.Int
	a.SetBytes(me.bits[:])
	return a.BitLen()
}

func (me *int160) SetBytes(b []byte) {
	n := copy(me.bits[:], b)
	if n != 20 {
		panic(n)
	}
}

func (me *int160) SetBit(index int, val bool) {
	var orVal uint8
	if val {
		orVal = 1 << (7 - index%8)
	}
	var mask uint8 = ^(1 << (7 - index%8))
	me.bits[index/8] = me.bits[index/8]&mask | orVal
}

func (me *int160) GetBit(index int) bool {
	return me.bits[index/8]>>(7-index%8)&1 == 1
}

func (me int160) Bytes() []byte {
	return me.bits[:]
}

func (l int160) Cmp(r int160) int {
	for i := range l.bits {
		if l.bits[i] < r.bits[i] {
			return -1
		} else if l.bits[i] > r.bits[i] {
			return 1
		}
	}
	return 0
}

func (me *int160) SetMax() {
	for i := range me.bits {
		me.bits[i] = math.MaxUint8
	}
}

func (me *int160) Xor(a, b *int160) {
	for i := range me.bits {
		me.bits[i] = a.bits[i] ^ b.bits[i]
	}
}

func (me *int160) IsZero() bool {
	for _, b := range me.bits {
		if b != 0 {
			return false
		}
	}
	return true
}

func int160FromBytes(b []byte) (ret int160) {
	ret.SetBytes(b)
	return
}

func int160FromByteArray(b [20]byte) (ret int160) {
	ret.SetBytes(b[:])
	return
}

func int160FromByteString(s string) (ret int160) {
	ret.SetBytes([]byte(s))
	return
}

func distance(a, b int160) (ret int160) {
	ret.Xor(&a, &b)
	return
}
