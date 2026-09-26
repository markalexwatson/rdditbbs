package reddit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type thing struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type listingData struct {
	After    string  `json:"after"`
	Children []thing `json:"children"`
}

type postData struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Subreddit     string  `json:"subreddit"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	Score         int     `json:"score"`
	NumComments   int     `json:"num_comments"`
	CreatedUTC    float64 `json:"created_utc"`
	URL           string  `json:"url"`
	Domain        string  `json:"domain"`
	Permalink     string  `json:"permalink"`
	IsSelf        bool    `json:"is_self"`
	Selftext      string  `json:"selftext"`
	Stickied      bool    `json:"stickied"`
	Over18        bool    `json:"over_18"`
	Distinguished *string `json:"distinguished"`
}

type commentData struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ParentID      string          `json:"parent_id"`
	Author        string          `json:"author"`
	Body          string          `json:"body"`
	Score         int             `json:"score"`
	CreatedUTC    float64         `json:"created_utc"`
	IsSubmitter   bool            `json:"is_submitter"`
	Distinguished *string         `json:"distinguished"`
	Replies       json.RawMessage `json:"replies"`
}

type moreData struct {
	ParentID string   `json:"parent_id"`
	Count    int      `json:"count"`
	Children []string `json:"children"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func unixTime(f float64) time.Time { return time.Unix(int64(f), 0).UTC() }

// ParseListing decodes a subreddit listing.
func ParseListing(r io.Reader) (Listing, error) {
	var t thing
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return Listing{}, fmt.Errorf("decode listing: %w", err)
	}
	return parseListing(t)
}

func parseListing(t thing) (Listing, error) {
	if t.Kind != "Listing" {
		return Listing{}, fmt.Errorf("expected Listing, got %q", t.Kind)
	}
	var ld listingData
	if err := json.Unmarshal(t.Data, &ld); err != nil {
		return Listing{}, fmt.Errorf("decode listing data: %w", err)
	}
	l := Listing{After: ld.After}
	for _, ch := range ld.Children {
		if ch.Kind != "t3" {
			continue
		}
		p, err := parsePost(ch.Data)
		if err != nil {
			return Listing{}, err
		}
		l.Posts = append(l.Posts, p)
	}
	return l, nil
}

func parsePost(raw json.RawMessage) (*Post, error) {
	var d postData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode post: %w", err)
	}
	return &Post{
		ID: d.ID, Fullname: d.Name, Subreddit: d.Subreddit, Title: d.Title, Author: d.Author,
		Score: d.Score, NumComments: d.NumComments, Created: unixTime(d.CreatedUTC),
		URL: d.URL, Domain: d.Domain, Permalink: d.Permalink, IsSelf: d.IsSelf, SelfText: d.Selftext,
		Stickied: d.Stickied, Over18: d.Over18, Distinguished: str(d.Distinguished),
	}, nil
}

// ParseThread decodes the two-element array returned by the comments endpoint.
func ParseThread(r io.Reader) (Thread, error) {
	var arr []thing
	if err := json.NewDecoder(r).Decode(&arr); err != nil {
		return Thread{}, fmt.Errorf("decode thread: %w", err)
	}
	if len(arr) < 2 {
		return Thread{}, errors.New("thread response has fewer than two listings")
	}
	posts, err := parseListing(arr[0])
	if err != nil {
		return Thread{}, err
	}
	if len(posts.Posts) == 0 {
		return Thread{}, errors.New("thread response has no post")
	}
	var ld listingData
	if arr[1].Kind != "Listing" {
		return Thread{}, fmt.Errorf("expected comment Listing, got %q", arr[1].Kind)
	}
	if err := json.Unmarshal(arr[1].Data, &ld); err != nil {
		return Thread{}, fmt.Errorf("decode comments: %w", err)
	}
	comments, more, err := parseForest(ld.Children, 0)
	if err != nil {
		return Thread{}, err
	}
	return Thread{Post: posts.Posts[0], Comments: comments, More: more}, nil
}

// parseForest converts a listing's children into comments at the given depth
// plus at most one trailing more stub. Unknown kinds are skipped.
func parseForest(children []thing, depth int) ([]*Comment, *MoreStub, error) {
	var out []*Comment
	var more *MoreStub
	for _, ch := range children {
		switch ch.Kind {
		case "t1":
			c, err := parseComment(ch.Data, depth)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, c)
		case "more":
			m, err := parseMore(ch.Data)
			if err != nil {
				return nil, nil, err
			}
			more = mergeStubs(more, m)
		}
	}
	return out, more, nil
}

// mergeStubs combines two stubs for the same parent. Reddit sends one per
// listing level, but if it ever sends more, no reply IDs are lost. A
// "continue this thread" stub (no IDs) is kept only when it stands alone,
// because loading the IDs is the more useful action.
func mergeStubs(a, b *MoreStub) *MoreStub {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.IsContinue():
		return b
	case b.IsContinue():
		return a
	}
	return &MoreStub{ParentFullname: a.ParentFullname, Count: a.Count + b.Count, IDs: append(append([]string(nil), a.IDs...), b.IDs...)}
}

func parseComment(raw json.RawMessage, depth int) (*Comment, error) {
	var d commentData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode comment: %w", err)
	}
	c := &Comment{
		ID: d.ID, Fullname: d.Name, ParentFullname: d.ParentID, Author: d.Author, Body: d.Body,
		Score: d.Score, Created: unixTime(d.CreatedUTC), Depth: depth, IsSubmitter: d.IsSubmitter,
		Distinguished: str(d.Distinguished),
		AuthorDeleted: d.Author == "[deleted]",
		BodyRemoved:   d.Body == "[deleted]" || d.Body == "[removed]",
	}
	// replies is "" when empty, or a Listing thing.
	if len(d.Replies) > 0 && d.Replies[0] == '{' {
		var rt thing
		if err := json.Unmarshal(d.Replies, &rt); err != nil {
			return nil, fmt.Errorf("decode replies: %w", err)
		}
		var ld listingData
		if err := json.Unmarshal(rt.Data, &ld); err != nil {
			return nil, fmt.Errorf("decode replies data: %w", err)
		}
		kids, more, err := parseForest(ld.Children, depth+1)
		if err != nil {
			return nil, err
		}
		c.Children, c.More = kids, more
	}
	return c, nil
}

func parseMore(raw json.RawMessage) (*MoreStub, error) {
	var d moreData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode more: %w", err)
	}
	return &MoreStub{ParentFullname: d.ParentID, Count: d.Count, IDs: d.Children}, nil
}

// ParseMoreChildren decodes a morechildren response into flat things.
func ParseMoreChildren(r io.Reader) (Things, error) {
	var resp struct {
		JSON struct {
			Errors [][]any `json:"errors"`
			Data   struct {
				Things []thing `json:"things"`
			} `json:"data"`
		} `json:"json"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return Things{}, fmt.Errorf("decode morechildren: %w", err)
	}
	if len(resp.JSON.Errors) > 0 {
		return Things{}, fmt.Errorf("reddit error: %v", resp.JSON.Errors[0])
	}
	var th Things
	for _, t := range resp.JSON.Data.Things {
		switch t.Kind {
		case "t1":
			c, err := parseComment(t.Data, 0)
			if err != nil {
				return Things{}, err
			}
			th.Comments = append(th.Comments, c)
		case "more":
			m, err := parseMore(t.Data)
			if err != nil {
				return Things{}, err
			}
			th.Stubs = append(th.Stubs, m)
		}
	}
	return th, nil
}
