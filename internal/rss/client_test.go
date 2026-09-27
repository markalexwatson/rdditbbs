package rss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

type srv struct {
	mu       sync.Mutex
	requests []*http.Request
	handler  func(w http.ResponseWriter, r *http.Request, n int) bool
}

func (s *srv) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r)
	n := len(s.requests)
	s.mu.Unlock()
	if s.handler != nil && s.handler(w, r, n) {
		return
	}
	w.Header().Set("X-Ratelimit-Remaining", "0")
	w.Header().Set("X-Ratelimit-Reset", "50")
	if len(r.URL.Path) > 12 && r.URL.Path[len(r.URL.Path)-5:] == "/.rss" && containsComments(r.URL.Path) {
		http.ServeFile(w, r, "testdata/thread.xml")
		return
	}
	http.ServeFile(w, r, "testdata/listing.xml")
}

func containsComments(p string) bool {
	for i := 0; i+9 <= len(p); i++ {
		if p[i:i+9] == "/comments" {
			return true
		}
	}
	return false
}

func (s *srv) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func newTestClient(t *testing.T, s *srv) (*Client, *fakeClock, *[]time.Duration) {
	t.Helper()
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error { slept = append(slept, d); clock.Advance(d); return nil }
	c := NewClient("test-agent/1", WithBaseURL(hs.URL), WithClock(clock.Now), WithSleep(sleep))
	return c, clock, &slept
}

func TestPostsRequestShapeAndParse(t *testing.T) {
	s := &srv{}
	c, _, _ := newTestClient(t, s)
	l, err := c.Posts(context.Background(), "linux", reddit.Top, "t3_prev", reddit.Fetch{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Posts) != 5 || l.After != "t3_1wrmkrs" {
		t.Errorf("posts=%d after=%q", len(l.Posts), l.After)
	}
	r := s.requests[0]
	q := r.URL.Query()
	if r.URL.Path != "/r/linux/top/.rss" || q.Get("limit") != "100" || q.Get("after") != "t3_prev" || q.Get("t") != "day" {
		t.Errorf("request = %s %v", r.URL.Path, q)
	}
	if r.UserAgent() != "test-agent/1" {
		t.Errorf("user agent = %q", r.UserAgent())
	}
	if r.Header.Get("Authorization") != "" {
		t.Error("RSS requests must not carry credentials")
	}
}

func TestThreadAndCache(t *testing.T) {
	s := &srv{}
	c, clock, slept := newTestClient(t, s)
	ctx := context.Background()
	th, err := c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if err != nil || th.Post == nil || len(th.Comments) != 5 {
		t.Fatalf("thread = %+v err %v", th.Post, err)
	}
	if s.requests[0].URL.Path != "/r/linux/comments/1wr4fmd/.rss" {
		t.Errorf("path = %s", s.requests[0].URL.Path)
	}
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if s.count() != 1 {
		t.Error("second thread request should be served from cache")
	}
	if len(*slept) != 0 {
		t.Errorf("a cached hit must not wait on the gate, slept %v", *slept)
	}
	clock.Advance(16 * time.Minute)
	c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{})
	if s.count() != 2 {
		t.Error("threads should expire after 15 minutes")
	}
}

func TestSecondRequestWaitsForReset(t *testing.T) {
	s := &srv{}
	c, _, slept := newTestClient(t, s)
	ctx := context.Background()
	c.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{})
	c.Posts(ctx, "rust", reddit.Hot, "", reddit.Fetch{})
	if len(*slept) != 1 || (*slept)[0] != 50*time.Second {
		t.Errorf("should wait for the feed's reset before the next fetch, slept %v", *slept)
	}
}

func TestRetryAfter429(t *testing.T) {
	s := &srv{}
	s.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(429)
			return true
		}
		return false
	}
	c, _, slept := newTestClient(t, s)
	if _, err := c.Posts(context.Background(), "linux", reddit.Hot, "", reddit.Fetch{}); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 7*time.Second {
		t.Errorf("slept %v", *slept)
	}
}

func TestErrorsAreTyped(t *testing.T) {
	s := &srv{}
	s.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.WriteHeader(404)
			return true
		}
		w.Write([]byte("<html>not a feed</html>"))
		return true
	}
	c, _, _ := newTestClient(t, s)
	_, err := c.Posts(context.Background(), "nosuch", reddit.Hot, "", reddit.Fetch{})
	var ae *reddit.APIError
	if !errors.As(err, &ae) || ae.Status != 404 {
		t.Errorf("404 should be an APIError, got %v", err)
	}
	_, err = c.Posts(context.Background(), "html", reddit.Hot, "", reddit.Fetch{})
	var pe *reddit.ParseError
	if !errors.As(err, &pe) {
		t.Errorf("bad feed should be a ParseError, got %v", err)
	}
}

func TestMoreChildrenAndSubtree(t *testing.T) {
	s := &srv{}
	c, _, _ := newTestClient(t, s)
	th, err := c.MoreChildren(context.Background(), "t3_x", []string{"a"}, reddit.Best)
	if err != nil || len(th.Comments) != 0 || len(th.Stubs) != 0 {
		t.Errorf("MoreChildren = %+v %v", th, err)
	}
	if _, err := c.Subtree(context.Background(), "linux", "1wr4fmd", "pc9r71o", reddit.Best); err != nil {
		t.Errorf("Subtree = %v", err)
	}
}

func TestPrefetchUsesSpareCapacityOnly(t *testing.T) {
	s := &srv{}
	c, clock, slept := newTestClient(t, s)
	ctx := context.Background()
	if !c.Prefetch(ctx, "linux", "1wr4fmd") {
		t.Fatal("a fresh client has capacity to prefetch")
	}
	c.WaitPrefetch()
	if s.count() != 1 {
		t.Fatalf("prefetch should have fetched, requests = %d", s.count())
	}
	// The reset has not passed: a second prefetch must be refused, not queued.
	if c.Prefetch(ctx, "linux", "other") {
		t.Error("prefetch must not start when the gate is exhausted")
	}
	c.WaitPrefetch()
	if s.count() != 1 || len(*slept) != 0 {
		t.Errorf("refused prefetch must not fetch or wait: requests=%d slept=%v", s.count(), *slept)
	}
	// The prefetched thread is served from cache with no wait.
	if _, err := c.Thread(ctx, "linux", "1wr4fmd", reddit.Best, reddit.Fetch{}); err != nil {
		t.Fatal(err)
	}
	if s.count() != 1 || len(*slept) != 0 {
		t.Errorf("thread should come from the prefetch cache: requests=%d slept=%v", s.count(), *slept)
	}
	clock.Advance(time.Minute)
	if !c.Prefetch(ctx, "linux", "1wr4fmd") {
		t.Error("an already cached thread still reports success")
	}
	c.WaitPrefetch()
	if s.count() != 1 {
		t.Error("prefetching a cached thread must not refetch")
	}
}
