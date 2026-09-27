// Package reddit fetches and parses Reddit listings and comment threads
// through the OAuth API using app-only credentials.
package reddit

import (
	"context"
	"time"
)

// Sort orders a subreddit's posts.
type Sort string

// Post sorts.
const (
	Hot    Sort = "hot"
	New    Sort = "new"
	Top    Sort = "top"
	Rising Sort = "rising"
)

// Next cycles hot, new, top, rising.
func (s Sort) Next() Sort {
	switch s {
	case Hot:
		return New
	case New:
		return Top
	case Top:
		return Rising
	default:
		return Hot
	}
}

// CommentSort orders a thread's comments.
type CommentSort string

// Comment sorts.
const (
	Best        CommentSort = "best"
	TopComments CommentSort = "top"
	NewComments CommentSort = "new"
)

// API returns the value Reddit expects in the sort parameter.
func (c CommentSort) API() string {
	if c == Best {
		return "confidence"
	}
	return string(c)
}

// Next cycles best, top, new.
func (c CommentSort) Next() CommentSort {
	switch c {
	case Best:
		return TopComments
	case TopComments:
		return NewComments
	default:
		return Best
	}
}

// Fetch carries per-call options.
type Fetch struct {
	Fresh bool // bypass and replace the cache entry
}

// Post is a submission.
type Post struct {
	ID, Fullname  string
	Subreddit     string
	Title, Author string
	Score         int
	NumComments   int
	Created       time.Time
	URL, Domain   string
	Permalink     string
	IsSelf        bool
	SelfText      string
	Stickied      bool
	Over18        bool
	Distinguished string
	StatsKnown    bool // false when the source (RSS) has no score or comment count
}

// IsOP reports whether author wrote the post.
func (p *Post) IsOP(author string) bool { return p != nil && author != "" && p.Author == author }

// Comment is one node of the thread tree.
type Comment struct {
	ID, Fullname   string
	ParentFullname string
	Author, Body   string
	Score          int
	Created        time.Time
	Depth          int
	IsSubmitter    bool
	Distinguished  string
	AuthorDeleted  bool
	BodyRemoved    bool
	StatsKnown     bool // false when the source (RSS) has no score
	Children       []*Comment
	More           *MoreStub // unloaded replies to this comment, if any
}

// MoreStub stands for replies not yet loaded. Count 0 with no IDs is
// Reddit's "continue this thread" marker.
type MoreStub struct {
	ParentFullname string
	Count          int
	IDs            []string
}

// IsContinue reports a "continue this thread" stub.
func (m *MoreStub) IsContinue() bool { return m.Count == 0 && len(m.IDs) == 0 }

// Listing is one page of posts.
type Listing struct {
	Posts []*Post
	After string // empty at the end
}

// Thread is a post with its comment forest.
type Thread struct {
	Post     *Post
	Comments []*Comment
	More     *MoreStub // unloaded top-level comments
}

// Things is the flat result of a morechildren call.
type Things struct {
	Comments []*Comment
	Stubs    []*MoreStub
}

// Clone deep-copies a thread so callers may mutate it freely.
func (t Thread) Clone() Thread {
	out := Thread{}
	if t.Post != nil {
		p := *t.Post
		out.Post = &p
	}
	out.Comments = cloneComments(t.Comments)
	out.More = t.More.clone()
	return out
}

// Clone deep-copies morechildren results.
func (th Things) Clone() Things {
	out := Things{Comments: cloneComments(th.Comments)}
	for _, s := range th.Stubs {
		out.Stubs = append(out.Stubs, s.clone())
	}
	return out
}

func cloneComments(cs []*Comment) []*Comment {
	if cs == nil {
		return nil
	}
	out := make([]*Comment, len(cs))
	for i, c := range cs {
		cc := *c
		cc.Children = cloneComments(c.Children)
		cc.More = c.More.clone()
		out[i] = &cc
	}
	return out
}

func (m *MoreStub) clone() *MoreStub {
	if m == nil {
		return nil
	}
	c := *m
	c.IDs = append([]string(nil), m.IDs...)
	return &c
}

// Store is what screens read from.
type Store interface {
	Posts(ctx context.Context, subreddit string, sort Sort, after string, f Fetch) (Listing, error)
	Thread(ctx context.Context, subreddit, postID string, sort CommentSort, f Fetch) (Thread, error)
	Subtree(ctx context.Context, subreddit, postID, commentID string, sort CommentSort) (Thread, error)
	MoreChildren(ctx context.Context, linkFullname string, ids []string, sort CommentSort) (Things, error)
}
