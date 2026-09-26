package reddit

import "testing"

func fixtureThread(t *testing.T) Thread {
	t.Helper()
	th, err := ParseThread(open(t, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func TestAttachTwoPassOrder(t *testing.T) {
	th := fixtureThread(t)
	c1 := th.Comments[0]
	stub := c1.More
	things, err := ParseMoreChildren(open(t, "morechildren.json"))
	if err != nil {
		t.Fatal(err)
	}
	n := Attach(&th, stub, things)
	if n != 2 {
		t.Errorf("attached %d, want 2", n)
	}
	if len(c1.Children) != 2 || c1.Children[1].ID != "x" || c1.Children[1].Depth != 1 {
		t.Fatalf("c1 children = %+v", c1.Children)
	}
	x := c1.Children[1]
	if len(x.Children) != 1 || x.Children[0].ID != "y" || x.Children[0].Depth != 2 {
		t.Errorf("x children = %+v", x.Children)
	}
	if x.More == nil || x.More.IDs[0] != "z" {
		t.Errorf("x more = %+v", x.More)
	}
	// Original stub listed x, y, z; all were returned (z via the nested stub), so c1 has no stub left.
	if c1.More != nil {
		t.Errorf("c1 stub should be consumed, got %+v", c1.More)
	}
}

func TestAttachKeepsUnreturnedIDs(t *testing.T) {
	th := fixtureThread(t)
	c1 := th.Comments[0]
	things := Things{Comments: []*Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Body: "x"}}}
	Attach(&th, c1.More, things)
	if c1.More == nil || c1.More.Count != 2 || len(c1.More.IDs) != 2 || c1.More.IDs[0] != "y" || c1.More.IDs[1] != "z" {
		t.Errorf("remaining stub = %+v", c1.More)
	}
}

func TestAttachTopLevelAndOrphans(t *testing.T) {
	th := fixtureThread(t)
	things := Things{Comments: []*Comment{
		{ID: "p", Fullname: "t1_p", ParentFullname: "t3_aaa", Body: "top"},
		{ID: "orphan", Fullname: "t1_orphan", ParentFullname: "t1_missing", Body: "lost"},
		{ID: "c2", Fullname: "t1_c2", ParentFullname: "t1_c1", Body: "duplicate of loaded"},
	}}
	n := Attach(&th, th.More, things)
	if n != 1 {
		t.Errorf("attached %d, want 1", n)
	}
	if len(th.Comments) != 4 || th.Comments[3].ID != "p" || th.Comments[3].Depth != 0 {
		t.Errorf("top-level = %d", len(th.Comments))
	}
	if th.More == nil || len(th.More.IDs) != 1 || th.More.IDs[0] != "q" {
		t.Errorf("thread stub = %+v", th.More)
	}
	if len(th.Comments[0].Children) != 1 {
		t.Error("duplicate c2 should not be attached twice")
	}
}
