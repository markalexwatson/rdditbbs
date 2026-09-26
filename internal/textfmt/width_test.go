package textfmt

import (
	"reflect"
	"testing"
)

func TestWidth(t *testing.T) {
	cases := map[string]int{"": 0, "hello": 5, "héllo": 5, "日本": 4, "a👍🏽b": 4, "é": 1}
	for in, want := range cases {
		if got := Width(in); got != want {
			t.Errorf("Width(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hell…"},
		{"日本語です", 5, "日本…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"a👍🏽b", 3, "a…"},
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.w); got != c.want {
			t.Errorf("Truncate(%q,%d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

func TestClip(t *testing.T) {
	if got := Clip("日本語", 3); got != "日" {
		t.Errorf("Clip = %q", got)
	}
	if got := Clip("abc", 5); got != "abc" {
		t.Errorf("Clip = %q", got)
	}
}

func TestPad(t *testing.T) {
	if got := PadRight("ab", 4); got != "ab  " {
		t.Errorf("PadRight = %q", got)
	}
	if got := PadRight("abcdef", 4); got != "abc…" {
		t.Errorf("PadRight long = %q", got)
	}
	if got := PadLeft("42", 5); got != "   42" {
		t.Errorf("PadLeft = %q", got)
	}
	if got := Width(PadRight("日本", 5)); got != 5 {
		t.Errorf("PadRight wide width = %d", got)
	}
}

func TestWrap(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want []string
	}{
		{"the quick brown fox", 10, []string{"the quick", "brown fox"}},
		{"the quick brown fox", 100, []string{"the quick brown fox"}},
		{"", 10, []string{""}},
		{"   spaced   out  ", 20, []string{"spaced out"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"x abcdefghij y", 4, []string{"x", "abcd", "efgh", "ij y"}},
		{"日本語 テスト", 6, []string{"日本語", "テスト"}},
		{"anything", 0, nil},
	}
	for _, c := range cases {
		if got := Wrap(c.in, c.w); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Wrap(%q,%d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}
