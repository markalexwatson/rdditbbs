package theme

import (
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
	for r := Frame; r <= NSFW; r++ {
		if _, ok := styles[r]; !ok {
			t.Errorf("role %d has no style", r)
		}
	}
}
