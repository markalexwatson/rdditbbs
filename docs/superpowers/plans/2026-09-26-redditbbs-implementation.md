# RedditBBS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go terminal Reddit reader that looks and behaves like a classic BBS: hotkey menus, cursor rows, threaded comment index with peek pane, one-message reader, read-only via Reddit app-only OAuth.

**Architecture:** A screen stack driven by an `App` loop over an abstract `term.Terminal`. Screens draw into a content sub-canvas and return navigation or async `Run` actions; results come back through one channel with per-instance cancellation and generation counters. Data comes from `reddit.Client` implementing a `Store` interface with token handling, a rate gate and a byte-level LRU cache, so every caller gets a fresh parse.

**Tech Stack:** Go 1.27, `github.com/gdamore/tcell/v2`, `github.com/rivo/uniseg`, `github.com/BurntSushi/toml`. Standard library `testing` only.

**Spec:** `docs/superpowers/specs/2026-09-26-redditbbs-design.md`

## Global Constraints

- Go 1.27; `go.mod` declares `go 1.27`. Go lives at `~/.local/go/bin`; every command below assumes `export PATH=$HOME/.local/go/bin:$PATH`.
- Module path `github.com/markwatson/redditbbs`. Single binary `cmd/redditbbs`.
- Only `internal/term` imports tcell. Only `internal/theme` names colours. Dependencies point downward per the spec table, with one addition recorded in Task 14: `internal/ui` may import `internal/ui/widgets`.
- Minimum terminal 80x24. Title bar rows 0 to 2, hotkey bar row `h-2`, prompt row `h-1`, content rows 3 to `h-3` (`h-5` rows).
- Colour scheme A: cyan frames, yellow headings, bright white subjects, green authors, grey chrome, reverse-video cursor row.
- Dates in chrome dd/mm/yy. Comment sort `best` is sent to the API as `confidence`.
- Reddit API base `https://oauth.reddit.com`; token URL `https://www.reddit.com/api/v1/access_token`; every request has `raw_json=1`, a `User-Agent` and `Authorization: bearer <token>`; HTTP timeout 15 s; body limit 10 MB; morechildren at most 100 IDs per call and one call in flight.
- Config `~/.config/redditbbs/config.toml`, mode 0600, atomic writes, environment overrides `REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET` never written to disk.
- Commit messages in British English, ending with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- TDD: write the failing test, watch it fail, implement, watch it pass, commit.

## Review Focus

1. A comment body containing an ANSI escape sequence or a 300-character unbroken URL must render without corrupting columns or overflowing the row. Pinned in Task 3 (`TestRenderStripsEscapes`, `TestRenderHardBreaksLongURL`).
2. A resize to a shorter terminal while the Post List cursor is on the last row must clamp the cursor and not index past the page. Pinned in Task 18 (`TestPostListResizeClampsCursor`).
3. A subreddit with zero posts must show "No messages" and Enter must do nothing. Pinned in Task 18 (`TestPostListEmpty`).
4. A thread whose top level is only a `more` stub, or whose comments have deleted authors with surviving bodies, must produce rows without panicking and keep the bodies. Pinned in Task 19 (`TestRowsOnlyStub`, `TestRowsDeletedAuthorKeepsBody`).
5. A config file that is valid TOML but has no `[reddit]` table, with credentials in the environment, must count as having credentials and must not write those secrets on save. Pinned in Task 11 (`TestEnvOnlyCredentialsNotSaved`).

## File Structure

| Path | Responsibility |
| --- | --- |
| `cmd/redditbbs/main.go` | Flags, config load, terminal setup, App run, panic recovery, exit codes |
| `internal/term/style.go` | `Color`, `Style` |
| `internal/term/key.go` | `Key`, `KeyCode`, `Resize`, `Event` |
| `internal/term/canvas.go` | `Canvas`, `Terminal` interfaces, `Sub` clipping canvas |
| `internal/term/tcell.go` | tcell-backed `Terminal`, event translation, paste tracking |
| `internal/term/sim.go` | `Sim` test terminal with own cell grid and event injection |
| `internal/theme/theme.go` | `Role` and `Style(role)` |
| `internal/textfmt/width.go` | `Width`, `Truncate`, `Clip`, `PadRight`, `PadLeft`, `Wrap` |
| `internal/textfmt/time.go` | `RelTime`, `Score` |
| `internal/textfmt/markdown.go` | `Kind`, `Span`, `Line`, `Doc`, `Render` |
| `internal/reddit/models.go` | `Post`, `Comment`, `MoreStub`, `Listing`, `Thread`, `Things`, `Sort`, `CommentSort`, `Fetch`, `Store` |
| `internal/reddit/parse.go` | `ParseListing`, `ParseThread`, `ParseMoreChildren` |
| `internal/reddit/attach.go` | `Attach` |
| `internal/reddit/token.go` | `TokenSource` |
| `internal/reddit/ratelimit.go` | `RateGate` |
| `internal/reddit/cache.go` | `Cache` |
| `internal/reddit/client.go` | `Client`, `APIError`, options |
| `internal/reddit/redditest/fake.go` | `FakeStore` and fixture builders for UI tests |
| `internal/config/config.go` | `Config`, `Area`, `Load`, `Save`, `DefaultPath` |
| `internal/session/session.go` | `Session` counters |
| `internal/browser/browser.go` | `Open` |
| `internal/ui/action.go` | `Action`, `Msg` types, `Screen` and optional interfaces |
| `internal/ui/app.go` | `App` loop, stack, Run binding, delivery, drawing, key precedence |
| `internal/ui/help.go` | Help overlay |
| `internal/ui/widgets/chrome.go` | `TitleBar`, `HotkeyBar`, `PromptLine`, `Prompt`, `KeyHelp` |
| `internal/ui/widgets/table.go` | `Table` cursor and scrolling state |
| `internal/ui/widgets/input.go` | `NumInput`, `TextInput` |
| `internal/ui/widgets/textbox.go` | `TextBox`, `DrawLine` |
| `internal/ui/widgets/tree.go` | `Connector` |
| `internal/ui/threadmodel/model.go` | `Model`, `Row`: visible order, numbering, collapse, stub splicing |
| `internal/ui/screens/deps.go` | `Deps`, `Rune` helper, confirm prompt helper |
| `internal/ui/screens/splash.go`, `goodbye.go`, `mainmenu.go`, `arealist.go`, `setup.go`, `postlist.go`, `threadindex.go`, `reader.go` | One screen each |
| `internal/ui/screens/smoke_test.go` | End-to-end key script through every screen |

---

### Task 1: Module scaffold

**Files:**
- Create: `go.mod`, `cmd/redditbbs/main.go`, `Makefile`, `README.md`

**Interfaces:**
- Produces: module path `github.com/markwatson/redditbbs`; `make test` runs `go test ./...`.

- [ ] **Step 1: Create go.mod and fetch dependencies**

```bash
export PATH=$HOME/.local/go/bin:$PATH
cd /home/markwatson/Projects/RedditBBS
go mod init github.com/markwatson/redditbbs
go get github.com/gdamore/tcell/v2@latest github.com/rivo/uniseg@latest github.com/BurntSushi/toml@latest
```

Expected: `go.mod` exists with `go 1.27` and the three requires.

- [ ] **Step 2: Write a main that builds**

`cmd/redditbbs/main.go`:

```go
// Command redditbbs is a BBS-style terminal reader for Reddit.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags "-X main.Version=…".
var Version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("redditbbs", Version)
		return
	}
	fmt.Fprintln(os.Stderr, "redditbbs: not yet wired up")
	os.Exit(1)
}
```

- [ ] **Step 3: Write the Makefile and README**

`Makefile`:

```make
GO ?= $(HOME)/.local/go/bin/go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet run clean

build:
	$(GO) build -ldflags "-X main.Version=$(VERSION)" -o bin/redditbbs ./cmd/redditbbs

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

run: build
	./bin/redditbbs

clean:
	rm -rf bin
```

`README.md`:

```markdown
# RedditBBS

A terminal Reddit reader that feels like a 1990s bulletin board system.

Read-only. Requires a free Reddit "script" app: register one at
https://www.reddit.com/prefs/apps and enter the client ID and secret on
first run. Reddit may need to approve the app before requests succeed.

Build with `make build`, run with `./bin/redditbbs`. Design notes live in
`docs/superpowers/specs/`.
```

- [ ] **Step 4: Verify it builds and vets**

Run: `make build vet && ./bin/redditbbs --version`
Expected: prints `redditbbs <version>`; no vet output.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum cmd Makefile README.md
git commit -m "Scaffold Go module and build targets

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: textfmt width, wrap and formatting helpers

**Files:**
- Create: `internal/textfmt/width.go`, `internal/textfmt/time.go`
- Test: `internal/textfmt/width_test.go`, `internal/textfmt/time_test.go`

**Interfaces:**
- Produces:
  - `func Width(s string) int` cells, grapheme-aware.
  - `func Truncate(s string, w int) string` fits in `w`, appends `…` when cut, `""` for `w <= 0`.
  - `func Clip(s string, w int) string` longest cluster prefix fitting `w`, no ellipsis.
  - `func PadRight(s string, w int) string`, `func PadLeft(s string, w int) string` exactly `w` cells.
  - `func Wrap(s string, w int) []string` lines of at most `w` cells; `[""]` for empty input; `nil` for `w <= 0`.
  - `func RelTime(t, now time.Time) string` such as `now`, `5m`, `3h`, `2d`, `4mo`, `1y`.
  - `func Score(n int) string` such as `842`, `1.2k`, `12k`, `-5`.

- [ ] **Step 1: Write the failing width tests**

`internal/textfmt/width_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/textfmt/`
Expected: FAIL, undefined: Width, Truncate, Clip, PadRight, PadLeft, Wrap.

- [ ] **Step 3: Implement width.go**

```go
// Package textfmt measures, wraps and formats text for a cell-based
// terminal, treating grapheme clusters as indivisible.
package textfmt

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Width returns the number of terminal cells s occupies.
func Width(s string) int { return uniseg.StringWidth(s) }

// Clip returns the longest prefix of whole grapheme clusters that fits in w cells.
func Clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cw := g.Width()
		if used+cw > w {
			break
		}
		b.WriteString(g.Str())
		used += cw
	}
	return b.String()
}

// Truncate returns s unchanged if it fits in w cells, otherwise the longest
// cluster prefix that fits in w-1 cells followed by an ellipsis.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return Clip(s, w-1) + "…"
}

// PadRight returns s truncated or space-padded to exactly w cells.
func PadRight(s string, w int) string {
	s = Truncate(s, w)
	return s + strings.Repeat(" ", w-Width(s))
}

// PadLeft returns s truncated or left-padded with spaces to exactly w cells.
func PadLeft(s string, w int) string {
	s = Truncate(s, w)
	return strings.Repeat(" ", w-Width(s)) + s
}

// Wrap splits s into lines of at most w cells, breaking on whitespace and
// hard-breaking words wider than w by cluster. Empty input yields one empty
// line. w <= 0 yields nil.
func Wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curW = 0
	}
	for _, word := range strings.Fields(s) {
		ww := Width(word)
		if ww > w {
			if curW > 0 {
				flush()
			}
			g := uniseg.NewGraphemes(word)
			for g.Next() {
				cw := g.Width()
				if curW+cw > w {
					flush()
				}
				cur.WriteString(g.Str())
				curW += cw
			}
			continue
		}
		switch {
		case curW == 0:
			cur.WriteString(word)
			curW = ww
		case curW+1+ww > w:
			flush()
			cur.WriteString(word)
			curW = ww
		default:
			cur.WriteByte(' ')
			cur.WriteString(word)
			curW += 1 + ww
		}
	}
	if curW > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}
```

- [ ] **Step 4: Run width tests**

Run: `go test ./internal/textfmt/ -run 'TestWidth|TestTruncate|TestClip|TestPad|TestWrap' -v`
Expected: PASS. If `Width("a👍🏽b")` disagrees with 4, check the uniseg version; the skin-tone emoji must be one 2-cell cluster.

- [ ] **Step 5: Write the failing time tests**

`internal/textfmt/time_test.go`:

```go
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
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/textfmt/ -run 'TestRelTime|TestScore'`
Expected: FAIL, undefined: RelTime, Score.

- [ ] **Step 7: Implement time.go**

```go
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
```

- [ ] **Step 8: Run all textfmt tests**

Run: `go test ./internal/textfmt/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/textfmt
git commit -m "Add textfmt width, wrap and formatting helpers

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: textfmt Markdown rendering

**Files:**
- Create: `internal/textfmt/markdown.go`
- Test: `internal/textfmt/markdown_test.go`

**Interfaces:**
- Consumes: `Width`, `Clip`, `Wrap` from Task 2.
- Produces:
  - `type Kind int` with constants `Text, Quote, Code, Bold, Link`.
  - `type Span struct { Text string; Kind Kind }`, `type Line []Span`.
  - `type Doc struct { Lines []Line; Links []string }`.
  - `func Render(md string, w int) Doc` wrapped to `w` cells.
  - `func LineText(l Line) string` concatenated text, used by tests and previews.
  - `func FirstLine(md string) string` first non-empty sanitised line with whitespace collapsed, for previews.

- [ ] **Step 1: Write the failing tests**

`internal/textfmt/markdown_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/textfmt/ -run Render`
Expected: FAIL, undefined: Render, LineText, FirstLine, Kind.

- [ ] **Step 3: Implement markdown.go**

```go
package textfmt

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

// Kind classifies a span for styling.
type Kind int

// Span kinds.
const (
	Text Kind = iota
	Quote
	Code
	Bold
	Link
)

// Span is a run of text with one style kind.
type Span struct {
	Text string
	Kind Kind
}

// Line is one terminal row of spans.
type Line []Span

// Doc is a rendered body: wrapped lines and the URLs referenced by [n] markers.
type Doc struct {
	Lines []Line
	Links []string
}

// LineText concatenates a line's text.
func LineText(l Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

var (
	ansiRe    = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	mdLinkRe  = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	bareURLRe = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)
	boldRe    = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

// sanitise removes escape sequences and control characters, expands tabs and
// normalises line endings.
func sanitise(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || (r >= 0x20 && r != 0x7f) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FirstLine returns the first non-blank sanitised line with markdown bold
// markers removed and whitespace collapsed, for one-line previews.
func FirstLine(md string) string {
	for _, l := range strings.Split(sanitise(md), "\n") {
		l = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
		if l == "" {
			continue
		}
		l = boldRe.ReplaceAllString(l, "$1$2")
		return l
	}
	return ""
}

type blockKind int

const (
	paraBlock blockKind = iota
	quoteBlock
	codeBlock
)

type block struct {
	kind  blockKind
	lines []string
}

// blocks splits sanitised text into paragraph, quote and code blocks.
func blocks(s string) []block {
	var out []block
	var cur *block
	start := func(k blockKind) {
		out = append(out, block{kind: k})
		cur = &out[len(out)-1]
	}
	end := func() { cur = nil }
	inFence := false
	prevBlank := true
	for _, raw := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			if inFence {
				inFence = false
				end()
			} else {
				inFence = true
				start(codeBlock)
			}
		case inFence:
			cur.lines = append(cur.lines, raw)
		case trimmed == "":
			end()
		case strings.HasPrefix(raw, "    ") && (prevBlank || (cur != nil && cur.kind == codeBlock)):
			if cur == nil || cur.kind != codeBlock {
				start(codeBlock)
			}
			cur.lines = append(cur.lines, strings.TrimPrefix(raw, "    "))
		case strings.HasPrefix(trimmed, ">"):
			if cur == nil || cur.kind != quoteBlock {
				start(quoteBlock)
			}
			cur.lines = append(cur.lines, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
		default:
			if cur == nil || cur.kind != paraBlock {
				start(paraBlock)
			}
			cur.lines = append(cur.lines, trimmed)
		}
		prevBlank = trimmed == ""
	}
	return out
}

// linkTable numbers URLs, reusing numbers for repeats.
type linkTable struct {
	urls  []string
	index map[string]int
}

func (lt *linkTable) add(u string) int {
	if strings.HasPrefix(u, "/") {
		u = "https://www.reddit.com" + u
	}
	if n, ok := lt.index[u]; ok {
		return n
	}
	lt.urls = append(lt.urls, u)
	lt.index[u] = len(lt.urls)
	return len(lt.urls)
}

// inline converts one paragraph of markdown into spans with Bold and Link
// kinds. base is the kind for plain text (Text or Quote).
func inline(s string, base Kind, lt *linkTable) []Span {
	var spans []Span
	// Markdown links become text[n] Link spans; split around them first.
	for len(s) > 0 {
		m := mdLinkRe.FindStringSubmatchIndex(s)
		if m == nil {
			spans = append(spans, boldSpans(s, base, lt)...)
			break
		}
		spans = append(spans, boldSpans(s[:m[0]], base, lt)...)
		text, url := s[m[2]:m[3]], s[m[4]:m[5]]
		n := lt.add(url)
		spans = append(spans, Span{Text: text + "[" + itoa(n) + "]", Kind: Link})
		s = s[m[1]:]
	}
	return spans
}

// boldSpans splits s on **bold** markers, recording bare URLs as it goes.
func boldSpans(s string, base Kind, lt *linkTable) []Span {
	var spans []Span
	for len(s) > 0 {
		m := boldRe.FindStringSubmatchIndex(s)
		if m == nil {
			spans = appendText(spans, s, base, lt)
			break
		}
		spans = appendText(spans, s[:m[0]], base, lt)
		inner := s[m[2]:m[3]]
		if m[2] < 0 {
			inner = s[m[4]:m[5]]
		}
		spans = append(spans, Span{Text: inner, Kind: Bold})
		s = s[m[1]:]
	}
	return spans
}

func appendText(spans []Span, s string, base Kind, lt *linkTable) []Span {
	if s == "" {
		return spans
	}
	for _, u := range bareURLRe.FindAllString(s, -1) {
		lt.add(u)
	}
	return append(spans, Span{Text: s, Kind: base})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// wrapSpans lays spans out into lines of at most w cells, breaking between
// words and hard-breaking words wider than w, keeping each cluster's kind.
func wrapSpans(spans []Span, w int) []Line {
	if w <= 0 {
		return nil
	}
	type tok struct {
		s string
		k Kind
		w int
	}
	var toks []tok
	for _, sp := range spans {
		rest := sp.Text
		for rest != "" {
			i := strings.IndexByte(rest, ' ')
			var word string
			if i < 0 {
				word, rest = rest, ""
			} else {
				word, rest = rest[:i], rest[i+1:]
			}
			if word != "" {
				toks = append(toks, tok{word, sp.Kind, Width(word)})
			}
			if i >= 0 {
				toks = append(toks, tok{" ", sp.Kind, 1})
			}
		}
	}
	var lines []Line
	var cur Line
	curW := 0
	flush := func() {
		lines = append(lines, cur)
		cur = nil
		curW = 0
	}
	push := func(s string, k Kind, cw int) {
		if n := len(cur); n > 0 && cur[n-1].Kind == k {
			cur[n-1].Text += s
		} else {
			cur = append(cur, Span{Text: s, Kind: k})
		}
		curW += cw
	}
	for _, t := range toks {
		if t.s == " " {
			if curW > 0 && curW < w {
				push(" ", t.k, 1)
			}
			continue
		}
		if t.w > w {
			if curW > 0 {
				flush()
			}
			g := uniseg.NewGraphemes(t.s)
			for g.Next() {
				if curW+g.Width() > w {
					flush()
				}
				push(g.Str(), t.k, g.Width())
			}
			continue
		}
		if curW+t.w > w {
			flush()
		}
		push(t.s, t.k, t.w)
	}
	if curW > 0 || len(lines) == 0 {
		flush()
	}
	// Trim trailing spaces on each line.
	for i, l := range lines {
		if n := len(l); n > 0 {
			l[n-1].Text = strings.TrimRight(l[n-1].Text, " ")
			if l[n-1].Text == "" {
				lines[i] = l[:n-1]
			}
		}
	}
	return lines
}

// Render converts a Reddit markdown body into styled lines wrapped to w cells.
func Render(md string, w int) Doc {
	lt := &linkTable{index: map[string]int{}}
	var doc Doc
	for i, b := range blocks(sanitise(md)) {
		if i > 0 {
			doc.Lines = append(doc.Lines, Line{})
		}
		switch b.kind {
		case codeBlock:
			for _, l := range b.lines {
				doc.Lines = append(doc.Lines, Line{{Text: "  " + Clip(l, w-2), Kind: Code}})
			}
		case quoteBlock:
			spans := inline(strings.Join(b.lines, " "), Quote, lt)
			for _, l := range wrapSpans(spans, w-2) {
				doc.Lines = append(doc.Lines, append(Line{{Text: "> ", Kind: Quote}}, l...))
			}
		default:
			for _, pl := range paragraphLines(b.lines) {
				doc.Lines = append(doc.Lines, wrapSpans(inline(pl, Text, lt), w)...)
			}
		}
	}
	doc.Links = lt.urls
	return doc
}

// paragraphLines joins soft-broken lines, but keeps list items on their own
// lines so markers stay at the start.
func paragraphLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if len(out) > 0 && !isListItem(l) && !isListItem(out[len(out)-1]) {
			out[len(out)-1] += " " + l
			continue
		}
		out = append(out, l)
	}
	return out
}

