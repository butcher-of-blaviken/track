# Track: Preliminary PRD

Status: draft. Domain terms are defined in [CONTEXT.md](../CONTEXT.md); decisions in [docs/adr](./adr).

## Problem

Work is spread across many tasks and interrupted constantly. Timers exist, but they don't know what you were working on, don't make you rest, and don't help you pick up where you left off. Track is a terminal app that tracks tasks, times focused work on them, enforces breaks, and captures a one-line hand-off note so resuming is cheap.

## Goals

- Track the things you're working on, with lightweight organisation (Tags) and fast fuzzy search.
- Time focused work per Task, and enforce breaks with a soft lock.
- Capture a hand-off note at the end of each session so the next session starts with context.
- Ship as a single binary via `go install` and Homebrew.
- Keep the UI cheap to redesign (see ADR-0002).

## Non-goals (v1)

- Background daemon, or a bell when the app is closed.
- Sync across machines, or multi-user features.
- Windows support (not deliberately broken, but unsupported).
- Pausing a Focus session.
- Desktop (OS-level) notifications.
- Per-Task default durations, and per-session duration overrides in the TUI.
- Hierarchical Tags or a Project concept.
- Rich reporting (see v2).

## Domain summary

See CONTEXT.md. In brief: a **Task** (Active, Done or Archived) has **Tags** and a timestamped log of **Hand-off notes**. A **Focus session** always belongs to one Task and ends completed or stopped early. A completed session starts a **Break**, a soft lock overridable with a confirmation, and overrides are recorded. An **Unfiled note** has no Task until filed.

## Functional requirements

### Tasks and Tags
- Create a Task from free text. `##tag` markers anywhere in the text create or attach Tags and are stripped from the title. Tags are case-insensitive, display with the casing of first use, and allow letters, digits, `-`, `_` and `/`. A Tag ends at whitespace.
- The list shows Active Tasks by default. A toggle includes Done and Archived. Done Tasks can be reopened.
- Task detail shows the total focused time and the note log.

### Focus sessions and Breaks
- Starting a session requires a Task, picked via the fuzzy picker, with inline creation when nothing matches.
- Sessions run for the configured length. The planned duration is recorded on the session and is never rewritten by later config or flag changes. If it differs from the current setting, the TUI shows a notice.
- Stopping early records the elapsed time, marks the session stopped early, and prompts for a Hand-off note. No Break follows. Resuming starts a fresh full-length session.
- A completed session starts a Break automatically. Every Nth Break (default N=4) is long, and only completed sessions count toward N.
- While a Break runs, starting a session requires a confirmation prompt. Confirmed overrides are recorded.
- Defaults: 30m focus, 10m short break, 20m long break.

### Hand-off notes
- Prompted at the end of every session, completed or stopped early. One line, skippable with Esc.
- Notes can be added to a Task at any time. Notes can be captured with no Task (an Unfiled note) and filed later, with the timestamp of when it was written. The unfiled count is shown persistently.
- Filing offers "create Task from this note".

### Persistence and recovery
- Timers use wall-clock timestamps persisted to disk. On launch the app reconciles: a running session resumes, and a session that ended while the app was closed shows "ended N minutes ago" and prompts for the note. Elapsed time is capped at the planned end.
- After reconciling, a clear prompt asks whether to resume the last active Task.

### Notifications
- The terminal bell rings when a Focus session ends and when a Break ends, repeating until a keypress. Defaults: every 30s, up to 10 times, both configurable.
- The end state is shown as an unmissable banner in the UI.

### Search
- One fuzzy picker component, used for filtering the list and for choosing a Task to start.
- Searches titles, Tags and Hand-off notes, weighted title > Tag > note. A result that matched on a note shows the matching note line.
- Default scope is Active Tasks, with a toggle for Done and Archived.
- Matching uses `sahilm/fuzzy` (MIT, dependency-free, verified by reading its source and a scratch experiment) as the per-field scorer, wrapped in a core-owned ranker so it can be swapped. The wrapper must:
  - Split the query on whitespace and require every token to match (AND), because the library treats spaces in a pattern as literal characters and returns nothing for `work auth`.
  - Combine field scores additively with per-field offsets (title > Tag > note), not by multiplication, because scores are unnormalised and often negative (each unmatched character costs 1), so a multiplier would invert the ranking.
  - Rank a contiguous, case-insensitive substring match above a scattered fuzzy match, because the library can pick a scattered alignment over a contiguous one (`auth` matched inside "about authentication" as a-u-t-h across two words).
  - Convert the returned byte offsets to rune offsets before highlighting.

