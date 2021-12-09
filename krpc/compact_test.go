package krpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalSlice(t *testing.T) {
	var data CompactIPv4NodeInfo
	err := data.UnmarshalBencode([]byte("52:" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x02\x03\x04\x05\x06" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x03\x04\x05\x06\x07"))
	require.NoError(t, err)
	require.Len(t, data, 2)
	assert.Equal(t, "1.2.3.4", data[0].Addr.IP.String())
	assert.Equal(t, "2.3.4.5", data[1].Addr.IP.String())
}

var nodeAddrIndexTests4 = []struct {
	v   CompactIPv4NodeAddrs
	a   NodeAddr
	out int
}{
	{[]NodeAddr{{IPv4(172, 16, 1, 1), 11}, {IPv4(192, 168, 0, 3), 11}}, NodeAddr{IPv4(172, 16, 1, 1), 11}, 0},
	{[]NodeAddr{{IPv4(172, 16, 1, 1), 11}, {IPv4(192, 168, 0, 3), 11}}, NodeAddr{IPv4(192, 168, 0, 3), 11}, 1},
	{[]NodeAddr{{IPv4(172, 16, 1, 1), 11}, {IPv4(192, 168, 0, 3), 11}}, NodeAddr{IPv4(127, 0, 0, 1), 11}, -1},
	{[]NodeAddr{}, NodeAddr{IPv4(127, 0, 0, 1), 11}, -1},
	{[]NodeAddr{}, NodeAddr{}, -1},
}

func TestNodeAddrIndex4(t *testing.T) {
	for _, tc := range nodeAddrIndexTests4 {
		out := tc.v.Index(tc.a)
		if out != tc.out {
			t.Errorf("CompactIPv4NodeAddrs(%v).Index(%v) = %v, want %v", tc.v, tc.a, out, tc.out)
		}
	}
}

var nodeAddrIndexTests6 = []struct {
	v   CompactIPv6NodeAddrs
	a   NodeAddr
	out int
}{
	{[]NodeAddr{{ParseIP("2001::1"), 11}, {ParseIP("4004::1"), 11}}, NodeAddr{ParseIP("2001::1"), 11}, 0},
	{[]NodeAddr{{ParseIP("2001::1"), 11}, {ParseIP("4004::1"), 11}}, NodeAddr{ParseIP("4004::1"), 11}, 1},
	{[]NodeAddr{{ParseIP("2001::1"), 11}, {ParseIP("4004::1"), 11}}, NodeAddr{ParseIP("::1"), 11}, -1},
	{[]NodeAddr{}, NodeAddr{ParseIP("::1"), 11}, -1},
	{[]NodeAddr{}, NodeAddr{}, -1},
}

func TestNodeAddrIndex6(t *testing.T) {
	for _, tc := range nodeAddrIndexTests6 {
		out := tc.v.Index(tc.a)
		if out != tc.out {
			t.Errorf("CompactIPv6NodeAddrs(%v).Index(%v) = %v, want %v", tc.v, tc.a, out, tc.out)
		}
	}
}
