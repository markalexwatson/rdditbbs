package screens

import (
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/term"
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

func TestSplashGoesToSetupWithoutCredentials(t *testing.T) {
	d, _ := newDeps(t)
	d.Config.SetCredentials("", "")
	app, _ := run(t, NewSplash(d))
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*Setup); !ok {
		t.Errorf("expected Setup, got %T", app.Top())
	}
}

func TestSplashDemoModeSkipsSetup(t *testing.T) {
	d, _ := newDeps(t)
	d.Config.SetCredentials("", "")
	d.Demo = true
	app, sim := run(t, NewSplash(d))
	mustContain(t, sim, "DEMO MODE")
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Errorf("demo mode should go straight to the menu, got %T", app.Top())
	}
}
