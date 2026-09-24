package getput

import (
	"context"
	"crypto/ed25519"
	"crypto/sha1"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/go-quicktest/qt"
)

func numTraversalGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), "traversal.(*Operation).run(")
		}
		buf = make([]byte, 2*len(buf))
	}
}

func assertTraversalGoroutines(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for numTraversalGoroutines() != want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	qt.Assert(t, qt.Equals(numTraversalGoroutines(), want))
}

// A traversal must not outlive a failure to get starting nodes.
func TestStartingNodesErrorDoesNotLeak(t *testing.T) {
	conn, err := net.ListenPacket("udp", "localhost:0")
	qt.Assert(t, qt.IsNil(err))
	cfg := dht.NewDefaultServerConfig()
	cfg.Conn = conn
	cfg.StartingNodes = func() ([]dht.Addr, error) { return nil, errors.New("no starting nodes") }
	s, err := dht.NewServer(cfg)
	qt.Assert(t, qt.IsNil(err))
	defer s.Close()
	before := numTraversalGoroutines()

	_, _, err = Get(context.Background(), bep44.Target{1}, s, nil, nil)
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)

	_, err = Put(context.Background(), krpc.ID{1}, s, nil, func(int64) bep44.Put { return bep44.Put{} })
	qt.Assert(t, qt.IsNotNil(err))
	assertTraversalGoroutines(t, before)
}

func TestCancelActiveTraversal(t *testing.T) {
	for _, operation := range []string{"get", "put"} {
		t.Run(operation, func(t *testing.T) {
			peer, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(func() { _ = peer.Close() })
			conn, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			s, err := dht.NewServer(&dht.ServerConfig{
				Conn: conn, NoSecurity: true,
				QueryResendDelay: func() time.Duration { return time.Hour },
				StartingNodes: func() ([]dht.Addr, error) {
					return []dht.Addr{dht.NewAddr(peer.LocalAddr())}, nil
				},
			})
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(s.Close)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				if operation == "get" {
					_, _, err := Get(ctx, bep44.Target{1}, s, nil, nil)
					done <- err
				} else {
					_, err := Put(ctx, krpc.ID{1}, s, nil, func(int64) bep44.Put { return bep44.Put{} })
					done <- err
				}
			}()
			qt.Assert(t, qt.IsNil(peer.SetReadDeadline(time.Now().Add(2*time.Second))))
			var packet [1500]byte
			_, _, err = peer.ReadFrom(packet[:])
			qt.Assert(t, qt.IsNil(err))
			cancel()
			select {
			case err := <-done:
				qt.Assert(t, qt.IsTrue(errors.Is(err, context.Canceled)))
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled traversal did not return")
			}
			qt.Assert(t, qt.Equals(s.Stats().OutstandingTransactions, 0))
		})
	}
}

func TestVerifiedResultRejectsAbsentMutableValue(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	var publicKey [ed25519.PublicKeySize]byte
	copy(publicKey[:], privateKey.Public().(ed25519.PublicKey))
	seq := int64(1)
	var signature [ed25519.SignatureSize]byte
	copy(signature[:], bep44.Sign(privateKey, nil, seq, nil))
	response := &krpc.Return{
		Bep44Return: krpc.Bep44Return{
			K:   publicKey,
			Sig: signature,
			Seq: &seq,
		},
	}

	_, ok := verifiedResult(response, bep44.MakeMutableTarget(publicKey, nil), nil)
	qt.Assert(t, qt.IsFalse(ok))
}

func TestVerifiedResultAcceptsEmptyBencodedValue(t *testing.T) {
	var response krpc.Msg
	qt.Assert(t, qt.IsNil(bencode.Unmarshal([]byte("d1:rd1:v0:ee"), &response)))
	qt.Assert(t, qt.IsNotNil(response.R))

	target := sha1.Sum([]byte("0:"))
	got, ok := verifiedResult(response.R, target, nil)
	qt.Assert(t, qt.IsTrue(ok))
	qt.Check(t, qt.Equals(string(got.V), "0:"))
}

