package screens

import (
	"context"
	"strings"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/config"
	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui"
)

var linuxArea = config.Area{Name: "Linux", Subreddit: "linux"}

func postListWith(t *testing.T, n int) (*ui.App, *term.Sim, *Deps, *redditest.FakeStore) {
	t.Helper()
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(n, "")
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	return app, sim, d, fs
}

func TestPostListLoadsAndDraws(t *testing.T) {
	_, sim, d, _ := postListWith(t, 3)
	mustContain(t, sim, "Post 1", "Post 2", "Post 3", "author_p1", "r/linux · HOT · Page 1", "Subject", "From", "Msgs")
	if _, st := sim.CellAt(4, 5); st != theme.Style(theme.Cursor) {
		t.Errorf("first data row should be the cursor row, got %+v", st)
	}
	if d.Session.AreasVisited() != 1 {
		t.Error("visiting an area should be recorded")
	}
}

func TestPostListEmpty(t *testing.T) {
	app, sim, _, _ := postListWith(t, 0)
	mustContain(t, sim, "No messages")
	press(app, term.K(term.KeyEnter), term.R('1'), term.K(term.KeyEnter), term.K(term.KeyDown))
	if _, ok := app.Top().(*PostList); !ok || app.Depth() != 1 {
		t.Error("Enter on an empty list must do nothing")
	}
}

func TestPostListNumberOpens(t *testing.T) {
	app, _, d, _ := postListWith(t, 3)
	press(app, term.R('2'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*PostList); ok {
		t.Error("number selection should open the thread")
	}
	if d.Session.ThreadsOpened != 1 {
		t.Error("ThreadsOpened not counted")
	}
}

func TestPostListBadNumber(t *testing.T) {
	app, sim, _, _ := postListWith(t, 3)
	press(app, term.R('9'), term.K(term.KeyEnter))
	mustContain(t, sim, "No such message")
	if _, ok := app.Top().(*PostList); !ok {
		t.Error("bad number must stay")
	}
}

func TestPostListPagination(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(20, "t3_p20")
	page2 := reddit.Listing{}
	for i := 21; i <= 25; i++ {
		page2.Posts = append(page2.Posts, redditest.SamplePost("p"+itoa(i), "Post "+itoa(i)))
	}
	fs.Listings["linux/hot/t3_p20"] = page2
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	// 80x24: content 19 rows, heading 2, so 17 data rows per page.
	mustContain(t, sim, "Post 1", "Post 17")
	mustNotContain(t, sim, "Post 18")
	press(app, term.R('n'))
	mustContain(t, sim, "Post 18", "Post 20", "Page 2")
	press(app, term.R('n')) // beyond loaded posts: fetches the next Reddit page
	pump(t, app)
	mustContain(t, sim, "Post 21", "Post 25", "Page 2")
	press(app, term.R('n'))
	mustContain(t, sim, "End of messages")
	press(app, term.R('p'))
	mustContain(t, sim, "Post 1", "Page 1")
	if fs.CallCount() != 2 {
		t.Errorf("store calls = %d, want 2", fs.CallCount())
	}
}

func TestPostListPgDnCompletesAfterFetch(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(20, "t3_p20")
	page2 := reddit.Listing{}
	for i := 21; i <= 40; i++ {
		page2.Posts = append(page2.Posts, redditest.SamplePost("p"+itoa(i), "Post "+itoa(i)))
	}
	fs.Listings["linux/hot/t3_p20"] = page2
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	press(app, term.K(term.KeyEnd))  // cursor 19 (Post 20)
	press(app, term.K(term.KeyPgDn)) // wants 36: fetch, owed 17
	pump(t, app)
	pl := app.Top().(*PostList)
	if pl.cursor != 36 {
		t.Errorf("cursor after PgDn+fetch = %d, want 36", pl.cursor)
	}
	mustContain(t, sim, "Post 37", "Page 3")
}

func TestPostListDownAtEndFetchesMore(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(2, "t3_p2")
	fs.Listings["linux/hot/t3_p2"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("p3", "Post 3")}}
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	press(app, term.K(term.KeyDown), term.K(term.KeyDown))
	pump(t, app)
	press(app, term.K(term.KeyDown))
	if _, st := sim.CellAt(4, 7); st != theme.Style(theme.Cursor) {
		t.Error("cursor should reach the newly loaded third row")
	}
	press(app, term.K(term.KeyDown)) // ended: no further fetch
	if fs.CallCount() != 2 {
		t.Errorf("calls = %d", fs.CallCount())
	}
}

