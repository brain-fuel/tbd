# Bug 0004: `tbd continue` silently drops the `feature finish` after a conflict

- **Component:** `internal/commands` (`continue`, `feature finish`), `internal/cli`
- **Affected command:** `tbd feature finish` (when its auto-rebase conflicts) + `tbd continue`
- **Version:** tbd 1.11.1 (commit 0661a26)
- **Severity:** medium — a resolved conflict leaves the feature rebased but NOT integrated; the user can believe their work landed on trunk when it did not
- **Status:** FIXED, verified (see Verification)

## Summary

`tbd feature finish` rebases the feature onto trunk and then, on success,
fast-forwards trunk, pushes, and deletes the branch. If the rebase conflicts,
finish leaves the rebase in progress (correct). But after the user resolves the
conflict and runs `tbd continue`, `continue` only completes the low-level
**rebase** - it does not run finish's remaining steps. Trunk is never
fast-forwarded and the branch is never deleted, yet `continue` prints a green
`✓ rebase complete`, so the operation looks done.

`tbd continue`'s own help even claims it resumes the operation: *"When tbd
commit, rebase, feature sync/finish/push, or cherry-up hits a conflict ... run
this."* For `feature finish` that promise was not kept.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit 0661a26 (v1.11.1)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
printf 'l1\nl2\n' > f.txt; git add .; git commit -qm seed
tbd init >/dev/null; git add -A; git commit -qm cfg

tbd feature start conf :local
printf 'l1\nFEAT\n' > f.txt; git commit -qam fe       # feature edits l2
git checkout -q develop
printf 'l1\nTRUNK\n' > f.txt; git commit -qam tr       # trunk edits l2 (conflict)
git checkout -q feature/conf

tbd feature finish :local        # conflicts, leaves rebase in progress
printf 'l1\nOK\n' > f.txt; git add f.txt
tbd continue :local
```

## Actual result (before fix)

```
✓ rebase complete; feature/conf @ 292b662
```

Then: still on `feature/conf`, `develop` still points at the trunk-only commit
(`tr`), and `feature/conf` still exists. The finish was dropped; the user must
somehow know to run `tbd feature finish` a second time to integrate.

## Expected result

`tbd continue` completes the interrupted finish: fast-forward trunk to the
replayed feature, delete the branch, land on trunk.

```
✓ rebase complete; feature/conf @ 0f22c9c
resuming: tbd feature finish :local
✓ develop fast-forwarded to 0f22c9c
✓ deleted feature/conf
```

## Root cause

`continue` knew nothing about the higher-level operation that started the
rebase. `feature finish`'s conflict path (`handleRebaseConflict`) left the
rebase in progress but recorded no intent, so `continue` could only finish the
rebase itself.

## Fix

- `cli.Context` now carries `Raw` (the argv the command was invoked with), set
  in `cli.Run`.
- When `feature finish` leaves a rebase in progress on conflict, it records that
  argv in `.git/tbd-resume` (new `internal/commands/resume.go`).
- `tbd continue`, after completing the rebase, replays the recorded argv via the
  new `cli.Dispatch`, so the finish runs to completion; the record is then
  cleared. `tbd continue :abort` and a clean finish also clear the record.
- Scope: only `feature finish` records a resume (the one multi-step op whose
  dropped tail is dangerous). Plain `rebase`, `commit`, `sync`, `push`, and
  `cherry-put` are unchanged - their `continue` behavior was already correct or
  safe.

## Verification

- Manual: the reproduction now finishes integration on `tbd continue`; abort and
  plain-`rebase` paths record nothing and behave as before.
- Automated: `TestFinishConflictContinueCompletesFinish` asserts branch deleted,
  trunk advanced to the feature, landed on develop, resume record cleared. The
  test was confirmed to FAIL with the resume dispatch removed and PASS with it.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
