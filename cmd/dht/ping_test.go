package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
)

// A response without an id is a successful KRPC reply. SenderID is nil, and ping must not
// dereference it. PingQueryInput already skips a nil id.
func TestPingResponseWithoutID(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		b := make([]byte, 1500)
		n, addr, err := pc.ReadFrom(b)
		if err != nil {
			return
		}
		var q krpc.Msg
		if err := bencode.Unmarshal(b[:n], &q); err != nil {
			return
		}
		rb, err := bencode.Marshal(krpc.Msg{Y: krpc.YResponse, T: q.T})
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(rb, addr)
	}()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:             conn,
		NoSecurity:       true,
		QueryResendDelay: func() time.Duration { return time.Hour },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	err = ping(context.Background(), pingArgs{
		Network: "udp",
		Nodes:   []string{pc.LocalAddr().String()},
	}, s)
	if err != nil {
		t.Fatal(err)
	}
}

func newPingLifecycleTestServer(t *testing.T) *dht.Server {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:             conn,
		NoSecurity:       true,
		QueryResendDelay: func() time.Duration { return time.Hour },
	})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		deadline := time.Now().Add(time.Second)
		for s.Stats().OutstandingTransactions != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	})
	return s
}

func newPingBlackhole(t *testing.T) (net.PacketConn, <-chan struct{}) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	gotQuery := make(chan struct{}, 1)
	go func() {
		b := make([]byte, 1500)
		for {
			if _, _, err := pc.ReadFrom(b); err != nil {
				return
			}
			select {
			case gotQuery <- struct{}{}:
			default:
			}
		}
	}()
	return pc, gotQuery
}

func waitForPingQuery(t *testing.T, gotQuery <-chan struct{}) {
	t.Helper()
	select {
	case <-gotQuery:
	case <-time.After(time.Second):
		t.Fatal("ping query was not sent")
	}
}

func TestPingTimeoutCancelsAndJoinsWorkers(t *testing.T) {
	pc, gotQuery := newPingBlackhole(t)
	s := newPingLifecycleTestServer(t)
	done := make(chan error, 1)
	go func() {
		done <- ping(context.Background(), pingArgs{
			Network: "udp",
			Timeout: 25 * time.Millisecond,
			Nodes:   []string{pc.LocalAddr().String()},
		}, s)
	}()
	waitForPingQuery(t, gotQuery)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ping returned nil error after timeout")
		}
	case <-time.After(time.Second):
		t.Fatal("ping did not return after its timeout")
	}
	if got := s.Stats().OutstandingTransactions; got != 0 {
		t.Errorf("outstanding transactions after ping returned = %d, want 0", got)
	}
}

func TestPingResolveErrorCancelsAndJoinsWorkers(t *testing.T) {
	pc, gotQuery := newPingBlackhole(t)
	s := newPingLifecycleTestServer(t)

	oldResolver := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = oldResolver })
	resolverStarted := make(chan struct{})
	releaseResolver := make(chan struct{})
	var resolverStart sync.Once
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			resolverStart.Do(func() { close(resolverStarted) })
			<-releaseResolver
			return nil, errors.New("injected DNS failure")
		},
	}

	done := make(chan error, 1)
	released, finished := false, false
	t.Cleanup(func() {
		if !released {
			close(releaseResolver)
		}
		if !finished {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("ping did not stop during test cleanup")
			}
		}
	})
	go func() {
		done <- ping(context.Background(), pingArgs{
			Network: "udp",
			Nodes: []string{
				pc.LocalAddr().String(),
				"resolve-failure.invalid:6881",
			},
		}, s)
	}()
	select {
	case <-resolverStarted:
	case <-time.After(time.Second):
		t.Fatal("ping did not reach the controlled DNS lookup")
	}
	waitForPingQuery(t, gotQuery)
	close(releaseResolver)
	released = true
	select {
	case err := <-done:
		finished = true
		if err == nil {
			t.Fatal("ping returned nil error after address resolution failed")
		}
	case <-time.After(time.Second):
		t.Fatal("ping did not return after address resolution failed")
	}
	if got := s.Stats().OutstandingTransactions; got != 0 {
		t.Errorf("outstanding transactions after ping returned = %d, want 0", got)
	}
}

func TestPingContextCancellationCancelsAndJoinsWorkers(t *testing.T) {
	pc, gotQuery := newPingBlackhole(t)
	s := newPingLifecycleTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ping(ctx, pingArgs{
			Network: "udp",
			Timeout: time.Hour,
			Nodes:   []string{pc.LocalAddr().String()},
		}, s)
	}()
	waitForPingQuery(t, gotQuery)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ping error after context cancellation = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ping did not return after context cancellation")
	}
	if got := s.Stats().OutstandingTransactions; got != 0 {
		t.Errorf("outstanding transactions after ping returned = %d, want 0", got)
	}
}

func TestPingTimeoutIncludesDNSResolution(t *testing.T) {
	s := newPingLifecycleTestServer(t)
	oldResolver := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = oldResolver })
	resolverStarted := make(chan struct{})
	releaseResolver := make(chan struct{})
	var once sync.Once
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			once.Do(func() { close(resolverStarted) })
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-releaseResolver:
				return nil, errors.New("resolver released by cleanup")
			}
		},
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		close(releaseResolver)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("ping did not release its DNS lookup")
		}
	})
	go func() {
		defer close(finished)
		done <- ping(context.Background(), pingArgs{
			Network: "udp",
			Timeout: 25 * time.Millisecond,
			Nodes:   []string{"blocked-resolution.invalid:6881"},
		}, s)
	}()
	select {
	case <-resolverStarted:
	case <-time.After(time.Second):
		t.Fatal("ping did not start DNS resolution")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ping DNS timeout error = %v, want context deadline", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("ping timeout did not include DNS resolution")
	}
}

func TestContextResolverPreservesLiteralAddressSemantics(t *testing.T) {
	for _, tc := range []struct{ network, address string }{
		{"udp", "127.0.0.1:6881"},
		{"udp6", "[::1]:6881"},
		{"udp6", "[fe80::1%en0]:6881"},
		{"udp", ":6881"},
		{"udp4", "[::1]:6881"},
		{"invalid", "127.0.0.1:6881"},
	} {
		t.Run(tc.network+"/"+tc.address, func(t *testing.T) {
			want, wantErr := net.ResolveUDPAddr(tc.network, tc.address)
			got, gotErr := resolveUDPAddr(context.Background(), tc.network, tc.address)
			if (wantErr != nil) != (gotErr != nil) {
				t.Fatalf("resolution errors: got %v, standard resolver %v", gotErr, wantErr)
			}
			if wantErr == nil && (got.Port != want.Port || got.Zone != want.Zone || !got.IP.Equal(want.IP)) {
				t.Fatalf("resolved %v, want %v", got, want)
			}
		})
	}
}
