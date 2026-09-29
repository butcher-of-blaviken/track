# Handoff: E3 (Breaks and recovery in the TUI)

Written at the end of E2. Read this, then `AGENTS.md`, `CONTEXT.md` and `docs/PRD.md`. Nothing here repeats them.

## State

- `main` is at `4ca16d2`. E1 (core) and E2 (minimal TUI) are closed. The next work is epic #3 with tickets #24, #25, #26 (bodies are one line each; the PRD sections "Focus sessions and Breaks", "Persistence and recovery" and "Hand-off notes" are the spec).
- Suggested order: #24, then #25, then #26. Confirm with the user.

## How the user wants to work (follow this per ticket)

1. Sync `main` (`git checkout main && git pull --ff-only && git fetch --prune`) before branching `<type>/<issue>-<slug>`.
2. Read the ticket and propose the design **and the test seams** to the user. Wait for approval before writing any test or code. The user answers with a short "approved" and sometimes trims scope ("keep it simple for v1").
3. Red, then green. Model tests first, then a tmux e2e test where behaviour is user-visible.
4. Run `make check`, `make e2e` and `go test -race ./internal/...`, commit with Conventional Commits, push, open a PR with `gh pr create` and wait for CI (`gh pr checks`).
5. Report, and do not merge until told. The user reviews by running `make run`.
6. No attribution lines in commits or PR bodies. Use the scratchpad dir for temp files, not `/tmp`.

## Facts that are not obvious from the code

- The core already does most of E3. `core.StartOptions{OverrideBreak: true}` starts during a Break and records `FocusSession.SkippedBreak` (the Break time skipped). Without it, `StartSession` returns `ErrBreakActive`. The TUI never passes it yet (`internal/tui/model.go`, `start`). The report needs the override count later (E5 #33).
- `describeSession` in `model.go` currently turns `ErrBreakActive` into a notice. #24 replaces that path with a confirmation.
- **The hand-off prompt has the keyboard.** After a completed session, the Break and the hand-off prompt run together. While the prompt is open, `s` and Enter are typed as text, so a user cannot start a session during the Break until they save or skip the note. Decide in #24's design how a start-during-Break request reaches the confirmation (for example, skipping or saving first, or a key the prompt does not consume) and confirm it with the user.
- The TUI has three modes (`modeList`, `modeAdd`, `modeHandoff`) sharing one Bubbles `textinput` and `promptErr`. A confirmation overlay is a natural fourth mode. `syncHandoff` opens the hand-off prompt only from `modeList`, and never takes the keyboard from another prompt.
- Reconcile on launch mostly exists already, because the snapshot is derived from the store and the clock. A session that ended while closed shows "ended N ago", rings once, and opens the hand-off prompt (tests: `TestHandoff_OpensOnLaunchForASessionThatEndedWhileAway`, `TestBell_ReopeningJustAfterTheEndRingsOnceAndShowsTheBanner`). #25's remaining work is the "resume the last active Task?" prompt after that. The user asked (in conversation) that it also show the last hand-off note, so the resume is informed. Notes are not displayed anywhere in the TUI yet. `Tracker.TaskNotes` exists in core.
- #26 needs the configured focus duration and the session's recorded `PlannedDuration`. `WithFocusDuration` exists on the model, but flags and config are E6 (#34, #35), and the user deferred the flags ("no need for the flags yet"). The current default is 30m in `main.go`.
- Bell state is UI-owned (`ringState` in `model.go`). Any key acknowledges the bell and is still handled.

## Testing notes

- Model tests use `newRig`, `booted`, `send`, `press`, `typeText`, `screen` and `wantScreen` (`internal/tui/*_test.go`), with a fake clock and `WithTick(noTick)`. `handoff_test.go` and `bell_test.go` show the patterns.
- e2e helpers are in `e2e/tmux_test.go`. Use `TRACK_TIME_SCALE=120` (30m focus is about 15s, a 10m Break about 5s) and `--data-dir`. Assert on labels only.
- **tmux gotcha:** an Esc followed at once by another key is read as Alt+key. Use `skipHandoff(tm)` (or wait for the screen to change) between them.
- BSD `sed` on macOS needs `sed -i ''`. A multi-line Python replace silently misses if indentation differs, so check with `git diff` or `grep`.
- The auto-mode permission check can fail transiently. Retry once, then stop and tell the user; they can run the command with `! <cmd>`.

## Open follow-ups (not scheduled)

- The `?` full-help overlay, e2e in CI (needs tmux on the runner), and a README note that the bell only sounds if the terminal enables it (comment on #42).
- Later epics: E4 search (#27-#30), E5 notes views and report (#31-#33), E6 config, flags and subcommands (#34-#38), E7 release (#39-#42).

## Suggested skills

- `tdd` for the red/green loop and for agreeing seams before any test.
- `grill-with-docs` if a design question touches the domain terms (Break override, resume) and `CONTEXT.md` needs updating.
- `code-review` before opening each PR.
