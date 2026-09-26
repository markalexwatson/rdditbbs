# RedditBBS design

Date: 26/09/2026
Status: revision 4, reconciled with the implementation plan

## 1. Purpose

RedditBBS is a terminal Reddit reader that looks and behaves like a classic
1990s bulletin board system: menu driven, single-key commands, a command
prompt at the bottom, whole-screen redraws, 16-colour ANSI styling.

It serves four goals at once, in this priority order when they conflict:

1. A daily driver for reading Reddit, so it must be reliable and quick.
2. Nostalgia and fun: the BBS feel is the point, not a skin.
3. Shareable: a single static binary others can run. The UI draws through
   an abstraction so a multi-user server can be attempted later, though
   that needs a transport adapter that is out of scope here.
4. A clean, well-structured Go codebase worth showing.

The first version is read-only. No login, voting, posting or search.

## 2. Decisions already made

| Topic | Decision |
| --- | --- |
| Language and terminal library | Go 1.27, `github.com/gdamore/tcell/v2` |
| Interaction model | Classic hotkey menus plus a cursor row moved by arrow keys; Enter opens the cursor row |
| Colour scheme | Scheme A: cyan frames, yellow headings, bright white subjects, green authors, grey chrome, inverse-video cursor row |
| Screen size | Fill the real terminal. Lists show as many rows as fit. Minimum 80x24 |
| Comment view | Threaded index with tree connectors and a peek pane, Enter opens a one-message reader |
| Reddit access | App-only OAuth (client-credentials grant) with a script app the user registers once |
| Deployment | Local CLI only in v1 |
| BBS flavour in v1 | ANSI welcome splash, main menu with named areas, goodbye screen with session stats |
| Not in v1 | Modem-speed emulation, user login, voting, posting, search, inline images, theme switching, multi-user server |

### Why app-only OAuth

On 26/09/2026 Reddit returned HTTP 403 for `www.reddit.com/r/linux/hot.json`
and `api.reddit.com` from a residential UK IP with a compliant User-Agent,
and `old.reddit.com` redirected the JSON URL to a login page. Only the RSS
feeds answered, and they lack scores, comment counts and threading.

The client-credentials grant gives read access to permitted public content
without any personal login. Reddit's Responsible Builder Policy means a
registered app may also need approval before API calls succeed, so setup
verifies a real listing fetch rather than only token issuance. The
documented allowance is about 100 queries per minute per OAuth client ID,
shared across every process using the same credentials. App-only tokens
cannot see anything requiring a user, and access to mature content may be
restricted.

## 3. Architecture

One Go module, `github.com/markalexwatson/redditbbs`, with a single binary at
`cmd/redditbbs`. Internal packages, each with one job:

| Package | Responsibility | Depends on |
| --- | --- | --- |
| `internal/term` | `Canvas` and `Terminal` interfaces, `Style`, key and event types, tcell implementation and a simulation implementation for tests. The only package that imports tcell | tcell, uniseg |
| `internal/theme` | Named colour roles mapped to `term.Style`. Nothing else names a colour | term |
| `internal/textfmt` | Markdown subset to lines of typed spans (text, quote, code, bold, link), grapheme-aware width, wrap, truncate, link extraction, relative time, score formatting | uniseg |
| `internal/ui` | `App` loop, `Screen` interface, actions, async requests, overlay composition, undersized-terminal override | term, theme |
| `internal/ui/widgets` | Drawing helpers: title bar, table with cursor row, styled text box with scrolling, hotkey bar, prompt and status line, text input, tree connectors | term, theme, textfmt |
| `internal/ui/screens` | One file per screen | ui, widgets, reddit (via Store), config, session |
| `internal/reddit` | Token source, HTTP client, rate gate, typed models, parsers, cache | net/http |
| `internal/config` | Load and save `config.toml`, environment overlay | BurntSushi/toml |
| `internal/session` | Start time and counters for the goodbye screen | none |
| `internal/browser` | Open an HTTPS URL with the OS opener | os/exec |

