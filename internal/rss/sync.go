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

// next picks the most useful fetch: a stale entry a reader opened, then any
// listing that is missing or stale, then missing or old threads in list order.
func (s *Syncer) next(failed map[string]bool) (job, string, bool) {
	for {
		j, ok := s.c.popRefresh()
		if !ok {
			break
		}
		if !failed[j.url] {
			return j, "refresh " + s.c.key(j.url), true
		}
	}
	areas := s.snapshot()
	for _, a := range areas {
		u := s.c.listingURL(a, s.Sort, "")
		if failed[u] {
			continue
		}
		if age, ok := s.c.age(u); !ok || age > listingTTL {
			return job{url: u, ttl: listingTTL, validate: validListing}, "r/" + a + " listing", true
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
			u := s.c.threadURL(a, p.ID)
			if failed[u] {
				continue
			}
			if age, ok := s.c.age(u); !ok || age > s.ThreadRefresh {
				return job{url: u, ttl: threadTTL, validate: validThread}, "r/" + a + " thread: " + p.Title, true
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
		if s.c.fg.Load() > 0 {
			if err := s.c.sleep(ctx, 2*time.Second); err != nil {
				return err
			}
			continue
		}
		if s.c.gate.TryAcquire() {
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

// Sweep fetches everything that is missing or stale and reports how many
// feeds it fetched. A feed that fails is skipped for the rest of the sweep.
func (s *Syncer) Sweep(ctx context.Context) (int, error) {
	if s.c.disk == nil {
		return 0, nil
	}
	fetched := 0
	failed := map[string]bool{}
	for {
		j, what, ok := s.next(failed)
		if !ok {
			return fetched, nil
		}
		if err := s.waitSlot(ctx); err != nil {
			return fetched, err
		}
		if err := s.c.runJob(ctx, j); err != nil {
			if ctx.Err() != nil {
				return fetched, ctx.Err()
			}
			failed[j.url] = true
			s.logf("skipped %s: %v", what, err)
			continue
		}
		fetched++
		s.logf("fetched %s", what)
	}
}

// Run sweeps repeatedly until ctx is cancelled, pausing between sweeps and
// pruning entries older than a day.
func (s *Syncer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if _, err := s.Sweep(ctx); err != nil {
			return
		}
		if s.c.disk != nil {
			s.c.disk.Prune(s.c.now(), maxAge)
		}
		if err := s.c.sleep(ctx, s.Idle); err != nil {
			return
		}
	}
}
