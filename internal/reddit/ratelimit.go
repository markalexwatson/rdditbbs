package reddit

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	fallbackPerMinute = 60
	minHeadroom       = 2
)

// RateGate paces requests from Reddit's X-Ratelimit headers, reserving one
// unit per in-flight request and waiting for the reset when headroom runs out.
type RateGate struct {
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)

	mu        sync.Mutex
	allowance float64 // assumed budget per minute when headers are absent or after a reset
	remaining float64
	reset     time.Time
	inflight  int
	waiters   int // foreground callers inside Acquire, which background work yields to
}

// NewRateGate creates a gate. sleep defaults to SleepContext; onWait may be nil.
func NewRateGate(now func() time.Time, sleep func(context.Context, time.Duration) error, onWait func(time.Duration)) *RateGate {
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = SleepContext
	}
	return &RateGate{now: now, sleep: sleep, onWait: onWait, allowance: fallbackPerMinute, remaining: fallbackPerMinute}
}

// SetAllowance sets the assumed budget per minute (used before the first
// response, when headers are missing, and after a reset). Feeds use 1.
func (g *RateGate) SetAllowance(n float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.allowance = n
	if g.reset.IsZero() {
		g.remaining = n
	}
}

// SleepContext sleeps for d or until ctx is done.
func SleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Acquire blocks until a request may be sent, then reserves a unit. While it
// waits it counts as a waiter, which TryAcquireIdle respects.
func (g *RateGate) Acquire(ctx context.Context) error {
	g.AddWaiter(1)
	defer g.AddWaiter(-1)
	for {
		g.mu.Lock()
		now := g.now()
		if !g.reset.IsZero() && !now.Before(g.reset) {
			g.remaining = g.allowance
			g.reset = time.Time{}
		}
		if g.remaining-float64(g.inflight) >= g.headroom() {
			g.inflight++
			g.mu.Unlock()
			return nil
		}
		wait := g.reset.Sub(now)
		if wait <= 0 {
			wait = time.Second
		}
		g.mu.Unlock()
		if g.onWait != nil {
			g.onWait(wait)
		}
		if err := g.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// AddWaiter adjusts the count of foreground callers waiting for the
// allowance. Acquire does this itself; it is exported for coordination
// between components and for tests.
func (g *RateGate) AddWaiter(delta int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.waiters += delta
	if g.waiters < 0 {
		g.waiters = 0
	}
}

// Waiting is the number of foreground callers waiting for the allowance.
func (g *RateGate) Waiting() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.waiters
}

// TryAcquireIdle is TryAcquire for background work: it refuses whenever a
// foreground caller is waiting, checked under the same lock as the
// reservation so a reader can never be overtaken.
func (g *RateGate) TryAcquireIdle() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.waiters > 0 {
		return false
	}
	return g.tryLocked()
}

// BlockUntil records that the allowance is spent until t, as learned from
// elsewhere (another process sharing the address). It never shortens a
// window already known.
func (g *RateGate) BlockUntil(t time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !t.After(g.now()) {
		return
	}
	g.remaining = 0
	if t.After(g.reset) {
		g.reset = t
	}
}

// State reports the remaining allowance and when the window resets.
func (g *RateGate) State() (remaining float64, reset time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.remaining, g.reset
}

// Unreserve gives back a reservation that was never used for a request.
func (g *RateGate) Unreserve() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight > 0 {
		g.inflight--
	}
}

// UntilReset is how long until the current window resets, or zero when no
// window is known.
func (g *RateGate) UntilReset() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.reset.IsZero() {
		return 0
	}
	if d := g.reset.Sub(g.now()); d > 0 {
		return d
	}
	return 0
}

// TryAcquire reserves a unit only if one is available now; it never waits.
// Background prefetching uses it so it can never delay a user's request.
func (g *RateGate) TryAcquire() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tryLocked()
}

func (g *RateGate) tryLocked() bool {
	now := g.now()
	if !g.reset.IsZero() && !now.Before(g.reset) {
		g.remaining = g.allowance
		g.reset = time.Time{}
	}
	if g.remaining-float64(g.inflight) >= g.headroom() {
		g.inflight++
		return true
	}
	return false
}

// headroom is the margin kept below the limit: two requests on a normal
// allowance, one when the allowance is a single request.
func (g *RateGate) headroom() float64 {
	if g.allowance < minHeadroom {
		return 1
	}
	return minHeadroom
}

// Release records a response's headers and frees the reservation. Missing
// headers count down an assumed allowance of 60 per minute. Responses can
// arrive out of order, so within one reset window a higher remaining count
// never replaces a lower one; only a new window accepts the count as given.
func (g *RateGate) Release(h http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inflight--
	var newReset time.Time
	if s := h.Get("X-Ratelimit-Reset"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			newReset = g.now().Add(time.Duration(secs) * time.Second)
		}
	}
	newWindow := !newReset.IsZero() && newReset.After(g.reset.Add(2*time.Second))
	if r := h.Get("X-Ratelimit-Remaining"); r != "" {
		if f, err := strconv.ParseFloat(r, 64); err == nil {
			if newWindow || f < g.remaining {
				g.remaining = f
			}
		}
	} else {
		g.remaining--
	}
	switch {
	case !newReset.IsZero():
		g.reset = newReset
	case g.reset.IsZero():
		g.reset = g.now().Add(time.Minute)
	}
}

// Reset clears the recorded limit after a 429 wait has elapsed, so the next
// Acquire does not wait a second time for the old reset.
func (g *RateGate) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.remaining = g.allowance
	g.reset = time.Time{}
}

// WaitFor returns how long a 429 response asks us to wait: Retry-After, else
// the response's ratelimit reset, else the reset the gate already knows,
// else one second.
func (g *RateGate) WaitFor(h http.Header) time.Duration {
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if s := h.Get("X-Ratelimit-Reset"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if d := g.reset.Sub(g.now()); d > 0 {
		return d
	}
	return time.Second
}