Dependencies point downward only. `screens` is the only package that knows
about both UI and data.

### 3.1 Canvas

```go
type Canvas interface {
    Size() (w, h int)
    // Put draws one grapheme cluster at x,y. Returns the cells it occupies (1 or 2).
    Put(x, y int, cluster string, st Style) int
    // Text draws s from x,y, clipping at maxWidth cells. Returns cells used.
    Text(x, y int, s string, st Style, maxWidth int) int
    Fill(x, y, w, h int, r rune, st Style)
    ShowCursor(x, y int)
    HideCursor()
}
```

`Terminal` extends `Canvas` with `Events() <-chan Event`, `Show()`,
`Sync()`, `Clear()` and `Fini()`, and is what App drives; the tcell and
simulation implementations both satisfy it.

`Style` is `term`'s own value type (foreground, background, bold,
reverse) so no other package imports tcell. tcell is pinned in `go.mod`
to the version the tests were written against. The tcell implementation wraps
a `tcell.Screen` and splits text into grapheme clusters with `uniseg`,
passing the first rune as the main rune and the rest as combining runes to
`SetContent`. Width is measured per cluster with `uniseg`, and clipping and wrapping
operate on clusters, so a cluster is never split. Terminals differ in how
they render some emoji sequences; the guarantee is only that our column
arithmetic and tcell's agree.

The abstraction exists as a test seam and to keep tcell out of the UI
code. It does not by itself make a telnet or ssh server possible; that
would need a `tcell.Tty` adapter handling terminal negotiation, sizing and
resize notification, and is deferred.

### 3.2 App loop

```go
type Screen interface {
    Init() Action              // once per instance, when first pushed
    Draw(c Canvas)
    HandleKey(k Key) Action
    Update(msg Msg) Action     // async results, resize, pop results, timers
    Title() string
    Keys() []KeyHelp           // for the hotkey bar and help overlay
}

// Action is one of:
//   nil, Push{Screen}, Pop{Result any}, Replace{Screen}, Quit{},
//   Run{Fn func(ctx context.Context) Msg}, Batch{Actions []Action}.
type Action interface{}
```

`App` owns the screen stack, a `term.Terminal`, one `results` channel and
a `done` channel closed at shutdown. The loop selects on
`Terminal.Events()` and `results`; workers send with a select on `done` so
they never block after shutdown. Each iteration:
translate the event into a `Key`, `Resize` or async `Msg`, dispatch, apply
the returned Action, then clear the buffer, call `Draw` on the visible
screens, and `Show()`. Ctrl-L calls `Sync()` to repair a corrupted
terminal.

Async work: a screen returns `Run{Fn}`. App binds the Run to the screen
instance that returned it, creates a context cancelled when that instance
is popped, and runs `Fn` in a goroutine with a deferred recover that
converts a panic into an error `Msg`. The returned `Msg` is sent on
`results`; App delivers it to that instance's `Update` if it is still on
the stack, otherwise drops it. Instances hold a `generation` counter they
bump on sort change or refresh and embed in each request; `Update` ignores
results from an older generation. Worker functions receive copies of what
they need and never touch screen state.

Order of applying a `Batch`: navigation actions first, then `Run`s are
bound to whichever instance returned the Batch, so a screen may `Push` a
new screen whose own `Init` returns the `Run`. If the Batch removed that
instance from the stack, its `Run`s are discarded without running. A `Run`
returned by a screen that is not on top is still bound to that screen.

`Pop{Result}` removes the top screen and delivers `PopResult{Result}` to
the new top screen's `Update`. `Result` is `any`; each producing screen
documents its concrete type, for example Message Reader returns
`SelectComment{ID string}` to Thread Index.

Covered screens do not receive keys. They do receive `Update` for their
own async results and `Resize`. A navigation Action returned from a
covered screen's `Update` is ignored, except `Quit`.

