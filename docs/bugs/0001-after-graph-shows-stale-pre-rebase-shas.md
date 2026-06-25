# Bug 0001: rebase "after" graph shows stale pre-rebase SHAs for replayed commits

- **Component:** `internal/render` (rebase visualization), `internal/commands` (`visualizeRebase`)
- **Affected commands:** `tbd feature finish`, `tbd commit`, `tbd rebase` (every path that prints the before/after graph)
- **Version:** tbd 1.11.0 (`git` commit a4df530)
- **Severity:** high — corrupts the tool's headline feature ("shows you the move ... nothing happens silently")
- **Status:** FIXED, verified (see Verification)

## Summary

When `tbd` auto-rebases a feature branch onto a moved trunk, it prints a
before/after ASCII graph. The **"after"** half draws the replayed feature
commits using their **pre-rebase** SHAs. A rebase rewrites those commits (their
parent changed), so the SHAs shown never exist on the resulting trunk. The
graph that is supposed to show the user exactly what happened instead shows
SHAs that are pure fiction.

## Environment

- OS: Linux 6.18 (WSL2)
- git: system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit a4df530 (v1.11.0)

## Steps to reproduce

```sh
# bare origin with develop as default branch
W=$(mktemp -d); cd "$W"
git init -q --bare origin.git
git -C origin.git symbolic-ref HEAD refs/heads/develop
git clone -q origin.git c1; cd c1
git config user.name dev; git config user.email dev@x
echo base > a.txt; git add .; git commit -qm init
git branch -M develop; git push -q origin develop
tbd init >/dev/null; git add -A; git commit -qm cfg; git push -q origin develop

# second clone moves trunk
cd "$W"; git clone -q origin.git c2; cd c2
git config user.name dev; git config user.email dev@x

# c1 starts a feature and commits
cd "$W/c1"; tbd feature start alpha :local >/dev/null
echo alpha >> a.txt; git commit -qam "alpha edits a"

# c2 advances trunk (non-conflicting)
cd "$W/c2"; echo trunkline >> c.txt; git add c.txt
git commit -qm "trunk move c"; git push -q origin develop

# c1 finishes -> auto-rebase prints before/after graph
cd "$W/c1"; tbd feature finish

# compare the SHA shown for the replayed commit in the "after" graph
# against the real SHA that actually landed on trunk:
git log --oneline -2 develop
```

## Actual result

The "after" graph labels the replayed commit `7afce12` (its pre-rebase SHA):

```
after
  ● 7afce12  alpha edits a  ← feature/alpha (replayed)
  ● fa58576  trunk move c  (trunk head: develop)
```

But the commit that actually landed on trunk is `82825db`:

```
82825db alpha edits a
fa58576 trunk move c
```

`7afce12` is a now-dangling object that is **not** on trunk. With multiple
feature commits, **every** replayed node shows a wrong SHA.

## Expected result

The "after" graph should show the real SHAs of the replayed commits as they
exist on trunk after the rebase (`82825db`), or otherwise not assert a concrete
SHA it cannot know. The displayed history must match `git log` of the result.

## Root cause

`internal/commands/common.go:visualizeRebase` builds the plan with
`rebasePlan()` **before** running the rebase, then renders the whole
before+after graph from that single pre-rebase snapshot. `internal/render/dag.go`
draws the "after" replayed nodes from `RebasePlan.FeatLine`, which holds the
*pre-rebase* commits. Rebasing rewrites those commits to new SHAs, but the plan
is never refreshed, so the "after" SHAs are stale.

## Fix

- Split `RebasePlan.Render` into `RenderBefore` and `RenderAfter`
  (`Render` retained = before + after, for the existing projection tests).
- `visualizeRebase` now prints the **before** graph (a true projection), runs
  the rebase, then re-reads the feature commits' **new** SHAs
  (`git log trunkRef..feature`) and prints the **after** graph from those.
- On a conflict the before graph has already been shown; the after graph is
  omitted because the outcome is genuinely unknown until the rebase completes.

## Verification

After the fix, running the reproduction above, the SHA in the "after" graph
matches `git log develop` exactly. An automated regression test asserts the
after-graph SHA equals the post-rebase trunk SHA.
