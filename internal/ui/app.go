package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/theme"
	"github.com/markalexwatson/redditbbs/internal/ui/widgets"
)

type entry struct {
	screen Screen
	ctx    context.Context
	cancel context.CancelFunc
}

type result struct {
	target *entry // nil means the current top screen
	msg    Msg
}

// App owns the terminal and the screen stack.
type App struct {
	t          term.Terminal
	stack      []*entry
	results    chan result
	done       chan struct{}
	w, h       int
	minW, minH int
	quit       bool
}

// Option configures App.
type Option func(*App)

// WithMinSize sets the minimum usable terminal size (default 80x24).
func WithMinSize(w, h int) Option { return func(a *App) { a.minW, a.minH = w, h } }

// New creates an App with root as the first screen and runs its Init.
func New(t term.Terminal, root Screen, opts ...Option) *App {
	a := &App{t: t, results: make(chan result), done: make(chan struct{}), minW: 80, minH: 24}
	for _, o := range opts {
		o(a)
	}
	a.w, a.h = t.Size()
	a.push(root)
	return a
}

// Run processes events until Quit or Ctrl-C, redrawing after each. It calls
// Close on exit.
func (a *App) Run() error {
	defer a.Close()
	a.Draw()
	for !a.quit {
		select {
		case ev, ok := <-a.t.Events():
			if !ok {
				return nil
			}
			a.Handle(ev)
		case r := <-a.results:
			a.deliver(r)
		}
		a.Draw()
	}
	return nil
}

// Close cancels every screen's outstanding work and releases workers. It is
// safe to call more than once; tests that drive Handle and Pump call it in cleanup.
func (a *App) Close() {
	for _, en := range a.stack {
		en.cancel()
	}
	select {
	case <-a.done:
	default:
		close(a.done)
	}
}

// Quitting reports whether a Quit has been applied.
func (a *App) Quitting() bool { return a.quit }

// Depth is the stack height.
func (a *App) Depth() int { return len(a.stack) }

// Top is the current top screen, or nil.
func (a *App) Top() Screen {
	if e := a.top(); e != nil {
		return e.screen
	}
	return nil
}

func (a *App) top() *entry {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) onStack(e *entry) bool {
	for _, x := range a.stack {
		if x == e {
			return true
		}
	}
	return false
}

func (a *App) small() bool { return a.w < a.minW || a.h < a.minH }

// Handle processes one terminal event synchronously.
func (a *App) Handle(ev term.Event) {
	switch e := ev.(type) {
	case term.Resize:
		a.w, a.h = e.W, e.H
		for _, en := range append([]*entry(nil), a.stack...) {
			if a.onStack(en) {
				a.apply(en, en.screen.Update(Resize{W: e.W, H: e.H}))
			}
		}
	case term.Key:
		a.handleKey(e)
	}
}

func (a *App) handleKey(k term.Key) {
	if k.Code == term.KeyCtrlC {
		a.quit = true
		return
	}
	if a.small() {
		return
	}
	top := a.top()
	if top == nil {
		a.quit = true
		return
	}
	if m, ok := top.screen.(Modal); ok && m.CapturesKeys() {
		a.apply(top, top.screen.HandleKey(k))
		return
	}
	if o, ok := top.screen.(Overlayer); ok && o.Overlay() {
		a.apply(top, top.screen.HandleKey(k))
		return
	}
	switch {
	case k.Code == term.KeyRune && k.Rune == '?':
		a.push(newHelp(top.screen))
	case k.Code == term.KeyCtrlL:
		a.t.Sync()
	default:
		a.apply(top, top.screen.HandleKey(k))
	}
}

// Pump waits up to timeout for one async result and delivers it. It is for
// tests; Run does this in its loop.
func (a *App) Pump(timeout time.Duration) bool {
	select {
	case r := <-a.results:
		a.deliver(r)
		return true
	case <-time.After(timeout):
		return false
	}
}

