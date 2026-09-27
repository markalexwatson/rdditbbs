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
)

// Client reads Reddit's Atom feeds. It satisfies reddit.Store. Feeds allow
// about one request a minute, so everything is cached and a spare slot can be
// used to prefetch the thread the user is likely to open next.
type Client struct {
	base   string
	http   *http.Client
	ua     string
	gate   *reddit.RateGate
	cache  *reddit.Cache
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)

	prefetch sync.WaitGroup
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

// WithOnWait sets a callback invoked with the duration before a rate-limit wait.
func WithOnWait(f func(time.Duration)) Option { return func(c *Client) { c.onWait = f } }

// NewClient builds a feed client identified by userAgent.
func NewClient(userAgent string, opts ...Option) *Client {
	c := &Client{base: DefaultBaseURL, http: &http.Client{Timeout: 15 * time.Second}, ua: userAgent, now: time.Now, sleep: reddit.SleepContext}
	for _, o := range opts {
		o(c)
	}
	c.gate = reddit.NewRateGate(c.now, c.sleep, func(d time.Duration) {
		if c.onWait != nil {
			c.onWait(d)
		}
	})
	c.cache = reddit.NewCache(cacheEntries, c.now)
	return c
}

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

// Posts implements reddit.Store.
func (c *Client) Posts(ctx context.Context, sub string, sort reddit.Sort, after string, f reddit.Fetch) (reddit.Listing, error) {
	b, err := c.get(ctx, c.listingURL(sub, sort, after), f.Fresh, false)
	if err != nil {
		return reddit.Listing{}, err
	}
	l, err := ParseListing(bytes.NewReader(b))
	if err != nil {
		return reddit.Listing{}, &reddit.ParseError{Err: err}
	}
	c.cache.Put(c.listingURL(sub, sort, after), b, listingTTL)
	return l, nil
}

// Thread implements reddit.Store.
func (c *Client) Thread(ctx context.Context, sub, postID string, _ reddit.CommentSort, f reddit.Fetch) (reddit.Thread, error) {
	b, err := c.get(ctx, c.threadURL(sub, postID), f.Fresh, false)
	if err != nil {
		return reddit.Thread{}, err
	}
	th, err := ParseThread(bytes.NewReader(b))
	if err != nil {
		return reddit.Thread{}, &reddit.ParseError{Err: err}
	}
	c.cache.Put(c.threadURL(sub, postID), b, threadTTL)
	return th, nil
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
// allowance has a spare slot right now. It never waits and never delays a
// user's request. It reports whether the thread is, or will be, cached.
func (c *Client) Prefetch(ctx context.Context, sub, postID string) bool {
	u := c.threadURL(sub, postID)
	if _, ok := c.cache.Get(u); ok {
		return true
	}
	if !c.gate.TryAcquire() {
		return false
	}
	c.prefetch.Add(1)
	go func() {
		defer c.prefetch.Done()
		b, err := c.get(ctx, u, true, true)
		if err != nil {
			return
		}
		if _, err := ParseThread(bytes.NewReader(b)); err == nil {
			c.cache.Put(u, b, threadTTL)
		}
	}()
	return true
}

// WaitPrefetch blocks until background prefetches finish (tests and shutdown).
func (c *Client) WaitPrefetch() { c.prefetch.Wait() }

// get fetches u, serving from cache unless fresh. reserved says the caller
// already holds a gate reservation (prefetch).
func (c *Client) get(ctx context.Context, u string, fresh, reserved bool) ([]byte, error) {
	if !fresh {
		if b, ok := c.cache.Get(u); ok {
			return b, nil
		}
	}
	retried := false
	for {
		if !reserved {
			if err := c.gate.Acquire(ctx); err != nil {
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
			if c.onWait != nil {
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
