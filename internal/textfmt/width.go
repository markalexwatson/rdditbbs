// Package textfmt measures, wraps and formats text for a cell-based
// terminal, treating grapheme clusters as indivisible.
package textfmt

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Width returns the number of terminal cells s occupies.
func Width(s string) int { return uniseg.StringWidth(s) }

// Clip returns the longest prefix of whole grapheme clusters that fits in w cells.
func Clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cw := g.Width()
		if used+cw > w {
			break
		}
		b.WriteString(g.Str())
		used += cw
	}
	return b.String()
}

// Truncate returns s unchanged if it fits in w cells, otherwise the longest
// cluster prefix that fits in w-1 cells followed by an ellipsis.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return Clip(s, w-1) + "…"
}

// PadRight returns s truncated or space-padded to exactly w cells.
func PadRight(s string, w int) string {
	s = Truncate(s, w)
	return s + strings.Repeat(" ", w-Width(s))
}

// PadLeft returns s truncated or left-padded with spaces to exactly w cells.
func PadLeft(s string, w int) string {
	s = Truncate(s, w)
	return strings.Repeat(" ", w-Width(s)) + s
}

// Wrap splits s into lines of at most w cells, breaking on whitespace and
// hard-breaking words wider than w by cluster. Empty input yields one empty
// line. w <= 0 yields nil.
func Wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curW = 0
	}
	for _, word := range strings.Fields(s) {
		ww := Width(word)
		if ww > w {
			if curW > 0 {
				flush()
			}
			g := uniseg.NewGraphemes(word)
			for g.Next() {
				cw := g.Width()
				if curW+cw > w {
					flush()
				}
				cur.WriteString(g.Str())
				curW += cw
			}
			continue
		}
		switch {
		case curW == 0:
			cur.WriteString(word)
			curW = ww
		case curW+1+ww > w:
			flush()
			cur.WriteString(word)
			curW = ww
		default:
			cur.WriteByte(' ')
			cur.WriteString(word)
			curW += 1 + ww
		}
	}
	if curW > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}
