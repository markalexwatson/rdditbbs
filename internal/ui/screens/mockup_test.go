package screens

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markalexwatson/redditbbs/internal/config"
	"github.com/markalexwatson/redditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/textfmt"
	"github.com/markalexwatson/redditbbs/internal/theme"
	"github.com/markalexwatson/redditbbs/internal/ui"
)

// TestRenderMockups writes every screen as an HTML fragment when
// REDDITBBS_MOCKUP_DIR is set, for screenshots in the README or a share page.
// It is skipped otherwise.
func TestRenderMockups(t *testing.T) {
	dir := os.Getenv("REDDITBBS_MOCKUP_DIR")
	if dir == "" {
		t.Skip("set REDDITBBS_MOCKUP_DIR to render screen mockups")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	d, fs := newDeps(t)
	d.Config.Areas = []config.Area{
		{Name: "Linux", Subreddit: "linux"},
		{Name: "Programming", Subreddit: "programming"},
		{Name: "Retro Battlestations", Subreddit: "retrobattlestations"},
		{Name: "Command Line", Subreddit: "commandline"},
		{Name: "Vintage Computing", Subreddit: "vintagecomputing"},
	}
	d.Session.VisitArea("linux")
	d.Session.VisitArea("commandline")
	d.Session.ThreadsOpened = 4
	d.Session.MessagesRead = 23
	d.Session.LinksOpened = 2
	d.Version = "0.1.0"
	fs.Listings["linux/hot/"] = redditest.DemoListing(testNow)
	fs.Threads["k72"] = redditest.DemoThread(testNow)

	sim := term.NewSim(100, 30)
	save := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(simHTML(sim)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app := ui.New(sim, NewSplash(d))
	app.Draw()
	save("01-splash")

	press(app, term.R(' '))
	save("02-mainmenu")

	press(app, term.R('m'))
	save("03-arealist")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	press(app, term.K(term.KeyDown)) // the kernel post
	save("04-postlist")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	save("05-threadindex")

	press(app, term.R('?'))
	save("06-help")
	press(app, term.R(' '))

	// The thread index under every other built-in theme.
	for _, name := range theme.Names() {
		if name == "classic" {
			continue
		}
		if err := theme.Set(name); err != nil {
			t.Fatal(err)
		}
		app.Draw()
		save("theme-" + name)
	}
	if err := theme.Set("classic"); err != nil {
		t.Fatal(err)
	}
	app.Draw()

	press(app, term.K(term.KeyEnter))
	save("07-reader")

	press(app, term.R('q'), term.R('q'), term.R('q'), term.R('q'), term.R('g'), term.R('y'))
	save("08-goodbye")
}

var ansiHex = map[term.Color]string{
	term.Default: "", term.Black: "#000000", term.Red: "#aa0000", term.Green: "#00aa00", term.Yellow: "#aa5500",
	term.Blue: "#0000aa", term.Magenta: "#aa00aa", term.Cyan: "#00aaaa", term.White: "#aaaaaa",
	term.BrightBlack: "#555555", term.BrightRed: "#ff5555", term.BrightGreen: "#55ff55", term.BrightYellow: "#ffff55",
	term.BrightBlue: "#5555ff", term.BrightMagenta: "#ff55ff", term.BrightCyan: "#55ffff", term.BrightWhite: "#ffffff",
}

func styleCSS(st term.Style) string {
	fg, bg := ansiHex[st.FG], ansiHex[st.BG]
	if fg == "" {
		fg = "#aaaaaa"
	}
	if st.Reverse {
		if bg == "" {
			bg = "#000000"
		}
		fg, bg = bg, fg
	}
	var b strings.Builder
	fmt.Fprintf(&b, "color:%s", fg)
	if bg != "" {
		fmt.Fprintf(&b, ";background:%s", bg)
	}
	if st.Bold {
		b.WriteString(";font-weight:bold")
	}
	return b.String()
}

// simHTML renders the simulated screen as a <pre> with one span per style run.
func simHTML(sim *term.Sim) string {
	w, h := sim.Size()
	var b strings.Builder
	b.WriteString(`<pre class="term">`)
	for y := 0; y < h; y++ {
		var run strings.Builder
		var cur string
		flush := func() {
			if run.Len() == 0 {
				return
			}
			fmt.Fprintf(&b, `<span style="%s">%s</span>`, cur, html.EscapeString(run.String()))
			run.Reset()
		}
		for x := 0; x < w; x++ {
			s, st := sim.CellAt(x, y)
			if s == "" {
				// blank cell or the second half of a wide cluster
				if prev, _ := sim.CellAt(x-1, y); x > 0 && textfmt.Width(prev) == 2 {
					continue
				}
				s = " "
			}
			css := styleCSS(st)
			if css != cur {
				flush()
				cur = css
			}
			run.WriteString(s)
		}
		flush()
		b.WriteString("\n")
	}
	b.WriteString("</pre>")
	return b.String()
}
