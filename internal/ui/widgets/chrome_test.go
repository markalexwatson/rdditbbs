package widgets

import (
	"strings"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/theme"
)

func TestTitleBar(t *testing.T) {
	sim := term.NewSim(60, 3)
	TitleBar(sim, "Message Areas", "r/linux 26/09/26")
	rows := strings.Split(sim.String(), "\n")
	if !strings.HasPrefix(rows[0], "╔═") || !strings.HasSuffix(rows[0], "═╗") {
		t.Errorf("top frame = %q", rows[0])
	}
	if !strings.Contains(rows[1], "R D D I T   B B S") || !strings.Contains(rows[1], "Message Areas") || !strings.HasSuffix(rows[1], "r/linux 26/09/26 ║") {
		t.Errorf("title row = %q", rows[1])
	}
	if !strings.HasPrefix(rows[2], "╚═") || !strings.HasSuffix(rows[2], "═╝") {
		t.Errorf("bottom frame = %q", rows[2])
	}
	if _, st := sim.CellAt(0, 0); st != theme.Style(theme.Frame) {
		t.Errorf("frame style = %+v", st)
	}
}

func TestHotkeyBarAndPrompt(t *testing.T) {
	sim := term.NewSim(40, 2)
	HotkeyBar(sim, 0, []KeyHelp{{"N", "ext"}, {"Q", "uit"}})
	if got := sim.Row(0); got != "  [N]ext [Q]uit" {
		t.Errorf("hotkeys = %q", got)
	}
	if _, st := sim.CellAt(3, 0); st != theme.Style(theme.Hotkey) {
		t.Errorf("hotkey style = %+v", st)
	}
	PromptLine(sim, 1, Prompt{Input: "12", Status: "Retrieving...", Cursor: true})
	if got := sim.Row(1); !strings.HasPrefix(got, "  Command: 12") || !strings.HasSuffix(got, "Retrieving...") || len(got) != 38 {
		t.Errorf("prompt = %q (status should be right-aligned two cells from the edge)", got)
	}
	if x, y, on := sim.Cursor(); !on || x != 13 || y != 1 {
		t.Errorf("cursor = %d,%d,%v", x, y, on)
	}
	PromptLine(sim, 1, Prompt{Label: "Join area:", Input: "linux", Cursor: true, CursorPos: 2})
	if x, _, _ := sim.Cursor(); x != 2+10+1+2 {
		t.Errorf("cursor with offset = %d, want 15", x)
	}
	PromptLine(sim, 1, Prompt{Label: "Log off? (y/N)", Status: "boom", Error: true})
	if !strings.HasPrefix(sim.Row(1), "  Log off? (y/N)") {
		t.Errorf("label prompt = %q", sim.Row(1))
	}
	if _, st := sim.CellAt(37, 1); st != theme.Style(theme.Error) {
		t.Errorf("error status style = %+v", st)
	}
}

func TestRuleAndCentre(t *testing.T) {
	sim := term.NewSim(11, 2)
	Rule(sim, 0)
	if sim.Row(0) != strings.Repeat("─", 11) {
		t.Errorf("rule = %q", sim.Row(0))
	}
	Centre(sim, 1, "hi", term.Style{})
	if sim.Row(1) != "    hi" {
		t.Errorf("centre = %q", sim.Row(1))
	}
}

func TestBarsUseThemeBarFill(t *testing.T) {
	t.Cleanup(func() { _ = theme.Set("classic") })
	if err := theme.Set("blue"); err != nil {
		t.Fatal(err)
	}
	sim := term.NewSim(60, 4)
	TitleBar(sim, "Message Areas", "r/linux")
	HotkeyBar(sim, 3, []KeyHelp{{Key: "N", Desc: "ext"}})
	bar := theme.Style(theme.Bar)
	if _, st := sim.CellAt(30, 1); st.BG != bar.BG {
		t.Errorf("title bar interior background = %+v, want %+v", st, bar)
	}
	if _, st := sim.CellAt(2, 1); st.BG != bar.BG || st.FG != theme.Style(theme.Logo).FG {
		t.Errorf("logo should sit on the bar: %+v", st)
	}
	if _, st := sim.CellAt(40, 3); st.BG != bar.BG {
		t.Errorf("hotkey bar background = %+v", st)
	}
	if _, st := sim.CellAt(3, 3); st.BG != bar.BG || st.FG != theme.Style(theme.Hotkey).FG {
		t.Errorf("hotkey letter should sit on the bar: %+v", st)
	}
}
