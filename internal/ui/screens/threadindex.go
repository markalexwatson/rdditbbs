package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/threadmodel"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newReader is indirected so this task compiles before Message Reader exists.
var newReader = func(d *Deps, m *threadmodel.Model, commentID string) ui.Screen { return NewReader(d, m, commentID) }

const (
	minTableRows = 5
	minPeekRows  = 5
	authorColW   = 22
	scoreColW    = 6
)

// ThreadIndex lists a post's comments as a tree with a peek pane.
type ThreadIndex struct {
	d       *Deps
	post    *reddit.Post
	sort    reddit.CommentSort // committed
	reqSort reddit.CommentSort // requested, shown while loading

	model *threadmodel.Model
	rows  []threadmodel.Row
	cap   int // indent cap the rows were built with
	table widgets.Table
	num   widgets.NumInput

	peek    bool
	peekTop int
	peekH   int // body lines in the peek pane at the last draw

	selectedID string
	loading    bool
	authFailed bool
	gen        int
	status     string
	statusErr  bool
}

type threadMsg struct {
	gen  int
	sort reddit.CommentSort
	th   reddit.Thread
	err  error
}

type moreMsg struct {
	gen    int
	stub   *reddit.MoreStub
	things reddit.Things
	err    error
}

type subtreeMsg struct {
	gen int
	id  string
	th  reddit.Thread
	err error
}

// NewThreadIndex creates the index for post.
func NewThreadIndex(d *Deps, post *reddit.Post) *ThreadIndex {
	t := &ThreadIndex{d: d, post: post, sort: reddit.Best, reqSort: reddit.Best, peek: d.Config.Display.PeekPane, cap: 10}
	t.table.SetHeight(minTableRows)
	return t
}

func (t *ThreadIndex) Init() ui.Action { return t.fetch(false) }
func (t *ThreadIndex) Title() string   { return "Thread Index" }

func (t *ThreadIndex) Info() string {
	return fmt.Sprintf("r/%s · %s · %d msgs", t.post.Subreddit, strings.ToUpper(string(t.reqSort)), t.post.NumComments)
}

func (t *ThreadIndex) Keys() []ui.KeyHelp {
	keys := []ui.KeyHelp{{Key: "#/⏎", Desc: "Read"}, {Key: "B", Desc: "ody"}, {Key: "-/+", Desc: "Fold"}, {Key: "Tab", Desc: "Peek"}, {Key: "S", Desc: "ort"}}
	if t.authFailed {
		keys = append(keys, ui.KeyHelp{Key: "L", Desc: "og in"})
	}
	return append(keys, ui.KeyHelp{Key: "O", Desc: "pen link"}, ui.KeyHelp{Key: "R", Desc: "efresh"}, ui.KeyHelp{Key: "Q", Desc: "uit"})
}

func (t *ThreadIndex) Prompt() widgets.Prompt {
	return widgets.Prompt{Input: t.num.Digits, Status: t.status, Error: t.statusErr}
}

func (t *ThreadIndex) fetch(fresh bool) ui.Action {
	t.gen++
	gen := t.gen
	t.loading = true
	t.status, t.statusErr = "Retrieving...", false
	store, sub, id, sort := t.d.Store, t.post.Subreddit, t.post.ID, t.reqSort
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		th, err := store.Thread(ctx, sub, id, sort, reddit.Fetch{Fresh: fresh})
		return threadMsg{gen: gen, sort: sort, th: th, err: err}
	}}
}

// refresh recomputes rows after any model change and restores the selection.
func (t *ThreadIndex) refresh() {
	t.rows = t.model.Rows()
	t.table.SetCount(len(t.rows))
	if t.selectedID != "" {
		if i := t.model.IndexOf(t.selectedID); i >= 0 {
			t.table.Select(i)
		}
	}
	t.syncSelected()
}

func (t *ThreadIndex) syncSelected() {
	if t.table.Cursor < len(t.rows) && t.rows[t.table.Cursor].Comment != nil {
		t.selectedID = t.rows[t.table.Cursor].Comment.ID
	}
}

