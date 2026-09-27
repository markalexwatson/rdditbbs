// Package htmltext converts the HTML bodies found in Reddit's RSS feeds into
// the Markdown-like plain text that textfmt renders.
package htmltext

import (
	"strings"

	"golang.org/x/net/html"
)

// Limits on hostile input: quote nesting and total output.
const (
	maxQuoteDepth   = 8
	maxOutput       = 256 << 10
	truncatedMarker = "\n\n[text truncated]"
)

type conv struct {
	out         strings.Builder
	pre         int
	quote       int
	listStack   []int // 0 for unordered lists, else the next ordinal
	atLineStart bool
	truncated   bool
}

// ToMarkdown renders HTML as Markdown-ish text: paragraphs separated by blank
// lines, "> " quotes, fenced code, "- " and "1. " lists, [text](url) links,
// **bold** and *italic*. Unknown tags are flattened to their text.
func ToMarkdown(src string) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return strings.TrimSpace(src)
	}
	c := &conv{atLineStart: true}
	c.walk(doc)
	out := tidy(c.out.String())
	if c.truncated {
		out += truncatedMarker
	}
	return out
}

func (c *conv) write(s string) {
	if s == "" || c.truncated {
		return
	}
	if room := maxOutput - c.out.Len(); room <= 0 {
		c.truncated = true
		return
	} else if len(s) > room {
		s, c.truncated = s[:room], true // cut a single oversized run too
	}
	if c.atLineStart {
		depth := c.quote
		if depth > maxQuoteDepth {
			depth = maxQuoteDepth
		}
		c.out.WriteString(strings.Repeat("> ", depth))
		c.atLineStart = false
	}
	c.out.WriteString(s)
}

func (c *conv) newline() {
	c.out.WriteString("\n")
	c.atLineStart = true
}

// blockBreak ends the current line and leaves one blank line.
func (c *conv) blockBreak() {
	if !c.atLineStart {
		c.newline()
	}
	c.newline()
}

func (c *conv) text(s string) {
	if c.pre > 0 {
		for i, line := range strings.Split(s, "\n") {
			if i > 0 {
				c.newline()
			}
			c.write(line)
		}
		return
	}
	collapsed := strings.Join(strings.Fields(s), " ")
	if collapsed == "" {
		if strings.ContainsAny(s, " \n\t") && !c.atLineStart {
			c.write(" ")
		}
		return
	}
	if strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\n") || strings.HasPrefix(s, "\t") {
		if !c.atLineStart {
			c.write(" ")
		}
	}
	c.write(collapsed)
	if strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\t") {
		c.write(" ")
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			rec(k)
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func (c *conv) children(n *html.Node) {
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		c.walk(k)
	}
}

func (c *conv) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		c.text(n.Data)
		return
	case html.CommentNode:
		return
	case html.ElementNode:
	default:
		c.children(n)
		return
	}
	switch n.Data {
	case "script", "style", "head":
		return
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
		c.blockBreak()
		c.children(n)
		c.blockBreak()
	case "br":
		c.newline()
	case "hr":
		c.blockBreak()
	case "blockquote":
		c.blockBreak()
		c.quote++
		c.children(n)
		c.quote--
		c.blockBreak()
	case "pre":
		c.blockBreak()
		c.write("```")
		c.newline()
		c.pre++
		c.children(n)
		c.pre--
		if !c.atLineStart {
			c.newline()
		}
		c.write("```")
		c.blockBreak()
	case "code":
		if c.pre > 0 {
			c.children(n)
		} else {
			c.write("`" + textOf(n) + "`")
		}
	case "strong", "b":
		c.write("**")
		c.children(n)
		c.write("**")
	case "em", "i":
		c.write("*")
		c.children(n)
		c.write("*")
	case "a":
		href := attr(n, "href")
		text := textOf(n)
		switch {
		case href == "" || text == href || text == "":
			c.write(text)
		default:
			c.write("[" + text + "](" + href + ")")
		}
	case "ul", "ol":
		c.blockBreak()
		start := 0
		if n.Data == "ol" {
			start = 1
		}
		c.listStack = append(c.listStack, start)
		c.children(n)
		c.listStack = c.listStack[:len(c.listStack)-1]
		c.blockBreak()
	case "li":
		if !c.atLineStart {
			c.newline()
		}
		marker := "- "
		if len(c.listStack) > 0 && c.listStack[len(c.listStack)-1] > 0 {
			i := len(c.listStack) - 1
			marker = itoa(c.listStack[i]) + ". "
			c.listStack[i]++
		}
		c.write(marker)
		c.children(n)
		if !c.atLineStart {
			c.newline()
		}
	case "tr":
		if !c.atLineStart {
			c.newline()
		}
		first := true
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			if k.Type == html.ElementNode && (k.Data == "td" || k.Data == "th") {
				if !first {
					c.write(" | ")
				}
				first = false
				c.write(textOf(k))
			}
		}
		c.newline()
	case "table", "thead", "tbody":
		c.blockBreak()
		c.children(n)
		c.blockBreak()
	default:
		c.children(n)
	}
}

func itoa(n int) string {
	var b []byte
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// tidy trims trailing spaces and collapses runs of blank lines outside
// fenced code, then trims the ends. Lines inside fences are kept verbatim.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	blank := 0
	inFence := false
	for _, l := range lines {
		bare := strings.TrimLeft(l, "> ")
		if strings.HasPrefix(bare, "```") {
			inFence = !inFence
			blank = 0
			out = append(out, l)
			continue
		}
		if inFence {
			out = append(out, l)
			continue
		}
		l = strings.TrimRight(l, " ")
		if strings.TrimSpace(strings.TrimLeft(l, "> ")) == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, l)
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}
