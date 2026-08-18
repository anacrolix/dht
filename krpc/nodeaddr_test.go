package krpc

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	IPv4    = net.IPv4
	ParseIP = net.ParseIP
)

func TestNodeAddrBinaryRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		addr NodeAddr
	}{
		{"four-byte IPv4", NodeAddr{IP: net.IP{192, 0, 2, 1}, Port: 0}},
		{"mapped IPv4", NodeAddr{IP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 0, 2, 1}, Port: 65535}},
		{"global IPv6", NodeAddr{IP: net.IP{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Port: 6881}},
		{"link-local IPv6", NodeAddr{IP: net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Port: 1}},
		{"unspecified IPv6", NodeAddr{IP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Port: 0}},
		{"nil IP", NodeAddr{IP: nil, Port: 65535}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := test.addr.MarshalBinary()
			require.NoError(t, err)

			var got NodeAddr
			require.NoError(t, got.UnmarshalBinary(encoded))
			assert.True(t, bytes.Equal(got.IP, test.addr.IP), "IP = %x, want %x", got.IP, test.addr.IP)
			assert.Equal(t, test.addr.Port, got.Port)
		})
	}
}

func FuzzNodeAddrUnmarshalBinary(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1})
	f.Add([]byte{0, 0})
	f.Add([]byte{1, 2, 3, 4, 0x1a, 0xe1})
	f.Add([]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, b []byte) {
		original := NodeAddr{IP: net.IP{192, 0, 2, 1}, Port: 6881}
		got := NodeAddr{IP: bytes.Clone(original.IP), Port: original.Port}

		err := got.UnmarshalBinary(b)
		if len(b) < 2 {
			require.Error(t, err)
			assert.True(t, bytes.Equal(got.IP, original.IP), "receiver IP changed from %x to %x", original.IP, got.IP)
			assert.Equal(t, original.Port, got.Port)
			return
		}

		require.NoError(t, err)
		assert.True(t, bytes.Equal(got.IP, b[:len(b)-2]), "IP = %x, want %x", got.IP, b[:len(b)-2])
		assert.Equal(t, int(binary.BigEndian.Uint16(b[len(b)-2:])), got.Port)
	})
}

var naEqualTests = []struct {
	a, b NodeAddr
	out  bool
}{
	{NodeAddr{IPv4(172, 16, 1, 1), 11}, NodeAddr{IPv4(172, 16, 1, 1), 11}, true},
	{NodeAddr{IPv4(172, 16, 1, 1), 11}, NodeAddr{IPv4(172, 16, 1, 1), 22}, false},
	{NodeAddr{IPv4(172, 16, 1, 1), 11}, NodeAddr{IPv4(192, 168, 0, 3), 11}, false},
	{NodeAddr{IPv4(172, 16, 1, 1), 11}, NodeAddr{IPv4(192, 168, 0, 3), 22}, false},
	{NodeAddr{ParseIP("2001:db8:1:2::1"), 11}, NodeAddr{ParseIP("2001:db8:1:2::1"), 11}, true},
	{NodeAddr{ParseIP("2001:db8:1:2::1"), 11}, NodeAddr{ParseIP("2001:db8:1:2::1"), 22}, false},
	{NodeAddr{ParseIP("2001:db8:1:2::1"), 11}, NodeAddr{ParseIP("fe80::420b"), 11}, false},
	{NodeAddr{ParseIP("2001:db8:1:2::1"), 11}, NodeAddr{ParseIP("fe80::420b"), 22}, false},
}

func TestNodeAddrEqual(t *testing.T) {
	for _, tc := range naEqualTests {
		out := tc.a.Equal(tc.b)
		if out != tc.out {
			t.Errorf("NodeAddr(%v).Equal(%v) = %v, want %v", tc.a, tc.b, out, tc.out)
		}
	}
}
