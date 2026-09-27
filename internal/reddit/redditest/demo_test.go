package redditest

import (
	"context"
	"testing"
	"time"
)

func TestDemoStoreServesAnySubredditAndPost(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC) }
	d := NewDemoStore(now)
	l, err := d.Posts(context.Background(), "anything", "hot", "", struct{ Fresh bool }{})
	if err != nil || len(l.Posts) < 20 || l.Posts[0].Subreddit != "anything" {
		t.Fatalf("posts = %d err = %v", len(l.Posts), err)
	}
	th, err := d.Thread(context.Background(), "anything", l.Posts[3].ID, "best", struct{ Fresh bool }{})
	if err != nil || th.Post.ID != l.Posts[3].ID || len(th.Comments) == 0 {
		t.Fatalf("thread = %+v err = %v", th.Post, err)
	}
	more, err := d.MoreChildren(context.Background(), "t3_x", []string{"a", "b", "c"}, "best")
	if err != nil || len(more.Comments) != 3 {
		t.Fatalf("more = %d err = %v", len(more.Comments), err)
	}
	if l2, _ := d.Posts(context.Background(), "anything", "hot", "t3_last", struct{ Fresh bool }{}); len(l2.Posts) != 0 {
		t.Error("paging past the demo listing should end")
	}
}
