package rss

import (
	"os"
	"strings"
	"testing"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseListing(t *testing.T) {
	l, err := ParseListing(open(t, "listing.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Posts) != 5 {
		t.Fatalf("posts = %d", len(l.Posts))
	}
	if l.After != "t3_1wrmkrs" || l.After != l.Posts[4].Fullname {
		t.Errorf("After should be the last entry's fullname, got %q", l.After)
	}
	p := l.Posts[0]
	if p.ID != "1wron16" || p.Fullname != "t3_1wron16" || p.Subreddit != "linux" || p.Title != "HPR - Human Pattern Recorder" {
		t.Errorf("post 0 = %+v", p)
	}
	if p.Author != "Plexescor" {
		t.Errorf("author should be bare, got %q", p.Author)
	}
	// An image post with a text body: keep the body, keep the image URL.
	if p.IsSelf || !strings.HasPrefix(p.URL, "https://i.redd.it/") || !strings.Contains(p.SelfText, "**works on wayland**") || !strings.Contains(p.SelfText, "- Hyprland") {
		t.Errorf("image post with caption: IsSelf=%v url=%q text=%q", p.IsSelf, p.URL, p.SelfText)
	}
	if strings.Contains(p.SelfText, "submitted by") || strings.Contains(p.SelfText, "[comments]") {
		t.Errorf("feed boilerplate leaked into the body: %q", p.SelfText)
	}
	if p.Permalink != "/r/linux/comments/1wron16/hpr_human_pattern_recorder/" || p.Domain != "i.redd.it" {
		t.Errorf("permalink=%q domain=%q", p.Permalink, p.Domain)
	}
	if s := l.Posts[2]; !s.IsSelf || s.Domain != "self.linux" || !strings.Contains(s.SelfText, "30th anniversary") {
		t.Errorf("self post = self %v domain %q text %q", s.IsSelf, s.Domain, s.SelfText[:40])
	}
	if p.StatsKnown || p.Score != 0 || p.NumComments != 0 {
		t.Errorf("RSS has no stats: %+v", p)
	}
	if p.Created.IsZero() || p.Created.Year() != 2026 {
		t.Errorf("created = %v", p.Created)
	}
	q := l.Posts[1] // a link post to another subreddit
	if q.IsSelf || q.URL != "https://www.reddit.com/r/PredictionsMarkets/comments/1wrnusj/kalshi_weather_bets_at_the_linux_cli_10_models/" || q.Domain != "www.reddit.com" {
		t.Errorf("link post = url %q domain %q self %v", q.URL, q.Domain, q.IsSelf)
	}
	if q.SelfText != "" {
		t.Errorf("link post should have no body, got %q", q.SelfText)
	}
}

func TestParseThread(t *testing.T) {
	th, err := ParseThread(open(t, "thread.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Post == nil || th.Post.ID != "1wr4fmd" || th.Post.Author != "LinuxMonarch" || th.Post.Domain != "i.redd.it" {
		t.Fatalf("post = %+v", th.Post)
	}
	if !strings.Contains(th.Post.SelfText, "Richard Stallman announced the GNU Project") {
		t.Errorf("post body = %q", th.Post.SelfText)
	}
	if len(th.Comments) != 5 {
		t.Fatalf("comments = %d", len(th.Comments))
	}
	c := th.Comments[0]
	if c.ID != "pc9r71o" || c.Fullname != "t1_pc9r71o" || c.Author != "CptSpeedydash" || c.Depth != 0 || c.ParentFullname != "t3_1wr4fmd" {
		t.Errorf("comment 0 = %+v", c)
	}
	if !strings.HasPrefix(c.Body, "And how 15 years later") || c.StatsKnown {
		t.Errorf("comment body = %q stats %v", c.Body, c.StatsKnown)
	}
	if th.Comments[1].Body != "Which law? I'm out of the loop" {
		t.Errorf("entities not decoded: %q", th.Comments[1].Body)
	}
	if th.More != nil {
		t.Error("RSS threads have no stubs")
	}
}

func TestParseRejectsNonFeed(t *testing.T) {
	if _, err := ParseListing(strings.NewReader("<html>nope</html>")); err == nil {
		t.Error("html should be rejected")
	}
	if _, err := ParseThread(strings.NewReader(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`)); err == nil {
		t.Error("a thread feed without a post entry should be rejected")
	}
}
