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

const atomHead = `<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"><category term="linux" label="r/linux"/>`

func entryXML(id, author, content, href string) string {
	return `<entry><author><name>/u/` + author + `</name></author><category term="linux" label="r/linux"/><content type="html">` + content +
		`</content><id>` + id + `</id><link href="` + href + `"/><updated>2026-09-27T10:00:00+00:00</updated><title>t</title></entry>`
}

func TestLinkTrailerIsNotTakenFromTheBody(t *testing.T) {
	body := `&lt;!-- SC_OFF --&gt;&lt;div class="md"&gt;&lt;p&gt;see &lt;a href="https://evil.example/x"&gt;[link]&lt;/a&gt; in my text&lt;/p&gt;&lt;/div&gt;&lt;!-- SC_ON --&gt; &amp;#32; submitted by &amp;#32; &lt;a href="https://www.reddit.com/user/a"&gt; /u/a &lt;/a&gt; &lt;br/&gt; &lt;span&gt;&lt;a href="https://www.reddit.com/r/linux/comments/abc/t/"&gt;[link]&lt;/a&gt;&lt;/span&gt; &amp;#32; &lt;span&gt;&lt;a href="https://www.reddit.com/r/linux/comments/abc/t/"&gt;[comments]&lt;/a&gt;&lt;/span&gt;`
	l, err := ParseListing(strings.NewReader(atomHead + entryXML("t3_abc", "a", body, "https://www.reddit.com/r/linux/comments/abc/t/") + "</feed>"))
	if err != nil {
		t.Fatal(err)
	}
	p := l.Posts[0]
	if !p.IsSelf || p.URL != "https://www.reddit.com/r/linux/comments/abc/t/" {
		t.Errorf("a [link] inside the body must not become the post destination: self=%v url=%q", p.IsSelf, p.URL)
	}
	if !strings.Contains(p.SelfText, "[[link]](https://evil.example/x)") && !strings.Contains(p.SelfText, "evil.example") {
		t.Errorf("body link should survive as body text: %q", p.SelfText)
	}
}

func TestLinkTrailerEntitiesDecoded(t *testing.T) {
	body := `&amp;#32; submitted by &amp;#32; &lt;a href="https://www.reddit.com/user/a"&gt; /u/a &lt;/a&gt; &lt;br/&gt; &lt;span&gt;&lt;a href="https://example.com/p?a=1&amp;amp;b=2"&gt;[link]&lt;/a&gt;&lt;/span&gt; &amp;#32; &lt;span&gt;&lt;a href="https://www.reddit.com/r/linux/comments/abc/t/"&gt;[comments]&lt;/a&gt;&lt;/span&gt;`
	l, err := ParseListing(strings.NewReader(atomHead + entryXML("t3_abc", "a", body, "https://www.reddit.com/r/linux/comments/abc/t/") + "</feed>"))
	if err != nil {
		t.Fatal(err)
	}
	if p := l.Posts[0]; p.IsSelf || p.URL != "https://example.com/p?a=1&b=2" || p.SelfText != "" {
		t.Errorf("link post = self %v url %q body %q", p.IsSelf, p.URL, p.SelfText)
	}
}

func TestCommentMentioningSubmittedByKept(t *testing.T) {
	body := `&lt;!-- SC_OFF --&gt;&lt;div class="md"&gt;&lt;p&gt;This patch was submitted by Alice.&lt;/p&gt;&lt;/div&gt;&lt;!-- SC_ON --&gt;`
	feed := atomHead + entryXML("t1_c1", "b", body, "https://www.reddit.com/r/linux/comments/abc/t/c1/") +
		entryXML("t3_abc", "a", `&lt;!-- SC_OFF --&gt;&lt;div class="md"&gt;&lt;p&gt;post&lt;/p&gt;&lt;/div&gt;&lt;!-- SC_ON --&gt;`, "https://www.reddit.com/r/linux/comments/abc/t/") + "</feed>"
	th, err := ParseThread(strings.NewReader(feed))
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 1 || th.Comments[0].Body != "This patch was submitted by Alice." {
		t.Errorf("comment = %+v", th.Comments)
	}
	if th.Comments[0].ParentFullname != "t3_abc" {
		t.Errorf("a comment listed before the post should still get the post as parent, got %q", th.Comments[0].ParentFullname)
	}
	if body := bodyOf(`<p>no markers, submitted by nobody</p>`); body != "no markers, submitted by nobody" {
		t.Errorf("plain content mentioning the phrase must be kept: %q", body)
	}
}
