package rss

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

// DefaultBaseURL is where the public feeds live.
const DefaultBaseURL = "https://www.reddit.com"

const (
	listingTTL   = 10 * time.Minute
	threadTTL    = 15 * time.Minute
	maxAge       = 24 * time.Hour  // older disk entries are not served
	staleHold    = 2 * time.Minute // how long a stale disk entry is held in memory
	cacheEntries = 200
	maxBody      = 10 << 20
	feedsPerMin  = 1 // Reddit's observed allowance for unauthenticated feeds
)

// job is one fetch the background syncer can perform.
type job struct {
	url      string
	ttl      time.Duration
	validate func([]byte) error
}

// Client reads Reddit's Atom feeds. It satisfies reddit.Store. Feeds allow
// about one request a minute, so responses are cached, identical requests in
// flight are shared, and a spare slot can prefetch the thread the user is
// likely to open next without ever delaying a foreground request.
type Client struct {
	base   string
	http   *http.Client
	ua     string
	gate   *reddit.RateGate
	cache  *reddit.Cache
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)

	mu      sync.Mutex
	calls   map[string]*call // in-flight fetches by URL
	refresh []job            // stale entries a reader opened, to refresh first
	queued  map[string]bool

	disk *Disk        // optional persistent store
	fg   atomic.Int32 // foreground requests waiting for the allowance

	bgCtx    context.Context
	bgCancel context.CancelFunc
	bg       sync.WaitGroup
}

// call is one in-flight fetch that several readers may wait on.
type call struct {
	done chan struct{}
	b    []byte
	err  error
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the feed host (tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.base = strings.TrimRight(u, "/") } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithClock overrides the clock.
func WithClock(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// WithSleep overrides how the client waits (tests).
func WithSleep(s func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.sleep = s }
}

// WithOnWait sets a callback invoked with the duration before a foreground rate-limit wait.
func WithOnWait(f func(time.Duration)) Option { return func(c *Client) { c.onWait = f } }

// NewClient builds a feed client identified by userAgent.
func NewClient(userAgent string, opts ...Option) *Client {
	c := &Client{base: DefaultBaseURL, http: &http.Client{Timeout: 15 * time.Second}, ua: userAgent, now: time.Now, sleep: reddit.SleepContext, calls: map[string]*call{}, queued: map[string]bool{}}
	for _, o := range opts {
		o(c)
	}
	c.gate = reddit.NewRateGate(c.now, c.sleep, func(d time.Duration) {
		if c.onWait != nil {
			c.onWait(d)
		}
	})
	c.gate.SetAllowance(feedsPerMin)
	c.cache = reddit.NewCache(cacheEntries, c.now)
	c.bgCtx, c.bgCancel = context.WithCancel(context.Background())
	return c
}

// UseDisk attaches a persistent store: fetched feeds are written to it and
// reads are served from it, so nothing waits for the network if it is there.
func (c *Client) UseDisk(d *Disk) { c.disk = d }

// key is the disk key for a URL: the path and query, without the host.
func (c *Client) key(u string) string { return strings.TrimPrefix(u, c.base) }

// age reports how long ago u was fetched, according to the disk store.
func (c *Client) age(u string) (time.Duration, bool) {
	if c.disk == nil {
		return 0, false
	}
	fetched, ok := c.disk.Fetched(c.key(u))
	if !ok {
		return 0, false
	}
	return c.now().Sub(fetched), true
}

// Cached reports whether a post's thread can be shown without the network.
func (c *Client) Cached(sub, postID string) bool {
	u := c.threadURL(sub, postID)
	if _, ok := c.cache.Get(u); ok {
		return true
	}
	age, ok := c.age(u)
	return ok && age <= maxAge
}

// Stats summarises the disk store; zero without one.
func (c *Client) Stats() DiskStats {
	if c.disk == nil {
		return DiskStats{}
	}
	return c.disk.Stats(c.now())
}

// PendingRefresh is how many stale entries are waiting for the syncer.
func (c *Client) PendingRefresh() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.refresh)
}

func (c *Client) enqueue(j job) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.queued[j.url] {
		c.queued[j.url] = true
		c.refresh = append(c.refresh, j)
	}
}