func TestPostListSortChangeKeepsOldUntilArrival(t *testing.T) {
	app, sim, _, fs := postListWith(t, 2)
	fs.Listings["linux/new/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("n1", "Fresh 1")}}
	fs.Block = make(chan struct{})
	press(app, term.R('s'))
	mustContain(t, sim, "Post 1", "Retrieving...", "NEW")
	close(fs.Block)
	pump(t, app)
	mustContain(t, sim, "Fresh 1")
	mustNotContain(t, sim, "Post 1")
}

func TestPostListErrorKeepsContent(t *testing.T) {
	app, sim, _, fs := postListWith(t, 2)
	fs.Err = &reddit.APIError{Status: 503}
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "Post 1", "Reddit is having trouble")
	last := fs.Calls[len(fs.Calls)-1]
	if !strings.Contains(last, "fresh=true") {
		t.Errorf("refresh should bypass cache: %s", last)
	}
}

func TestPostListResizeClampsCursor(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(20, "")
	sim := term.NewSim(80, 24)
	app := ui.New(sim, NewPostList(d, linuxArea, true), ui.WithMinSize(40, 10)) // allow the small size below
	app.Draw()
	pump(t, app)
	press(app, term.K(term.KeyEnd))
	mustContain(t, sim, "Post 20", "Page 2")
	sim.Resize(80, 12) // content 7 rows, 5 data rows per page
	app.Handle(<-sim.Events())
	app.Draw()
	mustContain(t, sim, "Post 20", "Page 4")
	press(app, term.K(term.KeyUp))
	mustContain(t, sim, "Post 19")
}

