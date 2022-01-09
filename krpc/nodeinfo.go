package krpc

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	"math/rand"
	"net"
)

type NodeInfo struct {
	ID   ID
	Addr NodeAddr
}

func (me NodeInfo) String() string {
	return fmt.Sprintf("{%x at %s}", me.ID, me.Addr)
}

func RandomNodeInfo(ipLen int) (ni NodeInfo) {
	rand.Read(ni.ID[:])
	ip := make(net.IP, ipLen)
	rand.Read(ip)
	port := rand.Intn(math.MaxUint16 + 1)
	ni.Addr = NewNodeIPAddr(ip, port)
	return
}

var _ interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
} = (*NodeInfo)(nil)

func (ni NodeInfo) MarshalBinary() ([]byte, error) {
	var w bytes.Buffer
	w.Write(ni.ID[:])
	addrBytes, err := ni.Addr.MarshalBinary()
	if err != nil {
		return nil, err
	}

	_, err = w.Write(addrBytes)
	if err != nil {
		return nil, err
	}

	return w.Bytes(), nil
}

func (ni *NodeInfo) UnmarshalBinary(b []byte) error {
	copy(ni.ID[:], b)
	return ni.Addr.UnmarshalBinary(b[20:])
}