func (c *Client) popRefresh() (job, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.refresh) == 0 {
		return job{}, false
	}
	j := c.refresh[0]
	c.refresh = c.refresh[1:]
	delete(c.queued, j.url)
	return j, true
}

// fromDisk serves u from the persistent store when it is recent enough,
// queuing a background refresh if it is past its freshness.
func (c *Client) fromDisk(u string, ttl time.Duration, validate func([]byte) error) ([]byte, bool) {
	if c.disk == nil {
		return nil, false
	}
	b, fetched, ok := c.disk.Get(c.key(u))
	if !ok {
		return nil, false
	}
	age := c.now().Sub(fetched)
	if age > maxAge || validate(b) != nil {
		return nil, false
	}
	if age < ttl {
		c.cache.Put(u, b, ttl-age)
	} else {
		hold := staleHold
		if left := maxAge - age; left < hold {
			hold = left // never keep an entry in memory past its maximum age
		}
		c.cache.Put(u, b, hold)
		c.enqueue(job{url: u, ttl: ttl, validate: validate})
	}
	return b, true
}

// runJob performs a background fetch using a reservation the caller already
// holds. If the URL is already being fetched the reservation is returned.
func (c *Client) runJob(ctx context.Context, j job) error {
	c.mu.Lock()
	if cl, busy := c.calls[j.url]; busy {
		c.mu.Unlock()
		c.gate.Unreserve()
		_, err := cl.wait(ctx)
		return err
	}
	cl := &call{done: make(chan struct{})}
	c.calls[j.url] = cl
	c.mu.Unlock()
	c.run(ctx, j.url, cl, j.ttl, j.validate, true, true)
	return cl.err
}

// Close cancels background prefetches and waits for them to stop.
func (c *Client) Close() {
	c.bgCancel()
	c.bg.Wait()
}

// WaitPrefetch blocks until background prefetches finish (tests).
func (c *Client) WaitPrefetch() { c.bg.Wait() }

func (c *Client) listingURL(sub string, sort reddit.Sort, after string) string {
	q := url.Values{"limit": {"100"}}
	if after != "" {
		q.Set("after", after)
	}
	if sort == reddit.Top {
		q.Set("t", "day")
	}
	return c.base + "/r/" + sub + "/" + string(sort) + "/.rss?" + q.Encode()
}

func (c *Client) threadURL(sub, postID string) string {
	return c.base + "/r/" + sub + "/comments/" + postID + "/.rss?limit=500"
}

func validListing(b []byte) error { _, err := ParseListing(bytes.NewReader(b)); return err }
func validThread(b []byte) error  { _, err := ParseThread(bytes.NewReader(b)); return err }

// Posts implements reddit.Store.
func (c *Client) Posts(ctx context.Context, sub string, sort reddit.Sort, after string, f reddit.Fetch) (reddit.Listing, error) {
	b, err := c.fetch(ctx, c.listingURL(sub, sort, after), f.Fresh, listingTTL, validListing, false)
	if err != nil {
		return reddit.Listing{}, err
	}
	return ParseListing(bytes.NewReader(b))
}

// Thread implements reddit.Store.
func (c *Client) Thread(ctx context.Context, sub, postID string, _ reddit.CommentSort, f reddit.Fetch) (reddit.Thread, error) {
	b, err := c.fetch(ctx, c.threadURL(sub, postID), f.Fresh, threadTTL, validThread, false)
	if err != nil {
		return reddit.Thread{}, err
	}
	return ParseThread(bytes.NewReader(b))
}

// Subtree implements reddit.Store; feeds have no subtrees, so it is the thread.
func (c *Client) Subtree(ctx context.Context, sub, postID, _ string, sort reddit.CommentSort) (reddit.Thread, error) {
	return c.Thread(ctx, sub, postID, sort, reddit.Fetch{})
}

// MoreChildren implements reddit.Store; feeds never produce stubs.
func (c *Client) MoreChildren(context.Context, string, []string, reddit.CommentSort) (reddit.Things, error) {
	return reddit.Things{}, nil
}

