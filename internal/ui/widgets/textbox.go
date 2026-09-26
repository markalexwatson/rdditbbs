package widgets

import (
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/textfmt"
	"github.com/markalexwatson/redditbbs/internal/theme"
)

// TextBox shows pre-wrapped lines from a scroll offset.
type TextBox struct {
	Lines []textfmt.Line
	Top   int
}

// Draw renders up to h lines starting at Top.
func (b *TextBox) Draw(c term.Canvas, x, y, w, h int) {
	for i := 0; i < h && b.Top+i < len(b.Lines); i++ {
		DrawLine(c, x, y+i, w, b.Lines[b.Top+i])
	}
}

// Scroll moves Top by delta, clamped so the last page is full where possible.
func (b *TextBox) Scroll(delta, h int) {
	b.Top += delta
	if max := len(b.Lines) - h; b.Top > max {
		b.Top = max
	}
	if b.Top < 0 {
		b.Top = 0
	}
}

// AtEnd reports whether the last line is visible.
func (b *TextBox) AtEnd(h int) bool { return b.Top+h >= len(b.Lines) }

// KindRole maps a span kind to its theme role.
func KindRole(k textfmt.Kind) theme.Role {
	switch k {
	case textfmt.Quote:
		return theme.Quote
	case textfmt.Code:
		return theme.Code
	case textfmt.Bold:
		return theme.Bold
	case textfmt.Link:
		return theme.Link
	default:
		return theme.Body
	}
}

// DrawLine draws spans with their own styles, clipped to w cells.
func DrawLine(c term.Canvas, x, y, w int, l textfmt.Line) int {
	used := 0
	for _, s := range l {
		if used >= w {
			break
		}
		used += c.Text(x+used, y, s.Text, theme.Style(KindRole(s.Kind)), w-used)
	}
	return used
}

// DrawLineStyled draws spans in one style, for cursor rows.
func DrawLineStyled(c term.Canvas, x, y, w int, l textfmt.Line, st term.Style) int {
	used := 0
	for _, s := range l {
		if used >= w {
			break
		}
		used += c.Text(x+used, y, s.Text, st, w-used)
	}
	return used
}
