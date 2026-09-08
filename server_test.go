package dht

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	"github.com/go-quicktest/qt"

	"github.com/anacrolix/dht/v2/bep44"
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

func newServer(t *testing.T, l log.Logger) *Server {
	cfg := NewDefaultServerConfig()
	cfg.WaitToReply = true

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
