package traversal

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// The counters are updated with atomic adds while a traversal runs. Copying them must use atomic
// loads; a plain struct copy races with those adds.
func TestLoadStatsDuringUpdates(t *testing.T) {
	var op Operation
	var wg sync.WaitGroup
	const updates = 10000
	started := make(chan struct{})
	wg.Go(func() {
		atomic.AddUint32(&op.stats.NumAddrsTried, 1)
		atomic.AddUint32(&op.stats.NumResponses, 1)
		close(started)
		for range updates {
			atomic.AddUint32(&op.stats.NumAddrsTried, 1)
			atomic.AddUint32(&op.stats.NumResponses, 1)
			runtime.Gosched()
		}
	})
	<-started
	var last Stats
	for range updates {
		current := op.LoadStats()
		if current.NumAddrsTried < last.NumAddrsTried || current.NumResponses < last.NumResponses {
			t.Errorf("counters went backwards: previous %+v, current %+v", last, current)
		}
		last = current
		runtime.Gosched()
	}
	wg.Wait()
	if got := op.LoadStats(); got.NumAddrsTried != updates+1 || got.NumResponses != updates+1 {
		t.Fatalf("final counters = %+v, want %d each", got, updates+1)
	}
}
