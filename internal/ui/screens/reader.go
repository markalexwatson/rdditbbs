package screens

import (
	"fmt"

	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/textfmt"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui"
	"github.com/markalexwatson/rdditbbs/internal/ui/threadmodel"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

// Reader shows one message (the post as message 0, or a comment) full width.
// The current message is tracked by comment ID so the shared model may change
// underneath it; "" means the post.
type Reader struct {
	d     *Deps
	model *threadmodel.Model
	curID string

	doc     textfmt.Doc
	box     widgets.TextBox
	docW    int
	docBody string
	docID   string
	bodyH   int

	replies   bool
	replyList []*reddit.Comment
	linkAsk   bool
	num       widgets.NumInput
	status    string
	statusErr bool
}

// NewReader opens the reader on commentID, or on the post when it is empty.
func NewReader(d *Deps, m *threadmodel.Model, commentID string) *Reader {
	return &Reader{d: d, model: m, curID: commentID, docW: -1}
}

func (r *Reader) Init() ui.Action {
	r.d.Session.MessagesRead++
	return nil
}

func (r *Reader) Title() string { return "Read Message" }

// Info shows the current message's Thread Index row number (0 for the post)
// over the total row count, so numbers match between the two screens.
func (r *Reader) Info() string {
	n, total := 0, len(r.model.Rows())
	if r.curID != "" {
		if i := r.model.IndexOf(r.curID); i >= 0 {
			n = i + 1
		}
	}
	return fmt.Sprintf("Msg %d of %d", n, total)
}

func (r *Reader) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{Key: "N", Desc: "ext"}, {Key: "P", Desc: "rev"}, {Key: "U", Desc: "p"}, {Key: "R", Desc: "eplies"}, {Key: "T", Desc: "hread"}, {Key: "O", Desc: "pen link"}, {Key: "Spc", Desc: "Page"}, {Key: "Q", Desc: "uit"}}
}

func (r *Reader) CapturesKeys() bool { return r.replies || r.linkAsk }

func (r *Reader) Prompt() widgets.Prompt {
	switch {
	case r.replies:
		return widgets.Prompt{Label: "Reply #:", Input: r.num.Digits, Cursor: true}
	case r.linkAsk:
		return widgets.Prompt{Label: "Link #:", Input: r.num.Digits, Cursor: true}
	}
	return widgets.Prompt{Status: r.status, Error: r.statusErr}
}

func (r *Reader) Update(msg ui.Msg) ui.Action {
	if le, ok := msg.(linkExit); ok && le.Err != nil {
		r.status, r.statusErr = "Browser exited with an error. URL: "+le.URL, true
	}
	return nil
}

// pos is the 1-based position of the current comment among the visible
// comments, or 0 for the post. A comment that is no longer visible falls
// back to the post.
func (r *Reader) pos() int {
	if r.curID == "" {
		return 0
	}
	for i, c := range r.model.Comments() {
		if c.ID == r.curID {
			return i + 1
		}
	}
	r.curID = ""
	return 0
}

// current returns the comment shown, or nil for message 0.
func (r *Reader) current() *reddit.Comment {
	if p := r.pos(); p > 0 {
		return r.model.Comments()[p-1]
	}
	return nil
}

func (r *Reader) bodyText() string {
	cm := r.current()
	if cm == nil {
		p := r.model.Post()
		if p.IsSelf {
			return p.SelfText
		}
		return "Link post: " + p.URL
	}
	return cm.Body
}