func TestPostListJoinReplaces(t *testing.T) {
	app, sim, _, fs := postListWith(t, 1)
	fs.Listings["rust/hot/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("r1", "Rusty")}}
	press(app, term.R('j'))
	typeString(app, "rust")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	pl, ok := app.Top().(*PostList)
	if !ok || pl.area.Subreddit != "rust" || app.Depth() != 1 {
		t.Fatalf("top = %T depth %d", app.Top(), app.Depth())
	}
	mustContain(t, sim, "Rusty", "r/rust")
}

func TestPostListAddArea(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["golang/hot/"] = redditest.SampleListing(1, "")
	app, sim := run(t, NewPostList(d, config.Area{Name: "r/golang", Subreddit: "golang"}, false))
	pump(t, app)
	mustContain(t, sim, "[A]dd area")
	press(app, term.R('a'))
	if !d.Config.HasArea("golang") {
		t.Error("area not added")
	}
	mustContain(t, sim, "Area saved")
	mustNotContain(t, sim, "[A]dd area")
}

func TestPostListOpenLink(t *testing.T) {
	app, _, d, _ := postListWith(t, 1)
	var opened string
	d.Open = func(u string, _ func(error)) error { opened = u; return nil }
	press(app, term.R('o'))
	if opened != "https://example.com/p1" || d.Session.LinksOpened != 1 {
		t.Errorf("opened=%q links=%d", opened, d.Session.LinksOpened)
	}
}

func TestPostListNumberOutsidePage(t *testing.T) {
	app, sim, _, _ := postListWith(t, 20)
	press(app, term.R('0'), term.K(term.KeyEnter))
	mustContain(t, sim, "No such message")
	press(app, term.R('1'), term.R('8'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*PostList); !ok {
		t.Error("18 is not on a 17-row page and must not open the next page's post")
	}
	press(app, term.R('n'), term.R('4'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*PostList); !ok {
		t.Error("page 2 has three posts; 4 must be rejected")
	}
}

func TestPostListRefreshKeepsSelection(t *testing.T) {
	app, sim, _, fs := postListWith(t, 5)
	press(app, term.K(term.KeyDown), term.K(term.KeyDown))
	fs.Listings["linux/hot/"] = redditest.SampleListing(6, "")
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "Post 6")
	if _, st := sim.CellAt(4, 7); st != theme.Style(theme.Cursor) {
		t.Error("refresh should keep the cursor on Post 3")
	}
}

func TestPostListSortFailureRestoresSort(t *testing.T) {
	app, sim, _, fs := postListWith(t, 2)
	fs.Err = &reddit.APIError{Status: 500}
	press(app, term.R('s'))
	mustContain(t, sim, "NEW")
	pump(t, app)
	mustContain(t, sim, "HOT", "Post 1")
	mustNotContain(t, sim, "NEW")
}

func TestPostListAuthFailureOffersLogin(t *testing.T) {
	app, sim, d, fs := postListWith(t, 2)
	fs.Err = &reddit.APIError{Status: 401}
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "Credentials rejected", "[L]og in")
	press(app, term.R('l'))
	if _, ok := app.Top().(*Setup); !ok {
		t.Fatalf("L should push setup, got %T", app.Top())
	}
	fs.Err = nil
	d.MakeStore = func(id, secret string) reddit.Store { return fs }
	typeString(app, "id2")
	press(app, term.K(term.KeyTab))
	typeString(app, "sec2")
	press(app, term.K(term.KeyEnter))
	pump(t, app) // setup verifies and pops
	if _, ok := app.Top().(*PostList); !ok {
		t.Fatalf("expected PostList after setup, got %T", app.Top())
	}
	pump(t, app) // post list refetches on return
	mustContain(t, sim, "Post 1")
	mustNotContain(t, sim, "Credentials rejected")
}

func TestPostListStaleResultIgnored(t *testing.T) {
	app, sim, _, fs := postListWith(t, 1)
	fs.Listings["linux/new/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("n1", "New 1")}}
	fs.Listings["linux/top/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("t1", "Top 1")}}
	press(app, term.R('s'), term.R('s')) // hot -> new -> top; the "new" result is stale
	pump(t, app)
	pump(t, app)
	mustContain(t, sim, "Top 1")
	mustNotContain(t, sim, "New 1")
}

func TestPostListNextAcrossRedditPageShowsNewPosts(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(100, "t3_p100")
	page2 := reddit.Listing{}
	for i := 101; i <= 200; i++ {
		page2.Posts = append(page2.Posts, redditest.SamplePost("p"+itoa(i), "Post "+itoa(i)))
	}
	fs.Listings["linux/hot/t3_p100"] = page2
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	for i := 0; i < 5; i++ {
		press(app, term.R('n'))
	}
	mustContain(t, sim, "Post 86", "Post 100", "Page 6")
	press(app, term.R('n')) // fetches the next Reddit page
	pump(t, app)
	mustContain(t, sim, "Post 101", "Post 102", "Page 6")
	if pl := app.Top().(*PostList); pl.cursor != 100 {
		t.Errorf("cursor should land on the first newly loaded post (index 100), got %d", pl.cursor)
	}
	press(app, term.R('n'))
	mustContain(t, sim, "Post 103", "Page 7")
}

func TestPostListCursorCycleEnds(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(2, "A")
	fs.Listings["linux/hot/A"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("p3", "Post 3")}, After: "B"}
	fs.Listings["linux/hot/B"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("p1", "Post 1")}, After: "A"} // Reddit loops back
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	press(app, term.K(term.KeyEnd), term.K(term.KeyDown))
	pump(t, app)
	press(app, term.K(term.KeyEnd), term.K(term.KeyDown))
	pump(t, app)
	press(app, term.K(term.KeyDown))
	mustContain(t, sim, "End of messages")
	if fs.CallCount() != 3 {
		t.Errorf("a repeated cursor must end paging; calls = %d", fs.CallCount())
	}
}

func TestPostListUnknownStatsShowDashes(t *testing.T) {
	d, fs := newDeps(t)
	d.Source = "rss"
	l := redditest.SampleListing(2, "")
	l.Posts[0].StatsKnown = false
	fs.Listings["linux/hot/"] = l
	sim := term.NewSim(100, 24)
	app := ui.New(sim, NewPostList(d, linuxArea, true))
	app.Draw()
	pump(t, app)
	row := sim.Row(5)
	if !strings.Contains(row, "Post 1") || !strings.Contains(row, "–") {
		t.Errorf("unknown msgs and score should show dashes: %q", row)
	}
	if !strings.Contains(sim.Row(6), "10") {
		t.Errorf("known stats still shown: %q", sim.Row(6))
	}
	mustContain(t, sim, "· RSS")
}

type prefetchStore struct {
	*redditest.FakeStore
	prefetched []string
}

func (p *prefetchStore) Prefetch(_ context.Context, sub, id string) bool {
	p.prefetched = append(p.prefetched, sub+"/"+id)
	return true
}

func TestPostListPrefetchesNextThread(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(3, "")
	fs.Threads["p1"] = redditest.SampleThread()
	ps := &prefetchStore{FakeStore: fs}
	d.Store = ps
	app, _ := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	press(app, term.K(term.KeyEnter))
	if len(ps.prefetched) != 0 {
		t.Errorf("the selected thread must load before anything is prefetched, got %v", ps.prefetched)
	}
	pump(t, app) // the thread arrives
	if len(ps.prefetched) != 1 || ps.prefetched[0] != "linux/p2" {
		t.Errorf("after post 1 loads, post 2's thread should be prefetched, got %v", ps.prefetched)
	}
}
