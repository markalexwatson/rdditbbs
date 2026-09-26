package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newThreadIndex is indirected so this task compiles before Thread Index exists.
var newThreadIndex = func(d *Deps, p *reddit.Post) ui.Screen { return &placeholder{name: "thread index"} }

// PostList shows one subreddit's posts, paginated to the screen height.
type PostList struct {
	d       *Deps
	area    config.Area
	saved   bool
	sort    reddit.Sort // committed: the sort the loaded posts have
	reqSort reddit.Sort // requested: shown in the title while a sort change loads

	posts  []*reddit.Post
	seen   map[string]bool
	after  string
	ended  bool
	cursor int
	rows   int // data rows per page

	num  widgets.NumInput
	join *widgets.TextInput

	loading        bool
	pendingAdvance bool
	pendingMove    int    // cursor delta still owed after a paging fetch (0 when none)
	keepID         string // reselect this post after a replacing fetch
	authFailed     bool
	gen            int
	status         string
	statusErr      bool
}

type postsMsg struct {
	gen     int
	sort    reddit.Sort
	after   string
	listing reddit.Listing
	err     error
	replace bool
}

// NewPostList creates a post list for area. saved says whether the area is in
// the config already.
func NewPostList(d *Deps, area config.Area, saved bool) *PostList {
	s := &PostList{d: d, area: area, saved: saved, sort: reddit.Sort(d.Config.Display.DefaultSort), seen: map[string]bool{}, rows: 17}
	if s.sort == "" {
		s.sort = reddit.Hot
	}
	s.reqSort = s.sort
	return s
}

func (s *PostList) Init() ui.Action {
	s.d.Session.VisitArea(s.area.Subreddit)
	return s.fetch("", true, false)
}

func (s *PostList) Title() string { return "Message Area" }

func (s *PostList) Info() string {
	return fmt.Sprintf("r/%s · %s · Page %d", s.area.Subreddit, strings.ToUpper(string(s.reqSort)), s.page()+1)
}

func (s *PostList) Keys() []ui.KeyHelp {
	keys := []ui.KeyHelp{{Key: "#/⏎", Desc: "Read"}, {Key: "N", Desc: "ext"}, {Key: "P", Desc: "rev"}, {Key: "S", Desc: "ort"}, {Key: "J", Desc: "oin"}}
	if !s.saved {
		keys = append(keys, ui.KeyHelp{Key: "A", Desc: "dd area"})
	}
	if s.authFailed {
		keys = append(keys, ui.KeyHelp{Key: "L", Desc: "og in"})
	}
	return append(keys, ui.KeyHelp{Key: "O", Desc: "pen link"}, ui.KeyHelp{Key: "R", Desc: "efresh"}, ui.KeyHelp{Key: "Q", Desc: "uit"})
}

func (s *PostList) CapturesKeys() bool { return s.join != nil }

func (s *PostList) Prompt() widgets.Prompt {
	if s.join != nil {
		return widgets.Prompt{Label: "Join area:", Input: s.join.Display(), Cursor: true, CursorPos: s.join.Cursor}
	}
	return widgets.Prompt{Input: s.num.Digits, Status: s.status, Error: s.statusErr}
}

func (s *PostList) page() int { return s.cursor / s.rows }

// rowsOnPage is how many posts the current page shows.
func (s *PostList) rowsOnPage() int {
	n := len(s.posts) - s.page()*s.rows
	if n > s.rows {
		n = s.rows
	}
	if n < 0 {
		n = 0
	}
	return n
}

func (s *PostList) setRows(rows int) {
	if rows < 1 {
		rows = 1
	}
	s.rows = rows
	s.clamp()
}

func (s *PostList) clamp() {
	if s.cursor >= len(s.posts) {
		s.cursor = len(s.posts) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// fetch starts a listing request. replace discards loaded posts on arrival
// and uses the requested sort; a paging fetch uses the committed sort.
func (s *PostList) fetch(after string, replace, fresh bool) ui.Action {
	s.gen++
	gen := s.gen
	s.loading = true
	s.status, s.statusErr = "Retrieving...", false
	sort := s.sort
	if replace {
		s.pendingAdvance, s.pendingMove = false, 0
		sort = s.reqSort
	}
	store, sub := s.d.Store, s.area.Subreddit
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		l, err := store.Posts(ctx, sub, sort, after, reddit.Fetch{Fresh: fresh})
		return postsMsg{gen: gen, sort: sort, after: after, listing: l, err: err, replace: replace}
	}}
}

