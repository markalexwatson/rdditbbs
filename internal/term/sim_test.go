package term

import "testing"

func TestSimTextAndWideCells(t *testing.T) {
	sim := NewSim(10, 2)
	n := sim.Text(0, 0, "日本x", Style{FG: Cyan}, 10)
	if n != 5 {
		t.Errorf("used %d cells, want 5", n)
	}
	if got := sim.Row(0); got != "日本x" {
		t.Errorf("row = %q", got)
	}
	if s, st := sim.CellAt(2, 0); s != "本" || st.FG != Cyan {
		t.Errorf("cell 2 = %q %+v", s, st)
	}
	if s, _ := sim.CellAt(1, 0); s != "" {
		t.Errorf("continuation cell should be empty, got %q", s)
	}
}

func TestSimTextClipsAtMaxWidth(t *testing.T) {
	sim := NewSim(10, 1)
	if n := sim.Text(0, 0, "abcdef", Style{}, 3); n != 3 {
		t.Errorf("used %d, want 3", n)
	}
	if got := sim.Row(0); got != "abc" {
		t.Errorf("row = %q", got)
	}
	if n := sim.Text(9, 0, "日", Style{}, 5); n != 0 {
		t.Errorf("wide cluster at last column should not draw, used %d", n)
	}
}

func TestSimPutCombining(t *testing.T) {
	sim := NewSim(4, 1)
	if n := sim.Put(0, 0, "é", Style{}); n != 1 {
		t.Errorf("width %d", n)
	}
	if s, _ := sim.CellAt(0, 0); s != "é" {
		t.Errorf("cell = %q", s)
	}
}

func TestSimStringClearAndResize(t *testing.T) {
	sim := NewSim(5, 2)
	sim.Text(0, 0, "hi", Style{}, 5)
	sim.Text(0, 1, "yo", Style{}, 5)
	if got := sim.String(); got != "hi\nyo" {
		t.Errorf("String = %q", got)
	}
	sim.Clear()
	if got := sim.String(); got != "\n" {
		t.Errorf("after Clear = %q", got)
	}
	sim.Resize(8, 3)
	if w, h := sim.Size(); w != 8 || h != 3 {
		t.Errorf("size = %dx%d", w, h)
	}
	ev := <-sim.Events()
	if r, ok := ev.(Resize); !ok || r.W != 8 || r.H != 3 {
		t.Errorf("event = %#v", ev)
	}
}

func TestSimInject(t *testing.T) {
	sim := NewSim(5, 2)
	sim.Inject(R('q'))
	if k, ok := (<-sim.Events()).(Key); !ok || k.Rune != 'q' {
		t.Errorf("got %#v", k)
	}
}
