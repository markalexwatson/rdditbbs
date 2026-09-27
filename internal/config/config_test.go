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
	env := map[string]string{"RDDITBBS_CLIENT_ID": "envid"}
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
	env := map[string]string{"RDDITBBS_CLIENT_ID": "envid", "RDDITBBS_CLIENT_SECRET": "envsecret"}
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

func TestThemeSettings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte(`
[display]
theme = "amber"

[theme]
base = "classic"
heading = "bright red"
cursor = "black on cyan"
`), 0o600)
	c, err := Load(p, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.Display.Theme != "amber" {
		t.Errorf("theme = %q", c.Display.Theme)
	}
	if c.Theme.Base != "classic" || c.Theme.Overrides["heading"] != "bright red" || c.Theme.Overrides["cursor"] != "black on cyan" {
		t.Errorf("theme table = %+v", c.Theme)
	}
	if !c.HasCustomTheme() {
		t.Error("overrides present should mean a custom theme exists")
	}
	c.Display.Theme = "custom"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	again, _ := Load(p, noEnv)
	if again.Display.Theme != "custom" || again.Theme.Overrides["heading"] != "bright red" || again.Theme.Base != "classic" {
		t.Errorf("round trip = %+v %+v", again.Display, again.Theme)
	}
}

func TestThemeDefaults(t *testing.T) {
	c, _ := Load(filepath.Join(t.TempDir(), "config.toml"), noEnv)
	if c.Display.Theme != "classic" || c.HasCustomTheme() || c.Theme.Base != "classic" {
		t.Errorf("defaults = %+v %+v", c.Display, c.Theme)
	}
}

func TestSourceSetting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[reddit]\nsource = \"rss\"\n"), 0o600)
	c, err := Load(p, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.Reddit.Source != "rss" {
		t.Errorf("source = %q", c.Reddit.Source)
	}
	d, _ := Load(filepath.Join(t.TempDir(), "config.toml"), noEnv)
	if d.Reddit.Source != "auto" {
		t.Errorf("default source = %q, want auto", d.Reddit.Source)
	}
}

func TestParseSubredditNames(t *testing.T) {
	in := `r/linux, /r/Programming
https://old.reddit.com/r/commandline/
https://www.reddit.com/r/rust+golang+vim
retrobattlestations  LINUX  bad!name x
r/this_name_is_far_too_long_for_reddit`
	names, invalid := ParseSubredditNames(in)
	want := []string{"linux", "Programming", "commandline", "rust", "golang", "vim", "retrobattlestations"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("names = %v\nwant    %v", names, want)
	}
	if strings.Join(invalid, ",") != "bad!name,x,r/this_name_is_far_too_long_for_reddit" { // reported as pasted
		t.Errorf("invalid = %v", invalid)
	}
}

func TestImportAreas(t *testing.T) {
	c := &Config{}
	c.AddArea(Area{Name: "Linux", Subreddit: "linux"})
	added := c.ImportAreas([]string{"LINUX", "rust", "golang", "rust"})
	if added != 2 || len(c.Areas) != 3 || c.Areas[1].Name != "rust" || c.Areas[1].Subreddit != "rust" {
		t.Errorf("added %d areas %+v", added, c.Areas)
	}
}

func TestParseSubredditNamesFromAddresses(t *testing.T) {
	names, invalid := ParseSubredditNames(`https://www.reddit.com/r/linux/top/?t=day
https://old.reddit.com/r/rust/comments/abc123/some_title/
reddit.com/r/vim?utm=x
https://example.com/r/evil
https://www.reddit.com/user/someone`)
	if strings.Join(names, ",") != "linux,rust,vim" {
		t.Errorf("names = %v", names)
	}
	if len(invalid) != 2 {
		t.Errorf("addresses on other sites, or without a subreddit, must be refused: %v", invalid)
	}
}
