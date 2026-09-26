package term

// KeyCode identifies a key. KeyRune carries a printable rune in Key.Rune.
type KeyCode int

// Key codes.
const (
	KeyRune KeyCode = iota
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyDelete
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyCtrlC
	KeyCtrlL
)

// Key is a key press. Paste is true for runes delivered inside a bracketed paste.
type Key struct {
	Code  KeyCode
	Rune  rune
	Paste bool
}

// Resize reports a new terminal size.
type Resize struct{ W, H int }

// Event is a Key or a Resize.
type Event any

// R builds a rune key.
func R(r rune) Key { return Key{Code: KeyRune, Rune: r} }

// K builds a special key.
func K(c KeyCode) Key { return Key{Code: c} }
