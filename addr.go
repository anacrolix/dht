package dht

import (
	"fmt"
	"net"
	"strconv"

	"github.com/anacrolix/dht/v2/krpc"
)

// The IP of a net.Addr, without going through its String form where the concrete type allows it.
// Panics if the address has no host that parses as an IP.
func addrIP(addr net.Addr) net.IP {
	if addr == nil {
		return nil
	}
	switch raw := addr.(type) {
	case *net.UDPAddr:
		return raw.IP
	case *net.TCPAddr:
		return raw.IP
	default:
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			panic(fmt.Errorf("splitting host and port of %q: %w", addr, err))
		}
		return net.ParseIP(host)
	}
}

// The port of a net.Addr, without going through its String form where the concrete type allows it.
// Panics if the address has no port that parses as an integer.
func addrPort(addr net.Addr) int {
	switch raw := addr.(type) {
	case *net.UDPAddr:
		return raw.Port
	case *net.TCPAddr:
		return raw.Port
	default:
		_, port, err := net.SplitHostPort(addr.String())
		if err != nil {
			panic(fmt.Errorf("splitting host and port of %q: %w", addr, err))
		}
		i64, err := strconv.ParseInt(port, 0, 0)
		if err != nil {
			panic(fmt.Errorf("parsing port %q of %q: %w", port, addr, err))
		}
		return int(i64)
	}
}

// Used internally to refer to node network addresses. String() is called a
// lot, and so can be optimized. Network() is not exposed, so that the
// interface does not satisfy net.Addr, as the underlying type must be passed
// to any OS-level function that take net.Addr.
type Addr interface {
	Raw() net.Addr
	Port() int
	IP() net.IP
	String() string
	KRPC() krpc.NodeAddr
}

// Speeds up some of the commonly called Addr methods.
type cachedAddr struct {
	raw  net.Addr
	port int
	ip   net.IP
	s    string
}

func (ca cachedAddr) String() string {
	return ca.s
}

func (ca cachedAddr) KRPC() krpc.NodeAddr {
	return krpc.NodeAddr{
		IP:   ca.ip,
		Port: ca.port,
	}
}

func (ca cachedAddr) IP() net.IP {
	return ca.ip
}

func (ca cachedAddr) Port() int {
	return ca.port
}

func (ca cachedAddr) Raw() net.Addr {
	return ca.raw
}

func NewAddr(raw net.Addr) Addr {
	return cachedAddr{
		raw:  raw,
		s:    raw.String(),
		ip:   addrIP(raw),
		port: addrPort(raw),
	}
}
