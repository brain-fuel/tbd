# `tbd sqr` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `rebase` command to `sqr` and give it an optional `onto:BRANCH` target so it squashes the current branch to one commit and rebases onto trunk (default) or any named branch.

**Architecture:** Generalize the two trunk-bound rendering helpers (`rebasePlan`, `visualizeRebase`) to take an explicit target ref+label, then build `runSqr` on top: default path reuses `finalizeOnTrunk` (guard + graph onto trunk); `onto:` path validates the branch and calls the generalized `visualizeRebase` directly. Squash logic is folded onto the existing shared `squashToOne`.

**Tech Stack:** Go 1.x, standard library only. Tests are Go table/fixture style using `repoFixture`/`gitRun` helpers already in `internal/commands`.

## Global Constraints

- Module path: `goforge.dev/tbd`. Package under edit: `internal/commands`.
- Colon arg syntax: `key:value` named, `:flag` boolean (tbd convention).
- Breaking rename: NO `rebase` alias is kept (sole consumer; safe).
- Commands register via `init()` + `cli.Register`; no `cmd/tbd/main.go` change needed.
- Conflict handling reuses `handleRebaseConflict` + `tbd continue`; no new resume record on the `sqr` paths (matches current `rebase`).
- Run `go test ./...` from the tbd repo root before each commit; keep stdout clean.
- No em-dashes / en-dashes in any prose or comments.

---

### Task 1: Generalize `rebasePlan` and `visualizeRebase` to take a target

Pure refactor. The two helpers currently hardcode `e.trunkRef` / `e.trunkLocal`. Parameterize them; every existing caller passes the trunk values, so all current tests stay green.

**Files:**
- Modify: `internal/commands/common.go` (`rebasePlan` ~L131, `visualizeRebase` ~L159)
- Modify: `internal/commands/feature.go` (3 call sites: L112, L159, L217)
- Modify: `internal/commands/commit.go` (1 call site: L124, inside `finalizeOnTrunk`)

**Interfaces:**
- Produces:
  - `func (e env) rebasePlan(feature, targetRef, targetLabel string) (render.RebasePlan, error)`
  - `func (e env) visualizeRebase(feature, targetRef, targetLabel string) error`
- Consumes: nothing new.

- [ ] **Step 1: Run the existing suite to confirm a green baseline**

Run: `go test ./...`
Expected: PASS (all packages ok).

- [ ] **Step 2: Parameterize `rebasePlan`**

In `internal/commands/common.go`, replace the whole `rebasePlan` function with:

```go
// rebasePlan builds the before/after picture for replaying feature onto targetRef.
func (e env) rebasePlan(feature, targetRef, targetLabel string) (render.RebasePlan, error) {
	fork, err := e.repo.MergeBase(targetRef, feature)
	if err != nil {
		return render.RebasePlan{}, err
	}
	forkShort, _ := e.repo.Short(fork)
	forkSubj := e.repo.Subject(fork)

	trunkCommits, _ := e.repo.LogRange(fork + ".." + targetRef)
	featCommits, _ := e.repo.LogRange(fork + ".." + feature)

	return render.RebasePlan{
		Feature:   feature,
		Trunk:     targetLabel,
		Fork:      render.Commit{Short: forkShort, Subject: forkSubj},
		TrunkLine: toRenderCommits(trunkCommits),
		FeatLine:  toRenderCommits(featCommits),
	}, nil
}
```

- [ ] **Step 3: Parameterize `visualizeRebase`**

In `internal/commands/common.go`, replace the whole `visualizeRebase` function with:

```go
// visualizeRebase prints the rebase plan, then rebases the checked-out feature
// onto targetRef (labeled targetLabel in the output). The feature must already
// be checked out. A conflict is returned wrapped in ErrRebaseConflict (the
// rebase is left in progress for the caller to resolve or abort).
func (e env) visualizeRebase(feature, targetRef, targetLabel string) error {
	plan, err := e.rebasePlan(feature, targetRef, targetLabel)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.out, e.colors.Bold("Rebasing "+feature+" onto "+targetLabel+":"))
	fmt.Fprintln(e.out)
	fmt.Fprint(e.out, plan.RenderBefore(e.colors))
	fmt.Fprintln(e.out)
	rebErr := e.step("rebasing "+feature+" onto "+targetLabel, func() error { return e.repo.Rebase(targetRef) })
	if rebErr != nil {
		return fmt.Errorf("%w: %v", ErrRebaseConflict, rebErr)
	}
	// The rebase rewrote the feature commits onto the target head, giving them new
	// SHAs. Re-read them so the "after" graph shows the commits that actually
	// landed, not the pre-rebase projection.
	if replayed, err := e.repo.LogRange(targetRef + ".." + feature); err == nil {
		plan.FeatLine = toRenderCommits(replayed)
	}
	fmt.Fprint(e.out, plan.RenderAfter(e.colors))
	fmt.Fprintln(e.out)
	fmt.Fprintln(e.out, e.colors.Green("✓ rebased - "+feature+" now sits on top of "+targetLabel))
	return nil
}
```