func (s *PostList) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case postsMsg:
		if m.gen != s.gen {
			return nil
		}
		s.loading = false
		advance, owed := s.pendingAdvance, s.pendingMove
		s.pendingAdvance, s.pendingMove = false, 0
		if m.err != nil {
			s.reqSort = s.sort // a failed sort change leaves the old sort in force
			s.keepID = ""
			s.authFailed = isAuthError(m.err)
			s.status, s.statusErr = errText(m.err), true
			return nil
		}
		s.authFailed = false
		if m.replace {
			s.posts, s.seen, s.cursor, s.ended, s.after = nil, map[string]bool{}, 0, false, ""
			s.sort = m.sort
		}
		wasPage := s.page()
		for _, p := range m.listing.Posts {
			if !s.seen[p.ID] {
				s.seen[p.ID] = true
				s.posts = append(s.posts, p)
			}
		}
		if m.listing.After == "" || m.listing.After == m.after {
			s.ended = true
		}
		s.after = m.listing.After
		s.status, s.statusErr = "", false
		if len(s.posts) == 0 {
			s.status = "No messages"
		}
		if m.replace && s.keepID != "" {
			for i, p := range s.posts {
				if p.ID == s.keepID {
					s.cursor = i
				}
			}
		}
		s.keepID = ""
		if advance {
			if next := (wasPage + 1) * s.rows; next < len(s.posts) {
				s.cursor = next
			} else if s.ended {
				s.status = "End of messages"
			}
		}
		if owed > 0 {
			s.cursor += owed
			if s.cursor >= len(s.posts) && s.ended {
				s.status = "End of messages"
			}
		}
		s.clamp()
	case ui.ErrMsg:
		s.loading = false
		s.pendingAdvance, s.pendingMove = false, 0
		s.reqSort = s.sort
		s.status, s.statusErr = errText(m.Err), true
	case linkExit:
		if m.Err != nil {
			s.status, s.statusErr = "Browser exited with an error. URL: "+m.URL, true
		}
	case ui.Resize:
		s.setRows(m.H - widgets.ChromeRows - 2)
	case ui.PopResult:
		s.status, s.statusErr = "", false
		if s.authFailed && s.d.Config.HasCredentials() {
			s.authFailed = false
			return s.fetch("", true, true)
		}
	case ui.RateLimited:
		s.status, s.statusErr = fmt.Sprintf("Rate limited, retrying in %ds", int(m.Wait.Seconds())), false
	}
	return nil
}

