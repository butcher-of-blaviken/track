# Agent Instructions

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) verbiage (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, etc.) for all commits in this repo.

## Workflow

Work is tracked as GitHub issues (epics with linked tickets). Prefer **stacked PRs**: one PR per ticket, each based on the one before it, so work continues without merging and syncing `main` between tickets. For each ticket:

1. Name the branch `<type>/<issue-number>-<short-slug>` (e.g. `feat/9-task-model`). For the first ticket of a stack, branch from an up-to-date `main` with `gh stack init <branch>`. For each later ticket, run `gh stack add <branch>` from the top of the stack. A ticket that does not build on the open stack starts a new stack from `main`.
2. Do the work, committing with Conventional Commits. Reference the ticket, using `Closes #N` in the final commit or the PR body.
3. Run `make check` locally before pushing. It runs, in order: `gofmt` check, `go vet`, `golangci-lint`, `go test`, `go build`. Fix everything it reports; do not push a red check. Run `make e2e` too when touching `internal/tui` or `main.go`.
4. Push and open the PR with `gh stack submit --auto --open`, then set a real title and body (linking the ticket) with `gh pr edit`, since `submit` generates them. CI runs the same checks on every PR in the stack. It can take ~20s to appear.
5. Report the PR and wait for the go-ahead before merging. Merge with `gh stack merge <pr> --yes --squash`, which merges that PR and every unmerged PR below it, in order. Then `gh stack sync --prune` to bring the rest up to date.

Always pass the non-interactive flags above: bare `gh stack` commands open prompts or a TUI and hang. The full command reference is in `.claude/skills/gh-stack/SKILL.md`. To change a lower layer, check it out, commit there, and run `gh stack rebase --upstack`.

A single ticket with no follow-up can still be a plain branch from `main` and a `gh pr create` PR against `main`.

Never commit directly to `main`.

To cut a release (a tag plus a GitHub Release), follow `docs/RELEASING.md`. Only tag a `main` commit whose CI has passed, and only when asked: a tag is permanent.

## Commands

- `make check`: all of the above in one go (run this before every push)
- `make e2e`: tmux end-to-end tests of the real binary (needs tmux, ~30s; run when touching `internal/tui` or `main.go`; not part of `make check`; `TestFullCycleFromTheKeyboard` runs the whole add, focus, hand-off, Break and restart loop)
- `make fmt`, `make vet`, `make lint`, `make test`, `make build`, `make run`, `make clean`

## Architecture rules

- Stay backward compatible (`docs/adr/0003-backward-compatibility.md`). A migration may only add; a config key, subcommand, flag or JSON export key is never removed or renamed. A new schema version needs a new `schema_vN.db` fixture in `internal/store/sqlite/testdata` (see `compat_test.go`); never edit an existing fixture, `config_v1.toml` or `json_v1_paths.txt` to make a test pass.
- Domain logic lives in `internal/core` and must not import UI packages (see `docs/adr/0002-ui-decoupled-pull-based-core.md`).
- Domain vocabulary is defined in `CONTEXT.md`; use those terms in code, tests and issues.

<!-- code-review-graph MCP tools -->
## MCP Tools: code-review-graph

**IMPORTANT: This project has a knowledge graph. ALWAYS use the
code-review-graph MCP tools BEFORE using Grep/Glob/Read to explore
the codebase.** The graph is faster, cheaper (fewer tokens), and gives
you structural context (callers, dependents, test coverage) that file
scanning cannot.

### When to use graph tools FIRST

- **Exploring code**: `semantic_search_nodes` or `query_graph` instead of Grep
- **Understanding impact**: `get_impact_radius` instead of manually tracing imports
- **Code review**: `detect_changes` + `get_review_context` instead of reading entire files
- **Finding relationships**: `query_graph` with callers_of/callees_of/imports_of/tests_for
- **Architecture questions**: `get_architecture_overview` + `list_communities`

Fall back to Grep/Glob/Read **only** when the graph doesn't cover what you need.

### Key Tools

| Tool | Use when |
| ------ | ---------- |
| `detect_changes` | Reviewing code changes — gives risk-scored analysis |
| `get_review_context` | Need source snippets for review — token-efficient |
| `get_impact_radius` | Understanding blast radius of a change |
| `get_affected_flows` | Finding which execution paths are impacted |
| `query_graph` | Tracing callers, callees, imports, tests, dependencies |
| `semantic_search_nodes` | Finding functions/classes by name or keyword |
| `get_architecture_overview` | Understanding high-level codebase structure |
| `refactor_tool` | Planning renames, finding dead code |

### Workflow

1. The graph auto-updates on file changes (via hooks).
2. Use `detect_changes` for code review.
3. Use `get_affected_flows` to understand impact.
4. Use `query_graph` pattern="tests_for" to check coverage.
