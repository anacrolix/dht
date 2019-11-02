package dht

import (
	"fmt"
	"net"

	"github.com/lukechampine/stm/stmutil"

	"github.com/anacrolix/dht/v2/krpc"
)

func mustListen(addr string) net.PacketConn {
	ret, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	return ret
}

func addrResolver(addr string) func() ([]Addr, error) {
	return func() ([]Addr, error) {
		ua, err := net.ResolveUDPAddr("udp", addr)
		return []Addr{NewAddr(ua)}, err
	}
}

type addrMaybeId struct {
	Addr krpc.NodeAddr
	Id   *int160
}

func (me addrMaybeId) String() string {
	if me.Id == nil {
		return fmt.Sprintf("unknown id at %s", me.Addr)
	} else {
		return fmt.Sprintf("%x at %v", *me.Id, me.Addr)
	}
}

func nodesByDistance(target int160) stmutil.Settish {
	return stmutil.NewSortedSet(func(_l, _r interface{}) bool {
		l := _l.(addrMaybeId)
		if l.Id == nil {
			return false
		}
		r := _r.(addrMaybeId)
		if r.Id == nil {
			return true
		}
		return distance(*l.Id, target).Cmp(distance(*r.Id, target)) < 0
	})
}
