package term

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestToTcellStyle(t *testing.T) {
	fg, bg, attrs := toTcell(Style{FG: Cyan, BG: BrightWhite, Bold: true, Reverse: true}).Decompose()
	if fg != tcell.PaletteColor(6) {
		t.Errorf("fg = %v", fg)
	}
	if bg != tcell.PaletteColor(15) {
		t.Errorf("bg = %v", bg)
	}
	if attrs&tcell.AttrBold == 0 || attrs&tcell.AttrReverse == 0 {
		t.Errorf("attrs = %v", attrs)
	}
	fg, bg, _ = toTcell(Style{}).Decompose()
	if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
		t.Errorf("default colours = %v %v", fg, bg)
	}
}

func TestTranslateKeys(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want Key
	}{
		{tcell.NewEventKey(tcell.KeyRune, 'x', 0), R('x')},
		{tcell.NewEventKey(tcell.KeyEnter, 0, 0), K(KeyEnter)},
		{tcell.NewEventKey(tcell.KeyEscape, 0, 0), K(KeyEscape)},
		{tcell.NewEventKey(tcell.KeyBackspace2, 0, 0), K(KeyBackspace)},
		{tcell.NewEventKey(tcell.KeyPgDn, 0, 0), K(KeyPgDn)},
		{tcell.NewEventKey(tcell.KeyCtrlC, 0, 0), K(KeyCtrlC)},
		{tcell.NewEventKey(tcell.KeyCtrlL, 0, 0), K(KeyCtrlL)},
		{tcell.NewEventKey(tcell.KeyCtrlT, 0, 0), K(KeyCtrlT)},
	}
	for _, c := range cases {
		got, ok := translate(c.ev, false)
		if !ok || got != c.want {
			t.Errorf("translate(%v) = %+v,%v want %+v", c.ev.Key(), got, ok, c.want)
		}
	}
	if _, ok := translate(tcell.NewEventKey(tcell.KeyF1, 0, 0), false); ok {
		t.Error("F1 should be ignored")
	}
	if k, _ := translate(tcell.NewEventKey(tcell.KeyRune, 'p', 0), true); !k.Paste {
		t.Error("paste flag not set")
	}
	if k, _ := translate(tcell.NewEventKey(tcell.KeyEnter, 0, 0), true); !k.Paste {
		t.Error("paste flag must be kept on special keys too")
	}
}

func TestGridClipsAtEdges(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(6, 2)
	g := grid{s}
	if n := g.Text(4, 0, "abcdef", Style{}, 10); n != 2 {
		t.Errorf("Text at the edge used %d cells, want 2", n)
	}
	if n := g.Put(5, 0, "日", Style{}); n != 0 {
		t.Errorf("wide cluster in the last column should not draw, got %d", n)
	}
	if n := g.Text(0, 5, "x", Style{}, 10); n != 0 {
		t.Errorf("off-screen row drew %d", n)
	}
}

func TestGridPutOnSimulationScreen(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(10, 2)
	g := grid{s}
	if n := g.Put(1, 0, "é", Style{FG: Green}); n != 1 {
		t.Errorf("width %d", n)
	}
	mainc, combc, st, w := s.GetContent(1, 0)
	if mainc != 'e' || len(combc) != 1 || combc[0] != 0x301 || w != 1 {
		t.Errorf("content = %q %v %d", mainc, combc, w)
	}
	if fg, _, _ := st.Decompose(); fg != tcell.PaletteColor(2) {
		t.Errorf("fg = %v", fg)
	}
}
