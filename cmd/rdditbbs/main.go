// Command rdditbbs is a BBS-style terminal reader for Reddit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	rdebug "runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/markalexwatson/rdditbbs/internal/browser"
	"github.com/markalexwatson/rdditbbs/internal/config"
	"github.com/markalexwatson/rdditbbs/internal/reddit"
	"github.com/markalexwatson/rdditbbs/internal/reddit/redditest"
	"github.com/markalexwatson/rdditbbs/internal/rss"
	"github.com/markalexwatson/rdditbbs/internal/session"
	"github.com/markalexwatson/rdditbbs/internal/term"
	"github.com/markalexwatson/rdditbbs/internal/theme"
	"github.com/markalexwatson/rdditbbs/internal/ui"
	"github.com/markalexwatson/rdditbbs/internal/ui/screens"
)

// Version is set at build time via -ldflags "-X main.Version=…".
var Version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run parses flags, loads config and runs the UI. Exit codes: 0 ok, 1 runtime
// failure, 2 usage or configuration error.
func run(args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet("rdditbbs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "", "path to config.toml")
	debugFlag := fs.Bool("debug", false, "save the last unparseable Reddit response to the state directory")
	showVersion := fs.Bool("version", false, "print the version and exit")
	demo := fs.Bool("demo", false, "browse built-in sample data without Reddit credentials")
	rssFlag := fs.Bool("rss", false, "read Reddit's public feeds instead of the API (no credentials, no scores, flat comments)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "rdditbbs", Version)
		return 0
	}

	path := *cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintln(stderr, "rdditbbs: config:", err)
			return 2
		}
		path = p
	}
	cfg, err := config.Load(path, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "rdditbbs: config:", err)
		return 2
	}
	if err := applyTheme(cfg); err != nil {
		fmt.Fprintln(stderr, "rdditbbs: config:", err)
		return 2
	}
	switch cfg.Reddit.Source {
	case "auto", "api", "rss":
	default:
		fmt.Fprintf(stderr, "rdditbbs: config: reddit.source must be auto, api or rss, not %q\n", cfg.Reddit.Source)
		return 2
	}
	source := chooseSource(cfg, *rssFlag, *demo)
	if fs.NArg() > 0 {
		switch fs.Arg(0) {
		case "import":
			return runImport(cfg, fs.Args()[1:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "rdditbbs: unknown command %q (commands: import)\n", fs.Arg(0))
			return 2
		}
	}

	t, err := term.NewTcell()
	if err != nil {
		fmt.Fprintln(stderr, "rdditbbs: terminal:", err)
		return 1
	}
	// One unconditional cleanup: Fini is idempotent, so every exit path
	// (normal, error or panic) restores the terminal exactly once.
	defer t.Fini()
	defer func() {
		if p := recover(); p != nil {
			t.Fini()
			fmt.Fprintf(stderr, "rdditbbs: panic: %v\n%s", p, rdebug.Stack())
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
	deps.Source = source
	switch source {
	case "api":
		if cfg.HasCredentials() {
			deps.Store = deps.MakeStore(cfg.ClientID(), cfg.ClientSecret())
		}
	case "rss":
		deps.Store = rss.NewClient(cfg.Reddit.UserAgent, rss.WithOnWait(func(d time.Duration) {
			if app != nil {
				app.Post(ui.RateLimited{Wait: d})
			}
		}))
		if len(cfg.Areas) == 0 {
			cfg.Areas = append([]config.Area(nil), config.DefaultAreas...)
		}
	case "demo":
		deps.Demo = true
		deps.Store = redditest.NewDemoStore(time.Now)
		if len(cfg.Areas) == 0 {
			cfg.Areas = append([]config.Area(nil), config.DefaultAreas...)
		}
	}

	app = ui.New(t, screens.NewSplash(deps), ui.WithThemeHook(func(name string) error {
		cfg.Display.Theme = name
		return cfg.Save() // the theme still applies for this session if this fails
	}))
	err = app.Run()
	t.Fini()
	if err != nil {
		fmt.Fprintln(stderr, "rdditbbs:", err)
		return 1
	}
	return 0
}

var badResponseMu sync.Mutex

// stdin is where import reads pasted names from; tests replace it.
var stdin io.Reader = os.Stdin

// runImport adds subreddits to the configured areas from its arguments, or
// from standard input when there are none.
func runImport(cfg *config.Config, args []string, stdout, stderr io.Writer) int {
	text := strings.Join(args, " ")
	if len(args) == 0 {
		b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			fmt.Fprintln(stderr, "rdditbbs: import:", err)
			return 1
		}
		text = string(b)
	}
	names, invalid := config.ParseSubredditNames(text)
	added := cfg.ImportAreas(names)
	if added > 0 {
		if err := cfg.Save(); err != nil {
			fmt.Fprintln(stderr, "rdditbbs: import: could not save config:", err)
			return 1
		}
	}
	plural := "s"
	if added == 1 {
		plural = ""
	}
	fmt.Fprintf(stdout, "%d area%s added, %d already present, %d areas in total.\n", added, plural, len(names)-added, len(cfg.Areas))
	if len(invalid) > 0 {
		fmt.Fprintf(stdout, "Skipped (not valid subreddit names): %s\n", strings.Join(invalid, ", "))
	}
	return 0
}

// chooseSource decides where posts come from: demo when asked, rss when
// forced by flag or config, api when pinned, otherwise the API if credentials
// exist and the feeds if not.
func chooseSource(cfg *config.Config, rssFlag, demo bool) string {
	switch {
	case demo:
		return "demo"
	case rssFlag, cfg.Reddit.Source == "rss":
		return "rss"
	case cfg.Reddit.Source == "api":
		return "api"
	case cfg.HasCredentials():
		return "api"
	default:
		return "rss"
	}
}

// applyTheme registers the [theme] table as the "custom" theme when it has
// overrides, then activates the theme named in [display].
func applyTheme(cfg *config.Config) error {
	if base := cfg.Theme.Base; base != "" {
		if _, ok := theme.Get(base); !ok {
			return fmt.Errorf("theme.base: unknown theme %q (available: %s)", base, strings.Join(theme.Names(), ", "))
		}
	}
	if cfg.HasCustomTheme() {
		t, err := theme.Custom(cfg.Theme.Base, cfg.Theme.Overrides)
		if err != nil {
			return err
		}
		theme.Register(t)
	} else {
		theme.Unregister("custom") // no overrides: "custom" is not on offer
	}
	name := cfg.Display.Theme
	if name == "" {
		name = "classic"
	}
	if err := theme.Set(name); err != nil {
		if name == "custom" {
			return fmt.Errorf("display.theme is \"custom\" but the [theme] table defines no overrides")
		}
		return fmt.Errorf("display.theme: %v", err)
	}
	return nil
}

// saveBadResponse atomically writes body to
// $XDG_STATE_HOME/rdditbbs/last-error.json with mode 0600.
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
	dir = filepath.Join(dir, "rdditbbs")
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
