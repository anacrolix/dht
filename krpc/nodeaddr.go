package krpc

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"github.com/anacrolix/torrent/bencode"
)

// This will be deprecated in favour of NodeAddrPort.
type NodeAddr struct {
	IP   net.IP
	Port int
}

func (me *NodeAddr) FromAddrPort(f netip.AddrPort) {
	me.IP = f.Addr().AsSlice()
	me.Port = int(f.Port())
}

func (me NodeAddr) ToNodeAddrPort() NodeAddrPort {
	addr, _ := netip.AddrFromSlice(me.IP)
	return NodeAddrPort{netip.AddrPortFrom(addr, uint16(me.Port))}
}

// A zero Port is taken to mean no port provided, per BEP 7.
func (me NodeAddr) String() string {
	return net.JoinHostPort(me.IP.String(), strconv.FormatInt(int64(me.Port), 10))
}

func (me *NodeAddr) UnmarshalBinary(b []byte) error {
	switch len(b) {
	case net.IPv4len + 2, net.IPv6len + 2:
	default:
		return fmt.Errorf("unmarshal NodeAddr from %d bytes: need 6-byte IPv4 or 18-byte IPv6 compact address", len(b))
	}
	me.IP = make(net.IP, len(b)-2)
	copy(me.IP, b[:len(b)-2])
	me.Port = int(binary.BigEndian.Uint16(b[len(b)-2:]))
	return nil
}

func (me *NodeAddr) UnmarshalBencode(b []byte) (err error) {
	var _b []byte
	err = bencode.Unmarshal(b, &_b)
	if err != nil {
		return
	}
	return me.UnmarshalBinary(_b)
}

func (me NodeAddr) MarshalBinary() ([]byte, error) {
	switch len(me.IP) {
	case net.IPv4len, net.IPv6len:
	default:
		return nil, fmt.Errorf("marshal NodeAddr with %d IP bytes: need 4 or 16", len(me.IP))
	}
	b := make([]byte, 0, len(me.IP)+2)
	b = append(b, me.IP...)
	return binary.BigEndian.AppendUint16(b, uint16(me.Port)), nil
}

func (me NodeAddr) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me NodeAddr) UDP() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   me.IP,
		Port: me.Port,
	}
}

func (me *NodeAddr) FromUDPAddr(ua *net.UDPAddr) {
	me.IP = ua.IP
	me.Port = ua.Port
}

func (me NodeAddr) Equal(x NodeAddr) bool {
	return me.IP.Equal(x.IP) && me.Port == x.Port
}
