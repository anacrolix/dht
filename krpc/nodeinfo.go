package krpc

import (
	"crypto/rand"
	"encoding"
	"encoding/binary"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"net"
)

type NodeInfo struct {
	ID   ID
	Addr NodeAddr
}

func (me NodeInfo) ToNodeInfoAddrPort() NodeInfoAddrPort {
	return NodeInfoAddrPort{me.ID, me.Addr.ToNodeAddrPort()}
}

func (me NodeInfo) String() string {
	return fmt.Sprintf("{%x at %s}", me.ID, me.Addr)
}

func RandomNodeInfo(ipLen int) (ni NodeInfo) {
	rand.Read(ni.ID[:])
	ni.Addr.IP = make(net.IP, ipLen)
	rand.Read(ni.Addr.IP)
	ni.Addr.Port = mathrand.IntN(math.MaxUint16 + 1)
	return
}

var _ interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
} = (*NodeInfo)(nil)

func (me NodeInfo) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, len(me.ID)+len(me.Addr.IP)+2)
	b = append(b, me.ID[:]...)
	b = append(b, me.Addr.IP...)
	return binary.BigEndian.AppendUint16(b, uint16(me.Addr.Port)), nil
}

func (me *NodeInfo) UnmarshalBinary(b []byte) error {
	if len(b) < len(me.ID) {
		return fmt.Errorf("unmarshal NodeInfo from %d bytes: need at least %d", len(b), len(me.ID))
	}
	copy(me.ID[:], b)
	return me.Addr.UnmarshalBinary(b[len(me.ID):])
}