Note: keep the doc-comment block above `visualizeRebase` replaced too (do not leave the old comment stranded).

- [ ] **Step 4: Update the 4 trunk call sites**

Each existing call `e.visualizeRebase(branch)` becomes `e.visualizeRebase(branch, e.trunkRef, e.trunkLocal)`.

`internal/commands/commit.go` (in `finalizeOnTrunk`, ~L124):

```go
		if rerr := e.visualizeRebase(branch, e.trunkRef, e.trunkLocal); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
```

`internal/commands/feature.go` L112 (`featureSync`):

```go
	if err := e.visualizeRebase(branch, e.trunkRef, e.trunkLocal); err != nil {
		return handleRebaseConflict(e, c, branch, err)
	}
```

`internal/commands/feature.go` L159 (`featurePush`):

```go
		if rerr := e.visualizeRebase(branch, e.trunkRef, e.trunkLocal); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
```

`internal/commands/feature.go` L217 (`featureFinish`):

```go
		if rerr := e.visualizeRebase(branch, e.trunkRef, e.trunkLocal); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr, c.Raw...)
		}
```

- [ ] **Step 5: Build and run the full suite to confirm nothing regressed**

Run: `go build ./... && go test ./...`
Expected: PASS. (Behavior is identical; only signatures changed.)

- [ ] **Step 6: Commit**

```bash
git add internal/commands/common.go internal/commands/feature.go internal/commands/commit.go
git commit -m "refactor: parameterize rebase visualization by target"
```

---

### Task 2: Rename `rebase` to `sqr` and add the `onto:` target

Replace the command. Rename the source and test files, add the `onto` named arg, fold the inline squash onto `squashToOne`, and branch on the target.

**Files:**
- Rename + rewrite: `internal/commands/rebase.go` -> `internal/commands/sqr.go`
- Rename + rewrite: `internal/commands/rebase_test.go` -> `internal/commands/sqr_test.go`

**Interfaces:**
- Consumes (from Task 1): `e.visualizeRebase(feature, targetRef, targetLabel string) error`.
- Consumes (existing): `e.squashToOne(branch, fork string, n int, msg string, edit bool) error` (cherryput.go); `finalizeOnTrunk(e env, c *cli.Context, branch string) error` (commit.go); `handleRebaseConflict(...)`.
- Produces: registered command `sqr`; `func runSqr(c *cli.Context) error`.

- [ ] **Step 1: Rename the test file, then write the failing `onto:` success test**

```bash
git mv internal/commands/rebase_test.go internal/commands/sqr_test.go
```

In `internal/commands/sqr_test.go`, update the existing tests to the new names, and add the new `onto:` test. Replace the whole file with:

