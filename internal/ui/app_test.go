package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// stub is a configurable test screen.
type stub struct {
	name     string
	init     Action
	onKey    func(k term.Key) Action
	onUpdate func(m Msg) Action
	keys     []term.Key
	msgs     []Msg
	drawn    int
	overlay  bool
	full     bool
	modal    bool
}

func (s *stub) Init() Action { return s.init }
func (s *stub) Draw(c term.Canvas) {
	s.drawn++
	c.Text(0, 0, "screen:"+s.name, term.Style{}, 40)
}
func (s *stub) HandleKey(k term.Key) Action {
	s.keys = append(s.keys, k)
	if s.onKey != nil {
		return s.onKey(k)
	}
	return nil
}
func (s *stub) Update(m Msg) Action {
	s.msgs = append(s.msgs, m)
	if s.onUpdate != nil {
		return s.onUpdate(m)
	}
	return nil
}
func (s *stub) Title() string          { return s.name }
func (s *stub) Keys() []KeyHelp        { return []KeyHelp{{Key: "X", Desc: "test"}} }
func (s *stub) Overlay() bool          { return s.overlay }
func (s *stub) Fullscreen() bool       { return s.full }
func (s *stub) CapturesKeys() bool     { return s.modal }
func (s *stub) Prompt() widgets.Prompt { return widgets.Prompt{Status: "st:" + s.name} }

func newApp(root Screen) (*App, *term.Sim) {
	sim := term.NewSim(80, 24)
	return New(sim, root), sim
}

func TestChromeAndContentSubCanvas(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	app.Draw()
	if !strings.Contains(sim.Row(1), "R E D D I T   B B S") || !strings.Contains(sim.Row(1), "root") {
		t.Errorf("title = %q", sim.Row(1))
	}
	if sim.Row(3) != "screen:root" {
		t.Errorf("content should start on row 3, got %q", sim.Row(3))
	}
	if !strings.Contains(sim.Row(22), "[X]test") || !strings.Contains(sim.Row(23), "st:root") {
		t.Errorf("footer = %q / %q", sim.Row(22), sim.Row(23))
	}
}

func TestFullscreenSkipsChrome(t *testing.T) {
	app, sim := newApp(&stub{name: "splash", full: true})
	app.Draw()
	if sim.Row(0) != "screen:splash" {
		t.Errorf("row 0 = %q", sim.Row(0))
	}
}

func TestPushPopWithResult(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	child.onKey = func(k term.Key) Action { return Pop{Result: "picked"} }
	app, sim := newApp(root)
	app.Handle(term.R('a'))
	if app.Depth() != 2 || app.Top() != child {
		t.Fatalf("depth=%d", app.Depth())
	}
	app.Draw()
	if sim.Row(3) != "screen:child" {
		t.Errorf("child not drawn: %q", sim.Row(3))
	}
	app.Handle(term.R('b'))
	if app.Depth() != 1 {
		t.Fatalf("pop failed, depth=%d", app.Depth())
	}
	if len(root.msgs) != 1 {
		t.Fatalf("root msgs = %v", root.msgs)
	}
	if pr, ok := root.msgs[0].(PopResult); !ok || pr.Result != "picked" {
		t.Errorf("pop result = %#v", root.msgs[0])
	}
}

func TestPopLastScreenQuits(t *testing.T) {
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Pop{} }
	app, _ := newApp(root)
	app.Handle(term.R('q'))
	if !app.Quitting() {
		t.Error("popping the last screen should quit")
	}
	app.Draw() // must not panic with an empty stack
}

func TestCloseCancelsOutstandingRuns(t *testing.T) {
	cancelled := make(chan struct{})
	root := &stub{name: "root"}
	root.init = Run{Fn: func(ctx context.Context) Msg { <-ctx.Done(); close(cancelled); return nil }}
	app, _ := newApp(root)
	app.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the run context")
	}
	app.Close() // idempotent
}

func TestReplace(t *testing.T) {
	next := &stub{name: "next"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Replace{next} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	if app.Depth() != 1 || app.Top() != next {
		t.Error("replace failed")
	}
	if len(root.msgs) != 0 {
		t.Error("replace must not deliver PopResult")
	}
}

func TestRunDeliversToOriginOnly(t *testing.T) {
	root := &stub{name: "root"}
	root.init = Run{Fn: func(ctx context.Context) Msg { return "hello" }}
	app, _ := newApp(root)
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	if len(root.msgs) != 1 || root.msgs[0] != "hello" {
		t.Errorf("msgs = %v", root.msgs)
	}
}

func TestRunDroppedAfterPop(t *testing.T) {
	release := make(chan struct{})
	child := &stub{name: "child"}
	child.init = Run{Fn: func(ctx context.Context) Msg {
		<-release
		return "late"
	}}
	child.onKey = func(k term.Key) Action { return Pop{} }
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b')) // pop child while its Run is outstanding
	close(release)
	app.Pump(time.Second) // the late result arrives but has no live target
	if len(child.msgs) != 0 {
		t.Errorf("child got %v", child.msgs)
	}
}

func TestRunContextCancelledOnPop(t *testing.T) {
	cancelled := make(chan struct{})
	child := &stub{name: "child"}
	child.init = Run{Fn: func(ctx context.Context) Msg {
		<-ctx.Done()
		close(cancelled)
		return nil
	}}
	child.onKey = func(k term.Key) Action { return Pop{} }
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b'))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("context not cancelled on pop")
	}
}

