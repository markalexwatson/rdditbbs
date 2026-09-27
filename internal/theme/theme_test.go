package theme

import (
	"strings"
	"testing"

	"github.com/markalexwatson/redditbbs/internal/term"
)

func TestSchemeA(t *testing.T) {
	cases := map[Role]term.Style{
		Frame:   {FG: term.Cyan},
		Heading: {FG: term.BrightYellow},
		Subject: {FG: term.BrightWhite},
		Author:  {FG: term.BrightGreen},
		Meta:    {FG: term.BrightBlack},
		Cursor:  {FG: term.Black, BG: term.White},
		Error:   {FG: term.BrightRed},
	}
	for r, want := range cases {
		if got := Style(r); got != want {
			t.Errorf("Style(%d) = %+v, want %+v", r, got, want)
		}
	}
}

func TestEveryRoleHasStyle(t *testing.T) {
	classic, _ := Get("classic")
	for r := Frame; r <= Bar; r++ {
		if _, ok := classic.Styles[r]; !ok {
			t.Errorf("role %s has no style", RoleName(r))
		}
		if RoleName(r) == "" {
			t.Errorf("role %d has no name", r)
		}
		if back, ok := RoleByName(RoleName(r)); !ok || back != r {
			t.Errorf("RoleByName(%q) = %v %v", RoleName(r), back, ok)
		}
	}
}

func TestBuiltInsCoverEveryRole(t *testing.T) {
	names := Names()
	if len(names) < 4 || names[0] != "classic" {
		t.Fatalf("Names() = %v", names)
	}
	for _, n := range names {
		th, ok := Get(n)
		if !ok {
			t.Fatalf("Get(%q) missing", n)
		}
		for r := Frame; r <= Bar; r++ {
			if _, ok := th.Styles[r]; !ok {
				t.Errorf("theme %q has no style for role %s", n, RoleName(r))
			}
		}
	}
}

func TestSetSwitchesActiveStyles(t *testing.T) {
	t.Cleanup(func() { _ = Set("classic") })
	if err := Set("amber"); err != nil {
		t.Fatal(err)
	}
	amber, _ := Get("amber")
	if Style(Heading) != amber.Styles[Heading] || Current() != "amber" {
		t.Errorf("active theme did not switch: %+v", Style(Heading))
	}
	if err := Set("nope"); err == nil {
		t.Error("unknown theme should be an error")
	}
	if Current() != "amber" {
		t.Error("a failed Set must not change the active theme")
	}
}

func TestNextCycles(t *testing.T) {
	t.Cleanup(func() { _ = Set("classic") })
	_ = Set("classic")
	seen := map[string]bool{}
	for range Names() {
		seen[Next()] = true
	}
	if len(seen) != len(Names()) || Current() != "classic" {
		t.Errorf("Next did not cycle through every theme and back: %v, now %q", seen, Current())
	}
}

func TestParseStyle(t *testing.T) {
	cases := map[string]term.Style{
		"bright cyan":           {FG: term.BrightCyan},
		"black on white":        {FG: term.Black, BG: term.White},
		"green bold":            {FG: term.Green, Bold: true},
		"Bright Yellow on Blue": {FG: term.BrightYellow, BG: term.Blue},
		"white reverse":         {FG: term.White, Reverse: true},
		"default":               {},
		"bold":                  {Bold: true},
		"grey":                  {FG: term.BrightBlack},
		"gray":                  {FG: term.BrightBlack},
	}
	for in, want := range cases {
		got, err := ParseStyle(in)
		if err != nil || got != want {
			t.Errorf("ParseStyle(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "purple", "red on", "on white", "red on mauve", "red bolder"} {
		if _, err := ParseStyle(bad); err == nil {
			t.Errorf("ParseStyle(%q) should fail", bad)
		}
	}
}

func TestCustomOverridesBase(t *testing.T) {
	th, err := Custom("classic", map[string]string{"heading": "bright red", "cursor": "black on cyan"})
	if err != nil {
		t.Fatal(err)
	}
	classic, _ := Get("classic")
	if th.Name != "custom" || th.Styles[Heading] != (term.Style{FG: term.BrightRed}) || th.Styles[Cursor] != (term.Style{FG: term.Black, BG: term.Cyan}) {
		t.Errorf("overrides not applied: %+v", th.Styles[Heading])
	}
	if th.Styles[Author] != classic.Styles[Author] {
		t.Error("untouched roles should come from the base")
	}
	if _, err := Custom("classic", map[string]string{"headline": "red"}); err == nil || !strings.Contains(err.Error(), "headline") {
		t.Errorf("unknown role should name the key, got %v", err)
	}
	if _, err := Custom("classic", map[string]string{"heading": "mauve"}); err == nil || !strings.Contains(err.Error(), "heading") {
		t.Errorf("bad colour should name the key, got %v", err)
	}
	if _, err := Custom("nope", nil); err == nil {
		t.Error("unknown base should fail")
	}
}

func TestOnBarUsesEffectiveBackground(t *testing.T) {
	t.Cleanup(func() { Unregister("custom"); _ = Set("classic") })
	th, err := Custom("classic", map[string]string{"bar": "white on blue reverse", "subject": "bright yellow"})
	if err != nil {
		t.Fatal(err)
	}
	Register(th)
	_ = Set("custom")
	got := OnBar(Subject)
	if got.BG != term.White || got.Reverse {
		t.Errorf("text on a reversed bar should take the bar's effective (white) background without reverse, got %+v", got)
	}
	th2, _ := Custom("classic", map[string]string{"bar": "white on blue", "cursor": "black on cyan"})
	Register(th2)
	if got := OnBar(Cursor); got.BG != term.Cyan {
		t.Errorf("a role with its own background keeps it, got %+v", got)
	}
}

func TestRegisterCustomAppearsInNames(t *testing.T) {
	t.Cleanup(func() { Unregister("custom"); _ = Set("classic") })
	th, _ := Custom("green", map[string]string{"heading": "white"})
	Register(th)
	names := Names()
	if names[len(names)-1] != "custom" {
		t.Errorf("custom should be last in %v", names)
	}
	if err := Set("custom"); err != nil || Style(Heading) != (term.Style{FG: term.White}) {
		t.Errorf("Set(custom) = %v, heading %+v", err, Style(Heading))
	}
}
