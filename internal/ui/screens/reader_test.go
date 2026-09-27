package screens

import (
	"strings"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/ui"
	"github.com/markalexwatson/rdditbbs/internal/ui/threadmodel"
)

func readerAt(t *testing.T, id string) (*ui.App, *term.Sim, *Deps, *Reader) {
	t.Helper()
	d, _ := newDeps(t)
	m := threadmodel.New(redditest.SampleThread(), 10)
	r := NewReader(d, m, id)
	app, sim := run(t, r)
	return app, sim, d, r
}

func TestReaderShowsComment(t *testing.T) {
	_, sim, d, _ := readerAt(t, "c1")
	mustContain(t, sim,
		"Subj: Kernel 7.2 released", "From: sched_nerd", "(+412)", "Date: 26/09/26",
		"Re: original post", "depth 0", "1 loaded replies",
		"The EEVDF changes are the headline.", "Lazy preemption is the real win.",
		"Msg 1 of 7", "Read Message",
	)
	if d.Session.MessagesRead != 1 {
		t.Errorf("MessagesRead = %d", d.Session.MessagesRead)
	}
}

func TestReaderNextPrevAndBounds(t *testing.T) {
	app, sim, d, _ := readerAt(t, "c1")
	press(app, term.R('n'))
	mustContain(t, sim, "Agreed.", "Re: #1 sched_nerd", "depth 1", "Msg 2 of 7")
	press(app, term.R('n'))
	mustContain(t, sim, "Body survives the account.", "From: [deleted]", "Msg 4 of 7") // row 3 is a stub
	press(app, term.R('n'))
	mustContain(t, sim, "[removed]", "From: modbot", "Msg 5 of 7")
	press(app, term.R('n'))
	mustContain(t, sim, "No more messages", "Msg 5 of 7")
	press(app, term.R('p'), term.R('p'), term.R('p'), term.R('p'))
	mustContain(t, sim, "Msg 0 of 7", "From: torvaldsfan", "https://example.com/aaa")
	press(app, term.R('p'))
	mustContain(t, sim, "No more messages")
	if d.Session.MessagesRead != 8 {
		t.Errorf("MessagesRead = %d, want 8 (1 initial + 7 moves)", d.Session.MessagesRead)
	}
}

func TestReaderUp(t *testing.T) {
	app, sim, _, _ := readerAt(t, "c2")
	press(app, term.R('u'))
	mustContain(t, sim, "Msg 1 of 7", "From: sched_nerd")
	press(app, term.R('u'))
	mustContain(t, sim, "Msg 0 of 7")
	press(app, term.R('u'))
	mustContain(t, sim, "Already at top")
}

func TestReaderTAndQReturnSelection(t *testing.T) {
	_, _, _, r := readerAt(t, "c2")
	if act := r.HandleKey(term.R('t')); act != (ui.Pop{Result: SelectComment{ID: "c2"}}) {
		t.Errorf("T action = %#v", act)
	}
	if act := r.HandleKey(term.K(term.KeyEscape)); act != (ui.Pop{Result: SelectComment{ID: "c2"}}) {
		t.Errorf("Esc action = %#v", act)
	}
	_, _, _, r0 := readerAt(t, "")
	if act := r0.HandleKey(term.R('q')); act != (ui.Pop{}) {
		t.Errorf("Q on message 0 = %#v", act)
	}
}

func TestReaderRepliesJump(t *testing.T) {
	app, sim, _, _ := readerAt(t, "c1")
	press(app, term.R('r'))
	mustContain(t, sim, "Replies to this message", "torvaldsfan", "Reply #:")
	press(app, term.R('?')) // literal during numeric entry mode: must not open help
	if app.Depth() != 1 {
		t.Fatal("help opened during reply selection")
	}
	press(app, term.R('1'), term.K(term.KeyEnter))
	mustContain(t, sim, "Agreed.", "Msg 2 of 7")
	mustNotContain(t, sim, "Reply #:")
}

func TestReaderRepliesFromPostListsTopLevel(t *testing.T) {
	app, sim, _, _ := readerAt(t, "")
	press(app, term.R('r'))
	mustContain(t, sim, "1. sched_nerd", "2. [deleted]", "3. modbot")
	press(app, term.K(term.KeyEscape))
	mustNotContain(t, sim, "Replies to this message")
	if app.Depth() != 1 {
		t.Error("Escape must only cancel the reply list")
	}
}

