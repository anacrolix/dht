package dhtutil

import (
	"github.com/anacrolix/stm"
	"github.com/benbjohnson/immutable"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/types"
)

type KNearestNodesElem struct {
	types.AddrMaybeId
	Data interface{}
}

type KNearestNodes struct {
	inner *immutable.SortedMap
	k     int
}

func NewKNearestNodes(target int160.T) KNearestNodes {
	return KNearestNodes{
		k: 8,
		inner: immutable.NewSortedMap(comparer{less: func(l, r interface{}) bool {
			return l.(KNearestNodesElem).AddrMaybeId.CloserThan(r.(KNearestNodesElem).AddrMaybeId, target)
		}}),
	}
}

func (me *KNearestNodes) Range(f func(interface{})) {
	iter := me.inner.Iterator()
	for !iter.Done() {
		key, _ := iter.Next()
		f(key)
	}
}

func (me KNearestNodes) Len() int {
	return me.inner.Len()
}

func (me KNearestNodes) Push(x KNearestNodesElem) KNearestNodes {
	me.inner = me.inner.Set(x, nil)
	for me.inner.Len() > me.k {
		iter := me.inner.Iterator()
		iter.Last()
		key, _ := iter.Next()
		me.inner = me.inner.Delete(key)
	}
	return me
}

func (me KNearestNodes) Pop(tx *stm.Tx) (KNearestNodes, KNearestNodesElem) {
	iter := me.inner.Iterator()
	x, _ := iter.Next()
	me.inner = me.inner.Delete(x)
	return me, x.(KNearestNodesElem)
}

func (me KNearestNodes) Farthest() (value KNearestNodesElem, ok bool) {
	iter := me.inner.Iterator()
	iter.Last()
	if iter.Done() {
		return
	}
	key, _ := iter.Next()
	value = key.(KNearestNodesElem)
	ok = true
	return
}

func (me KNearestNodes) Full() bool {
	return me.Len() >= me.k
}

type lessFunc func(l, r interface{}) bool

type comparer struct {
	less lessFunc
}

func (me comparer) Compare(i, j interface{}) int {
	if me.less(i, j) {
		return -1
	} else if me.less(j, i) {
		return 1
	} else {
		return 0
	}
}