func (s *PostList) Draw(c term.Canvas) {
	w, h := c.Size()
	if h-2 != s.rows {
		s.setRows(h - 2)
	}
	if len(s.posts) == 0 {
		if !s.loading {
			widgets.Centre(c, h/2, "No messages in r/"+s.area.Subreddit, theme.Style(theme.Meta))
		}
		return
	}
	numW := len(itoa(s.rows))
	if numW < 2 {
		numW = 2
	}
	fromW, msgsW := 14, 5
	wide := w >= 100
	scoreW, ageW := 0, 0
	fixed := 2 + numW + 2 + 2 + fromW + 2 + msgsW + 1
	if wide {
		scoreW, ageW = 6, 4
		fixed += 2 + scoreW + 2 + ageW
	}
	subjW := w - fixed
	if subjW < 10 {
		subjW = 10
	}

	hd := theme.Style(theme.Heading)
	x := 2
	x += c.Text(x, 0, textfmt.PadLeft("#", numW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadRight("Subject", subjW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadRight("From", fromW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadLeft("Msgs", msgsW), hd, w)
	if wide {
		c.Text(x, 0, "  "+textfmt.PadLeft("Score", scoreW)+"  "+textfmt.PadLeft("Age", ageW), hd, w)
	}
	widgets.Rule(c, 1)

	start := s.page() * s.rows
	for i := start; i < start+s.rows && i < len(s.posts); i++ {
		p := s.posts[i]
		y := 2 + i - start
		sel := i == s.cursor
		st := func(r theme.Role) term.Style {
			if sel {
				return theme.Style(theme.Cursor)
			}
			return theme.Style(r)
		}
		if sel {
			c.Fill(0, y, w, 1, ' ', theme.Style(theme.Cursor))
		}
		x := 2
		x += c.Text(x, y, textfmt.PadLeft(itoa(i-start+1), numW)+"  ", st(theme.Meta), w)
		sx := x
		if p.Stickied {
			sx += c.Text(sx, y, "* ", st(theme.Sticky), w)
		}
		if p.Over18 {
			sx += c.Text(sx, y, "[X] ", st(theme.NSFW), w)
		}
		avail := x + subjW - sx
		domain := ""
		if !p.IsSelf && p.Domain != "" {
			domain = " (" + p.Domain + ")"
		}
		if textfmt.Width(p.Title)+textfmt.Width(domain) > avail {
			domain = ""
		}
		sx += c.Text(sx, y, textfmt.Truncate(p.Title, avail), st(theme.Subject), avail)
		if domain != "" {
			c.Text(sx, y, domain, st(theme.Meta), x+subjW-sx)
		}
		x += subjW + 2
		x += c.Text(x, y, textfmt.PadRight(p.Author, fromW)+"  ", st(theme.Author), w)
		x += c.Text(x, y, textfmt.PadLeft(itoa(p.NumComments), msgsW), st(theme.Body), w)
		if wide {
			c.Text(x, y, "  "+textfmt.PadLeft(textfmt.Score(p.Score), scoreW)+"  "+textfmt.PadLeft(textfmt.RelTime(p.Created, s.d.now()), ageW), st(theme.Meta), w)
		}
	}
}

func (s *PostList) HandleKey(k term.Key) ui.Action {
	if s.join != nil {
		switch s.join.HandleKey(k) {
		case widgets.InputSubmit:
			name, ok := validSubreddit(s.join.Value)
			s.join = nil
			if !ok {
				s.status, s.statusErr = "Invalid area name", true
				return nil
			}
			return ui.Replace{Screen: NewPostList(s.d, config.Area{Name: "r/" + name, Subreddit: name}, s.d.Config.HasArea(name))}
		case widgets.InputCancel:
			s.join = nil
		}
		return nil
	}
	if k.Paste {
		return nil
	}
	if v, submitted, handled := s.num.HandleKey(k); handled {
		if submitted {
			if v < 1 || v > s.rowsOnPage() {
				s.status, s.statusErr = "No such message", true
				return nil
			}
			return s.open(s.page()*s.rows + v - 1)
		}
		return nil
	}
	switch k.Code {
	case term.KeyUp:
		return s.move(-1)
	case term.KeyDown:
		return s.move(1)
	case term.KeyPgUp:
		return s.move(-s.rows)
	case term.KeyPgDn:
		return s.move(s.rows)
	case term.KeyHome:
		s.cursor = 0
		return nil
	case term.KeyEnd:
		s.cursor = len(s.posts) - 1
		s.clamp()
		return nil
	case term.KeyEnter:
		return s.open(s.cursor)
	}
	switch {
	case Rune(k) == 'N':
		return s.nextPage()
	case Rune(k) == 'P':
		if s.page() > 0 {
			s.cursor = (s.page() - 1) * s.rows
		}
	case Rune(k) == 'S':
		s.reqSort = s.reqSort.Next()
		return s.fetch("", true, false)
	case Rune(k) == 'J':
		s.join = &widgets.TextInput{}
	case Rune(k) == 'A':
		s.addArea()
	case Rune(k) == 'L':
		if s.authFailed {
			return ui.Push{Screen: NewNestedSetup(s.d)}
		}
	case Rune(k) == 'O':
		return s.openLink()
	case Rune(k) == 'R':
		if len(s.posts) > 0 {
			s.keepID = s.posts[s.cursor].ID
		}
		return s.fetch("", true, true)
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

// move shifts the cursor, fetching the next Reddit page when it runs off the
// end of what is loaded.
func (s *PostList) move(delta int) ui.Action {
	if len(s.posts) == 0 {
		return nil
	}
	target := s.cursor + delta
	if target >= len(s.posts) {
		owed := target - (len(s.posts) - 1)
		s.cursor = len(s.posts) - 1
		switch {
		case s.ended:
			s.status, s.statusErr = "End of messages", false
		case !s.loading:
			s.pendingMove = owed
			return s.fetch(s.after, false, false)
		}
		return nil
	}
	if target < 0 {
		target = 0
	}
	s.cursor = target
	return nil
}

func (s *PostList) nextPage() ui.Action {
	if next := (s.page() + 1) * s.rows; next < len(s.posts) {
		s.cursor = next
		return nil
	}
	if s.ended {
		s.status, s.statusErr = "End of messages", false
		return nil
	}
	if s.loading {
		return nil
	}
	s.pendingAdvance = true
	return s.fetch(s.after, false, false)
}

func (s *PostList) open(i int) ui.Action {
	if i < 0 || i >= len(s.posts) {
		if len(s.posts) > 0 {
			s.status, s.statusErr = "No such message", true
		}
		return nil
	}
	s.cursor = i
	s.status = ""
	s.d.Session.ThreadsOpened++
	return ui.Push{Screen: newThreadIndex(s.d, s.posts[i])}
}

// addArea adds the area to the config and saves; a failed save can be retried.
func (s *PostList) addArea() {
	if s.saved {
		return
	}
	s.d.Config.AddArea(s.area)
	if err := s.d.Config.Save(); err != nil {
		s.status, s.statusErr = "Could not save config: "+errText(err), true
		return
	}
	s.saved = true
	s.status, s.statusErr = "Area saved", false
}

func (s *PostList) openLink() ui.Action {
	if len(s.posts) == 0 {
		return nil
	}
	p := s.posts[s.cursor]
	u := p.URL
	if p.IsSelf || u == "" {
		u = "https://www.reddit.com" + p.Permalink
	}
	act, err := s.d.openInBrowser(u)
	if err != nil {
		s.status, s.statusErr = "Could not open browser. URL: "+u, true
		return nil
	}
	s.status, s.statusErr = "Opened in browser", false
	return act
}
