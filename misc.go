package dht

import (
	"fmt"
	"net"

	"github.com/anacrolix/dht/krpc"
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

type nodesByDistance struct {
	nis    []addrMaybeId
	target int160
}

func (me nodesByDistance) Len() int { return len(me.nis) }
func (me nodesByDistance) Less(i, j int) bool {
	if me.nis[i].Id == nil {
		return false
	}
	if me.nis[j].Id == nil {
		return true
	}
	return distance(*me.nis[i].Id, me.target).Cmp(distance(*me.nis[j].Id, me.target)) < 0
}
func (me *nodesByDistance) Pop() interface{} {
	ret := me.nis[len(me.nis)-1]
	me.nis = me.nis[:len(me.nis)-1]
	return ret
}
func (me *nodesByDistance) Push(x interface{}) {
	me.nis = append(me.nis, x.(addrMaybeId))
}
func (me nodesByDistance) Swap(i, j int) {
	me.nis[i], me.nis[j] = me.nis[j], me.nis[i]
}
