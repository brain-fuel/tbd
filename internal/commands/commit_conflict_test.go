package commands

import (
	"errors"
	"strings"
	"testing"

	"goforge.dev/tbd/v2/internal/cli"
)

// setupCommitConflict makes feature/c carry a committed change to a.txt, then
// advances trunk with an incompatible change to the same file, so the next
// `tbd commit` rebase must conflict. Leaves HEAD on feature/c.
func setupCommitConflict(t *testing.T, dir string) {
	t.Helper()
	startFeatureBare(t, dir, "c")
	writeFile(t, dir, "a.txt", "feature version\n")
	if err := runCommit(mustCtx(dir, "commit", "message:feature work")); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "a.txt", "trunk version\n")
	gitRun(t, dir, "switch", "-q", "feature/c")
}

func TestCommitConflictLeavesRebaseInProgress(t *testing.T) {
	dir := repoFixture(t)
	setupCommitConflict(t, dir)

	err := runCommit(mustCtx(dir, "commit"))
	var ee cli.ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("expected ExitError{1} on conflict, got %v", err)
	}
	if !rebaseInProgress(dir) {
		t.Fatal("expected rebase left in progress")
	}
	// The feature commit was made before the rebase, so the work is not lost.
	if !committedContains(t, dir, "a.txt") {
		t.Fatal("feature change should already be committed")
	}
	gitRun(t, dir, "rebase", "--abort")
}

func TestCommitConflictAbortKeepsCommit(t *testing.T) {
	dir := repoFixture(t)
	setupCommitConflict(t, dir)

	err := runCommit(mustCtx(dir, "commit", ":abort-on-conflict"))
	var ee cli.ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("expected ExitError{1}, got %v", err)
	}
	if rebaseInProgress(dir) {
		t.Fatal("expected rebase aborted")
	}
	// One commit survives; it is NOT rebased onto trunk (still diverged).
	if n := commitCount(t, dir, "feature/c"); n != 1 {
		t.Fatalf("expected 1 commit after abort, got %d", n)
	}
	r, _ := openRepo(dir)
	th, _ := r.RevParse("develop")
	fh, _ := r.RevParse("feature/c")
	if r.IsAncestor(th, fh) {
		t.Fatal("after abort the feature should still be diverged from trunk")
	}
}

func TestContinueResolvesCommitConflict(t *testing.T) {
	dir := repoFixture(t)
	setupCommitConflict(t, dir)

	// Trigger the conflict.
	if err := runCommit(mustCtx(dir, "commit")); err == nil {
		t.Fatal("expected conflict")
	}

	// Continue while unresolved must refuse.
	if err := runContinue(mustCtx(dir, "continue")); err == nil {
		t.Fatal("continue should refuse while conflicts are unresolved")
	}

	// Resolve, stage, continue.
	writeFile(t, dir, "a.txt", "resolved\n")
	gitRun(t, dir, "add", "a.txt")
	if err := runContinue(mustCtx(dir, "continue")); err != nil {
		t.Fatalf("continue: %v", err)
	}

	if rebaseInProgress(dir) {
		t.Fatal("rebase should be complete")
	}
	if n := commitCount(t, dir, "feature/c"); n != 1 {
		t.Fatalf("expected exactly 1 commit after continue, got %d", n)
	}
	r, _ := openRepo(dir)
	th, _ := r.RevParse("develop")
	fh, _ := r.RevParse("feature/c")
	if !r.IsAncestor(th, fh) {
		t.Fatal("feature must sit on top of trunk after continue")
	}
}

func TestStatusShowsRebaseInProgress(t *testing.T) {
	dir := repoFixture(t)
	setupCommitConflict(t, dir)
	if err := runCommit(mustCtx(dir, "commit")); err == nil {
		t.Fatal("expected conflict")
	}
	ctx, out, _ := newCtx(dir, "status")
	if err := runStatus(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rebase in progress") {
		t.Fatalf("status should report the rebase in progress:\n%s", out.String())
	}
	gitRun(t, dir, "rebase", "--abort")
}

func TestContinueNoRebase(t *testing.T) {
	dir := repoFixture(t)
	if err := runContinue(mustCtx(dir, "continue")); err == nil {
		t.Fatal("expected error when no rebase is in progress")
	}
}

func TestContinueAbort(t *testing.T) {
	dir := repoFixture(t)
	setupCommitConflict(t, dir)
	if err := runCommit(mustCtx(dir, "commit")); err == nil {
		t.Fatal("expected conflict")
	}
	if err := runContinue(mustCtx(dir, "continue", ":abort")); err != nil {
		t.Fatalf("continue :abort: %v", err)
	}
	if rebaseInProgress(dir) {
		t.Fatal("rebase should be aborted")
	}
}

// committedContains reports whether file exists in the branch's tip commit.
func committedContains(t *testing.T, dir, file string) bool {
	t.Helper()
	_, err := gitCapture(dir, "cat-file", "-e", "HEAD:"+file)
	return err == nil
}