// ensureDoc rebuilds the wrapped document when the width, message or body changed.
func (r *Reader) ensureDoc(w int) {
	body := r.bodyText()
	if r.docW == w && r.docBody == body && r.docID == r.curID {
		return
	}
	r.doc = textfmt.Render(body, w)
	lines := append([]textfmt.Line(nil), r.doc.Lines...)
	if len(r.doc.Links) > 0 {
		lines = append(lines, textfmt.Line{}, textfmt.Line{{Text: "Links:", Kind: textfmt.Bold}})
		for i, u := range r.doc.Links {
			for _, l := range textfmt.Wrap("["+itoa(i+1)+"] "+u, w) {
				lines = append(lines, textfmt.Line{{Text: l, Kind: textfmt.Link}})
			}
		}
	}
	r.box.Lines = lines
	r.box.Top = 0
	r.docW, r.docBody, r.docID = w, body, r.curID
}

// signed formats a score with an explicit sign for non-negative values.
func signed(n int) string {
	if n >= 0 {
		return "+" + textfmt.Score(n)
	}
	return textfmt.Score(n)
}

func (r *Reader) Draw(c term.Canvas) {
	w, h := c.Size()
	r.ensureDoc(w - 4)
	p := r.model.Post()
	cm := r.current()
	hd := theme.Style(theme.Heading)

	c.Text(2, 0, "Subj: ", hd, w)
	c.Text(8, 0, textfmt.Truncate(p.Title, w-10), theme.Style(theme.Subject), w-10)

	author, role, score, created := p.Author, theme.OP, p.Score, p.Created
	if cm != nil {
		author, role = authorAndRole(cm, p)
		score, created = cm.Score, cm.Created
	}
	x := 2
	x += c.Text(x, 1, "From: ", hd, w)
	x += c.Text(x, 1, author, theme.Style(role), w)
	c.Text(x, 1, " ("+signed(score)+")", theme.Style(theme.Meta), w-x)
	date := "Date: " + created.Local().Format("02/01/06 15:04")
	c.Text(w-2-textfmt.Width(date), 1, date, theme.Style(theme.Meta), w)

	y := 2
	if cm != nil {
		re := "  Re: original post"
		if parent := r.model.Parent(cm); parent != nil {
			re = "  Re: #" + itoa(r.indexOf(parent.ID)) + " " + parent.Author
		}
		re += fmt.Sprintf(" · depth %d · %d loaded replies", cm.Depth, len(cm.Children))
		c.Text(2, y, textfmt.Truncate(re, w-4), theme.Style(theme.Meta), w-4)
		y++
	}
	widgets.Rule(c, y)
	y++
	r.bodyH = h - y
	if r.replies {
		r.drawReplies(c, y, w)
		return
	}
	r.box.Draw(c, 2, y, w-4, r.bodyH)
	if !r.box.AtEnd(r.bodyH) {
		c.Text(w-10, h-1, "▼ more", theme.Style(theme.Meta), 8)
	}
}

// indexOf is a comment's Thread Index row number, 0 if hidden.
func (r *Reader) indexOf(id string) int { return r.model.IndexOf(id) + 1 }

func (r *Reader) drawReplies(c term.Canvas, y, w int) {
	c.Text(2, y, "Replies to this message:", theme.Style(theme.Heading), w-4)
	if len(r.replyList) == 0 {
		c.Text(4, y+2, "No loaded replies", theme.Style(theme.Meta), w-6)
		return
	}
	for i, cm := range r.replyList {
		if 2+i >= r.bodyH {
			c.Text(4, y+r.bodyH-1, "… and "+itoa(len(r.replyList)-i)+" more; type a number", theme.Style(theme.Meta), w-6)
			break
		}
		author, role := authorAndRole(cm, r.model.Post())
		x := 4
		x += c.Text(x, y+2+i, itoa(i+1)+". ", theme.Style(theme.Meta), w)
		x += c.Text(x, y+2+i, author, theme.Style(role), w)
		preview := textfmt.FirstLine(cm.Body)
		if cm.BodyRemoved {
			preview = cm.Body
		}
		c.Text(x, y+2+i, " — "+textfmt.Truncate(preview, w-x-5), theme.Style(theme.Body), w-x-2)
	}
}