// Post delivers msg to whichever screen is on top when it is processed. It
// blocks until the loop takes it, so never call it from the loop goroutine.
func (a *App) Post(msg Msg) {
	select {
	case a.results <- result{msg: msg}:
	case <-a.done:
	}
}

func (a *App) deliver(r result) {
	target := r.target
	if target == nil {
		target = a.top()
	}
	if target == nil || !a.onStack(target) || r.msg == nil {
		return
	}
	a.apply(target, target.screen.Update(r.msg))
}

// apply performs act on behalf of origin: navigation first, then Runs bound
// to origin if it is still on the stack.
func (a *App) apply(origin *entry, act Action) {
	var runs []Run
	a.applyNav(origin, act, &runs)
	for _, r := range runs {
		if a.onStack(origin) {
			a.start(origin, r)
		}
	}
}

func (a *App) applyNav(origin *entry, act Action, runs *[]Run) {
	switch x := act.(type) {
	case nil:
	case Batch:
		for _, sub := range x.Actions {
			a.applyNav(origin, sub, runs)
		}
	case Run:
		*runs = append(*runs, x)
	case Quit:
		a.quit = true
	case Push:
		if a.top() == origin {
			a.push(x.Screen)
		}
	case Pop:
		if a.top() == origin {
			a.pop(x.Result)
		}
	case Replace:
		if a.top() == origin {
			a.remove()
			a.push(x.Screen)
		}
	}
}

func (a *App) push(s Screen) {
	ctx, cancel := context.WithCancel(context.Background())
	en := &entry{screen: s, ctx: ctx, cancel: cancel}
	a.stack = append(a.stack, en)
	a.apply(en, s.Init())
}

func (a *App) remove() {
	if en := a.top(); en != nil {
		en.cancel()
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) pop(res any) {
	a.remove()
	next := a.top()
	if next == nil {
		a.quit = true
		return
	}
	a.apply(next, next.screen.Update(PopResult{Result: res}))
}

func (a *App) start(en *entry, r Run) {
	go func() {
		var msg Msg
		func() {
			defer func() {
				if p := recover(); p != nil {
					msg = ErrMsg{Err: fmt.Errorf("internal error: %v", p)}
				}
			}()
			msg = r.Fn(en.ctx)
		}()
		select {
		case a.results <- result{target: en, msg: msg}:
		case <-a.done:
		}
	}()
}

// Draw renders the visible screens and presents the frame.
func (a *App) Draw() {
	a.t.Clear()
	a.t.HideCursor()
	if len(a.stack) == 0 {
		a.t.Show()
		return
	}
	if a.small() {
		widgets.Centre(a.t, a.h/2, fmt.Sprintf("Please enlarge your terminal to at least %dx%d", a.minW, a.minH), theme.Style(theme.Error))
		a.t.Show()
		return
	}
	start := len(a.stack) - 1
	for start > 0 {
		if o, ok := a.stack[start].screen.(Overlayer); ok && o.Overlay() {
			start--
			continue
		}
		break
	}
	for i := start; i < len(a.stack); i++ {
		a.drawScreen(a.stack[i].screen)
	}
	a.t.Show()
}

func (a *App) drawScreen(s Screen) {
	if f, ok := s.(Fullscreener); ok && f.Fullscreen() {
		s.Draw(a.t)
		return
	}
	if o, ok := s.(Overlayer); ok && o.Overlay() {
		s.Draw(a.t)
		return
	}
	info := ""
	if i, ok := s.(Infoer); ok {
		info = i.Info()
	}
	widgets.TitleBar(a.t, s.Title(), info)
	widgets.HotkeyBar(a.t, a.h-2, s.Keys())
	var p widgets.Prompt
	if pr, ok := s.(Prompter); ok {
		p = pr.Prompt()
	}
	widgets.PromptLine(a.t, a.h-1, p)
	s.Draw(term.Sub(a.t, 0, widgets.TitleRows, a.w, a.h-widgets.ChromeRows))
}
