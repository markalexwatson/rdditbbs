package threadmodel

import (
	"testing"

	"github.com/markalexwatson/redditbbs/internal/reddit"
	"github.com/markalexwatson/redditbbs/internal/reddit/redditest"
)

func ids(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case r.Comment != nil:
			out[i] = r.Comment.ID
		case r.Stub != nil && r.Stub.IsContinue():
			out[i] = "continue"
		default:
			out[i] = "more"
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRowsOrderNumbersAndConnectors(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	rows := m.Rows()
	want := []string{"c1", "c2", "more", "c3", "c4", "continue", "more"}
	if got := ids(rows); !equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i, r := range rows {
		if r.Number != i+1 {
			t.Errorf("row %d number = %d", i, r.Number)
		}
	}
	if rows[0].Depth != 0 || rows[0].Connector != "" {
		t.Errorf("c1 = %+v", rows[0])
	}
	if rows[1].Depth != 1 || rows[1].Connector != "├─" {
		t.Errorf("c2 connector = %q (a stub follows it)", rows[1].Connector)
	}
	if rows[2].Depth != 1 || rows[2].Connector != "└─" || rows[2].Stub.Count != 3 {
		t.Errorf("stub row = %+v", rows[2])
	}
	if rows[5].Connector != "└─" || !rows[5].Stub.IsContinue() {
		t.Errorf("continue row = %+v", rows[5])
	}
	if rows[6].Depth != 0 || rows[6].Stub.Count != 40 {
		t.Errorf("thread stub = %+v", rows[6])
	}
	if m.Loaded() != 4 {
		t.Errorf("loaded = %d", m.Loaded())
	}
}

func TestCollapseHidesDescendants(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	if !m.Toggle("c1") || !m.IsCollapsed("c1") {
		t.Fatal("toggle should collapse")
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got, []string{"c1", "c3", "c4", "continue", "more"}) {
		t.Fatalf("collapsed rows = %v", got)
	}
	if !rows[0].Collapsed || rows[0].Hidden != 1 {
		t.Errorf("c1 row = %+v", rows[0])
	}
	if m.Toggle("c1") {
		t.Error("second toggle should expand")
	}
	if len(m.Rows()) != 7 {
		t.Error("expand failed")
	}
	if got := len(m.Comments()); got != 4 {
		t.Errorf("comments = %d", got)
	}
}

func TestRowsOnlyStub(t *testing.T) {
	th := reddit.Thread{Post: redditest.SamplePost("z", "Empty"), More: &reddit.MoreStub{ParentFullname: "t3_z", Count: 12, IDs: []string{"a"}}}
	rows := New(th, 10).Rows()
	if len(rows) != 1 || rows[0].Stub == nil || rows[0].Number != 1 {
		t.Errorf("rows = %+v", rows)
	}
	if New(reddit.Thread{Post: th.Post}, 10).Rows() != nil {
		t.Error("no comments should give no rows")
	}
}

func TestRowsDeletedAuthorKeepsBody(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	r := m.Rows()[3]
	if r.Comment.ID != "c3" || !r.Comment.AuthorDeleted || r.Comment.Body != "Body survives the account." {
		t.Errorf("c3 = %+v", r.Comment)
	}
}

func TestAttachThroughModel(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	stub := m.Find("c1").More
	n := m.Attach(stub, reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Body: "X"}}})
	if n != 1 {
		t.Fatalf("attached %d", n)
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got[:4], []string{"c1", "c2", "x", "more"}) {
		t.Fatalf("rows = %v", got)
	}
	if rows[2].Depth != 1 || rows[2].Connector != "├─" || rows[3].Stub.Count != 2 {
		t.Errorf("x=%+v stub=%+v", rows[2], rows[3].Stub)
	}
}

func TestReplaceSubtree(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	sub := reddit.Thread{Post: m.Post(), Comments: []*reddit.Comment{{
		ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Body: "[removed]",
		Children: []*reddit.Comment{{ID: "k1", Fullname: "t1_k1", ParentFullname: "t1_c4", Body: "deep", Depth: 7,
			Children: []*reddit.Comment{{ID: "k2", Fullname: "t1_k2", ParentFullname: "t1_k1", Body: "deeper", Depth: 8}}}},
	}}}
	if !m.ReplaceSubtree("c4", sub) {
		t.Fatal("subtree not replaced")
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got, []string{"c1", "c2", "more", "c3", "c4", "k1", "k2", "more"}) {
		t.Fatalf("rows = %v", got)
	}
	if rows[5].Depth != 1 || rows[6].Depth != 2 || m.Find("k1").Depth != 1 {
		t.Errorf("depths not renumbered: %+v %+v", rows[5], rows[6])
	}
	if m.ReplaceSubtree("nope", sub) {
		t.Error("unknown id should return false")
	}
}

func TestDepthCap(t *testing.T) {
	chain := &reddit.Comment{ID: "d0", Fullname: "t1_d0", ParentFullname: "t3_p"}
	cur := chain
	for i := 1; i <= 3; i++ {
		next := &reddit.Comment{ID: "d" + string(rune('0'+i)), Fullname: "t1_d" + string(rune('0'+i)), ParentFullname: cur.Fullname, Depth: i}
		cur.Children = []*reddit.Comment{next}
		cur = next
	}
	m := New(reddit.Thread{Post: redditest.SamplePost("p", "P"), Comments: []*reddit.Comment{chain}}, 2)
	rows := m.Rows()
	if rows[2].Connector != "  └─" {
		t.Errorf("depth 2 connector = %q", rows[2].Connector)
	}
	if rows[3].Connector != "  »─" {
		t.Errorf("depth 3 (beyond cap) connector = %q", rows[3].Connector)
	}
}

func TestNewClonesInput(t *testing.T) {
	th := redditest.SampleThread()
	m := New(th, 10)
	m.Attach(m.Find("c1").More, reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1"}}})
	if len(th.Comments[0].Children) != 1 || th.Comments[0].More == nil {
		t.Error("model mutation leaked into the input thread")
	}
}

func TestReveal(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	m.Toggle("c1")
	if m.IndexOf("c2") != -1 {
		t.Fatal("c2 should be hidden")
	}
	m.Reveal("c2")
	if m.IsCollapsed("c1") || m.IndexOf("c2") != 1 {
		t.Error("Reveal should expand c1")
	}
}

func TestParentIndexOfAndReplace(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	c2 := m.Find("c2")
	if p := m.Parent(c2); p == nil || p.ID != "c1" {
		t.Errorf("parent of c2 = %+v", p)
	}
	if m.Parent(m.Find("c1")) != nil {
		t.Error("top-level parent should be nil")
	}
	if m.IndexOf("c3") != 3 || m.IndexOf("zzz") != -1 {
		t.Errorf("IndexOf = %d %d", m.IndexOf("c3"), m.IndexOf("zzz"))
	}
	m.Toggle("c1")
	m.Toggle("ghost")
	m.Replace(redditest.SampleThread())
	if !m.IsCollapsed("c1") || m.IsCollapsed("ghost") {
		t.Error("Replace should keep collapse state for surviving ids only")
	}
}
