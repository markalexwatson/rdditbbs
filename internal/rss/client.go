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
	"time"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

// DefaultBaseURL is where the public feeds live.
const DefaultBaseURL = "https://www.reddit.com"

const (
	listingTTL   = 10 * time.Minute
	threadTTL    = 15 * time.Minute
	cacheEntries = 200
	maxBody      = 10 << 20
	feedsPerMin  = 1 // Reddit's observed allowance for unauthenticated feeds
)

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

	mu    sync.Mutex
	calls map[string]*call // in-flight fetches by URL

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
	c := &Client{base: DefaultBaseURL, http: &http.Client{Timeout: 15 * time.Second}, ua: userAgent, now: time.Now, sleep: reddit.SleepContext, calls: map[string]*call{}}
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
			if err := c.gate.Acquire(ctx); err != nil {
				return nil, err
			}
			if b, ok := c.cache.Get(u); ok { // filled while we waited
				c.gate.Release(nil)
				return b, nil
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