func isListItem(l string) bool {
	if strings.HasPrefix(l, "* ") || strings.HasPrefix(l, "- ") {
		return true
	}
	i := 0
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(l) && l[i] == '.' && l[i+1] == ' '
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/textfmt/`
Expected: PASS. If `TestRenderFencedCode` fails on the `go(i)` line, check tab expansion happens in `sanitise` before block splitting (a tab inside the fence becomes four spaces, plus the two-space code indent, giving six).

- [ ] **Step 5: Commit**

```bash
git add internal/textfmt
git commit -m "Render Reddit markdown into styled wrapped lines

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: term package: styles, keys, Canvas, tcell and simulation terminals

**Files:**
- Create: `internal/term/style.go`, `internal/term/key.go`, `internal/term/canvas.go`, `internal/term/tcell.go`, `internal/term/sim.go`
- Test: `internal/term/canvas_test.go`, `internal/term/sim_test.go`, `internal/term/tcell_test.go`

**Interfaces:**
- Produces:
  - `type Color uint8` with `Default, Black, Red, Green, Yellow, Blue, Magenta, Cyan, White, BrightBlack … BrightWhite`.
  - `type Style struct { FG, BG Color; Bold, Reverse bool }`.
  - `type KeyCode int` with `KeyRune, KeyEnter, KeyEscape, KeyBackspace, KeyDelete, KeyTab, KeyUp, KeyDown, KeyLeft, KeyRight, KeyPgUp, KeyPgDn, KeyHome, KeyEnd, KeyCtrlC, KeyCtrlL`.
  - `type Key struct { Code KeyCode; Rune rune; Paste bool }`, `type Resize struct { W, H int }`, `type Event any`.
  - `func R(r rune) Key`, `func K(c KeyCode) Key` constructors.
  - `type Canvas interface { Size() (int, int); Put(x, y int, cluster string, st Style) int; Text(x, y int, s string, st Style, maxWidth int) int; Fill(x, y, w, h int, r rune, st Style); ShowCursor(x, y int); HideCursor() }`.
  - `type Terminal interface { Canvas; Events() <-chan Event; Show(); Sync(); Clear(); Fini() }`.
  - `func Sub(c Canvas, x, y, w, h int) Canvas` clipping sub-canvas with local coordinates.
  - `func NewTcell() (Terminal, error)`.
  - `type Sim` with `func NewSim(w, h int) *Sim`, `Inject(ev Event)`, `Resize(w, h int)`, `String() string`, `Row(y int) string`, `CellAt(x, y int) (string, Style)`, `Cursor() (x, y int, shown bool)`.

- [ ] **Step 1: Write style.go and key.go (no logic to test yet)**

`internal/term/style.go`:

```go
// Package term defines the terminal abstraction the UI draws through, with a
// tcell implementation for real terminals and a simulation for tests. It is
// the only package that imports tcell.
package term

// Color is one of the 16 ANSI palette colours, or Default for the terminal's own.
type Color uint8

// Palette colours. Values 1..16 map to ANSI palette indices 0..15.
const (
	Default Color = iota
	Black
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
	BrightBlack
	BrightRed
	BrightGreen
	BrightYellow
	BrightBlue
	BrightMagenta
	BrightCyan
	BrightWhite
)

// Style is how a cell is drawn.
type Style struct {
	FG, BG  Color
	Bold    bool
	Reverse bool
}

// Reversed returns the style with reverse video set.
func (s Style) Reversed() Style { s.Reverse = true; return s }

// Bolded returns the style with bold set.
func (s Style) Bolded() Style { s.Bold = true; return s }
```

`internal/term/key.go`:

```go
package term

// KeyCode identifies a key. KeyRune carries a printable rune in Key.Rune.
type KeyCode int

// Key codes.
const (
	KeyRune KeyCode = iota
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyDelete
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyCtrlC
	KeyCtrlL
)

// Key is a key press. Paste is true for runes delivered inside a bracketed paste.
type Key struct {
	Code  KeyCode
	Rune  rune
	Paste bool
}

// Resize reports a new terminal size.
type Resize struct{ W, H int }

// Event is a Key or a Resize.
type Event any

// R builds a rune key.
func R(r rune) Key { return Key{Code: KeyRune, Rune: r} }

// K builds a special key.
func K(c KeyCode) Key { return Key{Code: c} }
```

- [ ] **Step 2: Write the failing canvas tests**

`internal/term/canvas_test.go`:

```go
package term

import "testing"

func TestSubClipsAndOffsets(t *testing.T) {
	sim := NewSim(20, 6)
	sub := Sub(sim, 5, 2, 8, 2)
	if w, h := sub.Size(); w != 8 || h != 2 {
		t.Fatalf("size = %dx%d", w, h)
	}
	n := sub.Text(0, 0, "abcdefghijklmnop", Style{}, 100)
	if n != 8 {
		t.Errorf("Text drew %d cells, want 8", n)
	}
	if got := sim.Row(2); got != "     abcdefgh" {
		t.Errorf("row 2 = %q", got)
	}
	sub.Text(0, 5, "outside", Style{}, 10)
	if got := sim.Row(5); got != "" {
		t.Errorf("row 5 should be untouched, got %q", got)
	}
	sub.Fill(-2, 1, 100, 100, '#', Style{})
	if got := sim.Row(3); got != "     ########" {
		t.Errorf("fill row 3 = %q", got)
	}
	if got := sim.Row(4); got != "" {
		t.Errorf("fill should not spill to row 4, got %q", got)
	}
}

func TestSubClampsToParent(t *testing.T) {
	sim := NewSim(10, 4)
	sub := Sub(sim, 6, 2, 10, 10)
	if w, h := sub.Size(); w != 4 || h != 2 {
		t.Errorf("size = %dx%d, want 4x2", w, h)
	}
}

func TestSubCursor(t *testing.T) {
	sim := NewSim(10, 4)
	Sub(sim, 3, 1, 5, 2).ShowCursor(1, 1)
	if x, y, shown := sim.Cursor(); !shown || x != 4 || y != 2 {
		t.Errorf("cursor = %d,%d,%v", x, y, shown)
	}
}
```

`internal/term/sim_test.go`:

```go
package term

import "testing"

func TestSimTextAndWideCells(t *testing.T) {
	sim := NewSim(10, 2)
	n := sim.Text(0, 0, "日本x", Style{FG: Cyan}, 10)
	if n != 5 {
		t.Errorf("used %d cells, want 5", n)
	}
	if got := sim.Row(0); got != "日本x" {
		t.Errorf("row = %q", got)
	}
	if s, st := sim.CellAt(2, 0); s != "本" || st.FG != Cyan {
		t.Errorf("cell 2 = %q %+v", s, st)
	}
	if s, _ := sim.CellAt(1, 0); s != "" {
		t.Errorf("continuation cell should be empty, got %q", s)
	}
}

func TestSimTextClipsAtMaxWidth(t *testing.T) {
	sim := NewSim(10, 1)
	if n := sim.Text(0, 0, "abcdef", Style{}, 3); n != 3 {
		t.Errorf("used %d, want 3", n)
	}
	if got := sim.Row(0); got != "abc" {
		t.Errorf("row = %q", got)
	}
	if n := sim.Text(9, 0, "日", Style{}, 5); n != 0 {
		t.Errorf("wide cluster at last column should not draw, used %d", n)
	}
}

func TestSimPutCombining(t *testing.T) {
	sim := NewSim(4, 1)
	if n := sim.Put(0, 0, "é", Style{}); n != 1 {
		t.Errorf("width %d", n)
	}
	if s, _ := sim.CellAt(0, 0); s != "é" {
		t.Errorf("cell = %q", s)
	}
}

func TestSimStringClearAndResize(t *testing.T) {
	sim := NewSim(5, 2)
	sim.Text(0, 0, "hi", Style{}, 5)
	sim.Text(0, 1, "yo", Style{}, 5)
	if got := sim.String(); got != "hi\nyo" {
		t.Errorf("String = %q", got)
	}
	sim.Clear()
	if got := sim.String(); got != "\n" {
		t.Errorf("after Clear = %q", got)
	}
	sim.Resize(8, 3)
	if w, h := sim.Size(); w != 8 || h != 3 {
		t.Errorf("size = %dx%d", w, h)
	}
	ev := <-sim.Events()
	if r, ok := ev.(Resize); !ok || r.W != 8 || r.H != 3 {
		t.Errorf("event = %#v", ev)
	}
}

func TestSimInject(t *testing.T) {
	sim := NewSim(5, 2)
	sim.Inject(R('q'))
	if k, ok := (<-sim.Events()).(Key); !ok || k.Rune != 'q' {
		t.Errorf("got %#v", k)
	}
}
```

`internal/term/tcell_test.go`:

```go
package term

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestToTcellStyle(t *testing.T) {
	fg, bg, attrs := toTcell(Style{FG: Cyan, BG: BrightWhite, Bold: true, Reverse: true}).Decompose()
	if fg != tcell.PaletteColor(6) {
		t.Errorf("fg = %v", fg)
	}
	if bg != tcell.PaletteColor(15) {
		t.Errorf("bg = %v", bg)
	}
	if attrs&tcell.AttrBold == 0 || attrs&tcell.AttrReverse == 0 {
		t.Errorf("attrs = %v", attrs)
	}
	fg, bg, _ = toTcell(Style{}).Decompose()
	if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
		t.Errorf("default colours = %v %v", fg, bg)
	}
}

func TestTranslateKeys(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want Key
	}{
		{tcell.NewEventKey(tcell.KeyRune, 'x', 0), R('x')},
		{tcell.NewEventKey(tcell.KeyEnter, 0, 0), K(KeyEnter)},
		{tcell.NewEventKey(tcell.KeyEscape, 0, 0), K(KeyEscape)},
		{tcell.NewEventKey(tcell.KeyBackspace2, 0, 0), K(KeyBackspace)},
		{tcell.NewEventKey(tcell.KeyPgDn, 0, 0), K(KeyPgDn)},
		{tcell.NewEventKey(tcell.KeyCtrlC, 0, 0), K(KeyCtrlC)},
		{tcell.NewEventKey(tcell.KeyCtrlL, 0, 0), K(KeyCtrlL)},
	}
	for _, c := range cases {
		got, ok := translate(c.ev, false)
		if !ok || got != c.want {
			t.Errorf("translate(%v) = %+v,%v want %+v", c.ev.Key(), got, ok, c.want)
		}
	}
	if _, ok := translate(tcell.NewEventKey(tcell.KeyF1, 0, 0), false); ok {
		t.Error("F1 should be ignored")
	}
	if k, _ := translate(tcell.NewEventKey(tcell.KeyRune, 'p', 0), true); !k.Paste {
		t.Error("paste flag not set")
	}
}

func TestGridPutOnSimulationScreen(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(10, 2)
	g := grid{s}
	if n := g.Put(1, 0, "é", Style{FG: Green}); n != 1 {
		t.Errorf("width %d", n)
	}
	mainc, combc, st, w := s.GetContent(1, 0)
	if mainc != 'e' || len(combc) != 1 || combc[0] != 0x301 || w != 1 {
		t.Errorf("content = %q %v %d", mainc, combc, w)
	}
	if fg, _, _ := st.Decompose(); fg != tcell.PaletteColor(2) {
		t.Errorf("fg = %v", fg)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/term/`
Expected: FAIL, undefined: NewSim, Sub, toTcell, translate, grid.

- [ ] **Step 4: Implement canvas.go**

```go
package term

import "github.com/rivo/uniseg"

// Canvas is a rectangle of cells that screens draw into. Coordinates are
// local to the canvas; drawing outside it is ignored.
type Canvas interface {
	Size() (w, h int)
	// Put draws one grapheme cluster and returns the cells it occupies (0, 1 or 2).
	Put(x, y int, cluster string, st Style) int
	// Text draws s from x,y, clipping at maxWidth cells and at the canvas edge.
	// It returns the cells used.
	Text(x, y int, s string, st Style, maxWidth int) int
	Fill(x, y, w, h int, r rune, st Style)
	ShowCursor(x, y int)
	HideCursor()
}

// Terminal is a Canvas with an event source and a presentation lifecycle.
type Terminal interface {
	Canvas
	Events() <-chan Event
	Show()
	Sync()
	Clear()
	Fini()
}

type sub struct {
	p          Canvas
	x, y, w, h int
}

// Sub returns a canvas for the rectangle x,y,w,h of c, clamped to c's bounds.
func Sub(c Canvas, x, y, w, h int) Canvas {
	pw, ph := c.Size()
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > pw {
		w = pw - x
	}
	if y+h > ph {
		h = ph - y
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &sub{p: c, x: x, y: y, w: w, h: h}
}

func (s *sub) Size() (int, int) { return s.w, s.h }

func (s *sub) Put(x, y int, cluster string, st Style) int {
	if x < 0 || y < 0 || y >= s.h || x+uniseg.StringWidth(cluster) > s.w {
		return 0
	}
	return s.p.Put(s.x+x, s.y+y, cluster, st)
}

func (s *sub) Text(x, y int, str string, st Style, maxWidth int) int {
	if x < 0 || y < 0 || y >= s.h || x >= s.w {
		return 0
	}
	if m := s.w - x; maxWidth > m {
		maxWidth = m
	}
	return s.p.Text(s.x+x, s.y+y, str, st, maxWidth)
}

func (s *sub) Fill(x, y, w, h int, r rune, st Style) {
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > s.w {
		w = s.w - x
	}
	if y+h > s.h {
		h = s.h - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	s.p.Fill(s.x+x, s.y+y, w, h, r, st)
}

func (s *sub) ShowCursor(x, y int) { s.p.ShowCursor(s.x+x, s.y+y) }
func (s *sub) HideCursor()         { s.p.HideCursor() }

// drawText walks s by grapheme cluster, calling set for each cluster that
// fits within maxWidth cells, and returns the cells used. Zero-width
// clusters are skipped.
func drawText(set func(x int, cluster string, w int), x int, s string, maxWidth int) int {
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := g.Width()
		if w == 0 {
			continue
		}
		if used+w > maxWidth {
			break
		}
		set(x+used, g.Str(), w)
		used += w
	}
	return used
}
```

- [ ] **Step 5: Implement tcell.go**

```go
package term

import (
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// grid draws onto a tcell.Screen.
type grid struct{ s tcell.Screen }

func toTcell(st Style) tcell.Style {
	s := tcell.StyleDefault
	if st.FG != Default {
		s = s.Foreground(tcell.PaletteColor(int(st.FG) - 1))
	}
	if st.BG != Default {
		s = s.Background(tcell.PaletteColor(int(st.BG) - 1))
	}
	return s.Bold(st.Bold).Reverse(st.Reverse)
}

func (g grid) Size() (int, int) { return g.s.Size() }

func (g grid) set(x, y int, cluster string, st Style) {
	rs := []rune(cluster)
	g.s.SetContent(x, y, rs[0], rs[1:], toTcell(st))
}

func (g grid) Put(x, y int, cluster string, st Style) int {
	w := uniseg.StringWidth(cluster)
	if w == 0 || cluster == "" {
		return 0
	}
	g.set(x, y, cluster, st)
	return w
}

func (g grid) Text(x, y int, s string, st Style, maxWidth int) int {
	return drawText(func(cx int, cl string, _ int) { g.set(cx, y, cl, st) }, x, s, maxWidth)
}

func (g grid) Fill(x, y, w, h int, r rune, st Style) {
	ts := toTcell(st)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			g.s.SetContent(xx, yy, r, nil, ts)
		}
	}
}

func (g grid) ShowCursor(x, y int) { g.s.ShowCursor(x, y) }
func (g grid) HideCursor()         { g.s.HideCursor() }

type tcellTerm struct {
	grid
	events chan Event
	quit   chan struct{}
	once   sync.Once
}

// NewTcell initialises the real terminal. Call Fini to restore it.
func NewTcell() (Terminal, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnablePaste()
	s.Clear()
	t := &tcellTerm{grid: grid{s}, events: make(chan Event, 64), quit: make(chan struct{})}
	go t.pump()
	return t, nil
}

func (t *tcellTerm) pump() {
	ch := make(chan tcell.Event, 16)
	go t.s.ChannelEvents(ch, t.quit)
	paste := false
	for ev := range ch {
		switch e := ev.(type) {
		case *tcell.EventResize:
			w, h := e.Size()
			t.events <- Resize{W: w, H: h}
		case *tcell.EventPaste:
			paste = e.Start()
		case *tcell.EventKey:
			if k, ok := translate(e, paste); ok {
				t.events <- k
			}
		}
	}
	close(t.events)
}

func translate(e *tcell.EventKey, paste bool) (Key, bool) {
	codes := map[tcell.Key]KeyCode{
		tcell.KeyEnter: KeyEnter, tcell.KeyEscape: KeyEscape,
		tcell.KeyBackspace: KeyBackspace, tcell.KeyBackspace2: KeyBackspace,
		tcell.KeyDelete: KeyDelete, tcell.KeyTab: KeyTab,
		tcell.KeyUp: KeyUp, tcell.KeyDown: KeyDown, tcell.KeyLeft: KeyLeft, tcell.KeyRight: KeyRight,
		tcell.KeyPgUp: KeyPgUp, tcell.KeyPgDn: KeyPgDn, tcell.KeyHome: KeyHome, tcell.KeyEnd: KeyEnd,
		tcell.KeyCtrlC: KeyCtrlC, tcell.KeyCtrlL: KeyCtrlL,
	}
	if e.Key() == tcell.KeyRune {
		return Key{Code: KeyRune, Rune: e.Rune(), Paste: paste}, true
	}
	if c, ok := codes[e.Key()]; ok {
		return Key{Code: c}, true
	}
	return Key{}, false
}

func (t *tcellTerm) Events() <-chan Event { return t.events }
func (t *tcellTerm) Show()                { t.s.Show() }
func (t *tcellTerm) Sync()                { t.s.Sync() }
func (t *tcellTerm) Clear()               { t.s.Clear() }
// Fini restores the terminal. Safe to call more than once.
func (t *tcellTerm) Fini() {
	t.once.Do(func() {
		close(t.quit)
		t.s.Fini()
	})
}
```

- [ ] **Step 6: Implement sim.go**

```go
package term

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type simCell struct {
	text string // "" for blank or for the second half of a wide cluster
	cont bool   // true for the second half of a wide cluster
	st   Style
}

// Sim is an in-memory Terminal for tests. It records every cell itself and
// forwards drawing to a tcell simulation screen so the real code path runs.
type Sim struct {
	s      tcell.SimulationScreen
	w, h   int
	cells  [][]simCell
	events chan Event
	curX   int
	curY   int
	curOn  bool
}

// NewSim creates a simulated terminal of the given size.
func NewSim(w, h int) *Sim {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		panic(err)
	}
	s.SetSize(w, h)
	sim := &Sim{s: s, events: make(chan Event, 64)}
	sim.reset(w, h)
	return sim
}

func (m *Sim) reset(w, h int) {
	m.w, m.h = w, h
	m.cells = make([][]simCell, h)
	for y := range m.cells {
		m.cells[y] = make([]simCell, w)
	}
}

// Size reports the simulated size.
func (m *Sim) Size() (int, int) { return m.w, m.h }

func (m *Sim) set(x, y int, cluster string, w int, st Style) {
	if y < 0 || y >= m.h || x < 0 || x+w > m.w {
		return
	}
	m.cells[y][x] = simCell{text: cluster, st: st}
	for i := 1; i < w; i++ {
		m.cells[y][x+i] = simCell{cont: true, st: st}
	}
	rs := []rune(cluster)
	m.s.SetContent(x, y, rs[0], rs[1:], toTcell(st))
}

// Put draws one cluster.
func (m *Sim) Put(x, y int, cluster string, st Style) int {
	w := uniseg.StringWidth(cluster)
	if w == 0 || cluster == "" || x+w > m.w || x < 0 || y < 0 || y >= m.h {
		return 0
	}
	m.set(x, y, cluster, w, st)
	return w
}

// Text draws s clipped to maxWidth and the right edge.
func (m *Sim) Text(x, y int, s string, st Style, maxWidth int) int {
	if x < 0 || y < 0 || y >= m.h {
		return 0
	}
	if r := m.w - x; maxWidth > r {
		maxWidth = r
	}
	return drawText(func(cx int, cl string, w int) { m.set(cx, y, cl, w, st) }, x, s, maxWidth)
}

// Fill sets a rectangle to r.
func (m *Sim) Fill(x, y, w, h int, r rune, st Style) {
	for yy := y; yy < y+h && yy < m.h; yy++ {
		for xx := x; xx < x+w && xx < m.w; xx++ {
			if xx >= 0 && yy >= 0 {
				m.set(xx, yy, string(r), 1, st)
			}
		}
	}
}

// ShowCursor records the cursor position.
func (m *Sim) ShowCursor(x, y int) { m.curX, m.curY, m.curOn = x, y, true }

// HideCursor hides it.
func (m *Sim) HideCursor() { m.curOn = false }

// Cursor reports the cursor state.
func (m *Sim) Cursor() (int, int, bool) { return m.curX, m.curY, m.curOn }

// Events is the injected event stream.
func (m *Sim) Events() <-chan Event { return m.events }

// Inject queues an event as if the user had produced it.
func (m *Sim) Inject(ev Event) { m.events <- ev }

// Resize changes the size and queues a Resize event.
func (m *Sim) Resize(w, h int) {
	m.s.SetSize(w, h)
	m.reset(w, h)
	m.Inject(Resize{W: w, H: h})
}

// Show, Sync and Fini forward to the tcell simulation.
func (m *Sim) Show() { m.s.Show() }
func (m *Sim) Sync() { m.s.Sync() }
func (m *Sim) Fini() { m.s.Fini() }

// Clear blanks every cell.
func (m *Sim) Clear() {
	m.s.Clear()
	m.reset(m.w, m.h)
}

// CellAt returns the cluster and style at x,y. Continuation cells of a wide
// cluster return "".
func (m *Sim) CellAt(x, y int) (string, Style) {
	if y < 0 || y >= m.h || x < 0 || x >= m.w {
		return "", Style{}
	}
	c := m.cells[y][x]
	return c.text, c.st
}

// Row returns row y as text with trailing spaces removed.
func (m *Sim) Row(y int) string {
	var b strings.Builder
	for _, c := range m.cells[y] {
		switch {
		case c.cont:
		case c.text == "":
			b.WriteByte(' ')
		default:
			b.WriteString(c.text)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// String returns every row joined by newlines.
func (m *Sim) String() string {
	rows := make([]string, m.h)
	for y := range rows {
		rows[y] = m.Row(y)
	}
	return strings.Join(rows, "\n")
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/term/`
Expected: PASS. `TestSimStringClearAndResize` expects `"\n"` after Clear on a 5x2 terminal because two empty rows joined by one newline.

- [ ] **Step 8: Commit**

```bash
git add internal/term
git commit -m "Add terminal abstraction with tcell and simulation backends

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: theme package

**Files:**
- Create: `internal/theme/theme.go`
- Test: `internal/theme/theme_test.go`

**Interfaces:**
- Consumes: `term.Style`, `term.Color`.
- Produces: `type Role int` with `Frame, Logo, Heading, Subject, Author, OP, Mod, Meta, Body, Quote, Code, Bold, Link, Prompt, Hotkey, Error, Stub, Rule, Cursor, Sticky, NSFW`; `func Style(r Role) term.Style`.

- [ ] **Step 1: Write the failing test**

`internal/theme/theme_test.go`:

```go
package theme

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestSchemeA(t *testing.T) {
	cases := map[Role]term.Style{
		Frame:   {FG: term.Cyan},
		Heading: {FG: term.BrightYellow},
		Subject: {FG: term.BrightWhite},
		Author:  {FG: term.BrightGreen},
		Meta:    {FG: term.BrightBlack},
		Cursor:  {FG: term.Black, BG: term.White},
		Error:   {FG: term.BrightRed},
	}
	for r, want := range cases {
		if got := Style(r); got != want {
			t.Errorf("Style(%d) = %+v, want %+v", r, got, want)
		}
	}
}

func TestEveryRoleHasStyle(t *testing.T) {
	for r := Frame; r <= NSFW; r++ {
		if _, ok := styles[r]; !ok {
			t.Errorf("role %d has no style", r)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/theme/`
Expected: FAIL, undefined: Role, Style, styles.

- [ ] **Step 3: Implement theme.go**

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/theme/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/theme
git commit -m "Add colour theme with scheme A roles

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: reddit models and JSON parsers

**Files:**
- Create: `internal/reddit/models.go`, `internal/reddit/parse.go`
- Create fixtures: `internal/reddit/testdata/listing.json`, `internal/reddit/testdata/thread.json`, `internal/reddit/testdata/morechildren.json`
- Test: `internal/reddit/parse_test.go`

**Interfaces:**
- Produces:
  - `type Sort string` (`Hot, New, Top, Rising`) with `Next() Sort` cycling hot, new, top, rising.
  - `type CommentSort string` (`Best, TopComments, NewComments`) with `API() string` (best becomes `confidence`) and `Next()`.
  - `type Fetch struct{ Fresh bool }`.
  - `type Post`, `type Comment`, `type MoreStub`, `type Listing`, `type Thread`, `type Things` as below.
  - `type Store interface` with `Posts`, `Thread`, `Subtree`, `MoreChildren`.
  - `func ParseListing(r io.Reader) (Listing, error)`, `func ParseThread(r io.Reader) (Thread, error)`, `func ParseMoreChildren(r io.Reader) (Things, error)`.
  - `func (p *Post) IsOP(author string) bool`.

- [ ] **Step 1: Write the fixtures**

`internal/reddit/testdata/listing.json`:

```json
{"kind": "Listing", "data": {"after": "t3_bbb", "dist": 2, "children": [
  {"kind": "t3", "data": {"id": "aaa", "name": "t3_aaa", "subreddit": "linux", "title": "Kernel 7.2 released", "author": "torvaldsfan", "score": 2100, "num_comments": 342, "created_utc": 1790400000.0, "url": "https://kernel.org/", "domain": "kernel.org", "permalink": "/r/linux/comments/aaa/kernel_72_released/", "is_self": false, "selftext": "", "stickied": false, "over_18": false, "distinguished": null}},
  {"kind": "t3", "data": {"id": "bbb", "name": "t3_bbb", "subreddit": "linux", "title": "Weekly questions thread", "author": "AutoModerator", "score": 12, "num_comments": 57, "created_utc": 1790300000.0, "url": "https://www.reddit.com/r/linux/comments/bbb/weekly/", "domain": "self.linux", "permalink": "/r/linux/comments/bbb/weekly/", "is_self": true, "selftext": "Ask **anything** here.", "stickied": true, "over_18": false, "distinguished": "moderator"}}
]}}
```

`internal/reddit/testdata/thread.json`:

```json
[
{"kind": "Listing", "data": {"after": null, "children": [
  {"kind": "t3", "data": {"id": "aaa", "name": "t3_aaa", "subreddit": "linux", "title": "Kernel 7.2 released", "author": "torvaldsfan", "score": 2100, "num_comments": 342, "created_utc": 1790400000.0, "url": "https://kernel.org/", "domain": "kernel.org", "permalink": "/r/linux/comments/aaa/kernel_72_released/", "is_self": false, "selftext": "", "stickied": false, "over_18": false, "distinguished": null}}
]}},
{"kind": "Listing", "data": {"after": null, "children": [
  {"kind": "t1", "data": {"id": "c1", "name": "t1_c1", "parent_id": "t3_aaa", "author": "sched_nerd", "body": "The EEVDF changes are the headline.\n\nLazy preemption is the real win.", "score": 412, "created_utc": 1790403600.0, "is_submitter": false, "distinguished": null, "replies": {"kind": "Listing", "data": {"after": null, "children": [
    {"kind": "t1", "data": {"id": "c2", "name": "t1_c2", "parent_id": "t1_c1", "author": "torvaldsfan", "body": "Agreed.", "score": 98, "created_utc": 1790407200.0, "is_submitter": true, "distinguished": null, "replies": ""}},
    {"kind": "more", "data": {"id": "x", "name": "t1_x", "parent_id": "t1_c1", "count": 3, "children": ["x", "y", "z"]}}
  ]}}}},
  {"kind": "t1", "data": {"id": "c3", "name": "t1_c3", "parent_id": "t3_aaa", "author": "[deleted]", "body": "Body survives the account.", "score": 5, "created_utc": 1790403700.0, "is_submitter": false, "distinguished": null, "replies": ""}},
  {"kind": "t1", "data": {"id": "c4", "name": "t1_c4", "parent_id": "t3_aaa", "author": "modbot", "body": "[removed]", "score": 1, "created_utc": 1790403800.0, "is_submitter": false, "distinguished": "moderator", "replies": {"kind": "Listing", "data": {"after": null, "children": [
    {"kind": "more", "data": {"id": "_", "name": "t1__", "parent_id": "t1_c4", "count": 0, "children": []}}
  ]}}}},
  {"kind": "unknownkind", "data": {}},
  {"kind": "more", "data": {"id": "m1", "name": "t1_m1", "parent_id": "t3_aaa", "count": 40, "children": ["p", "q"]}}
]}}
]
```

`internal/reddit/testdata/morechildren.json`:

```json
{"json": {"errors": [], "data": {"things": [
  {"kind": "t1", "data": {"id": "y", "name": "t1_y", "parent_id": "t1_x", "author": "later", "body": "reply to x", "score": 2, "created_utc": 1790410000.0, "is_submitter": false, "distinguished": null, "replies": ""}},
  {"kind": "t1", "data": {"id": "x", "name": "t1_x", "parent_id": "t1_c1", "author": "xorg4life", "body": "X11 forever", "score": 7, "created_utc": 1790409000.0, "is_submitter": false, "distinguished": null, "replies": ""}},
  {"kind": "more", "data": {"id": "z", "name": "t1_z", "parent_id": "t1_x", "count": 1, "children": ["z"]}}
]}}}
```

- [ ] **Step 2: Write the failing tests**

`internal/reddit/parse_test.go`:

```go
package reddit

import (
	"os"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseListing(t *testing.T) {
	l, err := ParseListing(open(t, "listing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if l.After != "t3_bbb" || len(l.Posts) != 2 {
		t.Fatalf("after=%q posts=%d", l.After, len(l.Posts))
	}
	p := l.Posts[0]
	if p.ID != "aaa" || p.Fullname != "t3_aaa" || p.Title != "Kernel 7.2 released" || p.Author != "torvaldsfan" {
		t.Errorf("post 0 = %+v", p)
	}
	if p.Score != 2100 || p.NumComments != 342 || p.Domain != "kernel.org" || p.IsSelf {
		t.Errorf("post 0 numbers = %+v", p)
	}
	if !p.Created.Equal(time.Unix(1790400000, 0)) {
		t.Errorf("created = %v", p.Created)
	}
	q := l.Posts[1]
	if !q.IsSelf || !q.Stickied || q.Distinguished != "moderator" || q.SelfText != "Ask **anything** here." {
		t.Errorf("post 1 = %+v", q)
	}
}

func TestParseListingRejectsNonListing(t *testing.T) {
	if _, err := ParseListing(strings.NewReader(`{"kind":"t3","data":{}}`)); err == nil {
		t.Error("expected error")
	}
	if _, err := ParseListing(strings.NewReader(`not json`)); err == nil {
		t.Error("expected error")
	}
}

func TestParseThread(t *testing.T) {
	th, err := ParseThread(open(t, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Post == nil || th.Post.ID != "aaa" {
		t.Fatalf("post = %+v", th.Post)
	}
	if len(th.Comments) != 3 {
		t.Fatalf("top-level comments = %d, want 3 (unknown kind skipped)", len(th.Comments))
	}
	c1 := th.Comments[0]
	if c1.ID != "c1" || c1.Fullname != "t1_c1" || c1.ParentFullname != "t3_aaa" || c1.Depth != 0 {
		t.Errorf("c1 = %+v", c1)
	}
	if len(c1.Children) != 1 || c1.Children[0].ID != "c2" || c1.Children[0].Depth != 1 {
		t.Errorf("c1 children = %+v", c1.Children)
	}
	if !c1.Children[0].IsSubmitter {
		t.Error("c2 should be submitter")
	}
	if c1.More == nil || c1.More.Count != 3 || len(c1.More.IDs) != 3 || c1.More.ParentFullname != "t1_c1" {
		t.Errorf("c1 more = %+v", c1.More)
	}
	c3 := th.Comments[1]
	if !c3.AuthorDeleted || c3.BodyRemoved || c3.Body != "Body survives the account." {
		t.Errorf("c3 = %+v", c3)
	}
	c4 := th.Comments[2]
	if c4.AuthorDeleted || !c4.BodyRemoved || c4.Distinguished != "moderator" {
		t.Errorf("c4 = %+v", c4)
	}
	if c4.More == nil || c4.More.Count != 0 || len(c4.More.IDs) != 0 {
		t.Errorf("c4 continue stub = %+v", c4.More)
	}
	if th.More == nil || th.More.Count != 40 || th.More.ParentFullname != "t3_aaa" {
		t.Errorf("thread more = %+v", th.More)
	}
}

func TestParseThreadRejectsShortArray(t *testing.T) {
	if _, err := ParseThread(strings.NewReader(`[{"kind":"Listing","data":{"children":[]}}]`)); err == nil {
		t.Error("expected error")
	}
}

func TestParseMoreChildren(t *testing.T) {
	th, err := ParseMoreChildren(open(t, "morechildren.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 2 || th.Comments[0].ID != "y" || th.Comments[1].ID != "x" {
		t.Errorf("comments = %+v", th.Comments)
	}
	if len(th.Stubs) != 1 || th.Stubs[0].ParentFullname != "t1_x" || th.Stubs[0].IDs[0] != "z" {
		t.Errorf("stubs = %+v", th.Stubs)
	}
}

func TestParseMoreChildrenErrors(t *testing.T) {
	_, err := ParseMoreChildren(strings.NewReader(`{"json":{"errors":[["TOO_MANY","limit exceeded","children"]],"data":{"things":[]}}}`))
	if err == nil || !strings.Contains(err.Error(), "TOO_MANY") {
		t.Errorf("err = %v", err)
	}
}

func TestSorts(t *testing.T) {
	if Best.API() != "confidence" || TopComments.API() != "top" {
		t.Error("comment sort API mapping")
	}
	if Hot.Next() != New || Rising.Next() != Hot {
		t.Error("sort cycle")
	}
	if Best.Next() != TopComments || NewComments.Next() != Best {
		t.Error("comment sort cycle")
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/reddit/`
Expected: FAIL, undefined: ParseListing and friends.

- [ ] **Step 4: Implement models.go**

```go
// Package reddit fetches and parses Reddit listings and comment threads
// through the OAuth API using app-only credentials.
package reddit

import (
	"context"
	"time"
)

// Sort orders a subreddit's posts.
type Sort string

// Post sorts.
const (
	Hot    Sort = "hot"
	New    Sort = "new"
	Top    Sort = "top"
	Rising Sort = "rising"
)

// Next cycles hot, new, top, rising.
func (s Sort) Next() Sort {
	switch s {
	case Hot:
		return New
	case New:
		return Top
	case Top:
		return Rising
	default:
		return Hot
	}
}

// CommentSort orders a thread's comments.
type CommentSort string

// Comment sorts.
const (
	Best        CommentSort = "best"
	TopComments CommentSort = "top"
	NewComments CommentSort = "new"
)

// API returns the value Reddit expects in the sort parameter.
func (c CommentSort) API() string {
	if c == Best {
		return "confidence"
	}
	return string(c)
}

// Next cycles best, top, new.
func (c CommentSort) Next() CommentSort {
	switch c {
	case Best:
		return TopComments
	case TopComments:
		return NewComments
	default:
		return Best
	}
}

// Fetch carries per-call options.
type Fetch struct {
	Fresh bool // bypass and replace the cache entry
}

// Post is a submission.
type Post struct {
	ID, Fullname  string
	Subreddit     string
	Title, Author string
	Score         int
	NumComments   int
	Created       time.Time
	URL, Domain   string
	Permalink     string
	IsSelf        bool
	SelfText      string
	Stickied      bool
	Over18        bool
	Distinguished string
}

// IsOP reports whether author wrote the post.
func (p *Post) IsOP(author string) bool { return p != nil && author != "" && p.Author == author }

// Comment is one node of the thread tree.
type Comment struct {
	ID, Fullname   string
	ParentFullname string
	Author, Body   string
	Score          int
	Created        time.Time
	Depth          int
	IsSubmitter    bool
	Distinguished  string
	AuthorDeleted  bool
	BodyRemoved    bool
	Children       []*Comment
	More           *MoreStub // unloaded replies to this comment, if any
}

// MoreStub stands for replies not yet loaded. Count 0 with no IDs is
// Reddit's "continue this thread" marker.
type MoreStub struct {
	ParentFullname string
	Count          int
	IDs            []string
}

// IsContinue reports a "continue this thread" stub.
func (m *MoreStub) IsContinue() bool { return m.Count == 0 && len(m.IDs) == 0 }

// Listing is one page of posts.
type Listing struct {
	Posts []*Post
	After string // empty at the end
}

// Thread is a post with its comment forest.
type Thread struct {
	Post     *Post
	Comments []*Comment
	More     *MoreStub // unloaded top-level comments
}

// Things is the flat result of a morechildren call.
type Things struct {
	Comments []*Comment
	Stubs    []*MoreStub
}

// Store is what screens read from.
type Store interface {
	Posts(ctx context.Context, subreddit string, sort Sort, after string, f Fetch) (Listing, error)
	Thread(ctx context.Context, subreddit, postID string, sort CommentSort, f Fetch) (Thread, error)
	Subtree(ctx context.Context, subreddit, postID, commentID string, sort CommentSort) (Thread, error)
	MoreChildren(ctx context.Context, linkFullname string, ids []string, sort CommentSort) (Things, error)
}
```

- [ ] **Step 5: Implement parse.go**

```go
package reddit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type thing struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type listingData struct {
	After    string  `json:"after"`
	Children []thing `json:"children"`
}

type postData struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Subreddit     string  `json:"subreddit"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	Score         int     `json:"score"`
	NumComments   int     `json:"num_comments"`
	CreatedUTC    float64 `json:"created_utc"`
	URL           string  `json:"url"`
	Domain        string  `json:"domain"`
	Permalink     string  `json:"permalink"`
	IsSelf        bool    `json:"is_self"`
	Selftext      string  `json:"selftext"`
	Stickied      bool    `json:"stickied"`
	Over18        bool    `json:"over_18"`
	Distinguished *string `json:"distinguished"`
}

type commentData struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ParentID      string          `json:"parent_id"`
	Author        string          `json:"author"`
	Body          string          `json:"body"`
	Score         int             `json:"score"`
	CreatedUTC    float64         `json:"created_utc"`
	IsSubmitter   bool            `json:"is_submitter"`
	Distinguished *string         `json:"distinguished"`
	Replies       json.RawMessage `json:"replies"`
}

type moreData struct {
	ParentID string   `json:"parent_id"`
	Count    int      `json:"count"`
	Children []string `json:"children"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func unixTime(f float64) time.Time { return time.Unix(int64(f), 0).UTC() }

// ParseListing decodes a subreddit listing.
func ParseListing(r io.Reader) (Listing, error) {
	var t thing
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return Listing{}, fmt.Errorf("decode listing: %w", err)
	}
	return parseListing(t)
}

func parseListing(t thing) (Listing, error) {
	if t.Kind != "Listing" {
		return Listing{}, fmt.Errorf("expected Listing, got %q", t.Kind)
	}
	var ld listingData
	if err := json.Unmarshal(t.Data, &ld); err != nil {
		return Listing{}, fmt.Errorf("decode listing data: %w", err)
	}
	l := Listing{After: ld.After}
	for _, ch := range ld.Children {
		if ch.Kind != "t3" {
			continue
		}
		p, err := parsePost(ch.Data)
		if err != nil {
			return Listing{}, err
		}
		l.Posts = append(l.Posts, p)
	}
	return l, nil
}

func parsePost(raw json.RawMessage) (*Post, error) {
	var d postData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode post: %w", err)
	}
	return &Post{
		ID: d.ID, Fullname: d.Name, Subreddit: d.Subreddit, Title: d.Title, Author: d.Author,
		Score: d.Score, NumComments: d.NumComments, Created: unixTime(d.CreatedUTC),
		URL: d.URL, Domain: d.Domain, Permalink: d.Permalink, IsSelf: d.IsSelf, SelfText: d.Selftext,
		Stickied: d.Stickied, Over18: d.Over18, Distinguished: str(d.Distinguished),
	}, nil
}

// ParseThread decodes the two-element array returned by the comments endpoint.
func ParseThread(r io.Reader) (Thread, error) {
	var arr []thing
	if err := json.NewDecoder(r).Decode(&arr); err != nil {
		return Thread{}, fmt.Errorf("decode thread: %w", err)
	}
	if len(arr) < 2 {
		return Thread{}, errors.New("thread response has fewer than two listings")
	}
	posts, err := parseListing(arr[0])
	if err != nil {
		return Thread{}, err
	}
	if len(posts.Posts) == 0 {
		return Thread{}, errors.New("thread response has no post")
	}
	var ld listingData
	if arr[1].Kind != "Listing" {
		return Thread{}, fmt.Errorf("expected comment Listing, got %q", arr[1].Kind)
	}
	if err := json.Unmarshal(arr[1].Data, &ld); err != nil {
		return Thread{}, fmt.Errorf("decode comments: %w", err)
	}
	comments, more, err := parseForest(ld.Children, 0)
	if err != nil {
		return Thread{}, err
	}
	return Thread{Post: posts.Posts[0], Comments: comments, More: more}, nil
}

// parseForest converts a listing's children into comments at the given depth
// plus at most one trailing more stub. Unknown kinds are skipped.
func parseForest(children []thing, depth int) ([]*Comment, *MoreStub, error) {
	var out []*Comment
	var more *MoreStub
	for _, ch := range children {
		switch ch.Kind {
		case "t1":
			c, err := parseComment(ch.Data, depth)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, c)
		case "more":
			m, err := parseMore(ch.Data)
			if err != nil {
				return nil, nil, err
			}
			more = m
		}
	}
	return out, more, nil
}

func parseComment(raw json.RawMessage, depth int) (*Comment, error) {
	var d commentData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode comment: %w", err)
	}
	c := &Comment{
		ID: d.ID, Fullname: d.Name, ParentFullname: d.ParentID, Author: d.Author, Body: d.Body,
		Score: d.Score, Created: unixTime(d.CreatedUTC), Depth: depth, IsSubmitter: d.IsSubmitter,
		Distinguished: str(d.Distinguished),
		AuthorDeleted: d.Author == "[deleted]",
		BodyRemoved:   d.Body == "[deleted]" || d.Body == "[removed]",
	}
	// replies is "" when empty, or a Listing thing.
	if len(d.Replies) > 0 && d.Replies[0] == '{' {
		var rt thing
		if err := json.Unmarshal(d.Replies, &rt); err != nil {
			return nil, fmt.Errorf("decode replies: %w", err)
		}
		var ld listingData
		if err := json.Unmarshal(rt.Data, &ld); err != nil {
			return nil, fmt.Errorf("decode replies data: %w", err)
		}
		kids, more, err := parseForest(ld.Children, depth+1)
		if err != nil {
			return nil, err
		}
		c.Children, c.More = kids, more
	}
	return c, nil
}

func parseMore(raw json.RawMessage) (*MoreStub, error) {
	var d moreData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode more: %w", err)
	}
	return &MoreStub{ParentFullname: d.ParentID, Count: d.Count, IDs: d.Children}, nil
}

// ParseMoreChildren decodes a morechildren response into flat things.
func ParseMoreChildren(r io.Reader) (Things, error) {
	var resp struct {
		JSON struct {
			Errors [][]any `json:"errors"`
			Data   struct {
				Things []thing `json:"things"`
			} `json:"data"`
		} `json:"json"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return Things{}, fmt.Errorf("decode morechildren: %w", err)
	}
	if len(resp.JSON.Errors) > 0 {
		return Things{}, fmt.Errorf("reddit error: %v", resp.JSON.Errors[0])
	}
	var th Things
	for _, t := range resp.JSON.Data.Things {
		switch t.Kind {
		case "t1":
			c, err := parseComment(t.Data, 0)
			if err != nil {
				return Things{}, err
			}
			th.Comments = append(th.Comments, c)
		case "more":
			m, err := parseMore(t.Data)
			if err != nil {
				return Things{}, err
			}
			th.Stubs = append(th.Stubs, m)
		}
	}
	return th, nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/reddit/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/reddit
git commit -m "Add Reddit models and listing, thread and morechildren parsers

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Attach morechildren results into a thread

**Files:**
- Create: `internal/reddit/attach.go`
- Test: `internal/reddit/attach_test.go`

**Interfaces:**
- Consumes: `Thread`, `Comment`, `MoreStub`, `Things` from Task 6.
- Produces: `func Attach(t *Thread, stub *MoreStub, th Things) int` returning the number of comments attached. Removes `stub` from its parent, attaches in two passes by `ParentFullname`, re-adds a stub for IDs that were not returned, and sets `Depth` on attached comments.

- [ ] **Step 1: Write the failing tests**

`internal/reddit/attach_test.go`:

```go
package reddit

import "testing"

func fixtureThread(t *testing.T) Thread {
	t.Helper()
	th, err := ParseThread(open(t, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func TestAttachTwoPassOrder(t *testing.T) {
	th := fixtureThread(t)
	c1 := th.Comments[0]
	stub := c1.More
	things, err := ParseMoreChildren(open(t, "morechildren.json"))
	if err != nil {
		t.Fatal(err)
	}
	n := Attach(&th, stub, things)
	if n != 2 {
		t.Errorf("attached %d, want 2", n)
	}
	if len(c1.Children) != 2 || c1.Children[1].ID != "x" || c1.Children[1].Depth != 1 {
		t.Fatalf("c1 children = %+v", c1.Children)
	}
	x := c1.Children[1]
	if len(x.Children) != 1 || x.Children[0].ID != "y" || x.Children[0].Depth != 2 {
		t.Errorf("x children = %+v", x.Children)
	}
	if x.More == nil || x.More.IDs[0] != "z" {
		t.Errorf("x more = %+v", x.More)
	}
	// Original stub listed x, y, z; all were returned (z via the nested stub), so c1 has no stub left.
	if c1.More != nil {
		t.Errorf("c1 stub should be consumed, got %+v", c1.More)
	}
}

func TestAttachKeepsUnreturnedIDs(t *testing.T) {
	th := fixtureThread(t)
	c1 := th.Comments[0]
	things := Things{Comments: []*Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Body: "x"}}}
	Attach(&th, c1.More, things)
	if c1.More == nil || c1.More.Count != 2 || len(c1.More.IDs) != 2 || c1.More.IDs[0] != "y" || c1.More.IDs[1] != "z" {
		t.Errorf("remaining stub = %+v", c1.More)
	}
}

func TestAttachTopLevelAndOrphans(t *testing.T) {
	th := fixtureThread(t)
	things := Things{Comments: []*Comment{
		{ID: "p", Fullname: "t1_p", ParentFullname: "t3_aaa", Body: "top"},
		{ID: "orphan", Fullname: "t1_orphan", ParentFullname: "t1_missing", Body: "lost"},
		{ID: "c2", Fullname: "t1_c2", ParentFullname: "t1_c1", Body: "duplicate of loaded"},
	}}
	n := Attach(&th, th.More, things)
	if n != 1 {
		t.Errorf("attached %d, want 1", n)
	}
	if len(th.Comments) != 4 || th.Comments[3].ID != "p" || th.Comments[3].Depth != 0 {
		t.Errorf("top-level = %d", len(th.Comments))
	}
	if th.More == nil || len(th.More.IDs) != 1 || th.More.IDs[0] != "q" {
		t.Errorf("thread stub = %+v", th.More)
	}
	if len(th.Comments[0].Children) != 1 {
		t.Error("duplicate c2 should not be attached twice")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reddit/ -run Attach`
Expected: FAIL, undefined: Attach.

- [ ] **Step 3: Implement attach.go**

```go
package reddit

// Attach splices the result of a MoreChildren call for stub into t.
//
// It removes stub from its parent, attaches every returned comment whose
// parent is loaded or in the response (two passes, so order does not
// matter), attaches returned stubs, and re-adds a stub for any of stub's IDs
// the response did not cover. It returns the number of comments attached.
func Attach(t *Thread, stub *MoreStub, th Things) int {
	index := map[string]*Comment{}
	var walk func([]*Comment)
	walk = func(cs []*Comment) {
		for _, c := range cs {
			index[c.Fullname] = c
			walk(c.Children)
		}
	}
	walk(t.Comments)

	postName := ""
	if t.Post != nil {
		postName = t.Post.Fullname
	}
	setStub := func(parent string, s *MoreStub) {
		if parent == postName {
			t.More = s
		} else if p, ok := index[parent]; ok {
			p.More = s
		}
	}
	if stub != nil {
		setStub(stub.ParentFullname, nil)
	}

	returned := map[string]bool{}
	pending := append([]*Comment(nil), th.Comments...)
	attached := 0
	for progress := true; progress && len(pending) > 0; {
		progress = false
		var rest []*Comment
		for _, c := range pending {
			returned[c.ID] = true
			if _, dup := index[c.Fullname]; dup {
				progress = true
				continue
			}
			switch p, ok := index[c.ParentFullname]; {
			case c.ParentFullname == postName:
				c.Depth = 0
				t.Comments = append(t.Comments, c)
			case ok:
				c.Depth = p.Depth + 1
				p.Children = append(p.Children, c)
			default:
				rest = append(rest, c)
				continue
			}
			index[c.Fullname] = c
			attached++
			progress = true
		}
		pending = rest
	}

	for _, s := range th.Stubs {
		for _, id := range s.IDs {
			returned[id] = true
		}
		setStub(s.ParentFullname, s)
	}

	if stub != nil {
		var remaining []string
		for _, id := range stub.IDs {
			if !returned[id] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) > 0 {
			ns := &MoreStub{ParentFullname: stub.ParentFullname, Count: len(remaining), IDs: remaining}
			if existing := currentStub(t, index, stub.ParentFullname); existing != nil {
				ns.IDs = append(existing.IDs, remaining...)
				ns.Count = len(ns.IDs)
			}
			setStub(stub.ParentFullname, ns)
		}
	}
	return attached
}

func currentStub(t *Thread, index map[string]*Comment, parent string) *MoreStub {
	if t.Post != nil && parent == t.Post.Fullname {
		return t.More
	}
	if p, ok := index[parent]; ok {
		return p.More
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/reddit/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/reddit/attach.go internal/reddit/attach_test.go
git commit -m "Splice morechildren results into a thread tree

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: OAuth token source

**Files:**
- Create: `internal/reddit/token.go`
- Test: `internal/reddit/token_test.go`

**Interfaces:**
- Produces:
  - `type TokenConfig struct { ClientID, ClientSecret, UserAgent, URL string; HTTP *http.Client; Now func() time.Time }`.
  - `func NewTokenSource(cfg TokenConfig) *TokenSource` (defaults: URL `https://www.reddit.com/api/v1/access_token`, `http.DefaultClient` with 15 s timeout, `time.Now`).
  - `func (t *TokenSource) Token(ctx context.Context) (string, error)`.
  - `func (t *TokenSource) Invalidate(rejected string)` clears the token only if it still equals `rejected`.

- [ ] **Step 1: Write the failing tests**

`internal/reddit/token_test.go`:

```go
package reddit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time             { return c.t }
func (c *fakeClock) Advance(d time.Duration)    { c.t = c.t.Add(d) }

func tokenServer(t *testing.T, hits *int32, expires int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/access_token" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "myid" || secret != "mysecret" {
			w.WriteHeader(401)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("bad form: %v", r.Form)
		}
		if r.UserAgent() != "test-agent/1" {
			t.Errorf("user agent = %q", r.UserAgent())
		}
		n := atomic.AddInt32(hits, 1)
		fmt.Fprintf(w, `{"access_token":"tok%d","token_type":"bearer","expires_in":%d,"scope":"*"}`, n, expires)
	}))
}

func TestTokenCachedUntilNearExpiry(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	ts := NewTokenSource(TokenConfig{ClientID: "myid", ClientSecret: "mysecret", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token", Now: clock.Now})
	tok, err := ts.Token(context.Background())
	if err != nil || tok != "tok1" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	clock.Advance(3000 * time.Second)
	if tok, _ := ts.Token(context.Background()); tok != "tok1" {
		t.Errorf("token should still be cached, got %q", tok)
	}
	clock.Advance(550 * time.Second) // 50 s left: inside the 60 s refresh window
	if tok, _ := ts.Token(context.Background()); tok != "tok2" {
		t.Errorf("token should refresh near expiry, got %q", tok)
	}
	if hits != 2 {
		t.Errorf("token endpoint hit %d times, want 2", hits)
	}
}

func TestInvalidateOnlyMatchingToken(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	ts := NewTokenSource(TokenConfig{ClientID: "myid", ClientSecret: "mysecret", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token"})
	tok, _ := ts.Token(context.Background())
	ts.Invalidate("stale")
	if again, _ := ts.Token(context.Background()); again != tok {
		t.Error("invalidating a stale token must not refresh")
	}
	ts.Invalidate(tok)
	if again, _ := ts.Token(context.Background()); again == tok {
		t.Error("invalidating the current token must refresh")
	}
}

func TestTokenBadCredentials(t *testing.T) {
	var hits int32
	srv := tokenServer(t, &hits, 3600)
	defer srv.Close()
	ts := NewTokenSource(TokenConfig{ClientID: "wrong", ClientSecret: "x", UserAgent: "test-agent/1", URL: srv.URL + "/api/v1/access_token"})
	if _, err := ts.Token(context.Background()); err == nil {
		t.Error("expected error for rejected credentials")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reddit/ -run Token`
Expected: FAIL, undefined: NewTokenSource, TokenConfig.

- [ ] **Step 3: Implement token.go**

```go
package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultTokenURL is Reddit's OAuth token endpoint.
const DefaultTokenURL = "https://www.reddit.com/api/v1/access_token"

// TokenConfig configures a TokenSource. Zero fields take defaults.
type TokenConfig struct {
	ClientID, ClientSecret string
	UserAgent              string
	URL                    string
	HTTP                   *http.Client
	Now                    func() time.Time
}

// TokenSource obtains and refreshes an app-only bearer token.
type TokenSource struct {
	cfg TokenConfig

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewTokenSource creates a token source with the client-credentials grant.
func NewTokenSource(cfg TokenConfig) *TokenSource {
	if cfg.URL == "" {
		cfg.URL = DefaultTokenURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &TokenSource{cfg: cfg}
}

// Token returns a valid bearer token, refreshing when fewer than 60 seconds remain.
func (t *TokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && t.cfg.Now().Add(60*time.Second).Before(t.expiry) {
		return t.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(t.cfg.ClientID, t.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", t.cfg.UserAgent)
	resp, err := t.cfg.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: HTTP %d", resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token response: %s", firstNonEmpty(tr.Error, "no access_token"))
	}
	t.token = tr.AccessToken
	t.expiry = t.cfg.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return t.token, nil
}

// Invalidate discards the current token if it is the one that was rejected,
// so several callers seeing the same 401 cause one refresh.
func (t *TokenSource) Invalidate(rejected string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == rejected {
		t.token = ""
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/reddit/ -run Token`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/reddit/token.go internal/reddit/token_test.go
git commit -m "Add app-only OAuth token source

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Rate gate and cache

**Files:**
- Create: `internal/reddit/ratelimit.go`, `internal/reddit/cache.go`
- Test: `internal/reddit/ratelimit_test.go`, `internal/reddit/cache_test.go`

**Interfaces:**
- Produces:
  - `type RateGate` with `func NewRateGate(now func() time.Time, sleep func(context.Context, time.Duration) error, onWait func(time.Duration)) *RateGate`, `Acquire(ctx) error`, `Release(h http.Header)`, `WaitFor(h http.Header) time.Duration` (Retry-After or reset).
  - `type Cache` with `func NewCache(capacity int, now func() time.Time) *Cache`, `Get(key string) ([]byte, bool)`, `Put(key string, v []byte, ttl time.Duration)`, `Len() int`.
  - `func SleepContext(ctx context.Context, d time.Duration) error` real sleep honouring cancellation.

- [ ] **Step 1: Write the failing tests**

`internal/reddit/ratelimit_test.go`:

```go
package reddit

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fakeSleeper struct {
	clock *fakeClock
	slept []time.Duration
}

func (s *fakeSleeper) Sleep(_ context.Context, d time.Duration) error {
	s.slept = append(s.slept, d)
	s.clock.Advance(d)
	return nil
}

func headers(remaining, reset string) http.Header {
	h := http.Header{}
	h.Set("X-Ratelimit-Remaining", remaining)
	h.Set("X-Ratelimit-Reset", reset)
	return h
}

func TestRateGateWaitsWhenExhausted(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	var waited []time.Duration
	g := NewRateGate(clock.Now, sl.Sleep, func(d time.Duration) { waited = append(waited, d) })
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	g.Release(headers("1", "30"))
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 1 || sl.slept[0] != 30*time.Second {
		t.Errorf("slept %v, want [30s]", sl.slept)
	}
	if len(waited) != 1 {
		t.Errorf("onWait called %d times", len(waited))
	}
}

func TestRateGateReservesInFlight(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	g := NewRateGate(clock.Now, sl.Sleep, nil)
	_ = g.Acquire(context.Background())
	g.Release(headers("2", "60"))
	_ = g.Acquire(context.Background()) // remaining 2 - 0 in flight = 2, allowed
	_ = g.Acquire(context.Background()) // 2 - 1 = 1 < 2, must wait for reset
	if len(sl.slept) != 1 {
		t.Errorf("slept %v, want one wait", sl.slept)
	}
}

func TestRateGateFallbackWithoutHeaders(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	g := NewRateGate(clock.Now, sl.Sleep, nil)
	for i := 0; i < 59; i++ {
		_ = g.Acquire(context.Background())
		g.Release(http.Header{})
	}
	_ = g.Acquire(context.Background()) // remaining is 1: below the headroom of 2, so wait for the assumed reset
	g.Release(nil)
	if len(sl.slept) != 1 {
		t.Errorf("expected one wait after 60 unheadered requests, slept %v", sl.slept)
	}
}

func TestRateGateCancelled(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	g := NewRateGate(clock.Now, SleepContext, nil)
	_ = g.Acquire(context.Background())
	g.Release(headers("0", "600"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Acquire(ctx); err == nil {
		t.Error("expected context error")
	}
}

func TestWaitForPrefersRetryAfter(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	g := NewRateGate(clock.Now, nil, nil)
	h := headers("0", "45")
	h.Set("Retry-After", "7")
	if d := g.WaitFor(h); d != 7*time.Second {
		t.Errorf("WaitFor = %v", d)
	}
	if d := g.WaitFor(headers("0", "45")); d != 45*time.Second {
		t.Errorf("WaitFor reset = %v", d)
	}
	if d := g.WaitFor(http.Header{}); d != time.Second {
		t.Errorf("WaitFor empty = %v", d)
	}
}
```

`internal/reddit/cache_test.go`:

```go
package reddit

import (
	"testing"
	"time"
)

func TestCacheTTL(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(10, clock.Now)
	c.Put("a", []byte("1"), time.Minute)
	if v, ok := c.Get("a"); !ok || string(v) != "1" {
		t.Fatal("miss")
	}
	clock.Advance(61 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Error("expired entry returned")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(2, clock.Now)
	c.Put("a", []byte("1"), time.Hour)
	c.Put("b", []byte("2"), time.Hour)
	c.Get("a") // a is now most recent
	c.Put("c", []byte("3"), time.Hour)
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should survive")
	}
	if c.Len() != 2 {
		t.Errorf("len = %d", c.Len())
	}
}

func TestCachePutReplaces(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(2, clock.Now)
	c.Put("a", []byte("1"), time.Hour)
	c.Put("a", []byte("2"), time.Hour)
	if v, _ := c.Get("a"); string(v) != "2" || c.Len() != 1 {
		t.Errorf("v=%s len=%d", v, c.Len())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reddit/ -run 'RateGate|WaitFor|Cache'`
Expected: FAIL, undefined: NewRateGate, NewCache, SleepContext.

- [ ] **Step 3: Implement ratelimit.go**

```go
package reddit

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	fallbackPerMinute = 60
	minHeadroom       = 2
)

// RateGate paces requests from Reddit's X-Ratelimit headers, reserving one
// unit per in-flight request and waiting for the reset when headroom runs out.
type RateGate struct {
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)

	mu        sync.Mutex
	remaining float64
	reset     time.Time
	inflight  int
}

// NewRateGate creates a gate. sleep defaults to SleepContext; onWait may be nil.
func NewRateGate(now func() time.Time, sleep func(context.Context, time.Duration) error, onWait func(time.Duration)) *RateGate {
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = SleepContext
	}
	return &RateGate{now: now, sleep: sleep, onWait: onWait, remaining: fallbackPerMinute}
}

// SleepContext sleeps for d or until ctx is done.
func SleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Acquire blocks until a request may be sent, then reserves a unit.
func (g *RateGate) Acquire(ctx context.Context) error {
	for {
		g.mu.Lock()
		now := g.now()
		if !g.reset.IsZero() && !now.Before(g.reset) {
			g.remaining = fallbackPerMinute
			g.reset = time.Time{}
		}
		if g.remaining-float64(g.inflight) >= minHeadroom {
			g.inflight++
			g.mu.Unlock()
			return nil
		}
		wait := g.reset.Sub(now)
		if wait <= 0 {
			wait = time.Second
		}
		g.mu.Unlock()
		if g.onWait != nil {
			g.onWait(wait)
		}
		if err := g.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// Release records a response's headers and frees the reservation. Missing
// headers count down an assumed allowance of 60 per minute.
func (g *RateGate) Release(h http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inflight--
	if r := h.Get("X-Ratelimit-Remaining"); r != "" {
		if f, err := strconv.ParseFloat(r, 64); err == nil {
			g.remaining = f
		}
	} else {
		g.remaining--
	}
	if s := h.Get("X-Ratelimit-Reset"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			g.reset = g.now().Add(time.Duration(secs) * time.Second)
		}
	} else if g.reset.IsZero() {
		g.reset = g.now().Add(time.Minute)
	}
}

// WaitFor returns how long a 429 response asks us to wait: Retry-After, else
// the ratelimit reset, else one second.
func (g *RateGate) WaitFor(h http.Header) time.Duration {
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if s := h.Get("X-Ratelimit-Reset"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Second
}
```

- [ ] **Step 4: Implement cache.go**

```go
package reddit

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	key     string
	value   []byte
	expires time.Time
}

// Cache is a bounded LRU of response bodies with per-entry TTL. Storing bytes
// rather than parsed values means every caller gets its own parse.
type Cache struct {
	capacity int
	now      func() time.Time

	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element
}

// NewCache creates a cache holding at most capacity entries.
func NewCache(capacity int, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{capacity: capacity, now: now, ll: list.New(), items: map[string]*list.Element{}}
}

// Get returns a live entry and marks it most recently used.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if !c.now().Before(e.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.ll.MoveToFront(el)
	return e.value, true
}

// Put stores or replaces an entry, evicting the least recently used beyond capacity.
func (c *Cache) Put(key string, v []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value = &cacheEntry{key: key, value: v, expires: c.now().Add(ttl)}
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&cacheEntry{key: key, value: v, expires: c.now().Add(ttl)})
	for c.ll.Len() > c.capacity {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).key)
	}
}

// Len is the number of entries, expired or not.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/reddit/`
Expected: PASS. In `TestRateGateFallbackWithoutHeaders`: 59 released requests without headers take remaining from 60 to 1; the 60th Acquire sees 1 < 2 and waits once until the assumed one-minute reset, after which remaining is restored to 60.

- [ ] **Step 6: Commit**

```bash
git add internal/reddit/ratelimit.go internal/reddit/ratelimit_test.go internal/reddit/cache.go internal/reddit/cache_test.go
git commit -m "Add rate gate and LRU response cache

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: Reddit HTTP client implementing Store

**Files:**
- Create: `internal/reddit/client.go`
- Test: `internal/reddit/client_test.go`

**Interfaces:**
- Consumes: parsers (Task 6), `TokenSource` (Task 8), `RateGate`, `Cache`, `SleepContext` (Task 9).
- Produces:
  - `type Credentials struct { ClientID, ClientSecret, UserAgent string }`.
  - `type Option func(*Client)`: `WithBaseURL(string)`, `WithTokenURL(string)`, `WithHTTPClient(*http.Client)`, `WithClock(func() time.Time)`, `WithSleep(func(context.Context, time.Duration) error)`, `WithOnWait(func(time.Duration))`.
  - `func NewClient(c Credentials, opts ...Option) *Client`; `*Client` satisfies `Store`.
  - `type APIError struct { Status int; Reason string }` with `Error()`.
  - `func (c *Client) Verify(ctx context.Context) error` fetches `Posts("linux", Hot, "")` fresh and returns its error; used by New User Setup.

- [ ] **Step 1: Write the failing tests**

`internal/reddit/client_test.go`:

```go
package reddit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type apiServer struct {
	t        *testing.T
	mu       sync.Mutex
	tokenHit int
	requests []*http.Request
	handler  func(w http.ResponseWriter, r *http.Request, n int) bool // return true if handled
}

func (s *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if r.URL.Path == "/api/v1/access_token" {
		s.tokenHit++
		n := s.tokenHit
		s.mu.Unlock()
		fmt.Fprintf(w, `{"access_token":"tok%d","token_type":"bearer","expires_in":86400}`, n)
		return
	}
	s.requests = append(s.requests, r)
	n := len(s.requests)
	s.mu.Unlock()
	if s.handler != nil && s.handler(w, r, n) {
		return
	}
	w.Header().Set("X-Ratelimit-Remaining", "95")
	w.Header().Set("X-Ratelimit-Reset", "300")
	switch {
	case strings.HasPrefix(r.URL.Path, "/r/linux/comments/"):
		http.ServeFile(w, r, "testdata/thread.json")
	case r.URL.Path == "/api/morechildren":
		http.ServeFile(w, r, "testdata/morechildren.json")
	default:
		http.ServeFile(w, r, "testdata/listing.json")
	}
}

func newTestClient(t *testing.T, srv *apiServer) (*Client, *httptest.Server, *fakeClock, *fakeSleeper) {
	t.Helper()
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	sl := &fakeSleeper{clock: clock}
	c := NewClient(Credentials{ClientID: "id", ClientSecret: "sec", UserAgent: "test-agent/1"},
		WithBaseURL(hs.URL), WithTokenURL(hs.URL+"/api/v1/access_token"), WithClock(clock.Now), WithSleep(sl.Sleep))
	return c, hs, clock, sl
}

func TestPostsRequestShape(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	l, err := c.Posts(context.Background(), "linux", Top, "t3_prev", Fetch{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Posts) != 2 {
		t.Errorf("posts = %d", len(l.Posts))
	}
	r := srv.requests[0]
	if r.URL.Path != "/r/linux/top" {
		t.Errorf("path = %s", r.URL.Path)
	}
	q := r.URL.Query()
	if q.Get("limit") != "100" || q.Get("after") != "t3_prev" || q.Get("raw_json") != "1" || q.Get("t") != "day" {
		t.Errorf("query = %v", q)
	}
	if r.Header.Get("Authorization") != "bearer tok1" || r.UserAgent() != "test-agent/1" {
		t.Errorf("headers = %v", r.Header)
	}
}

func TestPostsCacheAndFresh(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, clock, _ := newTestClient(t, srv)
	ctx := context.Background()
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	if len(srv.requests) != 1 {
		t.Fatalf("cache miss: %d requests", len(srv.requests))
	}
	c.Posts(ctx, "linux", Hot, "", Fetch{Fresh: true})
	if len(srv.requests) != 2 {
		t.Fatalf("Fresh should bypass cache: %d requests", len(srv.requests))
	}
	clock.Advance(6 * time.Minute)
	c.Posts(ctx, "linux", Hot, "", Fetch{})
	if len(srv.requests) != 3 {
		t.Fatalf("listing TTL should be 5 minutes: %d requests", len(srv.requests))
	}
}

func TestCachedResultsAreIndependent(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	a, _ := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	a.Posts[0].Title = "mutated"
	b, _ := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	if b.Posts[0].Title == "mutated" {
		t.Error("cached value leaked a mutation")
	}
}

func TestRefreshesTokenOn401Once(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.WriteHeader(401)
			return true
		}
		return false
	}
	c, _, _, _ := newTestClient(t, srv)
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Fatal(err)
	}
	if srv.tokenHit != 2 || srv.requests[1].Header.Get("Authorization") != "bearer tok2" {
		t.Errorf("tokenHit=%d auth=%q", srv.tokenHit, srv.requests[1].Header.Get("Authorization"))
	}
}

func TestPersistent401IsAPIError(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool { w.WriteHeader(401); return true }
	c, _, _, _ := newTestClient(t, srv)
	_, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Errorf("err = %v", err)
	}
	if len(srv.requests) != 2 {
		t.Errorf("requests = %d, want exactly one retry", len(srv.requests))
	}
}

func TestRetriesOnceAfter429(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if n == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return true
		}
		return false
	}
	c, _, _, sl := newTestClient(t, srv)
	var waits []time.Duration
	c.onWait = func(d time.Duration) { waits = append(waits, d) }
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 1 || sl.slept[0] != 3*time.Second || len(waits) != 1 {
		t.Errorf("slept %v waits %v", sl.slept, waits)
	}
}

func TestForbiddenCarriesReason(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"reason":"private","message":"Forbidden","error":403}`)
		return true
	}
	c, _, _, _ := newTestClient(t, srv)
	_, err := c.Posts(context.Background(), "secret", Hot, "", Fetch{})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 403 || ae.Reason != "private" {
		t.Errorf("err = %v", err)
	}
}

func TestThreadAndSubtreeQueries(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	ctx := context.Background()
	if _, err := c.Thread(ctx, "linux", "aaa", Best, Fetch{}); err != nil {
		t.Fatal(err)
	}
	q := srv.requests[0].URL.Query()
	if srv.requests[0].URL.Path != "/r/linux/comments/aaa" || q.Get("sort") != "confidence" || q.Get("limit") != "500" || q.Get("depth") != "10" {
		t.Errorf("thread request = %s %v", srv.requests[0].URL.Path, q)
	}
	if _, err := c.Subtree(ctx, "linux", "aaa", "c4", TopComments); err != nil {
		t.Fatal(err)
	}
	q = srv.requests[1].URL.Query()
	if q.Get("comment") != "c4" || q.Get("context") != "0" || q.Get("sort") != "top" {
		t.Errorf("subtree query = %v", q)
	}
}

func TestMoreChildrenBatches(t *testing.T) {
	srv := &apiServer{t: t}
	c, _, _, _ := newTestClient(t, srv)
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i)
	}
	th, err := c.MoreChildren(context.Background(), "aaa", ids, Best)
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.requests) != 3 {
		t.Fatalf("requests = %d, want 3 batches", len(srv.requests))
	}
	sizes := []int{100, 100, 50}
	for i, r := range srv.requests {
		q := r.URL.Query()
		if r.URL.Path != "/api/morechildren" || q.Get("link_id") != "t3_aaa" || q.Get("api_type") != "json" || q.Get("sort") != "confidence" {
			t.Errorf("request %d = %s %v", i, r.URL.Path, q)
		}
		if got := len(strings.Split(q.Get("children"), ",")); got != sizes[i] {
			t.Errorf("batch %d size = %d, want %d", i, got, sizes[i])
		}
	}
	if len(th.Comments) != 6 || len(th.Stubs) != 3 {
		t.Errorf("things = %d comments %d stubs", len(th.Comments), len(th.Stubs))
	}
	c.MoreChildren(context.Background(), "t3_aaa", ids[:1], Best)
	if srv.requests[3].URL.Query().Get("link_id") != "t3_aaa" {
		t.Error("link_id should not be double-prefixed")
	}
}

func TestBodyLimit(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		w.Write([]byte(strings.Repeat("x", 11<<20)))
		return true
	}
	c, _, _, _ := newTestClient(t, srv)
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err == nil {
		t.Error("oversized body should fail to parse, not hang")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reddit/ -run 'Posts|Cached|Refreshes|Persistent|Retries|Forbidden|ThreadAnd|MoreChildrenBatches|BodyLimit'`
Expected: FAIL, undefined: NewClient, Credentials, WithBaseURL, APIError.

- [ ] **Step 3: Implement client.go**

```go
package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the OAuth API host.
const DefaultBaseURL = "https://oauth.reddit.com"

const (
	maxBody       = 10 << 20
	listingTTL    = 5 * time.Minute
	threadTTL     = 10 * time.Minute
	cacheEntries  = 200
	moreBatchSize = 100
)

// Credentials identify the registered script app.
type Credentials struct {
	ClientID, ClientSecret, UserAgent string
}

// APIError is a non-2xx response.
type APIError struct {
	Status int
	Reason string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("HTTP %d (%s)", e.Status, e.Reason)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// Client talks to the Reddit API. It satisfies Store.
type Client struct {
	base   string
	http   *http.Client
	ua     string
	tokens *TokenSource
	gate   *RateGate
	cache  *Cache
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	onWait func(time.Duration)
	moreMu sync.Mutex

	tokenURL string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API host (tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.base = strings.TrimRight(u, "/") } }

// WithTokenURL overrides the token endpoint (tests).
func WithTokenURL(u string) Option { return func(c *Client) { c.tokenURL = u } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithClock overrides the clock.
func WithClock(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// WithSleep overrides how the client waits (tests).
func WithSleep(s func(context.Context, time.Duration) error) Option { return func(c *Client) { c.sleep = s } }

// WithOnWait sets a callback invoked with the duration before any rate-limit wait.
func WithOnWait(f func(time.Duration)) Option { return func(c *Client) { c.onWait = f } }

// NewClient builds a client with token handling, rate gating and caching.
func NewClient(cr Credentials, opts ...Option) *Client {
	c := &Client{
		base:     DefaultBaseURL,
		http:     &http.Client{Timeout: 15 * time.Second},
		ua:       cr.UserAgent,
		now:      time.Now,
		sleep:    SleepContext,
		tokenURL: DefaultTokenURL,
	}
	for _, o := range opts {
		o(c)
	}
	c.tokens = NewTokenSource(TokenConfig{ClientID: cr.ClientID, ClientSecret: cr.ClientSecret, UserAgent: cr.UserAgent, URL: c.tokenURL, HTTP: c.http, Now: c.now})
	c.gate = NewRateGate(c.now, c.sleep, func(d time.Duration) {
		if c.onWait != nil {
			c.onWait(d)
		}
	})
	c.cache = NewCache(cacheEntries, c.now)
	return c
}

// Verify proves the credentials work by fetching a public listing.
func (c *Client) Verify(ctx context.Context) error {
	_, err := c.Posts(ctx, "linux", Hot, "", Fetch{Fresh: true})
	return err
}

// Posts fetches one page of a subreddit.
func (c *Client) Posts(ctx context.Context, subreddit string, sort Sort, after string, f Fetch) (Listing, error) {
	q := url.Values{"limit": {"100"}}
	if after != "" {
		q.Set("after", after)
	}
	if sort == Top {
		q.Set("t", "day")
	}
	b, err := c.get(ctx, "/r/"+subreddit+"/"+string(sort), q, f.Fresh, listingTTL)
	if err != nil {
		return Listing{}, err
	}
	return ParseListing(bytes.NewReader(b))
}

// Thread fetches a post and its comment forest.
func (c *Client) Thread(ctx context.Context, subreddit, postID string, sort CommentSort, f Fetch) (Thread, error) {
	q := url.Values{"sort": {sort.API()}, "limit": {"500"}, "depth": {"10"}}
	b, err := c.get(ctx, "/r/"+subreddit+"/comments/"+postID, q, f.Fresh, threadTTL)
	if err != nil {
		return Thread{}, err
	}
	return ParseThread(bytes.NewReader(b))
}

// Subtree fetches the thread rooted at one comment.
func (c *Client) Subtree(ctx context.Context, subreddit, postID, commentID string, sort CommentSort) (Thread, error) {
	q := url.Values{"sort": {sort.API()}, "limit": {"500"}, "depth": {"10"}, "comment": {commentID}, "context": {"0"}}
	b, err := c.get(ctx, "/r/"+subreddit+"/comments/"+postID, q, false, threadTTL)
	if err != nil {
		return Thread{}, err
	}
	return ParseThread(bytes.NewReader(b))
}

// MoreChildren loads comments by ID in batches of 100, one request at a time.
func (c *Client) MoreChildren(ctx context.Context, linkFullname string, ids []string, sort CommentSort) (Things, error) {
	c.moreMu.Lock()
	defer c.moreMu.Unlock()
	link := "t3_" + strings.TrimPrefix(linkFullname, "t3_")
	var all Things
	for start := 0; start < len(ids); start += moreBatchSize {
		end := start + moreBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		q := url.Values{
			"link_id":  {link},
			"children": {strings.Join(ids[start:end], ",")},
			"sort":     {sort.API()},
			"api_type": {"json"},
		}
		b, err := c.get(ctx, "/api/morechildren", q, true, 0)
		if err != nil {
			return all, err
		}
		th, err := ParseMoreChildren(bytes.NewReader(b))
		if err != nil {
			return all, err
		}
		all.Comments = append(all.Comments, th.Comments...)
		all.Stubs = append(all.Stubs, th.Stubs...)
	}
	return all, nil
}

// get performs a cached GET. ttl 0 disables caching.
func (c *Client) get(ctx context.Context, path string, q url.Values, fresh bool, ttl time.Duration) ([]byte, error) {
	q.Set("raw_json", "1")
	u := c.base + path + "?" + q.Encode()
	if !fresh && ttl > 0 {
		if b, ok := c.cache.Get(u); ok {
			return b, nil
		}
	}
	b, err := c.do(ctx, u)
	if err != nil {
		return nil, err
	}
	if ttl > 0 {
		c.cache.Put(u, b, ttl)
	}
	return b, nil
}

// do sends one authenticated GET, refreshing the token once on 401 and
// retrying once after a 429.
func (c *Client) do(ctx context.Context, u string) ([]byte, error) {
	retried401, retried429 := false, false
	for {
		tok, err := c.tokens.Token(ctx)
		if err != nil {
			return nil, err
		}
		if err := c.gate.Acquire(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			c.gate.Release(nil)
			return nil, err
		}
		req.Header.Set("Authorization", "bearer "+tok)
		req.Header.Set("User-Agent", c.ua)
		resp, err := c.http.Do(req)
		if err != nil {
			c.gate.Release(nil)
			return nil, fmt.Errorf("request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		c.gate.Release(resp.Header)
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, readErr
		case resp.StatusCode == http.StatusUnauthorized && !retried401:
			retried401 = true
			c.tokens.Invalidate(tok)
		case resp.StatusCode == http.StatusTooManyRequests && !retried429:
			retried429 = true
			wait := c.gate.WaitFor(resp.Header)
			if c.onWait != nil {
				c.onWait(wait)
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
		default:
			return nil, &APIError{Status: resp.StatusCode, Reason: reason(body)}
		}
	}
}

// reason extracts Reddit's "reason" field from an error body, if any.
func reason(body []byte) string {
	var r struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(body, &r) == nil {
		return r.Reason
	}
	return ""
}

var _ Store = (*Client)(nil)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/reddit/`
Expected: PASS. `TestBodyLimit` expects a parse error because the truncated body is not JSON; it must complete in well under a second.

- [ ] **Step 5: Commit**

```bash
git add internal/reddit/client.go internal/reddit/client_test.go
git commit -m "Add Reddit API client with auth, rate gating and caching

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 11: config package

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type Area struct { Name string; Subreddit string }`.
  - `type Config` with fields `Reddit struct{ ClientID, ClientSecret, UserAgent string }`, `Display struct{ PeekPane bool; DefaultSort string }`, `Areas []Area`, `Path string`.
  - `func DefaultPath() (string, error)`; `func Load(path string, getenv func(string) string) (*Config, error)`.
  - Methods: `ClientID() string`, `ClientSecret() string`, `HasCredentials() bool`, `SetCredentials(id, secret string)`, `RemoveArea(i int)`, `AddArea(a Area) bool` (false when the subreddit already exists, case-insensitive), `HasArea(subreddit string) bool`, `Save() error`.
  - `var DefaultAreas []Area`; `const DefaultUserAgent = "linux:redditbbs:0.1.0 (by /u/redditbbs)"`.

- [ ] **Step 1: Write the failing tests**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.toml"), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.HasCredentials() || !c.Display.PeekPane || c.Display.DefaultSort != "hot" || len(c.Areas) != 0 {
		t.Errorf("defaults = %+v", c)
	}
	if c.Reddit.UserAgent != DefaultUserAgent {
		t.Errorf("user agent = %q", c.Reddit.UserAgent)
	}
}

func TestLoadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte(`
[reddit]
client_id = "fileid"
client_secret = "filesecret"

[display]
peek_pane = false

[[areas]]
name = "Linux"
subreddit = "linux"
`), 0o600)
	c, err := Load(p, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientID() != "fileid" || c.ClientSecret() != "filesecret" || !c.HasCredentials() {
		t.Errorf("creds = %q %q", c.ClientID(), c.ClientSecret())
	}
	if c.Display.PeekPane || len(c.Areas) != 1 || c.Areas[0].Subreddit != "linux" {
		t.Errorf("config = %+v", c)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[reddit]\nclient_id = \"fileid\"\nclient_secret = \"filesecret\"\n"), 0o600)
	env := map[string]string{"REDDITBBS_CLIENT_ID": "envid"}
	c, _ := Load(p, func(k string) string { return env[k] })
	if c.ClientID() != "envid" || c.ClientSecret() != "filesecret" {
		t.Errorf("creds = %q %q", c.ClientID(), c.ClientSecret())
	}
}

func TestMalformedFileReportsLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[reddit\nclient_id = 1"), 0o600)
	_, err := Load(p, noEnv)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Errorf("err = %v", err)
	}
}

func TestSaveRoundTripAndMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	c, _ := Load(p, noEnv)
	c.SetCredentials("id1", "sec1")
	c.AddArea(Area{Name: "Linux", Subreddit: "linux"})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", st.Mode().Perm())
	}
	if dst, _ := os.Stat(filepath.Dir(p)); dst.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o", dst.Mode().Perm())
	}
	again, err := Load(p, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if again.ClientID() != "id1" || again.ClientSecret() != "sec1" || len(again.Areas) != 1 {
		t.Errorf("round trip = %+v", again)
	}
	if entries, _ := os.ReadDir(filepath.Dir(p)); len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestEnvOnlyCredentialsNotSaved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[[areas]]\nname = \"Linux\"\nsubreddit = \"linux\"\n"), 0o600)
	env := map[string]string{"REDDITBBS_CLIENT_ID": "envid", "REDDITBBS_CLIENT_SECRET": "envsecret"}
	c, err := Load(p, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasCredentials() {
		t.Fatal("env credentials should count")
	}
	c.AddArea(Area{Name: "Rust", Subreddit: "rust"})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "envsecret") || strings.Contains(string(data), "envid") {
		t.Errorf("secrets from environment written to disk:\n%s", data)
	}
	if !strings.Contains(string(data), "rust") {
		t.Error("area not saved")
	}
}

func TestSaveTightensPermissiveFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte(""), 0o644)
	c, _ := Load(p, noEnv)
	c.Save()
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", st.Mode().Perm())
	}
}

func TestAreaHelpers(t *testing.T) {
	c := &Config{}
	if !c.AddArea(Area{Name: "Linux", Subreddit: "linux"}) || c.AddArea(Area{Name: "L", Subreddit: "LINUX"}) {
		t.Error("AddArea duplicate handling")
	}
	if !c.HasArea("Linux") || c.HasArea("rust") {
		t.Error("HasArea")
	}
	c.AddArea(Area{Name: "Rust", Subreddit: "rust"})
	c.RemoveArea(0)
	if len(c.Areas) != 1 || c.Areas[0].Subreddit != "rust" {
		t.Errorf("areas = %+v", c.Areas)
	}
	c.RemoveArea(5) // out of range is a no-op
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/`
Expected: FAIL, undefined: Load and friends.

- [ ] **Step 3: Implement config.go**

```go
// Package config loads and saves the user's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultUserAgent identifies the client to Reddit when the user has not set one.
const DefaultUserAgent = "linux:redditbbs:0.1.0 (by /u/redditbbs)"

// Area is a named subreddit shown in the Area List.
type Area struct {
	Name      string `toml:"name"`
	Subreddit string `toml:"subreddit"`
}

// DefaultAreas are written on first-time setup.
var DefaultAreas = []Area{
	{Name: "Linux", Subreddit: "linux"},
	{Name: "Programming", Subreddit: "programming"},
	{Name: "Retro Battlestations", Subreddit: "retrobattlestations"},
	{Name: "Command Line", Subreddit: "commandline"},
}

type file struct {
	Reddit struct {
		ClientID     string `toml:"client_id"`
		ClientSecret string `toml:"client_secret"`
		UserAgent    string `toml:"user_agent"`
	} `toml:"reddit"`
	Display struct {
		PeekPane    bool   `toml:"peek_pane"`
		DefaultSort string `toml:"default_sort"`
	} `toml:"display"`
	Areas []Area `toml:"areas"`
}

// Config is the effective configuration: file values overlaid by environment
// variables. Only file values are ever saved.
type Config struct {
	file
	Path string

	envID, envSecret string
}

// DefaultPath is $XDG_CONFIG_HOME/redditbbs/config.toml or the OS equivalent.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "redditbbs", "config.toml"), nil
}

