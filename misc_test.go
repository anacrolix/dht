package dht

import (
	"testing"

	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/stretchr/testify/assert"
)

func TestNodesByDistance(t *testing.T) {
	a := nodesByDistance(int160{})
	amis := []addrMaybeId{
		addrMaybeId{},
		addrMaybeId{Id: new(int160)},
		addrMaybeId{Id: func() *int160 {
			var i int160
			i.bits[13] = 1
			return &i
		}()},
		addrMaybeId{Id: func() *int160 {
			var i int160
			i.bits[12] = 1
			return &i
		}()},
	}
	push := func(i int) {
		a = a.Add(amis[i])
	}
	push(2)
	push(0)
	push(3)
	push(0)
	push(1)
	pop := func(i int) {
		first, ok := iter.First(a.Iter)
		assert.True(t, ok)
		assert.Equal(t, amis[i], first)
		a = a.Delete(first)
	}
	pop(1)
	pop(2)
	pop(3)
	pop(0)
	assert.EqualValues(t, 0, a.Len())
}
