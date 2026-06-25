# Bug 0005: a stale `.git/tbd-resume` record hijacks an unrelated `tbd continue` into a destructive finish

- **Component:** `internal/commands` (`continue`, `feature finish`, `resume.go`)
- **Affected commands:** `tbd continue` after any rebase, once a `feature finish` has left a stale resume record
- **Version:** tbd 1.11.2 (commit c9ac5dd) - regression introduced by bug 0004's fix
- **Severity:** high — `tbd continue` performs an unrequested fast-forward of trunk and **deletes a branch the user never asked to finish**
- **Status:** FIXED, verified (see Verification)

## Summary

Bug 0004's fix records the argv of a conflicting `feature finish` in
`.git/tbd-resume`, and `tbd continue` replays it after the rebase completes. But
the record is not bound to the specific rebase it came from. If the original
rebase is ended by any path other than `tbd continue`/`tbd continue :abort`
(e.g. a plain `git rebase --abort`), the record is left behind. The **next**
`tbd continue` - for a completely unrelated rebase, on a different branch -
reads the stale record and replays `feature finish`, fast-forwarding trunk to
that branch and deleting it.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit c9ac5dd (v1.11.2)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
printf 'l1\nl2\n' > f.txt; git add .; git commit -qm seed
tbd init >/dev/null; git add -A; git commit -qm cfg

# 1) a feature finish conflicts -> writes .git/tbd-resume
tbd feature start aaa :local
printf 'l1\nA\n' > f.txt; git commit -qam a
git checkout -q develop; printf 'l1\nT\n' > f.txt; git commit -qam t
git checkout -q feature/aaa
tbd feature finish :local          # conflicts; resume = "feature finish :local"

# 2) user backs out with raw git (tbd never told) -> resume record left stale
git rebase --abort

# 3) later, an UNRELATED plain rebase on a DIFFERENT branch conflicts
tbd feature start bbb :local
printf 'l1\nB\n' > f.txt; git commit -qam b
git checkout -q develop; printf 'l1\nT2\n' > f.txt; git commit -qam t2
git checkout -q feature/bbb
tbd rebase :local                  # conflicts
printf 'l1\nR\n' > f.txt; git add f.txt
tbd continue :local                # <-- expected: just finish the rebase
```

## Actual result (before fix)

```
... continuing the rebase
✓ rebase complete; feature/bbb @ 403fe66
resuming: tbd feature finish :local
✓ develop fast-forwarded to 403fe66
✓ deleted feature/bbb
```

`tbd continue` performed a `feature finish` the user never requested: trunk was
advanced and `feature/bbb` was deleted. (`feature/aaa`, the branch the stale
record actually came from, is untouched - the replayed finish bound to whatever
branch happened to be current.)

## Expected result

`tbd continue` finishes only the rebase in progress. A resume record from an
unrelated, already-ended operation must never drive it - especially not on a
different branch.

## Root cause

The resume record stored only the argv, with no link to the branch/rebase it
belonged to. `continue` replayed it after any rebase regardless of context, and
nothing cleared it when the originating rebase ended outside tbd.

## Fix

- The resume record now stores the **resolved feature branch** alongside the
  argv (`writeResume(branch, argv)` / `readResume()` returns both).
- `tbd continue` replays the record **only when the current branch equals the
  recorded branch**; on any mismatch it discards the stale record and just
  finishes the rebase. The record is consumed once (cleared before replay).
- `feature finish` clears any pre-existing record on entry (before it might
  write a fresh one), so a stale record never outlives a new attempt.

This binds the resume to the branch being finished, so an unrelated rebase on a
different branch can no longer be hijacked. The remaining same-branch case
(raw-abort then manually rebase the very same branch and continue) replays a
finish of the branch the user was already finishing - non-destructive of
unrelated work.

## Verification

- Manual: the reproduction now ends with `tbd continue` finishing only the
  rebase; `feature/bbb` survives and trunk is not advanced.
- Automated: `TestContinueIgnoresStaleResumeOnOtherBranch` reproduces the
  cross-branch scenario and asserts the unrelated branch is not finished;
  confirmed to fail before the fix and pass after.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
