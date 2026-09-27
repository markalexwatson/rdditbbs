package screens

import (
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

var logoLines = []string{
	"██████╗ ██████╗ ██████╗ ██╗████████╗    ██████╗ ██████╗ ███████╗",
	"██╔══██╗██╔══██╗██╔══██╗██║╚══██╔══╝    ██╔══██╗██╔══██╗██╔════╝",
	"██████╔╝██║  ██║██║  ██║██║   ██║       ██████╔╝██████╔╝███████╗",
	"██╔══██╗██║  ██║██║  ██║██║   ██║       ██╔══██╗██╔══██╗╚════██║",
	"██║  ██║██████╔╝██████╔╝██║   ██║       ██████╔╝██████╔╝███████║",
	"╚═╝  ╚═╝╚═════╝ ╚═════╝ ╚═╝   ╚═╝       ╚═════╝ ╚═════╝ ╚══════╝",
}

// Splash is the logon screen.
type Splash struct{ d *Deps }

// NewSplash creates the splash screen.
func NewSplash(d *Deps) *Splash { return &Splash{d: d} }

func (s *Splash) Init() ui.Action         { return nil }
func (s *Splash) Update(ui.Msg) ui.Action { return nil }
func (s *Splash) Title() string           { return "Logon" }
func (s *Splash) Keys() []ui.KeyHelp      { return nil }
func (s *Splash) Fullscreen() bool        { return true }

func (s *Splash) Draw(c term.Canvas) {
	_, h := c.Size()
	y := h/2 - 7
	if y < 0 {
		y = 0
	}
	for i, l := range logoLines {
		st := theme.Style(theme.Logo)
		if i >= 3 {
			st = theme.Style(theme.Frame)
		}
		widgets.Centre(c, y+i, l, st)
	}
	y += len(logoLines) + 1
	widgets.Centre(c, y, "A bulletin board window onto Reddit", theme.Style(theme.Subject))
	widgets.Centre(c, y+2, "Node 1  ·  "+s.d.now().Format("02/01/2006 15:04")+"  ·  v"+s.d.Version, theme.Style(theme.Meta))
	widgets.Centre(c, y+4, "Press any key to log on", theme.Style(theme.Hotkey))
	switch {
	case s.d.Demo:
		widgets.Centre(c, y+6, "DEMO MODE: sample data, nothing is fetched from Reddit", theme.Style(theme.Error))
	case s.d.Source == "rss":
		widgets.Centre(c, y+6, "RSS MODE: public feeds, no scores or threading, about one fetch a minute", theme.Style(theme.Error))
	}
}

func (s *Splash) HandleKey(term.Key) ui.Action {
	if s.d.Demo || s.d.Source == "rss" || s.d.Config.HasCredentials() {
		return ui.Replace{Screen: NewMainMenu(s.d)}
	}
	return ui.Replace{Screen: NewSetup(s.d)}
}