// Prefetch loads a thread into the cache in the background if the feed
// allowance has a spare slot right now. It never waits, makes one attempt,
// and reports whether the thread is cached, in flight, or now being fetched.
func (c *Client) Prefetch(_ context.Context, sub, postID string) bool {
	u := c.threadURL(sub, postID)
	if _, ok := c.cache.Get(u); ok {
		return true
	}
	c.mu.Lock()
	if _, busy := c.calls[u]; busy {
		c.mu.Unlock()
		return true
	}
	if !c.gate.TryAcquire() {
		c.mu.Unlock()
		return false
	}
	cl := &call{done: make(chan struct{})}
	c.calls[u] = cl
	c.mu.Unlock()
	c.bg.Add(1)
	go func() {
		defer c.bg.Done()
		c.run(c.bgCtx, u, cl, threadTTL, validThread, true, true)
	}()
	return true
}

// fetch returns the body for u, from cache unless fresh, sharing any in-flight
// request for the same URL. A newly fetched body is validated and cached.
func (c *Client) fetch(ctx context.Context, u string, fresh bool, ttl time.Duration, validate func([]byte) error, quiet bool) ([]byte, error) {
	if !fresh {
		if b, ok := c.cache.Get(u); ok {
			return b, nil
		}
		if b, ok := c.fromDisk(u, ttl, validate); ok {
			return b, nil
		}
	}
	c.mu.Lock()
	if cl, ok := c.calls[u]; ok {
		c.mu.Unlock()
		return cl.wait(ctx)
	}
	cl := &call{done: make(chan struct{})}
	c.calls[u] = cl
	c.mu.Unlock()
	c.run(ctx, u, cl, ttl, validate, false, quiet)
	return cl.b, cl.err
}

func (cl *call) wait(ctx context.Context) ([]byte, error) {
	select {
	case <-cl.done:
		return cl.b, cl.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// run performs the fetch for cl, publishes the result and clears the entry.
// reserved says the caller already holds a gate reservation (prefetch), which
// also means a single attempt with no retry on 429.
func (c *Client) run(ctx context.Context, u string, cl *call, ttl time.Duration, validate func([]byte) error, reserved, quiet bool) {
	defer func() {
		c.mu.Lock()
		delete(c.calls, u)
		c.mu.Unlock()
		close(cl.done)
	}()
	b, err := c.do(ctx, u, reserved, quiet)
	if err == nil {
		if verr := validate(b); verr != nil {
			err = &reddit.ParseError{Err: verr}
		} else {
			c.cache.Put(u, b, ttl)
			if c.disk != nil {
				_ = c.disk.Put(c.key(u), b, c.now()) // best effort: memory still has it
			}
		}
	}
	cl.b, cl.err = b, err
}

// do sends one GET, waiting for the feed allowance unless reserved, and
// retrying once after a 429 for foreground requests.
func (c *Client) do(ctx context.Context, u string, reserved, quiet bool) ([]byte, error) {
	retried := reserved // prefetches never retry
	for {
		if !reserved {
			c.fg.Add(1)
			err := c.gate.Acquire(ctx)
			c.fg.Add(-1)
			if err != nil {
				return nil, err
			}
		}
		reserved = false
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			c.gate.Release(nil)
			return nil, err
		}
		req.Header.Set("User-Agent", c.ua)
		req.Header.Set("Accept", "application/atom+xml, application/xml;q=0.9, */*;q=0.5")
		resp, err := c.http.Do(req)
		if err != nil {
			c.gate.Release(nil)
			return nil, fmt.Errorf("request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		resp.Body.Close()
		c.gate.Release(resp.Header)
		if readErr == nil && len(body) > maxBody {
			readErr = fmt.Errorf("feed larger than %d bytes", maxBody)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, readErr
		case resp.StatusCode == http.StatusTooManyRequests && !retried:
			retried = true
			wait := c.gate.WaitFor(resp.Header)
			if c.onWait != nil && !quiet {
				c.onWait(wait)
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			if resp.Header.Get("Retry-After") != "" {
				c.gate.Reset()
			}
		default:
			return nil, &reddit.APIError{Status: resp.StatusCode}
		}
	}
}

var _ reddit.Store = (*Client)(nil)
