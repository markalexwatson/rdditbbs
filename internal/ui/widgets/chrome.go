// Package widgets holds drawing helpers and small stateful controls shared by
// the screens: chrome, tables, inputs, text boxes and tree connectors.
package widgets

import (
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/textfmt"
	"github.com/markalexwatson/redditbbs/internal/theme"
)

// Chrome heights.
const (
	TitleRows  = 3
	FooterRows = 2
	ChromeRows = TitleRows + FooterRows
)

// KeyHelp is one hotkey and its description, shown as [K]desc.
type KeyHelp struct {
	Key, Desc string
}

// Prompt is the state of the bottom prompt line.
type Prompt struct {
	Label     string // defaults to "Command:"
	Input     string
	Status    string
	Error     bool // draw Status in the error style
	Cursor    bool // show the terminal cursor within Input
	CursorPos int  // rune offset of the cursor in Input; 0 with Cursor set means the end
}

const logo = "R E D D I T   B B S"

// TitleBar draws the three-row framed title at the top of c.
func TitleBar(c term.Canvas, title, info string) {
	w, _ := c.Size()
	fr := theme.Style(theme.Frame)
	c.Put(0, 0, "╔", fr)
	c.Fill(1, 0, w-2, 1, '═', fr)
	c.Put(w-1, 0, "╗", fr)
	c.Put(0, 1, "║", fr)
	c.Fill(1, 1, w-2, 1, ' ', term.Style{})
	c.Put(w-1, 1, "║", fr)
	c.Put(0, 2, "╚", fr)
	c.Fill(1, 2, w-2, 1, '═', fr)
	c.Put(w-1, 2, "╝", fr)

	infoW := textfmt.Width(info)
	x := 2
	x += c.Text(x, 1, logo, theme.Style(theme.Logo), w)
	x += c.Text(x, 1, " · ", theme.Style(theme.Meta), w)
	avail := w - 2 - infoW - 1 - x
	if avail > 0 {
		c.Text(x, 1, textfmt.Truncate(title, avail), theme.Style(theme.Subject), avail)
	}
	c.Text(w-2-infoW, 1, info, theme.Style(theme.Meta), infoW) // ends one cell before the frame
}

// HotkeyBar draws [K]desc pairs across row y.
func HotkeyBar(c term.Canvas, y int, keys []KeyHelp) {
	w, _ := c.Size()
	x := 2
	meta, hot, body := theme.Style(theme.Meta), theme.Style(theme.Hotkey), theme.Style(theme.Body)
	for _, k := range keys {
		need := 3 + textfmt.Width(k.Key) + textfmt.Width(k.Desc)
		if x+need > w {
			return
		}
		x += c.Text(x, y, "[", meta, w)
		x += c.Text(x, y, k.Key, hot, w)
		x += c.Text(x, y, "]", meta, w)
		x += c.Text(x, y, k.Desc, body, w)
		x++
	}
}

// PromptLine draws the label, typed input and right-aligned status on row y.
func PromptLine(c term.Canvas, y int, p Prompt) {
	w, _ := c.Size()
	label := p.Label
	if label == "" {
		label = "Command:"
	}
	x := 2
	x += c.Text(x, y, label, theme.Style(theme.Prompt), w)
	x++
	inputX := x
	x += c.Text(x, y, p.Input, theme.Style(theme.Subject), w-x)
	if p.Cursor {
		rs := []rune(p.Input)
		pos := p.CursorPos
		if pos <= 0 || pos > len(rs) {
			pos = len(rs)
		}
		c.ShowCursor(inputX+textfmt.Width(string(rs[:pos])), y)
	} else {
		c.HideCursor()
	}
	if p.Status == "" {
		return
	}
	st := theme.Style(theme.Meta)
	if p.Error {
		st = theme.Style(theme.Error)
	}
	avail := w - 2 - (x + 1)
	status := textfmt.Truncate(p.Status, avail)
	c.Text(w-2-textfmt.Width(status), y, status, st, w)
}

// Rule draws a full-width horizontal rule on row y.
func Rule(c term.Canvas, y int) {
	w, _ := c.Size()
	c.Fill(0, y, w, 1, '─', theme.Style(theme.Rule))
}

// Centre draws s centred on row y.
func Centre(c term.Canvas, y int, s string, st term.Style) {
	w, _ := c.Size()
	s = textfmt.Truncate(s, w)
	c.Text((w-textfmt.Width(s))/2, y, s, st, w)
}
