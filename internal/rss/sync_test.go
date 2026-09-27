package rss

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func syncer(t *testing.T, s *srv, areas ...string) (*Syncer, *Client, *fakeClock, *[]time.Duration) {
	t.Helper()
	d, _ := NewDisk(t.TempDir())
	c, clock, slept := clientWithDisk(t, s, d)
	sy := NewSyncer(c, areas)
	sy.PerArea = 2
	return sy, c, clock, slept
}

func paths(s *srv) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, r := range s.requests {
		out = append(out, r.URL.Path)
	}
	return out
}

func TestSweepFetchesListingsThenThreads(t *testing.T) {
	s := &srv{}
	sy, c, _, slept := syncer(t, s, "linux")
	n, err := sy.Sweep(context.Background())
	if err != nil || n != 3 {
		t.Fatalf("sweep fetched %d err %v; requests %v", n, err, paths(s))
	}
	got := paths(s)
	if got[0] != "/r/linux/hot/.rss" || !strings.Contains(got[1], "/comments/1wron16/") || !strings.Contains(got[2], "/comments/1wrnx3z/") {
		t.Errorf("order = %v", got)
	}
	if len(*slept) < 2 {
		t.Errorf("the sweep should pace itself to the feed allowance, slept %v", *slept)
	}
	if !c.Cached("linux", "1wron16") || !c.Cached("linux", "1wrnx3z") || c.Cached("linux", "1wrn23t") {
		t.Error("the first two threads should be cached and the third not")
	}
	if n, _ := sy.Sweep(context.Background()); n != 0 || s.count() != 3 {
		t.Errorf("a second sweep straight away has nothing to do: fetched %d requests %d", n, s.count())
	}
}

func TestSweepRefreshesStaleListings(t *testing.T) {
	s := &srv{}
	sy, _, clock, _ := syncer(t, s, "linux")
	sy.Sweep(context.Background())
	clock.Advance(20 * time.Minute) // listing is stale (10 min); threads are not (1 hour)
	n, _ := sy.Sweep(context.Background())
	if n != 1 || paths(s)[3] != "/r/linux/hot/.rss" {
		t.Errorf("only the listing should refresh: fetched %d, requests %v", n, paths(s)[3:])
	}
}

func TestSweepTakesForegroundRefreshesFirst(t *testing.T) {
	s := &srv{}
	sy, c, clock, _ := syncer(t, s, "linux")
	sy.Sweep(context.Background())
	clock.Advance(16 * time.Minute)
	c.Thread(context.Background(), "linux", "1wrnx3z", "best", struct{ Fresh bool }{}) // stale: served and queued
	before := s.count()
	sy.Sweep(context.Background())
	if got := paths(s)[before]; !strings.Contains(got, "/comments/1wrnx3z/") {
		t.Errorf("the thread the reader just opened should refresh first, got %v", paths(s)[before:])
	}
}

func TestSweepYieldsWhileForegroundWaits(t *testing.T) {
	s := &srv{}
	sy, c, _, _ := syncer(t, s, "linux")
	c.fg.Add(1) // a reader is waiting for the allowance
	yielded := 0
	orig := c.sleep
	c.sleep = func(ctx context.Context, d time.Duration) error {
		if c.fg.Load() > 0 {
			yielded++
			if s.count() != 0 {
				t.Error("the syncer must not fetch while a foreground request is waiting")
			}
			c.fg.Add(-1)
		}
		return orig(ctx, d)
	}
	sy.Sweep(context.Background())
	if yielded == 0 {
		t.Error("the syncer should have waited for the foreground request")
	}
}

func TestSweepSkipsFailuresAndStopsOnCancel(t *testing.T) {
	s := &srv{}
	s.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if strings.Contains(r.URL.Path, "/comments/1wron16/") {
			w.WriteHeader(404)
			return true
		}
		return false
	}
	sy, c, _, _ := syncer(t, s, "linux")
	n, err := sy.Sweep(context.Background())
	if err != nil || n != 2 {
		t.Errorf("a failing thread should be skipped, not retried: fetched %d err %v requests %v", n, err, paths(s))
	}
	if c.Cached("linux", "1wron16") {
		t.Error("the failed thread must not be cached")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sy2, _, _, _ := syncer(t, &srv{}, "linux")
	if _, err := sy2.Sweep(ctx); err == nil {
		t.Error("a cancelled sweep should report the cancellation")
	}
}

func TestSetAreas(t *testing.T) {
	s := &srv{}
	sy, _, _, _ := syncer(t, s, "linux")
	sy.PerArea = 0
	sy.SetAreas([]string{"rust", "vim"})
	sy.Sweep(context.Background())
	if got := paths(s); len(got) != 2 || got[0] != "/r/rust/hot/.rss" || got[1] != "/r/vim/hot/.rss" {
		t.Errorf("requests = %v", got)
	}
}
