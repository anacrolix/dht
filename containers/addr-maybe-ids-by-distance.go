package containers

import (
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/types"
	"github.com/anacrolix/stm/stmutil"
)

type addrMaybeId = types.AddrMaybeId

func NewAddrMaybeIdsByDistance(target int160.T) stmutil.Settish {
	return stmutil.NewSortedSet(func(l, r interface{}) bool {
		return l.(addrMaybeId).CloserThan(r.(addrMaybeId), target)
	})
}
