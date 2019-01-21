package dht

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodesByDistance(t *testing.T) {
	var a nodesByDistance
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
		heap.Push(&a, amis[i])
	}
	push(2)
	push(0)
	push(3)
	push(0)
	push(1)
	pop := func(i int) {
		assert.Equal(t, amis[i], a.nis[0])
		assert.Equal(t, amis[i], heap.Pop(&a))
	}
	pop(1)
	pop(2)
	pop(3)
	pop(0)
	pop(0)
}
