# RedditBBS

A terminal Reddit reader that looks and feels like a 1990s bulletin board
system: hotkey menus, a command prompt, numbered messages, a threaded
comment index with a peek pane, and a one-message reader. Read-only.

![Thread index: a tree of comments with a peek pane](docs/screenshots/threadindex.png)

## Screens

| | |
| --- | --- |
| ![Logon splash](docs/screenshots/splash.png) | ![Main menu](docs/screenshots/mainmenu.png) |
| ![Message areas](docs/screenshots/arealist.png) | ![Post list](docs/screenshots/postlist.png) |
| ![Message reader](docs/screenshots/reader.png) | ![Help overlay](docs/screenshots/help.png) |
| ![Goodbye](docs/screenshots/goodbye.png) | |

The screenshots are captured from the program itself on a 100 by 30
simulated terminal with sample data; `make screenshots` regenerates them.

## Themes

Four themes are built in, all within the 16 ANSI colours so they work in
any terminal and follow your terminal's own palette. `Ctrl-T` cycles them
on any screen (except while you are typing into a field) and remembers the
choice.

| `classic` | `blue` |
| --- | --- |
| ![classic](docs/screenshots/threadindex.png) | ![blue](docs/screenshots/theme-blue.png) |

| `amber` | `green` |
| --- | --- |
| ![amber](docs/screenshots/theme-amber.png) | ![green](docs/screenshots/theme-green.png) |

Pick one in the config, or define your own by overriding roles on top of a
base theme. Each value is `<colour> [on <colour>] [bold] [reverse]` using
the ANSI names (`black`, `red`, `green`, `yellow`, `blue`, `magenta`,
`cyan`, `white`, their `bright` variants, `grey`, or `default`):

```toml
[display]
theme = "custom"

[theme]
base = "classic"
heading = "bright red"
cursor = "black on cyan"
author = "green bold"
```

Roles: `frame`, `logo`, `heading`, `subject`, `author`, `op`, `mod`, `meta`,
`body`, `quote`, `code`, `bold`, `link`, `prompt`, `hotkey`, `error`,
`stub`, `rule`, `cursor`, `sticky`, `nsfw`, `bar` (the fill behind the
title and hotkey bars). A mistake in the table is reported at startup with
the key that caused it.

## Setup

1. Build: `make build` (needs Go 1.27). The binary is `bin/redditbbs`, statically linked.
2. Register for Reddit API access first: submit a Data Access Request at
   https://support.reddithelp.com/hc/en-us/requests/new?ticket_form_id=14868593862164
   (role "I'm a developer", building an app outside Devvit). Reddit will
   refuse to create an app for an account that has not done this. Once
   approved, register a free "script" app at https://old.reddit.com/prefs/apps
   (type *script*, any redirect URI, and the name must not contain "reddit").
   Every user of this program needs their own app and credentials; none are
   shipped with it.
3. Run `./bin/redditbbs`. The New User Setup screen asks for the client ID
   and secret, checks them against Reddit, and saves them to
   `~/.config/redditbbs/config.toml` with mode 0600.

No credentials yet? `./bin/redditbbs --demo` runs the whole interface
against built-in sample data so you can try every screen and theme.

`REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET` override the file and
are never written to disk. `--config PATH` uses another file. `--debug`
saves the last unparseable API response to
`~/.local/state/redditbbs/last-error.json`.

## Keys

Everywhere: `?` help, `Q` or `Esc` back, arrows and PgUp/PgDn move the
cursor, type a number and `Enter` to select a row, `Ctrl-T` next theme,
`Ctrl-L` redraw, `Ctrl-C` quit.

| Screen | Keys |
| --- | --- |
| Main Menu | `M` message areas, `J` join any subreddit, `G` goodbye |
| Area List | `Enter` open, `D` delete |
| Post List | `Enter` read, `N`/`P` page, `S` sort, `J` join, `A` add area, `O` open link, `R` refresh |
| Thread Index | `Enter` read, `B` post body, `-`/`+` fold, `Tab` peek pane, `Space` scroll peek, `S` sort, `O` open link, `R` refresh |
| Message Reader | `N`/`P` next/previous, `U` parent, `R` replies, `T` back to index, `O` open link, `Space` page |

## Configuration


## Licence

MIT. See `LICENSE`.
