package term

import "testing"

func TestSubClipsAndOffsets(t *testing.T) {
	sim := NewSim(20, 6)
	sub := Sub(sim, 5, 2, 8, 2)
	if w, h := sub.Size(); w != 8 || h != 2 {
		t.Fatalf("size = %dx%d", w, h)
	}
	n := sub.Text(0, 0, "abcdefghijklmnop", Style{}, 100)
	if n != 8 {
		t.Errorf("Text drew %d cells, want 8", n)
	}
	if got := sim.Row(2); got != "     abcdefgh" {
		t.Errorf("row 2 = %q", got)
	}
	sub.Text(0, 5, "outside", Style{}, 10)
	if got := sim.Row(5); got != "" {
		t.Errorf("row 5 should be untouched, got %q", got)
	}
	sub.Fill(-2, 1, 100, 100, '#', Style{})
	if got := sim.Row(3); got != "     ########" {
		t.Errorf("fill row 3 = %q", got)
	}
	if got := sim.Row(4); got != "" {
		t.Errorf("fill should not spill to row 4, got %q", got)
	}
}

func TestSubClampsToParent(t *testing.T) {
	sim := NewSim(10, 4)
	sub := Sub(sim, 6, 2, 10, 10)
	if w, h := sub.Size(); w != 4 || h != 2 {
		t.Errorf("size = %dx%d, want 4x2", w, h)
	}
}

func TestSubCursor(t *testing.T) {
	sim := NewSim(10, 4)
	Sub(sim, 3, 1, 5, 2).ShowCursor(1, 1)
	if x, y, shown := sim.Cursor(); !shown || x != 4 || y != 2 {
		t.Errorf("cursor = %d,%d,%v", x, y, shown)
	}
}
