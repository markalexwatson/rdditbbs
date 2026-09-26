package widgets

import (
	"strconv"
	"strings"

	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/textfmt"
)

// NumInput collects typed digits for selecting a row by number.
type NumInput struct{ Digits string }

// HandleKey consumes digits always, and Backspace, Enter and Escape while
// digits are present. It reports the submitted value on Enter.
func (n *NumInput) HandleKey(k term.Key) (value int, submitted, handled bool) {
	if k.Code == term.KeyRune && k.Rune >= '0' && k.Rune <= '9' {
		if len(n.Digits) < 6 {
			n.Digits += string(k.Rune)
		}
		return 0, false, true
	}
	if n.Digits == "" {
		return 0, false, false
	}
	switch k.Code {
	case term.KeyBackspace:
		n.Digits = n.Digits[:len(n.Digits)-1]
	case term.KeyEnter:
		v, _ := strconv.Atoi(n.Digits)
		n.Digits = ""
		return v, true, true
	case term.KeyEscape:
		n.Digits = ""
	default:
		return 0, false, false
	}
	return 0, false, true
}

// InputResult is what a TextInput key press produced.
type InputResult int

// Input results.
const (
	InputNone InputResult = iota
	InputSubmit
	InputCancel
)

// TextInput is a single-line editor. Cursor is a rune index.
type TextInput struct {
	Value  string
	Cursor int
	Mask   bool
}

// HandleKey edits the value; every key is taken literally except Enter and
// Escape. A pasted Enter or Escape is ignored so a multi-line paste cannot
// submit or cancel the field.
func (t *TextInput) HandleKey(k term.Key) InputResult {
	rs := []rune(t.Value)
	if t.Cursor > len(rs) {
		t.Cursor = len(rs)
	}
	if k.Paste && (k.Code == term.KeyEnter || k.Code == term.KeyEscape) {
		return InputNone
	}
	switch k.Code {
	case term.KeyRune:
		rs = append(rs[:t.Cursor], append([]rune{k.Rune}, rs[t.Cursor:]...)...)
		t.Cursor++
	case term.KeyBackspace:
		if t.Cursor > 0 {
			rs = append(rs[:t.Cursor-1], rs[t.Cursor:]...)
			t.Cursor--
		}
	case term.KeyDelete:
		if t.Cursor < len(rs) {
			rs = append(rs[:t.Cursor], rs[t.Cursor+1:]...)
		}
	case term.KeyLeft:
		if t.Cursor > 0 {
			t.Cursor--
		}
	case term.KeyRight:
		if t.Cursor < len(rs) {
			t.Cursor++
		}
	case term.KeyHome:
		t.Cursor = 0
	case term.KeyEnd:
		t.Cursor = len(rs)
	case term.KeyEnter:
		return InputSubmit
	case term.KeyEscape:
		return InputCancel
	}
	t.Value = string(rs)
	return InputNone
}

// Display is the value as shown, masked with * when Mask is set.
func (t *TextInput) Display() string {
	if t.Mask {
		return strings.Repeat("*", len([]rune(t.Value)))
	}
	return t.Value
}

// Draw renders the value in w cells and places the cursor when focused.
func (t *TextInput) Draw(c term.Canvas, x, y, w int, st term.Style, focused bool) {
	shown := t.Display()
	c.Text(x, y, shown, st, w)
	if focused {
		before := string([]rune(shown)[:min(t.Cursor, len([]rune(shown)))])
		c.ShowCursor(x+textfmt.Width(before), y)
	}
}
