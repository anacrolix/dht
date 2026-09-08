package containers

import (
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/internal/testutil"
)

func TestSampleAddrsDiffer(t *testing.T) {
	for i, a := range testutil.SampleAddrMaybeIds {
		for j, b := range testutil.SampleAddrMaybeIds[i+1:] {
			qt.Assert(t, qt.Not(qt.Equals(a, b)), qt.Commentf("%v, %v", i, j+i+1))
		}
	}
}

func TestNodesByDistance(t *testing.T) {
	a := NewImmutableAddrMaybeIdsByDistance(int160.T{})
	push := func(i int) {
		a = a.Add(testutil.SampleAddrMaybeIds[i])
	}
	push(4)
	qt.Assert(t, qt.Equals(a.Len(), 1))
	push(2)
	qt.Assert(t, qt.Equals(a.Len(), 2))
	push(0)
	qt.Assert(t, qt.Equals(a.Len(), 3))
	push(3)
	qt.Assert(t, qt.Equals(a.Len(), 4))
	push(0)
	qt.Assert(t, qt.Equals(a.Len(), 4))
	push(1)
	qt.Assert(t, qt.Equals(a.Len(), 5))
	pop := func(is ...int) {
		ok := a.Len() != 0
		qt.Check(t, qt.IsTrue(ok))
		first := a.Next()
		qt.Check(t, qt.SliceContains(func() (ret []addrMaybeId) {
			for _, i := range is {
				ret = append(ret, testutil.SampleAddrMaybeIds[i])
			}
			return
		}(), first))
		a = a.Delete(first)
	}
	pop(1)
	pop(2)
	pop(3)
	pop(0, 4)
	pop(0, 4)
	// pop(0, 4)
	qt.Check(t, qt.Equals(a.Len(), 0))
}
