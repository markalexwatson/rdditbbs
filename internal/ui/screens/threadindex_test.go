package screens

import (
	"strings"
	"testing"

	"github.com/markalexwatson/redditbbs/internal/reddit"
	"github.com/markalexwatson/redditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/theme"
	"github.com/markalexwatson/redditbbs/internal/ui"
)

func threadIndexUp(t *testing.T) (*ui.App, *term.Sim, *Deps, *redditest.FakeStore, *ThreadIndex) {
	t.Helper()
	d, fs := newDeps(t)
	th := redditest.SampleThread()
	fs.Threads["aaa"] = th
	ti := NewThreadIndex(d, th.Post)
	app, sim := run(t, ti)
	pump(t, app)
	return app, sim, d, fs, ti
}

func TestThreadIndexLoadsAndDraws(t *testing.T) {
	_, sim, _, _, ti := threadIndexUp(t)
	mustContain(t, sim,
		"Kernel 7.2 released", "torvaldsfan", "342 comments",
		"sched_nerd", "├─torvaldsfan", "Agreed.",
		"[load 3 more replies]", "[continue this thread]", "[load 40 more replies]",
		"342 msgs", "BEST",
		"The EEVDF changes are the headline.", // peek pane body of the cursor comment
	)
	rowY := 3 + 2 + 1 + 1 // title bar + header + rule + table heading
	if _, st := sim.CellAt(3, rowY); st != theme.Style(theme.Cursor) {
		t.Errorf("first comment row should be highlighted, got %+v", st)
	}
	if ti.table.Height() < 5 {
		t.Errorf("table height = %d", ti.table.Height())
	}
}

func TestThreadIndexDeletedAuthorShowsBody(t *testing.T) {
	_, sim, _, _, _ := threadIndexUp(t)
	mustContain(t, sim, "[deleted]", "Body survives the account.", "[removed]")
}

func TestThreadIndexCollapseAndExpand(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	press(app, term.R('-'))
	mustContain(t, sim, "[+1 hidden]")
	mustNotContain(t, sim, "Agreed.")
	press(app, term.R('+'))
	mustContain(t, sim, "Agreed.")
	mustNotContain(t, sim, "hidden]")
}

func TestThreadIndexPeekToggle(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	mustContain(t, sim, "Lazy preemption is the real win.")
	press(app, term.K(term.KeyTab))
	mustNotContain(t, sim, "Lazy preemption is the real win.")
	press(app, term.K(term.KeyTab))
	mustContain(t, sim, "Lazy preemption is the real win.")
}

func TestThreadIndexPeekFollowsCursor(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	press(app, term.K(term.KeyDown))
	// Peek shows c2 (Agreed.) and the index still shows c1's preview.
	if strings.Count(sim.String(), "Agreed.") < 2 {
		t.Errorf("peek should show the selected comment's body:\n%s", sim.String())
	}
	press(app, term.K(term.KeyDown)) // stub row
	mustContain(t, sim, "Press Enter to load")
}

func TestThreadIndexLoadMore(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.More = reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Author: "xorg4life", Body: "X11 forever"}}}
	press(app, term.R('3'), term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "xorg4life", "X11 forever", "[load 2 more replies]")
	last := fs.Calls[len(fs.Calls)-1]
	if last != "more:t3_aaa/3" {
		t.Errorf("last call = %s", last)
	}
}

func TestThreadIndexContinueThread(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.Subtrees["c4"] = reddit.Thread{Post: redditest.SamplePost("aaa", "x"), Comments: []*reddit.Comment{{
		ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Author: "modbot", Body: "[removed]",
		Children: []*reddit.Comment{{ID: "k1", Fullname: "t1_k1", ParentFullname: "t1_c4", Author: "deepdiver", Body: "found it"}},
	}}}
	press(app, term.R('6'), term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "deepdiver", "found it")
	mustNotContain(t, sim, "[continue this thread]")
}

func TestThreadIndexOpenReaderAndReselect(t *testing.T) {
	app, _, _, _, ti := threadIndexUp(t)
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*ThreadIndex); ok {
		t.Fatal("Enter on a comment should open the reader")
	}
	ti.Update(ui.PopResult{Result: SelectComment{ID: "c3"}})
	if ti.table.Cursor != 3 {
		t.Errorf("cursor = %d, want row of c3", ti.table.Cursor)
	}
}

func TestThreadIndexSortRefetchAndInfo(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	press(app, term.R('s'))
	pump(t, app)
	mustContain(t, sim, "TOP")
	if last := fs.Calls[len(fs.Calls)-1]; !strings.HasPrefix(last, "thread:linux/aaa/top") {
		t.Errorf("last call = %s", last)
	}
}

func TestThreadIndexErrorKeepsRows(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.Err = &reddit.APIError{Status: 500}
	press(app, term.R('s'))
	mustContain(t, sim, "TOP")
	pump(t, app)
	mustContain(t, sim, "sched_nerd", "Reddit is having trouble", "BEST")
	mustNotContain(t, sim, "TOP")
}

func TestThreadIndexRefreshUpdatesPost(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	th := redditest.SampleThread()
	th.Post.Title = "Kernel 7.2 released (updated)"
	th.Post.NumComments = 400
	fs.Threads["aaa"] = th
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "(updated)", "400 msgs")
}

func TestThreadIndexResizeRebuildsConnectors(t *testing.T) {
	d, fs := newDeps(t)
	th := redditest.SampleThread()
	fs.Threads["aaa"] = th
	ti := NewThreadIndex(d, th.Post)
	sim := term.NewSim(80, 24)
	app := ui.New(sim, ti, ui.WithMinSize(20, 10))
	app.Draw()
	pump(t, app)
	sim.Resize(24, 24) // cap becomes 3
	app.Handle(<-sim.Events())
	app.Draw()
	if ti.cap != 3 {
		t.Errorf("cap = %d", ti.cap)
	}
}

func TestThreadIndexSelfPostPreviewAndBody(t *testing.T) {
	d, fs := newDeps(t)
	th := redditest.SampleThread()
	th.Post.IsSelf = true
	th.Post.SelfText = "Ask **anything** here.\n\nSecond paragraph."
	fs.Threads["aaa"] = th
	app, sim := run(t, NewThreadIndex(d, th.Post))
	pump(t, app)
	mustContain(t, sim, "Ask anything here.")
	press(app, term.R('b'))
	if _, ok := app.Top().(*ThreadIndex); ok {
		t.Error("B should open the post body in the reader")
	}
}

func TestThreadIndexOpenLinkAndQuit(t *testing.T) {
	app, _, d, _, _ := threadIndexUp(t)
	var opened string
	d.Open = func(u string, _ func(error)) error { opened = u; return nil }
	press(app, term.R('o'))
	if opened != "https://example.com/aaa" {
		t.Errorf("opened %q", opened)
	}
	press(app, term.R('q'))
	if !app.Quitting() {
		t.Error("Q on the only screen pops to quit")
	}
}

func TestThreadIndexPartialMoreResultsAttached(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.More = reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Author: "xorg4life", Body: "X11 forever"}}}
	fs.MoreErr = &reddit.APIError{Status: 500}
	press(app, term.R('3'), term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "xorg4life", "Reddit is having trouble", "[load 2 more replies]")
}
