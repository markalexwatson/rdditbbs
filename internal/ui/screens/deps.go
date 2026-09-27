// Package screens implements each BBS screen on top of the ui package.
package screens

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/markalexwatson/redditbbs/internal/config"
	"github.com/markalexwatson/redditbbs/internal/reddit"
	"github.com/markalexwatson/redditbbs/internal/session"
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/ui"
)

// Deps is everything screens need from the outside. Shared by pointer so
// New User Setup can install the Store.
type Deps struct {
	Store     reddit.Store
	MakeStore func(id, secret string) reddit.Store
	Config    *config.Config
	Session   *session.Session
	Open      func(url string, onExit func(error)) error
	Now       func() time.Time
	Version   string
	Demo      bool // sample data, no credentials: skip setup and say so on the splash
}

// SelectComment is the Pop result Message Reader hands back to Thread Index.
type SelectComment struct{ ID string }

// linkExit reports the browser opener's exit for a link a screen opened.
type linkExit struct {
	URL string
	Err error
}

// openInBrowser starts the OS opener and returns a Run that reports its exit,
// or nil when it could not start (the caller shows the URL in that case).
func (d *Deps) openInBrowser(u string) (ui.Action, error) {
	done := make(chan error, 1)
	if err := d.Open(u, func(err error) { done <- err }); err != nil {
		return nil, err
	}
	d.Session.LinksOpened++
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		select {
		case err := <-done:
			return linkExit{URL: u, Err: err}
		case <-ctx.Done():
			return nil
		}
	}}, nil
}

// placeholder stands in for a screen a later task provides, so each task's
// tests can assert that navigation left the current screen.
type placeholder struct{ name string }

func (p *placeholder) Init() ui.Action                { return nil }
func (p *placeholder) Draw(c term.Canvas)             { c.Text(2, 0, "placeholder: "+p.name, term.Style{}, 60) }
func (p *placeholder) HandleKey(k term.Key) ui.Action { return ui.Pop{} }
func (p *placeholder) Update(ui.Msg) ui.Action        { return nil }
func (p *placeholder) Title() string                  { return p.name }
func (p *placeholder) Keys() []ui.KeyHelp             { return nil }

// Rune returns the upper-cased rune of a typed (not pasted) rune key, else 0.
func Rune(k term.Key) rune {
	if k.Code != term.KeyRune || k.Paste {
		return 0
	}
	return unicode.ToUpper(k.Rune)
}

// IsBack reports Q or Escape.
func IsBack(k term.Key) bool { return k.Code == term.KeyEscape || Rune(k) == 'Q' }

var subredditRe = regexp.MustCompile(`^[A-Za-z0-9_]{2,21}$`)

// validSubreddit trims whitespace and a leading r/ and validates the name.
func validSubreddit(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(strings.TrimPrefix(name, "/r/"), "r/")
	return name, subredditRe.MatchString(name)
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// isAuthError reports a 401 so screens can offer the setup screen.
func isAuthError(err error) bool {
	var ae *reddit.APIError
	return errors.As(err, &ae) && ae.Status == 401
}

// errText shortens an error for the status line.
func errText(err error) string {
	var pe *reddit.ParseError
	if errors.As(err, &pe) {
		return "Unexpected response from Reddit"
	}
	if errors.Is(err, context.Canceled) {
		return "Cancelled"
	}
	if ae, ok := err.(*reddit.APIError); ok {
		switch ae.Status {
		case 401:
			return "Credentials rejected. Press L to log in again"
		case 403:
			if ae.Reason != "" {
				return "Access denied (" + ae.Reason + ")"
			}
			return "Access denied"
		case 404:
			return "No such area"
		case 429:
			return "Rate limited by Reddit"
		}
		if ae.Status >= 500 {
			return "Reddit is having trouble (HTTP " + itoa(ae.Status) + ")"
		}
	}
	s := err.Error()
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
