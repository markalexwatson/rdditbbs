package widgets

import (
	"testing"

	"github.com/markalexwatson/redditbbs/internal/term"
)

func TestNumInput(t *testing.T) {
	var n NumInput
	if _, _, handled := n.HandleKey(term.K(term.KeyEnter)); handled {
		t.Error("Enter with no digits must not be handled")
	}
	n.HandleKey(term.R('1'))
	n.HandleKey(term.R('7'))
	n.HandleKey(term.K(term.KeyBackspace))
	n.HandleKey(term.R('2'))
	if n.Digits != "12" {
		t.Errorf("digits = %q", n.Digits)
	}
	v, submitted, handled := n.HandleKey(term.K(term.KeyEnter))
	if v != 12 || !submitted || !handled || n.Digits != "" {
		t.Errorf("submit = %d %v %v %q", v, submitted, handled, n.Digits)
	}
	n.HandleKey(term.R('3'))
	if _, _, handled := n.HandleKey(term.K(term.KeyEscape)); !handled || n.Digits != "" {
		t.Error("Escape should clear digits")
	}
	if _, _, handled := n.HandleKey(term.K(term.KeyEscape)); handled {
		t.Error("Escape with no digits must pass through")
	}
}

func TestTextInputEditing(t *testing.T) {
	in := &TextInput{}
	for _, r := range "linux" {
		in.HandleKey(term.R(r))
	}
	in.HandleKey(term.K(term.KeyLeft))
	in.HandleKey(term.K(term.KeyBackspace))
	in.HandleKey(term.R('U'))
	if in.Value != "linUx" || in.Cursor != 4 {
		t.Errorf("value=%q cursor=%d", in.Value, in.Cursor)
	}
	in.HandleKey(term.K(term.KeyHome))
	in.HandleKey(term.K(term.KeyDelete))
	if in.Value != "inUx" {
		t.Errorf("after delete = %q", in.Value)
	}
	if in.HandleKey(term.K(term.KeyEnter)) != InputSubmit || in.HandleKey(term.K(term.KeyEscape)) != InputCancel {
		t.Error("submit/cancel results")
	}
	in.HandleKey(term.Key{Code: term.KeyRune, Rune: '?', Paste: true})
	if in.Value != "?inUx" {
		t.Errorf("pasted rune not inserted: %q", in.Value)
	}
	if in.HandleKey(term.Key{Code: term.KeyEnter, Paste: true}) != InputNone {
		t.Error("a pasted Enter must not submit")
	}
}

func TestTextInputMaskAndDraw(t *testing.T) {
	in := &TextInput{Mask: true}
	for _, r := range "abc" {
		in.HandleKey(term.R(r))
	}
	if in.Display() != "***" {
		t.Errorf("display = %q", in.Display())
	}
	sim := term.NewSim(10, 1)
	in.Draw(sim, 2, 0, 6, term.Style{}, true)
	if sim.Row(0) != "  ***" {
		t.Errorf("row = %q", sim.Row(0))
	}
	if x, _, on := sim.Cursor(); !on || x != 5 {
		t.Errorf("cursor x = %d on=%v", x, on)
	}
}
