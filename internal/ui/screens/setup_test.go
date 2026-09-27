package screens

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/config"
	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/ui"
)

func setupDeps(t *testing.T) (*Deps, *redditest.FakeStore) {
	d, _ := newDeps(t)
	d.Config.SetCredentials("", "")
	d.Config.Areas = nil
	d.Store = nil
	made := redditest.NewFakeStore()
	made.Listings["linux/hot/"] = redditest.SampleListing(2, "")
	d.MakeStore = func(id, secret string) reddit.Store {
		if id != "myid" || secret != "mysecret" {
			made.Err = &reddit.APIError{Status: 401}
		}
		return made
	}
	return d, made
}

func TestSetupHappyPath(t *testing.T) {
	d, made := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	mustContain(t, sim, "prefs/apps", "Client ID", "Client secret", "New User Setup")
	typeString(app, "myid")
	press(app, term.K(term.KeyTab))
	typeString(app, "mysecret")
	mustContain(t, sim, "********")
	mustNotContain(t, sim, "mysecret")
	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Checking credentials")
	pump(t, app)
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatalf("expected MainMenu, got %T", app.Top())
	}
	if d.Store != made || !d.Config.HasCredentials() || len(d.Config.Areas) != 4 {
		t.Errorf("store=%v creds=%v areas=%d", d.Store == made, d.Config.HasCredentials(), len(d.Config.Areas))
	}
	saved, err := loadConfig(t, d.Config.Path)
	if err != nil || saved.ClientID() != "myid" || len(saved.Areas) != 4 {
		t.Errorf("saved = %+v err=%v", saved, err)
	}
}

func TestSetupRejectedCredentialsStay(t *testing.T) {
	d, _ := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	typeString(app, "wrong")
	press(app, term.K(term.KeyEnter)) // Enter on the ID field moves to the secret field
	typeString(app, "x")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	if _, ok := app.Top().(*Setup); !ok {
		t.Fatal("rejected credentials must stay on setup")
	}
	mustContain(t, sim, "Credentials rejected")
	if d.Config.HasCredentials() {
		t.Error("rejected credentials must not be stored")
	}
}

func TestSetupPreservesExistingAreas(t *testing.T) {
	d, _ := setupDeps(t)
	d.Config.Areas = []config.Area{{Name: "Rust", Subreddit: "rust"}}
	app, _ := run(t, NewSetup(d))
	typeString(app, "myid")
	press(app, term.K(term.KeyTab))
	typeString(app, "mysecret")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	if len(d.Config.Areas) != 1 || d.Config.Areas[0].Subreddit != "rust" {
		t.Errorf("areas = %+v", d.Config.Areas)
	}
}

func TestSetupEmptyFieldsAndEscape(t *testing.T) {
	d, _ := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	press(app, term.K(term.KeyEnter), term.K(term.KeyEnter))
	mustContain(t, sim, "Both fields are required")
	press(app, term.K(term.KeyEscape))
	if !app.Quitting() {
		t.Error("Escape on setup should quit")
	}
}

func TestSetupNestedPopsOnSuccessAndEscape(t *testing.T) {
	d, _ := setupDeps(t)
	s := NewNestedSetup(d)
	if act := s.HandleKey(term.K(term.KeyEscape)); act != (ui.Pop{}) {
		t.Errorf("nested Escape should Pop, got %#v", act)
	}
	s.id.Value, s.secret.Value = "myid", "mysecret"
	msg := s.submit().(ui.Run).Fn(context.Background())
	if act := s.Update(msg); act != (ui.Pop{}) {
		t.Errorf("nested success should Pop, got %#v", act)
	}
}

func TestSetupPastedEnterDoesNotSubmit(t *testing.T) {
	d, _ := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	for _, r := range "myid" {
		app.Handle(term.Key{Code: term.KeyRune, Rune: r, Paste: true})
	}
	app.Handle(term.Key{Code: term.KeyEnter, Paste: true})
	app.Draw()
	mustNotContain(t, sim, "Checking credentials", "Both fields are required")
	if s := app.Top().(*Setup); s.focus != 0 || s.id.Value != "myid" {
		t.Errorf("pasted Enter changed state: focus=%d id=%q", s.focus, s.id.Value)
	}
}

func TestSetupWorkerPanicClearsBusy(t *testing.T) {
	d, _ := setupDeps(t)
	s := NewSetup(d)
	s.id.Value, s.secret.Value = "myid", "mysecret"
	s.submit()
	s.Update(ui.ErrMsg{Err: errors.New("internal error: boom")})
	if s.busy || !strings.Contains(s.status, "boom") {
		t.Errorf("busy=%v status=%q", s.busy, s.status)
	}
}

func TestSetupWithoutMakeStore(t *testing.T) {
	d, _ := setupDeps(t)
	d.MakeStore = nil
	s := NewSetup(d)
	s.id.Value, s.secret.Value = "a", "b"
	if act := s.submit(); act != nil {
		t.Errorf("submit without MakeStore should not start a run, got %#v", act)
	}
	if !errors.Is(s.lastErr, errNoStoreFactory) {
		t.Errorf("lastErr = %v", s.lastErr)
	}
}