func (t *ThreadIndex) fail(err error) {
	t.loading = false
	t.reqSort = t.sort
	t.authFailed = isAuthError(err)
	t.status, t.statusErr = errText(err), true
}

func (t *ThreadIndex) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case threadMsg:
		if m.gen != t.gen {
			return nil
		}
		if m.err != nil {
			t.fail(m.err)
			return nil
		}
		t.loading = false
		t.authFailed = false
		t.sort = m.sort
		if m.th.Post != nil {
			t.post = m.th.Post
		}
		if t.model == nil {
			t.model = threadmodel.New(m.th, t.cap)
		} else {
			t.model.Replace(m.th)
		}
		t.status, t.statusErr = "", false
		t.refresh()
	case moreMsg:
		if m.gen != t.gen {
			return nil
		}
		// A later batch may fail after earlier ones succeeded: keep what arrived.
		if len(m.things.Comments) > 0 || len(m.things.Stubs) > 0 {
			t.model.Attach(m.stub, m.things)
			t.refresh()
		}
		if m.err != nil {
			t.fail(m.err)
			return nil
		}
		t.loading = false
		t.status = ""
	case subtreeMsg:
		if m.gen != t.gen {
			return nil
		}
		if m.err != nil {
			t.fail(m.err)
			return nil
		}
		t.loading = false
		if !t.model.ReplaceSubtree(m.id, m.th) {
			t.status, t.statusErr = "Could not load that part of the thread", true
		} else {
			t.status = ""
		}
		t.refresh()
	case ui.ErrMsg:
		t.fail(m.Err)
	case linkExit:
		if m.Err != nil {
			t.status, t.statusErr = "Browser exited with an error. URL: "+m.URL, true
		}
	case ui.PopResult:
		if sc, ok := m.Result.(SelectComment); ok && sc.ID != "" && t.model != nil {
			t.selectedID = sc.ID
			t.refresh()
		}
		if t.authFailed && t.d.Config.HasCredentials() {
			t.authFailed = false
			return t.fetch(true)
		}
	case ui.RateLimited:
		t.status, t.statusErr = fmt.Sprintf("Rate limited, retrying in %ds", int(m.Wait.Seconds())), false
	}
	return nil
}

