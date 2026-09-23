package main

import (
	"os"
	"strings"
	"testing"

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
