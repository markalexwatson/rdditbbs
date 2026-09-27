package rss

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

// Syncer keeps the disk store stocked in the background, the way an offline
// mail reader polls a board: it spends the feed allowance on the listings of
// the configured areas and then on the threads of their leading posts, so
// that reading rarely has to wait. It always yields to a reader who is
// waiting for the allowance.
type Syncer struct {
	c *Client

	mu    sync.Mutex
	areas []string

	Sort          reddit.Sort
	PerArea       int           // threads per area to keep cached
	ThreadRefresh time.Duration // refetch a cached thread after this long
	Idle          time.Duration // pause between sweeps in Run
	Log           func(format string, args ...any)
}

// SweepResult counts what one sweep did.
type SweepResult struct {
	Fetched int // feeds fetched and stored
	Failed  int // feeds that could not be fetched
	Skipped int // feeds that something else refreshed while the sweep waited
}

// NewSyncer creates a syncer for a client that has a disk store.
func NewSyncer(c *Client, areas []string) *Syncer {
	return &Syncer{c: c, areas: append([]string(nil), areas...), Sort: reddit.Hot, PerArea: 10, ThreadRefresh: time.Hour, Idle: time.Minute}
}

// SetAreas replaces the subreddits to keep stocked.
func (s *Syncer) SetAreas(areas []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.areas = append([]string(nil), areas...)
}

func (s *Syncer) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.areas...)
}

func (s *Syncer) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

// due reports whether j still needs fetching.
func (s *Syncer) due(j job) bool {
	age, ok := s.c.age(j.url)
	return !ok || age > j.dueAfter
}

// next picks the most useful fetch not yet attempted in this sweep: a stale
// entry a reader opened, then any listing that is missing or stale, then
// missing or old threads in list order.
func (s *Syncer) next(attempted map[string]bool) (job, string, bool) {
	for {
		j, ok := s.c.popRefresh()
		if !ok {
			break
		}
		if !attempted[j.url] {
			return j, "refresh " + s.c.key(j.url), true
		}
	}
	areas := s.snapshot()
	for _, a := range areas {
		j := job{url: s.c.listingURL(a, s.Sort, ""), ttl: listingTTL, validate: validListing, dueAfter: listingTTL}
		if !attempted[j.url] && s.due(j) {
			return j, "r/" + a + " listing", true
		}
	}
	for _, a := range areas {
		b, _, ok := s.c.disk.Get(s.c.key(s.c.listingURL(a, s.Sort, "")))
		if !ok {
			continue
		}
		l, err := ParseListing(bytes.NewReader(b))
		if err != nil {
			continue
		}
		for i, p := range l.Posts {
			if i >= s.PerArea {
				break
			}
			j := job{url: s.c.threadURL(a, p.ID), ttl: threadTTL, validate: validThread, dueAfter: s.ThreadRefresh}
			if !attempted[j.url] && s.due(j) {
				return j, "r/" + a + " thread: " + p.Title, true
			}
		}
	}
	return job{}, "", false
}

// waitSlot blocks until the feed allowance has room and no reader is waiting
// for it, then reserves it.
func (s *Syncer) waitSlot(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.c.loadRate()
		if s.c.gate.Waiting() > 0 {
			if err := s.c.sleep(ctx, 2*time.Second); err != nil {
				return err
			}
			continue
		}
		if s.c.gate.TryAcquireIdle() {
			return nil
		}
		d := s.c.gate.UntilReset()
		if d <= 0 {
			d = time.Second
		}
		if err := s.c.sleep(ctx, d); err != nil {
			return err
		}
	}
}

// Sweep fetches what is missing or stale, attempting each feed at most once
// so it always finishes, and reports what it did. Work that something else
// completed while the sweep waited for the allowance is skipped.
func (s *Syncer) Sweep(ctx context.Context) (SweepResult, error) {
	var res SweepResult
	if s.c.disk == nil {
		return res, nil
	}
	attempted := map[string]bool{}
	for {
		j, what, ok := s.next(attempted)
		if !ok {
			return res, nil
		}
		attempted[j.url] = true
		if err := s.waitSlot(ctx); err != nil {
			return res, err
		}
		s.c.disk.Rescan()
		if !s.due(j) {
			s.c.gate.Unreserve()
			res.Skipped++
			continue
		}
		if err := s.c.runJob(ctx, j); err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			res.Failed++
			s.logf("skipped %s: %v", what, err)
			continue
		}
		res.Fetched++
		s.logf("fetched %s", what)
	}
}

// Run sweeps repeatedly until ctx is cancelled, pruning before each sweep and
// pausing between them.
func (s *Syncer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if s.c.disk != nil {
			s.c.disk.Prune(s.c.now(), maxAge)
		}
		if _, err := s.Sweep(ctx); err != nil {
			return
		}
		if err := s.c.sleep(ctx, s.Idle); err != nil {
			return
		}
	}
}
