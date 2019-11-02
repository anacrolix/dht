package rate

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/lukechampine/stm"
	"github.com/lukechampine/stm/stmutil"
)

type numTokens = int

type Limiter struct {
	max     *stm.Var
	cur     *stm.Var
	lastAdd *stm.Var
	rate    Limit
}

const Inf = Limit(math.MaxFloat64)

type Limit float64

func (l Limit) interval() time.Duration {
	return time.Duration(Limit(1*time.Second) / l)
}

func Every(interval time.Duration) Limit {
	if interval == 0 {
		return Inf
	}
	return Limit(time.Second / interval)
}

func NewLimiter(rate Limit, burst numTokens) *Limiter {
	rl := &Limiter{
		max:     stm.NewVar(burst),
		cur:     stm.NewVar(burst),
		lastAdd: stm.NewVar(time.Now()),
		rate:    rate,
	}
	if rate != Inf {
		go rl.tokenGenerator(rate.interval())
	}
	return rl
}

func (rl *Limiter) tokenGenerator(interval time.Duration) {
	for {
		lastAdd := stm.AtomicGet(rl.lastAdd).(time.Time)
		time.Sleep(time.Until(lastAdd.Add(interval)))
		now := time.Now()
		available := numTokens(now.Sub(lastAdd) / interval)
		if available < 1 {
			continue
		}
		stm.Atomically(func(tx *stm.Tx) {
			cur := tx.Get(rl.cur).(numTokens)
			max := tx.Get(rl.max).(numTokens)
			tx.Assert(cur < max)
			newCur := cur + available
			if newCur > max {
				newCur = max
			}
			if newCur != cur {
				tx.Set(rl.cur, newCur)
			}
			tx.Set(rl.lastAdd, lastAdd.Add(interval*time.Duration(available)))
		})
	}
}

func (rl *Limiter) Allow() bool {
	return rl.AllowN(1)
}

func (rl *Limiter) AllowN(n numTokens) bool {
	return stm.Atomically(func(tx *stm.Tx) {
		tx.Return(rl.takeTokens(tx, n))
	}).(bool)
}

func (rl *Limiter) AllowStm(tx *stm.Tx) bool {
	return rl.takeTokens(tx, 1)
}

func (rl *Limiter) takeTokens(tx *stm.Tx, n numTokens) bool {
	if rl.rate == Inf {
		return true
	}
	cur := tx.Get(rl.cur).(numTokens)
	if cur >= n {
		tx.Set(rl.cur, cur-n)
		return true
	}
	return false
}

func (rl *Limiter) Wait(ctx context.Context) error {
	return rl.WaitN(ctx, 1)
}

func (rl *Limiter) WaitN(ctx context.Context, n int) error {
	ctxDone, cancel := stmutil.ContextDoneVar(ctx)
	defer cancel()
	if err := stm.Atomically(func(tx *stm.Tx) {
		if tx.Get(ctxDone).(bool) {
			tx.Return(ctx.Err())
		}
		if rl.takeTokens(tx, n) {
			tx.Return(nil)
		}
		if n > tx.Get(rl.max).(numTokens) {
			tx.Return(errors.New("burst exceeded"))
		}
		if dl, ok := ctx.Deadline(); ok {
			if tx.Get(rl.cur).(numTokens)+numTokens(dl.Sub(tx.Get(rl.lastAdd).(time.Time))/rl.rate.interval()) < n {
				tx.Return(context.DeadlineExceeded)
			}
		}
		tx.Retry()
	}); err != nil {
		return err.(error)
	}
	return nil

}
