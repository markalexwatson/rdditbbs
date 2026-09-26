package textfmt

import (
	"fmt"
	"strconv"
	"time"
)

// RelTime formats how long ago t was relative to now, in the shortest
// customary unit: now, 5m, 3h, 2d, 4mo, 1y.
func RelTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// Score formats a vote count compactly: 842, 1.2k, 12k.
func Score(n int) string {
	a := n
	if a < 0 {
		a = -a
	}
	switch {
	case a < 1000:
		return strconv.Itoa(n)
	case a < 10000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%dk", n/1000)
	}
}
