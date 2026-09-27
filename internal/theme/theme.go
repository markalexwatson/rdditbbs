// Package theme maps semantic roles to terminal styles. It is the only place
// colours are named. Several named themes are built in; the active one can be
// switched at runtime and a custom theme can be assembled from colour names.
package theme

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/markalexwatson/rdditbbs/internal/term"
)

// Role is a semantic use of colour.
type Role int

// Roles.
const (
	Frame Role = iota
	Logo
	Heading
	Subject
	Author
	OP
	Mod
	Meta
	Body
	Quote
	Code
	Bold
	Link
	Prompt
	Hotkey
	Error
	Stub
	Rule
	Cursor
	Sticky
	NSFW
	Bar // fill behind the title bar interior and the hotkey bar
)

var roleNames = []string{
	"frame", "logo", "heading", "subject", "author", "op", "mod", "meta", "body", "quote", "code",
	"bold", "link", "prompt", "hotkey", "error", "stub", "rule", "cursor", "sticky", "nsfw", "bar",
}

// RoleName is the config key for a role.
func RoleName(r Role) string {
	if int(r) < len(roleNames) {
		return roleNames[r]
	}
	return fmt.Sprintf("role%d", r)
}

// RoleByName looks a role up by its config key.
func RoleByName(name string) (Role, bool) {
	for i, n := range roleNames {
		if n == strings.ToLower(strings.TrimSpace(name)) {
			return Role(i), true
		}
	}
	return 0, false
}

// Theme is a complete set of styles.
type Theme struct {
	Name   string
	Styles map[Role]term.Style
}

// mono builds a single-hue phosphor theme from a dim and a bright shade.
func mono(name string, dim, bright term.Color) Theme {
	return Theme{Name: name, Styles: map[Role]term.Style{
		Frame:   {FG: dim},
		Logo:    {FG: bright, Bold: true},
		Heading: {FG: bright, Bold: true},
		Subject: {FG: bright},
		Author:  {FG: bright},
		OP:      {FG: bright, Bold: true},
		Mod:     {FG: bright, Bold: true},
		Meta:    {FG: dim},
		Body:    {FG: dim},
		Quote:   {FG: bright},
		Code:    {FG: dim},
		Bold:    {FG: bright, Bold: true},
		Link:    {FG: bright},
		Prompt:  {FG: bright},
		Hotkey:  {FG: bright, Bold: true},
		Error:   {FG: term.Black, BG: bright},
		Stub:    {FG: dim},
		Rule:    {FG: dim},
		Cursor:  {FG: term.Black, BG: dim},
		Sticky:  {FG: bright, Bold: true},
		NSFW:    {FG: bright, Bold: true},
		Bar:     {FG: dim},
	}}
}