func TestReaderLinks(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	th.Comments[0].Body = "see [one](https://one.example) and [two](https://two.example/x)"
	th.Comments[0].Children[0].Body = "just https://bare.example" // c2 is the next message after c1
	m := threadmodel.New(th, 10)
	var opened []string
	d.Open = func(u string, _ func(error)) error { opened = append(opened, u); return nil }
	app, sim := run(t, NewReader(d, m, "c1"))
	mustContain(t, sim, "one[1]", "two[2]", "Links:", "[1] https://one.example", "[2] https://two.example/x")
	press(app, term.R('o'))
	mustContain(t, sim, "Link #:")
	press(app, term.R('2'), term.K(term.KeyEnter))
	if len(opened) != 1 || opened[0] != "https://two.example/x" {
		t.Errorf("opened = %v", opened)
	}
	press(app, term.R('n'), term.R('o'))
	if len(opened) != 2 || opened[1] != "https://bare.example" {
		t.Errorf("single link should open directly: %v", opened)
	}
	press(app, term.R('n'), term.R('o'))
	mustContain(t, sim, "No links in this message")
	if d.Session.LinksOpened != 2 {
		t.Errorf("LinksOpened = %d", d.Session.LinksOpened)
	}
}

func TestReaderPaging(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		b.WriteString("line ")
		b.WriteString(itoa(i))
		b.WriteString("\n\n")
	}
	th.Comments[0].Body = b.String()
	app, sim := run(t, NewReader(d, threadmodel.New(th, 10), "c1"))
	mustContain(t, sim, "line 1", "more")
	mustNotContain(t, sim, "line 40")
	press(app, term.R(' '))
	mustNotContain(t, sim, "line 1\n")
	press(app, term.K(term.KeyEnd))
	mustContain(t, sim, "line 60")
	mustNotContain(t, sim, "▼ more")
	press(app, term.K(term.KeyHome))
	mustContain(t, sim, "line 1")
}

func TestReaderRepliesIncludeCollapsedAndReveal(t *testing.T) {
	d, _ := newDeps(t)
	m := threadmodel.New(redditest.SampleThread(), 10)
	m.Toggle("c1")
	app, sim := run(t, NewReader(d, m, "c1"))
	press(app, term.R('r'))
	mustContain(t, sim, "1. torvaldsfan")
	press(app, term.R('1'), term.K(term.KeyEnter))
	mustContain(t, sim, "Agreed.", "Msg 2 of 7")
	if m.IsCollapsed("c1") {
		t.Error("jumping to a hidden reply should reveal it")
	}
}

func TestReaderNegativeScore(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	th.Comments[0].Score = -7
	_, sim := run(t, NewReader(d, threadmodel.New(th, 10), "c1"))
	mustContain(t, sim, "(-7)")
	mustNotContain(t, sim, "(+-7)")
}

func TestReaderIgnoresPostMoreStubForReplies(t *testing.T) {
	d, _ := newDeps(t)
	th := reddit.Thread{Post: redditest.SamplePost("z", "Empty"), More: &reddit.MoreStub{ParentFullname: "t3_z", Count: 3, IDs: []string{"a"}}}
	app, sim := run(t, NewReader(d, threadmodel.New(th, 10), ""))
	mustContain(t, sim, "Msg 0 of 1") // the lone stub is row 1
	press(app, term.R('n'))
	mustContain(t, sim, "No more messages")
	press(app, term.R('r'))
	mustContain(t, sim, "No loaded replies")
}

func TestReaderUnknownScoreAndLinkPostWithCaption(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	th.Post.IsSelf = false
	th.Post.SelfText = "Caption text under the image."
	th.Post.StatsKnown = false
	th.Comments[0].StatsKnown = false
	m := threadmodel.New(th, 10)
	app, sim := run(t, NewReader(d, m, ""))
	mustContain(t, sim, "Link: https://example.com/aaa", "Caption text under the image.")
	mustNotContain(t, sim, "(+")
	press(app, term.R('n'))
	mustContain(t, sim, "From: sched_nerd")
	mustNotContain(t, sim, "(+412)")
}
