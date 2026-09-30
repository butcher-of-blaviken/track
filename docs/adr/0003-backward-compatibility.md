# Backward compatibility of the data, config and commands

Track keeps a person's own history, so a new version must always open, read and upgrade what an older version wrote. Within v1 (and any later major version) we keep four things compatible:

- **The database.** Migrations only move forward and only add: new tables, and new columns that are nullable or have a default. A column or table is never dropped, renamed or given a new meaning; when a shape has to change, add the new one, copy the data across and stop reading the old one, leaving it in place. A database written by any released version opens and upgrades by itself. The reverse is not promised: an older binary refuses a database from a newer one (`ErrSchemaTooNew`) instead of guessing.
- **The config file.** Every key keeps its name and meaning. New keys are optional and have a default. A retired key is still accepted, ignored or mapped to its replacement. Unknown keys stay an error at startup, which catches typos, so a file written for a newer version does not load on an older one.
- **The commands.** Subcommands, flags and exit codes keep working. New ones may be added.
- **The JSON export.** Version 1 keeps every key it has. Keys may be added; removing or renaming one needs a new `version`.

If a break ever proves unavoidable, it ships as a new major version at a new module path (`github.com/butcher-of-blaviken/track/v2`), with a way to bring old data across. It is never slipped into a minor release.

## Considered Options

- **No promise, fix things as needed:** cheapest now, but people's history is what they are trusting us with, and a silent break loses it.
- **Promise forwards compatibility too:** an older binary opening a newer database would have to ignore what it does not understand and could corrupt it on write. Refusing is safer.

## Consequences

Schema and config design are slower and a bit untidy: old columns and keys stay. The promise is enforced by tests that fail when it is broken: `internal/store/sqlite/testdata/schema_vN.db` (one database per released schema, opened and read back by the current code), `internal/config/testdata/config_v1.toml`, and `internal/export/testdata/json_v1_paths.txt`. These fixtures are never edited to make a test pass; a new schema adds a new fixture (see `internal/store/sqlite/compat_test.go`).
