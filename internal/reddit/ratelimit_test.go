package reddit

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fakeSleeper struct {
	clock *fakeClock
	slept []time.Duration
}

func (s *fakeSleeper) Sleep(_ context.Context, d time.Duration) error {
	s.slept = append(s.slept, d)
	s.clock.Advance(d)
	return nil
}

func headers(remaining, reset string) http.Header {
	h := http.Header{}
	h.Set("X-Ratelimit-Remaining", remaining)
	h.Set("X-Ratelimit-Reset", reset)
	return h
}

func TestRateGateWaitsWhenExhausted(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	var waited []time.Duration
	g := NewRateGate(clock.Now, sl.Sleep, func(d time.Duration) { waited = append(waited, d) })
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	g.Release(headers("1", "30"))
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 1 || sl.slept[0] != 30*time.Second {
		t.Errorf("slept %v, want [30s]", sl.slept)
	}
	if len(waited) != 1 {
		t.Errorf("onWait called %d times", len(waited))
	}
}

func TestRateGateReservesInFlight(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	g := NewRateGate(clock.Now, sl.Sleep, nil)
	_ = g.Acquire(context.Background())
	g.Release(headers("2", "60"))
	_ = g.Acquire(context.Background()) // remaining 2 - 0 in flight = 2, allowed
	_ = g.Acquire(context.Background()) // 2 - 1 = 1 < 2, must wait for reset
	if len(sl.slept) != 1 {
		t.Errorf("slept %v, want one wait", sl.slept)
	}
}

func TestRateGateFallbackWithoutHeaders(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	g := NewRateGate(clock.Now, sl.Sleep, nil)
	for i := 0; i < 59; i++ {
		_ = g.Acquire(context.Background())
		g.Release(http.Header{})
	}
	_ = g.Acquire(context.Background()) // remaining is 1: below the headroom of 2, so wait for the assumed reset
	g.Release(nil)
	if len(sl.slept) != 1 {
		t.Errorf("expected one wait after 60 unheadered requests, slept %v", sl.slept)
	}
}

func TestRateGateCancelled(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	g := NewRateGate(clock.Now, SleepContext, nil)
	_ = g.Acquire(context.Background())
	g.Release(headers("0", "600"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Acquire(ctx); err == nil {
		t.Error("expected context error")
	}
}

func TestWaitForPrefersRetryAfter(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	g := NewRateGate(clock.Now, nil, nil)
	h := headers("0", "45")
	h.Set("Retry-After", "7")
	if d := g.WaitFor(h); d != 7*time.Second {
		t.Errorf("WaitFor = %v", d)
	}
	if d := g.WaitFor(headers("0", "45")); d != 45*time.Second {
		t.Errorf("WaitFor reset = %v", d)
	}
	if d := g.WaitFor(http.Header{}); d != time.Second {
		t.Errorf("WaitFor empty = %v", d)
	}
}
