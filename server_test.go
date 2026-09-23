package dht

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"
	"time"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	"github.com/go-quicktest/qt"
	"golang.org/x/time/rate"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
)

func TestPutGet(t *testing.T) {
	l := log.Default.WithNames(t.Name())
	s1 := newServer(t, l.WithNames("s1"))
	s2 := newServer(t, l.WithNames("s2"))

	s2Addr := NewAddr(s2.Addr())

	immuItem, err := bep44.NewItem("Hello World! immu", nil, 1, 1, nil)
	qt.Assert(t, qt.IsNil(err))

	// send get request to s2, we need a write token to put data
	qr := s1.Get(context.TODO(), s2Addr, immuItem.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNotNil(qr.Reply.R))
	qt.Assert(t, qt.IsNotNil(qr.Reply.R.Token))

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, immuItem.ToPut(), *qr.Reply.R.Token, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))

	qr = s1.Get(context.TODO(), s2Addr, immuItem.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	var vStr string // heueahea
	qt.Assert(t, qt.IsNil(bencode.Unmarshal(qr.Reply.R.V, &vStr)))
	qt.Assert(t, qt.Equals(vStr, "Hello World! immu"))

	_, priv, err := ed25519.GenerateKey(nil)
	qt.Assert(t, qt.IsNil(err))

	mutItem, err := bep44.NewItem("Hello World!", []byte("s1"), 1, 1, priv)
	qt.Assert(t, qt.IsNil(err))

	// send get request to s2, we need a write token to put data
	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNotNil(qr.Reply.R))

	mutToken := qr.Reply.R.Token
	qt.Assert(t, qt.IsNotNil(mutToken))

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, mutItem.ToPut(), *mutToken, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNil(bencode.Unmarshal(qr.Reply.R.V, &vStr)))
	qt.Assert(t, qt.Equals(vStr, "Hello World!"))

	ii, err := s2.store.Get(immuItem.Target())
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(ii.V, "Hello World! immu"))

	mi, err := s2.store.Get(mutItem.Target())
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(mi.V, "Hello World!"))

	// change mutable item
	ok := mutItem.Modify("Bye World!", priv)
	qt.Assert(t, qt.IsTrue(ok))
	qr = s1.Put(context.TODO(), s2Addr, mutItem.ToPut(), *mutToken, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))

	mi, err = s2.store.Get(mutItem.Target())
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(mi.V, "Bye World!"))

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNil(bencode.Unmarshal(qr.Reply.R.V, &vStr)))
	qt.Assert(t, qt.Equals(vStr, "Bye World!"))

	seqPtr := new(int64)
	*seqPtr = 3
	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), seqPtr, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNil(qr.Reply.R.V))
	qt.Assert(t, qt.Equals(*qr.Reply.R.Seq, int64(2)))
}

// Queries without an arguments dict must get a protocol error rather than crash the server.
func TestQueryWithoutArguments(t *testing.T) {
	s := newServer(t, log.Default.WithNames(t.Name()))
	conn := mustListen("localhost:0")
	defer conn.Close()
	for _, q := range []string{"get_peers", "find_node", "announce_peer", "put", "get"} {
		t.Run(q, func(t *testing.T) {
			b := bencode.MustMarshal(map[string]string{"t": q, "y": "q", "q": q})
			_, err := conn.WriteTo(b, s.Addr())
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.IsNil(conn.SetReadDeadline(time.Now().Add(5*time.Second))))
			buf := make([]byte, 0x10000)
			n, _, err := conn.ReadFrom(buf)
			qt.Assert(t, qt.IsNil(err))
			var m krpc.Msg
			qt.Assert(t, qt.IsNil(bencode.Unmarshal(buf[:n], &m)))
			qt.Check(t, qt.Equals(m.T, q))
			qt.Assert(t, qt.IsNotNil(m.Error()))
			qt.Check(t, qt.Equals(m.Error().Code, krpc.ErrorCodeProtocolError))
		})
	}
}

// BEP 44 immutable puts carry no seq; only mutable puts require one.
func TestPutSeqOnlyRequiredForMutable(t *testing.T) {
	l := log.Default.WithNames(t.Name())
	s1 := newServer(t, l.WithNames("s1"))
	s2 := newServer(t, l.WithNames("s2"))
	s2Addr := NewAddr(s2.Addr())

	item, err := bep44.NewItem("immutable", nil, 0, 0, nil)
	qt.Assert(t, qt.IsNil(err))
	qr := s1.Get(context.TODO(), s2Addr, item.Target(), nil, QueryRateLimiting{})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	qt.Assert(t, qt.IsNotNil(qr.Reply.R))
	qt.Assert(t, qt.IsNotNil(qr.Reply.R.Token))
	token := *qr.Reply.R.Token

	qr = s1.Query(context.TODO(), s2Addr, "put", QueryInput{
		MsgArgs: krpc.MsgArgs{Token: token, V: item.V},
	})
	qt.Assert(t, qt.IsNil(qr.ToError()))
	stored, err := s2.store.Get(item.Target())
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(stored.V, "immutable"))

	qr = s1.Query(context.TODO(), s2Addr, "put", QueryInput{
		MsgArgs: krpc.MsgArgs{Token: token, V: "mutable", K: [32]byte{1}},
	})
	qt.Assert(t, qt.IsNotNil(qr.Reply.Error()))
	qt.Check(t, qt.Equals(qr.Reply.Error().Code, krpc.ErrorCodeProtocolError))
}

// find_node and get replies must contain nodes near the queried target, not near the (unset) info
// hash.
func TestSetReturnNodesUsesTarget(t *testing.T) {
	cfg := NewDefaultServerConfig()
	cfg.Conn = mustListen("localhost:0")
	cfg.NodeId = krpc.ID{19: 1}
	cfg.NoSecurity = true
	s, err := NewServer(cfg)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(s.Close)
	target := s.id
	target.SetBit(0, true)
	ids := []int160.T{target}
	// More than K candidates: a lookup near zero must exclude target, whereas a lookup
	// for target must include it. Do not depend on bucket traversal or response order.
	for bit := 150; bit < 158; bit++ {
		id := s.id
		id.SetBit(bit, true)
		ids = append(ids, id)
	}
	s.mu.Lock()
	for i, id := range ids {
		n := &node{
			nodeKey: nodeKey{Id: id, Addr: NewAddr(&net.UDPAddr{
				IP: net.IPv4(1, 2, 3, byte(i)), Port: 1,
			})},
			lastGotResponse: time.Now(),
		}
		qt.Assert(t, qt.IsNil(s.table.addNode(n)))
	}
	var r krpc.Return
	s.setReturnNodes(&r, target.AsByteArray(), []krpc.Want{krpc.WantNodes},
		NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}))
	s.mu.Unlock()
	for _, n := range r.Nodes {
		if n.ID == target.AsByteArray() {
			return
		}
	}
	t.Fatal("response omitted the exact queried target in favor of nodes near zero")
}

func newServer(t *testing.T, l log.Logger) *Server {
	cfg := NewDefaultServerConfig()
	cfg.WaitToReply = true
	// The default limiter is shared process-wide, and error replies don't wait for it.
	cfg.SendLimiter = rate.NewLimiter(rate.Inf, 0)
	cfg.Conn = mustListen("localhost:0")
	cfg.Logger = l
	s, err := NewServer(cfg)
	if err != nil {
		panic(err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return s
}
