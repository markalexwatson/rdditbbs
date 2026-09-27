package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/config"
	"github.com/markalexwatson/rdditbbs/internal/rss"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/theme"
)

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("code = %d, stderr %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "rdditbbs ") {
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

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

func TestRunRejectsUnknownTheme(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := writeFile(path, "[display]\ntheme = \"nope\"\n"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "nope") || !strings.Contains(errb.String(), "classic") {
		t.Errorf("stderr should name the bad theme and the choices: %q", errb.String())
	}
}

func TestRunRejectsBadThemeOverride(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := writeFile(path, "[theme]\nheading = \"mauve\"\n"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "theme.heading") {
		t.Errorf("stderr should name the key: %q", errb.String())
	}
}

func TestApplyThemeRejectsBadBaseWithoutOverrides(t *testing.T) {
	cfg := &config.Config{}
	cfg.Display.Theme = "classic"
	cfg.Theme = config.ThemeSettings{Base: "nope"}
	if err := applyTheme(cfg); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("an unknown base should fail even with no overrides, got %v", err)
	}
}

func TestApplyThemeRegistersCustomAndSelects(t *testing.T) {
	t.Cleanup(func() { theme.Unregister("custom"); _ = theme.Set("classic") })
	cfg := &config.Config{}
	cfg.Display.Theme = "custom"
	cfg.Theme = config.ThemeSettings{Base: "green", Overrides: map[string]string{"heading": "white"}}
	if err := applyTheme(cfg); err != nil {
		t.Fatal(err)
	}
	if theme.Current() != "custom" || theme.Style(theme.Heading).FG != term.White {
		t.Errorf("theme = %q heading %+v", theme.Current(), theme.Style(theme.Heading))
	}
	cfg2 := &config.Config{}
	cfg2.Display.Theme = "custom" // selected but no overrides defined
	if err := applyTheme(cfg2); err == nil {
		t.Error("selecting custom without a [theme] table should fail")
	}
}

func TestChooseSource(t *testing.T) {
	withCreds := &config.Config{}
	withCreds.SetCredentials("id", "secret")
	withCreds.Reddit.Source = "auto"
	noCreds := &config.Config{}
	noCreds.Reddit.Source = "auto"
	pinnedAPI := &config.Config{}
	pinnedAPI.Reddit.Source = "api"
	pinnedRSS := &config.Config{}
	pinnedRSS.SetCredentials("id", "secret")
	pinnedRSS.Reddit.Source = "rss"
	cases := []struct {
		name string
		cfg  *config.Config
		rss  bool
		demo bool
		want string
	}{
		{"auto with credentials", withCreds, false, false, "api"},
		{"auto without credentials", noCreds, false, false, "rss"},
		{"flag forces rss", withCreds, true, false, "rss"},
		{"config pins rss", pinnedRSS, false, false, "rss"},
		{"config pins api without credentials", pinnedAPI, false, false, "api"},
		{"demo wins", withCreds, true, true, "demo"},
	}
	for _, c := range cases {
		if got := chooseSource(c.cfg, c.rss, c.demo); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestRunRejectsUnknownSource(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := writeFile(path, "[reddit]\nsource = \"carrier pigeon\"\n"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "source") {
		t.Errorf("code = %d stderr = %q", code, errb.String())
	}
}

func TestImportCommand(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	defer func(r io.Reader) { stdin = r }(stdin)
	stdin = strings.NewReader("r/linux r/rust\nhttps://old.reddit.com/r/vim+emacs\nbad!name\n")
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path, "import"}, &out, &errb); code != 0 {
		t.Fatalf("code = %d stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "4 areas added") || !strings.Contains(out.String(), "bad!name") {
		t.Errorf("stdout = %q", out.String())
	}
	cfg, err := config.Load(path, func(string) string { return "" })
	if err != nil || len(cfg.Areas) != 4 || cfg.Areas[3].Subreddit != "emacs" {
		t.Errorf("areas = %+v err %v", cfg.Areas, err)
	}
	stdin = strings.NewReader("r/linux")
	out.Reset()
	if code := run([]string{"--config", path, "import", "r/golang"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "1 area added") {
		t.Errorf("arguments should be accepted instead of stdin: code %d out %q", code, out.String())
	}
}

func TestSyncOnceCommand(t *testing.T) {
	var mu sync.Mutex
	var got []string
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Path)
		mu.Unlock()
		w.Header().Set("X-Ratelimit-Remaining", "50")
		w.Header().Set("X-Ratelimit-Reset", "1")
		if strings.Contains(r.URL.Path, "/comments/") {
			http.ServeFile(w, r, "../../internal/rss/testdata/thread.xml")
			return
		}
		http.ServeFile(w, r, "../../internal/rss/testdata/listing.xml")
	}))
	defer hs.Close()
	defer func(u string, c func() (string, error)) { feedBaseURL, cacheDir = u, c }(feedBaseURL, cacheDir)
	feedBaseURL = hs.URL
	cache := t.TempDir()
	cacheDir = func() (string, error) { return cache, nil }

	path := t.TempDir() + "/config.toml"
	if err := writeFile(path, "[[areas]]\nname = \"Linux\"\nsubreddit = \"linux\"\n"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path, "sync", "--once", "--threads", "2"}, &out, &errb); code != 0 {
		t.Fatalf("code = %d stderr %q", code, errb.String())
	}
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 3 {
		t.Errorf("one listing and two threads expected, got %v", got)
	}
	if !strings.Contains(out.String(), "r/linux listing") || !strings.Contains(out.String(), "3 feeds fetched") {
		t.Errorf("stdout = %q", out.String())
	}
	entries, _ := os.ReadDir(cache + "/feeds")
	feeds := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".feed") {
			feeds++
		}
	}
	if feeds != 3 {
		t.Errorf("cache should hold three feeds, has %d", feeds)
	}
}

func TestImportRejectsOversizedInput(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	defer func(r io.Reader) { stdin = r }(stdin)
	stdin = strings.NewReader(strings.Repeat("r/linux ", 200000)) // about 1.6 MB
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path, "import"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "too large") {
		t.Errorf("code = %d stderr %q", code, errb.String())
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("nothing should be saved when the input is refused")
	}
}

func TestSyncRefusesWhileAnotherSyncerRuns(t *testing.T) {
	defer func(u string, c func() (string, error)) { feedBaseURL, cacheDir = u, c }(feedBaseURL, cacheDir)
	feedBaseURL = "http://127.0.0.1:1" // must never be contacted
	cache := t.TempDir()
	cacheDir = func() (string, error) { return cache, nil }
	unlock, ok, err := rss.LockSync(cache + "/feeds")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer unlock()
	path := t.TempDir() + "/config.toml"
	writeFile(path, "[[areas]]\nname = \"Linux\"\nsubreddit = \"linux\"\n")
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path, "sync", "--once"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "already syncing") {
		t.Errorf("code = %d stdout %q stderr %q", code, out.String(), errb.String())
	}
}

func TestSyncNeedsACacheDirectory(t *testing.T) {
	defer func(c func() (string, error)) { cacheDir = c }(cacheDir)
	cacheDir = func() (string, error) { return "", errors.New("no home") }
	path := t.TempDir() + "/config.toml"
	writeFile(path, "[[areas]]\nname = \"Linux\"\nsubreddit = \"linux\"\n")
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path, "sync", "--once"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "cache") {
		t.Errorf("code = %d stderr %q", code, errb.String())
	}
}
