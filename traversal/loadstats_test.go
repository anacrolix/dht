package traversal

import (
	"sync"
	"sync/atomic"
	"testing"
)

// The counters are updated with atomic adds while a traversal runs. Copying them must use atomic
// loads; a plain struct copy races with those adds.
func TestLoadStatsDuringUpdates(t *testing.T) {
	var op Operation
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				atomic.AddUint32(&op.stats.NumAddrsTried, 1)
				atomic.AddUint32(&op.stats.NumResponses, 1)
			}
		}
	})
	var last Stats
	for range 10000 {
		last = op.LoadStats()
	}
	close(stop)
	wg.Wait()
	if last.NumAddrsTried == 0 || last.NumResponses == 0 {
		t.Fatalf("expected counters to move, got %+v", last)
	}
}