func (t *ThreadIndex) Draw(c term.Canvas) {
	w, h := c.Size()
	p := t.post
	c.Text(2, 0, textfmt.Truncate(p.Title, w-4), theme.Style(theme.Subject), w-4)
	meta := fmt.Sprintf("by %s · %s · %s · %d comments", p.Author, textfmt.Score(p.Score), textfmt.RelTime(p.Created, t.d.now()), p.NumComments)
	c.Text(2, 1, textfmt.Truncate(meta, w-4), theme.Style(theme.Meta), w-4)
	if t.model == nil {
		return
	}
	if cap := w / 8; cap != t.cap {
		t.cap = cap
		t.model.SetMaxDepth(cap)
		t.refresh()
	}

	// Row budget: header 2, preview, rule, table heading + rows, [rule, peek].
	var preview []textfmt.Line
	previewRows := 0
	if p.IsSelf && p.SelfText != "" {
		preview = textfmt.Render(p.SelfText, w-4).Lines
		previewRows = len(preview)
		if previewRows > h/4 {
			previewRows = h / 4
		}
	}
	peekRows := 0
	if t.peek {
		peekRows = h / 3
		if peekRows < minPeekRows {
			peekRows = minPeekRows
		}
	}
	avail := func() int {
		a := h - 2 - previewRows - 1 - 1
		if peekRows > 0 {
			a -= peekRows + 1
		}
		return a
	}
	for avail() < minTableRows && previewRows > 0 {
		previewRows--
	}
	for avail() < minTableRows && peekRows > minPeekRows {
		peekRows--
	}
	if avail() < minTableRows && peekRows > 0 {
		peekRows = 0
	}
	tableRows := avail()
	if tableRows < 1 {
		tableRows = 1
	}
	t.table.SetHeight(tableRows)
	t.peekH = peekRows - 1

	y := 2
	for i := 0; i < previewRows; i++ {
		widgets.DrawLine(c, 2, y+i, w-4, preview[i])
	}
	y += previewRows
	widgets.Rule(c, y)
	y++

	numW := len(itoa(len(t.rows)))
	if numW < 3 {
		numW = 3
	}
	hd := theme.Style(theme.Heading)
	c.Text(2, y, textfmt.PadLeft("#", numW)+"  "+textfmt.PadLeft("Score", scoreColW)+"  "+textfmt.PadRight("From", authorColW)+"  Preview", hd, w-2)
	y++
	previewW := w - (2 + numW + 2 + scoreColW + 2 + authorColW + 2) - 1
	start, end := t.table.Visible()
	for i := start; i < end; i++ {
		row := t.rows[i]
		ry := y + i - start
		sel := i == t.table.Cursor
		st := func(r theme.Role) term.Style {
			if sel {
				return theme.Style(theme.Cursor)
			}
			return theme.Style(r)
		}
		if sel {
			c.Fill(0, ry, w, 1, ' ', theme.Style(theme.Cursor))
		}
		x := 2
		x += c.Text(x, ry, textfmt.PadLeft(itoa(row.Number), numW)+"  ", st(theme.Meta), w)
		if row.Stub != nil {
			x += c.Text(x, ry, textfmt.PadLeft("", scoreColW)+"  ", st(theme.Meta), w)
			label := "[load " + itoa(row.Stub.Count) + " more replies]"
			if row.Stub.IsContinue() {
				label = "[continue this thread]"
			}
			c.Text(x, ry, row.Connector+label, st(theme.Stub), w-x-2)
			continue
		}
		cm := row.Comment
		x += c.Text(x, ry, textfmt.PadLeft(textfmt.Score(cm.Score), scoreColW)+"  ", st(theme.Meta), w)
		author, role := authorAndRole(cm, p)
		name := row.Connector + author
		if row.Collapsed {
			name += " [+" + itoa(row.Hidden) + " hidden]"
		}
		x += c.Text(x, ry, textfmt.PadRight(name, authorColW)+"  ", st(role), w)
		prev, prole := textfmt.FirstLine(cm.Body), theme.Body
		if cm.BodyRemoved {
			prev, prole = cm.Body, theme.Meta
		}
		c.Text(x, ry, textfmt.Truncate(prev, previewW), st(prole), previewW)
	}
	y += t.table.Height()

	if peekRows > 0 {
		widgets.Rule(c, y)
		y++
		t.drawPeek(c, y, w, peekRows)
	}
}

// authorAndRole picks the display name and colour role for a comment author.
func authorAndRole(cm *reddit.Comment, post *reddit.Post) (string, theme.Role) {
	switch {
	case cm.AuthorDeleted:
		return "[deleted]", theme.Meta
	case cm.Distinguished != "":
		return cm.Author, theme.Mod
	case cm.IsSubmitter || post.IsOP(cm.Author):
		return cm.Author, theme.OP
	}
	return cm.Author, theme.Author
}

func (t *ThreadIndex) drawPeek(c term.Canvas, y, w, rows int) {
	if len(t.rows) == 0 {
		return
	}
	row := t.rows[t.table.Cursor]
	if row.Comment == nil {
		c.Text(2, y, "Press Enter to load these replies", theme.Style(theme.Meta), w-4)
		return
	}
	cm := row.Comment
	author, role := authorAndRole(cm, t.post)
	x := 2
	x += c.Text(x, y, author, theme.Style(role), w-4)
	c.Text(x, y, " · "+textfmt.Score(cm.Score)+" · "+textfmt.RelTime(cm.Created, t.d.now()), theme.Style(theme.Meta), w-2-x)
	lines := textfmt.Render(cm.Body, w-4).Lines
	bodyRows := rows - 1
	if max := len(lines) - bodyRows; t.peekTop > max {
		t.peekTop = max
	}
	if t.peekTop < 0 {
		t.peekTop = 0
	}
	for i := 0; i < bodyRows && t.peekTop+i < len(lines); i++ {
		widgets.DrawLine(c, 2, y+1+i, w-4, lines[t.peekTop+i])
	}
}

