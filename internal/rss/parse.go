// Package rss reads Reddit's public Atom feeds as a fallback data source when
// no approved API credentials exist. Feeds carry titles, authors, bodies and
// links but no scores, comment counts or threading, and Reddit throttles them
// to roughly one request a minute per address.
package rss

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/htmltext"
	"github.com/markalexwatson/rdditbbs/internal/reddit"
)

type feed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []entry  `xml:"entry"`
}

type entry struct {
	ID        string   `xml:"id"`
	Title     string   `xml:"title"`
	Author    string   `xml:"author>name"`
	Published string   `xml:"published"`
	Updated   string   `xml:"updated"`
	Content   string   `xml:"content"`
	Links     []link   `xml:"link"`
	Category  category `xml:"category"`
}

type link struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type category struct {
	Term string `xml:"term,attr"`
}

var (
	bodyRe     = regexp.MustCompile(`(?s)<!-- SC_OFF -->(.*?)<!-- SC_ON -->`)
	linkHrefRe = regexp.MustCompile(`<a href="([^"]+)">\s*\[link\]\s*</a>`)
)

func decode(r io.Reader) (feed, error) {
	var f feed
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return feed{}, fmt.Errorf("decode feed: %w", err)
	}
	return f, nil
}

func (e entry) href() string {
	for _, l := range e.Links {
		if l.Rel == "" || l.Rel == "alternate" {
			return l.Href
		}
	}
	if len(e.Links) > 0 {
		return e.Links[0].Href
	}
	return ""
}

func (e entry) when() time.Time {
	for _, s := range []string{e.Published, e.Updated} {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func bareAuthor(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "/u/")
}

// body extracts the Markdown-ish text of the entry's md block, dropping the
// "submitted by / [link] / [comments]" trailer Reddit appends to posts.
func body(content string) string {
	if m := bodyRe.FindStringSubmatch(content); m != nil {
		return htmltext.ToMarkdown(m[1])
	}
	if strings.Contains(content, "submitted by") {
		return ""
	}
	return htmltext.ToMarkdown(content)
}

func permalinkPath(href string) string {
	if u, err := url.Parse(href); err == nil && u.Path != "" {
		return u.Path
	}
	return href
}

func (e entry) post() (*reddit.Post, error) {
	if !strings.HasPrefix(e.ID, "t3_") {
		return nil, fmt.Errorf("entry %q is not a post", e.ID)
	}
	permalink := e.href()
	p := &reddit.Post{
		ID:        strings.TrimPrefix(e.ID, "t3_"),
		Fullname:  e.ID,
		Subreddit: e.Category.Term,
		Title:     strings.TrimSpace(e.Title),
		Author:    bareAuthor(e.Author),
		Created:   e.when(),
		Permalink: permalinkPath(permalink),
		SelfText:  body(e.Content),
		URL:       permalink,
		IsSelf:    true,
	}
	if m := linkHrefRe.FindStringSubmatch(e.Content); m != nil {
		target := m[1]
		if strings.HasPrefix(target, "/") {
			target = "https://www.reddit.com" + target
		}
		if permalinkPath(target) != p.Permalink {
			// A link or image post; it may still carry a text body (an image caption).
			p.URL, p.IsSelf = target, false
		}
	}
	if p.IsSelf {
		p.Domain = "self." + p.Subreddit
	} else if u, err := url.Parse(p.URL); err == nil {
		p.Domain = u.Host
	}
	return p, nil
}

// ParseListing decodes a subreddit feed. After is the last entry's fullname,
// which Reddit accepts as the after cursor for the next page.
func ParseListing(r io.Reader) (reddit.Listing, error) {
	f, err := decode(r)
	if err != nil {
		return reddit.Listing{}, err
	}
	var l reddit.Listing
	for _, e := range f.Entries {
		if !strings.HasPrefix(e.ID, "t3_") {
			continue
		}
		p, err := e.post()
		if err != nil {
			return reddit.Listing{}, err
		}
		l.Posts = append(l.Posts, p)
	}
	if n := len(l.Posts); n > 0 {
		l.After = l.Posts[n-1].Fullname
	}
	return l, nil
}

// ParseThread decodes a post's comment feed: the first t3 entry is the post
// and every t1 entry is a top-level comment, since feeds carry no threading.
func ParseThread(r io.Reader) (reddit.Thread, error) {
	f, err := decode(r)
	if err != nil {
		return reddit.Thread{}, err
	}
	var th reddit.Thread
	for _, e := range f.Entries {
		switch {
		case strings.HasPrefix(e.ID, "t3_") && th.Post == nil:
			p, err := e.post()
			if err != nil {
				return reddit.Thread{}, err
			}
			th.Post = p
		case strings.HasPrefix(e.ID, "t1_"):
			parent := ""
			if th.Post != nil {
				parent = th.Post.Fullname
			}
			th.Comments = append(th.Comments, &reddit.Comment{
				ID:             strings.TrimPrefix(e.ID, "t1_"),
				Fullname:       e.ID,
				ParentFullname: parent,
				Author:         bareAuthor(e.Author),
				Body:           body(e.Content),
				Created:        e.when(),
			})
		}
	}
	if th.Post == nil {
		return reddit.Thread{}, errors.New("comment feed has no post entry")
	}
	return th, nil
}
