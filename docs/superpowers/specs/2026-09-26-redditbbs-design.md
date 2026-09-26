# RedditBBS design

Date: 26/09/2026
Status: approved direction, spec awaiting review

## 1. Purpose

RedditBBS is a terminal Reddit reader that looks and behaves like a classic
1990s bulletin board system: menu driven, single-key commands, a command
prompt at the bottom, whole-screen redraws, 16-colour ANSI styling.

It serves four goals at once, in this priority order when they conflict:

1. A daily driver for reading Reddit, so it must be reliable and quick.
2. Nostalgia and fun: the BBS feel is the point, not a skin.
3. Shareable: a single static binary others can run, and a design that
   allows a later multi-user telnet or ssh server without a rewrite.
4. A clean, well-structured Go codebase worth showing.

The first version is read-only. No login, voting, posting or search.

## 2. Decisions already made

| Topic | Decision |
| --- | --- |
| Language and terminal library | Go, `github.com/gdamore/tcell/v2` |
| Interaction model | Classic hotkey menus plus a cursor row moved by arrow keys; Enter opens the cursor row |
| Colour scheme | Scheme A: cyan frames, yellow headings, bright white subjects, green authors, grey chrome, inverse-video cursor row |
| Screen size | Fill the real terminal. Lists show as many rows as fit. Minimum 80x24 |
| Comment view | Threaded index with tree connectors and a peek pane, Enter opens a one-message reader |
| Reddit access | App-only OAuth (client-credentials grant) with a script app the user registers once |
| Deployment | Local CLI first. UI draws through an abstract Canvas so a server can come later |
| BBS flavour in v1 | ANSI welcome splash, main menu with named areas, goodbye screen with session stats |
| Not in v1 | Modem-speed emulation, user login, voting, posting, search, inline images, theme switching, multi-user server |

### Why app-only OAuth

On 26/09/2026 Reddit returned HTTP 403 for `www.reddit.com/r/linux/hot.json`
and `api.reddit.com` from a residential UK IP with a compliant User-Agent,
and `old.reddit.com` redirected the JSON URL to a login page. Only the RSS
feeds answered, and they lack scores, comment counts and threading. The
client-credentials grant gives full read access at roughly 100 requests per
minute without any personal login.

## 3. Architecture

One Go module, `github.com/markwatson/redditbbs`, with a single binary at
`cmd/redditbbs`. Internal packages, each with one job:

| Package | Responsibility | Depends on |
| --- | --- | --- |
| `internal/term` | `Canvas` interface and its tcell implementation. The only package that imports tcell for drawing | tcell |
| `internal/theme` | Named colour roles mapped to styles. Nothing else names a colour | term |
| `internal/ui` | `App` event loop, `Screen` interface, navigation actions, async message delivery | term, theme |
| `internal/ui/widgets` | Reusable drawing helpers: title bar, table with cursor row, wrapped text box, hotkey bar, prompt line, tree connectors | term, theme |
| `internal/ui/screens` | One file per screen | ui, widgets, reddit (via Store), config, session |
| `internal/reddit` | OAuth token handling, HTTP client, typed models, listing and thread parsing, cache | net/http |
| `internal/textfmt` | Markdown-to-terminal simplification, word wrap, link extraction, relative time, score formatting | none |
| `internal/config` | Load and save `config.toml`; areas and credentials | BurntSushi/toml |
| `internal/session` | Start time and counters for the goodbye screen | none |

Dependencies point downward only. `screens` is the only package that knows
about both UI and data.

### 3.1 Canvas and remote-terminal readiness

```go
type Canvas interface {
    Size() (w, h int)
    Put(x, y int, r rune, st Style)     // one cell
    Text(x, y int, s string, st Style) int // returns cells used
    Fill(x, y, w, h int, r rune, st Style)
    ShowCursor(x, y int)
    HideCursor()
}
```

`Style` is the package's own value type (foreground, background, bold,
reverse) so screens never import tcell. The tcell implementation wraps a
`tcell.Screen`. Because tcell can build a Screen from any `tcell.Tty`, a
future server constructs one per connection over a `net.Conn` and reuses
every screen unchanged. Nothing in v1 implements that, but no screen may
assume a single global terminal.

### 3.2 App loop and Screen interface

```go
type Screen interface {
    Init() Action            // called once when the screen becomes top; starts fetches
    Draw(c Canvas)
    HandleKey(k Key) Action
    Update(msg Msg) Action   // results of async work
    Title() string           // shown in the title bar
    Keys() []KeyHelp         // for the hotkey bar and help overlay
}

// Action is one of: nil, Push{Screen}, Pop{}, Replace{Screen}, Quit{},
// Run{func() Msg}, or Batch{[]Action} to combine, for example Push then Run.
type Action interface{}
```