```go
package commands

import (
	"strings"
	"testing"
)

func TestSqrSquashesAndRebasesOntoTrunk(t *testing.T) {
	dir := repoFixture(t)
	// A "normal" branch with several commits.
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w1.txt", "one")
	writeAndCommit(t, dir, "w2.txt", "two")
	writeAndCommit(t, dir, "w3.txt", "three")
	// Trunk advances behind it.
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "t.txt", "trunk")
	gitRun(t, dir, "switch", "-q", "work")

	if err := runSqr(mustCtx(dir, "sqr")); err != nil {
		t.Fatalf("sqr: %v", err)
	}
	if n := commitCount(t, dir, "work"); n != 1 {
		t.Fatalf("expected 1 commit after squash+rebase, got %d", n)
	}
	r, _ := openRepo(dir)
	th, _ := r.RevParse("develop")
	wh, _ := r.RevParse("work")
	if !r.IsAncestor(th, wh) {
		t.Fatal("branch must sit on top of trunk after rebase")
	}
}

func TestSqrOntoNamedBranch(t *testing.T) {
	dir := repoFixture(t)
	// A base branch that is NOT trunk, advanced past trunk.
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "base.txt", "base work")
	// A multi-commit work branch off trunk.
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w1.txt", "one")
	writeAndCommit(t, dir, "w2.txt", "two")

	if err := runSqr(mustCtx(dir, "sqr", "onto:base")); err != nil {
		t.Fatalf("sqr onto:base: %v", err)
	}
	// One squashed commit sitting on top of base, no merge commit.
	if n, _ := gitCapture(dir, "rev-list", "--count", "base..work"); n != "1" {
		t.Fatalf("expected 1 commit over base, got %q", n)
	}
	if n, _ := gitCapture(dir, "rev-list", "--count", "--merges", "base..work"); n != "0" {
		t.Fatalf("work should have no merge commit over base, got %q merges", n)
	}
	r, _ := openRepo(dir)
	bh, _ := r.RevParse("base")
	wh, _ := r.RevParse("work")
	if !r.IsAncestor(bh, wh) {
		t.Fatal("work must sit on top of base after sqr onto:base")
	}
}

func TestSqrOntoMissingBranch(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w.txt", "w")
	err := runSqr(mustCtx(dir, "sqr", "onto:nope"))
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing-branch error, got %v", err)
	}
}

// Regression for bug 0001: the "after" graph must show the real post-rebase SHA
// of the replayed commit, not the stale pre-rebase one.
func TestSqrAfterGraphShowsPostRebaseSha(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w1.txt", "one")
	writeAndCommit(t, dir, "w2.txt", "two")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "t.txt", "trunk")
	gitRun(t, dir, "switch", "-q", "work")

	ctx, out, _ := newCtx(dir, "sqr")
	if err := runSqr(ctx); err != nil {
		t.Fatalf("sqr: %v", err)
	}

	r, _ := openRepo(dir)
	head, _ := r.RevParse("work")
	short, _ := r.Short(head)

	output := out.String()
	_, after, found := strings.Cut(output, "after")
	if !found {
		t.Fatalf("no after graph in output:\n%s", output)
	}
	if !strings.Contains(after, short) {
		t.Fatalf("after graph must show post-rebase sha %s, got:\n%s", short, after)
	}
}

func TestSqrRefusesTrunk(t *testing.T) {
	dir := repoFixture(t) // on develop
	err := runSqr(mustCtx(dir, "sqr"))
	if err == nil || !strings.Contains(err.Error(), "refusing to squash-rebase the trunk") {
		t.Fatalf("expected trunk refusal, got %v", err)
	}
}

func TestSqrRefusesDirty(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w.txt", "w")
	writeFile(t, dir, "w.txt", "dirty change") // uncommitted
	err := runSqr(mustCtx(dir, "sqr"))
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("expected dirty refusal, got %v", err)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail (command not yet renamed)**

Run: `go test ./internal/commands/ -run TestSqr -v`
Expected: FAIL to COMPILE with "undefined: runSqr" (the old symbol is still `runRebase`).

- [ ] **Step 3: Rename the source file and rewrite it as `sqr`**

```bash
git mv internal/commands/rebase.go internal/commands/sqr.go
```

Replace the whole contents of `internal/commands/sqr.go` with:

```go
package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "sqr",
		Summary: "Squash the current branch into one commit and rebase it onto trunk (or onto:BRANCH)",
		Usage: "tbd sqr [onto:BRANCH] [message:\"...\"] [:edit] [:abort-on-conflict]\n\n" +
			"Collapses every commit on the current branch into a single commit and\n" +
			"rebases it onto the latest trunk head. Pass onto:BRANCH to rebase onto\n" +
			"another branch instead of trunk. Handy for adopting a \"normal\" git branch\n" +
			"into the one-commit shape. Keeps the latest message unless you pass\n" +
			"message:/m: or :edit. Requires a clean working tree.",
		Spec: argv.Spec{
			Named: argv.Opts("message", "m", "onto"),
			Flags: argv.Opts("edit", "abort-on-conflict"),
		},
		Run: runSqr,
	})
}

