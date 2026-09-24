package int160

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestBitLen(t *testing.T) {
	check := func(x T) {
		t.Helper()
		qt.Check(t, qt.Equals(x.BitLen(), new(big.Int).SetBytes(x.bits[:]).BitLen()), qt.Commentf("%v", x))
	}
	check(T{})
	var max T
	max.SetMax()
	check(max)
	for i := range 160 {
		var x T
		x.SetBit(i, true)
		check(x)
	}
	for range 100 {
		var b [20]byte
		rand.Read(b[:])
		check(FromByteArray(b))
	}
}

func TestCmp(t *testing.T) {
	var a, b T
	qt.Check(t, qt.Equals(a.Cmp(b), 0))
	b.SetBit(159, true)
	qt.Check(t, qt.Equals(a.Cmp(b), -1))
	qt.Check(t, qt.Equals(b.Cmp(a), 1))
	a.SetBit(0, true)
	qt.Check(t, qt.Equals(a.Cmp(b), 1))
}

func BenchmarkBitLen(b *testing.B) {
	var x T
	x.SetBit(100, true)
	for b.Loop() {
		_ = x.BitLen()
	}
}
