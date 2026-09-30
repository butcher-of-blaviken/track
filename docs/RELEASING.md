# Releasing

A release is a git tag `vX.Y.Z` on a commit of `main`. There is no build pipeline: people install it with `go install github.com/butcher-of-blaviken/track@vX.Y.Z` (or `@v1`, `@latest`), and Go builds it from the tag. A GitHub Release on the tag carries the notes.

## Choose the version

Semantic versioning, with the promise in [ADR 0003](adr/0003-backward-compatibility.md):

- **Patch** (`v1.0.1`): fixes only.
- **Minor** (`v1.1.0`): new features, flags, subcommands, config keys, export keys, or additive migrations. UI changes are minor.
- **Major**: only for a break the ADR forbids within v1 (removing or renaming a flag, config key, export key or column). It needs a new module path, `github.com/butcher-of-blaviken/track/v2`: change the `module` line in `go.mod`, rewrite the import paths, and say in the README how to move over.

## Release

1. Merge everything for the release to `main`.
2. Wait for CI on the `main` commit to pass, and note its SHA:

   ```
   git checkout main && git pull
   gh run list --branch main --limit 1 --json conclusion,headSha
   ```

   Do not tag a commit CI has not passed.
3. Create the tag and the GitHub Release in one step. The notes list the PRs merged since the previous tag:

   ```
   gh release create vX.Y.Z --target <sha> --title vX.Y.Z --generate-notes --latest
   ```

   For a release that needs more than a PR list, write the notes in a file and pass `--notes-file FILE` instead (see the `v1.0.0` release for the shape: install line, what is new, compatibility, known limits). Use `--prerelease` and a tag like `v1.1.0-rc.1` for a trial; `@latest` ignores it.
4. Verify the install from outside the repo, with an empty module cache so nothing is reused:

   ```
   cd "$(mktemp -d)"
   for q in vX.Y.Z v1 latest; do
     GOPATH=$PWD/gp GOBIN=$PWD/bin/$q GOFLAGS=-modcacherw go install github.com/butcher-of-blaviken/track@$q &&
       echo "@$q -> $(bin/$q/track --version)"
   done
   ```

   Each line should print `track vX.Y.Z` (`@v1` and `@latest` only once this is the newest v1 release). Try `bin/latest/track docs | head` and an `add` and `export` against a scratch `--data-dir`.
5. Close the epic or tickets the release finishes.

## If something goes wrong

- **A tag is permanent.** `proxy.golang.org` caches every version it serves, so deleting or moving a tag does not take it back for people who already fetched it, and a different commit under the same tag breaks checksums. Never move or reuse a tag.
- **A bad release**: fix it on `main` and release the next patch. To warn people off the bad version, add a `retract` directive to `go.mod` (for example `retract v1.0.1 // migration bug, use v1.0.2`) in that patch release.
- **The install fails just after a repository change** (for example it was private, or the first request came before the tag existed): the module proxy and checksum database cache a failed lookup for a while. A fresh tag is not affected; `GOPROXY=direct GONOSUMDB=github.com/butcher-of-blaviken/*` bypasses them to check the source itself.
- **Edit the notes** at any time: `gh release edit vX.Y.Z --notes-file FILE`.
