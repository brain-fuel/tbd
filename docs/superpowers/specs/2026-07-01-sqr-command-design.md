# `tbd sqr` — squash + rebase, onto trunk or a named branch

## Summary

Rename the `rebase` command to `sqr` and give it an optional `onto:BRANCH`
target. `sqr` squashes the current branch into one commit and rebases it onto a
target — trunk by default, or any named branch. It replaces `rebase` outright
(breaking rename; `tbd` has a sole consumer, so no `rebase` alias is kept).

## Behavior

```
tbd sqr                       squash current branch to one commit, rebase onto trunk
tbd sqr onto:BRANCH           squash, then rebase onto BRANCH instead of trunk
```

Existing options carry over unchanged:

- `message:"..."` / `m:"..."` — message for the squashed commit
- `:edit` — open editor for the squashed commit message
- `:abort-on-conflict` — abort the rebase instead of leaving it in progress

Preconditions (unchanged from `rebase`): a branch is checked out (not detached),
it is not the trunk branch, and the working tree is clean.

`tbd sqr onto:BRANCH` requires BRANCH to exist and to differ from the current
branch.

## Design

### Command surface

`internal/commands/rebase.go` → `internal/commands/sqr.go`.

- `Name: "sqr"`, updated `Summary`/`Usage` describing the `onto:` target.
- Spec adds `onto` to `Named`: `Named: argv.Opts("message", "m", "onto")`,
  `Flags: argv.Opts("edit", "abort-on-conflict")`.
- `runRebase` → `runSqr`.

### Two paths in `runSqr`

After the shared squash, branch on the target:

- **`onto == ""` (trunk, default):** the existing path — `finalizeOnTrunk`,
  which runs the full invariant guard and the before/after DAG visualization.
  Unchanged.
- **`onto` set:** validate BRANCH exists and is not the current branch, then
  rebase the squashed branch onto it via the generalized visualizer, using
  `handleRebaseConflict` for conflicts (the same `tbd continue` machinery
  cherry-put relies on).

### Squash reuse

`runRebase` currently inlines squash logic that duplicates `squashToOne`
(cherryput.go). Fold the inline code to call `e.squashToOne(branch, fork,
len(existing), msg, edit)` so there is one squash implementation.

### Generalized visualization (approach A)

Generalize the trunk-bound helpers to accept an explicit target:

- `rebasePlan(feature, targetRef, targetLabel string)` — replace the two
  `e.trunkRef` / `e.trunkLocal` references with the parameters.
- `visualizeRebase(feature, targetRef, targetLabel string)` — replace the
  `e.trunkRef` / `e.trunkLocal` references (banner text, `Rebase` target,
  post-rebase `LogRange`, success line) with the parameters.

Trunk callers (`finalizeOnTrunk`, `featureSync`, and any other callers) pass
`e.trunkRef, e.trunkLocal` and behave exactly as before. The `onto:` path passes
`onto, onto`. Both paths then render the same before/after graph.

### Invariant caveat

The `onto:` non-trunk path deliberately skips the trunk-ancestor guard — it is
an explicit override. If BRANCH is itself trunk-based, the rebased result
inherits trunk-ancestry transitively. This is documented behavior, not enforced.

## Tests

- Rename `rebase_test.go` → `sqr_test.go`; update command name in existing
  assertions (squash-then-rebase-onto-trunk still passes).
- Add a case: `tbd sqr onto:BRANCH` squashes and rebases onto a non-trunk
  branch, producing one commit on top of BRANCH.
- Add a conflict case on the `onto:` path: a conflicting rebase is left in
  progress and `tbd continue` completes it (mirrors cherry-put's conflict test).

## Docs

- README quick-start / command list: `rebase` → `sqr`, document `onto:`.
- `tbd learn` narration, if it mentions `rebase`.

## Out of scope

- No `rebase` back-compat alias.
- No change to `cherry-put` (its `onto:`+`as:` new-branch flow stays distinct).
- No new invariant enforcement on the `onto:` path.
