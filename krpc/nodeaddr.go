package krpc

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"

	"github.com/anacrolix/torrent/bencode"
)

const (
	IPV4 = iota
	IPV6
	I2P
)

type NodeAddr struct {
	IP       net.IP
	Port     int
	Host     string `bencode:"-"`
	hostType int    `bencode:"-"`
}

func NewNodeIPAddr(ip net.IP, port int) NodeAddr {
	hostAddrType := IPV4
	if ip.To4() == nil {
		hostAddrType = IPV6
	}

	return NodeAddr{
		IP:       ip,
		Port:     port,
		Host:     ip.String(),
		hostType: hostAddrType,
	}
}

func NewNodeI2PAddr(host string, port int) NodeAddr {
	return NodeAddr{
		Port:     port,
		Host:     host,
		hostType: I2P,
	}
}

// A zero Port is taken to mean no port provided, per BEP 7.
func (me NodeAddr) String() string {
	if me.Port == 0 {
		return me.Host
	}
	return net.JoinHostPort(me.Host, strconv.FormatInt(int64(me.Port), 10))
}

func (me *NodeAddr) UnmarshalBinary(b []byte) error {
	if len(b)-2 > 16 {
		me.hostType = I2P
		me.Host = string(b[:len(b)-2])
	} else {
		me.IP = make(net.IP, len(b)-2)
		copy(me.IP, b[:len(b)-2])
		if me.IP.To4() == nil {
			me.hostType = IPV6
		}
		me.Host = me.IP.String()
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
	if me.hostType == I2P {
		b.WriteString(me.Host)
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
	me.Host = ua.IP.String()
}

func (me NodeAddr) Compacted() NodeAddr {
	if me.hostType == I2P {
		//TODO: do we need i2p address compaction?
		return me
	}

	var ip net.IP
	if me.hostType == IPV4 {
		ip = me.IP.To4()
	} else if me.hostType == IPV6 {
		ip = me.IP.To16()
	}

	return NodeAddr{
		IP:       ip,
		Host:     me.Host,
		Port:     me.Port,
		hostType: me.hostType,
	}
}

func (me NodeAddr) Equal(x NodeAddr) bool {
	return me.IP.Equal(x.IP) && me.Port == x.Port
}
