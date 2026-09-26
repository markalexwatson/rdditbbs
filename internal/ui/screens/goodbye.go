package screens

import (
	"context"
	"fmt"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

const goodbyeTick = 1

// Goodbye is the sign-off screen.
type Goodbye struct{ d *Deps }

// NewGoodbye creates the goodbye screen.
func NewGoodbye(d *Deps) *Goodbye { return &Goodbye{d: d} }

func (g *Goodbye) Init() ui.Action {
	return ui.Sleep(context.Background(), 2*time.Second, goodbyeTick)
}

func (g *Goodbye) Update(m ui.Msg) ui.Action {
	if t, ok := m.(ui.Tick); ok && t.ID == goodbyeTick {
		return ui.Quit{}
	}
	return nil
}

func (g *Goodbye) HandleKey(term.Key) ui.Action { return ui.Quit{} }
func (g *Goodbye) Title() string                { return "Goodbye" }
func (g *Goodbye) Keys() []ui.KeyHelp           { return nil }
func (g *Goodbye) Fullscreen() bool             { return true }

func (g *Goodbye) Draw(c term.Canvas) {
	_, h := c.Size()
	s := g.d.Session
	online := s.Online(g.d.now()).Round(time.Minute)
	y := h/2 - 5
	if y < 0 {
		y = 0
	}
	widgets.Centre(c, y, "╒═══════════════════════════════════════╕", theme.Style(theme.Frame))
	widgets.Centre(c, y+1, "Thanks for calling RedditBBS", theme.Style(theme.Logo))
	widgets.Centre(c, y+2, "╘═══════════════════════════════════════╛", theme.Style(theme.Frame))
	stats := []string{
		fmt.Sprintf("Time online ....... %s", fmtDuration(online)),
		fmt.Sprintf("Areas visited ..... %d", s.AreasVisited()),
		fmt.Sprintf("Threads opened .... %d", s.ThreadsOpened),
		fmt.Sprintf("Messages read ..... %d", s.MessagesRead),
		fmt.Sprintf("Links opened ...... %d", s.LinksOpened),
	}
	for i, l := range stats {
		widgets.Centre(c, y+4+i, l, theme.Style(theme.Body))
	}
	widgets.Centre(c, y+10, "NO CARRIER", theme.Style(theme.Meta))
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return "under 1m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
