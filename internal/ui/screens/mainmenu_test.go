package screens

import (
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/term"
)

func TestMainMenuLists(t *testing.T) {
	d, _ := newDeps(t)
	_, sim := run(t, NewMainMenu(d))
	mustContain(t, sim, "[M] Message areas", "[J] Join area", "[G] Goodbye", "Main Menu")
}

func TestMainMenuOpensAreaList(t *testing.T) {
	d, _ := newDeps(t)
	app, _ := run(t, NewMainMenu(d))
	press(app, term.R('m'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Errorf("got %T", app.Top())
	}
}

func TestMainMenuLogoffConfirmation(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('q'))
	mustContain(t, sim, "Log off? (y/N)")
	press(app, term.R('n'))
	mustNotContain(t, sim, "Log off?")
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatal("n should cancel")
	}
	press(app, term.K(term.KeyEscape), term.R('y'))
	if _, ok := app.Top().(*Goodbye); !ok {
		t.Errorf("expected Goodbye, got %T", app.Top())
	}
}

func TestMainMenuJoinValidatesName(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('j'))
	mustContain(t, sim, "Join area:")
	typeString(app, "bad name!")
	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Invalid area name")
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatal("invalid name must stay on menu")
	}
	press(app, term.R('j'))
	typeString(app, "r/linux")
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*MainMenu); ok {
		t.Error("valid name should open the post list")
	}
}

func TestMainMenuQuestionMarkInsideJoinIsLiteral(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('j'), term.R('?'))
	if app.Depth() != 1 {
		t.Error("? during text entry must not open help")
	}
	mustContain(t, sim, "Join area: ?")
}
