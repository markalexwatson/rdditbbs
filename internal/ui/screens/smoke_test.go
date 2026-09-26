package screens

import (
	"strings"
	"testing"
	"time"

	"github.com/markalexwatson/redditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/ui"
)

// TestSmokeWalkthrough drives every screen in order through the App loop.
func TestSmokeWalkthrough(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(3, "")
	th := redditest.SampleThread()
	fs.Threads["p1"] = th

	app, sim := run(t, NewSplash(d))
	mustContain(t, sim, "Press any key to log on")

	press(app, term.R(' '))
	mustContain(t, sim, "[M] Message areas")

	press(app, term.R('m'))
	mustContain(t, sim, "r/linux", "r/rust")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "Post 1", "Post 3", "r/linux · HOT")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "sched_nerd", "[load 3 more replies]", "Thread Index")

	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Read Message", "The EEVDF changes are the headline.")

	press(app, term.R('n'))
	mustContain(t, sim, "Agreed.", "Msg 2 of 7")

	press(app, term.R('t'))
	ti, ok := app.Top().(*ThreadIndex)
	if !ok {
		t.Fatalf("expected ThreadIndex after T, got %T", app.Top())
	}
	if row := ti.rows[ti.table.Cursor]; row.Comment == nil || row.Comment.ID != "c2" {
		t.Errorf("thread index should reselect c2, got %+v", row)
	}

	press(app, term.R('?'))
	mustContain(t, sim, "Keys for Thread Index", "Ctrl-C")
	press(app, term.R(' '))

	press(app, term.R('q'))
	if _, ok := app.Top().(*PostList); !ok {
		t.Fatalf("expected PostList, got %T", app.Top())
	}
	press(app, term.R('q'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Fatalf("expected AreaList, got %T", app.Top())
	}
	press(app, term.R('q'))
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatalf("expected MainMenu, got %T", app.Top())
	}
	press(app, term.R('g'), term.R('y'))
	mustContain(t, sim, "Thanks for calling", "Messages read ..... 2", "Threads opened .... 1", "Areas visited ..... 1")

	press(app, term.R(' '))
	if !app.Quitting() {
		t.Error("goodbye should quit on a key")
	}
}

// TestSmokeRunLoop drives the real App.Run loop with injected events.
func TestSmokeRunLoop(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(1, "")
	sim := term.NewSim(80, 24)
	app := ui.New(sim, NewSplash(d))
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	for _, k := range []term.Key{term.R(' '), term.R('m'), term.K(term.KeyEnter)} {
		sim.Inject(k)
	}
	deadline := time.After(3 * time.Second)
	for !strings.Contains(sim.String(), "Post 1") {
		select {
		case <-deadline:
			t.Fatalf("post list never rendered:\n%s", sim.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	sim.Inject(term.K(term.KeyCtrlC))
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit on Ctrl-C")
	}
}
