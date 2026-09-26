package session

import (
	"testing"
	"time"
)

func TestSessionCounters(t *testing.T) {
	start := time.Date(2026, 9, 26, 20, 0, 0, 0, time.UTC)
	s := New(start)
	s.VisitArea("linux")
	s.VisitArea("LINUX")
	s.VisitArea("rust")
	if s.AreasVisited() != 2 {
		t.Errorf("areas = %d", s.AreasVisited())
	}
	s.ThreadsOpened++
	s.MessagesRead += 3
	if d := s.Online(start.Add(90 * time.Second)); d != 90*time.Second {
		t.Errorf("online = %v", d)
	}
}
