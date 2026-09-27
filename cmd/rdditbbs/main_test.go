package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/markalexwatson/rdditbbs/internal/config"
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
