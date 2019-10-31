package dht

import (
	"context"
	"sync"
	"time"

	"github.com/lukechampine/stm"
)

type numTokens int

type rateLimiter struct {
	max      *stm.Var
	cur      *stm.Var
	interval time.Duration

	mu sync.Mutex
	t  *time.Timer
}

func newRateLimiter(rate float64, burst numTokens) *rateLimiter {
	rl := &rateLimiter{
		max:      stm.NewVar(burst),
		cur:      stm.NewVar(burst),
		interval: time.Duration(float64(1*time.Second) / rate),
	}
	rl.mu.Lock()
	rl.t = time.AfterFunc(rl.interval, rl.timerCallback)
	rl.mu.Unlock()
	return rl
}

func (rl *rateLimiter) timerCallback() {
	stm.Atomically(func(tx *stm.Tx) {
		cur := tx.Get(rl.cur).(numTokens)
		max := tx.Get(rl.max).(numTokens)
		if cur < max {
			tx.Set(rl.cur, cur+1)
		}
	})
	rl.mu.Lock()
	rl.t.Reset(rl.interval)
	rl.mu.Unlock()
}

func (rl *rateLimiter) Allow() bool {
	return stm.Atomically(func(tx *stm.Tx) {
		tx.Return(rl.takeToken(tx))
	}).(bool)
}

func (rl *rateLimiter) takeToken(tx *stm.Tx) bool {
	cur := tx.Get(rl.cur).(numTokens)
	if cur > 0 {
		tx.Set(rl.cur, cur-1)
		return true
	}
	return false

}

func (rl *rateLimiter) Wait(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctxDone := stm.NewVar(false)
	go func() {
		<-ctx.Done()
		stm.AtomicSet(ctxDone, true)
	}()
	if err := stm.Atomically(func(tx *stm.Tx) {
		if tx.Get(ctxDone).(bool) {
			tx.Return(ctx.Err())
		}
		if rl.takeToken(tx) {
			tx.Return(nil)
		}
		tx.Retry()
	}); err != nil {
		return err.(error)
	}
	return nil
}
