package traversal

import (
	"fmt"
)

type Stats struct {
	// Count of (probably) distinct addresses we've sent traversal queries to. Accessed with atomic.
	NumAddrsTried int64
	// Number of responses we received to queries related to this traversal. Accessed with atomic.
	NumResponses int64
}

func (me Stats) String() string {
	return fmt.Sprintf("%#v", me)
}
