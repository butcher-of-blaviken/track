# Track

A terminal app for tracking the things you're working on, timing focused work on them, and leaving yourself notes about where you left off.

You keep a list of **tasks**. You start a timed **Focus session** on one of them, take a **Break** when it ends, and write a one-line **hand-off note** saying where you stopped, so the next session starts where the last one left off. Everything is stored locally in a SQLite file.

## Install

Track needs Go 1.26 or newer.

```
go install github.com/butcher-of-blaviken/track@latest
```

That puts a `track` binary in `$(go env GOBIN)`, or `~/go/bin` if that is not set; make sure it is on your `PATH`. To install or upgrade to a specific release, name its tag:

```
go install github.com/butcher-of-blaviken/track@v1.0.0
```

Check what you have with `track --version`.

## Quick start

```
track
```

Press `a`, type `write the design doc ##docs`, and press `enter` to add a task. The `##docs` becomes a tag. Press `enter` on it to start a 30 minute Focus session. When the session ends, Track rings the terminal bell and asks for a hand-off note; a Break starts on its own. Press `?` at any time to see every key for the screen you are on, and `q` to quit.

## Concepts

- **Task**: a unit of work you're tracking. It lives as long as you need and can span many days and many Focus sessions. A task is **Active** (shown by default), **Done** (hidden by default, searchable, reopenable) or **Archived** (abandoned; hidden unless you ask). Time totals and history are kept in every state.
- **Tag**: a flat label on a task, created on first use by writing `##name` in the task's text. The marker is stripped from the title. Tags are case-insensitive and shown with the casing of first use.
- **Focus session**: one timed block of work on exactly one task. It ends **completed** (ran its full length) or **stopped early** (ended by you, keeping the elapsed time). It cannot be paused; to resume work, start a new session.
- **Break**: a timed rest that starts automatically when a Focus session completes. Stopping a session early does not start one. While a Break runs, starting a new session asks you to confirm. Every few completed sessions (four by default) earns a longer Break.
- **Hand-off note**: a one-line note, asked for when a session ends (including early stops), saying where you left off. It is added to the task's log with a timestamp, and you can skip it. You can also add a note to a task at any time.
- **Unfiled note**: a note captured with no task, to be filed onto a task later.

If a session was still running when you last quit, Track offers to resume it on launch and shows the task's last note.

## Using the app

Keys are listed per screen; `?` shows the same list in the app. Letters that type text (in the add, note and find prompts) are text, not commands.

In a terminal at least 100 columns wide and 16 lines high, the list, the inbox and the detail of the task under the cursor show side by side, in three panels. The panel with a bright border has the keyboard: `i` moves to the inbox, `l` to the detail, and `esc` back. The report, the docs and the prompts take the whole screen. `v` switches to a single pane, one screen at a time, for the rest of the run; `layout` in the config file chooses which one Track starts in.

### The list

| Key | Action |
| --- | --- |
| `enter` | start a Focus session on the task under the cursor |
| `x` | stop the running session early |
| `a` | add a task |
| `n` | capture an unfiled note, to file onto a task later from the inbox |
| `d` | mark the task done |
| `D` | archive the task |
| `u` | reopen a done or archived task |
| `l` | open the task's detail: time focused, sessions and its notes |
| `r` | report: what you worked on today and this week |
| `i` | open the inbox of unfiled notes |
| `/` | find a task by title, tag or note text |
| `ctrl+p` | pick a task to start |
| `tab` | scope: Active tasks only, or every state |
| `j`/`k` | move the cursor |
| `v` | switch between the split layout and a single pane |
| `H` | docs: this README, inside the app |
| `?` | help |
| `q` | quit |

### The inbox

| Key | Action |
| --- | --- |
| `enter` | file the note onto a task |
| `c` | create a new task from the note |
| `j`/`k` | move the cursor |
| `esc` | back |
| `v` | switch between the split layout and a single pane |
| `H` | docs: this README, inside the app |
| `?` | help |
| `q` | quit |

### A task's detail

| Key | Action |
| --- | --- |
| `enter` | start a Focus session on this task |
| `n` | add a note to this task |
| `j`/`k` | scroll |
| `esc` | back |
| `v` | switch between the split layout and a single pane |
| `H` | docs: this README, inside the app |
| `?` | help |
| `q` | quit |

