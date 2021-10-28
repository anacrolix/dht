package dht

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/log"
	"github.com/stretchr/testify/require"
)

func TestPutGet(t *testing.T) {
	require := require.New(t)

	s1 := newServer(t)
	s2 := newServer(t)

	s2Addr := NewAddr(s2.Addr())

	immuItem, err := bep44.NewItem("Hello World! immu", nil, 1, 1, nil)
	require.NoError(err)

	// send get request to s2, we need a write token to put data
	qr := s1.Get(context.TODO(), s2Addr, immuItem.Target(), immuItem.Seq, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.NotNil(qr.Reply.R)
	require.NotNil(qr.Reply.R.Token)

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, immuItem.ToPut(), *qr.Reply.R.Token, QueryRateLimiting{})
	require.NoError(qr.ToError())

	qr = s1.Get(context.TODO(), s2Addr, immuItem.Target(), immuItem.Seq, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Equal("Hello World! immu", qr.Reply.R.V)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(err)

	mutItem, err := bep44.NewItem("Hello World!", []byte("s1"), 1, 1, priv)
	require.NoError(err)

	// send get request to s2, we need a write token to put data
	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), mutItem.Seq, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.NotNil(qr.Reply.R)

	mutToken := qr.Reply.R.Token
	require.NotNil(mutToken)

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, mutItem.ToPut(), *mutToken, QueryRateLimiting{})
	require.NoError(qr.ToError())

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), mutItem.Seq, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Equal("Hello World!", qr.Reply.R.V)

	ii, err := s2.store.Get(immuItem.Target())
	require.NoError(err)
	require.Equal("Hello World! immu", ii.V)

	mi, err := s2.store.Get(mutItem.Target())
	require.NoError(err)
	require.Equal("Hello World!", mi.V)

	//change mutable item
	ok := mutItem.Modify("Bye World!", priv)
	require.True(ok)
	qr = s1.Put(context.TODO(), s2Addr, mutItem.ToPut(), *mutToken, QueryRateLimiting{})
	require.NoError(qr.ToError())

	mi, err = s2.store.Get(mutItem.Target())
	require.NoError(err)
	require.Equal("Bye World!", mi.V)

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), mutItem.Seq, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Equal("Bye World!", qr.Reply.R.V)

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), 3, QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Nil(qr.Reply.R.V)
	require.Equal(int64(2), *qr.Reply.R.Seq)
}

func newServer(t *testing.T) *Server {
	cfg := NewDefaultServerConfig()
	conn1, err := net.ListenPacket("udp", "localhost:0")
	if err != nil {
		panic(err)
	}

	cfg.Conn = conn1
	cfg.Logger = log.Default.FilterLevel(log.Debug)
	s, err := NewServer(cfg)
	if err != nil {
		panic(err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return s
}
