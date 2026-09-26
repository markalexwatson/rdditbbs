package screens

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markalexwatson/redditbbs/internal/config"
	"github.com/markalexwatson/redditbbs/internal/reddit"
	"github.com/markalexwatson/redditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/redditbbs/internal/term"
	"github.com/markalexwatson/redditbbs/internal/textfmt"
	"github.com/markalexwatson/redditbbs/internal/ui"
)

// TestRenderMockups writes every screen as an HTML fragment when
// REDDITBBS_MOCKUP_DIR is set, for screenshots in the README or a share page.
// It is skipped otherwise.
func TestRenderMockups(t *testing.T) {
	dir := os.Getenv("REDDITBBS_MOCKUP_DIR")
	if dir == "" {
		t.Skip("set REDDITBBS_MOCKUP_DIR to render screen mockups")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	d, fs := newDeps(t)
	d.Config.Areas = []config.Area{
		{Name: "Linux", Subreddit: "linux"},
		{Name: "Programming", Subreddit: "programming"},
		{Name: "Retro Battlestations", Subreddit: "retrobattlestations"},
		{Name: "Command Line", Subreddit: "commandline"},
		{Name: "Vintage Computing", Subreddit: "vintagecomputing"},
	}
	d.Session.VisitArea("linux")
	d.Session.VisitArea("commandline")
	d.Session.ThreadsOpened = 4
	d.Session.MessagesRead = 23
	d.Session.LinksOpened = 2
	d.Version = "0.1.0"
	fs.Listings["linux/hot/"] = mockupListing()
	fs.Threads["k72"] = mockupThread()

	sim := term.NewSim(100, 30)
	save := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(simHTML(sim)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app := ui.New(sim, NewSplash(d))
	app.Draw()
	save("01-splash")

	press(app, term.R(' '))
	save("02-mainmenu")

	press(app, term.R('m'))
	save("03-arealist")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	press(app, term.K(term.KeyDown)) // the kernel post
	save("04-postlist")

	press(app, term.K(term.KeyEnter))
	pump(t, app)
	save("05-threadindex")

	press(app, term.R('?'))
	save("06-help")
	press(app, term.R(' '))

	press(app, term.K(term.KeyEnter))
	save("07-reader")

	press(app, term.R('q'), term.R('q'), term.R('q'), term.R('q'), term.R('g'), term.R('y'))
	save("08-goodbye")
}

func mockupListing() reddit.Listing {
	now := testNow
	mk := func(id, title, author string, score, comments int, age time.Duration, domain string, self, sticky bool) *reddit.Post {
		p := redditest.SamplePost(id, title)
		p.Author, p.Score, p.NumComments, p.Created = author, score, comments, now.Add(-age)
		p.Domain, p.IsSelf, p.Stickied = domain, self, sticky
		if self {
			p.Domain = "self.linux"
			p.URL = "https://www.reddit.com/r/linux/comments/" + id + "/"
		}
		return p
	}
	return reddit.Listing{Posts: []*reddit.Post{
		mk("wq1", "Weekly questions thread: ask anything about Linux here", "AutoModerator", 41, 212, 30*time.Hour, "", true, true),
		mk("k72", "Kernel 7.2 released with the new EEVDF scheduler and lazy preemption", "torvaldsfan", 2143, 342, 5*time.Hour, "kernel.org", false, false),
		mk("x11", "Why I switched back to X11 after two years on Wayland", "xorg4life", 618, 489, 7*time.Hour, "", true, false),
		mk("rce", "Show off your rice: September 2026 screenshot thread", "ricer_9000", 377, 215, 11*time.Hour, "", true, false),
		mk("deb", "Debian 14 'forky' freeze date announced for January", "dpkg_dan", 905, 157, 13*time.Hour, "lists.debian.org", false, false),
		mk("sys", "systemd 260: what actually changed and why your boot got faster", "unit_file", 512, 128, 16*time.Hour, "0pointer.net", false, false),
		mk("tty", "I run my whole workflow from a VT220. AMA", "serial_killer", 1288, 301, 19*time.Hour, "", true, false),
		mk("gpu", "NVIDIA open kernel modules now default in the 590 series", "nouveau_nick", 733, 96, 22*time.Hour, "phoronix.com", false, false),
		mk("bsd", "FreeBSD 15 ships with a Linux-compatible container runtime", "bsd_lurker", 264, 88, 26*time.Hour, "freebsd.org", false, false),
		mk("vim", "Neovim 0.13 drops Vimscript support for plugins", "modal_mike", 1560, 611, 28*time.Hour, "neovim.io", false, false),
		mk("pkg", "Flatpak vs Snap vs AppImage in 2026: a measured comparison", "sandbox_sam", 198, 143, 31*time.Hour, "", true, false),
		mk("rst", "Rust in the kernel: one year of the new networking drivers", "rustacean", 842, 274, 35*time.Hour, "lwn.net", false, false),
		mk("bbs", "Anyone else still dial into BBSes? Synchronet board list inside", "sysop_1994", 456, 67, 38*time.Hour, "", true, false),
		mk("ssh", "OpenSSH 10.2 makes post-quantum key exchange the default", "ed25519", 1021, 132, 41*time.Hour, "openssh.com", false, false),
		mk("lap", "Framework 16 second gen: the Linux review", "hinge_hater", 689, 203, 44*time.Hour, "", true, false),
		mk("zfs", "ZFS 2.4 lands RAIDZ expansion without the rewrite penalty", "pool_boy", 573, 91, 47*time.Hour, "openzfs.org", false, false),
		mk("wm", "Sway 2.0 released: tearing control, HDR and a new IPC", "tiling_tim", 934, 178, 50*time.Hour, "swaywm.org", false, false),
		mk("git", "Git 3.0 switches the default hash to SHA-256", "reflog", 1877, 402, 53*time.Hour, "git-scm.com", false, false),
		mk("cli", "The 40-year-old shell script that still runs our billing", "legacy_len", 2210, 356, 57*time.Hour, "", true, false),
		mk("arm", "Asahi Linux: M4 GPU driver reaches conformance", "marcan_fan", 1432, 167, 60*time.Hour, "asahilinux.org", false, false),
		mk("net", "WireGuard in userspace vs kernel: numbers from a 100G lab", "wg_wonk", 388, 74, 64*time.Hour, "", true, false),
	}}
}

func mockupThread() reddit.Thread {
	now := testNow
	c := func(id, parent, author, body string, score int, age time.Duration, kids ...*reddit.Comment) *reddit.Comment {
		return &reddit.Comment{ID: id, Fullname: "t1_" + id, ParentFullname: parent, Author: author, Body: body, Score: score, Created: now.Add(-age), Children: kids}
	}
	post := redditest.SamplePost("k72", "Kernel 7.2 released with the new EEVDF scheduler and lazy preemption")
	post.Author, post.Score, post.NumComments, post.Created, post.Domain = "torvaldsfan", 2143, 342, now.Add(-5*time.Hour), "kernel.org"
	post.URL = "https://kernel.org/"
	c1 := c("c1", "t3_k72", "sched_nerd", "The EEVDF changes are the headline but the real win is the lazy preemption work. On my 16-core box the tail latencies under a full kernel build dropped from ~40ms to ~6ms.\n\nBenchmarks are in the [mailing list post](https://lore.kernel.org/lkml/example) if anyone wants the numbers.\n\n> Has anyone tried it on a mixed big.LITTLE laptop yet?\n\nYes, X1 Nano Gen 5, works fine, battery slightly better.", 412, 4*time.Hour,
		c("c2", "t1_c1", "torvaldsfan", "Agreed. The one regression I've seen is with the old cgroup v1 CPU controller, which nobody should be using in 2026 anyway.", 98, 3*time.Hour,
			c("c3", "t1_c2", "xorg4life", "Which distro? I'm on Fedora and the new kernel isn't in updates-testing yet.", 41, 2*time.Hour),
			c("c4", "t1_c2", "dpkg_dan", "Debian will have it in about 2031.", 122, 2*time.Hour)),
		c("c5", "t1_c1", "unit_file", "Anyone benchmarked it against 7.1 on a laptop? I care more about idle power than build times.", 301, 3*time.Hour))
	c1.Children[0].IsSubmitter = true
	c1.More = &reddit.MoreStub{ParentFullname: "t1_c1", Count: 14, IDs: []string{"a", "b", "c"}}
	c6 := c("c6", "t3_k72", "kernel_karen", "I am once again asking for a stable driver ABI.", 156, 4*time.Hour,
		c("c7", "t1_c6", "gregkh_fan", "[removed]", 77, 3*time.Hour))
	c6.Children[0].BodyRemoved = true
	c8 := c("c8", "t3_k72", "rustacean", "More Rust drivers landed this cycle too, which nobody seems to have noticed because of the scheduler news.", 64, 3*time.Hour,
		c("c9", "t1_c8", "cpp_forever", "Oh no.", 12, 2*time.Hour,
			c("c10", "t1_c9", "rustacean", "Oh yes.", 9, time.Hour)))
	c11 := c("c11", "t3_k72", "[deleted]", "Meanwhile FreeBSD has had this for years.", 38, 2*time.Hour)
	c11.AuthorDeleted = true
	c12 := c("c12", "t3_k72", "modbot", "Reminder: keep it civil. Distro wars go in the weekly thread.", 5, time.Hour)
	c12.Distinguished = "moderator"
	th := reddit.Thread{Post: post, Comments: []*reddit.Comment{c1, c6, c8, c11, c12},
		More: &reddit.MoreStub{ParentFullname: "t3_k72", Count: 318, IDs: []string{"p", "q"}}}
	var setDepth func([]*reddit.Comment, int)
	setDepth = func(cs []*reddit.Comment, d int) {
		for _, c := range cs {
			c.Depth = d
			setDepth(c.Children, d+1)
		}
	}
	setDepth(th.Comments, 0)
	return th
}

var ansiHex = map[term.Color]string{
	term.Default: "", term.Black: "#000000", term.Red: "#aa0000", term.Green: "#00aa00", term.Yellow: "#aa5500",
	term.Blue: "#0000aa", term.Magenta: "#aa00aa", term.Cyan: "#00aaaa", term.White: "#aaaaaa",
	term.BrightBlack: "#555555", term.BrightRed: "#ff5555", term.BrightGreen: "#55ff55", term.BrightYellow: "#ffff55",
	term.BrightBlue: "#5555ff", term.BrightMagenta: "#ff55ff", term.BrightCyan: "#55ffff", term.BrightWhite: "#ffffff",
}

func styleCSS(st term.Style) string {
	fg, bg := ansiHex[st.FG], ansiHex[st.BG]
	if fg == "" {
		fg = "#aaaaaa"
	}
	if st.Reverse {
		if bg == "" {
			bg = "#000000"
		}
		fg, bg = bg, fg
	}
	var b strings.Builder
	fmt.Fprintf(&b, "color:%s", fg)
	if bg != "" {
		fmt.Fprintf(&b, ";background:%s", bg)
	}
	if st.Bold {
		b.WriteString(";font-weight:bold")
	}
	return b.String()
}

// simHTML renders the simulated screen as a <pre> with one span per style run.
func simHTML(sim *term.Sim) string {
	w, h := sim.Size()
	var b strings.Builder
	b.WriteString(`<pre class="term">`)
	for y := 0; y < h; y++ {
		var run strings.Builder
		var cur string
		flush := func() {
			if run.Len() == 0 {
				return
			}
			fmt.Fprintf(&b, `<span style="%s">%s</span>`, cur, html.EscapeString(run.String()))
			run.Reset()
		}
		for x := 0; x < w; x++ {
			s, st := sim.CellAt(x, y)
			if s == "" {
				// blank cell or the second half of a wide cluster
				if prev, _ := sim.CellAt(x-1, y); x > 0 && textfmt.Width(prev) == 2 {
					continue
				}
				s = " "
			}
			css := styleCSS(st)
			if css != cur {
				flush()
				cur = css
			}
			run.WriteString(s)
		}
		flush()
		b.WriteString("\n")
	}
	b.WriteString("</pre>")
	return b.String()
}