### The report

The report shows the day or the week by focused time per task and per tag. A day's report also lists the tasks you finished, and the notes you wrote that day under the tasks they belong to, with the unfiled ones under Notes. `[` and `]` step through earlier days; `y` copies the day shown as the same Markdown that `track report` writes (see [The daily report](#the-daily-report)). `y` asks the terminal to set the clipboard with an OSC 52 escape sequence, which most modern terminals honour, also over SSH; one that does not, or that has it switched off, ignores it, so Track can only say it sent the text, not that it arrived. Use `track report` to get the text another way.

| Key | Action |
| --- | --- |
| `tab` | switch between today and this week |
| `[`/`]` | show the day before, or the day after (up to today) |
| `y` | copy the day shown as Markdown, for a standup update |
| `j`/`k` | scroll |
| `esc` | back |
| `H` | docs: this README, inside the app |
| `?` | help |
| `q` | quit |

### The docs

`H` opens this README inside the app, from the list, the inbox, a task's detail or the report. It is shown as plain text and wraps to the window. A session keeps running, and its countdown stays on screen, while you read.

| Key | Action |
| --- | --- |
| `/` | search; every match is highlighted and the view jumps to the first |
| `n`/`N` | next and previous match |
| `j`/`k` | scroll |
| `space`/`b` | page down and up |
| `g`/`G` | top and bottom |
| `esc` | clear the search, then go back |
| `?` | help |
| `q` | quit |

### Colours and themes

By default Track uses only your terminal's own 16 colours, so it takes on whatever theme your terminal has: Focus is magenta, a Break and finished work are green, anything that needs you (a hand-off due, unfiled notes) is yellow, errors are red, and the accent for headings, the footer's keys and the focused panel's border is blue. The row under the cursor is your terminal's own foreground and background swapped, which is readable on any theme. Secondary text, such as times, and Done and Archived tasks, is faint. Colour is never the only cue: Done tasks still say `(done)` and sessions still say `completed` or `stopped early`.

On a dark terminal Track asks for the background colour and uses bright blue for the accent, because plain ANSI blue is a very dark blue on many of them. A terminal that does not answer keeps plain blue.

To get the same colours in any terminal, set `theme` in the config file to a fixed palette: `one-dark`, `gruvbox-dark`, `solarized-dark`, `one-light` or `solarized-light`. These use exact colours, and the cursor row is filled with the palette's accent. Track does not paint the terminal's background, so pick the palette that matches yours; a dark palette on a light terminal will not read well.

A palette needs a terminal with 256 colours or more. With 16 colours, with no colour at all, or with the `NO_COLOR` environment variable set, Track uses the default theme instead, so the cursor row, bold and faint text still work.

## Command line

Some things are quicker without opening the app. They use the same database, and a running app picks up what they write within a second.

```
track add "write the design doc ##docs"   # create a task; ##tags become tags
track note "ask about the retry logic"    # save an unfiled note to the inbox
track export                              # everything as JSON on stdout
track export --format markdown --output track.md
track report                              # today's work as Markdown, for a standup
track report --standup                    # yesterday and today so far
track report --day 2026-09-30 --format json
track docs                                # this README, on stdout
track version                             # also: track --version
```

The text of `add` and `note` may be several words; quote it, and put `--` before text that starts with a dash. `export` writes every task, Focus session and note in every state. `--format` is `json` (the default) or `markdown`, and `--output FILE` writes a file in one piece instead of stdout. Exit status is 0 on success, 1 when something failed and 2 for a usage mistake.

### The daily report

`track report` writes what you worked on in a day, ready to paste into a standup update, a message or a pull request:

```
## Wed 30 Sep: 2h 10m focused

### Finished
- release v1.0.0 #track

### Worked on
- **write the PRD** #docs #Q1 · 1h 25m (3 sessions)
  - left off at section 2; the retry numbers are still missing
- **call the bank**
  - they close at 5

### Notes
- ask whether the export needs a version field
```

Finished lists the tasks you marked done that day (a task finished before Track began recording when is not listed). Worked on lists each task with its focused time and the notes you wrote on it that day, hand-off notes included; a task you only wrote on appears without a time. Notes lists the unfiled notes you wrote that day. A section with nothing in it is left out.

