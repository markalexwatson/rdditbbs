// Package session tracks what happened during one call, for the goodbye screen.
package session

import (
	"strings"
	"time"
)

// Session holds counters for the current run.
type Session struct {
	Start         time.Time
	ThreadsOpened int
	MessagesRead  int
	LinksOpened   int

	areas map[string]bool
}

// New starts a session at the given time.
func New(start time.Time) *Session {
	return &Session{Start: start, areas: map[string]bool{}}
}

// VisitArea records a subreddit visit, ignoring case.
func (s *Session) VisitArea(subreddit string) { s.areas[strings.ToLower(subreddit)] = true }

// AreasVisited is the number of distinct subreddits visited.
func (s *Session) AreasVisited() int { return len(s.areas) }

// Online is the time since Start.
func (s *Session) Online(now time.Time) time.Duration { return now.Sub(s.Start) }