`App` owns a stack of screens, the tcell screen, and a goroutine-safe
inbox. The loop: poll a tcell event, translate to `Key` or `Resize`, call
the top screen, apply the returned Action, redraw the top screen. Every
event causes a full redraw of the top screen; tcell diffs cells, so this is
cheap and keeps screens stateless about what is already on the terminal.

Async work: a screen returns a `Run` action holding a function. App runs it
in a goroutine and posts the returned `Msg` through `tcell.Screen.PostEvent`
so it arrives on the event loop and is delivered to `Update` of the screen
that returned the `Run`. Screens therefore hold no reference to App and stay
easy to test. If that screen has been popped, the message is dropped.
Screens show a "Retrieving..." status while a request is outstanding.

Global keys handled by App before the screen sees them: `?` opens the Help
overlay; Ctrl-L forces a redraw; Ctrl-C quits immediately, restoring the
terminal.

### 3.3 Store interface

Screens depend on this, never on the HTTP client directly:

```go
type Store interface {
    Posts(ctx, subreddit string, sort Sort, after string) (Listing, error)
    Thread(ctx, subreddit, postID string, sort CommentSort) (Thread, error)
    MoreChildren(ctx, linkID string, ids []string, sort CommentSort) ([]Comment, error)
    About(ctx, subreddit string) (Subreddit, error)
}
```

`reddit.Client` implements it. Tests supply a fake.

## 4. Screens

Every screen has the same chrome: a two-line framed title bar at the top
(logo, screen title, area name, position indicator), a hotkey bar and a
`Command:` prompt line at the bottom. Content fills the space between.

Global behaviour:

- `Q` and Escape pop one level. On the Main Menu they go to Goodbye.
- Arrow keys, PgUp, PgDn, Home, End move the cursor on any list. The
  cursor row is drawn in reverse video.
- Typing digits selects a row by its displayed number; Enter confirms, so
  the classic "type the message number" habit works. Digits echo at the
  prompt.
- `?` shows a Help overlay for the current screen from `Keys()`.
- Resize redraws immediately. Below 80x24, every screen is replaced by a
  single centred message asking for a larger terminal until it grows back.

### 4.1 Splash

Block-character logo in yellow and cyan, "Node 1", system date and time
(dd/mm/yyyy), software version, "Press any key to log on". Any key pushes
Main Menu. If no config file exists, any key pushes New User Setup instead.

### 4.2 New User Setup

Shown only when credentials are missing. Explains in BBS voice how to
register a script app at `https://www.reddit.com/prefs/apps` and prompts
for client ID and secret as two text fields. On confirm, it tests the
credentials by fetching a token, saves `config.toml` with a default set of
areas, and replaces itself with Main Menu. On failure it shows the error
and lets the user retry or quit.

### 4.3 Main Menu

```
[M] Message areas      browse your configured subreddits
[J] Join area          type any subreddit name
[?] Help
[G] Goodbye            log off
```

`M` pushes Area List. `J` prompts for a name on the command line, then
pushes Post List for it. `G` pushes Goodbye.

### 4.4 Area List

Table of configured areas: number, name, subreddit, subscriber count and
description fetched lazily via `About` and cached. Enter or number opens
Post List. `A` adds the cursor area's subreddit to config if it came from
Join and is not yet saved. `D` removes it after confirmation.

### 4.5 Post List

Columns: number, subject, from, msgs. At 100 columns or wider, add score
and age columns. Subject truncates with `…` to fit. Stickied posts show a
`*` before the subject in yellow. Link posts show the domain in grey after
the subject when width allows. NSFW posts show `[X]` in red.

Keys: Enter or number reads (pushes Thread Index). `N` next page fetches
with `after`, `P` previous page (kept in memory). `S` cycles hot, new, top,
rising. `J` join another area. `O` opens the post's URL in the browser via
`xdg-open` (falls back to `open` on macOS). `R` refreshes, bypassing cache.

Rows per page equal the content height, so a tall terminal shows more
posts. Page size requested from Reddit is 100; the screen paginates
locally, only fetching when the user pages past what is loaded.

### 4.6 Thread Index

Top: post subject, author, score, age, comment count, and for a self post
the first lines of the body, up to a quarter of the content height. `B`
opens the full post body in the Message Reader as message 0. Below: one line per comment showing number,
score, tree connector, author and a preview of the first line of the body.
Author is green, the original poster's name is bright cyan, moderator
distinguished comments are magenta, deleted or removed comments grey.

