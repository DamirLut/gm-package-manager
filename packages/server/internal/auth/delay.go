package auth

import (
	"context"
	"sync"
	"time"
)

// limiter slows down repeated login failures per username+IP pair.
// The penalty grows exponentially with each failure and clears on success;
// there is no lockout, so an attacker cannot lock victims out.
type limiter struct {
	mu    sync.Mutex
	fails map[string]int
	base  time.Duration
	max   time.Duration
}

func newLimiter(base, max time.Duration) *limiter {
	return &limiter{fails: make(map[string]int), base: base, max: max}
}

func (l *limiter) key(username, ip string) string {
	return username + "\x00" + ip
}

func (l *limiter) penalty(k string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.fails[k]
	if n == 0 || l.base <= 0 {
		return 0 // the very first attempt pays nothing
	}
	d := l.base
	for range n - 1 {
		d *= 2
		if d >= l.max {
			return l.max
		}
	}
	return d
}

func (l *limiter) fail(k string) {
	l.mu.Lock()
	l.fails[k]++
	l.mu.Unlock()
}

func (l *limiter) reset(k string) {
	l.mu.Lock()
	delete(l.fails, k)
	l.mu.Unlock()
}

// sleepContext waits out the penalty but gives up when the request dies.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
