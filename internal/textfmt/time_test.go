package textfmt

import (
	"testing"
	"time"
)

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		30 * time.Second:       "now",
		5 * time.Minute:        "5m",
		3 * time.Hour:          "3h",
		47 * time.Hour:         "1d",
		10 * 24 * time.Hour:    "10d",
		100 * 24 * time.Hour:   "3mo",
		2 * 365 * 24 * time.Hour: "2y",
	}
	for ago, want := range cases {
		if got := RelTime(now.Add(-ago), now); got != want {
			t.Errorf("RelTime(-%v) = %q, want %q", ago, got, want)
		}
	}
}

func TestScore(t *testing.T) {
	cases := map[int]string{0: "0", 842: "842", -5: "-5", 1234: "1.2k", 9999: "10.0k", 12345: "12k", 210000: "210k", -1500: "-1.5k"}
	for n, want := range cases {
		if got := Score(n); got != want {
			t.Errorf("Score(%d) = %q, want %q", n, got, want)
		}
	}
}