func (r *Reader) HandleKey(k term.Key) ui.Action {
	if k.Paste {
		return nil
	}
	if r.replies || r.linkAsk {
		if v, submitted, handled := r.num.HandleKey(k); handled {
			if submitted {
				var act ui.Action
				if r.replies {
					r.jumpToReply(v)
				} else {
					act = r.openLink(v)
				}
				r.replies, r.linkAsk = false, false
				return act
			}
			return nil
		}
		if k.Code == term.KeyEscape || Rune(k) == 'R' || Rune(k) == 'O' {
			r.replies, r.linkAsk = false, false
			r.num.Digits = ""
		}
		return nil
	}
	switch k.Code {
	case term.KeyUp:
		r.box.Scroll(-1, r.bodyH)
	case term.KeyDown:
		r.box.Scroll(1, r.bodyH)
	case term.KeyPgUp:
		r.box.Scroll(-r.bodyH, r.bodyH)
	case term.KeyPgDn:
		r.box.Scroll(r.bodyH, r.bodyH)
	case term.KeyHome:
		r.box.Top = 0
	case term.KeyEnd:
		r.box.Scroll(len(r.box.Lines), r.bodyH)
	}
	cs := r.model.Comments()
	pos := r.pos()
	switch {
	case k.Code == term.KeyRune && k.Rune == ' ':
		r.box.Scroll(r.bodyH, r.bodyH)
	case Rune(k) == 'N':
		if pos < len(cs) {
			r.setCur(cs[pos].ID)
		} else {
			r.status, r.statusErr = "No more messages", false
		}
	case Rune(k) == 'P':
		switch {
		case pos > 1:
			r.setCur(cs[pos-2].ID)
		case pos == 1:
			r.setCur("")
		default:
			r.status, r.statusErr = "No more messages", false
		}
	case Rune(k) == 'U':
		cm := r.current()
		if cm == nil {
			r.status, r.statusErr = "Already at top", false
			break
		}
		if parent := r.model.Parent(cm); parent != nil {
			r.setCur(parent.ID)
		} else {
			r.setCur("")
		}
	case Rune(k) == 'R':
		r.replyList = r.loadedReplies()
		r.replies = true
		r.num.Digits = ""
	case Rune(k) == 'O':
		switch len(r.doc.Links) {
		case 0:
			r.status, r.statusErr = "No links in this message", false
		case 1:
			return r.openLink(1)
		default:
			r.linkAsk = true
			r.num.Digits = ""
		}
	case Rune(k) == 'T', IsBack(k):
		if cm := r.current(); cm != nil {
			return ui.Pop{Result: SelectComment{ID: cm.ID}}
		}
		return ui.Pop{}
	}
	return nil
}

// setCur moves to a message, revealing it if a collapse hid it.
func (r *Reader) setCur(id string) {
	if id != "" {
		r.model.Reveal(id)
	}
	r.curID = id
	r.status = ""
	r.d.Session.MessagesRead++
	if r.docW > 0 {
		r.ensureDoc(r.docW) // rebuild now so O sees the new message's links before the next Draw
	}
}

// loadedReplies lists every loaded direct reply of the current message.
func (r *Reader) loadedReplies() []*reddit.Comment {
	if cm := r.current(); cm != nil {
		return cm.Children
	}
	return r.model.Thread().Comments
}

func (r *Reader) jumpToReply(n int) {
	if n < 1 || n > len(r.replyList) {
		r.status, r.statusErr = "No such reply", true
		return
	}
	r.setCur(r.replyList[n-1].ID)
}

func (r *Reader) openLink(n int) ui.Action {
	if n < 1 || n > len(r.doc.Links) {
		r.status, r.statusErr = "No such link", true
		return nil
	}
	u := r.doc.Links[n-1]
	act, err := r.d.openInBrowser(u)
	if err != nil {
		r.status, r.statusErr = "Could not open browser. URL: "+u, true
		return nil
	}
	r.status, r.statusErr = "Opened in browser", false
	return act
}
