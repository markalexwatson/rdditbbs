package reddit

import (
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

// DefaultTokenURL is Reddit's OAuth token endpoint.
const DefaultTokenURL = "https://www.reddit.com/api/v1/access_token"

// TokenConfig configures a TokenSource. Zero fields take defaults.
type TokenConfig struct {
	ClientID, ClientSecret string
	UserAgent              string
	URL                    string
	HTTP                   *http.Client
	Now                    func() time.Time
}

// TokenSource obtains and refreshes an app-only bearer token.
type TokenSource struct {
	cfg TokenConfig

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewTokenSource creates a token source with the client-credentials grant.
func NewTokenSource(cfg TokenConfig) *TokenSource {
	if cfg.URL == "" {
		cfg.URL = DefaultTokenURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &TokenSource{cfg: cfg}
}

// Token returns a valid bearer token, refreshing when fewer than 60 seconds remain.
func (t *TokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && t.cfg.Now().Add(60*time.Second).Before(t.expiry) {
		return t.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(t.cfg.ClientID, t.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", t.cfg.UserAgent)
	resp, err := t.cfg.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", &APIError{Status: resp.StatusCode, Reason: reason(body)} // typed so screens can offer login again
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: HTTP %d", resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token response: %s", firstNonEmpty(tr.Error, "no access_token"))
	}
	t.token = tr.AccessToken
	t.expiry = t.cfg.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return t.token, nil
}

// Invalidate discards the current token if it is the one that was rejected,
// so several callers seeing the same 401 cause one refresh.
func (t *TokenSource) Invalidate(rejected string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == rejected {
		t.token = ""
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