Peek pane: the bottom third of the content area, separated by a grey rule,
shows the cursor comment's author, score, age and wrapped body, scrolled
from the top. `Tab` toggles the pane off and on. The pane is at least
5 lines tall.

Tree connectors: two columns per depth level using `├─`, `└─`, `│`. When
depth would exceed a quarter of the width, deeper comments show at the
maximum indent with a `»` marker.

`-` collapses the cursor subtree and shows `[+N hidden]` on the parent row;
`+` expands. Reddit's "more comments" stubs appear as grey rows reading
`[load N more replies]`; Enter on one fetches via `MoreChildren` and splices
the results in place.

Keys: Enter or number opens Message Reader at that comment. `S` cycles
best, top, new. `O` opens the post link. `R` refreshes.

### 4.7 Message Reader

One comment, full width:

```
Subj: <post title>
From: <author> (+score)                  Date: dd/mm/yy hh:mm
  Re: #<parent number> <parent author> · depth N · N replies
──────────────────────────────────────────────────────────────
<wrapped body>

Links: [1] https://…  [2] https://…
```

Body formatting (see `textfmt`): paragraphs separated by a blank line,
quote lines prefixed with `>` in cyan, fenced or indented code shown
indented in grey, `**bold**` rendered bold, list markers kept, inline
links replaced by their text followed by `[n]` with the URL listed at the
foot. Space and PgDn page long bodies.

Keys: `N` and `P` move depth-first through the flattened thread order.
`U` jumps to the parent. `R` shows a numbered list of direct replies to
jump to. `T` pops back to Thread Index with the cursor on this comment.
`O` opens link `[1]`; with several links it prompts for the number.

### 4.8 Help overlay

Centred box listing keys from the current screen's `Keys()` plus the global
keys. Any key closes it. Implemented as a Screen pushed on the stack that
draws the screen beneath first, then the box.

### 4.9 Goodbye

Full-screen ANSI sign-off: time online, areas visited, messages read,
posts opened, "Thanks for calling RedditBBS", then a two-second pause or
any key, then the App returns Quit. Reached from Main Menu via `G`, `Q` or
Escape after a `y/N` confirmation on the command line.

## 5. Data layer

### 5.1 Authentication

