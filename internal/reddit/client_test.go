package reddit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type apiServer struct {
	t        *testing.T
	mu       sync.Mutex
	tokenHit int
	requests []*http.Request
	handler  func(w http.ResponseWriter, r *http.Request, n int) bool // return true if handled
}

func (s *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if r.URL.Path == "/api/v1/access_token" {
		s.tokenHit++
		n := s.tokenHit
		s.mu.Unlock()
		fmt.Fprintf(w, `{"access_token":"tok%d","token_type":"bearer","expires_in":86400}`, n)
		return
	}
	s.requests = append(s.requests, r)
	n := len(s.requests)
	s.mu.Unlock()
	if s.handler != nil && s.handler(w, r, n) {
		return
	}
	w.Header().Set("X-Ratelimit-Remaining", "95")
	w.Header().Set("X-Ratelimit-Reset", "300")
	switch {
	case strings.HasPrefix(r.URL.Path, "/r/linux/comments/"):
		http.ServeFile(w, r, "testdata/thread.json")
	case r.URL.Path == "/api/morechildren":
		http.ServeFile(w, r, "testdata/morechildren.json")
	default:
		http.ServeFile(w, r, "testdata/listing.json")
	}
}

func newTestClient(t *testing.T, srv *apiServer) (*Client, *httptest.Server, *fakeClock, *fakeSleeper) {
	t.Helper()
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	c := NewClient(Credentials{ClientID: "id", ClientSecret: "sec", UserAgent: "test-agent/1"},
		WithBaseURL(hs.URL), WithTokenURL(hs.URL+"/api/v1/access_token"), WithClock(clock.Now), WithSleep(sl.Sleep))
	return c, hs, clock, sl
}

func TestPostsRequestShape(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	l, err := c.Posts(context.Background(), "linux", Top, "t3_prev", Fetch{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Posts) != 2 {
		t.Errorf("posts = %d", len(l.Posts))
	}
	r := srv.requests[0]
	if r.URL.Path != "/r/linux/top" {
		t.Errorf("path = %s", r.URL.Path)
	}
	q := r.URL.Query()
	if q.Get("limit") != "100" || q.Get("after") != "t3_prev" || q.Get("raw_json") != "1" || q.Get("t") != "day" {
		t.Errorf("query = %v", q)
	}
	if r.Header.Get("Authorization") != "bearer tok1" || r.UserAgent() != "test-agent/1" {
		t.Errorf("headers = %v", r.Header)
	}
}

func TestPostsCacheAndFresh(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, clock, _ := newTestClient(t, srv)
	ctx := context.Background()
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	if len(srv.requests) != 1 {
		t.Fatalf("cache miss: %d requests", len(srv.requests))
	}
	c.Posts(ctx, "linux", Hot, "", Fetch{Fresh: true})
	if len(srv.requests) != 2 {
		t.Fatalf("Fresh should bypass cache: %d requests", len(srv.requests))
	}
	clock.Advance(6 * time.Minute)
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	if len(srv.requests) != 3 {
		t.Fatalf("listing TTL should be 5 minutes: %d requests", len(srv.requests))
	}
}

func TestCachedResultsAreIndependent(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	a, _ := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	a.Posts[0].Title = "mutated"
	b, _ := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	if b.Posts[0].Title == "mutated" {
		t.Error("cached value leaked a mutation")
	}
}

func TestRefreshesTokenOn401Once(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.WriteHeader(401)
			return true
		}
		return false
	}
	c, _, _, _ := newTestClient(t, srv)
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Fatal(err)
	}
	if srv.tokenHit != 2 || srv.requests[1].Header.Get("Authorization") != "bearer tok2" {
		t.Errorf("tokenHit=%d auth=%q", srv.tokenHit, srv.requests[1].Header.Get("Authorization"))
	}
}

func TestPersistent401IsAPIError(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool { w.WriteHeader(401); return true }
	c, _, _, _ := newTestClient(t, srv)
	_, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Errorf("err = %v", err)
	}
	if len(srv.requests) != 2 {
		t.Errorf("requests = %d, want exactly one retry", len(srv.requests))
	}
}

