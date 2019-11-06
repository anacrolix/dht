package dht

import "fmt"

type TraversalStats struct {
	NumAddrsTried int
	NumResponses  int
}

func (me TraversalStats) String() string {
	return fmt.Sprintf("%#v", me)
}
