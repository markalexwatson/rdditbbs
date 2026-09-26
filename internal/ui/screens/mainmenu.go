package screens

import (
	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newPostList is indirected so this task compiles before Post List exists.
var newPostList = func(d *Deps, area config.Area, saved bool) ui.Screen { return NewPostList(d, area, saved) }

// MainMenu is the top-level menu.
type MainMenu struct {
	d         *Deps
	confirm   bool
	join      *widgets.TextInput
	status    string
	statusErr bool
}

// NewMainMenu creates the main menu.
func NewMainMenu(d *Deps) *MainMenu { return &MainMenu{d: d} }

func (m *MainMenu) Init() ui.Action { return nil }
func (m *MainMenu) Title() string   { return "Main Menu" }
func (m *MainMenu) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{Key: "M", Desc: "essage areas"}, {Key: "J", Desc: "oin area"}, {Key: "?", Desc: "Help"}, {Key: "G", Desc: "oodbye"}}
}
func (m *MainMenu) CapturesKeys() bool { return m.confirm || m.join != nil }

func (m *MainMenu) Prompt() widgets.Prompt {
	switch {
	case m.confirm:
		return widgets.Prompt{Label: "Log off? (y/N)"}
	case m.join != nil:
		return widgets.Prompt{Label: "Join area:", Input: m.join.Display(), Cursor: true, CursorPos: m.join.Cursor}
	}
	return widgets.Prompt{Status: m.status, Error: m.statusErr}
}

func (m *MainMenu) Update(msg ui.Msg) ui.Action {
	if _, ok := msg.(ui.PopResult); ok {
		m.status, m.statusErr = "", false
	}
	return nil
}

var menuItems = []struct{ key, name, desc string }{
	{"M", "Message areas", "browse your configured subreddits"},
	{"J", "Join area", "type any subreddit name"},
	{"?", "Help", "list the keys on any screen"},
	{"G", "Goodbye", "log off"},
}

func (m *MainMenu) Draw(c term.Canvas) {
	w, _ := c.Size()
	c.Text(2, 1, "Welcome to the board. Choose a command:", theme.Style(theme.Subject), w)
	for i, it := range menuItems {
		y := 3 + i*2
		x := 4
		x += c.Text(x, y, "[", theme.Style(theme.Meta), w)
		x += c.Text(x, y, it.key, theme.Style(theme.Hotkey), w)
		x += c.Text(x, y, "] ", theme.Style(theme.Meta), w)
		x += c.Text(x, y, it.name, theme.Style(theme.Subject), w)
		c.Text(24, y, it.desc, theme.Style(theme.Meta), w-24)
	}
	c.Text(2, 3+len(menuItems)*2+1, "Areas configured: "+itoa(len(m.d.Config.Areas)), theme.Style(theme.Meta), w)
}

func (m *MainMenu) HandleKey(k term.Key) ui.Action {
	if m.confirm {
		m.confirm = false
		if Rune(k) == 'Y' {
			return ui.Push{Screen: NewGoodbye(m.d)}
		}
		return nil
	}
	if m.join != nil {
		switch m.join.HandleKey(k) {
		case widgets.InputSubmit:
			name, ok := validSubreddit(m.join.Value)
			m.join = nil
			if !ok {
				m.status, m.statusErr = "Invalid area name", true
				return nil
			}
			m.status = ""
			return ui.Push{Screen: newPostList(m.d, config.Area{Name: "r/" + name, Subreddit: name}, m.d.Config.HasArea(name))}
		case widgets.InputCancel:
			m.join = nil
		}
		return nil
	}
	if k.Paste {
		return nil // pasted text outside a field is not a command
	}
	switch {
	case Rune(k) == 'M':
		return ui.Push{Screen: NewAreaList(m.d)}
	case Rune(k) == 'J':
		m.join = &widgets.TextInput{}
		m.status = ""
	case Rune(k) == 'G', IsBack(k):
		m.confirm = true
	}
	return nil
}