func (t *ThreadIndex) HandleKey(k term.Key) ui.Action {
	if k.Paste {
		return nil
	}
	if v, submitted, handled := t.num.HandleKey(k); handled {
		if submitted {
			return t.openRow(v - 1)
		}
		return nil
	}
	if t.table.HandleKey(k) {
		t.peekTop = 0
		t.syncSelected()
		return nil
	}
	switch k.Code {
	case term.KeyEnter:
		return t.openRow(t.table.Cursor)
	case term.KeyTab:
		t.peek = !t.peek
		return nil
	}
	switch {
	case k.Code == term.KeyRune && k.Rune == ' ':
		step := t.peekH
		if step < 1 {
			step = 1
		}
		t.peekTop += step
	case Rune(k) == 'B':
		if t.model != nil {
			return ui.Push{Screen: newReader(t.d, t.model, "")}
		}
	case Rune(k) == 'S':
		t.reqSort = t.reqSort.Next()
		return t.fetch(false)
	case Rune(k) == 'L':
		if t.authFailed {
			return ui.Push{Screen: NewNestedSetup(t.d)}
		}
	case Rune(k) == 'O':
		return t.openLink()
	case Rune(k) == 'R':
		return t.fetch(true)
	case Rune(k) == '-':
		t.fold(true)
	case Rune(k) == '+', Rune(k) == '=':
		t.fold(false)
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

func (t *ThreadIndex) fold(collapse bool) {
	if t.model == nil || t.table.Cursor >= len(t.rows) {
		return
	}
	row := t.rows[t.table.Cursor]
	if row.Comment == nil || t.model.IsCollapsed(row.Comment.ID) == collapse {
		return
	}
	t.model.Toggle(row.Comment.ID)
	t.refresh()
}

func (t *ThreadIndex) openRow(i int) ui.Action {
	if t.model == nil || i < 0 || i >= len(t.rows) {
		if len(t.rows) > 0 {
			t.status, t.statusErr = "No such message", true
		}
		return nil
	}
	t.table.Select(i)
	t.peekTop = 0
	t.syncSelected()
	row := t.rows[i]
	if row.Comment != nil {
		return ui.Push{Screen: newReader(t.d, t.model, row.Comment.ID)}
	}
	if t.loading {
		return nil
	}
	t.loading = true
	t.gen++
	gen := t.gen
	t.status, t.statusErr = "Retrieving...", false
	store, sub, postID, link, sort := t.d.Store, t.post.Subreddit, t.post.ID, t.post.Fullname, t.sort
	stub := row.Stub
	if stub.IsContinue() {
		parentID := strings.TrimPrefix(stub.ParentFullname, "t1_")
		return ui.Run{Fn: func(ctx context.Context) ui.Msg {
			th, err := store.Subtree(ctx, sub, postID, parentID, sort)
			return subtreeMsg{gen: gen, id: parentID, th: th, err: err}
		}}
	}
	ids := append([]string(nil), stub.IDs...)
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		things, err := store.MoreChildren(ctx, link, ids, sort)
		return moreMsg{gen: gen, stub: stub, things: things, err: err}
	}}
}

func (t *ThreadIndex) openLink() ui.Action {
	u := t.post.URL
	if t.post.IsSelf || u == "" {
		u = "https://www.reddit.com" + t.post.Permalink
	}
	act, err := t.d.openInBrowser(u)
	if err != nil {
		t.status, t.statusErr = "Could not open browser. URL: "+u, true
		return nil
	}
	t.status, t.statusErr = "Opened in browser", false
	return act
}
