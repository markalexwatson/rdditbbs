// Package term defines the terminal abstraction the UI draws through, with a
// tcell implementation for real terminals and a simulation for tests. It is
// the only package that imports tcell.
package term

// Color is one of the 16 ANSI palette colours, or Default for the terminal's own.
type Color uint8

// Palette colours. Values 1..16 map to ANSI palette indices 0..15.
const (
	Default Color = iota
	Black
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
	BrightBlack
	BrightRed
	BrightGreen
	BrightYellow
	BrightBlue
	BrightMagenta
	BrightCyan
	BrightWhite
)

// Style is how a cell is drawn.
type Style struct {
	FG, BG  Color
	Bold    bool
	Reverse bool
}

// Reversed returns the style with reverse video set.
func (s Style) Reversed() Style { s.Reverse = true; return s }

// Bolded returns the style with bold set.
func (s Style) Bolded() Style { s.Bold = true; return s }
