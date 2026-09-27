package rss

import (
	"context"
	"testing"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

func clientWithDisk(t *testing.T, s *srv, d *Disk) (*Client, *fakeClock, *[]time.Duration) {
	t.Helper()
	c, clock, slept := newTestClient(t, s)
	c.UseDisk(d)
	return c, clock, slept
}

func TestServedFromDiskAfterRestart(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	s1 := &srv{}
	c1, _, _ := clientWithDisk(t, s1, d)
	ctx := context.Background()
	if _, err := c1.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{}); err != nil {
		t.Fatal(err)
	}
	s2 := &srv{}
	c2, _, slept := clientWithDisk(t, s2, d)
	// The second client has a different base URL (its own test server), so key by path.
	th, err := c2.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if err != nil || th.Post == nil || len(th.Comments) != 5 {
		t.Fatalf("thread = %+v err %v", th.Post, err)
	}
	if s2.count() != 0 || len(*slept) != 0 {
		t.Errorf("a restart should read from disk: requests %d slept %v", s2.count(), *slept)
	}
	if !c2.Cached("linux", "1wr4fmd") || c2.Cached("linux", "nope") {
		t.Error("Cached should reflect the disk")
	}
}

func TestStaleDiskEntryServedAndQueuedForRefresh(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	s := &srv{}
	c, clock, _ := clientWithDisk(t, s, d)
	ctx := context.Background()
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	clock.Advance(2 * time.Hour) // past the 15 minute freshness, within the 24 hour maximum
	before := s.count()
	if _, err := c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{}); err != nil {
		t.Fatal(err)
	}
	if s.count() != before {
		t.Error("a stale entry should be served at once, not refetched in the foreground")
	}
	if n := c.PendingRefresh(); n != 1 {
		t.Errorf("the stale entry should be queued for background refresh, pending = %d", n)
	}
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if n := c.PendingRefresh(); n != 1 {
		t.Errorf("the same URL must not be queued twice, pending = %d", n)
	}
}

func TestTooOldDiskEntryIsRefetched(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	s := &srv{}
	c, clock, _ := clientWithDisk(t, s, d)
	ctx := context.Background()
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	clock.Advance(30 * time.Hour)
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if s.count() != 2 {
		t.Errorf("entries older than a day should be fetched again, requests = %d", s.count())
	}
}

func TestFreshBypassesDisk(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	s := &srv{}
	c, clock, _ := clientWithDisk(t, s, d)
	ctx := context.Background()
	c.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{})
	clock.Advance(2 * time.Minute)
	c.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{Fresh: true})
	if s.count() != 2 {
		t.Errorf("Fresh must go to the network, requests = %d", s.count())
	}
}

func TestRateWindowIsSharedBetweenProcesses(t *testing.T) {
	dir := t.TempDir()
	d1, _ := NewDisk(dir)
	d2, _ := NewDisk(dir)
	s1, s2 := &srv{}, &srv{}
	c1, clock1, _ := clientWithDisk(t, s1, d1)
	c2, clock2, slept2 := clientWithDisk(t, s2, d2) // stands in for a separate process with its own gate
	clock2.t = clock1.t
	ctx := context.Background()
	if _, err := c1.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{}); err != nil { // server says: nothing left for 50s
		t.Fatal(err)
	}
	if _, err := c2.Posts(ctx, "rust", reddit.Hot, "", reddit.Fetch{}); err != nil {
		t.Fatal(err)
	}
	if len(*slept2) != 1 || (*slept2)[0] != 50*time.Second {
		t.Errorf("the second process should wait out the window the first one used, slept %v", *slept2)
	}
}
