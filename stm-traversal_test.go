package dht

import (
	"testing"

	"github.com/anacrolix/stm"
	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

func TestTraversal(t *testing.T) {
	var target int160.T
	traversal := newTraversal(NewTraversalInput{Target: target.AsByteArray()})
	traversal.shouldContact = func(krpc.NodeAddr, *stm.Tx) bool {
		return true
	}
	assert.False(t, stm.Atomically(func(tx *stm.Tx) interface{} { _, ok := traversal.popNextContact(tx); return ok }).(bool))
	assert.False(t, stm.WouldBlock(stm.VoidOperation(traversal.waitFinished)))
	stm.Atomically(stm.Compose(func() (ret []stm.Operation) {
		for _, v := range sampleAddrMaybeIds[2:6] {
			ret = append(ret, traversal.pendContact(v))
		}
		return
	}()...))
	assert.False(t, stm.WouldBlock(stm.VoidOperation(func(tx *stm.Tx) { traversal.popNextContact(tx) })))
	assert.True(t, stm.WouldBlock(stm.VoidOperation(traversal.waitFinished)))
	addrs := stm.Atomically(func(tx *stm.Tx) interface{} {
		var ret []krpc.NodeAddr
		for {
			next, ok := traversal.popNextContact(tx)
			if !ok {
				break
			}
			ret = append(ret, next.Addr)
		}
		return ret
	}).([]krpc.NodeAddr)
	assert.False(t, stm.WouldBlock(stm.VoidOperation(traversal.waitFinished)))
	t.Log(addrs)
	assert.EqualValues(t, []krpc.NodeAddr{{Port: 1}, {}}, addrs)
}
