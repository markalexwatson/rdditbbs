package screens

import (
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/textfmt"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui"
	"github.com/markalexwatson/rdditbbs/internal/ui/widgets"
)

// AreaList shows the configured subreddits.
type AreaList struct {
	d         *Deps
	table     widgets.Table
	num       widgets.NumInput
	confirm   bool
	status    string
	statusErr bool
}

// NewAreaList creates the area list.
func NewAreaList(d *Deps) *AreaList {
	a := &AreaList{d: d}
	a.table.SetHeight(17)
	a.table.SetCount(len(d.Config.Areas))
	return a
}

func (a *AreaList) Init() ui.Action { return nil }
func (a *AreaList) Title() string   { return "Message Areas" }
func (a *AreaList) Info() string    { return itoa(len(a.d.Config.Areas)) + " areas" }
func (a *AreaList) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{Key: "#/⏎", Desc: "Open"}, {Key: "D", Desc: "elete"}, {Key: "Q", Desc: "uit"}}
}
func (a *AreaList) CapturesKeys() bool { return a.confirm }

func (a *AreaList) Prompt() widgets.Prompt {
	if a.confirm {
		return widgets.Prompt{Label: "Delete " + a.d.Config.Areas[a.table.Cursor].Name + "? (y/N)"}
	}
	return widgets.Prompt{Input: a.num.Digits, Status: a.status, Error: a.statusErr}
}

func (a *AreaList) Update(msg ui.Msg) ui.Action {
	if _, ok := msg.(ui.PopResult); ok {
		a.status = ""
	}
	return nil
}

func (a *AreaList) Draw(c term.Canvas) {
	w, h := c.Size()
	areas := a.d.Config.Areas
	a.table.SetHeight(h - 2)
	a.table.SetCount(len(areas))
	if len(areas) == 0 {
		widgets.Centre(c, h/2, "No areas configured. Use [J]oin from the main menu.", theme.Style(theme.Meta))
		return
	}
	c.Text(2, 0, "  #  Area                      Subreddit", theme.Style(theme.Heading), w)
	widgets.Rule(c, 1)
	start, end := a.table.Visible()
	for i := start; i < end; i++ {
		y := 2 + i - start
		line := textfmt.PadLeft(itoa(i+1), 3) + "  " + textfmt.PadRight(areas[i].Name, 24) + "  r/" + areas[i].Subreddit
		if i == a.table.Cursor {
			c.Fill(0, y, w, 1, ' ', theme.Style(theme.Cursor))
			c.Text(2, y, line, theme.Style(theme.Cursor), w-2)
			continue
		}
		x := 2
		x += c.Text(x, y, textfmt.PadLeft(itoa(i+1), 3)+"  ", theme.Style(theme.Meta), w)
		x += c.Text(x, y, textfmt.PadRight(areas[i].Name, 24)+"  ", theme.Style(theme.Subject), w)
		c.Text(x, y, "r/"+areas[i].Subreddit, theme.Style(theme.Author), w-x)
	}
}

func (a *AreaList) HandleKey(k term.Key) ui.Action {
	areas := a.d.Config.Areas
	if a.confirm {
		a.confirm = false
		if Rune(k) == 'Y' {
			a.d.Config.RemoveArea(a.table.Cursor)
			a.table.SetCount(len(a.d.Config.Areas))
			a.d.areasChanged()
			if err := a.d.Config.Save(); err != nil {
				a.status, a.statusErr = "Could not save config: "+errText(err), true
			}
		}
		return nil
	}
	if v, submitted, handled := a.num.HandleKey(k); handled {
		if submitted {
			return a.open(v - 1)
		}
		return nil
	}
	if a.table.HandleKey(k) {
		return nil
	}
	switch {
	case k.Code == term.KeyEnter:
		return a.open(a.table.Cursor)
	case Rune(k) == 'D':
		if len(areas) > 0 {
			a.confirm = true
		}
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

func (a *AreaList) open(i int) ui.Action {
	areas := a.d.Config.Areas
	if i < 0 || i >= len(areas) {
		if len(areas) > 0 {
			a.status, a.statusErr = "No such area", true
		}
		return nil
	}
	a.status = ""
	return ui.Push{Screen: newPostList(a.d, areas[i], true)}
}
