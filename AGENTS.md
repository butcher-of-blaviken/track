# Agent Instructions

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) verbiage (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, etc.) for all commits in this repo.

## Workflow

Work is tracked as GitHub issues (epics with linked tickets). For each ticket:

1. Branch from an up-to-date `main`, named `<type>/<issue-number>-<short-slug>` (e.g. `feat/9-task-model`).
2. Do the work, committing with Conventional Commits. Reference the ticket, using `Closes #N` in the final commit or the PR body.
3. Run `make check` locally before pushing. It runs, in order: `gofmt` check, `go vet`, `golangci-lint`, `go test`, `go build`. Fix everything it reports; do not push a red check.
4. Push the branch and open a PR against `main` with `gh pr create`, linking the ticket. CI runs the same checks.

Never commit directly to `main`.

## Commands

- `make check`: all of the above in one go (run this before every push)
- `make fmt`, `make vet`, `make lint`, `make test`, `make build`, `make run`, `make clean`

## Architecture rules

- Domain logic lives in `internal/core` and must not import UI packages (see `docs/adr/0002-ui-decoupled-pull-based-core.md`).
- Domain vocabulary is defined in `CONTEXT.md`; use those terms in code, tests and issues.
