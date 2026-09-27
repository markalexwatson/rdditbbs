// Package ui runs the screen stack: it owns the terminal, routes keys and
// async results to screens, draws the shared chrome and applies navigation
// actions. Screens never touch the terminal lifecycle or each other.
package ui

import (
	"context"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

// Msg is anything delivered to a screen's Update.
type Msg any

// Action is what a screen returns from Init, HandleKey or Update: nil, Push,
// Pop, Replace, Quit, Run or Batch.
type Action any

// Push puts a screen on top of the stack.
type Push struct{ Screen Screen }

// Pop removes the top screen and delivers PopResult{Result} to the one beneath.
type Pop struct{ Result any }

// Replace swaps the top screen without delivering a PopResult.
type Replace struct{ Screen Screen }

// Quit ends the application.
type Quit struct{}

// Run executes Fn in a goroutine and delivers its Msg to the screen that
// returned the Run. ctx is cancelled when that screen is popped.
type Run struct{ Fn func(ctx context.Context) Msg }

// Batch applies several actions: navigation first, then Runs.
type Batch struct{ Actions []Action }

// PopResult carries the result of a popped child screen.
type PopResult struct{ Result any }

// Resize reports the new terminal size to every screen on the stack.
type Resize struct{ W, H int }

// Tick is produced by Sleep.
type Tick struct{ ID int }

// ErrMsg reports a worker panic or an App-level error.
type ErrMsg struct{ Err error }

// RateLimited tells the top screen the client is waiting on Reddit's limit.
type RateLimited struct{ Wait time.Duration }

// KeyHelp is re-exported for screens.
type KeyHelp = widgets.KeyHelp

// Screen is one full-screen view.
type Screen interface {
	Init() Action
	Draw(c term.Canvas)
	HandleKey(k term.Key) Action
	Update(msg Msg) Action
	Title() string
	Keys() []KeyHelp
}

// Overlayer screens draw on top of the screen beneath them.
type Overlayer interface{ Overlay() bool }

// Fullscreener screens draw without chrome over the whole terminal.
type Fullscreener interface{ Fullscreen() bool }

// Modal screens receive every key, including the global ones except Ctrl-C.
type Modal interface{ CapturesKeys() bool }

// Prompter screens supply the prompt line contents.
type Prompter interface{ Prompt() widgets.Prompt }

// Infoer screens supply the right-hand title bar text.
type Infoer interface{ Info() string }

// Sleep returns a Run that yields Tick{id} after d, or nil if cancelled.
func Sleep(ctx context.Context, d time.Duration, id int) Run {
	return Run{Fn: func(ctx context.Context) Msg {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			return Tick{ID: id}
		}
	}}
}

// GlobalKeys are handled by App on every screen.
var GlobalKeys = []KeyHelp{
	{Key: "?", Desc: "Help"},
	{Key: "Q/Esc", Desc: "Back"},
	{Key: "↑↓ PgUp PgDn", Desc: "Move"},
	{Key: "0-9 ⏎", Desc: "Select by number"},
	{Key: "Ctrl-T", Desc: "Next theme"},
	{Key: "Ctrl-L", Desc: "Redraw"},
	{Key: "Ctrl-C", Desc: "Quit"},
}
