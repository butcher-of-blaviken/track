# SQLite (pure Go) for storage

Tasks, Focus sessions, notes, Tags and Break overrides are stored in a single SQLite file using a pure-Go driver (e.g. `modernc.org/sqlite`, no CGO), in the platform data directory. The data is relational, reports (Today/This week, per-Tag totals) are plain queries, and SQLite is safe when the TUI and a subcommand run concurrently. Pure Go keeps the build a single cross-compilable binary, which `go install` and the Homebrew tap both need.

## Considered Options

- **JSON/TOML file:** human-readable and git-syncable, but whole-file rewrites, no concurrency safety, and every query is hand-rolled.
- **Markdown file per Task:** greppable and editor-friendly, but session and time data fit awkwardly and querying is slow.
- **CGO SQLite driver:** faster, but breaks trivial cross-compilation and `go install` on machines without a C toolchain.

## Consequences

Data is not hand-editable. An `export` subcommand (JSON/Markdown) mitigates lock-in.
