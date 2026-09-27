// Package threadmodel turns a comment tree into the flat, numbered list of
// rows the Thread Index shows, tracking collapse state and splicing in
// comments loaded later.
package threadmodel

import (
	"strings"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

// Row is one line of the thread index: a comment or a stub.
type Row struct {
	Comment   *reddit.Comment
	Stub      *reddit.MoreStub
	Number    int // 1-based display number
	Depth     int
	Connector string
	Collapsed bool
	Hidden    int // loaded descendants hidden by collapse
}

// Model owns a mutable copy of a thread plus UI state.
type Model struct {
	thread    reddit.Thread
	collapsed map[string]bool
	maxDepth  int
}

// New deep-copies t so the Store's value is never mutated. maxDepth is the
// deepest level drawn with its own indent.
func New(t reddit.Thread, maxDepth int) *Model {
	m := &Model{thread: t.Clone(), collapsed: map[string]bool{}}
	m.SetMaxDepth(maxDepth)
	return m
}

// SetMaxDepth changes the indent cap (minimum 2).
func (m *Model) SetMaxDepth(d int) {
	if d < 2 {
		d = 2
	}
	m.maxDepth = d
}

// Thread exposes the underlying tree.
func (m *Model) Thread() *reddit.Thread { return &m.thread }

// Post is the thread's post.
func (m *Model) Post() *reddit.Post { return m.thread.Post }

// Replace swaps in a copy of a refetched thread, keeping collapse state for
// ids that survive.
func (m *Model) Replace(t reddit.Thread) {
	m.thread = t.Clone()
	alive := map[string]bool{}
	m.walk(func(c *reddit.Comment) { alive[c.ID] = true })
	for id := range m.collapsed {
		if !alive[id] {
			delete(m.collapsed, id)
		}
	}
}

func (m *Model) walk(fn func(*reddit.Comment)) {
	var rec func([]*reddit.Comment)
	rec = func(cs []*reddit.Comment) {
		for _, c := range cs {
			fn(c)
			rec(c.Children)
		}
	}
	rec(m.thread.Comments)
}

// Loaded counts comments in the tree.
func (m *Model) Loaded() int {
	n := 0
	m.walk(func(*reddit.Comment) { n++ })
	return n
}

// Find returns the comment with id, or nil.
func (m *Model) Find(id string) *reddit.Comment {
	var found *reddit.Comment
	m.walk(func(c *reddit.Comment) {
		if c.ID == id && found == nil {
			found = c
		}
	})
	return found
}

// Parent returns c's parent comment, or nil for a top-level comment.
func (m *Model) Parent(c *reddit.Comment) *reddit.Comment {
	if c == nil || !strings.HasPrefix(c.ParentFullname, "t1_") {
		return nil
	}
	return m.Find(strings.TrimPrefix(c.ParentFullname, "t1_"))
}

// Toggle flips collapse for id and returns the new state.
func (m *Model) Toggle(id string) bool {
	m.collapsed[id] = !m.collapsed[id]
	if !m.collapsed[id] {
		delete(m.collapsed, id)
	}
	return m.collapsed[id]
}

// IsCollapsed reports collapse state.
func (m *Model) IsCollapsed(id string) bool { return m.collapsed[id] }

// Reveal expands every collapsed ancestor of id so it becomes visible.
func (m *Model) Reveal(id string) {
	for c := m.Parent(m.Find(id)); c != nil; c = m.Parent(c) {
		delete(m.collapsed, c.ID)
	}
}

func countDescendants(c *reddit.Comment) int {
	n := 0
	for _, k := range c.Children {
		n += 1 + countDescendants(k)
	}
	return n
}

func (m *Model) connector(depth int, anc []bool, hasNext bool) string {
	if depth == 0 {
		return ""
	}
	if depth > m.maxDepth {
		return strings.Repeat("  ", m.maxDepth-1) + "»─"
	}
	return widgets.Connector(anc[1:], hasNext)
}

// Rows flattens the tree in display order, skipping collapsed descendants.
// anc carries one entry per ancestor level (true when that ancestor has later
// siblings), so len(anc) is the depth of the comments being walked.
func (m *Model) Rows() []Row {
	var rows []Row
	var walk func(cs []*reddit.Comment, anc []bool, trailingStub bool)
	walk = func(cs []*reddit.Comment, anc []bool, trailingStub bool) {
		depth := len(anc)
		for i, c := range cs {
			hasNext := i < len(cs)-1 || trailingStub
			row := Row{Comment: c, Depth: depth, Connector: m.connector(depth, anc, hasNext)}
			if m.collapsed[c.ID] && (len(c.Children) > 0 || c.More != nil) {
				row.Collapsed = true
				row.Hidden = countDescendants(c)
				rows = append(rows, row)
				continue
			}
			rows = append(rows, row)
			childAnc := append(append([]bool(nil), anc...), hasNext)
			walk(c.Children, childAnc, c.More != nil)
			if c.More != nil {
				rows = append(rows, Row{Stub: c.More, Depth: depth + 1, Connector: m.connector(depth+1, childAnc, false)})
			}
		}
	}
	walk(m.thread.Comments, nil, m.thread.More != nil)
	if m.thread.More != nil {
		rows = append(rows, Row{Stub: m.thread.More})
	}
	for i := range rows {
		rows[i].Number = i + 1
	}
	return rows
}

// Comments returns the visible comments in display order, without stubs.
func (m *Model) Comments() []*reddit.Comment {
	var out []*reddit.Comment
	for _, r := range m.Rows() {
		if r.Comment != nil {
			out = append(out, r.Comment)
		}
	}
	return out
}

// IndexOf returns the row index of comment id, or -1.
func (m *Model) IndexOf(id string) int {
	for i, r := range m.Rows() {
		if r.Comment != nil && r.Comment.ID == id {
			return i
		}
	}
	return -1
}

// Attach splices a copy of morechildren results for stub into the tree.
func (m *Model) Attach(stub *reddit.MoreStub, th reddit.Things) int {
	return reddit.Attach(&m.thread, stub, th.Clone())
}

// ReplaceSubtree replaces comment id's children with those of the matching
// root comment in sub (a Subtree response), renumbering depths.
func (m *Model) ReplaceSubtree(id string, sub reddit.Thread) bool {
	target := m.Find(id)
	if target == nil {
		return false
	}
	sub = sub.Clone()
	var root *reddit.Comment
	for _, c := range sub.Comments {
		if c.ID == id {
			root = c
			break
		}
	}
	if root == nil {
		return false
	}
	target.Children, target.More = root.Children, root.More
	redepth(target.Children, target.Depth+1)
	return true
}

func redepth(cs []*reddit.Comment, d int) {
	for _, c := range cs {
		c.Depth = d
		redepth(c.Children, d+1)
	}
}
