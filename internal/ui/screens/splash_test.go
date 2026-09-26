package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestSplashShowsLogoAndGoesToMenu(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewSplash(d))
	mustContain(t, sim, "Node 1", "26/09/2026", "Press any key to log on", "vtest")
	press(app, term.R('x'))
	if _, ok := app.Top().(*MainMenu); !ok || app.Depth() != 1 {
		t.Errorf("expected MainMenu on top, got %T depth %d", app.Top(), app.Depth())
	}
}
