package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
)

func TestAreaListRowsAndCursor(t *testing.T) {
	d, _ := newDeps(t)
	_, sim := run(t, NewAreaList(d))
	mustContain(t, sim, "Linux", "r/linux", "Rust", "r/rust", "2 areas")
	if _, st := sim.CellAt(2, 5); st != theme.Style(theme.Cursor) {
		t.Errorf("first row should be the cursor row, style %+v", st)
	}
}

func TestAreaListDeleteWithConfirmation(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewAreaList(d))
	press(app, term.K(term.KeyDown), term.R('d'))
	mustContain(t, sim, "Delete Rust? (y/N)")
	press(app, term.R('y'))
	mustNotContain(t, sim, "r/rust")
	if len(d.Config.Areas) != 1 {
		t.Errorf("areas = %+v", d.Config.Areas)
	}
	again, _ := loadConfig(t, d.Config.Path)
	if len(again.Areas) != 1 {
		t.Error("deletion not saved")
	}
}

func TestAreaListOpenByNumber(t *testing.T) {
	d, _ := newDeps(t)
	app, _ := run(t, NewAreaList(d))
	press(app, term.R('2'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*AreaList); ok {
		t.Error("number selection should open the area")
	}
}

func TestAreaListEmpty(t *testing.T) {
	d, _ := newDeps(t)
	d.Config.Areas = nil
	app, sim := run(t, NewAreaList(d))
	mustContain(t, sim, "No areas configured")
	press(app, term.K(term.KeyEnter), term.R('d'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Error("Enter and D on an empty list must do nothing")
	}
}
