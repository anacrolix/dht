package containers

import (
	"testing"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/internal/testutil"
	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/stretchr/testify/assert"
)

func TestNodesByDistance(t *testing.T) {
	a := NewAddrMaybeIdsByDistance(int160.T{})
	push := func(i int) {
		a = a.Add(testutil.SampleAddrMaybeIds[i])
	}
	push(4)
	push(2)
	push(0)
	push(3)
	push(0)
	push(1)
	pop := func(is ...int) {
		first, ok := iter.First(a.Iter)
		assert.True(t, ok)
		assert.Contains(t, func() (ret []addrMaybeId) {
			for _, i := range is {
				ret = append(ret, testutil.SampleAddrMaybeIds[i])
			}
			return
		}(), first)
		a = a.Delete(first)
	}
	pop(1)
	pop(2)
	pop(3)
	pop(0, 4)
	pop(0, 4)
	//pop(0, 4)
	assert.EqualValues(t, 0, a.Len())
}