### Reporting (Modest)
- A Today / This week view: focused time per Task and per Tag, and the count of Break overrides.

### CLI surface
- `track` opens the TUI.
- The running TUI picks up changes made by other processes (e.g. `track note`) by re-querying storage on its one-second tick. No file watcher or change notification is used.
- Subcommands: `track add "<text>"` (create a Task), `track note "<text>"` (create an Unfiled note) and `track export` (JSON/Markdown dump). Timer control is TUI-only.
- Flags `--focus-duration`, `--break-duration`, `--long-break-duration` and `--long-break-interval` apply to the whole run.
- Precedence: flags > config file > defaults. Config is TOML at the platform config dir (`config.toml`) and is optional. Keys mirror the flags, e.g. `focus_duration = "30m"` and `break_duration = "10m"`.
- In fish, an unquoted `##tag` starts a comment, so docs must show quoted arguments.

## UI

- A single main screen: a Task list plus a persistent status panel (state, countdown, unfiled count).
- Overlays: fuzzy picker (`/` or Ctrl-P), Hand-off note prompt, Break-override confirmation, launch resume prompt, and `?` help.
- Secondary views, switched by key: Task detail and Report.
- Vim-style keys (`j`/`k`, `/`, `a`, `s`, `x`, `d`).
- Built with Bubble Tea and Bubbles, as a thin adapter over the core.

## Architecture (see ADRs)

- **ADR-0001:** a single SQLite file via a pure-Go driver. `export` mitigates lock-in.
- **ADR-0002:** a UI-agnostic, pull-based core with an injected clock and a storage interface. The TUI and subcommands are adapters.

## Testability and headless operation

- The TUI must be drivable headlessly: a fixed terminal size (e.g. via flags or environment) and an injectable clock, so a full Focus, Break and Hand-off cycle can be exercised in seconds by an automated driver (tmux `send-keys` and `capture-pane`, or `teatest`) without waiting real time.
- The clock and data directory must be overridable without touching the user's real data (e.g. a `--data-dir` flag or environment variable), so test runs are isolated.
- Behaviour is tested mainly against the core with a fake clock. UI tests only cover rendering and key handling.
- Automated checks verify that the bell character was emitted. Audibility and visual design are checked by hand.

## Distribution

- macOS and Linux, amd64 and arm64.
- `go install github.com/butcher-of-blaviken/track@latest`, with `main` at the repo root and the rest under `internal/`.
- A personal Homebrew tap (`butcher-of-blaviken/homebrew-tap`), with GoReleaser building binaries, publishing releases and updating the formula on tag. Homebrew core is a later goal.
- `track --version` reads Go build info, with GoReleaser injecting the same value.

## Acceptance criteria (selected)

- With a fake clock, a session that completes while the app is closed is reconciled on next launch, with the note prompt shown.
- Stopping early records elapsed time, no Break starts, and the long-Break counter is unchanged.
- A confirmed Break override is recorded and counted in the report.
- A change made by `track note` while the TUI is open appears in the TUI on its next refresh.
- Bell repeat count and interval follow config, and a keypress silences it.
- Changing `focus_duration` never alters an existing session's recorded duration.
- The core builds and its tests run with no Bubble Tea import.
- With a fixed terminal size, an injected clock and an isolated data directory, an automated driver can run a full Focus, Break and Hand-off cycle and assert on the rendered screen.

## v2 candidates

- Rich reporting: streaks, charts, per-day heatmaps, CSV export.
- Windows support.
- Desktop notifications (behind a config flag) and possibly a background daemon.
- Per-Task default durations, and per-session duration overrides.
- Homebrew core submission.

## Milestones (proposed)

1. Core: domain model, clock, storage interface, SQLite and in-memory stores, snapshot function, tests.
2. Minimal TUI: Task list, start and stop, countdown, bell, Hand-off prompt.
3. Breaks and soft lock, reconcile-on-launch, and the resume prompt.
4. Tags, fuzzy picker, and note search.
5. Unfiled notes, Task detail, Modest report.
6. Subcommands, config and flags, packaging and release.
7. Dogfood, then redesign the UI as needed (the core stays put).

## Open questions

- Exact key bindings.
