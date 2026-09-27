package redditest

import (
	"context"
	"time"

	"github.com/markalexwatson/redditbbs/internal/reddit"
)

// DemoListing is a realistic r/linux front page for demo mode and screenshots.
func DemoListing(now time.Time) reddit.Listing {
	mk := func(id, title, author string, score, comments int, age time.Duration, domain string, self, sticky bool) *reddit.Post {
		p := SamplePost(id, title)
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

// DemoThread is a realistic comment thread for demo mode and screenshots.
func DemoThread(now time.Time) reddit.Thread {
	c := func(id, parent, author, body string, score int, age time.Duration, kids ...*reddit.Comment) *reddit.Comment {
		return &reddit.Comment{ID: id, Fullname: "t1_" + id, ParentFullname: parent, Author: author, Body: body, Score: score, Created: now.Add(-age), Children: kids}
	}
	post := SamplePost("k72", "Kernel 7.2 released with the new EEVDF scheduler and lazy preemption")
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

// DemoStore serves the demo listing for every subreddit and the demo thread
// for every post, so the UI can be explored without Reddit credentials.
type DemoStore struct{ now func() time.Time }

// NewDemoStore creates a demo store; ages are computed from now.
func NewDemoStore(now func() time.Time) *DemoStore { return &DemoStore{now: now} }

// Posts implements Store.
func (d *DemoStore) Posts(_ context.Context, sub string, _ reddit.Sort, after string, _ reddit.Fetch) (reddit.Listing, error) {
	if after != "" {
		return reddit.Listing{}, nil
	}
	l := DemoListing(d.now())
	for _, p := range l.Posts {
		p.Subreddit = sub
	}
	return l, nil
}

// Thread implements Store.
func (d *DemoStore) Thread(_ context.Context, sub, postID string, _ reddit.CommentSort, _ reddit.Fetch) (reddit.Thread, error) {
	th := DemoThread(d.now())
	for _, p := range DemoListing(d.now()).Posts {
		if p.ID == postID {
			th.Post = p
			th.Post.Subreddit = sub
			break
		}
	}
	return th, nil
}

// Subtree implements Store.
func (d *DemoStore) Subtree(ctx context.Context, sub, postID, commentID string, sort reddit.CommentSort) (reddit.Thread, error) {
	th, _ := d.Thread(ctx, sub, postID, sort, reddit.Fetch{})
	return th, nil
}

// MoreChildren implements Store.
func (d *DemoStore) MoreChildren(_ context.Context, _ string, ids []string, _ reddit.CommentSort) (reddit.Things, error) {
	var th reddit.Things
	for i, id := range ids {
		th.Comments = append(th.Comments, &reddit.Comment{ID: id, Fullname: "t1_" + id, ParentFullname: "t1_c1",
			Author: "demo_user_" + id, Body: "Demo reply number " + itoa(i+1) + ", loaded on request.", Score: 3 - i, Created: d.now().Add(-time.Hour)})
	}
	return th, nil
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

var _ reddit.Store = (*DemoStore)(nil)
