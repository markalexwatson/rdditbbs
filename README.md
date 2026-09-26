# RedditBBS

A terminal Reddit reader that looks and feels like a 1990s bulletin board
system: hotkey menus, a command prompt, numbered messages, a threaded
comment index with a peek pane, and a one-message reader. Read-only.

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

`REDDITBBS_CLIENT_ID` and `REDDITBBS_CLIENT_SECRET` override the file and
are never written to disk. `--config PATH` uses another file. `--debug`
saves the last unparseable API response to
`~/.local/state/redditbbs/last-error.json`.

## Keys

Everywhere: `?` help, `Q` or `Esc` back, arrows and PgUp/PgDn move the
cursor, type a number and `Enter` to select a row, `Ctrl-L` redraw,
`Ctrl-C` quit.

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
