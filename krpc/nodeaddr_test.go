package krpc

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	IPv4    = net.IPv4
	ParseIP = net.ParseIP
)

func TestUnmarshalNodeAddr(t *testing.T) {
	var na NodeAddr
	require.NoError(t, na.UnmarshalBinary([]byte("\x01\x02\x03\x04\x05\x06")))
	assert.EqualValues(t, "1.2.3.4", na.IP.String())
}

var naEqualTests = []struct {
	a, b NodeAddr
	out  bool
}{
	{NewNodeIPAddr(IPv4(172, 16, 1, 1), 11), NewNodeIPAddr(IPv4(172, 16, 1, 1), 11), true},
	{NewNodeIPAddr(IPv4(172, 16, 1, 1), 11), NewNodeIPAddr(IPv4(172, 16, 1, 1), 22), false},
	{NewNodeIPAddr(IPv4(172, 16, 1, 1), 11), NewNodeIPAddr(IPv4(192, 168, 0, 3), 11), false},
	{NewNodeIPAddr(IPv4(172, 16, 1, 1), 11), NewNodeIPAddr(IPv4(192, 168, 0, 3), 22), false},
	{NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 11), NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 11), true},
	{NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 11), NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 22), false},
	{NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 11), NewNodeIPAddr(ParseIP("fe80::420b"), 11), false},
	{NewNodeIPAddr(ParseIP("2001:db8:1:2::1"), 11), NewNodeIPAddr(ParseIP("fe80::420b"), 22), false},
	{NewNodeI2PAddr(makeTestAddr32(dest32_2), 22), NewNodeI2PAddr(makeTestAddr32(dest32_2), 22), true},
	{NewNodeI2PAddr(makeTestAddr64(dest64_1), 77), NewNodeI2PAddr(makeTestAddr64(dest64_1), 77), true},
	{NewNodeI2PAddr(makeTestAddr32(dest32_1), 11), NewNodeI2PAddr(makeTestAddr32(dest32_2), 11), false},
	{NewNodeI2PAddr(makeTestAddr64(dest64_2), 11), NewNodeI2PAddr(makeTestAddr64(dest64_3), 11), false},
	{NewNodeI2PAddr(makeTestAddr32(dest32_3), 11), NewNodeI2PAddr(makeTestAddr64(dest64_3), 11), false},
	{NewNodeI2PAddr(makeTestAddr32(dest32_3), 11), NewNodeI2PAddr(makeTestAddr64(dest64_1), 11), false},
}

func TestNodeAddrEqual(t *testing.T) {
	for _, tc := range naEqualTests {
		out := tc.a.Equal(tc.b)
		if out != tc.out {
			t.Errorf("NodeAddr(%v).Equal(%v) = %v, want %v", tc.a, tc.b, out, tc.out)
		}
	}
}
