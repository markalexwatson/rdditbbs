package screens

import "github.com/markwatson/redditbbs/internal/ui"

// NewSetup is replaced by the real New User Setup screen in Task 17.
func NewSetup(d *Deps) ui.Screen { return &placeholder{name: "setup"} }
