package dht

import (
	"crypto/sha1"
	"encoding/binary"
	"time"
)

// Manages creation and validation of tokens issued to querying nodes.
type tokenServer struct {
	// Something only we know that peers can't guess, so they can't deduce valid tokens.
	secret []byte
	// How long between token changes.
	interval time.Duration
	// How many intervals may pass between the current interval, and one used to generate a token before it is invalid.
	maxIntervalDelta int
	timeNow          func() time.Time
}

func (ts *tokenServer) CreateToken(addr Addr) string {
	return ts.createToken(addr, ts.getTimeNow())
}

func (ts *tokenServer) createToken(addr Addr, t time.Time) string {
	h := sha1.New()
	ip := addr.IP().To16()
	if len(ip) != 16 {
		panic(ip)
	}
	h.Write(ip)
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(t.UnixNano()/int64(ts.interval))))
	h.Write(ts.secret)
	return string(h.Sum(nil))
}

func (ts *tokenServer) ValidToken(token string, addr Addr) bool {
	t := ts.getTimeNow()
	for range ts.maxIntervalDelta + 1 {
		if ts.createToken(addr, t) == token {
			return true
		}
		t = t.Add(-ts.interval)
	}
	return false
}

func (ts *tokenServer) getTimeNow() time.Time {
	if ts.timeNow == nil {
		return time.Now()
	}
	return ts.timeNow()
}
