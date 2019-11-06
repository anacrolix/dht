package dht

import (
	"testing"

	"github.com/anacrolix/stm"
	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/dht/v2/krpc"
)

func TestTraversal(t *testing.T) {
	var target int160
	traversal := newTraversal(target)
	assert.True(t, stm.WouldBlock(stm.VoidOperation(func(tx *stm.Tx) { traversal.nextAddr(tx) })))
	assert.False(t, stm.WouldBlock(stm.VoidOperation(traversal.finished)))
	stm.Atomically(stm.Compose(func() (ret []stm.Operation) {
		for _, v := range sampleAddrMaybeIds[2:6] {
			ret = append(ret, traversal.pendContact(v))
		}
		return
	}()...))
	assert.False(t, stm.WouldBlock(stm.VoidOperation(func(tx *stm.Tx) { traversal.nextAddr(tx) })))
	assert.True(t, stm.WouldBlock(stm.VoidOperation(traversal.finished)))
	pop := func(tx *stm.Tx) interface{} { return traversal.nextAddr(tx) }
	var addrs []krpc.NodeAddr
	for !stm.WouldBlock(pop) {
		addrs = append(addrs, stm.Atomically(pop).(krpc.NodeAddr))
	}
	assert.False(t, stm.WouldBlock(stm.VoidOperation(traversal.finished)))
	t.Log(addrs)
	assert.EqualValues(t, []krpc.NodeAddr{{Port: 1}, {}}, addrs)
}
