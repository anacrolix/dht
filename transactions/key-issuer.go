package transactions

import (
	"encoding/binary"
	"sync/atomic"
)

type IdIssuer interface {
	Issue() Id
}

var DefaultIdIssuer varintIdIssuer

// Issues sequential IDs encoded as unsigned varints, so they stay short. Safe for concurrent use.
type varintIdIssuer struct {
	next atomic.Uint64
}

func (me *varintIdIssuer) Issue() Id {
	return string(binary.AppendUvarint(nil, me.next.Add(1)-1))
}
