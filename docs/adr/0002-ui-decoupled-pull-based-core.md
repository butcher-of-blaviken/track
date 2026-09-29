# UI-decoupled, pull-based core

All domain logic (Tasks, Tags, Focus sessions, Breaks, the soft lock, tag parsing, search ranking, reporting) lives in a core package with no Bubble Tea imports. The TUI and the non-interactive subcommands are adapters over it. The UI is expected to change substantially after first use, so it must be cheap to rewrite without touching the domain.

The core is **pull-based**: it exposes a pure snapshot of state given "now" (Idle, Focus with time remaining, Break, awaiting Hand-off note, bell due), and runs no background goroutines. The UI ticks and renders. This is the same code path as reconcile-on-launch, and bell repeats are derived from the end time, `now` and acknowledged bells. Time is injected via a clock interface; storage sits behind a small interface with the SQLite implementation and an in-memory fake.

## Considered Options

- **Logic inside Bubble Tea models:** the shortest path to a working app, but couples the domain to one UI and makes redesigns expensive.
- **Push (core-owned timer goroutine emitting events):** adds concurrency and lifecycle to the core, and the UI can miss or double-handle events. It can be layered on later if an adapter needs it.

## Consequences

Some indirection for a small app. Timer, Break and bell behaviour are testable with a fake clock and no sleeping.
