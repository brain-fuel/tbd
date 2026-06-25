# Bug 0003: `tbd release cut` with a ref-invalid version leaks raw git porcelain mid-operation

- **Component:** `internal/commands` (`runRelease`), `internal/git`
- **Affected command:** `tbd release cut VERSION`
- **Version:** tbd 1.11.0 (commit a4df530)
- **Severity:** low/medium — leaks git internals; with `strategy:branch,tag` it can partially complete (branch created) before failing on the tag
- **Status:** FIXED, verified (see Verification)

## Summary

`tbd release cut` does not validate VERSION before building refs from it. A
version that forms an invalid git ref (e.g. one containing a space) is passed
straight to `git branch` / `git tag`, leaking raw `fatal:` porcelain plus git
advice hints. The on-trunk check and its `✓ ... is on trunk` line are printed
*before* the failure, and with `strategy:branch,tag` the release branch can be
created before the tag step fails - leaving a half-cut release.

This is the same defect class as bug 0002 (`feature start`), in a different
command.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit a4df530 (v1.11.0)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
echo s > s; git add .; git commit -qm s
tbd init >/dev/null; git add -A; git commit -qm cfg
tbd release cut "1 0" strategy:branch :local
tbd release cut "1 0" strategy:tag :local
```

## Actual result (before fix)

```
✓ develop @ 7110e3a is on trunk
tbd release: git branch release/1 0 develop: fatal: 'release/1 0' is not a valid branch name
hint: See 'git help check-ref-format'
hint: Disable this message with "git config set advice.refSyntax false"
```

(and for the tag strategy, `fatal: 'v1 0' is not a valid tag name.`)

## Expected result

A clean, tbd-style error emitted before the fetch / on-trunk check and before
any ref is created:

```
tbd release: version "1 0" is not valid for a release (it would make the invalid branch "release/1 0")
```

## Root cause

`runRelease` checked only `version == ""`, then built `ReleaseBranchPrefix +
version` and `ReleaseTagTemplate` with `{version}` substituted and handed them
to git, which rejected the malformed ref.

Note: this is specifically about *ref-invalid* versions. Accepting otherwise
unusual but ref-valid versions (calver `2026.06`, `1.0.0-rc1`, etc.) is
intentional - `tbd` does not impose semver, so no semver check was added.

## Fix

- Added `Repo.ValidTagName(tag)` in `internal/git/git.go` (mirrors
  `ValidBranchName`, validates `refs/tags/<tag>`).
- `runRelease` now validates the resulting branch and/or tag names (per the
  selected strategy) immediately after resolving the strategy - before the
  fetch, on-trunk check, and any ref creation - returning a clean error.

## Verification

- Manual: the reproduction now prints clean errors with exit 1, no porcelain,
  no `✓ ... is on trunk` line, and no ref created; valid versions still cut.
- Automated: `TestReleaseCutRejectsRefInvalidVersion` (branch and tag
  strategies) and `TestReleaseCutValidVersion`.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
