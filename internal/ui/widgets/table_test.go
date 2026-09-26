package widgets

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestTableMovementAndScrolling(t *testing.T) {
	var tb Table
	tb.SetHeight(5)
	tb.SetCount(12)
	tb.Move(-1)
	if tb.Cursor != 0 {
		t.Error("cursor below zero")
	}
	tb.Move(6)
	if tb.Cursor != 6 || tb.Top != 2 {
		t.Errorf("after down 6: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if s, e := tb.Visible(); s != 2 || e != 7 {
		t.Errorf("visible = %d..%d", s, e)
	}
	tb.HandleKey(term.K(term.KeyEnd))
	if tb.Cursor != 11 || tb.Top != 7 {
		t.Errorf("end: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	tb.HandleKey(term.K(term.KeyPgUp))
	if tb.Cursor != 6 {
		t.Errorf("pgup cursor=%d", tb.Cursor)
	}
	tb.HandleKey(term.K(term.KeyHome))
	if tb.Cursor != 0 || tb.Top != 0 {
		t.Errorf("home: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if tb.HandleKey(term.R('x')) {
		t.Error("rune should not be handled")
	}
}

func TestTableClampsWhenCountShrinks(t *testing.T) {
	var tb Table
	tb.SetHeight(3)
	tb.SetCount(10)
	tb.Select(9)
	tb.SetCount(4)
	if tb.Cursor != 3 || tb.Top != 1 {
		t.Errorf("cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	tb.SetCount(0)
	if tb.Cursor != 0 || tb.Top != 0 {
		t.Errorf("empty: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if s, e := tb.Visible(); s != 0 || e != 0 {
		t.Errorf("empty visible = %d..%d", s, e)
	}
}

func TestTableHeightShrinkKeepsCursorVisible(t *testing.T) {
	var tb Table
	tb.SetHeight(10)
	tb.SetCount(20)
	tb.Select(9)
	tb.SetHeight(4)
	if s, e := tb.Visible(); tb.Cursor < s || tb.Cursor >= e {
		t.Errorf("cursor %d not within %d..%d", tb.Cursor, s, e)
	}
}
