package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
)

func TestMutableCommandsRejectBadKeyLengths(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"derive short", []string{"dht", "derive-put-target", "mutable", "--key", "00"}},
		{"derive long", []string{"dht", "derive-put-target", "mutable", "--key", strings.Repeat("00", 33)}},
		{"put missing", []string{"dht", "put", "--mutable", "--strings", "value"}},
		{"put short", []string{"dht", "put", "--mutable", "--key", "00", "--strings", "value"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			if got := runMain(); got != 2 {
				t.Fatalf("runMain exit = %d, want usage error 2", got)
			}
		})
	}
}

func TestReturnedNodeAddressesRejectsMalformedAndErrorReplies(t *testing.T) {
	tests := []struct {
		name  string
		reply krpc.Msg
	}{
		{"missing return dictionary", krpc.Msg{Y: krpc.YResponse}},
		{"protocol error", krpc.Msg{
			Y: krpc.YError,
			E: &krpc.Error{Code: krpc.ErrorCodeProtocolError, Msg: "bad request"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if nodes, err := returnedNodeAddresses(dht.QueryResult{Reply: tc.reply}); err == nil {
				t.Fatalf("returned nodes %v from malformed reply", nodes)
			}
		})
	}
}

func TestQueryCommandsCancelDNS(t *testing.T) {
	for _, command := range []string{"query", "ping-nodes"} {
		t.Run(command, func(t *testing.T) {
			oldArgs, oldResolver := os.Args, net.DefaultResolver
			t.Cleanup(func() { os.Args, net.DefaultResolver = oldArgs, oldResolver })
			os.Args = []string{"dht", command, "blocked-command.invalid:6881", "find_node"}
			started := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			net.DefaultResolver = &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-release:
						return nil, errors.New("DNS released by test cleanup")
					}
				},
			}
			done := make(chan int, 1)
			finished := make(chan struct{})
			t.Cleanup(func() {
				close(release)
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
					t.Error("command did not stop during cleanup")
				}
			})
			go func() {
				defer close(finished)
				done <- runMain()
			}()
			select {
			case <-started:
			case code := <-done:
				t.Fatalf("command exited with %d before DNS started", code)
			case <-time.After(5 * time.Second):
				t.Fatal("command did not reach DNS")
			}
			process, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if err := process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case code := <-done:
				if code != 2 {
					t.Fatalf("command exit = %d, want cancellation error exit 2", code)
				}
			case <-time.After(time.Second):
				t.Fatal("command ignored cancellation during DNS resolution")
			}
		})
	}
}
