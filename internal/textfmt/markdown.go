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
	// CSI sequences, OSC sequences (terminated by BEL or ST), and any other
	// two-byte escape.
	ansiRe    = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b.`)
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
		if r == '\n' || (r >= 0x20 && r != 0x7f && !(r >= 0x80 && r <= 0x9f)) {
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
		inner := ""
		if m[2] >= 0 {
			inner = s[m[2]:m[3]]
		} else {
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
// Widths below 4 are treated as 4 so prefixes and wide clusters always fit.
func Render(md string, w int) Doc {
	if w < 4 {
		w = 4
	}
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
