package term

import (
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// grid draws onto a tcell.Screen.
type grid struct{ s tcell.Screen }

func toTcell(st Style) tcell.Style {
	s := tcell.StyleDefault
	if st.FG != Default {
		s = s.Foreground(tcell.PaletteColor(int(st.FG) - 1))
	}
	if st.BG != Default {
		s = s.Background(tcell.PaletteColor(int(st.BG) - 1))
	}
	return s.Bold(st.Bold).Reverse(st.Reverse)
}

func (g grid) Size() (int, int) { return g.s.Size() }

func (g grid) set(x, y int, cluster string, st Style) {
	rs := []rune(cluster)
	g.s.SetContent(x, y, rs[0], rs[1:], toTcell(st))
}

func (g grid) Put(x, y int, cluster string, st Style) int {
	w := uniseg.StringWidth(cluster)
	sw, sh := g.s.Size()
	if w == 0 || cluster == "" || x < 0 || y < 0 || y >= sh || x+w > sw {
		return 0
	}
	g.set(x, y, cluster, st)
	return w
}

func (g grid) Text(x, y int, s string, st Style, maxWidth int) int {
	sw, sh := g.s.Size()
	if x < 0 || y < 0 || y >= sh {
		return 0
	}
	if r := sw - x; maxWidth > r {
		maxWidth = r
	}
	return drawText(func(cx int, cl string, _ int) { g.set(cx, y, cl, st) }, x, s, maxWidth)
}

func (g grid) Fill(x, y, w, h int, r rune, st Style) {
	ts := toTcell(st)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			g.s.SetContent(xx, yy, r, nil, ts)
		}
	}
}

func (g grid) ShowCursor(x, y int) { g.s.ShowCursor(x, y) }
func (g grid) HideCursor()         { g.s.HideCursor() }

type tcellTerm struct {
	grid
	events chan Event
	quit   chan struct{}
	once   sync.Once
}

// NewTcell initialises the real terminal. Call Fini to restore it.
func NewTcell() (Terminal, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnablePaste()
	s.Clear()
	t := &tcellTerm{grid: grid{s}, events: make(chan Event, 64), quit: make(chan struct{})}
	go t.pump()
	return t, nil
}

func (t *tcellTerm) pump() {
	ch := make(chan tcell.Event, 16)
	go t.s.ChannelEvents(ch, t.quit)
	paste := false
	for ev := range ch {
		switch e := ev.(type) {
		case *tcell.EventResize:
			w, h := e.Size()
			t.events <- Resize{W: w, H: h}
		case *tcell.EventPaste:
			paste = e.Start()
		case *tcell.EventKey:
			if k, ok := translate(e, paste); ok {
				t.events <- k
			}
		}
	}
	close(t.events)
}

func translate(e *tcell.EventKey, paste bool) (Key, bool) {
	codes := map[tcell.Key]KeyCode{
		tcell.KeyEnter: KeyEnter, tcell.KeyEscape: KeyEscape,
		tcell.KeyBackspace: KeyBackspace, tcell.KeyBackspace2: KeyBackspace,
		tcell.KeyDelete: KeyDelete, tcell.KeyTab: KeyTab,
		tcell.KeyUp: KeyUp, tcell.KeyDown: KeyDown, tcell.KeyLeft: KeyLeft, tcell.KeyRight: KeyRight,
		tcell.KeyPgUp: KeyPgUp, tcell.KeyPgDn: KeyPgDn, tcell.KeyHome: KeyHome, tcell.KeyEnd: KeyEnd,
		tcell.KeyCtrlC: KeyCtrlC, tcell.KeyCtrlL: KeyCtrlL,
	}
	if e.Key() == tcell.KeyRune {
		return Key{Code: KeyRune, Rune: e.Rune(), Paste: paste}, true
	}
	if c, ok := codes[e.Key()]; ok {
		return Key{Code: c, Paste: paste}, true
	}
	return Key{}, false
}

func (t *tcellTerm) Events() <-chan Event { return t.events }
func (t *tcellTerm) Show()                { t.s.Show() }
func (t *tcellTerm) Sync()                { t.s.Sync() }
func (t *tcellTerm) Clear()               { t.s.Clear() }

// Fini restores the terminal. Safe to call more than once.
func (t *tcellTerm) Fini() {
	t.once.Do(func() {
		close(t.quit)
		t.s.Fini()
	})
}
