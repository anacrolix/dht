package krpc

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"

	"github.com/anacrolix/torrent/bencode"
)

const (
	IPNet = iota
	I2PNet
)

var networkType = IPNet

// Switch between IP Addresses and I2P destinations
// This determines how bencoded data is interpreted and unmarshaled
func SetNetworkType(netType int) {
	networkType = netType

	if networkType == I2PNet {
		nodeAddrNumBytes = i2pAddrSize
	}
}

const (
	IPV4Addr = iota
	IPV6Addr
	I2PAddr
)

type NodeAddr struct {
	IP         net.IP
	Port       int
	I2PAddress I2PDest
	hostType   int
}

func NewNodeIPAddr(ip net.IP, port int) NodeAddr {
	hostAddrType := IPV4Addr
	if ip.To4() == nil {
		hostAddrType = IPV6Addr
	}

	return NodeAddr{
		IP:       ip,
		Port:     port,
		hostType: hostAddrType,
	}
}

func NewNodeI2PAddr(dest I2PDest, port int) NodeAddr {
	return NodeAddr{
		I2PAddress: dest,
		Port:       port,
		hostType:   I2PAddr,
	}
}

// A zero Port is taken to mean no port provided, per BEP 7.
func (me NodeAddr) String() string {
	if me.Port == 0 {
		return me.Host()
	}
	return net.JoinHostPort(me.Host(), strconv.FormatInt(int64(me.Port), 10))
}

func (me NodeAddr) Host() string {
	if networkType == I2PNet {
		return me.I2PAddress.String()
	}
	return me.IP.String()
}

func (me *NodeAddr) UnmarshalBinary(b []byte) error {
	if networkType == I2PNet {
		me.hostType = I2PAddr
		me.I2PAddress = b[:len(b)-2]
	} else {
		me.IP = make(net.IP, len(b)-2)
		copy(me.IP, b[:len(b)-2])
		if me.IP.To4() == nil {
			me.hostType = IPV6Addr
		}
	}

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
	var b bytes.Buffer
	if me.hostType == I2PAddr {
		b.Write(me.I2PAddress)
	} else {
		b.Write(me.IP)
	}

	binary.Write(&b, binary.BigEndian, uint16(me.Port))
	return b.Bytes(), nil
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

//TODO: should this be removed? Only used in test
func (me *NodeAddr) FromUDPAddr(ua *net.UDPAddr) {
	me.IP = ua.IP
	me.Port = ua.Port
}

func (me NodeAddr) Compacted() NodeAddr {
	if me.hostType == I2PAddr {
		return NodeAddr{
			I2PAddress: me.I2PAddress.Compact(),
			Port:       me.Port,
			hostType:   me.hostType,
		}
	}

	var ip net.IP
	if me.hostType == IPV4Addr {
		ip = me.IP.To4()
	} else if me.hostType == IPV6Addr {
		ip = me.IP.To16()
	}

	return NodeAddr{
		IP:       ip,
		Port:     me.Port,
		hostType: me.hostType,
	}
}

func (me NodeAddr) Equal(x NodeAddr) bool {
	if me.hostType == I2PAddr {
		return bytes.Equal(me.I2PAddress, x.I2PAddress) && me.Port == x.Port
	}

	return me.IP.Equal(x.IP) && me.Port == x.Port
}