func runSqr(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out a branch to squash-rebase")
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to squash-rebase the trunk branch %q onto itself", branch)
	}
	if clean, err := e.repo.IsClean(); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	// Default target is trunk; onto:BRANCH overrides it.
	onto := c.Args.GetOr("onto", "")
	targetRef := e.trunkRef
	if onto != "" {
		if !e.repo.Exists(onto) {
			return fmt.Errorf("onto branch %q does not exist", onto)
		}
		if onto == branch {
			return fmt.Errorf("onto: must differ from the current branch %q", branch)
		}
		targetRef = onto
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))
	edit := c.Args.Flag("edit")

	// Squash fork..branch into one commit against whichever target we rebase onto.
	fork, err := e.repo.MergeBase(targetRef, branch)
	if err != nil {
		fork = targetRef
	}
	existing, _ := e.repo.LogRange(fork + ".." + branch)
	if err := e.squashToOne(branch, fork, len(existing), msg, edit); err != nil {
		return err
	}

	// Trunk target keeps the guard + trunk graph; a named target rebases directly.
	if onto == "" {
		return finalizeOnTrunk(e, c, branch)
	}
	if rerr := e.visualizeRebase(branch, onto, onto); rerr != nil {
		return handleRebaseConflict(e, c, branch, rerr)
	}
	return nil
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/commands/ -run TestSqr -v`
Expected: PASS (TestSqrSquashesAndRebasesOntoTrunk, TestSqrOntoNamedBranch, TestSqrOntoMissingBranch, TestSqrAfterGraphShowsPostRebaseSha, TestSqrRefusesTrunk, TestSqrRefusesDirty).

- [ ] **Step 5: Add the `onto:` conflict test**

The `onto:` path leaves a conflicting rebase in progress and `tbd continue` completes it. Append to `internal/commands/sqr_test.go`:

```go
func TestSqrOntoConflictThenContinue(t *testing.T) {
	dir := repoFixture(t)
	// base and work both touch the same file with different content.
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "shared.txt", "base version\n")
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "shared.txt", "work version\n")

	err := runSqr(mustCtx(dir, "sqr", "onto:base"))
	if err == nil {
		t.Fatal("expected a rebase conflict")
	}
	r, _ := openRepo(dir)
	if !r.RebaseInProgress() {
		t.Fatal("rebase should be left in progress after a conflict")
	}

	// Resolve the conflict and continue.
	writeFile(t, dir, "shared.txt", "resolved\n")
	gitRun(t, dir, "add", "shared.txt")
	if err := runContinue(mustCtx(dir, "continue")); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if r.RebaseInProgress() {
		t.Fatal("rebase should be finished after continue")
	}
	bh, _ := r.RevParse("base")
	wh, _ := r.RevParse("work")
	if !r.IsAncestor(bh, wh) {
		t.Fatal("work must sit on top of base after continue")
	}
}
```

- [ ] **Step 6: Run the conflict test**

Run: `go test ./internal/commands/ -run TestSqrOntoConflictThenContinue -v`
Expected: PASS.

- [ ] **Step 7: Run the whole suite**

Run: `go build ./... && go test ./...`
Expected: PASS. If anything else referenced `runRebase` or the `"rebase"` command name, fix it now (grep: `grep -rn "runRebase\|\"rebase\"" internal cmd` should return nothing).

- [ ] **Step 8: Commit**

```bash
git add internal/commands/sqr.go internal/commands/sqr_test.go
git commit -m "feat: rename rebase to sqr, add onto:BRANCH target"
```

---

### Task 3: Update docs (`README.md`, `tbd learn`)

Point every user-facing mention of the `rebase` *command* at `sqr`. Do NOT rename the general git *action* "rebase" in prose (e.g. "rebases onto trunk", "the rebase conflicts") — only the command name.

**Files:**
- Modify: `README.md`
- Modify: `internal/commands/learn.go` (L220)

- [ ] **Step 1: Update the README command table row**

In `README.md`, replace the `rebase` table row (~L78):

```
| `sqr [onto:BRANCH]` | Squash the current branch to one commit and rebase it onto trunk (or onto BRANCH) | single commit on trunk |
```

- [ ] **Step 2: Update the `continue` table row wording (optional consistency)**

In `README.md` (~L77), the `continue` row says "Resume a tbd rebase". Leave the git-action word "rebase" as is; no change required. (Listed so the implementer does not "fix" it.)

- [ ] **Step 3: Update `tbd learn` narration**

In `internal/commands/learn.go` (~L220), replace:

```go
	l.p("Adopting a \"normal\" multi-commit branch? " + l.c.Green("tbd sqr") + " squashes it to one")
```

Leave all other lines mentioning the rebase *action* unchanged.

- [ ] **Step 4: Grep for any remaining `tbd rebase` command references**

Run: `grep -rn "tbd rebase" README.md internal docs`
Expected: no lines that refer to the command as an invocation (`tbd rebase`). General prose about rebasing is fine and should remain.

- [ ] **Step 5: Build and run learn test**

Run: `go build ./... && go test ./internal/commands/ -run TestLearn -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add README.md internal/commands/learn.go
git commit -m "docs: point rebase command references at sqr"
```

---

## Notes / Deferred

- `onto:` non-trunk path deliberately skips the trunk-ancestor guard (explicit override); result inherits trunk-ancestry transitively if BRANCH is trunk-based. Documented in the spec, not enforced.
- `cherry-put` is untouched; its `onto:`+`as:` new-branch flow stays distinct from `sqr`'s in-place rebase.
- Version bump / tag (per tbd dogfood release process) is a separate follow-up, not part of this plan.
