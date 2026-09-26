package screens

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

var logoLines = []string{
	"██████╗ ███████╗██████╗ ██████╗ ██╗████████╗    ██████╗ ██████╗ ███████╗",
	"██╔══██╗██╔════╝██╔══██╗██╔══██╗██║╚══██╔══╝    ██╔══██╗██╔══██╗██╔════╝",
	"██████╔╝█████╗  ██║  ██║██║  ██║██║   ██║       ██████╔╝██████╔╝███████╗",
	"██╔══██╗██╔══╝  ██║  ██║██║  ██║██║   ██║       ██╔══██╗██╔══██╗╚════██║",
	"██║  ██║███████╗██████╔╝██████╔╝██║   ██║       ██████╔╝██████╔╝███████║",
	"╚═╝  ╚═╝╚══════╝╚═════╝ ╚═════╝ ╚═╝   ╚═╝       ╚═════╝ ╚═════╝ ╚══════╝",
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
}

func (s *Splash) HandleKey(term.Key) ui.Action {
	if s.d.Config.HasCredentials() {
		return ui.Replace{Screen: NewMainMenu(s.d)}
	}
	return ui.Replace{Screen: NewSetup(s.d)}
}
