// Package config loads and saves the user's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultUserAgent identifies the client to Reddit when the user has not set one.
const DefaultUserAgent = "linux:redditbbs:0.1.0 (by /u/redditbbs)"

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
	} `toml:"reddit"`
	Display struct {
		PeekPane    bool   `toml:"peek_pane"`
		DefaultSort string `toml:"default_sort"`
	} `toml:"display"`
	Areas []Area `toml:"areas"`
}

// Config is the effective configuration: file values overlaid by environment
// variables. Only file values are ever saved.
type Config struct {
	file
	Path string

	envID, envSecret string
}

// DefaultPath is $XDG_CONFIG_HOME/redditbbs/config.toml or the OS equivalent.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "redditbbs", "config.toml"), nil
}

// Load reads path if it exists, applies defaults, then overlays
// REDDITBBS_CLIENT_ID and REDDITBBS_CLIENT_SECRET from getenv.
func Load(path string, getenv func(string) string) (*Config, error) {
	c := &Config{Path: path}
	c.Display.PeekPane = true
	c.Display.DefaultSort = "hot"
	c.Reddit.UserAgent = DefaultUserAgent
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
	}
	c.envID = getenv("REDDITBBS_CLIENT_ID")
	c.envSecret = getenv("REDDITBBS_CLIENT_SECRET")
	return c, nil
}

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

// RemoveArea deletes the area at index i; out-of-range is a no-op.
func (c *Config) RemoveArea(i int) {
	if i < 0 || i >= len(c.Areas) {
		return
	}
	c.Areas = append(c.Areas[:i], c.Areas[i+1:]...)
}

// Save writes the file atomically with mode 0600 in a 0700 directory.
func (c *Config) Save() error {
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