func TestGetRejectsResponseWithoutValue(t *testing.T) {
	peer, err := net.ListenPacket("udp", "127.0.0.1:0")
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(func() { _ = peer.Close() })
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	qt.Assert(t, qt.IsNil(err))
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn: conn, NoSecurity: true,
		QueryResendDelay: func() time.Duration { return time.Hour },
		StartingNodes: func() ([]dht.Addr, error) {
			return []dht.Addr{dht.NewAddr(peer.LocalAddr())}, nil
		},
	})
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(s.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	type getResult struct {
		value GetResult
		err   error
	}
	done := make(chan getResult, 1)
	go func() {
		target := sha1.Sum(nil)
		value, _, err := Get(ctx, target, s, nil, nil)
		done <- getResult{value: value, err: err}
	}()

	qt.Assert(t, qt.IsNil(peer.SetReadDeadline(time.Now().Add(5*time.Second))))
	var packet [1500]byte
	n, source, err := peer.ReadFrom(packet[:])
	qt.Assert(t, qt.IsNil(err))
	var request krpc.Msg
	qt.Assert(t, qt.IsNil(bencode.Unmarshal(packet[:n], &request)))
	qt.Assert(t, qt.Equals(request.Q, "get"))
	reply, err := bencode.Marshal(krpc.Msg{
		T: request.T, Y: krpc.YResponse,
		R: &krpc.Return{ID: krpc.ID{1}},
	})
	qt.Assert(t, qt.IsNil(err))
	_, err = peer.WriteTo(reply, source)
	qt.Assert(t, qt.IsNil(err))

	select {
	case result := <-done:
		qt.Assert(t, qt.IsNotNil(result.err))
		qt.Check(t, qt.IsFalse(errors.Is(result.err, context.DeadlineExceeded)))
		qt.Check(t, qt.IsNil(result.value.V))
	case <-ctx.Done():
		t.Fatal("Get did not finish after response without a value")
	}
}

func TestPutReportsRemoteOutcome(t *testing.T) {
	for _, test := range []struct {
		name        string
		returnToken bool
		rejectPut   bool
	}{
		{name: "no eligible nodes"},
		{name: "all nodes reject", returnToken: true, rejectPut: true},
		{name: "accepted", returnToken: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			peer, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(func() { _ = peer.Close() })
			conn, err := net.ListenPacket("udp", "127.0.0.1:0")
			qt.Assert(t, qt.IsNil(err))
			s, err := dht.NewServer(&dht.ServerConfig{
				Conn: conn, NoSecurity: true,
				QueryResendDelay: func() time.Duration { return time.Hour },
				StartingNodes: func() ([]dht.Addr, error) {
					return []dht.Addr{dht.NewAddr(peer.LocalAddr())}, nil
				},
			})
			qt.Assert(t, qt.IsNil(err))
			t.Cleanup(s.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				_, err := Put(ctx, krpc.ID{1}, s, nil, func(int64) bep44.Put {
					return bep44.Put{V: "value"}
				})
				done <- err
			}()

			var responseErr error
			for _, expectedQuery := range []string{"get", "put"} {
				if expectedQuery == "put" && !test.returnToken {
					break
				}
				qt.Assert(t, qt.IsNil(peer.SetReadDeadline(time.Now().Add(5*time.Second))))
				var packet [1500]byte
				n, source, err := peer.ReadFrom(packet[:])
				qt.Assert(t, qt.IsNil(err))
				var request krpc.Msg
				qt.Assert(t, qt.IsNil(bencode.Unmarshal(packet[:n], &request)))
				qt.Assert(t, qt.Equals(request.Q, expectedQuery))

				var reply krpc.Msg
				reply.T = request.T
				if expectedQuery == "get" {
					reply.Y = krpc.YResponse
					returned := &krpc.Return{ID: krpc.ID{1}}
					if test.returnToken {
						token := "write-token"
						returned.Token = &token
					}
					reply.R = returned
				} else if test.rejectPut {
					reply.Y = krpc.YError
					reply.E = &krpc.Error{
						Code: krpc.ErrorCodeInvalidSignature,
						Msg:  "invalid signature",
					}
				} else {
					reply.Y = krpc.YResponse
					reply.R = &krpc.Return{ID: krpc.ID{1}}
				}
				encoded, err := bencode.Marshal(reply)
				qt.Assert(t, qt.IsNil(err))
				_, err = peer.WriteTo(encoded, source)
				qt.Assert(t, qt.IsNil(err))
			}
			select {
			case err := <-done:
				if test.returnToken && !test.rejectPut {
					qt.Assert(t, qt.IsNil(err))
				} else {
					qt.Assert(t, qt.IsNotNil(err))
				}
				if test.rejectPut {
					var remoteErr *krpc.Error
					qt.Assert(t, qt.IsTrue(errors.As(err, &remoteErr)))
					qt.Check(t, qt.Equals(remoteErr.Code, krpc.ErrorCodeInvalidSignature))
				}
			case <-ctx.Done():
				responseErr = ctx.Err()
			}
			qt.Assert(t, qt.IsNil(responseErr))
		})
	}
}