| Flag | Meaning | Default |
| --- | --- | --- |
| `--day` | `today`, `yesterday` or a date written `YYYY-MM-DD`, in local time | `today` |
| `--standup` | yesterday and today so far; cannot be combined with `--day` | off |
| `--format`, `-f` | `markdown` (or `md`) or `json` | `markdown` |
| `--output`, `-o` | write a file, in one piece, instead of stdout | stdout |

`--format json` has a `version` and one entry per day, with the same conventions as `track export`: snake_case keys, UTC times, durations in whole seconds, and keys that are only ever added. It only reads the database, so it is safe while the app is open, and like the other subcommands it ignores the config file. Exit status is as for `export`.

### fish and `##tag`

In fish an unquoted `##tag` starts a comment, so the rest of the line is dropped. Always quote the text:

```
track add "write the design doc ##docs"     # good
track add write the design doc ##docs       # in fish: the tag is lost
```

### Flags

Flags go before the subcommand, for example `track --data-dir ~/scratch add "try it"`. The four duration flags apply to this run only and override the config file.

| Flag | Meaning | Default |
| --- | --- | --- |
| `--focus-duration` | length of a Focus session | `30m` |
| `--break-duration` | length of a short Break | `10m` |
| `--long-break-duration` | length of a long Break | `20m` |
| `--long-break-interval` | every nth completed session earns a long Break | `4` |
| `--data-dir` | directory for Track's data | the platform data directory |
| `--config` | config file to read | `config.toml` in the platform config directory |

Durations are written like `25m` or `1h30m`.

## Config file

The config file is optional. Keys it leaves out keep their defaults; an unknown key or a bad value stops Track at startup with a message naming the file and the key. Flags override the file, and the file overrides the defaults.

```toml
focus_duration = "30m"
break_duration = "10m"
long_break_duration = "20m"
long_break_interval = 4
bell_interval = "30s"
bell_repeats = 10
layout = "auto"
theme = "default"
```

`bell_interval` and `bell_repeats` say how the terminal bell repeats when a session or Break ends: once, then `bell_repeats` more times, one interval apart.

`layout` is `"auto"` or `"split"` (the panels side by side when the terminal is wide enough; the two are the same today) or `"single"` (one pane at a time, always).

`theme` is `"default"`, which uses the colours of your terminal's own theme, or a fixed palette: `"one-dark"`, `"gruvbox-dark"`, `"solarized-dark"`, `"one-light"` or `"solarized-light"`. See Colours and themes above.

Track looks for the file at `$XDG_CONFIG_HOME/track/config.toml` if that is set to an absolute path, otherwise at `~/Library/Application Support/track/config.toml` on macOS and `~/.config/track/config.toml` elsewhere. `--config FILE` names one explicitly, and then it must exist.

## Where your data lives

Everything is in one SQLite file, `track.db`, in `$XDG_DATA_HOME/track` if that is set to an absolute path, otherwise in `~/Library/Application Support/track` on macOS and `~/.local/share/track` elsewhere. Use `--data-dir DIR` to keep it somewhere else. `track export` takes your data out as JSON or Markdown, and it is safe to run while the app is open.

## Compatibility

Your data is yours, so within version 1 Track stays compatible with what you already have:

- A newer Track always opens and upgrades a database an older one wrote. Going the other way does not work: an older Track refuses a database from a newer one and tells you to upgrade.
- Config keys keep their names and meanings. New keys are optional. An unknown key is an error, so a config written for a newer Track will not load on an older one.
- Subcommands and flags keep working, and the JSON export keeps every key it has (its `version` field is `1`); new ones may be added.

If a break is ever unavoidable it will be a new major version, installed from a new path (`…/track/v2`), with a way to bring your data across. See `docs/adr/0003-backward-compatibility.md`.

## Development

```
make check   # gofmt, go vet, golangci-lint, tests, build
make e2e     # tmux end-to-end tests of the real binary (needs tmux)
```

`CONTEXT.md` defines the domain vocabulary and `AGENTS.md` describes the workflow and architecture rules. `docs/RELEASING.md` describes how to cut a release.

## License

MIT; see [LICENSE](LICENSE).
