package types

import (
	"math"
	"math/rand/v2"
	"net"
	"testing"

	"github.com/go-quicktest/qt"

	"github.com/anacrolix/dht/v2/krpc"
)

func TestNoIdFarther(tb *testing.T) {
	var a AddrMaybeId
	a.FromNodeInfo(krpc.RandomNodeInfo(16))
	target := krpc.RandomNodeID().Int160()
	b := a
	qt.Assert(tb, qt.IsFalse(a.CloserThan(b, target)))
	b.Id.SetNone()
	qt.Assert(tb, qt.IsTrue(a.CloserThan(b, target)))
	qt.Assert(tb, qt.IsFalse(b.CloserThan(a, target)))
	qt.Assert(tb, qt.IsFalse(b.CloserThan(b, target)))
	b.Id.SetSomeZeroValue()
	b.Id = a.Id
	qt.Assert(tb, qt.IsFalse(a.CloserThan(b, target)))
	id := a.Id.UnwrapPtr()
	for i := range 160 {
		if target.GetBit(i) != id.GetBit(i) {
			id.SetBit(i, target.GetBit(i))
			break
		}
	}
	tb.Log(a)
	tb.Log(b)
	tb.Log(target)
	qt.Assert(tb, qt.IsTrue(a.CloserThan(b, target)))
}

func TestCloserThanId(tb *testing.T) {
	var a AddrMaybeId
	a.FromNodeInfo(krpc.RandomNodeInfo(16))
	target := krpc.RandomNodeID().Int160()
	qt.Assert(tb, qt.IsFalse(a.CloserThan(a, target)))
	b := a
	b.Id.SetSomeZeroValue()
	b.Id = a.Id
	qt.Assert(tb, qt.IsFalse(a.CloserThan(b, target)))
	for i := range 160 {
		if target.GetBit(i) != a.Id.UnwrapPtr().GetBit(i) {
			a.Id.UnwrapPtr().SetBit(i, target.GetBit(i))
			break
		}
	}
	tb.Log(a)
	tb.Log(b)
	tb.Log(target)
	qt.Assert(tb, qt.IsTrue(a.CloserThan(b, target)))
}

func BenchmarkDeterministicAddr(tb *testing.B) {
	ip := net.ParseIP("1.2.3.4")
	target := krpc.RandomNodeID().Int160()
	for tb.Loop() {
		a := AddrMaybeId{
			Addr: krpc.NodeAddr{
				IP:   ip,
				Port: rand.IntN(math.MaxUint16 + 1),
			}.ToNodeAddrPort(),
		}
		b := AddrMaybeId{
			Addr: krpc.NodeAddr{
				IP:   ip,
				Port: rand.IntN(math.MaxUint16 + 1),
			}.ToNodeAddrPort(),
		}
		if first := a.CloserThan(b, target); a.CloserThan(b, target) != first {
			tb.Fatal("not deterministic")
		}
	}
}
