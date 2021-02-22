package dht

import (
	"testing"

	"github.com/anacrolix/missinggo/v2/iter"
	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

func int160WithBitSet(bit int) *int160.T {
	var i int160.T
	i.SetBit(7+bit*8, true)
	return &i
}

var sampleAddrMaybeIds = []addrMaybeId{
	addrMaybeId{},
	addrMaybeId{Id: new(int160.T)},
	addrMaybeId{Id: int160WithBitSet(13)},
	addrMaybeId{Id: int160WithBitSet(12)},
	addrMaybeId{Addr: krpc.NodeAddr{Port: 1}},
	addrMaybeId{
		Id:   int160WithBitSet(14),
		Addr: krpc.NodeAddr{Port: 1}},
}

func TestNodesByDistance(t *testing.T) {
	a := nodesByDistance(int160.T{})
	push := func(i int) {
		a = a.Add(sampleAddrMaybeIds[i])
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
				ret = append(ret, sampleAddrMaybeIds[i])
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