Timers: a screen wanting a delay returns `Run` with a function that sleeps
on the context and returns a `Tick` message. Cancellation on pop means no
stray ticks.

Overlays: the Help screen sets `Overlay() bool` (an optional interface).
App draws the screen beneath first, then the overlay. App constructs Help
itself when `?` is pressed, passing the top screen's `Keys()` and
`Title()`.

Undersized terminal: when the size is below 80x24, App keeps the stack
intact, draws a centred "Please enlarge your terminal to 80x24" message
instead, and ignores keys other than Ctrl-C until a `Resize` restores the
minimum.

Key precedence: Ctrl-C quits at once; then an active text input or
confirmation prompt receives every key literally; then an overlay on top
of the stack; then the global keys `?` (Help) and Ctrl-L; then the top
screen's `HandleKey`. Hotkeys are case-insensitive outside text entry.

### 3.3 Store interface

```go
type Fetch struct{ Fresh bool } // Fresh bypasses and replaces the cache entry

type Store interface {
    Posts(ctx context.Context, subreddit string, sort Sort, after string, f Fetch) (Listing, error)
    Thread(ctx context.Context, subreddit, postID string, sort CommentSort, f Fetch) (Thread, error)
    Subtree(ctx context.Context, subreddit, postID, commentID string, sort CommentSort) (Thread, error)
    MoreChildren(ctx context.Context, linkFullname string, ids []string, sort CommentSort) (Things, error)
}

// Things is the flat morechildren result: comments plus any nested stubs,
// attached into the tree by reddit.Attach.
type Things struct{ Comments []*Comment; Stubs []*MoreStub }
```

`reddit.Client` implements it. Tests supply a fake. The client parses a
fresh value from cached bytes on every call, and the thread model
deep-copies what it receives, so no screen ever mutates a value another
holder can see.

## 4. Screens

### 4.1 Common layout

Every screen at height `h` and width `w`:

| Rows | Content |
| --- | --- |
| 0 to 2 | Title bar: frame line, content line (logo, screen title, area, position), frame line |
| 3 to h-3 | Screen content, `h-5` rows |
| h-2 | Hotkey bar from `Keys()` |
| h-1 | Prompt and status line: `Command:` and the typed digits on the left, status text (Retrieving..., errors, rate-limit countdown) on the right |

Tables consume their heading and rule from the content rows, so a table on
an 80x24 screen has 17 data rows.

Global behaviour:

- `Q` and Escape return `Pop`. On the Main Menu they behave like `G`.
- Arrow keys, PgUp, PgDn, Home, End move the cursor on any list. The cursor
  row is drawn in reverse video and clamped after resize or data change.
- Typing digits starts numeric selection, echoed at the prompt. Enter
  confirms, Escape cancels, Backspace edits. A number with no matching row
  shows "No such message" in the status line. Selection is by displayed
  number; internal selection tracks the item's Reddit ID so a refresh or
  sort change re-selects the same item when it is still present.
- Text inputs (subreddit name, credentials, confirmations) take keys
  literally, support Backspace, Left and Right, and bracketed paste. Enter
  submits, Escape cancels. The client secret field masks with `*`.
  Subreddit names are validated against `^[A-Za-z0-9_]{2,21}$`.
- `?` pushes the Help overlay.
- Dates in chrome are dd/mm/yy. Ages in lists are relative (`3h`, `2d`).

### 4.2 Splash

Block-character logo in yellow and cyan, "Node 1", system date and time,
software version, "Press any key to log on". Any key pushes Main Menu, or
New User Setup when effective credentials are incomplete (see section 6).

### 4.3 New User Setup

Explains in BBS voice how to register a script app at
`https://www.reddit.com/prefs/apps`, that Reddit may require approval of
the app, and prompts for client ID and secret. On submit it obtains a
token and then fetches `Posts("linux", Hot, "")` to prove real access. On
success it saves the credentials into `config.toml`, adds the default
areas only when the config has no areas at all, and replaces itself with
Main Menu. Reopening Setup after a 401 therefore never disturbs existing
areas. On failure it shows the error text and lets the user edit
and retry or quit.

