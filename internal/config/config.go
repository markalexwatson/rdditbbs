// Package config loads and saves the user's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultUserAgent identifies the client to Reddit when the user has not set one.
const DefaultUserAgent = "linux:rdditbbs:0.1.0 (by /u/rdditbbs)"

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
		Source       string `toml:"source"` // auto, api or rss
	} `toml:"reddit"`
	Display struct {
		PeekPane    bool   `toml:"peek_pane"`
		DefaultSort string `toml:"default_sort"`
		Theme       string `toml:"theme"`
	} `toml:"display"`
	ThemeTable map[string]string `toml:"theme,omitempty"`
	Areas      []Area            `toml:"areas"`
}

// ThemeSettings is the optional [theme] table: a base theme plus per-role
// colour overrides keyed by role name (see theme.RoleName).
type ThemeSettings struct {
	Base      string
	Overrides map[string]string
}

// Config is the effective configuration: file values overlaid by environment
// variables. Only file values are ever saved.
type Config struct {
	file
	Path  string
	Theme ThemeSettings

	envID, envSecret string
}

// DefaultPath is $XDG_CONFIG_HOME/rdditbbs/config.toml or the OS equivalent.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rdditbbs", "config.toml"), nil
}

// Load reads path if it exists, applies defaults, then overlays
// RDDITBBS_CLIENT_ID and RDDITBBS_CLIENT_SECRET from getenv.
func Load(path string, getenv func(string) string) (*Config, error) {
	c := &Config{Path: path}
	c.Display.PeekPane = true
	c.Display.DefaultSort = "hot"
	c.Reddit.UserAgent = DefaultUserAgent
	c.Reddit.Source = "auto"
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
		if c.Reddit.Source == "" {
			c.Reddit.Source = "auto"
		}
	}
	if c.Display.Theme == "" {
		c.Display.Theme = "classic"
	}
	c.Theme = ThemeSettings{Base: "classic", Overrides: map[string]string{}}
	for k, v := range c.ThemeTable {
		if k == "base" {
			c.Theme.Base = v
			continue
		}
		c.Theme.Overrides[k] = v
	}
	c.envID = getenv("RDDITBBS_CLIENT_ID")
	c.envSecret = getenv("RDDITBBS_CLIENT_SECRET")
	return c, nil
}

// HasCustomTheme reports whether the [theme] table overrides any role.
func (c *Config) HasCustomTheme() bool { return len(c.Theme.Overrides) > 0 }

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

var subredditName = regexp.MustCompile(`^[A-Za-z0-9_]{2,21}$`)

// ParseSubredditNames extracts subreddit names from pasted text: names
// separated by whitespace, commas or plus signs, with or without r/ or a
// reddit URL in front. It returns valid names once each (first spelling
// wins, compared without case) and the tokens it could not accept.
func ParseSubredditNames(text string) (names, invalid []string) {
	seen := map[string]bool{}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '+' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
	}) {
		name := tok
		if i := strings.LastIndex(name, "/r/"); i >= 0 {
			name = name[i+3:]
		}
		name = strings.TrimPrefix(name, "r/")
		name = strings.Trim(name, "/")
		if !subredditName.MatchString(name) {
			invalid = append(invalid, name)
			continue
		}
		if key := strings.ToLower(name); !seen[key] {
			seen[key] = true
			names = append(names, name)
		}
	}
	return names, invalid
}

// ImportAreas adds an area for each name not already configured and reports
// how many were added.
func (c *Config) ImportAreas(names []string) int {
	added := 0
	for _, n := range names {
		if c.AddArea(Area{Name: n, Subreddit: n}) {
			added++
		}
	}
	return added
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
	c.ThemeTable = nil
	if c.HasCustomTheme() || (c.Theme.Base != "" && c.Theme.Base != "classic") {
		c.ThemeTable = map[string]string{"base": c.Theme.Base}
		for k, v := range c.Theme.Overrides {
			c.ThemeTable[k] = v
		}
	}
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
