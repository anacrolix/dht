package k_nearest_nodes

import (
	"cmp"
	"hash/maphash"

	"github.com/benbjohnson/immutable"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

type Key = krpc.NodeInfoAddrPort

type Elem struct {
	Key
	Data any
}

type Type struct {
	inner *immutable.SortedMap[Key, any]
	k     int
}

func New(target int160.T, k int) Type {
	return Type{
		k:     k,
		inner: immutable.NewSortedMap[Key, any](comparer{target: target, seed: maphash.MakeSeed()}),
	}
}

// Orders keys by distance to target, breaking ties with a seeded hash of the address so that
// distinct addresses sharing an ID are retained.
type comparer struct {
	target int160.T
	seed   maphash.Seed
}

func (c comparer) Compare(l, r Key) int {
	if d := l.ID.Int160().Distance(c.target).Cmp(r.ID.Int160().Distance(c.target)); d != 0 {
		return d
	}
	return cmp.Compare(maphash.String(c.seed, l.Addr.String()), maphash.String(c.seed, r.Addr.String()))
}

func (me *Type) Range(f func(Elem)) {
	iter := me.inner.Iterator()
	for !iter.Done() {
		key, value, _ := iter.Next()
		f(Elem{
			Key:  key,
			Data: value,
		})
	}
}

func (me Type) Len() int {
	return me.inner.Len()
}

func (me Type) Push(elem Elem) Type {
	me.inner = me.inner.Set(elem.Key, elem.Data)
	for me.inner.Len() > me.k {
		iter := me.inner.Iterator()
		iter.Last()
		key, _, _ := iter.Next()
		me.inner = me.inner.Delete(key)
	}
	return me
}

func (me Type) Farthest() (elem Elem) {
	iter := me.inner.Iterator()
	iter.Last()
	key, value, ok := iter.Next()
	if !ok {
		panic(me.k)
	}
	return Elem{
		Key:  key,
		Data: value,
	}
}

func (me Type) Full() bool {
	return me.Len() >= me.k
}
