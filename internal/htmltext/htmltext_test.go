package htmltext

import "testing"

func TestToMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"paragraphs and inline", "<p>Hi <code>x</code> and <strong>bold</strong>.</p><ul><li>a</li><li>b</li></ul><p>end</p>", "Hi `x` and **bold**.\n\n- a\n- b\n\nend"},
		{"ordered list", "<ol><li>one</li><li>two</li></ol>", "1. one\n2. two"},
		{"blockquote", "<blockquote><p>quoted line</p></blockquote><p>reply</p>", "> quoted line\n\nreply"},
		{"pre keeps layout", "<p>see</p><pre><code>for i\n  go(i)\n</code></pre>", "see\n\n```\nfor i\n  go(i)\n```"},
		{"link with text", `<p>read <a href="https://x.example/a">the docs</a> now</p>`, "read [the docs](https://x.example/a) now"},
		{"bare link", `<p><a href="https://x.example">https://x.example</a></p>`, "https://x.example"},
		{"relative link kept", `<p><a href="/r/linux">/r/linux</a></p>`, "/r/linux"},
		{"entities", "<p>I&#39;m &amp; you &lt;3</p>", "I'm & you <3"},
		{"br", "<p>a<br/>b</p>", "a\nb"},
		{"whitespace collapsed", "<p>  lots   of\n space </p>", "lots of space"},
		{"nested emphasis", "<p><em>it</em> <strong><em>both</em></strong></p>", "*it* ***both***"},
		{"heading and rule", "<h2>Title</h2><hr/><p>text</p>", "Title\n\ntext"},
		{"table flattened", "<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>2</td></tr></table>", "a | b\n1 | 2"},
		{"plain text passthrough", "just text", "just text"},
		{"reddit wrapper", `<!-- SC_OFF --><div class="md"><p>body</p></div><!-- SC_ON -->`, "body"},
	}
	for _, c := range cases {
		if got := ToMarkdown(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}
