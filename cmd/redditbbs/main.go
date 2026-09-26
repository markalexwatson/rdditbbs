// Command redditbbs is a BBS-style terminal reader for Reddit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	rdebug "runtime/debug"
	"sync"
	"time"

	"github.com/markwatson/redditbbs/internal/browser"
	"github.com/markwatson/redditbbs/internal/config"
	"github.com/markwatson/redditbbs/internal/reddit"
	"github.com/markwatson/redditbbs/internal/session"
	"github.com/markwatson/redditbbs/internal/term"
	"github.com/markwatson/redditbbs/internal/ui"
	"github.com/markwatson/redditbbs/internal/ui/screens"
)

// Version is set at build time via -ldflags "-X main.Version=…".
var Version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run parses flags, loads config and runs the UI. Exit codes: 0 ok, 1 runtime
// failure, 2 usage or configuration error.
func run(args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet("redditbbs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "", "path to config.toml")
	debugFlag := fs.Bool("debug", false, "save the last unparseable Reddit response to the state directory")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "redditbbs", Version)
		return 0
	}

	path := *cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintln(stderr, "redditbbs: config:", err)
			return 2
		}
		path = p
	}
	cfg, err := config.Load(path, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs: config:", err)
		return 2
	}

	t, err := term.NewTcell()
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs: terminal:", err)
		return 1
	}
	// One unconditional cleanup: Fini is idempotent, so every exit path
	// (normal, error or panic) restores the terminal exactly once.
	defer t.Fini()
	defer func() {
		if p := recover(); p != nil {
			t.Fini()
			fmt.Fprintf(stderr, "redditbbs: panic: %v\n%s", p, rdebug.Stack())
			code = 1
		}
	}()

	var app *ui.App
	deps := &screens.Deps{
		Config:  cfg,
		Session: session.New(time.Now()),
		Open:    browser.Open,
		Now:     time.Now,
		Version: Version,
	}
	deps.MakeStore = func(id, secret string) reddit.Store {
		opts := []reddit.Option{reddit.WithOnWait(func(d time.Duration) {
			if app != nil {
				app.Post(ui.RateLimited{Wait: d})
			}
		})}
		if *debugFlag {
			opts = append(opts, reddit.WithBadResponseSink(saveBadResponse))
		}
		return reddit.NewClient(reddit.Credentials{ClientID: id, ClientSecret: secret, UserAgent: cfg.Reddit.UserAgent}, opts...)
	}
	if cfg.HasCredentials() {
		deps.Store = deps.MakeStore(cfg.ClientID(), cfg.ClientSecret())
	}

	app = ui.New(t, screens.NewSplash(deps))
	err = app.Run()
	t.Fini()
	if err != nil {
		fmt.Fprintln(stderr, "redditbbs:", err)
		return 1
	}
	return 0
}

var badResponseMu sync.Mutex

// saveBadResponse atomically writes body to
// $XDG_STATE_HOME/redditbbs/last-error.json with mode 0600.
func saveBadResponse(body []byte) {
	badResponseMu.Lock()
	defer badResponseMu.Unlock()
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "redditbbs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".last-error-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	tmp.Close()
	if err := os.Rename(name, filepath.Join(dir, "last-error.json")); err != nil {
		os.Remove(name)
	}
}
