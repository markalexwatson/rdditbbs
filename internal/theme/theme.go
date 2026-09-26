// Package theme maps semantic roles to terminal styles. It is the only place
// colours are named. Scheme A: cyan frames, yellow headings, white subjects,
// green authors, grey chrome, black-on-white cursor row.
package theme

import "github.com/markwatson/redditbbs/internal/term"

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
)

var styles = map[Role]term.Style{
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
}

// Style returns the style for a role.
func Style(r Role) term.Style { return styles[r] }
