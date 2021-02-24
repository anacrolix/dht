package dht

import (
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/stm"
	"github.com/benbjohnson/immutable"
)

type pendingAnnouncePeers struct {
	inner *immutable.SortedMap
	k     int
}

func newPendingAnnouncePeers(target int160.T) pendingAnnouncePeers {
	return pendingAnnouncePeers{
		k: 8,
		inner: immutable.NewSortedMap(comparer{less: func(l, r interface{}) bool {
			return l.(pendingAnnouncePeer).addrMaybeId.closerThan(r.(pendingAnnouncePeer).addrMaybeId, target)
		}}),
	}
}

func (me *pendingAnnouncePeers) Range(f func(interface{})) {
	iter := me.inner.Iterator()
	for !iter.Done() {
		key, _ := iter.Next()
		f(key)
	}
}

func (me pendingAnnouncePeers) Len() int {
	return me.inner.Len()
}

func (me pendingAnnouncePeers) Push(x pendingAnnouncePeer) pendingAnnouncePeers {
	me.inner = me.inner.Set(x, nil)
	for me.inner.Len() > me.k {
		iter := me.inner.Iterator()
		iter.Last()
		key, _ := iter.Next()
		me.inner = me.inner.Delete(key)
	}
	return me
}

func (me pendingAnnouncePeers) Pop(tx *stm.Tx) (pendingAnnouncePeers, pendingAnnouncePeer) {
	iter := me.inner.Iterator()
	x, _ := iter.Next()
	me.inner = me.inner.Delete(x)
	return me, x.(pendingAnnouncePeer)
}

func (me pendingAnnouncePeers) Farthest() (value pendingAnnouncePeer, ok bool) {
	iter := me.inner.Iterator()
	iter.Last()
	if iter.Done() {
		return
	}
	key, _ := iter.Next()
	value = key.(pendingAnnouncePeer)
	ok = true
	return
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
