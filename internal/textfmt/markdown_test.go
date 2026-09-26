package textfmt

import (
	"reflect"
	"strings"
	"testing"
)

func texts(d Doc) []string {
	out := make([]string, len(d.Lines))
	for i, l := range d.Lines {
		out[i] = LineText(l)
	}
	return out
}

func kinds(l Line) []Kind {
	out := make([]Kind, len(l))
	for i, s := range l {
		out[i] = s.Kind
	}
	return out
}

func TestRenderParagraphsAndSoftBreaks(t *testing.T) {
	d := Render("first line\nsame para\n\nsecond para", 40)
	want := []string{"first line same para", "", "second para"}
	if got := texts(d); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRenderWrapsAtWidth(t *testing.T) {
	d := Render("the quick brown fox jumps over the lazy dog", 12)
	for _, l := range d.Lines {
		if Width(LineText(l)) > 12 {
			t.Errorf("line too wide: %q", LineText(l))
		}
	}
	if len(d.Lines) < 3 {
		t.Errorf("expected several lines, got %d", len(d.Lines))
	}
}

func TestRenderQuote(t *testing.T) {
	d := Render("> quoted text\n> continues\n\nreply", 40)
	if got := texts(d); !reflect.DeepEqual(got, []string{"> quoted text continues", "", "reply"}) {
		t.Errorf("got %q", got)
	}
	for _, s := range d.Lines[0] {
		if s.Kind != Quote {
			t.Errorf("quote span kind = %v", s.Kind)
		}
	}
	if d.Lines[2][0].Kind != Text {
		t.Errorf("reply kind = %v", d.Lines[2][0].Kind)
	}
}

func TestRenderQuoteWrapPrefixesEveryLine(t *testing.T) {
	d := Render("> one two three four five six", 12)
	if len(d.Lines) < 2 {
		t.Fatalf("expected wrapped quote, got %q", texts(d))
	}
	for _, l := range d.Lines {
		if !strings.HasPrefix(LineText(l), "> ") {
			t.Errorf("quote line missing prefix: %q", LineText(l))
		}
	}
}

func TestRenderFencedCode(t *testing.T) {
	d := Render("before\n\n```\nfor i := range x {\n\tgo(i)\n}\n```\n\nafter", 40)
	want := []string{"before", "", "  for i := range x {", "      go(i)", "  }", "", "after"}
	if got := texts(d); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
	if d.Lines[2][0].Kind != Code {
		t.Errorf("code kind = %v", d.Lines[2][0].Kind)
	}
}

func TestRenderIndentedCodeIsClippedNotWrapped(t *testing.T) {
	d := Render("\n    "+strings.Repeat("x", 60), 20)
	if got := texts(d); len(got) != 1 || Width(got[0]) != 20 {
		t.Errorf("got %q", got)
	}
}

func TestRenderBold(t *testing.T) {
	d := Render("this is **very** important and __also__ this", 80)
	if got := kinds(d.Lines[0]); !reflect.DeepEqual(got, []Kind{Text, Bold, Text, Bold, Text}) {
		t.Errorf("kinds = %v (%q)", got, texts(d))
	}
	if LineText(d.Lines[0]) != "this is very important and also this" {
		t.Errorf("text = %q", LineText(d.Lines[0]))
	}
}

func TestRenderLinks(t *testing.T) {
	d := Render("see [the docs](https://example.com/a) and /r/linux and https://bare.example/x", 80)
	if got := LineText(d.Lines[0]); got != "see the docs[1] and /r/linux and https://bare.example/x" {
		t.Errorf("text = %q", got)
	}
	if !reflect.DeepEqual(d.Links, []string{"https://example.com/a", "https://bare.example/x"}) {
		t.Errorf("links = %q", d.Links)
	}
	if d.Lines[0][1].Kind != Link || d.Lines[0][1].Text != "the docs[1]" {
		t.Errorf("link span = %+v", d.Lines[0][1])
	}
}

func TestRenderRelativeLinkResolved(t *testing.T) {
	d := Render("[sub](/r/linux/comments/abc)", 80)
	if !reflect.DeepEqual(d.Links, []string{"https://www.reddit.com/r/linux/comments/abc"}) {
		t.Errorf("links = %q", d.Links)
	}
}

func TestRenderDuplicateURLNumberedOnce(t *testing.T) {
	d := Render("[a](https://x.example) and [b](https://x.example)", 80)
	if got := LineText(d.Lines[0]); got != "a[1] and b[1]" {
		t.Errorf("text = %q", got)
	}
	if len(d.Links) != 1 {
		t.Errorf("links = %q", d.Links)
	}
}

func TestRenderStripsEscapes(t *testing.T) {
	d := Render("safe \x1b[31mred\x1b[0m text\x07 here", 80)
	if got := LineText(d.Lines[0]); got != "safe red text here" {
		t.Errorf("text = %q", got)
	}
	d = Render("title \x1b]0;evil\x07set \x1b]2;x\x1b\\done \u0085 c1", 80)
	if got := LineText(d.Lines[0]); got != "title set done  c1" {
		t.Errorf("OSC/C1 text = %q", got)
	}
	if d := Render("日本", 1); len(d.Lines) != 1 || LineText(d.Lines[0]) != "日本" {
		t.Errorf("tiny width = %q", texts(d))
	}
}

func TestRenderHardBreaksLongURL(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("abc/", 70)
	d := Render(long, 40)
	for _, l := range d.Lines {
		if Width(LineText(l)) > 40 {
			t.Errorf("line too wide: %d", Width(LineText(l)))
		}
	}
	if len(d.Links) != 1 {
		t.Errorf("links = %d", len(d.Links))
	}
}

func TestRenderTabsAndCRLF(t *testing.T) {
	d := Render("a\tb\r\nc", 80)
	if got := LineText(d.Lines[0]); got != "a    b c" {
		t.Errorf("text = %q", got)
	}
}

func TestRenderListMarkersKept(t *testing.T) {
	d := Render("* one\n* two\n\n1. first", 80)
	if got := texts(d); !reflect.DeepEqual(got, []string{"* one", "* two", "", "1. first"}) {
		t.Errorf("got %q", got)
	}
}

func TestRenderEmpty(t *testing.T) {
	if d := Render("", 80); len(d.Lines) != 0 {
		t.Errorf("lines = %d", len(d.Lines))
	}
	if d := Render("   \n\n  ", 80); len(d.Lines) != 0 {
		t.Errorf("whitespace lines = %d", len(d.Lines))
	}
}

func TestFirstLine(t *testing.T) {
	if got := FirstLine("\n\n  hello   there\nmore"); got != "hello there" {
		t.Errorf("FirstLine = %q", got)
	}
	if got := FirstLine("**bold** start"); got != "bold start" {
		t.Errorf("FirstLine bold = %q", got)
	}
}