// Load reads path if it exists, applies defaults, then overlays
// REDDITBBS_CLIENT_ID and REDDITBBS_CLIENT_SECRET from getenv.
func Load(path string, getenv func(string) string) (*Config, error) {
	c := &Config{Path: path}
	c.Display.PeekPane = true
	c.Display.DefaultSort = "hot"
	c.Reddit.UserAgent = DefaultUserAgent
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := toml.Unmarshal(data, &c.file); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if c.Reddit.UserAgent == "" {
			c.Reddit.UserAgent = DefaultUserAgent
		}
		if c.Display.DefaultSort == "" {
			c.Display.DefaultSort = "hot"
		}
	}
	c.envID = getenv("REDDITBBS_CLIENT_ID")
	c.envSecret = getenv("REDDITBBS_CLIENT_SECRET")
	return c, nil
}

// ClientID is the effective client ID.
func (c *Config) ClientID() string {
	if c.envID != "" {
		return c.envID
	}
	return c.Reddit.ClientID
}

// ClientSecret is the effective client secret.
func (c *Config) ClientSecret() string {
	if c.envSecret != "" {
		return c.envSecret
	}
	return c.Reddit.ClientSecret
}

// HasCredentials reports whether both effective values are set.
func (c *Config) HasCredentials() bool { return c.ClientID() != "" && c.ClientSecret() != "" }

// SetCredentials stores credentials in the file section and drops any
// environment override so the saved values take effect.
func (c *Config) SetCredentials(id, secret string) {
	c.Reddit.ClientID, c.Reddit.ClientSecret = id, secret
	c.envID, c.envSecret = "", ""
}

// HasArea reports whether the subreddit is configured, ignoring case.
func (c *Config) HasArea(subreddit string) bool {
	for _, a := range c.Areas {
		if strings.EqualFold(a.Subreddit, subreddit) {
			return true
		}
	}
	return false
}

// AddArea appends an area unless its subreddit is already present.
func (c *Config) AddArea(a Area) bool {
	if c.HasArea(a.Subreddit) {
		return false
	}
	c.Areas = append(c.Areas, a)
	return true
}

// RemoveArea deletes the area at index i; out-of-range is a no-op.
func (c *Config) RemoveArea(i int) {
	if i < 0 || i >= len(c.Areas) {
		return
	}
	c.Areas = append(c.Areas[:i], c.Areas[i+1:]...)
}

// Save writes the file atomically with mode 0600 in a 0700 directory.
func (c *Config) Save() error {
	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(c.file); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, c.Path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/`
Expected: PASS. `TestSaveRoundTripAndMode` checks the directory is 0700; `os.MkdirAll` honours the mode subject to umask, which on this machine is 022 and leaves 0700 intact.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "Add config loading with environment overlay and atomic saves

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 12: session counters and browser opener

**Files:**
- Create: `internal/session/session.go`, `internal/browser/browser.go`
- Test: `internal/session/session_test.go`, `internal/browser/browser_test.go`

**Interfaces:**
- Produces:
  - `type Session struct { Start time.Time; ThreadsOpened, MessagesRead, LinksOpened int }` with `func New(start time.Time) *Session`, `VisitArea(subreddit string)`, `AreasVisited() int`, `Online(now time.Time) time.Duration`.
  - `func Open(rawURL string, onExit func(error)) error` in package `browser`.

- [ ] **Step 1: Write the failing tests**

`internal/session/session_test.go`:

```go
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
```

`internal/browser/browser_test.go`:

```go
package browser

import (
	"os/exec"
	"testing"
	"time"
)

func TestOpenRefusesNonHTTP(t *testing.T) {
	if err := Open("ftp://example.com", nil); err == nil {
		t.Error("ftp should be refused")
	}
	if err := Open("javascript:alert(1)", nil); err == nil {
		t.Error("javascript should be refused")
	}
	if err := Open("not a url", nil); err == nil {
		t.Error("garbage should be refused")
	}
}

func TestOpenReportsExitStatus(t *testing.T) {
	defer func(orig func(string, ...string) *exec.Cmd) { execCommand = orig }(execCommand)
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)
		return exec.Command("sh", "-c", "exit 3")
	}
	done := make(chan error, 1)
	if err := Open("https://example.com/x", func(err error) { done <- err }); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected non-zero exit to be reported")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onExit never called")
	}
	if len(gotArgs) < 2 || gotArgs[len(gotArgs)-1] != "https://example.com/x" {
		t.Errorf("args = %v", gotArgs)
	}
}