`reddit.TokenSource` holds client ID and secret. It POSTs to
`https://www.reddit.com/api/v1/access_token` with HTTP basic auth and
`grant_type=client_credentials`, stores the bearer token and its expiry
(the response's `expires_in`, typically 86400 seconds), and refreshes when
fewer than 60 seconds remain or when a request returns 401. Refresh is
serialised with a mutex so concurrent requests cause one refresh.

All API requests go to `https://oauth.reddit.com`, carry
`Authorization: bearer <token>`, the configured `User-Agent`, and
`raw_json=1` so bodies arrive unescaped.

### 5.2 Endpoints

| Store method | Request |
| --- | --- |
| Posts | `GET /r/{sub}/{sort}?limit=100&after={after}` (`top` adds `t=day`) |
| Thread | `GET /r/{sub}/comments/{id}?sort={sort}&limit=500&depth=10` |
| MoreChildren | `GET /api/morechildren?link_id=t3_{id}&children={ids}&sort={sort}&api_type=json` |
| About | `GET /r/{sub}/about` |

The Thread response is a two-element array: a listing holding the post
and a listing holding the comment forest. Comments are `t1` kinds nested
under `replies`; `more` kinds carry `children` IDs and a `count`. The
parser produces a `Thread{Post, Comments []*Comment}` tree where each
comment has `Depth`, `Parent`, `Children` and an optional `More` stub.

### 5.3 Rate limiting

Reddit returns `X-Ratelimit-Remaining`, `X-Ratelimit-Used` and
`X-Ratelimit-Reset` headers. The client records them. When remaining drops
below 5, requests wait until reset. A 429 response is retried once after
the reset interval. The status line shows "Rate limited, retrying in Ns"
while waiting.

### 5.4 Cache

In-memory map keyed by full request URL, guarded by a mutex. Listings
expire after 5 minutes, threads after 10, `About` after 1 hour. `R` on a
screen bypasses the cache for that request and replaces the entry. The
cache is not persisted; a fresh process starts empty.

### 5.5 Models

Minimal typed structs, only the fields screens use:

- `Post`: ID, Subreddit, Title, Author, Score, NumComments, CreatedUTC,
  URL, Domain, Permalink, IsSelf, SelfText, Stickied, Over18,
  Distinguished.
- `Comment`: ID, Author, Body, Score, CreatedUTC, Depth, IsSubmitter,
  Distinguished, Deleted (author or body is `[deleted]` or `[removed]`),
  Children, More (`*MoreStub{Count, IDs}`).
- `Listing`: Posts, After.
- `Subreddit`: Name, Title, Subscribers, PublicDescription, Over18.

## 6. Configuration

Path: `$XDG_CONFIG_HOME/redditbbs/config.toml`, defaulting to
`~/.config/redditbbs/config.toml`. Written with mode 0600 because it holds
the secret.

```toml
[reddit]
client_id = "…"
client_secret = "…"
user_agent = "linux:redditbbs:0.1.0 (by /u/yourname)"

[display]
peek_pane = true
default_sort = "hot"

[[areas]]
name = "Linux"
subreddit = "linux"

[[areas]]
name = "Retro Computing"
subreddit = "retrobattlestations"
```

Environment variables `REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET`
override the file, so the binary can be tried without writing a secret to
disk. A `--config` flag overrides the path. New User Setup writes a
default area list of `linux`, `programming`, `retrobattlestations` and
`commandline`.

## 7. Layout and rendering rules

- Widths are measured in terminal cells using `go-runewidth`, so East
  Asian text and emoji in titles do not break columns.
- Truncation appends `…` and never splits a wide rune.
- Word wrap breaks on spaces, hard-breaks words longer than the width,
  and preserves blank lines between paragraphs.
- Control characters and ANSI sequences in Reddit text are stripped before
  display so a comment cannot corrupt the screen.
- The title bar is always 3 lines (frame, content, frame). The hotkey bar
  and prompt are always the last 2 lines. Content height is `h - 5`.
- Numbers on list rows are right-aligned in a column wide enough for the
  largest number on the page.
- Dates in chrome are dd/mm/yy. Ages in lists are relative (`3h`, `2d`).

## 8. Error handling

| Situation | Behaviour |
| --- | --- |
| Network error or 5xx | Status line shows the error in red for that screen. Existing content stays. `R` retries |
| 401 | Refresh token once and retry; if still 401, show "Credentials rejected" and offer New User Setup |
| 403 or 404 on a subreddit | Pop back with a status message: private, banned, quarantined or not found |
| 429 | Wait per headers, retry once, then show the error |
| Malformed JSON | Log the body to `$XDG_STATE_HOME/redditbbs/last-error.json`, show "Unexpected response" |
| `xdg-open` missing | Status line shows the URL so it can be copied |
| Panic anywhere | Deferred recover in `main` restores the terminal, prints the panic and stack to stderr, exits 1 |
| Terminal too small | Placeholder screen until resized |

Errors never leave the terminal in raw mode.

## 9. Testing

- `term`: a `SimCanvas` backed by `tcell.NewSimulationScreen` exposes
  `String()` and `Cell(x, y)` for assertions.
- `widgets` and `screens`: golden tests render a screen at fixed sizes
  (80x24 and 120x40) with a fake Store returning fixture data, and compare
  a plain-text dump plus a style dump against files under `testdata`.
  Key handling tests drive `HandleKey` and assert on the returned Action
  and on cursor position.
- `reddit`: parser tests use JSON fixtures under `testdata` shaped like the
  live API (listing, thread with nested `more` stubs, deleted comments,
  about). Client tests use `httptest.Server` to check auth headers,
  `raw_json=1`, token refresh on 401, rate-limit waiting and cache
  expiry with an injected clock.
- `textfmt`: table-driven tests for wrap, truncate, markdown
  simplification, link extraction and relative time.
- `config`: round-trip load and save, env override, missing file.
- An end-to-end smoke test builds the binary and runs it against the
  simulation screen with a fake Store through the Splash, Main Menu, Post
  List, Thread Index, Message Reader and Goodbye paths.

Test-driven development applies throughout: each unit's tests are written
before its implementation.

## 10. Repository layout

```
cmd/redditbbs/main.go
internal/term/…
internal/theme/…
internal/ui/…
internal/ui/widgets/…
internal/ui/screens/…
internal/reddit/…
internal/textfmt/…
internal/config/…
internal/session/…
docs/superpowers/specs/
docs/superpowers/plans/
README.md
Makefile            (build, test, lint, run)
.golangci.yml
```

Go 1.27. Dependencies: `tcell/v2`, `go-runewidth`, `BurntSushi/toml`.
No UI framework.

## 11. Out of scope for v1, noted for later

- User OAuth login, front page, saved posts, voting, replying.
- Multi-user telnet or ssh server with nodes and "who's online".
- Modem-speed emulation.
- Theme switching (the theme package already makes this trivial).
- Search within a subreddit.
- Image preview via sixel or block art.
- Persistent read-tracking across sessions.