### 4.4 Main Menu

```
[M] Message areas      browse your configured subreddits
[J] Join area          type any subreddit name
[?] Help
[G] Goodbye            log off
```

`M` pushes Area List. `J` prompts for a name, then pushes Post List for
it. `G`, `Q` and Escape all ask `Log off? (y/N)` on the prompt line; `y`
pushes Goodbye.

### 4.5 Area List

Table of configured areas: number, name, subreddit. No network calls.
Enter or number pushes Post List. `D` removes the cursor area after a
`y/N` confirmation and saves the config.

### 4.6 Post List

Columns: number, subject, from, msgs. At 100 columns or wider, add score
and age. Subject truncates with `…`. Stickied posts show `*` before the
subject in yellow. Link posts show the domain in grey after the subject
when width allows. NSFW posts show `[X]` in red.

Post numbers are page-local, 1 to the number of data rows on the screen,
which is the content height minus the two heading rows. The client
requests 100 posts per Reddit page and the screen paginates locally in
blocks of that data-row count; the last local page may be partially
filled. `N` moves to the next local page,
fetching the next Reddit page with `after` when the loaded posts are
exhausted and `after` is non-null; at the true end, or when Reddit returns
the same `after` cursor twice, the status line says "End of messages". `P` moves to the previous local page. PgDn and PgUp
move the cursor by a page within loaded posts and follow the same
fetching rule. A resize re-paginates so that the selected post stays
visible. Posts with an ID already loaded are skipped when a new page
arrives.

Keys: Enter or number pushes Thread Index. `S` cycles hot, new, top (day),
rising; this bumps the generation and starts a fetch while the current
posts stay on screen with "Retrieving..." in the status line. When the new
listing arrives it replaces the posts and resets the cursor; on failure
the old posts remain with the error shown. `J` joins another area (replaces this screen). `A` saves this area
to config when it came from Join and is not yet saved. `O` opens the
post's URL in the browser. `R` refetches the first page with `Fresh` and
discards later pages.

### 4.7 Thread Index

Row budget within the content area, top to bottom: post header of 2 rows
plus, for a self post, body preview rows up to a quarter of the content
height; a rule; the comment table with its heading row; a rule; the peek
pane. The comment table gets whatever remains and never fewer than 5 data
rows, shrinking the body preview first and then the peek pane to achieve
that. `B` opens the full post body in Message Reader as message 0.

Below: one row per visible comment: thread-local number, score, tree
connector, author, first line of the body as a preview. Numbers are
assigned in display order over loaded comments and are reassigned after
sort, refresh, expansion or collapse; the selected comment is tracked by
ID. Authors are green, the original poster bright cyan, moderator
distinguished comments magenta. A comment whose author is deleted shows
`[deleted]` in grey as the author but keeps its body. A comment whose body
is removed or deleted shows `[removed]` or `[deleted]` in grey as the
preview.

Peek pane: a third of the content area, at least 5 rows, showing the
selected comment's author, score, age and wrapped body. Space scrolls the
pane. `Tab` hides or shows it; the comment table takes the freed rows.

Tree connectors use two columns per depth with `├─`, `└─` and `│`. Beyond
a depth of `w/8` levels the row stays at the maximum indent with a `»`
marker.

`-` collapses the selected subtree; the parent row shows `[+N hidden]`
counting loaded descendants. `+` expands. Collapse state is UI state keyed
by comment ID.

