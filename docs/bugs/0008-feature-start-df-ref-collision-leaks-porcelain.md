# Bug 0008: `tbd feature start` leaks raw git porcelain on a directory/file ref collision

- **Component:** `internal/commands` (`featureStart`), `internal/git`
- **Affected command:** `tbd feature start NAME`
- **Version:** tbd 1.11.3 (commit 4adee34)
- **Severity:** low — confusing error; fails safely (no corruption)
- **Status:** FIXED, verified (see Verification)

## Summary

Bug 0002 added `check-ref-format` validation so syntactically-invalid feature
names are rejected cleanly. But that validates the name in isolation; it cannot
see a directory/file collision with an *existing* branch. git stores branches as
files under `.git/refs/heads`, so `feature/a` (a file) and `feature/a/b`
(needs `feature/a/` to be a directory) cannot coexist. Creating one when the
other exists makes `git branch` fail with raw `fatal: cannot lock ref ...`
porcelain, surfaced verbatim by tbd.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit 4adee34 (v1.11.3)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
echo s > s; git add .; git commit -qm seed
tbd init >/dev/null; git add -A; git commit -qm cfg
tbd feature start a :local
git checkout -q develop
tbd feature start a/b :local
```

## Actual result (before fix)

```
tbd feature: git branch feature/a/b develop: fatal: cannot lock ref 'refs/heads/feature/a/b': 'refs/heads/feature/a' exists; cannot create 'refs/heads/feature/a/b'
hint: ...
```

## Expected result

```
tbd feature: cannot create branch "feature/a/b": it collides with the existing branch "feature/a" (git stores branches as files, so the two cannot coexist)
```

## Root cause

`featureStart` checked only exact-duplicate existence (`repo.Exists`), not a
path-prefix (directory/file) collision, so git's lock failure leaked through.

## Fix

- Added `Repo.ConflictingBranch(branch)`: scans existing local branches for one
  that is a path-prefix of `branch` or vice-versa.
- `featureStart` calls it after the duplicate check and returns a clear tbd
  error. Both collision directions (`feature/a` blocks `feature/a/b`, and
  `feature/a/b` blocks `feature/a`) are covered; unrelated nested names
  (`feature/p/q`) still create normally.

## Note (out of scope)

A pathologically long name (hundreds of chars) can still fail with a
filesystem-level `File name too long` from git; that limit is OS/filesystem
dependent and not predictable from the name alone, so it is left to git.

## Verification

- Manual: the reproduction (and the reverse direction) now print the clean error
  with exit 1; `feature/p/q` with no conflict still succeeds.
- Automated: `TestFeatureStartRejectsDirFileCollision`.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
