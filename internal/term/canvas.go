package term

import "github.com/rivo/uniseg"

// Canvas is a rectangle of cells that screens draw into. Coordinates are
// local to the canvas; drawing outside it is ignored.
type Canvas interface {
	Size() (w, h int)
	// Put draws one grapheme cluster and returns the cells it occupies (0, 1 or 2).
	Put(x, y int, cluster string, st Style) int
	// Text draws s from x,y, clipping at maxWidth cells and at the canvas edge.
	// It returns the cells used.
	Text(x, y int, s string, st Style, maxWidth int) int
	Fill(x, y, w, h int, r rune, st Style)
	ShowCursor(x, y int)
	HideCursor()
}

// Terminal is a Canvas with an event source and a presentation lifecycle.
type Terminal interface {
	Canvas
	Events() <-chan Event
	Show()
	Sync()
	Clear()
	Fini()
}

type sub struct {
	p          Canvas
	x, y, w, h int
}

// Sub returns a canvas for the rectangle x,y,w,h of c, clamped to c's bounds.
func Sub(c Canvas, x, y, w, h int) Canvas {
	pw, ph := c.Size()
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > pw {
		w = pw - x
	}
	if y+h > ph {
		h = ph - y
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &sub{p: c, x: x, y: y, w: w, h: h}
}

func (s *sub) Size() (int, int) { return s.w, s.h }

func (s *sub) Put(x, y int, cluster string, st Style) int {
	if x < 0 || y < 0 || y >= s.h || x+uniseg.StringWidth(cluster) > s.w {
		return 0
	}
	return s.p.Put(s.x+x, s.y+y, cluster, st)
}

func (s *sub) Text(x, y int, str string, st Style, maxWidth int) int {
	if x < 0 || y < 0 || y >= s.h || x >= s.w {
		return 0
	}
	if m := s.w - x; maxWidth > m {
		maxWidth = m
	}
	return s.p.Text(s.x+x, s.y+y, str, st, maxWidth)
}

func (s *sub) Fill(x, y, w, h int, r rune, st Style) {
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > s.w {
		w = s.w - x
	}
	if y+h > s.h {
		h = s.h - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	s.p.Fill(s.x+x, s.y+y, w, h, r, st)
}

func (s *sub) ShowCursor(x, y int) { s.p.ShowCursor(s.x+x, s.y+y) }
func (s *sub) HideCursor()         { s.p.HideCursor() }

// drawText walks s by grapheme cluster, calling set for each cluster that
// fits within maxWidth cells, and returns the cells used. Zero-width
// clusters are skipped.
func drawText(set func(x int, cluster string, w int), x int, s string, maxWidth int) int {
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := g.Width()
		if w == 0 {
			continue
		}
		if used+w > maxWidth {
			break
		}
		set(x+used, g.Str(), w)
		used += w
	}
	return used
}
