package dht

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"
	"testing"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/log"
	"github.com/stretchr/testify/require"
)

func TestPutGet(t *testing.T) {
	require := require.New(t)

	s1 := newServerFromPort(4580)
	s2 := newServerFromPort(4581)

	s2Addr := NewAddr(s2.Addr())

	immuItem, err := bep44.NewItem([]byte("Hello World! immu"), nil, 1, 1, nil)
	require.NoError(err)

	// send get request to s2, we need a write token to put data
	qr := s1.Get(context.TODO(), s2Addr, immuItem.Target(), QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.NotNil(qr.Reply.R)
	require.NotNil(qr.Reply.R.Token)

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, immuItem, *qr.Reply.R.Token, QueryRateLimiting{})
	require.NoError(qr.ToError())

	qr = s1.Get(context.TODO(), s2Addr, immuItem.Target(), QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Equal([]byte("Hello World! immu"), qr.Reply.R.V)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(err)

	mutItem, err := bep44.NewItem([]byte("Hello World!"), []byte("s1"), 1, 1, priv)
	require.NoError(err)

	// send get request to s2, we need a write token to put data
	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.NotNil(qr.Reply.R)

	mutToken := qr.Reply.R.Token
	require.NotNil(mutToken)

	// send put request to s2
	qr = s1.Put(context.TODO(), s2Addr, mutItem, *mutToken, QueryRateLimiting{})
	require.NoError(qr.ToError())

	qr = s1.Get(context.TODO(), s2Addr, mutItem.Target(), QueryRateLimiting{})
	require.NoError(qr.ToError())
	require.Equal([]byte("Hello World!"), qr.Reply.R.V)

	ii, err := s2.store.Get(immuItem.Target())
	require.NoError(err)
	require.Equal([]byte("Hello World! immu"), ii.V)

	mi, err := s2.store.Get(mutItem.Target())
	require.NoError(err)
	require.Equal([]byte("Hello World!"), mi.V)

	//change mutable item
	ok := mutItem.Modify([]byte("Bye World!"), priv)
	require.True(ok)
	qr = s1.Put(context.TODO(), s2Addr, mutItem, *mutToken, QueryRateLimiting{})
	require.NoError(qr.ToError())

	mi, err = s2.store.Get(mutItem.Target())
	require.NoError(err)
	require.Equal([]byte("Bye World!"), mi.V)
}

func newServerFromPort(port int) *Server {
	cfg := NewDefaultServerConfig()
	conn1, err := net.ListenPacket("udp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(err)
	}

	cfg.Conn = conn1
	cfg.Logger = log.Default.FilterLevel(log.Debug)
	s, err := NewServer(cfg)
	if err != nil {
		panic(err)
	}

	return s
}
