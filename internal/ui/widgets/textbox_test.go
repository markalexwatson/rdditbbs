package widgets

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
)

func TestTextBoxDrawAndScroll(t *testing.T) {
	doc := textfmt.Render("one\n\ntwo\n\n> three\n\nfour", 20)
	box := &TextBox{Lines: doc.Lines}
	sim := term.NewSim(20, 3)
	box.Draw(sim, 0, 0, 20, 3)
	if sim.Row(0) != "one" || sim.Row(2) != "two" {
		t.Errorf("rows = %q", sim.String())
	}
	box.Scroll(2, 3)
	sim.Clear()
	box.Draw(sim, 0, 0, 20, 3)
	if sim.Row(0) != "two" || sim.Row(2) != "> three" {
		t.Errorf("after scroll = %q", sim.String())
	}
	if _, st := sim.CellAt(0, 2); st != theme.Style(theme.Quote) {
		t.Errorf("quote style = %+v", st)
	}
	box.Scroll(100, 3)
	if box.Top != len(doc.Lines)-3 || !box.AtEnd(3) {
		t.Errorf("top = %d", box.Top)
	}
	box.Scroll(-100, 3)
	if box.Top != 0 {
		t.Errorf("top = %d", box.Top)
	}
}

func TestDrawLineStyledOverridesKinds(t *testing.T) {
	sim := term.NewSim(20, 1)
	l := textfmt.Line{{Text: "a ", Kind: textfmt.Text}, {Text: "b", Kind: textfmt.Bold}}
	DrawLineStyled(sim, 0, 0, 20, l, theme.Style(theme.Cursor))
	if _, st := sim.CellAt(2, 0); st != theme.Style(theme.Cursor) {
		t.Errorf("style = %+v", st)
	}
}
