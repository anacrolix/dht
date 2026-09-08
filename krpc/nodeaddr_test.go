package krpc

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/go-quicktest/qt"
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
			qt.Assert(t, qt.IsNil(err))

			var got NodeAddr
			qt.Assert(t, qt.IsNil(got.UnmarshalBinary(encoded)))
			qt.Check(t, qt.IsTrue(bytes.Equal(got.IP, test.addr.IP)), qt.Commentf("IP = %x, want %x", got.IP, test.addr.IP))
			qt.Check(t, qt.Equals(got.Port, test.addr.Port))
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
			qt.Assert(t, qt.IsNotNil(err))
			qt.Check(t, qt.IsTrue(bytes.Equal(got.IP, original.IP)), qt.Commentf("receiver IP changed from %x to %x", original.IP, got.IP))
			qt.Check(t, qt.Equals(got.Port, original.Port))
			return
		}

		qt.Assert(t, qt.IsNil(err))
		qt.Check(t, qt.IsTrue(bytes.Equal(got.IP, b[:len(b)-2])), qt.Commentf("IP = %x, want %x", got.IP, b[:len(b)-2]))
		qt.Check(t, qt.Equals(got.Port, int(binary.BigEndian.Uint16(b[len(b)-2:]))))
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
