package rss

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskRoundTripAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "feeds")
	d, err := NewDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)
	if err := d.Put("https://x/r/linux/hot/.rss?limit=100", []byte("<feed/>"), at); err != nil {
		t.Fatal(err)
	}
	b, fetched, ok := d.Get("https://x/r/linux/hot/.rss?limit=100")
	if !ok || string(b) != "<feed/>" || !fetched.Equal(at) {
		t.Fatalf("get = %q %v %v", b, fetched, ok)
	}
	if _, _, ok := d.Get("https://x/other"); ok {
		t.Error("unknown URL should miss")
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("files = %v (no temp files should remain)", entries)
	}
	if info, _ := entries[0].Info(); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o", info.Mode().Perm())
	}
}

func TestDiskSurvivesReopenAndIgnoresCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	d, _ := NewDisk(dir)
	at := time.Unix(1_790_000_000, 0)
	d.Put("https://x/a", []byte("body a"), at)
	os.WriteFile(filepath.Join(dir, "garbage.feed"), []byte("not a cache file"), 0o600)
	again, err := NewDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _, ok := again.Get("https://x/a"); !ok || string(b) != "body a" {
		t.Errorf("reopen lost the entry: %q %v", b, ok)
	}
	if n := again.Stats(time.Unix(1_790_000_000, 0)).Entries; n != 1 {
		t.Errorf("corrupt files must not count, entries = %d", n)
	}
}

func TestDiskPruneAndStats(t *testing.T) {
	d, _ := NewDisk(t.TempDir())
	now := time.Unix(1_790_000_000, 0)
	d.Put("https://x/r/linux/hot/.rss?limit=100", []byte("l"), now.Add(-30*time.Hour))
	d.Put("https://x/r/linux/comments/abc/.rss?limit=500", []byte("t1"), now.Add(-2*time.Hour))
	d.Put("https://x/r/linux/comments/def/.rss?limit=500", []byte("t2"), now.Add(-time.Minute))
	st := d.Stats(now)
	if st.Entries != 3 || st.Threads != 2 || st.Listings != 1 || !st.Newest.Equal(now.Add(-time.Minute)) {
		t.Errorf("stats = %+v", st)
	}
	if removed := d.Prune(now, 24*time.Hour); removed != 1 {
		t.Errorf("pruned %d, want 1", removed)
	}
	if _, _, ok := d.Get("https://x/r/linux/hot/.rss?limit=100"); ok {
		t.Error("entry older than the maximum age should be gone")
	}
	if d.Stats(now).Entries != 2 {
		t.Error("two entries should remain")
	}
}