var builtIns = []Theme{
	{Name: "classic", Styles: map[Role]term.Style{
		Frame:   {FG: term.Cyan},
		Logo:    {FG: term.BrightYellow, Bold: true},
		Heading: {FG: term.BrightYellow},
		Subject: {FG: term.BrightWhite},
		Author:  {FG: term.BrightGreen},
		OP:      {FG: term.BrightCyan},
		Mod:     {FG: term.BrightMagenta},
		Meta:    {FG: term.BrightBlack},
		Body:    {FG: term.White},
		Quote:   {FG: term.Cyan},
		Code:    {FG: term.BrightBlack},
		Bold:    {FG: term.BrightWhite, Bold: true},
		Link:    {FG: term.BrightCyan},
		Prompt:  {FG: term.BrightCyan},
		Hotkey:  {FG: term.BrightYellow},
		Error:   {FG: term.BrightRed},
		Stub:    {FG: term.BrightBlack},
		Rule:    {FG: term.BrightBlack},
		Cursor:  {FG: term.Black, BG: term.White},
		Sticky:  {FG: term.BrightYellow},
		NSFW:    {FG: term.BrightRed},
		Bar:     {},
	}},
	{Name: "blue", Styles: map[Role]term.Style{
		Frame:   {FG: term.Blue},
		Logo:    {FG: term.BrightYellow, Bold: true},
		Heading: {FG: term.BrightWhite},
		Subject: {FG: term.White},
		Author:  {FG: term.Cyan},
		OP:      {FG: term.BrightCyan},
		Mod:     {FG: term.BrightMagenta},
		Meta:    {FG: term.BrightBlack},
		Body:    {FG: term.White},
		Quote:   {FG: term.Cyan},
		Code:    {FG: term.BrightBlack},
		Bold:    {FG: term.BrightWhite, Bold: true},
		Link:    {FG: term.BrightCyan},
		Prompt:  {FG: term.BrightCyan},
		Hotkey:  {FG: term.BrightYellow},
		Error:   {FG: term.BrightRed},
		Stub:    {FG: term.BrightBlack},
		Rule:    {FG: term.Blue},
		Cursor:  {FG: term.BrightWhite, BG: term.Blue, Bold: true},
		Sticky:  {FG: term.BrightYellow},
		NSFW:    {FG: term.BrightRed},
		Bar:     {FG: term.BrightWhite, BG: term.Blue},
	}},
	mono("amber", term.Yellow, term.BrightYellow),
	mono("green", term.Green, term.BrightGreen),
}

var (
	mu       sync.RWMutex
	themes   = map[string]Theme{}
	order    []string
	active   Theme
	fallback = builtIns[0]
)

func init() {
	for _, t := range builtIns {
		themes[t.Name] = t
		order = append(order, t.Name)
	}
	active = fallback
}

// Names lists the available themes: the built-ins in order, then any registered ones.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	return append([]string(nil), order...)
}

// Get returns a theme by name.
func Get(name string) (Theme, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := themes[name]
	return t, ok
}

// Register adds or replaces a theme. It is appended to Names unless present.
func Register(t Theme) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := themes[t.Name]; !ok {
		order = append(order, t.Name)
	}
	themes[t.Name] = t
	if active.Name == t.Name {
		active = t
	}
}

// Unregister removes a registered theme (built-ins cannot be removed).
func Unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	for _, b := range builtIns {
		if b.Name == name {
			return
		}
	}
	delete(themes, name)
	for i, n := range order {
		if n == name {
			order = append(order[:i], order[i+1:]...)
			break
		}
	}
	if active.Name == name {
		active = fallback
	}
}

// Set makes the named theme active.
func Set(name string) error {
	mu.Lock()
	defer mu.Unlock()
	t, ok := themes[name]
	if !ok {
		return fmt.Errorf("unknown theme %q (available: %s)", name, strings.Join(order, ", "))
	}
	active = t
	return nil
}

// Current is the active theme's name.
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return active.Name
}

// Next activates the theme after the current one, wrapping, and returns its name.
func Next() string {
	mu.Lock()
	defer mu.Unlock()
	for i, n := range order {
		if n == active.Name {
			active = themes[order[(i+1)%len(order)]]
			return active.Name
		}
	}
	active = themes[order[0]]
	return active.Name
}

// Style returns the active theme's style for a role.
func Style(r Role) term.Style {
	mu.RLock()
	defer mu.RUnlock()
	return active.Styles[r]
}

// OnBar returns a role's style drawn over the Bar fill: the role's foreground
// and attributes with the bar's effective background (its foreground when the
// bar is reversed) when the role has no background of its own.
func OnBar(r Role) term.Style {
	mu.RLock()
	defer mu.RUnlock()
	st, bar := active.Styles[r], active.Styles[Bar]
	if st.BG == term.Default {
		st.BG = bar.BG
		if bar.Reverse {
			st.BG = bar.FG
		}
	}
	return st
}