Stubs: a `more` object with a non-zero count appears as a grey row
`[load N more replies]`; Enter fetches via `MoreChildren` in batches of up
to 100 IDs, sequentially, and inserts the results by `parent_id`. A `more`
object with count 0 (Reddit's "continue this thread") appears as
`[continue this thread]`; Enter calls `Subtree` for its parent comment and
replaces that comment's subtree with the result.

Keys: Enter or number pushes Message Reader for that comment. `S` cycles
best, top, new (best is sent to the API as `confidence`); this bumps the
generation and refetches. `O` opens the post link. `R` refetches with
`Fresh`, keeping collapse state for IDs that still exist.

### 4.8 Message Reader

One comment full width:

```
Subj: <post title>
From: <author> (+score)                  Date: dd/mm/yy hh:mm
  Re: #<parent number> <parent author> · depth N · N loaded replies
──────────────────────────────────────────────────────────────
<styled, wrapped body>

Links: [1] https://…  [2] https://…
```

Message 0 is the post itself: `From` is the post author and `Re:` is
omitted.

Traversal order is message 0 followed by the Thread Index's visible
order: loaded comments, excluding collapsed descendants and stubs. `N` and
`P` move through it and stop at the ends with "No more messages". `U`
jumps to the parent; at depth 0 it goes to message 0, and at message 0 it
says "Already at top". `R` shows a numbered list of loaded direct replies
to jump to; at message 0 these are the top-level comments. `T` returns `Pop{Result: commentID}` so Thread Index
re-selects this comment. Space and PgDn page long bodies. `O` opens link
`[1]`; with several links it prompts for the number.

### 4.9 Help overlay

Centred box listing the current screen's `Keys()` plus the global keys.
Any key pops it.

### 4.10 Goodbye

Full-screen ANSI sign-off: time online, areas visited, messages read,
posts opened, "Thanks for calling RedditBBS". After a two-second `Tick`
or any key it returns `Quit`.

## 5. Text formatting

`textfmt` turns a Reddit Markdown body into `[]Line`, each a slice of
`Span{Text string; Kind Kind}` (Text, Quote, Code, Bold, Link), already
wrapped to a given width; `widgets` maps each Kind to a theme role so
`textfmt` stays free of theme knowledge.
Supported subset:

- Paragraphs separated by blank lines. Single newlines inside a paragraph
  are soft breaks.
- Lines starting with `>` become quote spans (cyan), with the `>` kept.
- Fenced ``` blocks and 4-space indented blocks become code spans (grey),
  indented two cells, never re-wrapped, clipped at the width.
- `**bold**` and `__bold__` become bold spans. Other emphasis markers are
  left as text.
- List markers `* `, `- ` and `1. ` are kept.
- `[text](url)` becomes `text[n]` and the URL is added to the links list.
  Bare `http(s)://` URLs are left in place and also added to the links
  list. Relative Reddit links such as `/r/linux` are resolved against
  `https://www.reddit.com`.
- Tabs expand to 4 spaces. Control characters and escape sequences are
  removed. Newlines are preserved.

Wrapping breaks on spaces, hard-breaks words wider than the width, and
measures in grapheme clusters via `uniseg`. Truncation appends `…` and
never splits a cluster.

## 6. Configuration

Path: `$XDG_CONFIG_HOME/redditbbs/config.toml`, defaulting to
`~/.config/redditbbs/config.toml`. `--config` overrides the path.

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
```

Loading: apply defaults, read the file if it exists (a malformed file is
an error shown at startup with the path and line, then exit 2), then
overlay `REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET` from the
environment. Effective credentials are complete when both values are
non-empty after the overlay; otherwise Splash leads to New User Setup.

Saving: the file is written atomically (temp file in the same directory,
fsync, rename) with mode 0600, and the directory is created with 0700.
If an existing file is more permissive than 0600 its mode is tightened on
save. Values that came only from the environment are not written to the
file. Save failures show in the status line and do not lose the in-memory
change.

New User Setup writes default areas `linux`, `programming`,
`retrobattlestations` and `commandline`.

## 7. Data layer

### 7.1 Authentication

`TokenSource` POSTs to `https://www.reddit.com/api/v1/access_token` with
HTTP basic auth (client ID and secret) and `grant_type=client_credentials`,
and stores the bearer token with its expiry from `expires_in`. A token is
refreshed when fewer than 60 seconds remain, or when a request returns 401
with that token. Refresh is guarded by a mutex; a caller that arrives with
a rejected token re-checks under the lock whether the current token
already differs and, if so, reuses it instead of refreshing again.

Every API request goes to `https://oauth.reddit.com`, carries
`Authorization: bearer <token>`, the configured `User-Agent` and
`raw_json=1`. The HTTP client has a 15-second timeout and responses are
read through a 10 MB limit.

### 7.2 Endpoints

| Store method | Request |
| --- | --- |
| Posts | `GET /r/{sub}/{sort}?limit=100&after={after}` (`top` adds `t=day`) |
| Thread | `GET /r/{sub}/comments/{id}?sort={apiSort}&limit=500&depth=10` |
| Subtree | `GET /r/{sub}/comments/{id}?comment={commentID}&context=0&sort={apiSort}&limit=500&depth=10` |
| MoreChildren | `GET /api/morechildren?link_id={linkFullname}&children={ids}&sort={apiSort}&api_type=json`, at most 100 IDs per call, never more than one call in flight. `children` is bare comment IDs joined by commas; `linkFullname` is normalised to exactly one `t3_` prefix |

`apiSort` maps best to `confidence`; top and new are passed through.

The Thread response is a two-element array: a listing holding the post
and a listing holding the comment forest. Comments are kind `t1` nested
under `replies`, which is either a listing object or an empty string when
there are none. Kind `more` carries `count`, `children` and `parent_id`.
Unknown kinds are skipped. The parser produces a `Thread{Post *Post;
Comments []*Comment}` tree.

The MoreChildren response is `{"json": {"errors": [...], "data":
{"things": [...]}}}`. Non-empty `errors` is an error. Things are flat and
may include further `more` stubs; they are attached using `parent_id`,
which is `t1_<id>` for a comment parent or `t3_<id>` for the post.
Attachment is two-pass: index every returned thing by fullname first, then
attach each to a parent that is either already loaded or in the response,
so order within the response does not matter. Only things whose parent is
in neither set are dropped. Things whose ID is already loaded
are ignored. IDs that the response did not return remain in the stub so
the row can be retried.

### 7.3 Rate limiting

A `RateGate` records `X-Ratelimit-Remaining`, `X-Ratelimit-Used` and
`X-Ratelimit-Reset` after every response and reserves one unit per
in-flight request. When remaining minus reservations is below 2, callers
wait on the context until reset. A 429 waits for `Retry-After` when
present, else until reset, then retries once. While waiting, the client
reports progress through a callback so the status line can show "Rate
limited, retrying in Ns". If headers are absent the gate assumes 60
requests per minute.

### 7.4 Cache

In-memory map keyed by full request URL. Listings expire after 5 minutes,
threads and subtrees after 10 minutes. Capacity is 200 entries with
least-recently-used eviction. `Fetch{Fresh: true}` bypasses and replaces
an entry. Cached values are never mutated after insertion.

### 7.5 Models

- `Post`: ID, Fullname, Subreddit, Title, Author, Score, NumComments,
  CreatedUTC, URL, Domain, Permalink, IsSelf, SelfText, Stickied, Over18,
  Distinguished.
- `Comment`: ID, Fullname, ParentFullname, Author, Body, Score,
  CreatedUTC, Depth, IsSubmitter, Distinguished, AuthorDeleted (author is
  `[deleted]`), BodyRemoved (body is `[deleted]` or `[removed]`), Children
  `[]*Comment`, More `*MoreStub`.
- `MoreStub`: ParentFullname, Count, IDs.
- `Listing`: Posts, After (empty at the end).

## 8. Error handling

| Situation | Behaviour |
| --- | --- |
| Network error or 5xx | Status line shows a short error in red. Existing content stays. `R` retries |
| 401 | Refresh token once and retry; if still 401, status "Credentials rejected. Press L to log in again"; `L` pushes New User Setup in nested mode, which pops back on success and the screen refetches |
| 403 | Status "Access denied" plus Reddit's `reason` field when present (private, quarantined, gated). Screen stays |
| 404 | Status "No such area" on Post List; the screen stays so `J` can try again |
| 429 | Wait per section 7.3, retry once, then show the error |
| Malformed JSON | Status "Unexpected response from Reddit". With `--debug`, the first 64 KB of the body is written to `$XDG_STATE_HOME/redditbbs/last-error.json` (directory 0700, file 0600) |
| Browser opener missing or failing | Status line shows the URL so it can be copied |
| Panic in a worker | Recovered in the worker, delivered as an error `Msg` |
| Panic on the event loop | Deferred recover in `main` restores the terminal, prints the panic and stack to stderr, exits 1 |
| Terminal too small | App-level override until resized |

Terminal cleanup runs from one place in `main` on every exit path.

`browser.Open` accepts only `http` and `https` URLs, runs `xdg-open` on
Linux and `open` on macOS as an argument array without a shell, returns
after `Start` so the UI is not blocked, and reaps the process with `Wait`
in a goroutine, reporting a non-zero exit through a callback that the
status line shows.

## 9. Testing

Test-driven development applies throughout: each unit's tests are written
before its implementation.

- `term`: `SimCanvas` wraps `tcell.NewSimulationScreen` and exposes
  `String()` and `CellAt(x, y)` for assertions on text and style.
- `textfmt`: table-driven tests for wrap, truncate, grapheme width,
  Markdown subset, link extraction and relative time.
- `reddit`: parser tests on JSON fixtures shaped like the live API,
  covering a listing, a thread with nested stubs, `replies: ""`, a
  count-0 stub, deleted comments, and a morechildren response. Client
  tests use `httptest.Server` to check headers, `raw_json=1`, token
  refresh on 401 with the double-refresh guard, rate gate waiting and 429
  retry, cache expiry and eviction, all with an injected clock.
- `ui`: App tests with a `SimCanvas`, fake screens and an injected clock
  covering action ordering, Run binding and cancellation on pop, stale
  generation dropping, PopResult delivery, overlay drawing, key
  precedence and the undersized-terminal override.
- `widgets` and `screens`: screen tests render each screen at 80x24 into
  the simulated terminal with a fake Store and assert on text and on the
  style of specific cells (no golden files, which rot). Key tests drive
  the App with key sequences and assert on the Action, on navigation and
  on selection. Resize tests check the row arithmetic after shrinking.
- Smoke tests: an in-process test constructs App with the simulated
  terminal, a fake Store and a fixed clock, feeds a key script through
  Splash, Main Menu, Area List, Post List, Thread Index, Message Reader
  and Goodbye, and asserts on the canvas at each step; a second test runs
  the real `App.Run` loop with injected events and checks it exits on
  Ctrl-C.

## 10. Repository layout

```
cmd/redditbbs/main.go
internal/term/
internal/theme/
internal/textfmt/
internal/ui/
internal/ui/widgets/
internal/ui/screens/
internal/reddit/
internal/config/
internal/session/
internal/browser/
docs/superpowers/specs/
docs/superpowers/plans/
README.md
Makefile            (build, test, lint, run)
.golangci.yml
```

Dependencies: `tcell/v2`, `rivo/uniseg`, `BurntSushi/toml`. No UI
framework.

## 11. Out of scope for v1, noted for later

- User OAuth login, front page, saved posts, voting, replying.
- Multi-user telnet or ssh server; needs a `tcell.Tty` transport adapter.
- Modem-speed emulation.
- Theme switching.
- Search within a subreddit.
- Image preview via sixel or block art.
- Persistent read-tracking across sessions.
- Subreddit metadata (subscriber counts, descriptions) in Area List.
