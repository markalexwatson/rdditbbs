package term

import (
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type simCell struct {
	text string // "" for blank or for the second half of a wide cluster
	cont bool   // true for the second half of a wide cluster
	st   Style
}

// Sim is an in-memory Terminal for tests. It records every cell itself and
// forwards drawing to a tcell simulation screen so the real code path runs.
type Sim struct {
	mu     sync.Mutex // Run draws from its own goroutine while tests read String()
	s      tcell.SimulationScreen
	w, h   int
	cells  [][]simCell
	events chan Event
	curX   int
	curY   int
	curOn  bool
}

// NewSim creates a simulated terminal of the given size.
func NewSim(w, h int) *Sim {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		panic(err)
	}
	s.SetSize(w, h)
	sim := &Sim{s: s, events: make(chan Event, 64)}
	sim.reset(w, h)
	return sim
}

func (m *Sim) reset(w, h int) {
	m.w, m.h = w, h
	m.cells = make([][]simCell, h)
	for y := range m.cells {
		m.cells[y] = make([]simCell, w)
	}
}

// Size reports the simulated size.
func (m *Sim) Size() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.w, m.h
}

// set assumes m.mu is held.
func (m *Sim) set(x, y int, cluster string, w int, st Style) {
	if y < 0 || y >= m.h || x < 0 || x+w > m.w {
		return
	}
	m.cells[y][x] = simCell{text: cluster, st: st}
	for i := 1; i < w; i++ {
		m.cells[y][x+i] = simCell{cont: true, st: st}
	}
	rs := []rune(cluster)
	m.s.SetContent(x, y, rs[0], rs[1:], toTcell(st))
}

// Put draws one cluster.
func (m *Sim) Put(x, y int, cluster string, st Style) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := uniseg.StringWidth(cluster)
	if w == 0 || cluster == "" || x+w > m.w || x < 0 || y < 0 || y >= m.h {
		return 0
	}
	m.set(x, y, cluster, w, st)
	return w
}

// Text draws s clipped to maxWidth and the right edge.
func (m *Sim) Text(x, y int, s string, st Style, maxWidth int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if x < 0 || y < 0 || y >= m.h {
		return 0
	}
	if r := m.w - x; maxWidth > r {
		maxWidth = r
	}
	return drawText(func(cx int, cl string, w int) { m.set(cx, y, cl, w, st) }, x, s, maxWidth)
}

// Fill sets a rectangle to r.
func (m *Sim) Fill(x, y, w, h int, r rune, st Style) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for yy := y; yy < y+h && yy < m.h; yy++ {
		for xx := x; xx < x+w && xx < m.w; xx++ {
			if xx >= 0 && yy >= 0 {
				m.set(xx, yy, string(r), 1, st)
			}
		}
	}
}

// ShowCursor records the cursor position.
func (m *Sim) ShowCursor(x, y int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.curX, m.curY, m.curOn = x, y, true
}

// HideCursor hides it.
func (m *Sim) HideCursor() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.curOn = false
}

// Cursor reports the cursor state.
func (m *Sim) Cursor() (int, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.curX, m.curY, m.curOn
}

// Events is the injected event stream.
func (m *Sim) Events() <-chan Event { return m.events }

// Inject queues an event as if the user had produced it.
func (m *Sim) Inject(ev Event) { m.events <- ev }

// Resize changes the size and queues a Resize event.
func (m *Sim) Resize(w, h int) {
	m.mu.Lock()
	m.s.SetSize(w, h)
	m.reset(w, h)
	m.mu.Unlock()
	m.Inject(Resize{W: w, H: h})
}

// Show, Sync and Fini forward to the tcell simulation.
func (m *Sim) Show() { m.s.Show() }
func (m *Sim) Sync() { m.s.Sync() }
func (m *Sim) Fini() { m.s.Fini() }

// Clear blanks every cell.
func (m *Sim) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Clear()
	m.reset(m.w, m.h)
}

// CellAt returns the cluster and style at x,y. Continuation cells of a wide
// cluster return "".
func (m *Sim) CellAt(x, y int) (string, Style) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if y < 0 || y >= m.h || x < 0 || x >= m.w {
		return "", Style{}
	}
	c := m.cells[y][x]
	return c.text, c.st
}

// Row returns row y as text with trailing spaces removed.
func (m *Sim) Row(y int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.row(y)
}

// row assumes m.mu is held.
func (m *Sim) row(y int) string {
	var b strings.Builder
	for _, c := range m.cells[y] {
		switch {
		case c.cont:
		case c.text == "":
			b.WriteByte(' ')
		default:
			b.WriteString(c.text)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// String returns every row joined by newlines.
func (m *Sim) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := make([]string, m.h)
	for y := range rows {
		rows[y] = m.row(y)
	}
	return strings.Join(rows, "\n")
}
