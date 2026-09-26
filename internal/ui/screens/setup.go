package screens

import (
	"context"
	"errors"
	"strings"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

var errNoStoreFactory = errors.New("no store factory configured")

// Setup collects and verifies app credentials on first run, or again after a
// 401 when pushed from another screen (nested).
type Setup struct {
	d          *Deps
	nested     bool
	id, secret widgets.TextInput
	focus      int
	busy       bool
	gen        int
	status     string
	statusErr  bool
	lastErr    error
}

type setupMsg struct {
	gen   int
	store reddit.Store
	err   error
}

// NewSetup creates the first-run setup screen, which replaces itself with the
// Main Menu on success.
func NewSetup(d *Deps) *Setup {
	s := &Setup{d: d}
	s.secret.Mask = true
	return s
}

// NewNestedSetup creates a setup screen pushed over another screen; on
// success it pops back with a nil result and on Escape it pops.
func NewNestedSetup(d *Deps) *Setup {
	s := NewSetup(d)
	s.nested = true
	return s
}

func (s *Setup) Init() ui.Action    { return nil }
func (s *Setup) Title() string      { return "New User Setup" }
func (s *Setup) CapturesKeys() bool { return true }
func (s *Setup) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{Key: "Tab", Desc: "Next field"}, {Key: "⏎", Desc: "Continue"}, {Key: "Esc", Desc: "Quit"}}
}

func (s *Setup) Prompt() widgets.Prompt {
	if s.busy && s.status == "" {
		return widgets.Prompt{Status: "Checking credentials with Reddit..."}
	}
	return widgets.Prompt{Status: s.status, Error: s.statusErr}
}

var setupText = []string{
	"Welcome, new user! RedditBBS reads Reddit through its official API,",
	"which needs a free 'script' app registered to your Reddit account.",
	"",
	"  1. Visit https://www.reddit.com/prefs/apps and choose 'create app'.",
	"  2. Pick the type 'script'. Any redirect URI will do, e.g. http://localhost:8080",
	"  3. Copy the client ID (shown under the app name) and the secret below.",
	"",
	"Reddit may need to approve the app before requests succeed.",
	"Credentials are stored with mode 0600 in your config file.",
}

func (s *Setup) Draw(c term.Canvas) {
	w, _ := c.Size()
	for i, l := range setupText {
		st := theme.Style(theme.Body)
		if i == 0 {
			st = theme.Style(theme.Subject)
		}
		c.Text(2, i, l, st, w-2)
	}
	y := len(setupText) + 1
	fieldX, fieldW := 18, w-20
	c.Text(2, y, "Client ID:", theme.Style(theme.Heading), w)
	c.Text(2, y+2, "Client secret:", theme.Style(theme.Heading), w)
	c.Fill(fieldX, y, fieldW, 1, '_', theme.Style(theme.Meta))
	c.Fill(fieldX, y+2, fieldW, 1, '_', theme.Style(theme.Meta))
	s.id.Draw(c, fieldX, y, fieldW, theme.Style(theme.Subject), s.focus == 0 && !s.busy)
	s.secret.Draw(c, fieldX, y+2, fieldW, theme.Style(theme.Subject), s.focus == 1 && !s.busy)
}

func (s *Setup) HandleKey(k term.Key) ui.Action {
	if k.Code == term.KeyEscape && !k.Paste {
		if s.nested {
			return ui.Pop{}
		}
		return ui.Quit{}
	}
	if s.busy {
		return nil
	}
	if !k.Paste {
		switch k.Code {
		case term.KeyTab:
			s.focus = 1 - s.focus
			return nil
		case term.KeyEnter:
			if s.focus == 0 {
				s.focus = 1
				return nil
			}
			return s.submit()
		}
	}
	if s.focus == 0 {
		s.id.HandleKey(k)
	} else {
		s.secret.HandleKey(k)
	}
	return nil
}

// submit verifies the typed credentials asynchronously.
func (s *Setup) submit() ui.Action {
	id, secret := strings.TrimSpace(s.id.Value), strings.TrimSpace(s.secret.Value)
	if id == "" || secret == "" {
		s.status, s.statusErr = "Both fields are required", true
		return nil
	}
	if s.d.MakeStore == nil {
		s.lastErr = errNoStoreFactory
		s.status, s.statusErr = "Internal error: no Reddit client available", true
		return nil
	}
	store := s.d.MakeStore(id, secret)
	s.busy = true
	s.status, s.statusErr = "", false
	s.gen++
	gen := s.gen
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		return setupMsg{gen: gen, store: store, err: verifyStore(ctx, store)}
	}}
}

func verifyStore(ctx context.Context, store reddit.Store) error {
	if v, ok := store.(interface{ Verify(context.Context) error }); ok {
		return v.Verify(ctx)
	}
	_, err := store.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{Fresh: true})
	return err
}

func (s *Setup) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case setupMsg:
		if m.gen != s.gen {
			return nil
		}
		s.busy = false
		if m.err != nil {
			s.lastErr = m.err
			s.status, s.statusErr = errText(m.err), true
			return nil
		}
		s.d.Store = m.store
		s.d.Config.SetCredentials(strings.TrimSpace(s.id.Value), strings.TrimSpace(s.secret.Value))
		if len(s.d.Config.Areas) == 0 {
			s.d.Config.Areas = append([]config.Area(nil), config.DefaultAreas...)
		}
		if err := s.d.Config.Save(); err != nil {
			s.lastErr = err
			s.status, s.statusErr = "Could not save config: "+errText(err), true
			return nil
		}
		if s.nested {
			return ui.Pop{}
		}
		return ui.Replace{Screen: NewMainMenu(s.d)}
	case ui.ErrMsg:
		s.busy = false
		s.lastErr = m.Err
		s.status, s.statusErr = errText(m.Err), true
	case ui.RateLimited:
		s.status, s.statusErr = "Rate limited, retrying in "+itoa(int(m.Wait.Seconds()))+"s", false
	}
	return nil
}
