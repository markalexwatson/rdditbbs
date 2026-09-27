package screens

import (
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/ui"
)

func TestGoodbyeShowsStatsAndQuitsOnKey(t *testing.T) {
	d, _ := newDeps(t)
	d.Session.VisitArea("linux")
	d.Session.ThreadsOpened = 2
	d.Session.MessagesRead = 7
	app, sim := run(t, NewGoodbye(d))
	mustContain(t, sim, "Thanks for calling", "5m", "1", "2", "7")
	press(app, term.R(' '))
	if !app.Quitting() {
		t.Error("any key should quit")
	}
}

func TestGoodbyeQuitsOnTick(t *testing.T) {
	d, _ := newDeps(t)
	g := NewGoodbye(d)
	if act := g.Update(ui.Tick{ID: goodbyeTick}); act != (ui.Quit{}) {
		t.Errorf("tick action = %#v", act)
	}
	if act := g.Update(ui.Tick{ID: 99}); act != nil {
		t.Errorf("foreign tick should be ignored, got %#v", act)
	}
}
