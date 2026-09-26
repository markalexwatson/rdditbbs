package widgets

import "github.com/markalexwatson/redditbbs/internal/term"

// Table tracks a cursor over count rows shown height at a time, keeping the
// cursor visible.
type Table struct {
	Cursor, Top   int
	count, height int
}

// SetCount sets the number of rows and clamps.
func (t *Table) SetCount(n int) { t.count = n; t.clamp() }

// SetHeight sets the visible rows and clamps.
func (t *Table) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	t.height = h
	t.clamp()
}

// Count is the row count.
func (t *Table) Count() int { return t.count }

// Height is the visible row count.
func (t *Table) Height() int { return t.height }

func (t *Table) clamp() {
	if t.Cursor >= t.count {
		t.Cursor = t.count - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	if t.height < 1 {
		t.height = 1
	}
	if t.Cursor < t.Top {
		t.Top = t.Cursor
	}
	if t.Cursor >= t.Top+t.height {
		t.Top = t.Cursor - t.height + 1
	}
	if t.Top > t.count-t.height {
		t.Top = t.count - t.height
	}
	if t.Top < 0 {
		t.Top = 0
	}
}

// Move shifts the cursor by delta.
func (t *Table) Move(delta int) { t.Cursor += delta; t.clamp() }

// Select moves the cursor to row i.
func (t *Table) Select(i int) { t.Cursor = i; t.clamp() }

// HandleKey applies arrow, page, Home and End keys. It returns false for other keys.
func (t *Table) HandleKey(k term.Key) bool {
	switch k.Code {
	case term.KeyUp:
		t.Move(-1)
	case term.KeyDown:
		t.Move(1)
	case term.KeyPgUp:
		t.Move(-t.height)
	case term.KeyPgDn:
		t.Move(t.height)
	case term.KeyHome:
		t.Select(0)
	case term.KeyEnd:
		t.Select(t.count - 1)
	default:
		return false
	}
	return true
}

// Visible returns the half-open range of rows on screen.
func (t *Table) Visible() (start, end int) {
	end = t.Top + t.height
	if end > t.count {
		end = t.count
	}
	return t.Top, end
}
