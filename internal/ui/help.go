package ui

import (
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/textfmt"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

// helpScreen is the overlay listing the keys of the screen beneath it.
type helpScreen struct {
	title string
	keys  []KeyHelp
}

func newHelp(under Screen) *helpScreen {
	return &helpScreen{title: under.Title(), keys: under.Keys()}
}

func (h *helpScreen) Init() Action              { return nil }
func (h *helpScreen) HandleKey(term.Key) Action { return Pop{} }
func (h *helpScreen) Update(Msg) Action         { return nil }
func (h *helpScreen) Title() string             { return "Help" }
func (h *helpScreen) Keys() []KeyHelp           { return nil }
func (h *helpScreen) Overlay() bool             { return true }

func (h *helpScreen) Draw(c term.Canvas) {
	w, hgt := c.Size()
	lines := []string{"Keys for " + h.title + ":", ""}
	for _, k := range h.keys {
		lines = append(lines, "  ["+k.Key+"] "+k.Desc)
	}
	lines = append(lines, "", "Everywhere:", "")
	for _, k := range GlobalKeys {
		lines = append(lines, "  ["+k.Key+"] "+k.Desc)
	}
	lines = append(lines, "", "Press any key to close")
	bw := 30
	for _, l := range lines {
		if n := textfmt.Width(l) + 4; n > bw {
			bw = n
		}
	}
	if bw > w-2 {
		bw = w - 2
	}
	bh := len(lines) + 2
	if bh > hgt-2 {
		bh = hgt - 2
	}
	x0, y0 := (w-bw)/2, (hgt-bh)/2
	fr := theme.Style(theme.Frame)
	c.Fill(x0, y0, bw, bh, ' ', term.Style{})
	c.Put(x0, y0, "╔", fr)
	c.Fill(x0+1, y0, bw-2, 1, '═', fr)
	c.Put(x0+bw-1, y0, "╗", fr)
	c.Put(x0, y0+bh-1, "╚", fr)
	c.Fill(x0+1, y0+bh-1, bw-2, 1, '═', fr)
	c.Put(x0+bw-1, y0+bh-1, "╝", fr)
	for i := 1; i < bh-1; i++ {
		c.Put(x0, y0+i, "║", fr)
		c.Put(x0+bw-1, y0+i, "║", fr)
	}
	for i, l := range lines {
		if i+1 >= bh-1 {
			break
		}
		st := theme.Style(theme.Body)
		if i == 0 || l == "Everywhere:" {
			st = theme.Style(theme.Heading)
		}
		c.Text(x0+2, y0+1+i, textfmt.Truncate(l, bw-4), st, bw-4)
	}
	widgets.Centre(c, y0+bh-1, " Help ", theme.Style(theme.Heading))
}