func TestOpenStartFailure(t *testing.T) {
	defer func(orig func(string, ...string) *exec.Cmd) { execCommand = orig }(execCommand)
	execCommand = func(name string, args ...string) *exec.Cmd { return exec.Command("/nonexistent/opener") }
	if err := Open("https://example.com", nil); err == nil {
		t.Error("expected start error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/session/ ./internal/browser/`
Expected: FAIL, undefined: New, Open, execCommand.

- [ ] **Step 3: Implement session.go**

```go
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
```

- [ ] **Step 4: Implement browser.go**

```go
// Package browser opens URLs with the operating system's default handler.
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

var execCommand = exec.Command

// Open launches the OS opener for an http or https URL without a shell. It
// returns once the process has started; onExit, if set, is called from a
// goroutine with the process's exit error, nil on success.
func Open(rawURL string, onExit func(error)) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("refusing to open %q: only http and https URLs are opened", rawURL)
	}
	name, args := opener(runtime.GOOS)
	cmd := execCommand(name, append(args, rawURL)...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		err := cmd.Wait()
		if onExit != nil {
			onExit(err)
		}
	}()
	return nil
}

func opener(goos string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		return "xdg-open", nil
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/session/ ./internal/browser/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/session internal/browser
git commit -m "Add session counters and safe browser opener

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 13: widgets package

**Files:**
- Create: `internal/ui/widgets/chrome.go`, `internal/ui/widgets/table.go`, `internal/ui/widgets/input.go`, `internal/ui/widgets/textbox.go`, `internal/ui/widgets/tree.go`
- Test: `internal/ui/widgets/chrome_test.go`, `internal/ui/widgets/table_test.go`, `internal/ui/widgets/input_test.go`, `internal/ui/widgets/textbox_test.go`, `internal/ui/widgets/tree_test.go`

**Interfaces:**
- Consumes: `term`, `theme`, `textfmt`.
- Produces:
  - `type KeyHelp struct { Key, Desc string }`; `type Prompt struct { Label, Input, Status string; Error, Cursor bool }`.
  - `const TitleRows = 3; const FooterRows = 2; const ChromeRows = 5`.
  - `func TitleBar(c term.Canvas, title, info string)`; `func HotkeyBar(c term.Canvas, y int, keys []KeyHelp)`; `func PromptLine(c term.Canvas, y int, p Prompt)`; `func Rule(c term.Canvas, y int)`; `func Centre(c term.Canvas, y int, s string, st term.Style)`.
  - `type Table struct { Cursor, Top int }` with `SetCount(int)`, `SetHeight(int)`, `Count() int`, `Height() int`, `Move(int)`, `Select(int)`, `HandleKey(term.Key) bool`, `Visible() (start, end int)`.
  - `type NumInput struct { Digits string }` with `HandleKey(term.Key) (value int, submitted, handled bool)`.
  - `type TextInput struct { Value string; Cursor int; Mask bool }` with `HandleKey(term.Key) InputResult`, `Display() string`, `Draw(c term.Canvas, x, y, w int, st term.Style, focused bool)`; `type InputResult int` with `InputNone, InputSubmit, InputCancel`.
  - `type TextBox struct { Lines []textfmt.Line; Top int }` with `Draw(c term.Canvas, x, y, w, h int)`, `Scroll(delta, h int)`, `AtEnd(h int) bool`.
  - `func DrawLine(c term.Canvas, x, y, w int, l textfmt.Line) int`; `func DrawLineStyled(c term.Canvas, x, y, w int, l textfmt.Line, st term.Style) int`; `func KindRole(k textfmt.Kind) theme.Role`.
  - `func Connector(ancestorsHaveNext []bool, hasNext bool) string`.

- [ ] **Step 1: Write the failing tests**

`internal/ui/widgets/chrome_test.go`:

```go
package widgets

import (
	"strings"
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
)

func TestTitleBar(t *testing.T) {
	sim := term.NewSim(60, 3)
	TitleBar(sim, "Message Areas", "r/linux 26/09/26")
	rows := strings.Split(sim.String(), "\n")
	if !strings.HasPrefix(rows[0], "╔═") || !strings.HasSuffix(rows[0], "═╗") {
		t.Errorf("top frame = %q", rows[0])
	}
	if !strings.Contains(rows[1], "R E D D I T   B B S") || !strings.Contains(rows[1], "Message Areas") || !strings.HasSuffix(rows[1], "r/linux 26/09/26 ║") {
		t.Errorf("title row = %q", rows[1])
	}
	if !strings.HasPrefix(rows[2], "╚═") || !strings.HasSuffix(rows[2], "═╝") {
		t.Errorf("bottom frame = %q", rows[2])
	}
	if _, st := sim.CellAt(0, 0); st != theme.Style(theme.Frame) {
		t.Errorf("frame style = %+v", st)
	}
}

func TestHotkeyBarAndPrompt(t *testing.T) {
	sim := term.NewSim(40, 2)
	HotkeyBar(sim, 0, []KeyHelp{{"N", "ext"}, {"Q", "uit"}})
	if got := sim.Row(0); got != "  [N]ext [Q]uit" {
		t.Errorf("hotkeys = %q", got)
	}
	if _, st := sim.CellAt(3, 0); st != theme.Style(theme.Hotkey) {
		t.Errorf("hotkey style = %+v", st)
	}
	PromptLine(sim, 1, Prompt{Input: "12", Status: "Retrieving...", Cursor: true})
	if got := sim.Row(1); !strings.HasPrefix(got, "  Command: 12") || !strings.HasSuffix(got, "Retrieving...") || len(got) != 38 {
		t.Errorf("prompt = %q (status should be right-aligned two cells from the edge)", got)
	}
	if x, y, on := sim.Cursor(); !on || x != 13 || y != 1 {
		t.Errorf("cursor = %d,%d,%v", x, y, on)
	}
	PromptLine(sim, 1, Prompt{Label: "Log off? (y/N)", Status: "boom", Error: true})
	if !strings.HasPrefix(sim.Row(1), "  Log off? (y/N)") {
		t.Errorf("label prompt = %q", sim.Row(1))
	}
	if _, st := sim.CellAt(37, 1); st != theme.Style(theme.Error) {
		t.Errorf("error status style = %+v", st)
	}
}

func TestRuleAndCentre(t *testing.T) {
	sim := term.NewSim(11, 2)
	Rule(sim, 0)
	if sim.Row(0) != strings.Repeat("─", 11) {
		t.Errorf("rule = %q", sim.Row(0))
	}
	Centre(sim, 1, "hi", term.Style{})
	if sim.Row(1) != "    hi" {
		t.Errorf("centre = %q", sim.Row(1))
	}
}
```

`internal/ui/widgets/table_test.go`:

```go
package widgets

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestTableMovementAndScrolling(t *testing.T) {
	var tb Table
	tb.SetHeight(5)
	tb.SetCount(12)
	tb.Move(-1)
	if tb.Cursor != 0 {
		t.Error("cursor below zero")
	}
	tb.Move(6)
	if tb.Cursor != 6 || tb.Top != 2 {
		t.Errorf("after down 6: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if s, e := tb.Visible(); s != 2 || e != 7 {
		t.Errorf("visible = %d..%d", s, e)
	}
	tb.HandleKey(term.K(term.KeyEnd))
	if tb.Cursor != 11 || tb.Top != 7 {
		t.Errorf("end: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	tb.HandleKey(term.K(term.KeyPgUp))
	if tb.Cursor != 6 {
		t.Errorf("pgup cursor=%d", tb.Cursor)
	}
	tb.HandleKey(term.K(term.KeyHome))
	if tb.Cursor != 0 || tb.Top != 0 {
		t.Errorf("home: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if tb.HandleKey(term.R('x')) {
		t.Error("rune should not be handled")
	}
}

func TestTableClampsWhenCountShrinks(t *testing.T) {
	var tb Table
	tb.SetHeight(3)
	tb.SetCount(10)
	tb.Select(9)
	tb.SetCount(4)
	if tb.Cursor != 3 || tb.Top != 1 {
		t.Errorf("cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	tb.SetCount(0)
	if tb.Cursor != 0 || tb.Top != 0 {
		t.Errorf("empty: cursor=%d top=%d", tb.Cursor, tb.Top)
	}
	if s, e := tb.Visible(); s != 0 || e != 0 {
		t.Errorf("empty visible = %d..%d", s, e)
	}
}

func TestTableHeightShrinkKeepsCursorVisible(t *testing.T) {
	var tb Table
	tb.SetHeight(10)
	tb.SetCount(20)
	tb.Select(9)
	tb.SetHeight(4)
	if s, e := tb.Visible(); tb.Cursor < s || tb.Cursor >= e {
		t.Errorf("cursor %d not within %d..%d", tb.Cursor, s, e)
	}
}
```

`internal/ui/widgets/input_test.go`:

```go
package widgets

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestNumInput(t *testing.T) {
	var n NumInput
	if _, _, handled := n.HandleKey(term.K(term.KeyEnter)); handled {
		t.Error("Enter with no digits must not be handled")
	}
	n.HandleKey(term.R('1'))
	n.HandleKey(term.R('7'))
	n.HandleKey(term.K(term.KeyBackspace))
	n.HandleKey(term.R('2'))
	if n.Digits != "12" {
		t.Errorf("digits = %q", n.Digits)
	}
	v, submitted, handled := n.HandleKey(term.K(term.KeyEnter))
	if v != 12 || !submitted || !handled || n.Digits != "" {
		t.Errorf("submit = %d %v %v %q", v, submitted, handled, n.Digits)
	}
	n.HandleKey(term.R('3'))
	if _, _, handled := n.HandleKey(term.K(term.KeyEscape)); !handled || n.Digits != "" {
		t.Error("Escape should clear digits")
	}
	if _, _, handled := n.HandleKey(term.K(term.KeyEscape)); handled {
		t.Error("Escape with no digits must pass through")
	}
}

func TestTextInputEditing(t *testing.T) {
	in := &TextInput{}
	for _, r := range "linux" {
		in.HandleKey(term.R(r))
	}
	in.HandleKey(term.K(term.KeyLeft))
	in.HandleKey(term.K(term.KeyBackspace))
	in.HandleKey(term.R('U'))
	if in.Value != "linUx" || in.Cursor != 4 {
		t.Errorf("value=%q cursor=%d", in.Value, in.Cursor)
	}
	in.HandleKey(term.K(term.KeyHome))
	in.HandleKey(term.K(term.KeyDelete))
	if in.Value != "inUx" {
		t.Errorf("after delete = %q", in.Value)
	}
	if in.HandleKey(term.K(term.KeyEnter)) != InputSubmit || in.HandleKey(term.K(term.KeyEscape)) != InputCancel {
		t.Error("submit/cancel results")
	}
	in.HandleKey(term.Key{Code: term.KeyRune, Rune: '?', Paste: true})
	if in.Value != "?inUx" {
		t.Errorf("pasted rune not inserted: %q", in.Value)
	}
}

func TestTextInputMaskAndDraw(t *testing.T) {
	in := &TextInput{Mask: true}
	for _, r := range "abc" {
		in.HandleKey(term.R(r))
	}
	if in.Display() != "***" {
		t.Errorf("display = %q", in.Display())
	}
	sim := term.NewSim(10, 1)
	in.Draw(sim, 2, 0, 6, term.Style{}, true)
	if sim.Row(0) != "  ***" {
		t.Errorf("row = %q", sim.Row(0))
	}
	if x, _, on := sim.Cursor(); !on || x != 5 {
		t.Errorf("cursor x = %d on=%v", x, on)
	}
}
```

`internal/ui/widgets/textbox_test.go`:

```go
package widgets

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
)

func TestTextBoxDrawAndScroll(t *testing.T) {
	doc := textfmt.Render("one\n\ntwo\n\n> three\n\nfour", 20)
	box := &TextBox{Lines: doc.Lines}
	sim := term.NewSim(20, 3)
	box.Draw(sim, 0, 0, 20, 3)
	if sim.Row(0) != "one" || sim.Row(2) != "two" {
		t.Errorf("rows = %q", sim.String())
	}
	box.Scroll(2, 3)
	sim.Clear()
	box.Draw(sim, 0, 0, 20, 3)
	if sim.Row(0) != "two" || sim.Row(2) != "> three" {
		t.Errorf("after scroll = %q", sim.String())
	}
	if _, st := sim.CellAt(0, 2); st != theme.Style(theme.Quote) {
		t.Errorf("quote style = %+v", st)
	}
	box.Scroll(100, 3)
	if box.Top != len(doc.Lines)-3 || !box.AtEnd(3) {
		t.Errorf("top = %d", box.Top)
	}
	box.Scroll(-100, 3)
	if box.Top != 0 {
		t.Errorf("top = %d", box.Top)
	}
}

func TestDrawLineStyledOverridesKinds(t *testing.T) {
	sim := term.NewSim(20, 1)
	l := textfmt.Line{{Text: "a ", Kind: textfmt.Text}, {Text: "b", Kind: textfmt.Bold}}
	DrawLineStyled(sim, 0, 0, 20, l, theme.Style(theme.Cursor))
	if _, st := sim.CellAt(2, 0); st != theme.Style(theme.Cursor) {
		t.Errorf("style = %+v", st)
	}
}
```

`internal/ui/widgets/tree_test.go`:

```go
package widgets

import "testing"

func TestConnector(t *testing.T) {
	cases := []struct {
		anc  []bool
		next bool
		want string
	}{
		{nil, true, "├─"},
		{nil, false, "└─"},
		{[]bool{true}, true, "│ ├─"},
		{[]bool{false}, false, "  └─"},
		{[]bool{true, false}, true, "│   ├─"},
	}
	for _, c := range cases {
		if got := Connector(c.anc, c.next); got != c.want {
			t.Errorf("Connector(%v,%v) = %q want %q", c.anc, c.next, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/widgets/`
Expected: FAIL, undefined symbols.

- [ ] **Step 3: Implement chrome.go**

```go
// Package widgets holds drawing helpers and small stateful controls shared by
// the screens: chrome, tables, inputs, text boxes and tree connectors.
package widgets

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
)

// Chrome heights.
const (
	TitleRows  = 3
	FooterRows = 2
	ChromeRows = TitleRows + FooterRows
)

// KeyHelp is one hotkey and its description, shown as [K]desc.
type KeyHelp struct {
	Key, Desc string
}

// Prompt is the state of the bottom prompt line.
type Prompt struct {
	Label  string // defaults to "Command:"
	Input  string
	Status string
	Error  bool // draw Status in the error style
	Cursor bool // show the terminal cursor after Input
}

const logo = "R E D D I T   B B S"

// TitleBar draws the three-row framed title at the top of c.
func TitleBar(c term.Canvas, title, info string) {
	w, _ := c.Size()
	fr := theme.Style(theme.Frame)
	c.Put(0, 0, "╔", fr)
	c.Fill(1, 0, w-2, 1, '═', fr)
	c.Put(w-1, 0, "╗", fr)
	c.Put(0, 1, "║", fr)
	c.Fill(1, 1, w-2, 1, ' ', term.Style{})
	c.Put(w-1, 1, "║", fr)
	c.Put(0, 2, "╚", fr)
	c.Fill(1, 2, w-2, 1, '═', fr)
	c.Put(w-1, 2, "╝", fr)

	infoW := textfmt.Width(info)
	x := 2
	x += c.Text(x, 1, logo, theme.Style(theme.Logo), w)
	x += c.Text(x, 1, " · ", theme.Style(theme.Meta), w)
	avail := w - 3 - infoW - 1 - x
	if avail > 0 {
		c.Text(x, 1, textfmt.Truncate(title, avail), theme.Style(theme.Subject), avail)
	}
	c.Text(w-3-infoW, 1, info, theme.Style(theme.Meta), infoW) // one space before the frame
}

// HotkeyBar draws [K]desc pairs across row y.
func HotkeyBar(c term.Canvas, y int, keys []KeyHelp) {
	w, _ := c.Size()
	x := 2
	meta, hot, body := theme.Style(theme.Meta), theme.Style(theme.Hotkey), theme.Style(theme.Body)
	for _, k := range keys {
		need := 3 + textfmt.Width(k.Key) + textfmt.Width(k.Desc)
		if x+need > w {
			return
		}
		x += c.Text(x, y, "[", meta, w)
		x += c.Text(x, y, k.Key, hot, w)
		x += c.Text(x, y, "]", meta, w)
		x += c.Text(x, y, k.Desc, body, w)
		x++
	}
}

// PromptLine draws the label, typed input and right-aligned status on row y.
func PromptLine(c term.Canvas, y int, p Prompt) {
	w, _ := c.Size()
	label := p.Label
	if label == "" {
		label = "Command:"
	}
	x := 2
	x += c.Text(x, y, label, theme.Style(theme.Prompt), w)
	x++
	x += c.Text(x, y, p.Input, theme.Style(theme.Subject), w-x)
	if p.Cursor {
		c.ShowCursor(x, y)
	} else {
		c.HideCursor()
	}
	if p.Status == "" {
		return
	}
	st := theme.Style(theme.Meta)
	if p.Error {
		st = theme.Style(theme.Error)
	}
	avail := w - 2 - (x + 1)
	status := textfmt.Truncate(p.Status, avail)
	c.Text(w-2-textfmt.Width(status), y, status, st, w)
}

// Rule draws a full-width horizontal rule on row y.
func Rule(c term.Canvas, y int) {
	w, _ := c.Size()
	c.Fill(0, y, w, 1, '─', theme.Style(theme.Rule))
}

// Centre draws s centred on row y.
func Centre(c term.Canvas, y int, s string, st term.Style) {
	w, _ := c.Size()
	s = textfmt.Truncate(s, w)
	c.Text((w-textfmt.Width(s))/2, y, s, st, w)
}
```

- [ ] **Step 4: Implement table.go**

```go
package widgets

import "github.com/markwatson/redditbbs/internal/term"

// Table tracks a cursor over count rows shown height at a time, keeping the
// cursor visible.
type Table struct {
	Cursor, Top   int
	count, height int
}

// SetCount sets the number of rows and clamps.
func (t *Table) SetCount(n int) { t.count = n; t.clamp() }

// SetHeight sets the visible rows and clamps.
func (t *Table) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	t.height = h
	t.clamp()
}

// Count is the row count.
func (t *Table) Count() int { return t.count }

// Height is the visible row count.
func (t *Table) Height() int { return t.height }

func (t *Table) clamp() {
	if t.Cursor >= t.count {
		t.Cursor = t.count - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	if t.height < 1 {
		t.height = 1
	}
	if t.Cursor < t.Top {
		t.Top = t.Cursor
	}
	if t.Cursor >= t.Top+t.height {
		t.Top = t.Cursor - t.height + 1
	}
	if t.Top > t.count-t.height {
		t.Top = t.count - t.height
	}
	if t.Top < 0 {
		t.Top = 0
	}
}

// Move shifts the cursor by delta.
func (t *Table) Move(delta int) { t.Cursor += delta; t.clamp() }

// Select moves the cursor to row i.
func (t *Table) Select(i int) { t.Cursor = i; t.clamp() }

// HandleKey applies arrow, page, Home and End keys. It returns false for other keys.
func (t *Table) HandleKey(k term.Key) bool {
	switch k.Code {
	case term.KeyUp:
		t.Move(-1)
	case term.KeyDown:
		t.Move(1)
	case term.KeyPgUp:
		t.Move(-t.height)
	case term.KeyPgDn:
		t.Move(t.height)
	case term.KeyHome:
		t.Select(0)
	case term.KeyEnd:
		t.Select(t.count - 1)
	default:
		return false
	}
	return true
}

// Visible returns the half-open range of rows on screen.
func (t *Table) Visible() (start, end int) {
	end = t.Top + t.height
	if end > t.count {
		end = t.count
	}
	return t.Top, end
}
```

- [ ] **Step 5: Implement input.go**

```go
package widgets

import (
	"strconv"
	"strings"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
)

// NumInput collects typed digits for selecting a row by number.
type NumInput struct{ Digits string }

// HandleKey consumes digits always, and Backspace, Enter and Escape while
// digits are present. It reports the submitted value on Enter.
func (n *NumInput) HandleKey(k term.Key) (value int, submitted, handled bool) {
	if k.Code == term.KeyRune && k.Rune >= '0' && k.Rune <= '9' {
		if len(n.Digits) < 6 {
			n.Digits += string(k.Rune)
		}
		return 0, false, true
	}
	if n.Digits == "" {
		return 0, false, false
	}
	switch k.Code {
	case term.KeyBackspace:
		n.Digits = n.Digits[:len(n.Digits)-1]
	case term.KeyEnter:
		v, _ := strconv.Atoi(n.Digits)
		n.Digits = ""
		return v, true, true
	case term.KeyEscape:
		n.Digits = ""
	default:
		return 0, false, false
	}
	return 0, false, true
}

// InputResult is what a TextInput key press produced.
type InputResult int

// Input results.
const (
	InputNone InputResult = iota
	InputSubmit
	InputCancel
)

// TextInput is a single-line editor. Cursor is a rune index.
type TextInput struct {
	Value  string
	Cursor int
	Mask   bool
}

// HandleKey edits the value; every key is taken literally except Enter and Escape.
func (t *TextInput) HandleKey(k term.Key) InputResult {
	rs := []rune(t.Value)
	if t.Cursor > len(rs) {
		t.Cursor = len(rs)
	}
	switch k.Code {
	case term.KeyRune:
		rs = append(rs[:t.Cursor], append([]rune{k.Rune}, rs[t.Cursor:]...)...)
		t.Cursor++
	case term.KeyBackspace:
		if t.Cursor > 0 {
			rs = append(rs[:t.Cursor-1], rs[t.Cursor:]...)
			t.Cursor--
		}
	case term.KeyDelete:
		if t.Cursor < len(rs) {
			rs = append(rs[:t.Cursor], rs[t.Cursor+1:]...)
		}
	case term.KeyLeft:
		if t.Cursor > 0 {
			t.Cursor--
		}
	case term.KeyRight:
		if t.Cursor < len(rs) {
			t.Cursor++
		}
	case term.KeyHome:
		t.Cursor = 0
	case term.KeyEnd:
		t.Cursor = len(rs)
	case term.KeyEnter:
		return InputSubmit
	case term.KeyEscape:
		return InputCancel
	}
	t.Value = string(rs)
	return InputNone
}

// Display is the value as shown, masked with * when Mask is set.
func (t *TextInput) Display() string {
	if t.Mask {
		return strings.Repeat("*", len([]rune(t.Value)))
	}
	return t.Value
}

// Draw renders the value in w cells and places the cursor when focused.
func (t *TextInput) Draw(c term.Canvas, x, y, w int, st term.Style, focused bool) {
	shown := t.Display()
	c.Text(x, y, shown, st, w)
	if focused {
		before := string([]rune(shown)[:min(t.Cursor, len([]rune(shown)))])
		c.ShowCursor(x+textfmt.Width(before), y)
	}
}
```

- [ ] **Step 6: Implement textbox.go and tree.go**

`internal/ui/widgets/textbox.go`:

```go
package widgets

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
)

// TextBox shows pre-wrapped lines from a scroll offset.
type TextBox struct {
	Lines []textfmt.Line
	Top   int
}

// Draw renders up to h lines starting at Top.
func (b *TextBox) Draw(c term.Canvas, x, y, w, h int) {
	for i := 0; i < h && b.Top+i < len(b.Lines); i++ {
		DrawLine(c, x, y+i, w, b.Lines[b.Top+i])
	}
}

// Scroll moves Top by delta, clamped so the last page is full where possible.
func (b *TextBox) Scroll(delta, h int) {
	b.Top += delta
	if max := len(b.Lines) - h; b.Top > max {
		b.Top = max
	}
	if b.Top < 0 {
		b.Top = 0
	}
}

// AtEnd reports whether the last line is visible.
func (b *TextBox) AtEnd(h int) bool { return b.Top+h >= len(b.Lines) }

// KindRole maps a span kind to its theme role.
func KindRole(k textfmt.Kind) theme.Role {
	switch k {
	case textfmt.Quote:
		return theme.Quote
	case textfmt.Code:
		return theme.Code
	case textfmt.Bold:
		return theme.Bold
	case textfmt.Link:
		return theme.Link
	default:
		return theme.Body
	}
}

// DrawLine draws spans with their own styles, clipped to w cells.
func DrawLine(c term.Canvas, x, y, w int, l textfmt.Line) int {
	used := 0
	for _, s := range l {
		if used >= w {
			break
		}
		used += c.Text(x+used, y, s.Text, theme.Style(KindRole(s.Kind)), w-used)
	}
	return used
}

// DrawLineStyled draws spans in one style, for cursor rows.
func DrawLineStyled(c term.Canvas, x, y, w int, l textfmt.Line, st term.Style) int {
	used := 0
	for _, s := range l {
		if used >= w {
			break
		}
		used += c.Text(x+used, y, s.Text, st, w-used)
	}
	return used
}
```

`internal/ui/widgets/tree.go`:

```go
package widgets

import "strings"

// Connector returns the tree prefix for a node whose ancestors above the
// top level are described by ancestorsHaveNext (true when that ancestor has
// later siblings, so its rail continues) and whose own hasNext says whether a
// sibling follows. Two cells per level.
func Connector(ancestorsHaveNext []bool, hasNext bool) string {
	var b strings.Builder
	for _, a := range ancestorsHaveNext {
		if a {
			b.WriteString("│ ")
		} else {
			b.WriteString("  ")
		}
	}
	if hasNext {
		b.WriteString("├─")
	} else {
		b.WriteString("└─")
	}
	return b.String()
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/ui/widgets/`
Expected: PASS. In `TestHotkeyBarAndPrompt` the cursor lands at column 13: two spaces, `Command:` (8), one space, `12` (2) gives 13.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/widgets
git commit -m "Add chrome, table, input, text box and tree widgets

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 14: ui core: actions, Screen interface, App loop, Help overlay

**Files:**
- Create: `internal/ui/action.go`, `internal/ui/app.go`, `internal/ui/help.go`
- Test: `internal/ui/app_test.go`
- Modify: `docs/superpowers/specs/2026-09-26-redditbbs-design.md` (dependency table row for `internal/ui`: add `widgets`)

**Interfaces:**
- Consumes: `term`, `theme`, `widgets`.
- Produces:
  - `type Msg any`; `type Action any`; navigation types `Push{Screen}`, `Pop{Result any}`, `Replace{Screen}`, `Quit{}`, `Run{Fn func(context.Context) Msg}`, `Batch{Actions []Action}`.
  - Messages: `PopResult{Result any}`, `Resize{W, H int}`, `Tick{ID int}`, `ErrMsg{Err error}`, `RateLimited{Wait time.Duration}`.
  - `type KeyHelp = widgets.KeyHelp`.
  - `type Screen interface { Init() Action; Draw(c term.Canvas); HandleKey(k term.Key) Action; Update(msg Msg) Action; Title() string; Keys() []KeyHelp }`.
  - Optional interfaces: `Overlayer{ Overlay() bool }`, `Fullscreener{ Fullscreen() bool }`, `Modal{ CapturesKeys() bool }`, `Prompter{ Prompt() widgets.Prompt }`, `Infoer{ Info() string }`.
  - `func Sleep(ctx context.Context, d time.Duration, id int) Run` helper returning a Run that yields `Tick{id}` after d unless cancelled (then `nil` Msg, which App ignores).
  - `type App` with `func New(t term.Terminal, root Screen, opts ...Option) *App`, `Run() error`, `Handle(ev term.Event)`, `Pump(timeout time.Duration) bool`, `Draw()`, `Post(msg Msg)`, `Quitting() bool`, `Depth() int`, `Top() Screen`; option `WithMinSize(w, h int)`.
  - `var GlobalKeys []KeyHelp`.

- [ ] **Step 1: Amend the spec dependency table**

In `docs/superpowers/specs/2026-09-26-redditbbs-design.md`, change the `internal/ui` row's "Depends on" cell from `term, theme` to `term, theme, widgets` and add this sentence under the table: "`ui` draws the shared chrome and the Help overlay through `widgets`, so it depends on it; `widgets` never imports `ui`." Commit with the code below.

- [ ] **Step 2: Write the failing tests**

`internal/ui/app_test.go`:

```go
package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// stub is a configurable test screen.
type stub struct {
	name     string
	init     Action
	onKey    func(k term.Key) Action
	onUpdate func(m Msg) Action
	keys     []term.Key
	msgs     []Msg
	drawn    int
	overlay  bool
	full     bool
	modal    bool
}

func (s *stub) Init() Action { return s.init }
func (s *stub) Draw(c term.Canvas) {
	s.drawn++
	c.Text(0, 0, "screen:"+s.name, term.Style{}, 40)
}
func (s *stub) HandleKey(k term.Key) Action {
	s.keys = append(s.keys, k)
	if s.onKey != nil {
		return s.onKey(k)
	}
	return nil
}
func (s *stub) Update(m Msg) Action {
	s.msgs = append(s.msgs, m)
	if s.onUpdate != nil {
		return s.onUpdate(m)
	}
	return nil
}
func (s *stub) Title() string          { return s.name }
func (s *stub) Keys() []KeyHelp        { return []KeyHelp{{"X", "test"}} }
func (s *stub) Overlay() bool          { return s.overlay }
func (s *stub) Fullscreen() bool       { return s.full }
func (s *stub) CapturesKeys() bool     { return s.modal }
func (s *stub) Prompt() widgets.Prompt { return widgets.Prompt{Status: "st:" + s.name} }

func newApp(root Screen) (*App, *term.Sim) {
	sim := term.NewSim(80, 24)
	return New(sim, root), sim
}

func TestChromeAndContentSubCanvas(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	app.Draw()
	if !strings.Contains(sim.Row(1), "R E D D I T   B B S") || !strings.Contains(sim.Row(1), "root") {
		t.Errorf("title = %q", sim.Row(1))
	}
	if sim.Row(3) != "screen:root" {
		t.Errorf("content should start on row 3, got %q", sim.Row(3))
	}
	if !strings.Contains(sim.Row(22), "[X]test") || !strings.Contains(sim.Row(23), "st:root") {
		t.Errorf("footer = %q / %q", sim.Row(22), sim.Row(23))
	}
}

func TestFullscreenSkipsChrome(t *testing.T) {
	app, sim := newApp(&stub{name: "splash", full: true})
	app.Draw()
	if sim.Row(0) != "screen:splash" {
		t.Errorf("row 0 = %q", sim.Row(0))
	}
}

func TestPushPopWithResult(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	child.onKey = func(k term.Key) Action { return Pop{Result: "picked"} }
	app, sim := newApp(root)
	app.Handle(term.R('a'))
	if app.Depth() != 2 || app.Top() != child {
		t.Fatalf("depth=%d", app.Depth())
	}
	app.Draw()
	if sim.Row(3) != "screen:child" {
		t.Errorf("child not drawn: %q", sim.Row(3))
	}
	app.Handle(term.R('b'))
	if app.Depth() != 1 {
		t.Fatalf("pop failed, depth=%d", app.Depth())
	}
	if len(root.msgs) != 1 {
		t.Fatalf("root msgs = %v", root.msgs)
	}
	if pr, ok := root.msgs[0].(PopResult); !ok || pr.Result != "picked" {
		t.Errorf("pop result = %#v", root.msgs[0])
	}
}

func TestPopLastScreenQuits(t *testing.T) {
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Pop{} }
	app, _ := newApp(root)
	app.Handle(term.R('q'))
	if !app.Quitting() {
		t.Error("popping the last screen should quit")
	}
}

func TestReplace(t *testing.T) {
	next := &stub{name: "next"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Replace{next} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	if app.Depth() != 1 || app.Top() != next {
		t.Error("replace failed")
	}
	if len(root.msgs) != 0 {
		t.Error("replace must not deliver PopResult")
	}
}

func TestRunDeliversToOriginOnly(t *testing.T) {
	root := &stub{name: "root"}
	root.init = Run{Fn: func(ctx context.Context) Msg { return "hello" }}
	app, _ := newApp(root)
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	if len(root.msgs) != 1 || root.msgs[0] != "hello" {
		t.Errorf("msgs = %v", root.msgs)
	}
}

func TestRunDroppedAfterPop(t *testing.T) {
	release := make(chan struct{})
	child := &stub{name: "child"}
	child.init = Run{Fn: func(ctx context.Context) Msg {
		<-release
		return "late"
	}}
	child.onKey = func(k term.Key) Action { return Pop{} }
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b')) // pop child while its Run is outstanding
	close(release)
	app.Pump(time.Second) // the late result arrives but has no live target
	if len(child.msgs) != 0 {
		t.Errorf("child got %v", child.msgs)
	}
}

func TestRunContextCancelledOnPop(t *testing.T) {
	cancelled := make(chan struct{})
	child := &stub{name: "child"}
	child.init = Run{Fn: func(ctx context.Context) Msg {
		<-ctx.Done()
		close(cancelled)
		return nil
	}}
	child.onKey = func(k term.Key) Action { return Pop{} }
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b'))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("context not cancelled on pop")
	}
}

func TestBatchPushThenRunBindsToOrigin(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action {
		return Batch{Actions: []Action{Push{child}, Run{Fn: func(ctx context.Context) Msg { return "for-root" }}}}
	}
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	if len(root.msgs) != 1 || root.msgs[0] != "for-root" || len(child.msgs) != 0 {
		t.Errorf("root=%v child=%v", root.msgs, child.msgs)
	}
}

func TestBatchPopDiscardsOwnRuns(t *testing.T) {
	ran := make(chan struct{}, 1)
	child := &stub{name: "child"}
	child.onKey = func(k term.Key) Action {
		return Batch{Actions: []Action{Pop{}, Run{Fn: func(ctx context.Context) Msg { ran <- struct{}{}; return "x" }}}}
	}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.R('b'))
	select {
	case <-ran:
		t.Error("run from a popped screen must not start")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCoveredScreenNavigationIgnored(t *testing.T) {
	child := &stub{name: "child"}
	root := &stub{name: "root"}
	root.onKey = func(k term.Key) Action { return Push{child} }
	root.onUpdate = func(m Msg) Action { return Pop{} }
	root.init = nil
	app, _ := newApp(root)
	app.Handle(term.R('a'))
	app.Handle(term.Resize{W: 100, H: 30}) // both screens get Resize; root's Pop must be ignored
	if app.Depth() != 2 {
		t.Errorf("depth = %d", app.Depth())
	}
	if len(root.msgs) != 1 || len(child.msgs) != 1 {
		t.Errorf("resize delivery root=%v child=%v", root.msgs, child.msgs)
	}
}

func TestWorkerPanicBecomesErrMsg(t *testing.T) {
	root := &stub{name: "root"}
	root.init = Run{Fn: func(ctx context.Context) Msg { panic("boom") }}
	app, _ := newApp(root)
	if !app.Pump(time.Second) {
		t.Fatal("no result")
	}
	em, ok := root.msgs[0].(ErrMsg)
	if !ok || !strings.Contains(em.Err.Error(), "boom") {
		t.Errorf("msg = %#v", root.msgs[0])
	}
}

func TestHelpOverlayAndGlobalKeys(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	app.Handle(term.R('?'))
	if app.Depth() != 2 {
		t.Fatal("help not pushed")
	}
	app.Draw()
	out := sim.String()
	if !strings.Contains(out, "screen:root") || !strings.Contains(out, "[X] test") || !strings.Contains(out, "Ctrl-C") {
		t.Errorf("overlay should draw over root and list keys:\n%s", out)
	}
	app.Handle(term.R('z'))
	if app.Depth() != 1 || len(root.keys) != 0 {
		t.Error("any key should close help without reaching root")
	}
	app.Handle(term.K(term.KeyCtrlC))
	if !app.Quitting() {
		t.Error("Ctrl-C must quit")
	}
}

func TestModalScreenGetsQuestionMark(t *testing.T) {
	root := &stub{name: "root", modal: true}
	app, _ := newApp(root)
	app.Handle(term.R('?'))
	if app.Depth() != 1 || len(root.keys) != 1 {
		t.Error("modal screen should receive ? itself")
	}
}

func TestUndersizedTerminal(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	sim.Resize(60, 20)
	app.Handle(<-sim.Events())
	app.Draw()
	if !strings.Contains(sim.String(), "80x24") {
		t.Errorf("expected enlarge message:\n%s", sim.String())
	}
	app.Handle(term.R('a'))
	if len(root.keys) != 0 {
		t.Error("keys must be swallowed while undersized")
	}
	sim.Resize(80, 24)
	app.Handle(<-sim.Events())
	app.Handle(term.R('a'))
	if len(root.keys) != 1 {
		t.Error("keys should flow again after resize")
	}
}

func TestPostDeliversToTop(t *testing.T) {
	root := &stub{name: "root"}
	app, _ := newApp(root)
	go app.Post(RateLimited{Wait: 3 * time.Second})
	if !app.Pump(time.Second) {
		t.Fatal("no message")
	}
	if rl, ok := root.msgs[0].(RateLimited); !ok || rl.Wait != 3*time.Second {
		t.Errorf("msg = %#v", root.msgs[0])
	}
}

func TestSleepHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m := Sleep(ctx, time.Hour, 7).Fn(ctx); m != nil {
		t.Errorf("cancelled sleep should yield nil, got %#v", m)
	}
	if m := Sleep(context.Background(), time.Millisecond, 7).Fn(context.Background()); m != (Tick{ID: 7}) {
		t.Errorf("got %#v", m)
	}
}

func TestRunLoopQuitsOnCtrlC(t *testing.T) {
	root := &stub{name: "root"}
	app, sim := newApp(root)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	sim.Inject(term.K(term.KeyCtrlC))
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit")
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/ui/`
Expected: FAIL, undefined: New, Push, and the rest.

- [ ] **Step 4: Implement action.go**

```go
// Package ui runs the screen stack: it owns the terminal, routes keys and
// async results to screens, draws the shared chrome and applies navigation
// actions. Screens never touch the terminal lifecycle or each other.
package ui

import (
	"context"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// Msg is anything delivered to a screen's Update.
type Msg any

// Action is what a screen returns from Init, HandleKey or Update: nil, Push,
// Pop, Replace, Quit, Run or Batch.
type Action any

// Push puts a screen on top of the stack.
type Push struct{ Screen Screen }

// Pop removes the top screen and delivers PopResult{Result} to the one beneath.
type Pop struct{ Result any }

// Replace swaps the top screen without delivering a PopResult.
type Replace struct{ Screen Screen }

// Quit ends the application.
type Quit struct{}

// Run executes Fn in a goroutine and delivers its Msg to the screen that
// returned the Run. ctx is cancelled when that screen is popped.
type Run struct{ Fn func(ctx context.Context) Msg }

// Batch applies several actions: navigation first, then Runs.
type Batch struct{ Actions []Action }

// PopResult carries the result of a popped child screen.
type PopResult struct{ Result any }

// Resize reports the new terminal size to every screen on the stack.
type Resize struct{ W, H int }

// Tick is produced by Sleep.
type Tick struct{ ID int }

// ErrMsg reports a worker panic or an App-level error.
type ErrMsg struct{ Err error }

// RateLimited tells the top screen the client is waiting on Reddit's limit.
type RateLimited struct{ Wait time.Duration }

// KeyHelp is re-exported for screens.
type KeyHelp = widgets.KeyHelp

// Screen is one full-screen view.
type Screen interface {
	Init() Action
	Draw(c term.Canvas)
	HandleKey(k term.Key) Action
	Update(msg Msg) Action
	Title() string
	Keys() []KeyHelp
}

// Overlayer screens draw on top of the screen beneath them.
type Overlayer interface{ Overlay() bool }

// Fullscreener screens draw without chrome over the whole terminal.
type Fullscreener interface{ Fullscreen() bool }

// Modal screens receive every key, including the global ones except Ctrl-C.
type Modal interface{ CapturesKeys() bool }

// Prompter screens supply the prompt line contents.
type Prompter interface{ Prompt() widgets.Prompt }

// Infoer screens supply the right-hand title bar text.
type Infoer interface{ Info() string }

// Sleep returns a Run that yields Tick{id} after d, or nil if cancelled.
func Sleep(ctx context.Context, d time.Duration, id int) Run {
	return Run{Fn: func(ctx context.Context) Msg {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			return Tick{ID: id}
		}
	}}
}

// GlobalKeys are handled by App on every screen.
var GlobalKeys = []KeyHelp{
	{"?", "Help"},
	{"Q/Esc", "Back"},
	{"↑↓ PgUp PgDn", "Move"},
	{"0-9 ⏎", "Select by number"},
	{"Ctrl-L", "Redraw"},
	{"Ctrl-C", "Quit"},
}
```

- [ ] **Step 5: Implement app.go**

```go
package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

type entry struct {
	screen Screen
	ctx    context.Context
	cancel context.CancelFunc
}

type result struct {
	target *entry // nil means the current top screen
	msg    Msg
}

// App owns the terminal and the screen stack.
type App struct {
	t          term.Terminal
	stack      []*entry
	results    chan result
	done       chan struct{}
	w, h       int
	minW, minH int
	quit       bool
}

// Option configures App.
type Option func(*App)

// WithMinSize sets the minimum usable terminal size (default 80x24).
func WithMinSize(w, h int) Option { return func(a *App) { a.minW, a.minH = w, h } }

// New creates an App with root as the first screen and runs its Init.
func New(t term.Terminal, root Screen, opts ...Option) *App {
	a := &App{t: t, results: make(chan result), done: make(chan struct{}), minW: 80, minH: 24}
	for _, o := range opts {
		o(a)
	}
	a.w, a.h = t.Size()
	a.push(root)
	return a
}

// Run processes events until Quit or Ctrl-C, redrawing after each.
func (a *App) Run() error {
	defer close(a.done)
	a.Draw()
	for !a.quit {
		select {
		case ev, ok := <-a.t.Events():
			if !ok {
				return nil
			}
			a.Handle(ev)
		case r := <-a.results:
			a.deliver(r)
		}
		a.Draw()
	}
	return nil
}

// Quitting reports whether a Quit has been applied.
func (a *App) Quitting() bool { return a.quit }

// Depth is the stack height.
func (a *App) Depth() int { return len(a.stack) }

// Top is the current top screen, or nil.
func (a *App) Top() Screen {
	if e := a.top(); e != nil {
		return e.screen
	}
	return nil
}

func (a *App) top() *entry {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) onStack(e *entry) bool {
	for _, x := range a.stack {
		if x == e {
			return true
		}
	}
	return false
}

func (a *App) small() bool { return a.w < a.minW || a.h < a.minH }

// Handle processes one terminal event synchronously.
func (a *App) Handle(ev term.Event) {
	switch e := ev.(type) {
	case term.Resize:
		a.w, a.h = e.W, e.H
		for _, en := range append([]*entry(nil), a.stack...) {
			if a.onStack(en) {
				a.apply(en, en.screen.Update(Resize{W: e.W, H: e.H}))
			}
		}
	case term.Key:
		a.handleKey(e)
	}
}

func (a *App) handleKey(k term.Key) {
	if k.Code == term.KeyCtrlC {
		a.quit = true
		return
	}
	if a.small() {
		return
	}
	top := a.top()
	if top == nil {
		a.quit = true
		return
	}
	if m, ok := top.screen.(Modal); ok && m.CapturesKeys() {
		a.apply(top, top.screen.HandleKey(k))
		return
	}
	if o, ok := top.screen.(Overlayer); ok && o.Overlay() {
		a.apply(top, top.screen.HandleKey(k))
		return
	}
	switch {
	case k.Code == term.KeyRune && k.Rune == '?':
		a.push(newHelp(top.screen))
	case k.Code == term.KeyCtrlL:
		a.t.Sync()
	default:
		a.apply(top, top.screen.HandleKey(k))
	}
}

// Pump waits up to timeout for one async result and delivers it. It is for
// tests; Run does this in its loop.
func (a *App) Pump(timeout time.Duration) bool {
	select {
	case r := <-a.results:
		a.deliver(r)
		return true
	case <-time.After(timeout):
		return false
	}
}

// Post delivers msg to whichever screen is on top when it is processed. It
// blocks until the loop takes it, so never call it from the loop goroutine.
func (a *App) Post(msg Msg) {
	select {
	case a.results <- result{msg: msg}:
	case <-a.done:
	}
}

func (a *App) deliver(r result) {
	target := r.target
	if target == nil {
		target = a.top()
	}
	if target == nil || !a.onStack(target) || r.msg == nil {
		return
	}
	a.apply(target, target.screen.Update(r.msg))
}

// apply performs act on behalf of origin: navigation first, then Runs bound
// to origin if it is still on the stack.
func (a *App) apply(origin *entry, act Action) {
	var runs []Run
	a.applyNav(origin, act, &runs)
	for _, r := range runs {
		if a.onStack(origin) {
			a.start(origin, r)
		}
	}
}

func (a *App) applyNav(origin *entry, act Action, runs *[]Run) {
	switch x := act.(type) {
	case nil:
	case Batch:
		for _, sub := range x.Actions {
			a.applyNav(origin, sub, runs)
		}
	case Run:
		*runs = append(*runs, x)
	case Quit:
		a.quit = true
	case Push:
		if a.top() == origin {
			a.push(x.Screen)
		}
	case Pop:
		if a.top() == origin {
			a.pop(x.Result)
		}
	case Replace:
		if a.top() == origin {
			a.remove()
			a.push(x.Screen)
		}
	}
}

func (a *App) push(s Screen) {
	ctx, cancel := context.WithCancel(context.Background())
	en := &entry{screen: s, ctx: ctx, cancel: cancel}
	a.stack = append(a.stack, en)
	a.apply(en, s.Init())
}

func (a *App) remove() {
	if en := a.top(); en != nil {
		en.cancel()
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) pop(res any) {
	a.remove()
	next := a.top()
	if next == nil {
		a.quit = true
		return
	}
	a.apply(next, next.screen.Update(PopResult{Result: res}))
}

func (a *App) start(en *entry, r Run) {
	go func() {
		var msg Msg
		func() {
			defer func() {
				if p := recover(); p != nil {
					msg = ErrMsg{Err: fmt.Errorf("internal error: %v", p)}
				}
			}()
			msg = r.Fn(en.ctx)
		}()
		select {
		case a.results <- result{target: en, msg: msg}:
		case <-a.done:
		}
	}()
}

// Draw renders the visible screens and presents the frame.
func (a *App) Draw() {
	a.t.Clear()
	a.t.HideCursor()
	if a.small() {
		widgets.Centre(a.t, a.h/2, fmt.Sprintf("Please enlarge your terminal to at least %dx%d", a.minW, a.minH), theme.Style(theme.Error))
		a.t.Show()
		return
	}
	start := len(a.stack) - 1
	for start > 0 {
		if o, ok := a.stack[start].screen.(Overlayer); ok && o.Overlay() {
			start--
			continue
		}
		break
	}
	for i := start; i < len(a.stack); i++ {
		a.drawScreen(a.stack[i].screen)
	}
	a.t.Show()
}

func (a *App) drawScreen(s Screen) {
	if f, ok := s.(Fullscreener); ok && f.Fullscreen() {
		s.Draw(a.t)
		return
	}
	if o, ok := s.(Overlayer); ok && o.Overlay() {
		s.Draw(a.t)
		return
	}
	info := ""
	if i, ok := s.(Infoer); ok {
		info = i.Info()
	}
	widgets.TitleBar(a.t, s.Title(), info)
	widgets.HotkeyBar(a.t, a.h-2, s.Keys())
	var p widgets.Prompt
	if pr, ok := s.(Prompter); ok {
		p = pr.Prompt()
	}
	widgets.PromptLine(a.t, a.h-1, p)
	s.Draw(term.Sub(a.t, 0, widgets.TitleRows, a.w, a.h-widgets.ChromeRows))
}
```

- [ ] **Step 6: Implement help.go**

```go
package ui

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// helpScreen is the overlay listing the keys of the screen beneath it.
type helpScreen struct {
	title string
	keys  []KeyHelp
}

func newHelp(under Screen) *helpScreen {
	return &helpScreen{title: under.Title(), keys: under.Keys()}
}

func (h *helpScreen) Init() Action                 { return nil }
func (h *helpScreen) HandleKey(term.Key) Action    { return Pop{} }
func (h *helpScreen) Update(Msg) Action            { return nil }
func (h *helpScreen) Title() string                { return "Help" }
func (h *helpScreen) Keys() []KeyHelp              { return nil }
func (h *helpScreen) Overlay() bool                { return true }

func (h *helpScreen) Draw(c term.Canvas) {
	w, hgt := c.Size()
	lines := []string{"Keys for " + h.title + ":", ""}
	for _, k := range h.keys {
		lines = append(lines, "  ["+k.Key+"] "+k.Desc)
	}
	lines = append(lines, "", "Everywhere:", "")
	for _, k := range GlobalKeys {
		lines = append(lines, "  ["+k.Key+"] "+k.Desc)
	}
	lines = append(lines, "", "Press any key to close")
	bw := 30
	for _, l := range lines {
		if n := textfmt.Width(l) + 4; n > bw {
			bw = n
		}
	}
	if bw > w-2 {
		bw = w - 2
	}
	bh := len(lines) + 2
	if bh > hgt-2 {
		bh = hgt - 2
	}
	x0, y0 := (w-bw)/2, (hgt-bh)/2
	fr := theme.Style(theme.Frame)
	c.Fill(x0, y0, bw, bh, ' ', term.Style{})
	c.Put(x0, y0, "╔", fr)
	c.Fill(x0+1, y0, bw-2, 1, '═', fr)
	c.Put(x0+bw-1, y0, "╗", fr)
	c.Put(x0, y0+bh-1, "╚", fr)
	c.Fill(x0+1, y0+bh-1, bw-2, 1, '═', fr)
	c.Put(x0+bw-1, y0+bh-1, "╝", fr)
	for i := 1; i < bh-1; i++ {
		c.Put(x0, y0+i, "║", fr)
		c.Put(x0+bw-1, y0+i, "║", fr)
	}
	for i, l := range lines {
		if i+1 >= bh-1 {
			break
		}
		st := theme.Style(theme.Body)
		if i == 0 || l == "Everywhere:" {
			st = theme.Style(theme.Heading)
		}
		c.Text(x0+2, y0+1+i, textfmt.Truncate(l, bw-4), st, bw-4)
	}
	widgets.Centre(c, y0+bh-1, " Help ", theme.Style(theme.Heading))
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/ui/`
Expected: PASS. `TestHelpOverlayAndGlobalKeys` looks for `[X] test` (with the space the Help box inserts) and `Ctrl-C`.

- [ ] **Step 8: Commit**

```bash
git add internal/ui docs/superpowers/specs/2026-09-26-redditbbs-design.md
git commit -m "Add App loop with screen stack, async runs and help overlay

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 15: fake store, screen dependencies and Goodbye

**Files:**
- Create: `internal/reddit/redditest/fake.go`, `internal/ui/screens/deps.go`, `internal/ui/screens/goodbye.go`
- Test: `internal/ui/screens/helpers_test.go`, `internal/ui/screens/goodbye_test.go`

**Interfaces:**
- Consumes: `reddit.Store`, `config.Config`, `session.Session`, `ui.*`, `widgets.*`, `term.*`.
- Produces:
  - `redditest.FakeStore` with fields `Listings map[string]reddit.Listing` (key `sub/sort/after`), `Threads map[string]reddit.Thread` (key post ID), `Subtrees map[string]reddit.Thread` (key comment ID), `More reddit.Things`, `Err error`, `Calls []string`, `Block chan struct{}` (when non-nil every call waits for it to close); constructor `NewFakeStore()`; builders `SamplePost(id, title string) *reddit.Post`, `SampleListing(n int, after string) reddit.Listing`, `SampleThread() reddit.Thread` (mirrors `testdata/thread.json`).
  - `screens.Deps struct { Store reddit.Store; MakeStore func(id, secret string) reddit.Store; Config *config.Config; Session *session.Session; Open func(url string, onExit func(error)) error; Now func() time.Time; Version string }`.
  - `screens.Rune(k term.Key) rune` upper-cased rune for non-paste rune keys, else 0; `screens.IsBack(k term.Key) bool` for Q, q or Escape.
  - `screens.SelectComment struct{ ID string }` pop result type.
  - `screens.NewGoodbye(d *Deps) *Goodbye`; `screens.validSubreddit(name string) (string, bool)` trims `r/` and validates `^[A-Za-z0-9_]{2,21}$`.
  - test helper `newDeps(t) (*Deps, *redditest.FakeStore)` in `screens/helpers_test.go`.

- [ ] **Step 1: Write the fake store**

`internal/reddit/redditest/fake.go`:

```go
// Package redditest provides a fake Store and sample data for UI tests.
package redditest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/markwatson/redditbbs/internal/reddit"
)

// FakeStore serves canned data. Keys: Listings "sub/sort/after", Threads by
// post ID, Subtrees by comment ID.
type FakeStore struct {
	mu       sync.Mutex
	Listings map[string]reddit.Listing
	Threads  map[string]reddit.Thread
	Subtrees map[string]reddit.Thread
	More     reddit.Things
	Err      error
	Calls    []string
	Block    chan struct{}
}

// NewFakeStore returns an empty fake.
func NewFakeStore() *FakeStore {
	return &FakeStore{Listings: map[string]reddit.Listing{}, Threads: map[string]reddit.Thread{}, Subtrees: map[string]reddit.Thread{}}
}

func (f *FakeStore) record(ctx context.Context, call string) error {
	f.mu.Lock()
	f.Calls = append(f.Calls, call)
	block, err := f.Block, f.Err
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// CallCount returns how many calls were recorded.
func (f *FakeStore) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

// Posts implements Store.
func (f *FakeStore) Posts(ctx context.Context, sub string, sort reddit.Sort, after string, fe reddit.Fetch) (reddit.Listing, error) {
	if err := f.record(ctx, fmt.Sprintf("posts:%s/%s/%s/fresh=%v", sub, sort, after, fe.Fresh)); err != nil {
		return reddit.Listing{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.Listings[sub+"/"+string(sort)+"/"+after]
	if !ok {
		return reddit.Listing{}, &reddit.APIError{Status: 404}
	}
	return l, nil
}

// Thread implements Store.
func (f *FakeStore) Thread(ctx context.Context, sub, postID string, sort reddit.CommentSort, fe reddit.Fetch) (reddit.Thread, error) {
	if err := f.record(ctx, fmt.Sprintf("thread:%s/%s/%s/fresh=%v", sub, postID, sort, fe.Fresh)); err != nil {
		return reddit.Thread{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	th, ok := f.Threads[postID]
	if !ok {
		return reddit.Thread{}, &reddit.APIError{Status: 404}
	}
	return th, nil
}

// Subtree implements Store.
func (f *FakeStore) Subtree(ctx context.Context, sub, postID, commentID string, sort reddit.CommentSort) (reddit.Thread, error) {
	if err := f.record(ctx, "subtree:"+commentID); err != nil {
		return reddit.Thread{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	th, ok := f.Subtrees[commentID]
	if !ok {
		return reddit.Thread{}, &reddit.APIError{Status: 404}
	}
	return th, nil
}

// MoreChildren implements Store.
func (f *FakeStore) MoreChildren(ctx context.Context, link string, ids []string, sort reddit.CommentSort) (reddit.Things, error) {
	if err := f.record(ctx, fmt.Sprintf("more:%s/%d", link, len(ids))); err != nil {
		return reddit.Things{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.More, nil
}

var _ reddit.Store = (*FakeStore)(nil)

var base = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// SamplePost builds a link post.
func SamplePost(id, title string) *reddit.Post {
	return &reddit.Post{
		ID: id, Fullname: "t3_" + id, Subreddit: "linux", Title: title, Author: "author_" + id,
		Score: 100, NumComments: 10, Created: base.Add(-3 * time.Hour),
		URL: "https://example.com/" + id, Domain: "example.com", Permalink: "/r/linux/comments/" + id + "/",
	}
}

// SampleListing builds n posts titled "Post 1".."Post n" with IDs p1..pn.
func SampleListing(n int, after string) reddit.Listing {
	l := reddit.Listing{After: after}
	for i := 1; i <= n; i++ {
		l.Posts = append(l.Posts, SamplePost(fmt.Sprintf("p%d", i), fmt.Sprintf("Post %d", i)))
	}
	return l
}

// SampleThread mirrors internal/reddit/testdata/thread.json.
func SampleThread() reddit.Thread {
	post := SamplePost("aaa", "Kernel 7.2 released")
	post.Author = "torvaldsfan"
	post.NumComments = 342
	c2 := &reddit.Comment{ID: "c2", Fullname: "t1_c2", ParentFullname: "t1_c1", Author: "torvaldsfan", Body: "Agreed.", Score: 98, Created: base.Add(-time.Hour), Depth: 1, IsSubmitter: true}
	c1 := &reddit.Comment{ID: "c1", Fullname: "t1_c1", ParentFullname: "t3_aaa", Author: "sched_nerd", Body: "The EEVDF changes are the headline.\n\nLazy preemption is the real win.", Score: 412, Created: base.Add(-2 * time.Hour), Children: []*reddit.Comment{c2}, More: &reddit.MoreStub{ParentFullname: "t1_c1", Count: 3, IDs: []string{"x", "y", "z"}}}
	c3 := &reddit.Comment{ID: "c3", Fullname: "t1_c3", ParentFullname: "t3_aaa", Author: "[deleted]", AuthorDeleted: true, Body: "Body survives the account.", Score: 5, Created: base.Add(-2 * time.Hour)}
	c4 := &reddit.Comment{ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Author: "modbot", Body: "[removed]", BodyRemoved: true, Score: 1, Created: base.Add(-2 * time.Hour), Distinguished: "moderator", More: &reddit.MoreStub{ParentFullname: "t1_c4"}}
	return reddit.Thread{Post: post, Comments: []*reddit.Comment{c1, c3, c4}, More: &reddit.MoreStub{ParentFullname: "t3_aaa", Count: 40, IDs: []string{"p", "q"}}}
}
```

- [ ] **Step 2: Write deps.go**

`internal/ui/screens/deps.go`:

```go
// Package screens implements each BBS screen on top of the ui package.
package screens

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/session"
	"github.com/markwatson/redditbbs/internal/term"
)

// Deps is everything screens need from the outside. Shared by pointer so
// New User Setup can install the Store.
type Deps struct {
	Store     reddit.Store
	MakeStore func(id, secret string) reddit.Store
	Config    *config.Config
	Session   *session.Session
	Open      func(url string, onExit func(error)) error
	Now       func() time.Time
	Version   string
}

// SelectComment is the Pop result Message Reader hands back to Thread Index.
type SelectComment struct{ ID string }

// Rune returns the upper-cased rune of a typed (not pasted) rune key, else 0.
func Rune(k term.Key) rune {
	if k.Code != term.KeyRune || k.Paste {
		return 0
	}
	return unicode.ToUpper(k.Rune)
}

// IsBack reports Q or Escape.
func IsBack(k term.Key) bool { return k.Code == term.KeyEscape || Rune(k) == 'Q' }

var subredditRe = regexp.MustCompile(`^[A-Za-z0-9_]{2,21}$`)

// validSubreddit trims whitespace and a leading r/ and validates the name.
func validSubreddit(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(strings.TrimPrefix(name, "/r/"), "r/")
	return name, subredditRe.MatchString(name)
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// errText shortens an error for the status line.
func errText(err error) string {
	if ae, ok := err.(*reddit.APIError); ok {
		switch ae.Status {
		case 401:
			return "Credentials rejected"
		case 403:
			if ae.Reason != "" {
				return "Access denied (" + ae.Reason + ")"
			}
			return "Access denied"
		case 404:
			return "No such area"
		case 429:
			return "Rate limited by Reddit"
		}
		if ae.Status >= 500 {
			return "Reddit is having trouble (HTTP " + itoa(ae.Status) + ")"
		}
	}
	s := err.Error()
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
```

- [ ] **Step 3: Write the test helper and failing Goodbye tests**

`internal/ui/screens/helpers_test.go`:

```go
package screens

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/session"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui"
)

var testNow = time.Date(2026, 9, 26, 20, 30, 0, 0, time.UTC)

func newDeps(t *testing.T) (*Deps, *redditest.FakeStore) {
	t.Helper()
	fs := redditest.NewFakeStore()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetCredentials("id", "secret")
	cfg.AddArea(config.Area{Name: "Linux", Subreddit: "linux"})
	cfg.AddArea(config.Area{Name: "Rust", Subreddit: "rust"})
	d := &Deps{
		Store:   fs,
		Config:  cfg,
		Session: session.New(testNow.Add(-5 * time.Minute)),
		Open:    func(string, func(error)) error { return nil },
		Now:     func() time.Time { return testNow },
		Version: "test",
	}
	return d, fs
}

// run mounts a screen in an App on an 80x24 sim and draws it.
func run(t *testing.T, s ui.Screen) (*ui.App, *term.Sim) {
	t.Helper()
	sim := term.NewSim(80, 24)
	app := ui.New(sim, s)
	app.Draw()
	return app, sim
}

// pump waits for one async result and redraws.
func pump(t *testing.T, app *ui.App) {
	t.Helper()
	if !app.Pump(2 * time.Second) {
		t.Fatal("no async result arrived")
	}
	app.Draw()
}

func press(app *ui.App, keys ...term.Key) {
	for _, k := range keys {
		app.Handle(k)
	}
	app.Draw()
}

func typeString(app *ui.App, s string) {
	for _, r := range s {
		app.Handle(term.R(r))
	}
	app.Draw()
}

func mustContain(t *testing.T, sim *term.Sim, wants ...string) {
	t.Helper()
	out := sim.String()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("screen missing %q:\n%s", w, out)
		}
	}
}

func mustNotContain(t *testing.T, sim *term.Sim, wants ...string) {
	t.Helper()
	out := sim.String()
	for _, w := range wants {
		if strings.Contains(out, w) {
			t.Errorf("screen should not contain %q:\n%s", w, out)
		}
	}
}
```

`internal/ui/screens/goodbye_test.go`:

```go
package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui"
)

func TestGoodbyeShowsStatsAndQuitsOnKey(t *testing.T) {
	d, _ := newDeps(t)
	d.Session.VisitArea("linux")
	d.Session.ThreadsOpened = 2
	d.Session.MessagesRead = 7
	app, sim := run(t, NewGoodbye(d))
	mustContain(t, sim, "Thanks for calling", "5m", "1", "2", "7")
	press(app, term.R(' '))
	if !app.Quitting() {
		t.Error("any key should quit")
	}
}

func TestGoodbyeQuitsOnTick(t *testing.T) {
	d, _ := newDeps(t)
	g := NewGoodbye(d)
	if act := g.Update(ui.Tick{ID: goodbyeTick}); act != (ui.Quit{}) {
		t.Errorf("tick action = %#v", act)
	}
	if act := g.Update(ui.Tick{ID: 99}); act != nil {
		t.Errorf("foreign tick should be ignored, got %#v", act)
	}
}
```

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/ui/screens/`
Expected: FAIL, undefined: NewGoodbye, goodbyeTick.

- [ ] **Step 5: Implement goodbye.go**

```go
package screens

import (
	"context"
	"fmt"
	"time"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

const goodbyeTick = 1

// Goodbye is the sign-off screen.
type Goodbye struct{ d *Deps }

// NewGoodbye creates the goodbye screen.
func NewGoodbye(d *Deps) *Goodbye { return &Goodbye{d: d} }

func (g *Goodbye) Init() ui.Action {
	return ui.Sleep(context.Background(), 2*time.Second, goodbyeTick)
}

func (g *Goodbye) Update(m ui.Msg) ui.Action {
	if t, ok := m.(ui.Tick); ok && t.ID == goodbyeTick {
		return ui.Quit{}
	}
	return nil
}

func (g *Goodbye) HandleKey(term.Key) ui.Action { return ui.Quit{} }
func (g *Goodbye) Title() string               { return "Goodbye" }
func (g *Goodbye) Keys() []ui.KeyHelp          { return nil }
func (g *Goodbye) Fullscreen() bool            { return true }

func (g *Goodbye) Draw(c term.Canvas) {
	_, h := c.Size()
	s := g.d.Session
	online := s.Online(g.d.now()).Round(time.Minute)
	y := h/2 - 5
	if y < 0 {
		y = 0
	}
	widgets.Centre(c, y, "╒═══════════════════════════════════════╕", theme.Style(theme.Frame))
	widgets.Centre(c, y+1, "Thanks for calling RedditBBS", theme.Style(theme.Logo))
	widgets.Centre(c, y+2, "╘═══════════════════════════════════════╛", theme.Style(theme.Frame))
	stats := []string{
		fmt.Sprintf("Time online ....... %s", fmtDuration(online)),
		fmt.Sprintf("Areas visited ..... %d", s.AreasVisited()),
		fmt.Sprintf("Threads opened .... %d", s.ThreadsOpened),
		fmt.Sprintf("Messages read ..... %d", s.MessagesRead),
		fmt.Sprintf("Links opened ...... %d", s.LinksOpened),
	}
	for i, l := range stats {
		widgets.Centre(c, y+4+i, l, theme.Style(theme.Body))
	}
	widgets.Centre(c, y+10, "NO CARRIER", theme.Style(theme.Meta))
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return "under 1m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/ui/screens/ ./internal/reddit/redditest/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/reddit/redditest internal/ui/screens
git commit -m "Add fake store, screen dependencies and goodbye screen

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 16: Splash, Main Menu and Area List

**Files:**
- Create: `internal/ui/screens/splash.go`, `internal/ui/screens/mainmenu.go`, `internal/ui/screens/arealist.go`
- Test: `internal/ui/screens/splash_test.go`, `internal/ui/screens/mainmenu_test.go`, `internal/ui/screens/arealist_test.go`

**Interfaces:**
- Consumes: `Deps`, `Rune`, `IsBack`, `validSubreddit`, `errText` (Task 15); `NewGoodbye` (Task 15); `NewSetup` (Task 17, referenced by Splash: add a stub `func NewSetup(d *Deps) ui.Screen` returning `NewMainMenu(d)` in `setup.go` for this task only if Task 17 has not landed; Task 17 replaces it); `NewPostList(d *Deps, area config.Area, saved bool) ui.Screen` (Task 18: until it lands, `arealist.go` and `mainmenu.go` call a package-level variable `var newPostList = func(d *Deps, area config.Area, saved bool) ui.Screen { return NewMainMenu(d) }` which Task 18 reassigns to the real constructor).
- Produces: `NewSplash(d *Deps) *Splash`, `NewMainMenu(d *Deps) *MainMenu`, `NewAreaList(d *Deps) *AreaList`.

- [ ] **Step 1: Write the failing tests**

`internal/ui/screens/splash_test.go`:

```go
package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestSplashShowsLogoAndGoesToMenu(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewSplash(d))
	mustContain(t, sim, "Node 1", "26/09/2026", "Press any key to log on", "vtest")
	press(app, term.R('x'))
	if _, ok := app.Top().(*MainMenu); !ok || app.Depth() != 1 {
		t.Errorf("expected MainMenu on top, got %T depth %d", app.Top(), app.Depth())
	}
}
```

`internal/ui/screens/mainmenu_test.go`:

```go
package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
)

func TestMainMenuLists(t *testing.T) {
	d, _ := newDeps(t)
	_, sim := run(t, NewMainMenu(d))
	mustContain(t, sim, "[M] Message areas", "[J] Join area", "[G] Goodbye", "Main Menu")
}

func TestMainMenuOpensAreaList(t *testing.T) {
	d, _ := newDeps(t)
	app, _ := run(t, NewMainMenu(d))
	press(app, term.R('m'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Errorf("got %T", app.Top())
	}
}

func TestMainMenuLogoffConfirmation(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('q'))
	mustContain(t, sim, "Log off? (y/N)")
	press(app, term.R('n'))
	mustNotContain(t, sim, "Log off?")
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatal("n should cancel")
	}
	press(app, term.K(term.KeyEscape), term.R('y'))
	if _, ok := app.Top().(*Goodbye); !ok {
		t.Errorf("expected Goodbye, got %T", app.Top())
	}
}

func TestMainMenuJoinValidatesName(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('j'))
	mustContain(t, sim, "Join area:")
	typeString(app, "bad name!")
	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Invalid area name")
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatal("invalid name must stay on menu")
	}
	press(app, term.R('j'))
	typeString(app, "r/linux")
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*MainMenu); ok {
		t.Error("valid name should open the post list")
	}
}

func TestMainMenuQuestionMarkInsideJoinIsLiteral(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewMainMenu(d))
	press(app, term.R('j'), term.R('?'))
	if app.Depth() != 1 {
		t.Error("? during text entry must not open help")
	}
	mustContain(t, sim, "Join area: ?")
}
```

`internal/ui/screens/arealist_test.go`:

```go
package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
)

func TestAreaListRowsAndCursor(t *testing.T) {
	d, _ := newDeps(t)
	_, sim := run(t, NewAreaList(d))
	mustContain(t, sim, "Linux", "r/linux", "Rust", "r/rust", "2 areas")
	if _, st := sim.CellAt(2, 5); st != theme.Style(theme.Cursor) {
		t.Errorf("first row should be the cursor row, style %+v", st)
	}
}

func TestAreaListDeleteWithConfirmation(t *testing.T) {
	d, _ := newDeps(t)
	app, sim := run(t, NewAreaList(d))
	press(app, term.K(term.KeyDown), term.R('d'))
	mustContain(t, sim, "Delete Rust? (y/N)")
	press(app, term.R('y'))
	mustNotContain(t, sim, "r/rust")
	if len(d.Config.Areas) != 1 {
		t.Errorf("areas = %+v", d.Config.Areas)
	}
	again, _ := loadConfig(t, d.Config.Path)
	if len(again.Areas) != 1 {
		t.Error("deletion not saved")
	}
}

func TestAreaListOpenByNumber(t *testing.T) {
	d, _ := newDeps(t)
	app, _ := run(t, NewAreaList(d))
	press(app, term.R('2'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*AreaList); ok {
		t.Error("number selection should open the area")
	}
}

func TestAreaListEmpty(t *testing.T) {
	d, _ := newDeps(t)
	d.Config.Areas = nil
	app, sim := run(t, NewAreaList(d))
	mustContain(t, sim, "No areas configured")
	press(app, term.K(term.KeyEnter), term.R('d'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Error("Enter and D on an empty list must do nothing")
	}
}
```

Add to `helpers_test.go`:

```go
func loadConfig(t *testing.T, path string) (*config.Config, error) {
	t.Helper()
	return config.Load(path, func(string) string { return "" })
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/screens/`
Expected: FAIL, undefined: NewSplash, NewMainMenu, NewAreaList.

- [ ] **Step 3: Implement splash.go**

```go
package screens

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

var logoLines = []string{
	"██████╗ ███████╗██████╗ ██████╗ ██╗████████╗    ██████╗ ██████╗ ███████╗",
	"██╔══██╗██╔════╝██╔══██╗██╔══██╗██║╚══██╔══╝    ██╔══██╗██╔══██╗██╔════╝",
	"██████╔╝█████╗  ██║  ██║██║  ██║██║   ██║       ██████╔╝██████╔╝███████╗",
	"██╔══██╗██╔══╝  ██║  ██║██║  ██║██║   ██║       ██╔══██╗██╔══██╗╚════██║",
	"██║  ██║███████╗██████╔╝██████╔╝██║   ██║       ██████╔╝██████╔╝███████║",
	"╚═╝  ╚═╝╚══════╝╚═════╝ ╚═════╝ ╚═╝   ╚═╝       ╚═════╝ ╚═════╝ ╚══════╝",
}

// Splash is the logon screen.
type Splash struct{ d *Deps }

// NewSplash creates the splash screen.
func NewSplash(d *Deps) *Splash { return &Splash{d: d} }

func (s *Splash) Init() ui.Action         { return nil }
func (s *Splash) Update(ui.Msg) ui.Action { return nil }
func (s *Splash) Title() string           { return "Logon" }
func (s *Splash) Keys() []ui.KeyHelp      { return nil }
func (s *Splash) Fullscreen() bool        { return true }

func (s *Splash) Draw(c term.Canvas) {
	_, h := c.Size()
	y := h/2 - 7
	if y < 0 {
		y = 0
	}
	for i, l := range logoLines {
		st := theme.Style(theme.Logo)
		if i >= 3 {
			st = theme.Style(theme.Frame)
		}
		widgets.Centre(c, y+i, l, st)
	}
	y += len(logoLines) + 1
	widgets.Centre(c, y, "A bulletin board window onto Reddit", theme.Style(theme.Subject))
	widgets.Centre(c, y+2, "Node 1  ·  "+s.d.now().Format("02/01/2006 15:04")+"  ·  v"+s.d.Version, theme.Style(theme.Meta))
	widgets.Centre(c, y+4, "Press any key to log on", theme.Style(theme.Hotkey))
}

func (s *Splash) HandleKey(term.Key) ui.Action {
	if s.d.Config.HasCredentials() {
		return ui.Replace{Screen: NewMainMenu(s.d)}
	}
	return ui.Replace{Screen: NewSetup(s.d)}
}
```

- [ ] **Step 4: Implement mainmenu.go**

```go
package screens

import (
	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newPostList is indirected so this task compiles before Post List exists.
var newPostList = func(d *Deps, area config.Area, saved bool) ui.Screen { return NewMainMenu(d) }

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
	return []ui.KeyHelp{{"M", "essage areas"}, {"J", "oin area"}, {"?", "Help"}, {"G", "oodbye"}}
}
func (m *MainMenu) CapturesKeys() bool { return m.confirm || m.join != nil }

func (m *MainMenu) Prompt() widgets.Prompt {
	switch {
	case m.confirm:
		return widgets.Prompt{Label: "Log off? (y/N)"}
	case m.join != nil:
		return widgets.Prompt{Label: "Join area:", Input: m.join.Display(), Cursor: true}
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
```

- [ ] **Step 5: Implement arealist.go**

```go
package screens

import (
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// AreaList shows the configured subreddits.
type AreaList struct {
	d         *Deps
	table     widgets.Table
	num       widgets.NumInput
	confirm   bool
	status    string
	statusErr bool
}

// NewAreaList creates the area list.
func NewAreaList(d *Deps) *AreaList {
	a := &AreaList{d: d}
	a.table.SetHeight(17)
	a.table.SetCount(len(d.Config.Areas))
	return a
}

func (a *AreaList) Init() ui.Action { return nil }
func (a *AreaList) Title() string   { return "Message Areas" }
func (a *AreaList) Info() string    { return itoa(len(a.d.Config.Areas)) + " areas" }
func (a *AreaList) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{"#/⏎", "Open"}, {"D", "elete"}, {"Q", "uit"}}
}
func (a *AreaList) CapturesKeys() bool { return a.confirm }

func (a *AreaList) Prompt() widgets.Prompt {
	if a.confirm {
		return widgets.Prompt{Label: "Delete " + a.d.Config.Areas[a.table.Cursor].Name + "? (y/N)"}
	}
	return widgets.Prompt{Input: a.num.Digits, Status: a.status, Error: a.statusErr}
}

func (a *AreaList) Update(msg ui.Msg) ui.Action {
	if _, ok := msg.(ui.PopResult); ok {
		a.status = ""
	}
	return nil
}

func (a *AreaList) Draw(c term.Canvas) {
	w, h := c.Size()
	areas := a.d.Config.Areas
	a.table.SetHeight(h - 2)
	a.table.SetCount(len(areas))
	if len(areas) == 0 {
		widgets.Centre(c, h/2, "No areas configured. Use [J]oin from the main menu.", theme.Style(theme.Meta))
		return
	}
	c.Text(2, 0, "  #  Area                      Subreddit", theme.Style(theme.Heading), w)
	widgets.Rule(c, 1)
	start, end := a.table.Visible()
	for i := start; i < end; i++ {
		y := 2 + i - start
		line := textfmt.PadLeft(itoa(i+1), 3) + "  " + textfmt.PadRight(areas[i].Name, 24) + "  r/" + areas[i].Subreddit
		if i == a.table.Cursor {
			c.Fill(0, y, w, 1, ' ', theme.Style(theme.Cursor))
			c.Text(2, y, line, theme.Style(theme.Cursor), w-2)
			continue
		}
		x := 2
		x += c.Text(x, y, textfmt.PadLeft(itoa(i+1), 3)+"  ", theme.Style(theme.Meta), w)
		x += c.Text(x, y, textfmt.PadRight(areas[i].Name, 24)+"  ", theme.Style(theme.Subject), w)
		c.Text(x, y, "r/"+areas[i].Subreddit, theme.Style(theme.Author), w-x)
	}
}

func (a *AreaList) HandleKey(k term.Key) ui.Action {
	areas := a.d.Config.Areas
	if a.confirm {
		a.confirm = false
		if Rune(k) == 'Y' {
			a.d.Config.RemoveArea(a.table.Cursor)
			a.table.SetCount(len(a.d.Config.Areas))
			if err := a.d.Config.Save(); err != nil {
				a.status, a.statusErr = "Could not save config: "+errText(err), true
			}
		}
		return nil
	}
	if v, submitted, handled := a.num.HandleKey(k); handled {
		if submitted {
			return a.open(v - 1)
		}
		return nil
	}
	if a.table.HandleKey(k) {
		return nil
	}
	switch {
	case k.Code == term.KeyEnter:
		return a.open(a.table.Cursor)
	case Rune(k) == 'D':
		if len(areas) > 0 {
			a.confirm = true
		}
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

func (a *AreaList) open(i int) ui.Action {
	areas := a.d.Config.Areas
	if i < 0 || i >= len(areas) {
		if len(areas) > 0 {
			a.status, a.statusErr = "No such area", true
		}
		return nil
	}
	a.status = ""
	return ui.Push{Screen: newPostList(a.d, areas[i], true)}
}
```

If Task 17 has not been done yet, add to a temporary `internal/ui/screens/setup.go`:

```go
package screens

import "github.com/markwatson/redditbbs/internal/ui"

// NewSetup is replaced by the real New User Setup screen in Task 17.
func NewSetup(d *Deps) ui.Screen { return NewMainMenu(d) }
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/ui/screens/`
Expected: PASS. `TestAreaListRowsAndCursor` checks cell (2,5): content row 2 is terminal row 5, and the cursor row is filled in the Cursor style.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screens
git commit -m "Add splash, main menu and area list screens

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 17: New User Setup screen

**Files:**
- Create or replace: `internal/ui/screens/setup.go`
- Test: `internal/ui/screens/setup_test.go`, add a case to `splash_test.go`

**Interfaces:**
- Consumes: `Deps.MakeStore`, `config.Config.SetCredentials`, `config.DefaultAreas`, `reddit.Store`.
- Produces: `NewSetup(d *Deps) *Setup` (replaces the Task 16 stub; keep the name and make the return type `*Setup`, which still satisfies the `ui.Screen` uses).

- [ ] **Step 1: Write the failing tests**

`internal/ui/screens/setup_test.go`:

```go
package screens

import (
	"errors"
	"testing"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/term"
)

func setupDeps(t *testing.T) (*Deps, *redditest.FakeStore) {
	d, _ := newDeps(t)
	d.Config.SetCredentials("", "")
	d.Config.Areas = nil
	d.Store = nil
	made := redditest.NewFakeStore()
	made.Listings["linux/hot/"] = redditest.SampleListing(2, "")
	d.MakeStore = func(id, secret string) reddit.Store {
		if id != "myid" || secret != "mysecret" {
			made.Err = &reddit.APIError{Status: 401}
		}
		return made
	}
	return d, made
}

func TestSetupHappyPath(t *testing.T) {
	d, made := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	mustContain(t, sim, "prefs/apps", "Client ID", "Client secret", "New User Setup")
	typeString(app, "myid")
	press(app, term.K(term.KeyTab))
	typeString(app, "mysecret")
	mustContain(t, sim, "********")
	mustNotContain(t, sim, "mysecret")
	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Checking credentials")
	pump(t, app)
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatalf("expected MainMenu, got %T", app.Top())
	}
	if d.Store != made || !d.Config.HasCredentials() || len(d.Config.Areas) != 4 {
		t.Errorf("store=%v creds=%v areas=%d", d.Store == made, d.Config.HasCredentials(), len(d.Config.Areas))
	}
	saved, err := loadConfig(t, d.Config.Path)
	if err != nil || saved.ClientID() != "myid" || len(saved.Areas) != 4 {
		t.Errorf("saved = %+v err=%v", saved, err)
	}
}

func TestSetupRejectedCredentialsStay(t *testing.T) {
	d, _ := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	typeString(app, "wrong")
	press(app, term.K(term.KeyEnter)) // Enter on the ID field moves to the secret field
	typeString(app, "x")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	if _, ok := app.Top().(*Setup); !ok {
		t.Fatal("rejected credentials must stay on setup")
	}
	mustContain(t, sim, "Credentials rejected")
	if d.Config.HasCredentials() {
		t.Error("rejected credentials must not be stored")
	}
}

func TestSetupPreservesExistingAreas(t *testing.T) {
	d, _ := setupDeps(t)
	d.Config.Areas = []config.Area{{Name: "Rust", Subreddit: "rust"}}
	app, _ := run(t, NewSetup(d))
	typeString(app, "myid")
	press(app, term.K(term.KeyTab))
	typeString(app, "mysecret")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	if len(d.Config.Areas) != 1 || d.Config.Areas[0].Subreddit != "rust" {
		t.Errorf("areas = %+v", d.Config.Areas)
	}
}

func TestSetupEmptyFieldsAndEscape(t *testing.T) {
	d, _ := setupDeps(t)
	app, sim := run(t, NewSetup(d))
	press(app, term.K(term.KeyEnter), term.K(term.KeyEnter))
	mustContain(t, sim, "Both fields are required")
	press(app, term.K(term.KeyEscape))
	if !app.Quitting() {
		t.Error("Escape on setup should quit")
	}
}

func TestSetupWithoutMakeStore(t *testing.T) {
	d, _ := setupDeps(t)
	d.MakeStore = nil
	s := NewSetup(d)
	s.id.Value, s.secret.Value = "a", "b"
	if act := s.submit(); act != nil {
		t.Errorf("submit without MakeStore should not start a run, got %#v", act)
	}
	if !errors.Is(s.lastErr, errNoStoreFactory) {
		t.Errorf("lastErr = %v", s.lastErr)
	}
}
```

Add to `splash_test.go`:

```go
func TestSplashGoesToSetupWithoutCredentials(t *testing.T) {
	d, _ := newDeps(t)
	d.Config.SetCredentials("", "")
	app, _ := run(t, NewSplash(d))
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*Setup); !ok {
		t.Errorf("expected Setup, got %T", app.Top())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/screens/ -run Setup`
Expected: FAIL: `NewSetup` returns `ui.Screen` (stub) so `*Setup` is undefined.

- [ ] **Step 3: Implement setup.go**

```go
package screens

import (
	"context"
	"errors"
	"strings"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

var errNoStoreFactory = errors.New("no store factory configured")

// Setup collects and verifies app credentials on first run.
type Setup struct {
	d          *Deps
	id, secret widgets.TextInput
	focus      int
	busy       bool
	gen        int
	status     string
	statusErr  bool
	lastErr    error
}

type setupMsg struct {
	gen   int
	store reddit.Store
	err   error
}

// NewSetup creates the setup screen.
func NewSetup(d *Deps) *Setup {
	s := &Setup{d: d}
	s.secret.Mask = true
	return s
}

func (s *Setup) Init() ui.Action    { return nil }
func (s *Setup) Title() string      { return "New User Setup" }
func (s *Setup) CapturesKeys() bool { return true }
func (s *Setup) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{"Tab", "Next field"}, {"⏎", "Continue"}, {"Esc", "Quit"}}
}

func (s *Setup) Prompt() widgets.Prompt {
	if s.busy {
		return widgets.Prompt{Status: "Checking credentials with Reddit..."}
	}
	return widgets.Prompt{Status: s.status, Error: s.statusErr}
}

var setupText = []string{
	"Welcome, new user! RedditBBS reads Reddit through its official API,",
	"which needs a free 'script' app registered to your Reddit account.",
	"",
	"  1. Visit https://www.reddit.com/prefs/apps and choose 'create app'.",
	"  2. Pick the type 'script'. Any redirect URI will do, e.g. http://localhost:8080",
	"  3. Copy the client ID (shown under the app name) and the secret below.",
	"",
	"Reddit may need to approve the app before requests succeed.",
	"Credentials are stored with mode 0600 in your config file.",
}

func (s *Setup) Draw(c term.Canvas) {
	w, _ := c.Size()
	for i, l := range setupText {
		st := theme.Style(theme.Body)
		if i == 0 {
			st = theme.Style(theme.Subject)
		}
		c.Text(2, i, l, st, w-2)
	}
	y := len(setupText) + 1
	fieldX, fieldW := 18, w-20
	c.Text(2, y, "Client ID:", theme.Style(theme.Heading), w)
	c.Text(2, y+2, "Client secret:", theme.Style(theme.Heading), w)
	c.Fill(fieldX, y, fieldW, 1, '_', theme.Style(theme.Meta))
	c.Fill(fieldX, y+2, fieldW, 1, '_', theme.Style(theme.Meta))
	s.id.Draw(c, fieldX, y, fieldW, theme.Style(theme.Subject), s.focus == 0 && !s.busy)
	s.secret.Draw(c, fieldX, y+2, fieldW, theme.Style(theme.Subject), s.focus == 1 && !s.busy)
}

func (s *Setup) HandleKey(k term.Key) ui.Action {
	if k.Code == term.KeyEscape {
		return ui.Quit{}
	}
	if s.busy {
		return nil
	}
	switch k.Code {
	case term.KeyTab:
		s.focus = 1 - s.focus
		return nil
	case term.KeyEnter:
		if s.focus == 0 {
			s.focus = 1
			return nil
		}
		return s.submit()
	}
	if s.focus == 0 {
		s.id.HandleKey(k)
	} else {
		s.secret.HandleKey(k)
	}
	return nil
}

// submit verifies the typed credentials asynchronously.
func (s *Setup) submit() ui.Action {
	id, secret := strings.TrimSpace(s.id.Value), strings.TrimSpace(s.secret.Value)
	if id == "" || secret == "" {
		s.status, s.statusErr = "Both fields are required", true
		return nil
	}
	if s.d.MakeStore == nil {
		s.lastErr = errNoStoreFactory
		s.status, s.statusErr = "Internal error: no Reddit client available", true
		return nil
	}
	store := s.d.MakeStore(id, secret)
	s.busy = true
	s.gen++
	gen := s.gen
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		return setupMsg{gen: gen, store: store, err: verifyStore(ctx, store)}
	}}
}

func verifyStore(ctx context.Context, store reddit.Store) error {
	if v, ok := store.(interface{ Verify(context.Context) error }); ok {
		return v.Verify(ctx)
	}
	_, err := store.Posts(ctx, "linux", reddit.Hot, "", reddit.Fetch{Fresh: true})
	return err
}

func (s *Setup) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case setupMsg:
		if m.gen != s.gen {
			return nil
		}
		s.busy = false
		if m.err != nil {
			s.lastErr = m.err
			s.status, s.statusErr = errText(m.err), true
			return nil
		}
		s.d.Store = m.store
		s.d.Config.SetCredentials(strings.TrimSpace(s.id.Value), strings.TrimSpace(s.secret.Value))
		if len(s.d.Config.Areas) == 0 {
			s.d.Config.Areas = append([]config.Area(nil), config.DefaultAreas...)
		}
		if err := s.d.Config.Save(); err != nil {
			s.lastErr = err
			s.status, s.statusErr = "Could not save config: "+errText(err), true
			return nil
		}
		return ui.Replace{Screen: NewMainMenu(s.d)}
	case ui.RateLimited:
		s.status, s.statusErr = "Rate limited, retrying in "+itoa(int(m.Wait.Seconds()))+"s", false
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/screens/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screens
git commit -m "Add new user setup screen with credential verification

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 18: Post List screen

**Files:**
- Create: `internal/ui/screens/postlist.go`
- Modify: `internal/ui/screens/mainmenu.go` (the `newPostList` variable now calls `NewPostList`)
- Test: `internal/ui/screens/postlist_test.go`

**Interfaces:**
- Consumes: `Deps`, `widgets.NumInput`, `widgets.TextInput`, `textfmt`, `reddit.Store.Posts`.
- Produces: `NewPostList(d *Deps, area config.Area, saved bool) *PostList`; package variable `var newThreadIndex = func(d *Deps, p *reddit.Post) ui.Screen { return NewMainMenu(d) }` which Task 20 reassigns.

- [ ] **Step 1: Write the failing tests**

`internal/ui/screens/postlist_test.go`:

```go
package screens

import (
	"strings"
	"testing"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
)

var linuxArea = config.Area{Name: "Linux", Subreddit: "linux"}

func postListWith(t *testing.T, n int) (*ui.App, *term.Sim, *Deps, *redditest.FakeStore) {
	t.Helper()
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(n, "")
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	return app, sim, d, fs
}

func TestPostListLoadsAndDraws(t *testing.T) {
	_, sim, d, _ := postListWith(t, 3)
	mustContain(t, sim, "Post 1", "Post 2", "Post 3", "author_p1", "r/linux · HOT · Page 1", "Subject", "From", "Msgs")
	if _, st := sim.CellAt(4, 5); st != theme.Style(theme.Cursor) {
		t.Errorf("first data row should be the cursor row, got %+v", st)
	}
	if d.Session.AreasVisited() != 1 {
		t.Error("visiting an area should be recorded")
	}
}

func TestPostListEmpty(t *testing.T) {
	app, sim, _, _ := postListWith(t, 0)
	mustContain(t, sim, "No messages")
	press(app, term.K(term.KeyEnter), term.R('1'), term.K(term.KeyEnter), term.K(term.KeyDown))
	if _, ok := app.Top().(*PostList); !ok || app.Depth() != 1 {
		t.Error("Enter on an empty list must do nothing")
	}
}

func TestPostListNumberOpens(t *testing.T) {
	app, _, d, _ := postListWith(t, 3)
	press(app, term.R('2'), term.K(term.KeyEnter))
	if _, ok := app.Top().(*PostList); ok {
		t.Error("number selection should open the thread")
	}
	if d.Session.ThreadsOpened != 1 {
		t.Error("ThreadsOpened not counted")
	}
}

func TestPostListBadNumber(t *testing.T) {
	app, sim, _, _ := postListWith(t, 3)
	press(app, term.R('9'), term.K(term.KeyEnter))
	mustContain(t, sim, "No such message")
	if _, ok := app.Top().(*PostList); !ok {
		t.Error("bad number must stay")
	}
}

func TestPostListPagination(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(20, "t3_p20")
	page2 := reddit.Listing{}
	for i := 21; i <= 25; i++ {
		page2.Posts = append(page2.Posts, redditest.SamplePost("p"+itoa(i), "Post "+itoa(i)))
	}
	fs.Listings["linux/hot/t3_p20"] = page2
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	// 80x24: content 19 rows, heading 2, so 17 data rows per page.
	mustContain(t, sim, "Post 1", "Post 17")
	mustNotContain(t, sim, "Post 18")
	press(app, term.R('n'))
	mustContain(t, sim, "Post 18", "Post 20", "Page 2")
	press(app, term.R('n')) // beyond loaded posts: fetches the next Reddit page
	pump(t, app)
	mustContain(t, sim, "Post 21", "Post 25", "Page 2")
	press(app, term.R('n'))
	mustContain(t, sim, "End of messages")
	press(app, term.R('p'))
	mustContain(t, sim, "Post 1", "Page 1")
	if fs.CallCount() != 2 {
		t.Errorf("store calls = %d, want 2", fs.CallCount())
	}
}

func TestPostListDownAtEndFetchesMore(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(2, "t3_p2")
	fs.Listings["linux/hot/t3_p2"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("p3", "Post 3")}}
	app, sim := run(t, NewPostList(d, linuxArea, true))
	pump(t, app)
	press(app, term.K(term.KeyDown), term.K(term.KeyDown))
	pump(t, app)
	press(app, term.K(term.KeyDown))
	if _, st := sim.CellAt(4, 7); st != theme.Style(theme.Cursor) {
		t.Error("cursor should reach the newly loaded third row")
	}
	press(app, term.K(term.KeyDown)) // ended: no further fetch
	if fs.CallCount() != 2 {
		t.Errorf("calls = %d", fs.CallCount())
	}
}

func TestPostListSortChangeKeepsOldUntilArrival(t *testing.T) {
	app, sim, _, fs := postListWith(t, 2)
	fs.Listings["linux/new/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("n1", "Fresh 1")}}
	fs.Block = make(chan struct{})
	press(app, term.R('s'))
	mustContain(t, sim, "Post 1", "Retrieving...", "NEW")
	close(fs.Block)
	pump(t, app)
	mustContain(t, sim, "Fresh 1")
	mustNotContain(t, sim, "Post 1")
}

func TestPostListErrorKeepsContent(t *testing.T) {
	app, sim, _, fs := postListWith(t, 2)
	fs.Err = &reddit.APIError{Status: 503}
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "Post 1", "Reddit is having trouble")
	last := fs.Calls[len(fs.Calls)-1]
	if !strings.Contains(last, "fresh=true") {
		t.Errorf("refresh should bypass cache: %s", last)
	}
}

func TestPostListResizeClampsCursor(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(20, "")
	sim := term.NewSim(80, 24)
	app := ui.New(sim, NewPostList(d, linuxArea, true), ui.WithMinSize(40, 10)) // allow the small size below
	app.Draw()
	pump(t, app)
	press(app, term.K(term.KeyEnd))
	mustContain(t, sim, "Post 20", "Page 2")
	sim.Resize(80, 12) // content 7 rows, 5 data rows per page
	app.Handle(<-sim.Events())
	app.Draw()
	mustContain(t, sim, "Post 20", "Page 4")
	press(app, term.K(term.KeyUp))
	mustContain(t, sim, "Post 19")
}

func TestPostListJoinReplaces(t *testing.T) {
	app, sim, _, fs := postListWith(t, 1)
	fs.Listings["rust/hot/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("r1", "Rusty")}}
	press(app, term.R('j'))
	typeString(app, "rust")
	press(app, term.K(term.KeyEnter))
	pump(t, app)
	pl, ok := app.Top().(*PostList)
	if !ok || pl.area.Subreddit != "rust" || app.Depth() != 1 {
		t.Fatalf("top = %T depth %d", app.Top(), app.Depth())
	}
	mustContain(t, sim, "Rusty", "r/rust")
}

func TestPostListAddArea(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["golang/hot/"] = redditest.SampleListing(1, "")
	app, sim := run(t, NewPostList(d, config.Area{Name: "r/golang", Subreddit: "golang"}, false))
	pump(t, app)
	mustContain(t, sim, "[A]dd area")
	press(app, term.R('a'))
	if !d.Config.HasArea("golang") {
		t.Error("area not added")
	}
	mustContain(t, sim, "Area saved")
	mustNotContain(t, sim, "[A]dd area")
}

func TestPostListOpenLink(t *testing.T) {
	app, _, d, _ := postListWith(t, 1)
	var opened string
	d.Open = func(u string, _ func(error)) error { opened = u; return nil }
	press(app, term.R('o'))
	if opened != "https://example.com/p1" || d.Session.LinksOpened != 1 {
		t.Errorf("opened=%q links=%d", opened, d.Session.LinksOpened)
	}
}

func TestPostListStaleResultIgnored(t *testing.T) {
	app, sim, _, fs := postListWith(t, 1)
	fs.Listings["linux/new/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("n1", "New 1")}}
	fs.Listings["linux/top/"] = reddit.Listing{Posts: []*reddit.Post{redditest.SamplePost("t1", "Top 1")}}
	press(app, term.R('s'), term.R('s')) // hot -> new -> top; the "new" result is stale
	pump(t, app)
	pump(t, app)
	mustContain(t, sim, "Top 1")
	mustNotContain(t, sim, "New 1")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/screens/ -run PostList`
Expected: FAIL, undefined: NewPostList.

- [ ] **Step 3: Implement postlist.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newThreadIndex is indirected so this task compiles before Thread Index exists.
var newThreadIndex = func(d *Deps, p *reddit.Post) ui.Screen { return NewMainMenu(d) }

// PostList shows one subreddit's posts, paginated to the screen height.
type PostList struct {
	d     *Deps
	area  config.Area
	saved bool
	sort  reddit.Sort

	posts  []*reddit.Post
	seen   map[string]bool
	after  string
	ended  bool
	cursor int
	rows   int // data rows per page, set on Draw

	num  widgets.NumInput
	join *widgets.TextInput

	loading        bool
	pendingAdvance bool
	gen            int
	status         string
	statusErr      bool
}

type postsMsg struct {
	gen     int
	listing reddit.Listing
	err     error
	replace bool
}

// NewPostList creates a post list for area. saved says whether the area is in
// the config already.
func NewPostList(d *Deps, area config.Area, saved bool) *PostList {
	s := &PostList{d: d, area: area, saved: saved, sort: reddit.Sort(d.Config.Display.DefaultSort), seen: map[string]bool{}, rows: 17}
	if s.sort == "" {
		s.sort = reddit.Hot
	}
	return s
}

func (s *PostList) Init() ui.Action {
	s.d.Session.VisitArea(s.area.Subreddit)
	return s.fetch("", true, false)
}

func (s *PostList) Title() string { return "Message Area" }

func (s *PostList) Info() string {
	return fmt.Sprintf("r/%s · %s · Page %d", s.area.Subreddit, strings.ToUpper(string(s.sort)), s.page()+1)
}

func (s *PostList) Keys() []ui.KeyHelp {
	keys := []ui.KeyHelp{{"#/⏎", "Read"}, {"N", "ext"}, {"P", "rev"}, {"S", "ort"}, {"J", "oin"}}
	if !s.saved {
		keys = append(keys, ui.KeyHelp{Key: "A", Desc: "dd area"})
	}
	return append(keys, ui.KeyHelp{Key: "O", Desc: "pen link"}, ui.KeyHelp{Key: "R", Desc: "efresh"}, ui.KeyHelp{Key: "Q", Desc: "uit"})
}

func (s *PostList) CapturesKeys() bool { return s.join != nil }

func (s *PostList) Prompt() widgets.Prompt {
	if s.join != nil {
		return widgets.Prompt{Label: "Join area:", Input: s.join.Display(), Cursor: true}
	}
	return widgets.Prompt{Input: s.num.Digits, Status: s.status, Error: s.statusErr}
}

func (s *PostList) page() int { return s.cursor / s.rows }

func (s *PostList) clamp() {
	if s.rows < 1 {
		s.rows = 1
	}
	if s.cursor >= len(s.posts) {
		s.cursor = len(s.posts) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// fetch starts a listing request; replace discards loaded posts on arrival.
func (s *PostList) fetch(after string, replace, fresh bool) ui.Action {
	s.gen++
	gen := s.gen
	s.loading = true
	s.status, s.statusErr = "Retrieving...", false
	store, sub, sort := s.d.Store, s.area.Subreddit, s.sort
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		l, err := store.Posts(ctx, sub, sort, after, reddit.Fetch{Fresh: fresh})
		return postsMsg{gen: gen, listing: l, err: err, replace: replace}
	}}
}

func (s *PostList) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case postsMsg:
		if m.gen != s.gen {
			return nil
		}
		s.loading = false
		advance := s.pendingAdvance
		s.pendingAdvance = false
		if m.err != nil {
			s.status, s.statusErr = errText(m.err), true
			return nil
		}
		if m.replace {
			s.posts, s.seen, s.cursor, s.ended, s.after = nil, map[string]bool{}, 0, false, ""
		}
		wasPage := s.page()
		added := 0
		for _, p := range m.listing.Posts {
			if !s.seen[p.ID] {
				s.seen[p.ID] = true
				s.posts = append(s.posts, p)
				added++
			}
		}
		if m.listing.After == "" || m.listing.After == s.after || added == 0 {
			s.ended = true
		}
		s.after = m.listing.After
		s.status, s.statusErr = "", false
		if len(s.posts) == 0 {
			s.status = "No messages"
		}
		if advance {
			if next := (wasPage + 1) * s.rows; next < len(s.posts) {
				s.cursor = next
			}
		}
		s.clamp()
	case ui.PopResult:
		s.status, s.statusErr = "", false
	case ui.RateLimited:
		s.status, s.statusErr = fmt.Sprintf("Rate limited, retrying in %ds", int(m.Wait.Seconds())), false
	}
	return nil
}

func (s *PostList) Draw(c term.Canvas) {
	w, h := c.Size()
	if rows := h - 2; rows != s.rows {
		s.rows = rows
		s.clamp()
	}
	if len(s.posts) == 0 {
		if !s.loading {
			widgets.Centre(c, h/2, "No messages in r/"+s.area.Subreddit, theme.Style(theme.Meta))
		}
		return
	}
	numW := len(itoa(s.rows))
	if numW < 2 {
		numW = 2
	}
	fromW, msgsW := 14, 5
	wide := w >= 100
	scoreW, ageW := 0, 0
	fixed := 2 + numW + 2 + 2 + fromW + 2 + msgsW + 1
	if wide {
		scoreW, ageW = 6, 4
		fixed += 2 + scoreW + 2 + ageW
	}
	subjW := w - fixed
	if subjW < 10 {
		subjW = 10
	}

	hd := theme.Style(theme.Heading)
	x := 2
	x += c.Text(x, 0, textfmt.PadLeft("#", numW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadRight("Subject", subjW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadRight("From", fromW)+"  ", hd, w)
	x += c.Text(x, 0, textfmt.PadLeft("Msgs", msgsW), hd, w)
	if wide {
		c.Text(x, 0, "  "+textfmt.PadLeft("Score", scoreW)+"  "+textfmt.PadLeft("Age", ageW), hd, w)
	}
	widgets.Rule(c, 1)

	start := s.page() * s.rows
	for i := start; i < start+s.rows && i < len(s.posts); i++ {
		p := s.posts[i]
		y := 2 + i - start
		sel := i == s.cursor
		st := func(r theme.Role) term.Style {
			if sel {
				return theme.Style(theme.Cursor)
			}
			return theme.Style(r)
		}
		if sel {
			c.Fill(0, y, w, 1, ' ', theme.Style(theme.Cursor))
		}
		x := 2
		x += c.Text(x, y, textfmt.PadLeft(itoa(i-start+1), numW)+"  ", st(theme.Meta), w)
		sx := x
		if p.Stickied {
			sx += c.Text(sx, y, "* ", st(theme.Sticky), w)
		}
		if p.Over18 {
			sx += c.Text(sx, y, "[X] ", st(theme.NSFW), w)
		}
		avail := x + subjW - sx
		domain := ""
		if !p.IsSelf && p.Domain != "" {
			domain = " (" + p.Domain + ")"
		}
		if textfmt.Width(p.Title)+textfmt.Width(domain) > avail {
			domain = ""
		}
		sx += c.Text(sx, y, textfmt.Truncate(p.Title, avail), st(theme.Subject), avail)
		if domain != "" {
			c.Text(sx, y, domain, st(theme.Meta), x+subjW-sx)
		}
		x += subjW + 2
		x += c.Text(x, y, textfmt.PadRight(p.Author, fromW)+"  ", st(theme.Author), w)
		x += c.Text(x, y, textfmt.PadLeft(itoa(p.NumComments), msgsW), st(theme.Body), w)
		if wide {
			c.Text(x, y, "  "+textfmt.PadLeft(textfmt.Score(p.Score), scoreW)+"  "+textfmt.PadLeft(textfmt.RelTime(p.Created, s.d.now()), ageW), st(theme.Meta), w)
		}
	}
}

func (s *PostList) HandleKey(k term.Key) ui.Action {
	if s.join != nil {
		switch s.join.HandleKey(k) {
		case widgets.InputSubmit:
			name, ok := validSubreddit(s.join.Value)
			s.join = nil
			if !ok {
				s.status, s.statusErr = "Invalid area name", true
				return nil
			}
			return ui.Replace{Screen: NewPostList(s.d, config.Area{Name: "r/" + name, Subreddit: name}, s.d.Config.HasArea(name))}
		case widgets.InputCancel:
			s.join = nil
		}
		return nil
	}
	if v, submitted, handled := s.num.HandleKey(k); handled {
		if submitted {
			return s.open(s.page()*s.rows + v - 1)
		}
		return nil
	}
	switch k.Code {
	case term.KeyUp:
		return s.move(-1)
	case term.KeyDown:
		return s.move(1)
	case term.KeyPgUp:
		return s.move(-s.rows)
	case term.KeyPgDn:
		return s.move(s.rows)
	case term.KeyHome:
		s.cursor = 0
		return nil
	case term.KeyEnd:
		s.cursor = len(s.posts) - 1
		s.clamp()
		return nil
	case term.KeyEnter:
		return s.open(s.cursor)
	}
	switch {
	case Rune(k) == 'N':
		return s.nextPage()
	case Rune(k) == 'P':
		if s.page() > 0 {
			s.cursor = (s.page() - 1) * s.rows
		}
	case Rune(k) == 'S':
		s.sort = s.sort.Next()
		return s.fetch("", true, false)
	case Rune(k) == 'J':
		s.join = &widgets.TextInput{}
	case Rune(k) == 'A':
		s.addArea()
	case Rune(k) == 'O':
		s.openLink()
	case Rune(k) == 'R':
		return s.fetch("", true, true)
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

// move shifts the cursor, fetching the next Reddit page when it runs off the
// end of what is loaded.
func (s *PostList) move(delta int) ui.Action {
	if len(s.posts) == 0 {
		return nil
	}
	target := s.cursor + delta
	if target >= len(s.posts) {
		s.cursor = len(s.posts) - 1
		if !s.ended && !s.loading {
			return s.fetch(s.after, false, false)
		}
		return nil
	}
	if target < 0 {
		target = 0
	}
	s.cursor = target
	return nil
}

func (s *PostList) nextPage() ui.Action {
	if next := (s.page() + 1) * s.rows; next < len(s.posts) {
		s.cursor = next
		return nil
	}
	if s.ended {
		s.status, s.statusErr = "End of messages", false
		return nil
	}
	if s.loading {
		return nil
	}
	s.pendingAdvance = true
	return s.fetch(s.after, false, false)
}

func (s *PostList) open(i int) ui.Action {
	if i < 0 || i >= len(s.posts) {
		if len(s.posts) > 0 {
			s.status, s.statusErr = "No such message", true
		}
		return nil
	}
	s.cursor = i
	s.status = ""
	s.d.Session.ThreadsOpened++
	return ui.Push{Screen: newThreadIndex(s.d, s.posts[i])}
}

func (s *PostList) addArea() {
	if s.saved {
		return
	}
	if !s.d.Config.AddArea(s.area) {
		s.saved = true
		return
	}
	if err := s.d.Config.Save(); err != nil {
		s.status, s.statusErr = "Could not save config: "+errText(err), true
		return
	}
	s.saved = true
	s.status, s.statusErr = "Area saved", false
}

func (s *PostList) openLink() {
	if len(s.posts) == 0 {
		return
	}
	p := s.posts[s.cursor]
	u := p.URL
	if p.IsSelf || u == "" {
		u = "https://www.reddit.com" + p.Permalink
	}
	if err := s.d.Open(u, nil); err != nil {
		s.status, s.statusErr = "Could not open browser. URL: "+u, true
		return
	}
	s.d.Session.LinksOpened++
	s.status, s.statusErr = "Opened in browser", false
}
```

In `mainmenu.go`, change the indirection to the real constructor:

```go
var newPostList = func(d *Deps, area config.Area, saved bool) ui.Screen { return NewPostList(d, area, saved) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/screens/`
Expected: PASS. Notes for failures: `TestPostListDownAtEndFetchesMore` checks terminal row 7 (content row 4, the third data row). `TestPostListResizeClampsCursor` at 80x12 has content height 7 and 5 data rows, so post 20 (index 19) is on page 4 (index 3).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screens
git commit -m "Add post list screen with local pagination and sorting

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 19: thread model: visible rows, numbering, collapse, splicing

**Files:**
- Create: `internal/ui/threadmodel/model.go`
- Test: `internal/ui/threadmodel/model_test.go`

**Interfaces:**
- Consumes: `reddit.Thread`, `reddit.Comment`, `reddit.MoreStub`, `reddit.Things`, `reddit.Attach`, `widgets.Connector`.
- Produces:
  - `type Row struct { Comment *reddit.Comment; Stub *reddit.MoreStub; Number, Depth int; Connector string; Collapsed bool; Hidden int }`.
  - `type Model` with `func New(t reddit.Thread, maxDepth int) *Model`, `Thread() *reddit.Thread`, `Post() *reddit.Post`, `Replace(t reddit.Thread)`, `SetMaxDepth(int)`, `Rows() []Row`, `Comments() []*reddit.Comment`, `Toggle(id string) bool`, `IsCollapsed(id string) bool`, `IndexOf(id string) int`, `Find(id string) *reddit.Comment`, `Parent(c *reddit.Comment) *reddit.Comment`, `Attach(stub *reddit.MoreStub, th reddit.Things) int`, `ReplaceSubtree(id string, sub reddit.Thread) bool`, `Loaded() int`.

- [ ] **Step 1: Write the failing tests**

`internal/ui/threadmodel/model_test.go`:

```go
package threadmodel

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
)

func ids(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case r.Comment != nil:
			out[i] = r.Comment.ID
		case r.Stub != nil && r.Stub.IsContinue():
			out[i] = "continue"
		default:
			out[i] = "more"
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRowsOrderNumbersAndConnectors(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	rows := m.Rows()
	want := []string{"c1", "c2", "more", "c3", "c4", "continue", "more"}
	if got := ids(rows); !equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i, r := range rows {
		if r.Number != i+1 {
			t.Errorf("row %d number = %d", i, r.Number)
		}
	}
	if rows[0].Depth != 0 || rows[0].Connector != "" {
		t.Errorf("c1 = %+v", rows[0])
	}
	if rows[1].Depth != 1 || rows[1].Connector != "├─" {
		t.Errorf("c2 connector = %q (a stub follows it)", rows[1].Connector)
	}
	if rows[2].Depth != 1 || rows[2].Connector != "└─" || rows[2].Stub.Count != 3 {
		t.Errorf("stub row = %+v", rows[2])
	}
	if rows[5].Connector != "└─" || !rows[5].Stub.IsContinue() {
		t.Errorf("continue row = %+v", rows[5])
	}
	if rows[6].Depth != 0 || rows[6].Stub.Count != 40 {
		t.Errorf("thread stub = %+v", rows[6])
	}
	if m.Loaded() != 4 {
		t.Errorf("loaded = %d", m.Loaded())
	}
}

func TestCollapseHidesDescendants(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	if !m.Toggle("c1") || !m.IsCollapsed("c1") {
		t.Fatal("toggle should collapse")
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got, []string{"c1", "c3", "c4", "continue", "more"}) {
		t.Fatalf("collapsed rows = %v", got)
	}
	if !rows[0].Collapsed || rows[0].Hidden != 1 {
		t.Errorf("c1 row = %+v", rows[0])
	}
	if m.Toggle("c1") {
		t.Error("second toggle should expand")
	}
	if len(m.Rows()) != 7 {
		t.Error("expand failed")
	}
	if got := len(m.Comments()); got != 4 {
		t.Errorf("comments = %d", got)
	}
}

func TestRowsOnlyStub(t *testing.T) {
	th := reddit.Thread{Post: redditest.SamplePost("z", "Empty"), More: &reddit.MoreStub{ParentFullname: "t3_z", Count: 12, IDs: []string{"a"}}}
	rows := New(th, 10).Rows()
	if len(rows) != 1 || rows[0].Stub == nil || rows[0].Number != 1 {
		t.Errorf("rows = %+v", rows)
	}
	if New(reddit.Thread{Post: th.Post}, 10).Rows() != nil {
		t.Error("no comments should give no rows")
	}
}

func TestRowsDeletedAuthorKeepsBody(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	r := m.Rows()[3]
	if r.Comment.ID != "c3" || !r.Comment.AuthorDeleted || r.Comment.Body != "Body survives the account." {
		t.Errorf("c3 = %+v", r.Comment)
	}
}

func TestAttachThroughModel(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	stub := m.Find("c1").More
	n := m.Attach(stub, reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Body: "X"}}})
	if n != 1 {
		t.Fatalf("attached %d", n)
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got[:4], []string{"c1", "c2", "x", "more"}) {
		t.Fatalf("rows = %v", got)
	}
	if rows[2].Depth != 1 || rows[2].Connector != "├─" || rows[3].Stub.Count != 2 {
		t.Errorf("x=%+v stub=%+v", rows[2], rows[3].Stub)
	}
}

func TestReplaceSubtree(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	sub := reddit.Thread{Post: m.Post(), Comments: []*reddit.Comment{{
		ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Body: "[removed]",
		Children: []*reddit.Comment{{ID: "k1", Fullname: "t1_k1", ParentFullname: "t1_c4", Body: "deep", Depth: 7,
			Children: []*reddit.Comment{{ID: "k2", Fullname: "t1_k2", ParentFullname: "t1_k1", Body: "deeper", Depth: 8}}}},
	}}}
	if !m.ReplaceSubtree("c4", sub) {
		t.Fatal("subtree not replaced")
	}
	rows := m.Rows()
	if got := ids(rows); !equal(got, []string{"c1", "c2", "more", "c3", "c4", "k1", "k2", "more"}) {
		t.Fatalf("rows = %v", got)
	}
	if rows[5].Depth != 1 || rows[6].Depth != 2 || m.Find("k1").Depth != 1 {
		t.Errorf("depths not renumbered: %+v %+v", rows[5], rows[6])
	}
	if m.ReplaceSubtree("nope", sub) {
		t.Error("unknown id should return false")
	}
}

func TestDepthCap(t *testing.T) {
	chain := &reddit.Comment{ID: "d0", Fullname: "t1_d0", ParentFullname: "t3_p"}
	cur := chain
	for i := 1; i <= 3; i++ {
		next := &reddit.Comment{ID: "d" + string(rune('0'+i)), Fullname: "t1_d" + string(rune('0'+i)), ParentFullname: cur.Fullname, Depth: i}
		cur.Children = []*reddit.Comment{next}
		cur = next
	}
	m := New(reddit.Thread{Post: redditest.SamplePost("p", "P"), Comments: []*reddit.Comment{chain}}, 2)
	rows := m.Rows()
	if rows[2].Connector != "  └─" {
		t.Errorf("depth 2 connector = %q", rows[2].Connector)
	}
	if rows[3].Connector != "  »─" {
		t.Errorf("depth 3 (beyond cap) connector = %q", rows[3].Connector)
	}
}

func TestParentIndexOfAndReplace(t *testing.T) {
	m := New(redditest.SampleThread(), 10)
	c2 := m.Find("c2")
	if p := m.Parent(c2); p == nil || p.ID != "c1" {
		t.Errorf("parent of c2 = %+v", p)
	}
	if m.Parent(m.Find("c1")) != nil {
		t.Error("top-level parent should be nil")
	}
	if m.IndexOf("c3") != 3 || m.IndexOf("zzz") != -1 {
		t.Errorf("IndexOf = %d %d", m.IndexOf("c3"), m.IndexOf("zzz"))
	}
	m.Toggle("c1")
	m.Toggle("ghost")
	m.Replace(redditest.SampleThread())
	if !m.IsCollapsed("c1") || m.IsCollapsed("ghost") {
		t.Error("Replace should keep collapse state for surviving ids only")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/threadmodel/`
Expected: FAIL, undefined: New, Row.

- [ ] **Step 3: Implement model.go**

```go
// Package threadmodel turns a comment tree into the flat, numbered list of
// rows the Thread Index shows, tracking collapse state and splicing in
// comments loaded later.
package threadmodel

import (
	"strings"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// Row is one line of the thread index: a comment or a stub.
type Row struct {
	Comment   *reddit.Comment
	Stub      *reddit.MoreStub
	Number    int // 1-based display number
	Depth     int
	Connector string
	Collapsed bool
	Hidden    int // loaded descendants hidden by collapse
}

// Model owns a mutable copy of a thread plus UI state.
type Model struct {
	thread    reddit.Thread
	collapsed map[string]bool
	maxDepth  int
}

// New wraps t. maxDepth is the deepest level drawn with its own indent.
func New(t reddit.Thread, maxDepth int) *Model {
	m := &Model{thread: t, collapsed: map[string]bool{}}
	m.SetMaxDepth(maxDepth)
	return m
}

// SetMaxDepth changes the indent cap (minimum 2).
func (m *Model) SetMaxDepth(d int) {
	if d < 2 {
		d = 2
	}
	m.maxDepth = d
}

// Thread exposes the underlying tree.
func (m *Model) Thread() *reddit.Thread { return &m.thread }

// Post is the thread's post.
func (m *Model) Post() *reddit.Post { return m.thread.Post }

// Replace swaps in a refetched thread, keeping collapse state for ids that survive.
func (m *Model) Replace(t reddit.Thread) {
	m.thread = t
	alive := map[string]bool{}
	m.walk(func(c *reddit.Comment) { alive[c.ID] = true })
	for id := range m.collapsed {
		if !alive[id] {
			delete(m.collapsed, id)
		}
	}
}

func (m *Model) walk(fn func(*reddit.Comment)) {
	var rec func([]*reddit.Comment)
	rec = func(cs []*reddit.Comment) {
		for _, c := range cs {
			fn(c)
			rec(c.Children)
		}
	}
	rec(m.thread.Comments)
}

// Loaded counts comments in the tree.
func (m *Model) Loaded() int {
	n := 0
	m.walk(func(*reddit.Comment) { n++ })
	return n
}

// Find returns the comment with id, or nil.
func (m *Model) Find(id string) *reddit.Comment {
	var found *reddit.Comment
	m.walk(func(c *reddit.Comment) {
		if c.ID == id && found == nil {
			found = c
		}
	})
	return found
}

// Parent returns c's parent comment, or nil for a top-level comment.
func (m *Model) Parent(c *reddit.Comment) *reddit.Comment {
	if c == nil || !strings.HasPrefix(c.ParentFullname, "t1_") {
		return nil
	}
	return m.Find(strings.TrimPrefix(c.ParentFullname, "t1_"))
}

// Toggle flips collapse for id and returns the new state.
func (m *Model) Toggle(id string) bool {
	m.collapsed[id] = !m.collapsed[id]
	if !m.collapsed[id] {
		delete(m.collapsed, id)
	}
	return m.collapsed[id]
}

// IsCollapsed reports collapse state.
func (m *Model) IsCollapsed(id string) bool { return m.collapsed[id] }

func countDescendants(c *reddit.Comment) int {
	n := 0
	for _, k := range c.Children {
		n += 1 + countDescendants(k)
	}
	return n
}

func (m *Model) connector(depth int, anc []bool, hasNext bool) string {
	if depth == 0 {
		return ""
	}
	if depth > m.maxDepth {
		return strings.Repeat("  ", m.maxDepth-1) + "»─"
	}
	return widgets.Connector(anc[1:], hasNext)
}

// Rows flattens the tree in display order, skipping collapsed descendants.
// anc carries one entry per ancestor level (true when that ancestor has later
// siblings), so len(anc) is the depth of the comments being walked.
func (m *Model) Rows() []Row {
	var rows []Row
	var walk func(cs []*reddit.Comment, anc []bool, trailingStub bool)
	walk = func(cs []*reddit.Comment, anc []bool, trailingStub bool) {
		depth := len(anc)
		for i, c := range cs {
			hasNext := i < len(cs)-1 || trailingStub
			row := Row{Comment: c, Depth: depth, Connector: m.connector(depth, anc, hasNext)}
			if m.collapsed[c.ID] && (len(c.Children) > 0 || c.More != nil) {
				row.Collapsed = true
				row.Hidden = countDescendants(c)
				rows = append(rows, row)
				continue
			}
			rows = append(rows, row)
			childAnc := append(append([]bool(nil), anc...), hasNext)
			walk(c.Children, childAnc, c.More != nil)
			if c.More != nil {
				rows = append(rows, Row{Stub: c.More, Depth: depth + 1, Connector: m.connector(depth+1, childAnc, false)})
			}
		}
	}
	walk(m.thread.Comments, nil, m.thread.More != nil)
	if m.thread.More != nil {
		rows = append(rows, Row{Stub: m.thread.More})
	}
	for i := range rows {
		rows[i].Number = i + 1
	}
	return rows
}

// Comments returns the visible comments in display order, without stubs.
func (m *Model) Comments() []*reddit.Comment {
	var out []*reddit.Comment
	for _, r := range m.Rows() {
		if r.Comment != nil {
			out = append(out, r.Comment)
		}
	}
	return out
}

// IndexOf returns the row index of comment id, or -1.
func (m *Model) IndexOf(id string) int {
	for i, r := range m.Rows() {
		if r.Comment != nil && r.Comment.ID == id {
			return i
		}
	}
	return -1
}

// Attach splices morechildren results for stub into the tree.
func (m *Model) Attach(stub *reddit.MoreStub, th reddit.Things) int {
	return reddit.Attach(&m.thread, stub, th)
}

// ReplaceSubtree replaces comment id's children with those of the matching
// root comment in sub (a Subtree response), renumbering depths.
func (m *Model) ReplaceSubtree(id string, sub reddit.Thread) bool {
	target := m.Find(id)
	if target == nil {
		return false
	}
	var root *reddit.Comment
	for _, c := range sub.Comments {
		if c.ID == id {
			root = c
			break
		}
	}
	if root == nil {
		return false
	}
	target.Children, target.More = root.Children, root.More
	redepth(target.Children, target.Depth+1)
	return true
}

func redepth(cs []*reddit.Comment, d int) {
	for _, c := range cs {
		c.Depth = d
		redepth(c.Children, d+1)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/threadmodel/`
Expected: PASS. In `TestDepthCap` with `maxDepth` 2, depth 2 gets `Connector([false], false)` = `"  └─"` and depth 3 gets the cap string `"  »─"`.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/threadmodel
git commit -m "Add thread model for visible rows, collapse and splicing

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 20: Thread Index screen

**Files:**
- Create: `internal/ui/screens/threadindex.go`
- Modify: `internal/ui/screens/postlist.go` (`newThreadIndex` now calls `NewThreadIndex`)
- Test: `internal/ui/screens/threadindex_test.go`

**Interfaces:**
- Consumes: `threadmodel.Model`, `widgets.Table`, `textfmt`, `reddit.Store.Thread/Subtree/MoreChildren`.
- Produces: `NewThreadIndex(d *Deps, post *reddit.Post) *ThreadIndex`; package variable `var newReader = func(d *Deps, m *threadmodel.Model, commentID string) ui.Screen { return NewMainMenu(d) }` which Task 21 reassigns.

- [ ] **Step 1: Write the failing tests**

`internal/ui/screens/threadindex_test.go`:

```go
package screens

import (
	"strings"
	"testing"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
)

func threadIndexUp(t *testing.T) (*ui.App, *term.Sim, *Deps, *redditest.FakeStore, *ThreadIndex) {
	t.Helper()
	d, fs := newDeps(t)
	th := redditest.SampleThread()
	fs.Threads["aaa"] = th
	ti := NewThreadIndex(d, th.Post)
	app, sim := run(t, ti)
	pump(t, app)
	return app, sim, d, fs, ti
}

func TestThreadIndexLoadsAndDraws(t *testing.T) {
	_, sim, _, _, ti := threadIndexUp(t)
	mustContain(t, sim,
		"Kernel 7.2 released", "torvaldsfan", "342 comments",
		"sched_nerd", "├─torvaldsfan", "Agreed.",
		"[load 3 more replies]", "[continue this thread]", "[load 40 more replies]",
		"342 msgs", "BEST",
		"The EEVDF changes are the headline.", // peek pane body of the cursor comment
	)
	rowY := 3 + 2 + 1 + 1 // title bar + header + rule + table heading
	if _, st := sim.CellAt(3, rowY); st != theme.Style(theme.Cursor) {
		t.Errorf("first comment row should be highlighted, got %+v", st)
	}
	if ti.table.Height() < 5 {
		t.Errorf("table height = %d", ti.table.Height())
	}
}

func TestThreadIndexDeletedAuthorShowsBody(t *testing.T) {
	_, sim, _, _, _ := threadIndexUp(t)
	mustContain(t, sim, "[deleted]", "Body survives the account.", "[removed]")
}

func TestThreadIndexCollapseAndExpand(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	press(app, term.R('-'))
	mustContain(t, sim, "[+1 hidden]")
	mustNotContain(t, sim, "Agreed.")
	press(app, term.R('+'))
	mustContain(t, sim, "Agreed.")
	mustNotContain(t, sim, "hidden]")
}

func TestThreadIndexPeekToggle(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	mustContain(t, sim, "Lazy preemption is the real win.")
	press(app, term.K(term.KeyTab))
	mustNotContain(t, sim, "Lazy preemption is the real win.")
	press(app, term.K(term.KeyTab))
	mustContain(t, sim, "Lazy preemption is the real win.")
}

func TestThreadIndexPeekFollowsCursor(t *testing.T) {
	app, sim, _, _, _ := threadIndexUp(t)
	press(app, term.K(term.KeyDown))
	// Peek shows c2 (Agreed.) and the index still shows c1's preview.
	if strings.Count(sim.String(), "Agreed.") < 2 {
		t.Errorf("peek should show the selected comment's body:\n%s", sim.String())
	}
	press(app, term.K(term.KeyDown)) // stub row
	mustContain(t, sim, "Press Enter to load")
}

func TestThreadIndexLoadMore(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.More = reddit.Things{Comments: []*reddit.Comment{{ID: "x", Fullname: "t1_x", ParentFullname: "t1_c1", Author: "xorg4life", Body: "X11 forever"}}}
	press(app, term.R('3'), term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "xorg4life", "X11 forever", "[load 2 more replies]")
	last := fs.Calls[len(fs.Calls)-1]
	if last != "more:t3_aaa/3" {
		t.Errorf("last call = %s", last)
	}
}

func TestThreadIndexContinueThread(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.Subtrees["c4"] = reddit.Thread{Post: redditest.SamplePost("aaa", "x"), Comments: []*reddit.Comment{{
		ID: "c4", Fullname: "t1_c4", ParentFullname: "t3_aaa", Author: "modbot", Body: "[removed]",
		Children: []*reddit.Comment{{ID: "k1", Fullname: "t1_k1", ParentFullname: "t1_c4", Author: "deepdiver", Body: "found it"}},
	}}}
	press(app, term.R('6'), term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "deepdiver", "found it")
	mustNotContain(t, sim, "[continue this thread]")
}

func TestThreadIndexOpenReaderAndReselect(t *testing.T) {
	app, _, _, _, ti := threadIndexUp(t)
	press(app, term.K(term.KeyEnter))
	if _, ok := app.Top().(*ThreadIndex); ok {
		t.Fatal("Enter on a comment should open the reader")
	}
	ti.Update(ui.PopResult{Result: SelectComment{ID: "c3"}})
	if ti.table.Cursor != 3 {
		t.Errorf("cursor = %d, want row of c3", ti.table.Cursor)
	}
}

func TestThreadIndexSortRefetchAndInfo(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	press(app, term.R('s'))
	pump(t, app)
	mustContain(t, sim, "TOP")
	if last := fs.Calls[len(fs.Calls)-1]; !strings.HasPrefix(last, "thread:linux/aaa/top") {
		t.Errorf("last call = %s", last)
	}
}

func TestThreadIndexErrorKeepsRows(t *testing.T) {
	app, sim, _, fs, _ := threadIndexUp(t)
	fs.Err = &reddit.APIError{Status: 500}
	press(app, term.R('r'))
	pump(t, app)
	mustContain(t, sim, "sched_nerd", "Reddit is having trouble")
}

func TestThreadIndexSelfPostPreviewAndBody(t *testing.T) {
	d, fs := newDeps(t)
	th := redditest.SampleThread()
	th.Post.IsSelf = true
	th.Post.SelfText = "Ask **anything** here.\n\nSecond paragraph."
	fs.Threads["aaa"] = th
	app, sim := run(t, NewThreadIndex(d, th.Post))
	pump(t, app)
	mustContain(t, sim, "Ask anything here.")
	press(app, term.R('b'))
	if _, ok := app.Top().(*ThreadIndex); ok {
		t.Error("B should open the post body in the reader")
	}
}

func TestThreadIndexOpenLinkAndQuit(t *testing.T) {
	app, _, d, _, _ := threadIndexUp(t)
	var opened string
	d.Open = func(u string, _ func(error)) error { opened = u; return nil }
	press(app, term.R('o'))
	if opened != "https://example.com/aaa" {
		t.Errorf("opened %q", opened)
	}
	press(app, term.R('q'))
	if !app.Quitting() {
		t.Error("Q on the only screen pops to quit")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/screens/ -run ThreadIndex`
Expected: FAIL, undefined: NewThreadIndex.

- [ ] **Step 3: Implement threadindex.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/threadmodel"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// newReader is indirected so this task compiles before Message Reader exists.
var newReader = func(d *Deps, m *threadmodel.Model, commentID string) ui.Screen { return NewMainMenu(d) }

const (
	minTableRows = 5
	minPeekRows  = 5
	authorColW   = 22
	scoreColW    = 6
)

// ThreadIndex lists a post's comments as a tree with a peek pane.
type ThreadIndex struct {
	d    *Deps
	post *reddit.Post
	sort reddit.CommentSort

	model *threadmodel.Model
	rows  []threadmodel.Row
	table widgets.Table
	num   widgets.NumInput

	peek    bool
	peekTop int
	peekH   int // body lines in the peek pane at the last draw

	selectedID string
	loading    bool
	gen        int
	status     string
	statusErr  bool
}

type threadMsg struct {
	gen int
	th  reddit.Thread
	err error
}

type moreMsg struct {
	gen    int
	stub   *reddit.MoreStub
	things reddit.Things
	err    error
}

type subtreeMsg struct {
	gen int
	id  string
	th  reddit.Thread
	err error
}

// NewThreadIndex creates the index for post.
func NewThreadIndex(d *Deps, post *reddit.Post) *ThreadIndex {
	t := &ThreadIndex{d: d, post: post, sort: reddit.Best, peek: d.Config.Display.PeekPane}
	t.table.SetHeight(minTableRows)
	return t
}

func (t *ThreadIndex) Init() ui.Action { return t.fetch(false) }
func (t *ThreadIndex) Title() string   { return "Thread Index" }

func (t *ThreadIndex) Info() string {
	return fmt.Sprintf("r/%s · %s · %d msgs", t.post.Subreddit, strings.ToUpper(string(t.sort)), t.post.NumComments)
}

func (t *ThreadIndex) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{"#/⏎", "Read"}, {"B", "ody"}, {"-/+", "Fold"}, {"Tab", "Peek"}, {"S", "ort"}, {"O", "pen link"}, {"R", "efresh"}, {"Q", "uit"}}
}

func (t *ThreadIndex) Prompt() widgets.Prompt {
	return widgets.Prompt{Input: t.num.Digits, Status: t.status, Error: t.statusErr}
}

func (t *ThreadIndex) fetch(fresh bool) ui.Action {
	t.gen++
	gen := t.gen
	t.loading = true
	t.status, t.statusErr = "Retrieving...", false
	store, sub, id, sort := t.d.Store, t.post.Subreddit, t.post.ID, t.sort
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		th, err := store.Thread(ctx, sub, id, sort, reddit.Fetch{Fresh: fresh})
		return threadMsg{gen: gen, th: th, err: err}
	}}
}

// refresh recomputes rows after any model change and restores the selection.
func (t *ThreadIndex) refresh() {
	t.rows = t.model.Rows()
	t.table.SetCount(len(t.rows))
	if t.selectedID != "" {
		if i := t.model.IndexOf(t.selectedID); i >= 0 {
			t.table.Select(i)
		}
	}
	t.syncSelected()
}

func (t *ThreadIndex) syncSelected() {
	if t.table.Cursor < len(t.rows) && t.rows[t.table.Cursor].Comment != nil {
		t.selectedID = t.rows[t.table.Cursor].Comment.ID
	}
}

func (t *ThreadIndex) Update(msg ui.Msg) ui.Action {
	switch m := msg.(type) {
	case threadMsg:
		if m.gen != t.gen {
			return nil
		}
		t.loading = false
		if m.err != nil {
			t.status, t.statusErr = errText(m.err), true
			return nil
		}
		if t.model == nil {
			t.model = threadmodel.New(m.th, 10)
		} else {
			t.model.Replace(m.th)
		}
		t.status, t.statusErr = "", false
		t.refresh()
	case moreMsg:
		if m.gen != t.gen {
			return nil
		}
		t.loading = false
		if m.err != nil {
			t.status, t.statusErr = errText(m.err), true
			return nil
		}
		t.model.Attach(m.stub, m.things)
		t.status = ""
		t.refresh()
	case subtreeMsg:
		if m.gen != t.gen {
			return nil
		}
		t.loading = false
		if m.err != nil {
			t.status, t.statusErr = errText(m.err), true
			return nil
		}
		if !t.model.ReplaceSubtree(m.id, m.th) {
			t.status, t.statusErr = "Could not load that part of the thread", true
		} else {
			t.status = ""
		}
		t.refresh()
	case ui.PopResult:
		if sc, ok := m.Result.(SelectComment); ok && sc.ID != "" && t.model != nil {
			t.selectedID = sc.ID
			t.refresh()
		}
	case ui.RateLimited:
		t.status, t.statusErr = fmt.Sprintf("Rate limited, retrying in %ds", int(m.Wait.Seconds())), false
	}
	return nil
}

func (t *ThreadIndex) Draw(c term.Canvas) {
	w, h := c.Size()
	p := t.post
	c.Text(2, 0, textfmt.Truncate(p.Title, w-4), theme.Style(theme.Subject), w-4)
	meta := fmt.Sprintf("by %s · %s · %s · %d comments", p.Author, textfmt.Score(p.Score), textfmt.RelTime(p.Created, t.d.now()), p.NumComments)
	c.Text(2, 1, textfmt.Truncate(meta, w-4), theme.Style(theme.Meta), w-4)
	if t.model == nil {
		return
	}
	t.model.SetMaxDepth(w / 8)

	// Row budget: header 2, preview, rule, table heading + rows, [rule, peek].
	var preview []textfmt.Line
	previewRows := 0
	if p.IsSelf && p.SelfText != "" {
		preview = textfmt.Render(p.SelfText, w-4).Lines
		previewRows = len(preview)
		if previewRows > h/4 {
			previewRows = h / 4
		}
	}
	peekRows := 0
	if t.peek {
		peekRows = h / 3
		if peekRows < minPeekRows {
			peekRows = minPeekRows
		}
	}
	avail := func() int {
		a := h - 2 - previewRows - 1 - 1
		if peekRows > 0 {
			a -= peekRows + 1
		}
		return a
	}
	for avail() < minTableRows && previewRows > 0 {
		previewRows--
	}
	for avail() < minTableRows && peekRows > minPeekRows {
		peekRows--
	}
	if avail() < minTableRows && peekRows > 0 {
		peekRows = 0
	}
	tableRows := avail()
	if tableRows < 1 {
		tableRows = 1
	}
	t.table.SetHeight(tableRows)
	t.peekH = peekRows - 1

	y := 2
	for i := 0; i < previewRows; i++ {
		widgets.DrawLine(c, 2, y+i, w-4, preview[i])
	}
	y += previewRows
	widgets.Rule(c, y)
	y++

	numW := len(itoa(len(t.rows)))
	if numW < 3 {
		numW = 3
	}
	hd := theme.Style(theme.Heading)
	c.Text(2, y, textfmt.PadLeft("#", numW)+"  "+textfmt.PadLeft("Score", scoreColW)+"  "+textfmt.PadRight("From", authorColW)+"  Preview", hd, w-2)
	y++
	previewW := w - (2 + numW + 2 + scoreColW + 2 + authorColW + 2) - 1
	start, end := t.table.Visible()
	for i := start; i < end; i++ {
		row := t.rows[i]
		ry := y + i - start
		sel := i == t.table.Cursor
		st := func(r theme.Role) term.Style {
			if sel {
				return theme.Style(theme.Cursor)
			}
			return theme.Style(r)
		}
		if sel {
			c.Fill(0, ry, w, 1, ' ', theme.Style(theme.Cursor))
		}
		x := 2
		x += c.Text(x, ry, textfmt.PadLeft(itoa(row.Number), numW)+"  ", st(theme.Meta), w)
		if row.Stub != nil {
			x += c.Text(x, ry, textfmt.PadLeft("", scoreColW)+"  ", st(theme.Meta), w)
			label := "[load " + itoa(row.Stub.Count) + " more replies]"
			if row.Stub.IsContinue() {
				label = "[continue this thread]"
			}
			c.Text(x, ry, row.Connector+label, st(theme.Stub), w-x-2)
			continue
		}
		cm := row.Comment
		x += c.Text(x, ry, textfmt.PadLeft(textfmt.Score(cm.Score), scoreColW)+"  ", st(theme.Meta), w)
		author, role := authorAndRole(cm, p)
		name := row.Connector + author
		if row.Collapsed {
			name += " [+" + itoa(row.Hidden) + " hidden]"
		}
		x += c.Text(x, ry, textfmt.PadRight(name, authorColW)+"  ", st(role), w)
		prev, prole := textfmt.FirstLine(cm.Body), theme.Body
		if cm.BodyRemoved {
			prev, prole = cm.Body, theme.Meta
		}
		c.Text(x, ry, textfmt.Truncate(prev, previewW), st(prole), previewW)
	}
	y += t.table.Height()

	if peekRows > 0 {
		widgets.Rule(c, y)
		y++
		t.drawPeek(c, y, w, peekRows)
	}
}

// authorAndRole picks the display name and colour role for a comment author.
func authorAndRole(cm *reddit.Comment, post *reddit.Post) (string, theme.Role) {
	switch {
	case cm.AuthorDeleted:
		return "[deleted]", theme.Meta
	case cm.Distinguished != "":
		return cm.Author, theme.Mod
	case cm.IsSubmitter || post.IsOP(cm.Author):
		return cm.Author, theme.OP
	}
	return cm.Author, theme.Author
}

func (t *ThreadIndex) drawPeek(c term.Canvas, y, w, rows int) {
	if len(t.rows) == 0 {
		return
	}
	row := t.rows[t.table.Cursor]
	if row.Comment == nil {
		c.Text(2, y, "Press Enter to load these replies", theme.Style(theme.Meta), w-4)
		return
	}
	cm := row.Comment
	author, role := authorAndRole(cm, t.post)
	x := 2
	x += c.Text(x, y, author, theme.Style(role), w-4)
	c.Text(x, y, " · "+textfmt.Score(cm.Score)+" · "+textfmt.RelTime(cm.Created, t.d.now()), theme.Style(theme.Meta), w-2-x)
	body := cm.Body
	lines := textfmt.Render(body, w-4).Lines
	bodyRows := rows - 1
	if max := len(lines) - bodyRows; t.peekTop > max {
		t.peekTop = max
	}
	if t.peekTop < 0 {
		t.peekTop = 0
	}
	for i := 0; i < bodyRows && t.peekTop+i < len(lines); i++ {
		widgets.DrawLine(c, 2, y+1+i, w-4, lines[t.peekTop+i])
	}
}

func (t *ThreadIndex) HandleKey(k term.Key) ui.Action {
	if v, submitted, handled := t.num.HandleKey(k); handled {
		if submitted {
			return t.openRow(v - 1)
		}
		return nil
	}
	if t.table.HandleKey(k) {
		t.peekTop = 0
		t.syncSelected()
		return nil
	}
	switch k.Code {
	case term.KeyEnter:
		return t.openRow(t.table.Cursor)
	case term.KeyTab:
		t.peek = !t.peek
		return nil
	}
	switch {
	case k.Code == term.KeyRune && k.Rune == ' ':
		step := t.peekH
		if step < 1 {
			step = 1
		}
		t.peekTop += step
	case Rune(k) == 'B':
		if t.model != nil {
			return ui.Push{Screen: newReader(t.d, t.model, "")}
		}
	case Rune(k) == 'S':
		t.sort = t.sort.Next()
		return t.fetch(false)
	case Rune(k) == 'O':
		t.openLink()
	case Rune(k) == 'R':
		return t.fetch(true)
	case Rune(k) == '-':
		t.fold(true)
	case Rune(k) == '+', Rune(k) == '=':
		t.fold(false)
	case IsBack(k):
		return ui.Pop{}
	}
	return nil
}

func (t *ThreadIndex) fold(collapse bool) {
	if t.model == nil || t.table.Cursor >= len(t.rows) {
		return
	}
	row := t.rows[t.table.Cursor]
	if row.Comment == nil || t.model.IsCollapsed(row.Comment.ID) == collapse {
		return
	}
	t.model.Toggle(row.Comment.ID)
	t.refresh()
}

func (t *ThreadIndex) openRow(i int) ui.Action {
	if t.model == nil || i < 0 || i >= len(t.rows) {
		if len(t.rows) > 0 {
			t.status, t.statusErr = "No such message", true
		}
		return nil
	}
	t.table.Select(i)
	t.peekTop = 0
	t.syncSelected()
	row := t.rows[i]
	if row.Comment != nil {
		return ui.Push{Screen: newReader(t.d, t.model, row.Comment.ID)}
	}
	if t.loading {
		return nil
	}
	t.loading = true
	t.gen++
	gen := t.gen
	t.status, t.statusErr = "Retrieving...", false
	store, sub, postID, link, sort := t.d.Store, t.post.Subreddit, t.post.ID, t.post.Fullname, t.sort
	stub := row.Stub
	if stub.IsContinue() {
		parentID := strings.TrimPrefix(stub.ParentFullname, "t1_")
		return ui.Run{Fn: func(ctx context.Context) ui.Msg {
			th, err := store.Subtree(ctx, sub, postID, parentID, sort)
			return subtreeMsg{gen: gen, id: parentID, th: th, err: err}
		}}
	}
	ids := append([]string(nil), stub.IDs...)
	return ui.Run{Fn: func(ctx context.Context) ui.Msg {
		things, err := store.MoreChildren(ctx, link, ids, sort)
		return moreMsg{gen: gen, stub: stub, things: things, err: err}
	}}
}

func (t *ThreadIndex) openLink() {
	u := t.post.URL
	if t.post.IsSelf || u == "" {
		u = "https://www.reddit.com" + t.post.Permalink
	}
	if err := t.d.Open(u, nil); err != nil {
		t.status, t.statusErr = "Could not open browser. URL: "+u, true
		return
	}
	t.d.Session.LinksOpened++
	t.status, t.statusErr = "Opened in browser", false
}
```

In `postlist.go`, point the indirection at the real constructor:

```go
var newThreadIndex = func(d *Deps, p *reddit.Post) ui.Screen { return NewThreadIndex(d, p) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/screens/`
Expected: PASS. If `TestThreadIndexLoadsAndDraws` fails on the highlighted row, recount: title bar 3 rows, header 2, rule 1, table heading 1 gives the first data row at terminal row 7.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screens
git commit -m "Add thread index screen with tree, peek pane and stub loading

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 21: Message Reader screen

**Files:**
- Create: `internal/ui/screens/reader.go`
- Modify: `internal/ui/screens/threadindex.go` (`newReader` now calls `NewReader`)
- Test: `internal/ui/screens/reader_test.go`

**Interfaces:**
- Consumes: `threadmodel.Model` (`Comments`, `Parent`, `Post`, `Thread`), `textfmt.Render`, `widgets.TextBox`, `widgets.NumInput`.
- Produces: `NewReader(d *Deps, m *threadmodel.Model, commentID string) *Reader`. Pops with `SelectComment{ID}` for a comment, or `nil` result from message 0.

- [ ] **Step 1: Write the failing tests**

`internal/ui/screens/reader_test.go`:

```go
package screens

import (
	"strings"
	"testing"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/threadmodel"
)

func readerAt(t *testing.T, id string) (*ui.App, *term.Sim, *Deps, *Reader) {
	t.Helper()
	d, _ := newDeps(t)
	m := threadmodel.New(redditest.SampleThread(), 10)
	r := NewReader(d, m, id)
	app, sim := run(t, r)
	return app, sim, d, r
}

func TestReaderShowsComment(t *testing.T) {
	_, sim, d, _ := readerAt(t, "c1")
	mustContain(t, sim,
		"Subj: Kernel 7.2 released", "From: sched_nerd", "(+412)", "Date: 26/09/26",
		"Re: original post", "depth 0", "1 loaded replies",
		"The EEVDF changes are the headline.", "Lazy preemption is the real win.",
		"Msg 1 of 4", "Read Message",
	)
	if d.Session.MessagesRead != 1 {
		t.Errorf("MessagesRead = %d", d.Session.MessagesRead)
	}
}

func TestReaderNextPrevAndBounds(t *testing.T) {
	app, sim, d, _ := readerAt(t, "c1")
	press(app, term.R('n'))
	mustContain(t, sim, "Agreed.", "Re: #1 sched_nerd", "depth 1", "Msg 2 of 4")
	press(app, term.R('n'))
	mustContain(t, sim, "Body survives the account.", "From: [deleted]")
	press(app, term.R('n'))
	mustContain(t, sim, "[removed]", "From: modbot")
	press(app, term.R('n'))
	mustContain(t, sim, "No more messages", "Msg 4 of 4")
	press(app, term.R('p'), term.R('p'), term.R('p'), term.R('p'))
	mustContain(t, sim, "Msg 0 of 4", "From: torvaldsfan", "https://example.com/aaa")
	press(app, term.R('p'))
	mustContain(t, sim, "No more messages")
	if d.Session.MessagesRead != 8 {
		t.Errorf("MessagesRead = %d, want 8 (1 initial + 7 moves)", d.Session.MessagesRead)
	}
}

func TestReaderUp(t *testing.T) {
	app, sim, _, _ := readerAt(t, "c2")
	press(app, term.R('u'))
	mustContain(t, sim, "Msg 1 of 4", "From: sched_nerd")
	press(app, term.R('u'))
	mustContain(t, sim, "Msg 0 of 4")
	press(app, term.R('u'))
	mustContain(t, sim, "Already at top")
}

func TestReaderTAndQReturnSelection(t *testing.T) {
	_, _, _, r := readerAt(t, "c2")
	if act := r.HandleKey(term.R('t')); act != (ui.Pop{Result: SelectComment{ID: "c2"}}) {
		t.Errorf("T action = %#v", act)
	}
	if act := r.HandleKey(term.K(term.KeyEscape)); act != (ui.Pop{Result: SelectComment{ID: "c2"}}) {
		t.Errorf("Esc action = %#v", act)
	}
	_, _, _, r0 := readerAt(t, "")
	if act := r0.HandleKey(term.R('q')); act != (ui.Pop{}) {
		t.Errorf("Q on message 0 = %#v", act)
	}
}

func TestReaderRepliesJump(t *testing.T) {
	app, sim, _, _ := readerAt(t, "c1")
	press(app, term.R('r'))
	mustContain(t, sim, "Replies to this message", "torvaldsfan", "Reply #:")
	press(app, term.R('?')) // literal during numeric entry mode: must not open help
	if app.Depth() != 1 {
		t.Fatal("help opened during reply selection")
	}
	press(app, term.R('1'), term.K(term.KeyEnter))
	mustContain(t, sim, "Agreed.", "Msg 2 of 4")
	mustNotContain(t, sim, "Reply #:")
}

func TestReaderRepliesFromPostListsTopLevel(t *testing.T) {
	app, sim, _, _ := readerAt(t, "")
	press(app, term.R('r'))
	mustContain(t, sim, "1. sched_nerd", "2. [deleted]", "3. modbot")
	press(app, term.K(term.KeyEscape))
	mustNotContain(t, sim, "Replies to this message")
	if app.Depth() != 1 {
		t.Error("Escape must only cancel the reply list")
	}
}

func TestReaderLinks(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	th.Comments[0].Body = "see [one](https://one.example) and [two](https://two.example/x)"
	th.Comments[1].Body = "just https://bare.example"
	m := threadmodel.New(th, 10)
	var opened []string
	d.Open = func(u string, _ func(error)) error { opened = append(opened, u); return nil }
	app, sim := run(t, NewReader(d, m, "c1"))
	mustContain(t, sim, "one[1]", "two[2]", "Links:", "[1] https://one.example", "[2] https://two.example/x")
	press(app, term.R('o'))
	mustContain(t, sim, "Link #:")
	press(app, term.R('2'), term.K(term.KeyEnter))
	if len(opened) != 1 || opened[0] != "https://two.example/x" {
		t.Errorf("opened = %v", opened)
	}
	press(app, term.R('n'), term.R('o'))
	if len(opened) != 2 || opened[1] != "https://bare.example" {
		t.Errorf("single link should open directly: %v", opened)
	}
	press(app, term.R('n'), term.R('o'))
	mustContain(t, sim, "No links in this message")
	if d.Session.LinksOpened != 2 {
		t.Errorf("LinksOpened = %d", d.Session.LinksOpened)
	}
}

func TestReaderPaging(t *testing.T) {
	d, _ := newDeps(t)
	th := redditest.SampleThread()
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		b.WriteString("line ")
		b.WriteString(itoa(i))
		b.WriteString("\n\n")
	}
	th.Comments[0].Body = b.String()
	app, sim := run(t, NewReader(d, threadmodel.New(th, 10), "c1"))
	mustContain(t, sim, "line 1", "more")
	mustNotContain(t, sim, "line 40")
	press(app, term.R(' '))
	mustNotContain(t, sim, "line 1\n")
	press(app, term.K(term.KeyEnd))
	mustContain(t, sim, "line 60")
	mustNotContain(t, sim, "▼ more")
	press(app, term.K(term.KeyHome))
	mustContain(t, sim, "line 1")
}

func TestReaderIgnoresPostMoreStubForReplies(t *testing.T) {
	d, _ := newDeps(t)
	th := reddit.Thread{Post: redditest.SamplePost("z", "Empty"), More: &reddit.MoreStub{ParentFullname: "t3_z", Count: 3, IDs: []string{"a"}}}
	app, sim := run(t, NewReader(d, threadmodel.New(th, 10), ""))
	mustContain(t, sim, "Msg 0 of 0")
	press(app, term.R('n'))
	mustContain(t, sim, "No more messages")
	press(app, term.R('r'))
	mustContain(t, sim, "No loaded replies")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/screens/ -run Reader`
Expected: FAIL, undefined: NewReader.

- [ ] **Step 3: Implement reader.go**

```go
package screens

import (
	"fmt"

	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/textfmt"
	"github.com/markwatson/redditbbs/internal/theme"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/threadmodel"
	"github.com/markwatson/redditbbs/internal/ui/widgets"
)

// Reader shows one message (the post as message 0, or a comment) full width.
type Reader struct {
	d     *Deps
	model *threadmodel.Model
	pos   int // 0 = post, n = nth visible comment

	doc    textfmt.Doc
	box    widgets.TextBox
	docW   int
	docPos int
	bodyH  int

	replies   bool
	replyList []*reddit.Comment
	linkAsk   bool
	num       widgets.NumInput
	status    string
	statusErr bool
}

// NewReader opens the reader on commentID, or on the post when it is empty.
func NewReader(d *Deps, m *threadmodel.Model, commentID string) *Reader {
	r := &Reader{d: d, model: m, docPos: -1}
	if commentID != "" {
		for i, c := range m.Comments() {
			if c.ID == commentID {
				r.pos = i + 1
				break
			}
		}
	}
	return r
}

func (r *Reader) Init() ui.Action {
	r.d.Session.MessagesRead++
	return nil
}

func (r *Reader) Title() string { return "Read Message" }
func (r *Reader) Info() string  { return fmt.Sprintf("Msg %d of %d", r.pos, len(r.model.Comments())) }
func (r *Reader) Keys() []ui.KeyHelp {
	return []ui.KeyHelp{{"N", "ext"}, {"P", "rev"}, {"U", "p"}, {"R", "eplies"}, {"T", "hread"}, {"O", "pen link"}, {"Spc", "Page"}, {"Q", "uit"}}
}
func (r *Reader) CapturesKeys() bool { return r.replies || r.linkAsk }

func (r *Reader) Prompt() widgets.Prompt {
	switch {
	case r.replies:
		return widgets.Prompt{Label: "Reply #:", Input: r.num.Digits, Cursor: true}
	case r.linkAsk:
		return widgets.Prompt{Label: "Link #:", Input: r.num.Digits, Cursor: true}
	}
	return widgets.Prompt{Status: r.status, Error: r.statusErr}
}

func (r *Reader) Update(ui.Msg) ui.Action { return nil }

// current returns the comment shown, or nil for message 0.
func (r *Reader) current() *reddit.Comment {
	if r.pos == 0 {
		return nil
	}
	cs := r.model.Comments()
	if r.pos-1 >= len(cs) {
		r.pos = len(cs)
		if r.pos == 0 {
			return nil
		}
	}
	return cs[r.pos-1]
}

func (r *Reader) bodyText() string {
	cm := r.current()
	if cm == nil {
		p := r.model.Post()
		if p.IsSelf {
			return p.SelfText
		}
		return "Link post: " + p.URL
	}
	return cm.Body
}

func (r *Reader) ensureDoc(w int) {
	if r.docW == w && r.docPos == r.pos {
		return
	}
	r.doc = textfmt.Render(r.bodyText(), w)
	lines := append([]textfmt.Line(nil), r.doc.Lines...)
	if len(r.doc.Links) > 0 {
		lines = append(lines, textfmt.Line{}, textfmt.Line{{Text: "Links:", Kind: textfmt.Bold}})
		for i, u := range r.doc.Links {
			lines = append(lines, textfmt.Line{{Text: "[" + itoa(i+1) + "] " + u, Kind: textfmt.Link}})
		}
	}
	r.box.Lines = lines
	r.box.Top = 0
	r.docW, r.docPos = w, r.pos
}

func (r *Reader) Draw(c term.Canvas) {
	w, h := c.Size()
	r.ensureDoc(w - 4)
	p := r.model.Post()
	cm := r.current()
	hd := theme.Style(theme.Heading)

	c.Text(2, 0, "Subj: ", hd, w)
	c.Text(8, 0, textfmt.Truncate(p.Title, w-10), theme.Style(theme.Subject), w-10)

	author, role, score, created := p.Author, theme.OP, p.Score, p.Created
	if cm != nil {
		author, role = authorAndRole(cm, p)
		score, created = cm.Score, cm.Created
	}
	x := 2
	x += c.Text(x, 1, "From: ", hd, w)
	x += c.Text(x, 1, author, theme.Style(role), w)
	c.Text(x, 1, " (+"+textfmt.Score(score)+")", theme.Style(theme.Meta), w-x)
	date := "Date: " + created.Local().Format("02/01/06 15:04")
	c.Text(w-2-textfmt.Width(date), 1, date, theme.Style(theme.Meta), w)

	y := 2
	if cm != nil {
		re := "  Re: original post"
		if parent := r.model.Parent(cm); parent != nil {
			re = "  Re: #" + itoa(r.model.IndexOf(parent.ID)+1) + " " + parent.Author
		}
		re += fmt.Sprintf(" · depth %d · %d loaded replies", cm.Depth, len(cm.Children))
		c.Text(2, y, textfmt.Truncate(re, w-4), theme.Style(theme.Meta), w-4)
		y++
	}
	widgets.Rule(c, y)
	y++
	r.bodyH = h - y
	if r.replies {
		r.drawReplies(c, y, w)
		return
	}
	r.box.Draw(c, 2, y, w-4, r.bodyH)
	if !r.box.AtEnd(r.bodyH) {
		c.Text(w-10, h-1, "▼ more", theme.Style(theme.Meta), 8)
	}
}

func (r *Reader) drawReplies(c term.Canvas, y, w int) {
	c.Text(2, y, "Replies to this message:", theme.Style(theme.Heading), w-4)
	if len(r.replyList) == 0 {
		c.Text(4, y+2, "No loaded replies", theme.Style(theme.Meta), w-6)
		return
	}
	for i, cm := range r.replyList {
		if y+2+i >= y+r.bodyH {
			break
		}
		author, role := authorAndRole(cm, r.model.Post())
		x := 4
		x += c.Text(x, y+2+i, itoa(i+1)+". ", theme.Style(theme.Meta), w)
		x += c.Text(x, y+2+i, author, theme.Style(role), w)
		preview := textfmt.FirstLine(cm.Body)
		if cm.BodyRemoved {
			preview = cm.Body
		}
		c.Text(x, y+2+i, " — "+textfmt.Truncate(preview, w-x-5), theme.Style(theme.Body), w-x-2)
	}
}

func (r *Reader) HandleKey(k term.Key) ui.Action {
	if r.replies || r.linkAsk {
		if v, submitted, handled := r.num.HandleKey(k); handled {
			if submitted {
				if r.replies {
					r.jumpToReply(v)
				} else {
					r.openLink(v)
				}
				r.replies, r.linkAsk = false, false
			}
			return nil
		}
		if k.Code == term.KeyEscape || Rune(k) == 'R' || Rune(k) == 'O' {
			r.replies, r.linkAsk = false, false
			r.num.Digits = ""
		}
		return nil
	}
	switch k.Code {
	case term.KeyUp:
		r.box.Scroll(-1, r.bodyH)
	case term.KeyDown:
		r.box.Scroll(1, r.bodyH)
	case term.KeyPgUp:
		r.box.Scroll(-r.bodyH, r.bodyH)
	case term.KeyPgDn:
		r.box.Scroll(r.bodyH, r.bodyH)
	case term.KeyHome:
		r.box.Top = 0
	case term.KeyEnd:
		r.box.Scroll(len(r.box.Lines), r.bodyH)
	}
	cs := r.model.Comments()
	switch {
	case k.Code == term.KeyRune && k.Rune == ' ':
		r.box.Scroll(r.bodyH, r.bodyH)
	case Rune(k) == 'N':
		if r.pos < len(cs) {
			r.setPos(r.pos + 1)
		} else {
			r.status, r.statusErr = "No more messages", false
		}
	case Rune(k) == 'P':
		if r.pos > 0 {
			r.setPos(r.pos - 1)
		} else {
			r.status, r.statusErr = "No more messages", false
		}
	case Rune(k) == 'U':
		cm := r.current()
		if cm == nil {
			r.status, r.statusErr = "Already at top", false
			break
		}
		parent := r.model.Parent(cm)
		if parent == nil {
			r.setPos(0)
			break
		}
		for i, c := range cs {
			if c.ID == parent.ID {
				r.setPos(i + 1)
				break
			}
		}
	case Rune(k) == 'R':
		r.replyList = r.visibleReplies()
		r.replies = true
		r.num.Digits = ""
	case Rune(k) == 'O':
		switch len(r.doc.Links) {
		case 0:
			r.status, r.statusErr = "No links in this message", false
		case 1:
			r.openLink(1)
		default:
			r.linkAsk = true
			r.num.Digits = ""
		}
	case Rune(k) == 'T', IsBack(k):
		if cm := r.current(); cm != nil {
			return ui.Pop{Result: SelectComment{ID: cm.ID}}
		}
		return ui.Pop{}
	}
	return nil
}

func (r *Reader) setPos(p int) {
	r.pos = p
	r.status = ""
	r.d.Session.MessagesRead++
	r.docPos = -1
	if r.docW > 0 {
		r.ensureDoc(r.docW) // rebuild now so O sees the new message's links before the next Draw
	}
}

// visibleReplies lists the current message's direct replies that are visible
// in the thread index (loaded and not hidden by a collapse).
func (r *Reader) visibleReplies() []*reddit.Comment {
	visible := map[string]bool{}
	for _, c := range r.model.Comments() {
		visible[c.ID] = true
	}
	var candidates []*reddit.Comment
	if cm := r.current(); cm != nil {
		candidates = cm.Children
	} else {
		candidates = r.model.Thread().Comments
	}
	var out []*reddit.Comment
	for _, c := range candidates {
		if visible[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func (r *Reader) jumpToReply(n int) {
	if n < 1 || n > len(r.replyList) {
		r.status, r.statusErr = "No such reply", true
		return
	}
	target := r.replyList[n-1]
	for i, c := range r.model.Comments() {
		if c.ID == target.ID {
			r.setPos(i + 1)
			return
		}
	}
}

func (r *Reader) openLink(n int) {
	if n < 1 || n > len(r.doc.Links) {
		r.status, r.statusErr = "No such link", true
		return
	}
	u := r.doc.Links[n-1]
	if err := r.d.Open(u, nil); err != nil {
		r.status, r.statusErr = "Could not open browser. URL: "+u, true
		return
	}
	r.d.Session.LinksOpened++
	r.status, r.statusErr = "Opened in browser", false
}
```

In `threadindex.go`, point the indirection at the real constructor:

```go
var newReader = func(d *Deps, m *threadmodel.Model, commentID string) ui.Screen { return NewReader(d, m, commentID) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/screens/`
Expected: PASS. `TestReaderShowsComment` expects `Date: 26/09/26`; the fixture times are on 26/09/2026 UTC and the test machine is in the UK, so local and UTC dates agree. If this runs elsewhere, set `TZ=UTC` for the test run.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screens
git commit -m "Add message reader screen with thread traversal and links

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 22: main wiring, bad-response sink, smoke test and README

**Files:**
- Modify: `cmd/redditbbs/main.go`, `internal/reddit/client.go` (add `WithBadResponseSink`), `README.md`
- Test: `cmd/redditbbs/main_test.go`, `internal/ui/screens/smoke_test.go`, add `TestBadResponseSink` to `internal/reddit/client_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `reddit.WithBadResponseSink(func([]byte)) Option`; `main.run(args []string, stdout, stderr io.Writer) int`.

- [ ] **Step 1: Write the failing client sink test**

Append to `internal/reddit/client_test.go`:

```go
func TestBadResponseSink(t *testing.T) {
	srv := &apiServer{t: t}
	srv.handler = func(w http.ResponseWriter, r *http.Request, n int) bool {
		fmt.Fprint(w, `{"kind":"Listing","data":{"children":[{"kind":"t3","data":"not an object"}]}}`)
		return true
	}
	hs := httptest.NewServer(srv)
	defer hs.Close()
	var got []byte
	c := NewClient(Credentials{ClientID: "id", ClientSecret: "sec", UserAgent: "ua"},
		WithBaseURL(hs.URL), WithTokenURL(hs.URL+"/api/v1/access_token"), WithBadResponseSink(func(b []byte) { got = b }))
	if _, err := c.Posts(context.Background(), "linux", Hot, "", Fetch{}); err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(string(got), "not an object") {
		t.Errorf("sink did not receive the body: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reddit/ -run BadResponseSink`
Expected: FAIL, undefined: WithBadResponseSink.

- [ ] **Step 3: Add the sink to client.go**

Add a field and option:

```go
	sink   func([]byte) // receives bodies that failed to parse
```

```go
// WithBadResponseSink receives the body of any 200 response that fails to parse.
func WithBadResponseSink(f func([]byte)) Option { return func(c *Client) { c.sink = f } }
```

Add a helper and use it in the four Store methods wherever a parse error is returned:

```go
// parsed reports a parse failure to the sink and returns the error unchanged.
func (c *Client) parsed(b []byte, err error) error {
	if err != nil && c.sink != nil {
		if len(b) > 64<<10 {
			b = b[:64<<10]
		}
		c.sink(b)
	}
	return err
}
```

For example in `Posts`:

```go
	l, err := ParseListing(bytes.NewReader(b))
	return l, c.parsed(b, err)
```

Apply the same shape to `Thread`, `Subtree` and the per-batch parse in `MoreChildren`.

- [ ] **Step 4: Run client tests**

Run: `go test ./internal/reddit/`
Expected: PASS.

- [ ] **Step 5: Write the failing smoke test**

`internal/ui/screens/smoke_test.go`:

```go
package screens

import (
	"testing"

	"github.com/markwatson/redditbbs/internal/reddit/redditest"
	"github.com/markwatson/redditbbs/internal/term"
)

// TestSmokeWalkthrough drives every screen in order through the App loop.
func TestSmokeWalkthrough(t *testing.T) {
	d, fs := newDeps(t)
	fs.Listings["linux/hot/"] = redditest.SampleListing(3, "")
	th := redditest.SampleThread()
	fs.Threads["p1"] = th

	app, sim := run(t, NewSplash(d))
	mustContain(t, sim, "Press any key to log on")

	press(app, term.R(' '))
	mustContain(t, sim, "[M] Message areas")

	press(app, term.R('m'))
	mustContain(t, sim, "r/linux", "r/rust")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "Post 1", "Post 3", "r/linux · HOT")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	mustContain(t, sim, "sched_nerd", "[load 3 more replies]", "Thread Index")

	press(app, term.K(term.KeyEnter))
	mustContain(t, sim, "Read Message", "The EEVDF changes are the headline.")

	press(app, term.R('n'))
	mustContain(t, sim, "Agreed.", "Msg 2 of 4")

	press(app, term.R('t'))
	ti, ok := app.Top().(*ThreadIndex)
	if !ok {
		t.Fatalf("expected ThreadIndex after T, got %T", app.Top())
	}
	if row := ti.rows[ti.table.Cursor]; row.Comment == nil || row.Comment.ID != "c2" {
		t.Errorf("thread index should reselect c2, got %+v", row)
	}

	press(app, term.R('?'))
	mustContain(t, sim, "Keys for Thread Index", "Ctrl-C")
	press(app, term.R(' '))

	press(app, term.R('q'))
	if _, ok := app.Top().(*PostList); !ok {
		t.Fatalf("expected PostList, got %T", app.Top())
	}
	press(app, term.R('q'))
	if _, ok := app.Top().(*AreaList); !ok {
		t.Fatalf("expected AreaList, got %T", app.Top())
	}
	press(app, term.R('q'))
	if _, ok := app.Top().(*MainMenu); !ok {
		t.Fatalf("expected MainMenu, got %T", app.Top())
	}
	press(app, term.R('g'), term.R('y'))
	mustContain(t, sim, "Thanks for calling", "Messages read ..... 2", "Threads opened .... 1", "Areas visited ..... 1")

	press(app, term.R(' '))
	if !app.Quitting() {
		t.Error("goodbye should quit on a key")
	}
}
```

- [ ] **Step 6: Run the smoke test**

Run: `go test ./internal/ui/screens/ -run Smoke -v`
Expected: PASS (all screens exist by now). If it fails, the failure is a real integration bug between screens: fix the screen, not the test.

- [ ] **Step 7: Write the failing main test**

`cmd/redditbbs/main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("code = %d, stderr %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "redditbbs ") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--nonsense"}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
}

func TestRunBadConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.toml"
	if err := writeFile(path, "[reddit\nbroken"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "config") {
		t.Errorf("stderr = %q", errb.String())
	}
}
```

Add `writeFile` to the test file:

```go
func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }
```

with `"os"` imported.

- [ ] **Step 8: Run to verify failure**

Run: `go test ./cmd/redditbbs/`
Expected: FAIL, undefined: run.

- [ ] **Step 9: Implement main.go**

```go
// Command redditbbs is a BBS-style terminal reader for Reddit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	rdebug "runtime/debug"
	"time"

	"github.com/markwatson/redditbbs/internal/browser"
	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/session"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/screens"
)

// Version is set at build time via -ldflags "-X main.Version=…".
var Version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run parses flags, loads config and runs the UI. Exit codes: 0 ok, 1 runtime
// failure, 2 usage or configuration error.
func run(args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet("redditbbs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "", "path to config.toml")
	debugFlag := fs.Bool("debug", false, "save the last unparseable Reddit response to the state directory")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "redditbbs", Version)
		return 0
	}

	path := *cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintln(stderr, "redditbbs: config:", err)
			return 2
		}
		path = p
	}
	cfg, err := config.Load(path, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs: config:", err)
		return 2
	}

	t, err := term.NewTcell()
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs: terminal:", err)
		return 1
	}
	defer func() {
		if p := recover(); p != nil {
			t.Fini()
			fmt.Fprintf(stderr, "redditbbs: panic: %v\n%s", p, rdebug.Stack())
			code = 1
		}
	}()

	var app *ui.App
	deps := &screens.Deps{
		Config:  cfg,
		Session: session.New(time.Now()),
		Open:    browser.Open,
		Now:     time.Now,
		Version: Version,
	}
	deps.MakeStore = func(id, secret string) reddit.Store {
		opts := []reddit.Option{reddit.WithOnWait(func(d time.Duration) {
			if app != nil {
				app.Post(ui.RateLimited{Wait: d})
			}
		})}
		if *debugFlag {
			opts = append(opts, reddit.WithBadResponseSink(saveBadResponse))
		}
		return reddit.NewClient(reddit.Credentials{ClientID: id, ClientSecret: secret, UserAgent: cfg.Reddit.UserAgent}, opts...)
	}
	if cfg.HasCredentials() {
		deps.Store = deps.MakeStore(cfg.ClientID(), cfg.ClientSecret())
	}

	app = ui.New(t, screens.NewSplash(deps))
	err = app.Run()
	t.Fini()
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs:", err)
		return 1
	}
	return 0
}

// saveBadResponse writes body to $XDG_STATE_HOME/redditbbs/last-error.json.
func saveBadResponse(body []byte) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "redditbbs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "last-error.json"), body, 0o600)
}
```

- [ ] **Step 10: Run everything**

Run: `make test vet && make build && ./bin/redditbbs --version`
Expected: all packages PASS, vet clean, version printed.

- [ ] **Step 11: Write the README**

Replace `README.md`:

```markdown
# RedditBBS

A terminal Reddit reader that looks and feels like a 1990s bulletin board
system: hotkey menus, a command prompt, numbered messages, a threaded
comment index with a peek pane, and a one-message reader. Read-only.

## Setup

1. Build: `make build` (needs Go 1.27). The binary is `bin/redditbbs`.
2. Register a free Reddit "script" app at https://www.reddit.com/prefs/apps
   (type *script*, any redirect URI). Reddit may need to approve the app
   before API requests succeed.
3. Run `./bin/redditbbs`. The New User Setup screen asks for the client ID
   and secret, checks them against Reddit, and saves them to
   `~/.config/redditbbs/config.toml` with mode 0600.

`REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET` override the file and
are never written to disk. `--config PATH` uses another file. `--debug`
saves the last unparseable API response to
`~/.local/state/redditbbs/last-error.json`.

## Keys

Everywhere: `?` help, `Q` or `Esc` back, arrows and PgUp/PgDn move the
cursor, type a number and `Enter` to select a row, `Ctrl-L` redraw,
`Ctrl-C` quit.

| Screen | Keys |
| --- | --- |
| Main Menu | `M` message areas, `J` join any subreddit, `G` goodbye |
| Area List | `Enter` open, `D` delete |
| Post List | `Enter` read, `N`/`P` page, `S` sort, `J` join, `A` add area, `O` open link, `R` refresh |
| Thread Index | `Enter` read, `B` post body, `-`/`+` fold, `Tab` peek pane, `Space` scroll peek, `S` sort, `O` open link, `R` refresh |
| Message Reader | `N`/`P` next/previous, `U` parent, `R` replies, `T` back to index, `O` open link, `Space` page |

## Configuration

```toml
[reddit]
client_id = "…"
client_secret = "…"
user_agent = "linux:redditbbs:0.1.0 (by /u/yourname)"

[display]
peek_pane = true
default_sort = "hot"

[[areas]]
name = "Linux"
subreddit = "linux"
```

## Development

`make test` runs the tests, `make vet` runs `go vet`. The design spec is in
`docs/superpowers/specs/` and the implementation plan in
`docs/superpowers/plans/`.
```

- [ ] **Step 12: Commit**

```bash
git add cmd internal/reddit README.md internal/ui/screens/smoke_test.go
git commit -m "Wire up the binary, add smoke test and README

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Self-review notes

- Every spec section maps to a task: purpose and decisions (header), architecture and Canvas (Task 4), App loop (Task 14), Store (Tasks 6 and 10), screens (Tasks 15 to 21), text formatting (Task 3), configuration (Task 11), data layer (Tasks 6 to 10), error handling (spread across Tasks 10, 14 and the screens via `errText`), testing (each task), repository layout (Task 1 and Task 22).
- Deferred constructors (`newPostList`, `newThreadIndex`, `newReader`, the temporary `NewSetup`) exist only so each task compiles and tests alone; each later task replaces its indirection and the smoke test in Task 22 proves the real chain.
- The rate-limit countdown is delivered by `App.Post` from the client's wait callback, which runs on a worker goroutine; `Post` must never be called from the loop goroutine.
