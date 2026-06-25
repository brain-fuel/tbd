# Bug 0002: `tbd feature start` with an invalid name leaks raw git porcelain (after a needless fetch)

- **Component:** `internal/commands` (`featureStart`), `internal/git`
- **Affected command:** `tbd feature start NAME`
- **Version:** tbd 1.11.0 (commit a4df530)
- **Severity:** low — wrong-looking error message; also performs a wasted network fetch before failing
- **Status:** FIXED, verified (see Verification)

## Summary

`tbd feature start` only checks that NAME is non-empty. Any other malformed
name (containing a space, `..`, a trailing `/`, etc.) is passed straight to
`git branch`, which fails with raw `fatal:` porcelain plus git's "hint:" advice
lines - inconsistent with tbd's otherwise clean `tbd feature: <message>` error
style. Worse, the failure happens *after* `tbd` has already done a network
fetch, so an obviously-invalid name still hits the network.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit a4df530 (v1.11.0)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
echo s > s; git add .; git commit -qm s
tbd feature start "has space"
```

## Actual result (before fix)

```
... fetching origin
tbd feature: git branch feature/has space origin/develop: fatal: 'feature/has space' is not a valid branch name
hint: See 'git help check-ref-format'
hint: Disable this message with "git config set advice.refSyntax false"
```

A network fetch ran first, then git's internal message and advice hints leaked
through.

## Expected result

A clean, tbd-style error, emitted *before* any network access:

```
tbd feature: "has space" is not a valid feature name (it would make the invalid branch "feature/has space")
```

## Root cause

`featureStart` validated only `name == ""`, then fetched, then called
`BranchCreate`, letting git reject the name.

## Fix

- Added `Repo.ValidBranchName(branch)` in `internal/git/git.go`, which runs
  `git check-ref-format refs/heads/<branch>` (validates the full ref, so
  leading-dash and `@{...}` components are rejected rather than reinterpreted).
- `featureStart` now calls it right after building the branch name - before the
  existence check and before the fetch - returning a clean error.

## Verification

- Manual: the reproduction now prints the clean error with exit 1 and no fetch;
  valid names (`login`, `a/b/c`) still succeed.
- Automated: `TestFeatureStartRejectsInvalidName` asserts `"has space"`,
  `"a..b"`, and `"a/"` are rejected with the clean message and create no branch.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
