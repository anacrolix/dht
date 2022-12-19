package transactions

import "encoding/binary"

type IdIssuer interface {
	Issue() Id
}

var DefaultIdIssuer varintIdIssuer

type varintIdIssuer struct {
	buf  [binary.MaxVarintLen64]byte
	next uint64
}

func (me *varintIdIssuer) Issue() Id {
	n := binary.PutUvarint(me.buf[:], me.next)
	me.next++
	return string(me.buf[:n])
}
