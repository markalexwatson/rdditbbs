package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the OAuth API host.
const DefaultBaseURL = "https://oauth.reddit.com"

const (
	maxBody       = 10 << 20
	listingTTL    = 5 * time.Minute
	threadTTL     = 10 * time.Minute
	cacheEntries  = 200
	moreBatchSize = 100
)

// Credentials identify the registered script app.
type Credentials struct {
	ClientID, ClientSecret, UserAgent string
}

// ParseError is a 200 response whose body could not be decoded.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "unexpected response from Reddit: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// APIError is a non-2xx response.
type APIError struct {
	Status int
	Reason string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("HTTP %d (%s)", e.Status, e.Reason)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// Client talks to the Reddit API. It satisfies Store.
type Client struct {
	base   string
	http   *http.Client
	ua     string
	tokens *TokenSource
	gate   *RateGate
	cache  *Cache
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)
	sink   func([]byte) // receives bodies that failed to parse
	moreMu sync.Mutex

	tokenURL string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API host (tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.base = strings.TrimRight(u, "/") } }

// WithTokenURL overrides the token endpoint (tests).
func WithTokenURL(u string) Option { return func(c *Client) { c.tokenURL = u } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithClock overrides the clock.
func WithClock(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// WithSleep overrides how the client waits (tests).
func WithSleep(s func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.sleep = s }
}

// WithOnWait sets a callback invoked with the duration before any rate-limit wait.
func WithOnWait(f func(time.Duration)) Option { return func(c *Client) { c.onWait = f } }

// WithBadResponseSink receives (at most 64 KB of) any 200 body that fails to parse.
func WithBadResponseSink(f func([]byte)) Option { return func(c *Client) { c.sink = f } }

// NewClient builds a client with token handling, rate gating and caching.
func NewClient(cr Credentials, opts ...Option) *Client {
	c := &Client{
		base:     DefaultBaseURL,
		http:     &http.Client{Timeout: 15 * time.Second},
		ua:       cr.UserAgent,
		now:      time.Now,
		sleep:    SleepContext,
		tokenURL: DefaultTokenURL,
	}
	for _, o := range opts {
		o(c)
	}
	c.tokens = NewTokenSource(TokenConfig{ClientID: cr.ClientID, ClientSecret: cr.ClientSecret, UserAgent: cr.UserAgent, URL: c.tokenURL, HTTP: c.http, Now: c.now})
	c.gate = NewRateGate(c.now, c.sleep, func(d time.Duration) {
		if c.onWait != nil {
			c.onWait(d)
		}
	})
	c.cache = NewCache(cacheEntries, c.now)
	return c
}

// Verify proves the credentials work by fetching a public listing.
func (c *Client) Verify(ctx context.Context) error {
	_, err := c.Posts(ctx, "linux", Hot, "", Fetch{Fresh: true})
	return err
}

// Posts fetches one page of a subreddit.
func (c *Client) Posts(ctx context.Context, subreddit string, sort Sort, after string, f Fetch) (Listing, error) {
	q := url.Values{"limit": {"100"}}
	if after != "" {
		q.Set("after", after)
	}
	if sort == Top {
		q.Set("t", "day")
	}
	b, key, err := c.get(ctx, "/r/"+subreddit+"/"+string(sort), q, f.Fresh, listingTTL)
	if err != nil {
		return Listing{}, err
	}
	l, err := ParseListing(bytes.NewReader(b))
	return l, c.finish(key, b, listingTTL, err)
}

// Thread fetches a post and its comment forest.
func (c *Client) Thread(ctx context.Context, subreddit, postID string, sort CommentSort, f Fetch) (Thread, error) {
	q := url.Values{"sort": {sort.API()}, "limit": {"500"}, "depth": {"10"}}
	b, key, err := c.get(ctx, "/r/"+subreddit+"/comments/"+postID, q, f.Fresh, threadTTL)
	if err != nil {
		return Thread{}, err
	}
	th, err := ParseThread(bytes.NewReader(b))
	return th, c.finish(key, b, threadTTL, err)
}

// Subtree fetches the thread rooted at one comment.
func (c *Client) Subtree(ctx context.Context, subreddit, postID, commentID string, sort CommentSort) (Thread, error) {
	q := url.Values{"sort": {sort.API()}, "limit": {"500"}, "depth": {"10"}, "comment": {commentID}, "context": {"0"}}
	b, key, err := c.get(ctx, "/r/"+subreddit+"/comments/"+postID, q, false, threadTTL)
	if err != nil {
		return Thread{}, err
	}
	th, err := ParseThread(bytes.NewReader(b))
	return th, c.finish(key, b, threadTTL, err)
}

// MoreChildren loads comments by ID in batches of 100, one request at a time.
func (c *Client) MoreChildren(ctx context.Context, linkFullname string, ids []string, sort CommentSort) (Things, error) {
	c.moreMu.Lock()
	defer c.moreMu.Unlock()
	link := "t3_" + strings.TrimPrefix(linkFullname, "t3_")
	var all Things
	for start := 0; start < len(ids); start += moreBatchSize {
		end := start + moreBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		q := url.Values{
			"link_id":  {link},
			"children": {strings.Join(ids[start:end], ",")},
			"sort":     {sort.API()},
			"api_type": {"json"},
		}
		b, _, err := c.get(ctx, "/api/morechildren", q, true, 0)
		if err != nil {
			return all, err
		}
		th, err := ParseMoreChildren(bytes.NewReader(b))
		if err = c.finish("", b, 0, err); err != nil {
			return all, err
		}
		all.Comments = append(all.Comments, th.Comments...)
		all.Stubs = append(all.Stubs, th.Stubs...)
	}
	return all, nil
}

// get performs a GET, serving from cache unless fresh. It returns the body
// and the cache key; the caller stores the body with finish once it parses.
func (c *Client) get(ctx context.Context, path string, q url.Values, fresh bool, ttl time.Duration) ([]byte, string, error) {
	q.Set("raw_json", "1")
	u := c.base + path + "?" + q.Encode()
	if !fresh && ttl > 0 {
		if b, ok := c.cache.Get(u); ok {
			return b, "", nil // already cached: finish must not re-store
		}
	}
	b, err := c.do(ctx, u)
	if err != nil {
		return nil, "", err
	}
	return b, u, nil
}

// finish caches a body that parsed successfully, or reports a parse failure
// to the sink and wraps it as a ParseError. key "" means nothing to cache.
func (c *Client) finish(key string, b []byte, ttl time.Duration, err error) error {
	if err != nil {
		if c.sink != nil {
			if len(b) > 64<<10 {
				b = b[:64<<10]
			}
			c.sink(b)
		}
		return &ParseError{Err: err}
	}
	if key != "" && ttl > 0 {
		c.cache.Put(key, b, ttl)
	}
	return nil
}

// do sends one authenticated GET, refreshing the token once on 401 and
// retrying once after a 429.
func (c *Client) do(ctx context.Context, u string) ([]byte, error) {
	retried401, retried429 := false, false
	for {
		tok, err := c.tokens.Token(ctx)
		if err != nil {
			return nil, err
		}
		if err := c.gate.Acquire(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			c.gate.Release(nil)
			return nil, err
		}
		req.Header.Set("Authorization", "bearer "+tok)
		req.Header.Set("User-Agent", c.ua)
		resp, err := c.http.Do(req)
		if err != nil {
			c.gate.Release(nil)
			return nil, fmt.Errorf("request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		resp.Body.Close()
		c.gate.Release(resp.Header)
		if readErr == nil && len(body) > maxBody {
			readErr = fmt.Errorf("response larger than %d bytes", maxBody)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, readErr
		case resp.StatusCode == http.StatusUnauthorized && !retried401:
			retried401 = true
			c.tokens.Invalidate(tok)
		case resp.StatusCode == http.StatusTooManyRequests && !retried429:
			retried429 = true
			wait := c.gate.WaitFor(resp.Header)
			if c.onWait != nil {
				c.onWait(wait)
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			c.gate.Reset() // Retry-After takes precedence over the recorded reset
		default:
			return nil, &APIError{Status: resp.StatusCode, Reason: reason(body)}
		}
	}
}

// reason extracts Reddit's "reason" field from an error body, if any.
func reason(body []byte) string {
	var r struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(body, &r) == nil {
		return r.Reason
	}
	return ""
}

var _ Store = (*Client)(nil)
