package reddit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func tokenServer(t *testing.T, hits *int32, expires int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/access_token" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "myid" || secret != "mysecret" {
			w.WriteHeader(401)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("bad form: %v", r.Form)
		}
		if r.UserAgent() != "test-agent/1" {
			t.Errorf("user agent = %q", r.UserAgent())
		}
		n := atomic.AddInt32(hits, 1)
		fmt.Fprintf(w, `{"access_token":"tok%d","token_type":"bearer","expires_in":%d,"scope":"*"}`, n, expires)
	}))
}

func TestTokenCachedUntilNearExpiry(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	ts := NewTokenSource(TokenConfig{ClientID: "myid", ClientSecret: "mysecret", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token", Now: clock.Now})
	tok, err := ts.Token(context.Background())
	if err != nil || tok != "tok1" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	clock.Advance(3000 * time.Second)
	if tok, _ := ts.Token(context.Background()); tok != "tok1" {
		t.Errorf("token should still be cached, got %q", tok)
	}
	clock.Advance(550 * time.Second) // 50 s left: inside the 60 s refresh window
	if tok, _ := ts.Token(context.Background()); tok != "tok2" {
		t.Errorf("token should refresh near expiry, got %q", tok)
	}
	if hits != 2 {
		t.Errorf("token endpoint hit %d times, want 2", hits)
	}
}

func TestInvalidateOnlyMatchingToken(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	ts := NewTokenSource(TokenConfig{ClientID: "myid", ClientSecret: "mysecret", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token"})
	tok, _ := ts.Token(context.Background())
	ts.Invalidate("stale")
	if again, _ := ts.Token(context.Background()); again != tok {
		t.Error("invalidating a stale token must not refresh")
	}
	ts.Invalidate(tok)
	if again, _ := ts.Token(context.Background()); again == tok {
		t.Error("invalidating the current token must refresh")
	}
}

func TestTokenBadCredentials(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	ts := NewTokenSource(TokenConfig{ClientID: "wrong", ClientSecret: "x", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token"})
	if _, err := ts.Token(context.Background()); err == nil {
		t.Error("expected error for rejected credentials")
	}
}
