package dht

import (
	"net"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

func TestTokenServer(t *testing.T) {
	addr1 := NewAddr(&net.UDPAddr{
		IP: []byte{1, 2, 3, 4},
	})
	addr2 := NewAddr(&net.UDPAddr{
		IP: []byte{1, 2, 3, 3},
	})
	ts := tokenServer{
		secret:           []byte("42"),
		interval:         5 * time.Minute,
		maxIntervalDelta: 2,
	}
	tok := ts.CreateToken(addr1)
	qt.Check(t, qt.HasLen(tok, 20))
	qt.Check(t, qt.IsTrue(ts.ValidToken(tok, addr1)))
	qt.Check(t, qt.IsFalse(ts.ValidToken(tok[1:], addr1)))
	qt.Check(t, qt.IsFalse(ts.ValidToken(tok, addr2)))
	func() {
		ts0 := ts
		ts0.secret = nil
		qt.Check(t, qt.IsFalse(ts0.ValidToken(tok, addr1)))
	}()
	now := time.Now()
	setTime := func(t time.Time) {
		ts.timeNow = func() time.Time {
			return t
		}
	}
	setTime(now)
	tok = ts.CreateToken(addr1)
	qt.Check(t, qt.IsTrue(ts.ValidToken(tok, addr1)))
	setTime(time.Time{})
	qt.Check(t, qt.IsFalse(ts.ValidToken(tok, addr1)))
	setTime(now.Add(-5 * time.Minute))
	qt.Check(t, qt.IsFalse(ts.ValidToken(tok, addr1)))
	setTime(now)
	qt.Check(t, qt.IsTrue(ts.ValidToken(tok, addr1)))
	setTime(now.Add(5 * time.Minute))
	qt.Check(t, qt.IsTrue(ts.ValidToken(tok, addr1)))
	setTime(now.Add(2 * 5 * time.Minute))
	qt.Check(t, qt.IsTrue(ts.ValidToken(tok, addr1)))
	setTime(now.Add(3 * 5 * time.Minute))
	qt.Check(t, qt.IsFalse(ts.ValidToken(tok, addr1)))
}