func TestRetriesOnceAfter429(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return true
		}
		return false
	}
	c, _, _, sl := newTestClient(t, srv)
	var waits []time.Duration
	c.onWait = func(d time.Duration) { waits = append(waits, d) }
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 1 || sl.slept[0] != 3*time.Second || len(waits) != 1 {
		t.Errorf("slept %v waits %v", sl.slept, waits)
	}
}

func TestForbiddenCarriesReason(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"reason":"private","message":"Forbidden","error":403}`)
		return true
	}
	c, _, _, _ := newTestClient(t, srv)
	_, err := c.Posts(context.Background(), "secret", Hot, "", Fetch{})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 403 || ae.Reason != "private" {
		t.Errorf("err = %v", err)
	}
}

func TestThreadAndSubtreeQueries(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	ctx := context.Background()
	if _, err := c.Thread(ctx, "linux", "aaa", Best, Fetch{}); err != nil {
		t.Fatal(err)
	}
	q := srv.requests[0].URL.Query()
	if srv.requests[0].URL.Path != "/r/linux/comments/aaa" || q.Get("sort") != "confidence" || q.Get("limit") != "500" || q.Get("depth") != "10" {
		t.Errorf("thread request = %s %v", srv.requests[0].URL.Path, q)
	}
	if _, err := c.Subtree(ctx, "linux", "aaa", "c4", TopComments); err != nil {
		t.Fatal(err)
	}
	q = srv.requests[1].URL.Query()
	if q.Get("comment") != "c4" || q.Get("context") != "0" || q.Get("sort") != "top" {
		t.Errorf("subtree query = %v", q)
	}
}

func TestMoreChildrenBatches(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i)
	}
	th, err := c.MoreChildren(context.Background(), "aaa", ids, Best)
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.requests) != 3 {
		t.Fatalf("requests = %d, want 3 batches", len(srv.requests))
	}
	sizes := []int{100, 100, 50}
	for i, r := range srv.requests {
		q := r.URL.Query()
		if r.URL.Path != "/api/morechildren" || q.Get("link_id") != "t3_aaa" || q.Get("api_type") != "json" || q.Get("sort") != "confidence" {
			t.Errorf("request %d = %s %v", i, r.URL.Path, q)
		}
		if got := len(strings.Split(q.Get("children"), ",")); got != sizes[i] {
			t.Errorf("batch %d size = %d, want %d", i, got, sizes[i])
		}
	}
	if len(th.Comments) != 6 || len(th.Stubs) != 3 {
		t.Errorf("things = %d comments %d stubs", len(th.Comments), len(th.Stubs))
	}
	c.MoreChildren(context.Background(), "t3_aaa", ids[:1], Best)
	if srv.requests[3].URL.Query().Get("link_id") != "t3_aaa" {
		t.Error("link_id should not be double-prefixed")
	}
}

func TestBodyLimit(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		w.Write([]byte(strings.Repeat("x", 11<<20)))
		return true
	}
	c, _, _, _ := newTestClient(t, srv)
	_, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("oversized body should be rejected, got %v", err)
	}
}

func TestBadBodyNotCachedAndSinkCalled(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			fmt.Fprint(w, `{"kind":"Listing","data":{"children":[{"kind":"t3","data":"not an object"}]}}`)
			return true
		}
		return false
	}
	c, _, _, _ := newTestClient(t, srv)
	var got []byte
	c.sink = func(b []byte) { got = b }
	_, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want ParseError, got %v", err)
	}
	if !strings.Contains(string(got), "not an object") {
		t.Errorf("sink did not receive the body: %q", got)
	}
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Errorf("second call should refetch, not serve the bad body: %v", err)
	}
	if len(srv.requests) != 2 {
		t.Errorf("requests = %d, want 2", len(srv.requests))
	}
}

func TestRetryAfterResetsGate(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.Header().Set("X-Ratelimit-Remaining", "0")
			w.Header().Set("X-Ratelimit-Reset", "300")
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return true
		}
		return false
	}
	c, _, _, sl := newTestClient(t, srv)
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 1 || sl.slept[0] != 3*time.Second {
		t.Errorf("should wait Retry-After only, slept %v", sl.slept)
	}
}
