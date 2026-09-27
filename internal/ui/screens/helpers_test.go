package screens

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/config"
	"github.com/markalexwatson/rdditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/rdditbbs/internal/session"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/ui"
)

var testNow = time.Date(2026, 9, 26, 20, 30, 0, 0, time.UTC)

func newDeps(t *testing.T) (*Deps, *redditest.FakeStore) {
	t.Helper()
	fs := redditest.NewFakeStore()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetCredentials("id", "secret")
	cfg.AddArea(config.Area{Name: "Linux", Subreddit: "linux"})
	cfg.AddArea(config.Area{Name: "Rust", Subreddit: "rust"})
	d := &Deps{
		Store:   fs,
		Config:  cfg,
		Session: session.New(testNow.Add(-5 * time.Minute)),
		Open:    func(string, func(error)) error { return nil },
		Now:     func() time.Time { return testNow },
		Version: "test",
	}
	return d, fs
}

// run mounts a screen in an App on an 80x24 sim and draws it.
func run(t *testing.T, s ui.Screen) (*ui.App, *term.Sim) {
	t.Helper()
	sim := term.NewSim(80, 24)
	app := ui.New(sim, s)
	app.Draw()
	return app, sim
}

// pump waits for one async result and redraws.
func pump(t *testing.T, app *ui.App) {
	t.Helper()
	if !app.Pump(2 * time.Second) {
		t.Fatal("no async result arrived")
	}
	app.Draw()
}

func press(app *ui.App, keys ...term.Key) {
	for _, k := range keys {
		app.Handle(k)
	}
	app.Draw()
}

func typeString(app *ui.App, s string) {
	for _, r := range s {
		app.Handle(term.R(r))
	}
	app.Draw()
}

func mustContain(t *testing.T, sim *term.Sim, wants ...string) {
	t.Helper()
	out := sim.String()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("screen missing %q:\n%s", w, out)
		}
	}
}

func mustNotContain(t *testing.T, sim *term.Sim, wants ...string) {
	t.Helper()
	out := sim.String()
	for _, w := range wants {
		if strings.Contains(out, w) {
			t.Errorf("screen should not contain %q:\n%s", w, out)
		}
	}
}

func loadConfig(t *testing.T, path string) (*config.Config, error) {
	t.Helper()
	return config.Load(path, func(string) string { return "" })
}