var colourNames = map[string]term.Color{
	"default": term.Default,
	"black":   term.Black, "red": term.Red, "green": term.Green, "yellow": term.Yellow,
	"blue": term.Blue, "magenta": term.Magenta, "cyan": term.Cyan, "white": term.White,
	"bright black": term.BrightBlack, "grey": term.BrightBlack, "gray": term.BrightBlack,
	"bright red": term.BrightRed, "bright green": term.BrightGreen, "bright yellow": term.BrightYellow,
	"bright blue": term.BrightBlue, "bright magenta": term.BrightMagenta, "bright cyan": term.BrightCyan,
	"bright white": term.BrightWhite,
}

// ColourNames lists the accepted colour words, sorted.
func ColourNames() []string {
	out := make([]string, 0, len(colourNames))
	for n := range colourNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ParseStyle reads "<fg> [on <bg>] [bold] [reverse]" using the 16 ANSI colour
// names (with "bright" prefixes, and "grey"/"gray" for bright black).
// "default" means the terminal's own colour. The foreground may be omitted
// only when no background is given ("bold" alone is valid); a background
// always needs a foreground before "on". Attributes may follow either colour.
func ParseStyle(s string) (term.Style, error) {
	words := strings.Fields(strings.ToLower(s))
	if len(words) == 0 {
		return term.Style{}, fmt.Errorf("empty style")
	}
	var st term.Style
	// Split into the part before "on" and the part after.
	fgWords, bgWords := words, []string(nil)
	for i, w := range words {
		if w == "on" {
			fgWords, bgWords = words[:i], words[i+1:]
			if len(bgWords) == 0 {
				return term.Style{}, fmt.Errorf("%q: missing colour after \"on\"", s)
			}
			break
		}
	}
	take := func(ws []string) (term.Color, []string, bool, error) {
		// Longest match first: "bright cyan" before "bright".
		if len(ws) >= 2 {
			if c, ok := colourNames[ws[0]+" "+ws[1]]; ok {
				return c, ws[2:], true, nil
			}
		}
		if len(ws) >= 1 {
			if c, ok := colourNames[ws[0]]; ok {
				return c, ws[1:], true, nil
			}
		}
		return term.Default, ws, false, nil
	}
	fg, rest, hasFG, _ := take(fgWords)
	if hasFG {
		st.FG = fg
	}
	for _, w := range rest {
		switch w {
		case "bold":
			st.Bold = true
		case "reverse":
			st.Reverse = true
		default:
			return term.Style{}, fmt.Errorf("%q: unknown colour or attribute %q", s, w)
		}
	}
	if bgWords != nil {
		if !hasFG {
			return term.Style{}, fmt.Errorf("%q: a background needs a foreground colour before \"on\"", s)
		}
		bg, rest, ok, _ := take(bgWords)
		if !ok {
			return term.Style{}, fmt.Errorf("%q: unknown background colour %q", s, strings.Join(bgWords, " "))
		}
		st.BG = bg
		for _, w := range rest {
			switch w {
			case "bold":
				st.Bold = true
			case "reverse":
				st.Reverse = true
			default:
				return term.Style{}, fmt.Errorf("%q: unknown colour or attribute %q", s, w)
			}
		}
	}
	return st, nil
}

// Custom builds the theme named "custom" from a base theme with per-role
// overrides keyed by role name. Errors name the offending key.
func Custom(base string, overrides map[string]string) (Theme, error) {
	b, ok := Get(base)
	if !ok {
		return Theme{}, fmt.Errorf("theme base %q is not a built-in theme (available: %s)", base, strings.Join(Names(), ", "))
	}
	t := Theme{Name: "custom", Styles: map[Role]term.Style{}}
	for r, st := range b.Styles {
		t.Styles[r] = st
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r, ok := RoleByName(k)
		if !ok {
			return Theme{}, fmt.Errorf("theme.%s: unknown role (roles: %s)", k, strings.Join(roleNames, ", "))
		}
		st, err := ParseStyle(overrides[k])
		if err != nil {
			return Theme{}, fmt.Errorf("theme.%s: %v", k, err)
		}
		t.Styles[r] = st
	}
	return t, nil
}
