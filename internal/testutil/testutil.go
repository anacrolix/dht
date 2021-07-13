package testutil

import (
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/types"
)

func Int160WithBitSet(bit int) *int160.T {
	var i int160.T
	i.SetBit(7+bit*8, true)
	return &i
}

type addrMaybeId = types.AddrMaybeId

var SampleAddrMaybeIds = []addrMaybeId{
	addrMaybeId{},
	addrMaybeId{Id: new(int160.T)},
	addrMaybeId{Id: Int160WithBitSet(13)},
	addrMaybeId{Id: Int160WithBitSet(12)},
	addrMaybeId{Addr: krpc.NodeAddr{Port: 1}},
	addrMaybeId{
		Id:   Int160WithBitSet(14),
		Addr: krpc.NodeAddr{Port: 1}},
}
