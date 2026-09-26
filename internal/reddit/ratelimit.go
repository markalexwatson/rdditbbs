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
	remaining float64
	reset     time.Time
	inflight  int
}

// NewRateGate creates a gate. sleep defaults to SleepContext; onWait may be nil.
func NewRateGate(now func() time.Time, sleep func(context.Context, time.Duration) error, onWait func(time.Duration)) *RateGate {
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = SleepContext
	}
	return &RateGate{now: now, sleep: sleep, onWait: onWait, remaining: fallbackPerMinute}
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

// Acquire blocks until a request may be sent, then reserves a unit.
func (g *RateGate) Acquire(ctx context.Context) error {
	for {
		g.mu.Lock()
		now := g.now()
		if !g.reset.IsZero() && !now.Before(g.reset) {
			g.remaining = fallbackPerMinute
			g.reset = time.Time{}
		}
		if g.remaining-float64(g.inflight) >= minHeadroom {
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

// Release records a response's headers and frees the reservation. Missing
// headers count down an assumed allowance of 60 per minute.
func (g *RateGate) Release(h http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inflight--
	if r := h.Get("X-Ratelimit-Remaining"); r != "" {
		if f, err := strconv.ParseFloat(r, 64); err == nil {
			g.remaining = f
		}
	} else {
		g.remaining--
	}
	if s := h.Get("X-Ratelimit-Reset"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			g.reset = g.now().Add(time.Duration(secs) * time.Second)
		}
	} else if g.reset.IsZero() {
		g.reset = g.now().Add(time.Minute)
	}
}

// Reset clears the recorded limit after a 429 wait has elapsed, so the next
// Acquire does not wait a second time for the old reset.
func (g *RateGate) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.remaining = fallbackPerMinute
	g.reset = time.Time{}
}

// WaitFor returns how long a 429 response asks us to wait: Retry-After, else
// the ratelimit reset, else one second.
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
	return time.Second
}
