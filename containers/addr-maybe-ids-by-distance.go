package containers

import (
	"github.com/benbjohnson/immutable"

	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/types"
)

type addrMaybeId = types.AddrMaybeId

type AddrMaybeIdsByDistance interface {
	Add(addrMaybeId) AddrMaybeIdsByDistance
	Next() addrMaybeId
	Delete(addrMaybeId) AddrMaybeIdsByDistance
	Len() int
}

// Orders addrMaybeIds by their distance to a target. CloserThan is a total order, so elements that
// compare equal are the same set element.
type closerThanTarget struct {
	target int160.T
}

func (me closerThanTarget) Compare(l, r addrMaybeId) int {
	if l.CloserThan(r, me.target) {
		return -1
	}
	if r.CloserThan(l, me.target) {
		return 1
	}
	return 0
}

// A persistent set of addrMaybeId ordered by distance to a target, backed by an immutable sorted
// map with empty values.
type sortedSet struct {
	m *immutable.SortedMap[addrMaybeId, struct{}]
}

// Returns the element closest to the target. Panics if the set is empty.
func (me sortedSet) Next() addrMaybeId {
	first, _, ok := me.m.Iterator().Next()
	if !ok {
		panic("next called on empty set")
	}
	return first
}

func (me sortedSet) Delete(x addrMaybeId) AddrMaybeIdsByDistance {
	return sortedSet{me.m.Delete(x)}
}

func (me sortedSet) Len() int {
	return me.m.Len()
}

func (me sortedSet) Add(x addrMaybeId) AddrMaybeIdsByDistance {
	return sortedSet{me.m.Set(x, struct{}{})}
}

func NewImmutableAddrMaybeIdsByDistance(target int160.T) AddrMaybeIdsByDistance {
	return sortedSet{immutable.NewSortedMap[addrMaybeId, struct{}](closerThanTarget{target})}
}