func TestBatchPushThenRunBindsToOrigin(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action {
		return Batch{Actions: []Action{Push{child}, Run{Fn: func(ctx context.Context) Msg { return "for-root" }}}}
	}
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	if len(root.msgs) != 1 || root.msgs[0] != "for-root" || len(child.msgs) != 0 {
		t.Errorf("root=%v child=%v", root.msgs, child.msgs)
	}
}

func TestBatchPopDiscardsOwnRuns(t *testing.T) {
	ran := make(chan struct{}, 1)
	child := &stub{name: "child"}
	child.onKey = func(k term.Key) Action {
		return Batch{Actions: []Action{Pop{}, Run{Fn: func(ctx context.Context) Msg { ran <- struct{}{}; return "x" }}}}
	}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b'))
	select {
	case <-ran:
		t.Error("run from a popped screen must not start")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCoveredScreenNavigationIgnored(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	root.onUpdate = func(m Msg) Action { return Pop{} }
	root.init = nil
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.Resize{W: 100, H: 30}) // both screens get Resize; root's Pop must be ignored
	if app.Depth() != 2 {
		t.Errorf("depth = %d", app.Depth())
	}
	if len(root.msgs) != 1 || len(child.msgs) != 1 {
		t.Errorf("resize delivery root=%v child=%v", root.msgs, child.msgs)
	}
}

func TestWorkerPanicBecomesErrMsg(t *testing.T) {
	root := &stub{name: "root"}
	root.init = Run{Fn: func(ctx context.Context) Msg { panic("boom") }}
	app, _ := newApp(root)
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	em, ok := root.msgs[0].(ErrMsg)
	if !ok || !strings.Contains(em.Err.Error(), "boom") {
		t.Errorf("msg = %#v", root.msgs[0])
	}
}

func TestHelpOverlayAndGlobalKeys(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	app.Handle(term.R('?'))
	if app.Depth() != 2 {
		t.Fatal("help not pushed")
	}
	app.Draw()
	out := sim.String()
	if !strings.Contains(out, "screen:root") || !strings.Contains(out, "[X] test") || !strings.Contains(out, "Ctrl-C") {
		t.Errorf("overlay should draw over root and list keys:\n%s", out)
	}
	app.Handle(term.R('z'))
	if app.Depth() != 1 || len(root.keys) != 0 {
		t.Error("any key should close help without reaching root")
	}
	app.Handle(term.K(term.KeyCtrlC))
	if !app.Quitting() {
		t.Error("Ctrl-C must quit")
	}
}

func TestModalScreenGetsQuestionMark(t *testing.T) {
	root := &stub{name: "root", modal: true}
	app, _ := newApp(root)
	app.Handle(term.R('?'))
	if app.Depth() != 1 || len(root.keys) != 1 {
		t.Error("modal screen should receive ? itself")
	}
}

func TestUndersizedTerminal(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	sim.Resize(60, 20)
	app.Handle(<-sim.Events())
	app.Draw()
	if !strings.Contains(sim.String(), "80x24") {
		t.Errorf("expected enlarge message:\n%s", sim.String())
	}
	app.Handle(term.R('a'))
	if len(root.keys) != 0 {
		t.Error("keys must be swallowed while undersized")
	}
	sim.Resize(80, 24)
	app.Handle(<-sim.Events())
	app.Handle(term.R('a'))
	if len(root.keys) != 1 {
		t.Error("keys should flow again after resize")
	}
}

func TestPostDeliversToTop(t *testing.T) {
	root := &stub{name: "root"}
	app, _ := newApp(root)
	go app.Post(RateLimited{Wait: 3 * time.Second})
	if !app.Pump(time.Second) {
		t.Fatal("no message")
	}
	if rl, ok := root.msgs[0].(RateLimited); !ok || rl.Wait != 3*time.Second {
		t.Errorf("msg = %#v", root.msgs[0])
	}
}

func TestSleepHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m := Sleep(ctx, time.Hour, 7).Fn(ctx); m != nil {
		t.Errorf("cancelled sleep should yield nil, got %#v", m)
	}
	if m := Sleep(context.Background(), time.Millisecond, 7).Fn(context.Background()); m != (Tick{ID: 7}) {
		t.Errorf("got %#v", m)
	}
}

func TestRunLoopQuitsOnCtrlC(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	sim.Inject(term.K(term.KeyCtrlC))
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit")
	}
}
