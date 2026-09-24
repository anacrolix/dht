package main

import (
	"math"
	"testing"

	"github.com/anacrolix/dht/v2/bep44"
)

func TestAutoSequenceDoesNotWrap(t *testing.T) {
	put := makeSeqToPut(true, false, bep44.Put{}, nil)(math.MaxInt64)
	if put.Seq != math.MaxInt64 {
		t.Fatalf("auto sequence wrapped to %d", put.Seq)
	}
}
