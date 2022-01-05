package krpc

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalSlice(t *testing.T) {
	var data CompactNodeInfo
	err := data.UnmarshalBencode([]byte("52:" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x02\x03\x04\x05\x06" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x03\x04\x05\x06\x07"))
	require.NoError(t, err)
	require.Len(t, data, 2)
	assert.Equal(t, "1.2.3.4", data[0].Addr.Host())
	assert.Equal(t, "2.3.4.5", data[1].Addr.Host())
}

var testNodeIPAddr1 = NewNodeIPAddr(IPv4(172, 16, 1, 1), 11)
var testNodeIPAddr2 = NewNodeIPAddr(IPv4(192, 168, 0, 3), 11)
var testNodeIPAddr3 = NewNodeIPAddr(IPv4(127, 0, 0, 1), 11)

var nodeAddrIndexTests4 = []struct {
	v   CompactNodeAddrs
	a   NodeAddr
	out int
}{
	{[]NodeAddr{testNodeIPAddr1, testNodeIPAddr2}, testNodeIPAddr1, 0},
	{[]NodeAddr{testNodeIPAddr1, testNodeIPAddr2}, testNodeIPAddr2, 1},
	{[]NodeAddr{testNodeIPAddr1, testNodeIPAddr2}, testNodeIPAddr3, -1},
	{[]NodeAddr{}, testNodeIPAddr3, -1},
}

func TestNodeAddrIndex4(t *testing.T) {
	for _, tc := range nodeAddrIndexTests4 {
		out := tc.v.Index(tc.a)
		if out != tc.out {
			t.Errorf("CompactIPv4NodeAddrs(%v).Index(%v) = %v, want %v", tc.v, tc.a, out, tc.out)
		}
	}
}

var testNodeIPv6Addr1 = NewNodeIPAddr(ParseIP("2001::1"), 11)
var testNodeIPv6Addr2 = NewNodeIPAddr(ParseIP("4004::1"), 11)
var testNodeIPv6Addr3 = NewNodeIPAddr(ParseIP("::1"), 11)

var nodeAddrIndexTests6 = []struct {
	v   CompactIPv6NodeAddrs
	a   NodeAddr
	out int
}{
	{[]NodeAddr{testNodeIPv6Addr1, testNodeIPv6Addr2}, testNodeIPv6Addr1, 0},
	{[]NodeAddr{testNodeIPv6Addr1, testNodeIPv6Addr2}, testNodeIPv6Addr2, 1},
	{[]NodeAddr{testNodeIPv6Addr1, testNodeIPv6Addr2}, testNodeIPv6Addr3, -1},
	{[]NodeAddr{}, testNodeIPv6Addr3, -1},
}

func TestNodeAddrIndex6(t *testing.T) {
	for _, tc := range nodeAddrIndexTests6 {
		out := tc.v.Index(tc.a)
		if out != tc.out {
			t.Errorf("CompactIPv6NodeAddrs(%v).Index(%v) = %v, want %v", tc.v, tc.a, out, tc.out)
		}
	}
}

var testMarshalIPv4Addr1 = NewNodeIPAddr(net.IP{172, 16, 1, 1}, 3)
var testMarshalIPv4Addr2 = NewNodeIPAddr(net.IPv4(172, 16, 1, 1), 4)
var testMarshalIPv4Addr3 = NewNodeIPAddr(net.IPv4(172, 16, 1, 1), 5)
var testMarshalIPv4Addr4 = NewNodeIPAddr(net.IPv4(192, 168, 0, 3), 6)

var marshalIPv4SliceTests = []struct {
	in     CompactNodeAddrs
	out    []byte
	panics bool
}{
	{[]NodeAddr{testMarshalIPv4Addr1}, []byte{172, 16, 1, 1, 0, 3}, false},
	{[]NodeAddr{testMarshalIPv4Addr2}, []byte{172, 16, 1, 1, 0, 4}, false},
	{[]NodeAddr{testMarshalIPv4Addr3, testMarshalIPv4Addr4}, []byte{
		172, 16, 1, 1, 0, 5,
		192, 168, 0, 3, 0, 6,
	}, false},
	{[]NodeAddr{NewNodeIPAddr(ParseIP("2001::1"), 7)}, nil, true},
	{[]NodeAddr{NewNodeIPAddr(nil, 8)}, nil, true},
}

func TestMarshalCompactIPv4NodeAddrs(t *testing.T) {
	for _, tc := range marshalIPv4SliceTests {
		runFunc := assert.NotPanics
		if tc.panics {
			runFunc = assert.Panics
		}
		runFunc(t, func() {
			out, err := tc.in.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, tc.out, out, "for input %v, %v", tc.in)
		})
	}
}
