package reddit

import (
	"os"
	"strings"
	"testing"
	"time"
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
	l, err := ParseListing(open(t, "listing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if l.After != "t3_bbb" || len(l.Posts) != 2 {
		t.Fatalf("after=%q posts=%d", l.After, len(l.Posts))
	}
	p := l.Posts[0]
	if p.ID != "aaa" || p.Fullname != "t3_aaa" || p.Title != "Kernel 7.2 released" || p.Author != "torvaldsfan" {
		t.Errorf("post 0 = %+v", p)
	}
	if p.Score != 2100 || p.NumComments != 342 || p.Domain != "kernel.org" || p.IsSelf {
		t.Errorf("post 0 numbers = %+v", p)
	}
	if !p.Created.Equal(time.Unix(1790400000, 0)) {
		t.Errorf("created = %v", p.Created)
	}
	if !p.StatsKnown {
		t.Error("API posts carry scores and comment counts, so StatsKnown must be true")
	}
	q := l.Posts[1]
	if !q.IsSelf || !q.Stickied || q.Distinguished != "moderator" || q.SelfText != "Ask **anything** here." {
		t.Errorf("post 1 = %+v", q)
	}
}

func TestParseListingRejectsNonListing(t *testing.T) {
	if _, err := ParseListing(strings.NewReader(`{"kind":"t3","data":{}}`)); err == nil {
		t.Error("expected error")
	}
	if _, err := ParseListing(strings.NewReader(`not json`)); err == nil {
		t.Error("expected error")
	}
}

func TestParseThread(t *testing.T) {
	th, err := ParseThread(open(t, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Post == nil || th.Post.ID != "aaa" {
		t.Fatalf("post = %+v", th.Post)
	}
	if len(th.Comments) != 3 {
		t.Fatalf("top-level comments = %d, want 3 (unknown kind skipped)", len(th.Comments))
	}
	c1 := th.Comments[0]
	if c1.ID != "c1" || c1.Fullname != "t1_c1" || c1.ParentFullname != "t3_aaa" || c1.Depth != 0 {
		t.Errorf("c1 = %+v", c1)
	}
	if len(c1.Children) != 1 || c1.Children[0].ID != "c2" || c1.Children[0].Depth != 1 {
		t.Errorf("c1 children = %+v", c1.Children)
	}
	if !c1.Children[0].IsSubmitter {
		t.Error("c2 should be submitter")
	}
	if !c1.StatsKnown || !c1.Children[0].StatsKnown {
		t.Error("API comments carry scores, so StatsKnown must be true")
	}
	if c1.More == nil || c1.More.Count != 3 || len(c1.More.IDs) != 3 || c1.More.ParentFullname != "t1_c1" {
		t.Errorf("c1 more = %+v", c1.More)
	}
	c3 := th.Comments[1]
	if !c3.AuthorDeleted || c3.BodyRemoved || c3.Body != "Body survives the account." {
		t.Errorf("c3 = %+v", c3)
	}
	c4 := th.Comments[2]
	if c4.AuthorDeleted || !c4.BodyRemoved || c4.Distinguished != "moderator" {
		t.Errorf("c4 = %+v", c4)
	}
	if c4.More == nil || c4.More.Count != 0 || len(c4.More.IDs) != 0 {
		t.Errorf("c4 continue stub = %+v", c4.More)
	}
	if th.More == nil || th.More.Count != 40 || th.More.ParentFullname != "t3_aaa" {
		t.Errorf("thread more = %+v", th.More)
	}
}

func TestMergeStubs(t *testing.T) {
	a := &MoreStub{ParentFullname: "t1_p", Count: 2, IDs: []string{"a", "b"}}
	b := &MoreStub{ParentFullname: "t1_p", Count: 1, IDs: []string{"c"}}
	cont := &MoreStub{ParentFullname: "t1_p"}
	if m := mergeStubs(a, b); m.Count != 3 || len(m.IDs) != 3 {
		t.Errorf("merge = %+v", m)
	}
	if m := mergeStubs(cont, a); m != a {
		t.Error("a continue stub should give way to a stub with IDs")
	}
	if m := mergeStubs(nil, cont); m != cont || !m.IsContinue() {
		t.Error("a lone continue stub is kept")
	}
}

func TestParseThreadRejectsShortArray(t *testing.T) {
	if _, err := ParseThread(strings.NewReader(`[{"kind":"Listing","data":{"children":[]}}]`)); err == nil {
		t.Error("expected error")
	}
}

func TestParseMoreChildren(t *testing.T) {
	th, err := ParseMoreChildren(open(t, "morechildren.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 2 || th.Comments[0].ID != "y" || th.Comments[1].ID != "x" {
		t.Errorf("comments = %+v", th.Comments)
	}
	if len(th.Stubs) != 1 || th.Stubs[0].ParentFullname != "t1_x" || th.Stubs[0].IDs[0] != "z" {
		t.Errorf("stubs = %+v", th.Stubs)
	}
}

func TestParseMoreChildrenErrors(t *testing.T) {
	_, err := ParseMoreChildren(strings.NewReader(`{"json":{"errors":[["TOO_MANY","limit exceeded","children"]],"data":{"things":[]}}}`))
	if err == nil || !strings.Contains(err.Error(), "TOO_MANY") {
		t.Errorf("err = %v", err)
	}
}

func TestThreadClone(t *testing.T) {
	th, _ := ParseThread(open(t, "thread.json"))
	c := th.Clone()
	c.Comments[0].Children[0].Body = "changed"
	c.Comments[0].More.IDs[0] = "changed"
	c.Post.Title = "changed"
	if th.Comments[0].Children[0].Body == "changed" || th.Comments[0].More.IDs[0] == "changed" || th.Post.Title == "changed" {
		t.Error("Clone must not share comments, stubs or the post")
	}
	if c.Comments[2].More == nil || len(c.Comments) != 3 {
		t.Error("Clone lost structure")
	}
}

func TestSorts(t *testing.T) {
	if Best.API() != "confidence" || TopComments.API() != "top" {
		t.Error("comment sort API mapping")
	}
	if Hot.Next() != New || Rising.Next() != Hot {
		t.Error("sort cycle")
	}
	if Best.Next() != TopComments || NewComments.Next() != Best {
		t.Error("comment sort cycle")
	}
}
