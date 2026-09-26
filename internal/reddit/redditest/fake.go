// Package redditest provides a fake Store and sample data for UI tests.
package redditest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/markwatson/redditbbs/internal/reddit"
)

// FakeStore serves canned data. Keys: Listings "sub/sort/after", Threads by
// post ID, Subtrees by comment ID.
type FakeStore struct {
	mu       sync.Mutex
	Listings map[string]reddit.Listing
	Threads  map[string]reddit.Thread
	Subtrees map[string]reddit.Thread
	More     reddit.Things
	MoreErr  error // returned alongside More, for partial-batch failures
	Err      error
	Calls    []string
	Block    chan struct{}
}

// NewFakeStore returns an empty fake.
func NewFakeStore() *FakeStore {
	return &FakeStore{Listings: map[string]reddit.Listing{}, Threads: map[string]reddit.Thread{}, Subtrees: map[string]reddit.Thread{}}
}

func (f *FakeStore) record(ctx context.Context, call string) error {
	f.mu.Lock()
	f.Calls = append(f.Calls, call)
	block, err := f.Block, f.Err
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// CallCount returns how many calls were recorded.
func (f *FakeStore) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

// Posts implements Store.
func (f *FakeStore) Posts(ctx context.Context, sub string, sort reddit.Sort, after string, fe reddit.Fetch) (reddit.Listing, error) {
	if err := f.record(ctx, fmt.Sprintf("posts:%s/%s/%s/fresh=%v", sub, sort, after, fe.Fresh)); err != nil {
		return reddit.Listing{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.Listings[sub+"/"+string(sort)+"/"+after]
	if !ok {
		return reddit.Listing{}, &reddit.APIError{Status: 404}
	}
	return l, nil
}

// Thread implements Store.
func (f *FakeStore) Thread(ctx context.Context, sub, postID string, sort reddit.CommentSort, fe reddit.Fetch) (reddit.Thread, error) {
	if err := f.record(ctx, fmt.Sprintf("thread:%s/%s/%s/fresh=%v", sub, postID, sort, fe.Fresh)); err != nil {
		return reddit.Thread{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	th, ok := f.Threads[postID]
	if !ok {
		return reddit.Thread{}, &reddit.APIError{Status: 404}
	}
	return th, nil
}

// Subtree implements Store.
func (f *FakeStore) Subtree(ctx context.Context, sub, postID, commentID string, sort reddit.CommentSort) (reddit.Thread, error) {
	if err := f.record(ctx, "subtree:"+commentID); err != nil {
		return reddit.Thread{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	th, ok := f.Subtrees[commentID]
	if !ok {
		return reddit.Thread{}, &reddit.APIError{Status: 404}
	}
	return th, nil
}

// MoreChildren implements Store.
func (f *FakeStore) MoreChildren(ctx context.Context, link string, ids []string, sort reddit.CommentSort) (reddit.Things, error) {
	if err := f.record(ctx, fmt.Sprintf("more:%s/%d", link, len(ids))); err != nil {
		return reddit.Things{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.More, f.MoreErr
}

var _ reddit.Store = (*FakeStore)(nil)

var base = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// SamplePost builds a link post.
func SamplePost(id, title string) *reddit.Post {
	return &reddit.Post{
		ID: id, Fullname: "t3_" + id, Subreddit: "linux", Title: title, Author: "author_" + id,
		Score: 100, NumComments: 10, Created: base.Add(-3 * time.Hour),
		URL: "https://example.com/" + id, Domain: "example.com", Permalink: "/r/linux/comments/" + id + "/",
	}
}

// SampleListing builds n posts titled "Post 1".."Post n" with IDs p1..pn.
func SampleListing(n int, after string) reddit.Listing {
	l := reddit.Listing{After: after}
	for i := 1; i <= n; i++ {
		l.Posts = append(l.Posts, SamplePost(fmt.Sprintf("p%d", i), fmt.Sprintf("Post %d", i)))
	}
	return l
}

// SampleThread mirrors internal/reddit/testdata/thread.json.
func SampleThread() reddit.Thread {
	post := SamplePost("aaa", "Kernel 7.2 released")
	post.Author = "torvaldsfan"
	post.NumComments = 342
	c2 := &reddit.Comment{ID: "c2", Fullname: "t1_c2", ParentFullname: "t1_c1", Author: "torvaldsfan", Body: "Agreed.", Score: 98, Created: base.Add(-time.Hour), Depth: 1, IsSubmitter: true}
	c1 := &reddit.Comment{ID: "c1", Fullname: "t1_c1", ParentFullname: "t3_aaa", Author: "sched_nerd", Body: "The EEVDF changes are the headline.\n\nLazy preemption is the real win.", Score: 412, Created: base.Add(-2 * time.Hour), Children: []*reddit.Comment{c2}, More: &reddit.MoreStub{ParentFullname: "t1_c1", Count: 3, IDs: []string{"x", "y", "z"}}}
	c3 := &reddit.Comment{ID: "c3", Fullname: "t1_c3", ParentFullname: "t3_aaa", Author: "[deleted]", AuthorDeleted: true, Body: "Body survives the account.", Score: 5, Created: base.Add(-2 * time.Hour)}
	c4 := &reddit.Comment{ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Author: "modbot", Body: "[removed]", BodyRemoved: true, Score: 1, Created: base.Add(-2 * time.Hour), Distinguished: "moderator", More: &reddit.MoreStub{ParentFullname: "t1_c4"}}
	return reddit.Thread{Post: post, Comments: []*reddit.Comment{c1, c3, c4}, More: &reddit.MoreStub{ParentFullname: "t3_aaa", Count: 40, IDs: []string{"p", "q"}}}
}
