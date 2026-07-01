package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"goforge.dev/tbd/internal/cli"
)

// setupConflict creates feature/c and trunk commits that edit the SAME file in
// incompatible ways, so a rebase of the feature onto trunk must conflict.
func setupConflict(t *testing.T, dir string) {
	t.Helper()
	ctx, _, _ := newCtx(dir, "feature", "start", "c")
	if err := featureStart(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	writeAndCommit(t, dir, "shared.txt", "feature version")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "shared.txt", "trunk version")
	gitRun(t, dir, "switch", "-q", "feature/c")
}

func rebaseInProgress(dir string) bool {
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(dir, ".git", d)); err == nil {
			return true
		}
	}
	return false
}

func TestFinishConflictLeavesRebaseInProgress(t *testing.T) {
	dir := repoFixture(t)
	setupConflict(t, dir)

	ctx, _, _ := newCtx(dir, "feature", "finish", ":no-push")
	err := featureFinish(ctx)

	var ee cli.ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("expected ExitError{1} on conflict, got %v", err)
	}
	if !rebaseInProgress(dir) {
		t.Fatal("expected the rebase to be left in progress for the user to resolve")
	}
	// Clean up so t.TempDir removal is not blocked by rebase state.
	gitRun(t, dir, "rebase", "--abort")
}

// Regression for bug 0004: after a finish hits a conflict, resolving it and
// running tbd continue must complete the finish (fast-forward trunk, delete the
// branch), not just finish the low-level rebase.
func TestFinishConflictContinueCompletesFinish(t *testing.T) {
	dir := repoFixture(t)
	setupConflict(t, dir)
	featSubject := "feature version" // the feature commit's content/subject marker

	// Finish conflicts and leaves the rebase in progress, with a resume record.
	fctx, _, _ := newCtx(dir, "feature", "finish", ":no-push")
	if err := featureFinish(fctx); err == nil {
		t.Fatal("expected finish to conflict")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tbd-resume")); err != nil {
		t.Fatalf("expected a resume record after the conflicting finish: %v", err)
	}

	// Resolve the conflict and continue.
	writeFile(t, dir, "shared.txt", "resolved")
	gitRun(t, dir, "add", "shared.txt")
	cctx, _, _ := newCtx(dir, "continue")
	if err := runContinue(cctx); err != nil {
		t.Fatalf("continue: %v", err)
	}

	r, _ := openRepo(dir)
	// The finish must have completed: branch gone, trunk advanced to the feature.
	if r.Exists("feature/c") {
		t.Fatal("feature/c should be deleted after continue resumes the finish")
	}
	if subj := r.Subject("develop"); subj != featSubject {
		t.Fatalf("develop head = %q, want the resumed feature commit %q", subj, featSubject)
	}
	cur, _ := r.CurrentBranch()
	if cur != "develop" {
		t.Fatalf("expected to land on develop after finish, got %q", cur)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tbd-resume")); !os.IsNotExist(err) {
		t.Fatal("resume record should be cleared after a completed finish")
	}
}

// Regression for bug 0005: a resume record orphaned by a raw "git rebase
// --abort" must not hijack a later, unrelated tbd continue on a different branch
// into a destructive finish.
func TestContinueIgnoresStaleResumeOnOtherBranch(t *testing.T) {
	dir := repoFixture(t)

	// 1) feature/aaa finish conflicts -> writes a resume record.
	ctx, _, _ := newCtx(dir, "feature", "start", "aaa")
	if err := featureStart(ctx); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, "shared.txt", "aaa version")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "shared.txt", "trunk one")
	gitRun(t, dir, "switch", "-q", "feature/aaa")
	if err := featureFinish(mustCtx(dir, "feature", "finish", ":no-push")); err == nil {
		t.Fatal("expected aaa finish to conflict")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tbd-resume")); err != nil {
		t.Fatalf("expected a resume record: %v", err)
	}

	// 2) user backs out with raw git -> the resume record is left stale.
	gitRun(t, dir, "rebase", "--abort")

	// 3) an unrelated plain rebase on feature/bbb conflicts.
	gitRun(t, dir, "switch", "-q", "develop")
	bctx, _, _ := newCtx(dir, "feature", "start", "bbb")
	if err := featureStart(bctx); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, "shared.txt", "bbb version")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "shared.txt", "trunk two")
	gitRun(t, dir, "switch", "-q", "feature/bbb")
	if err := runSqr(mustCtx(dir, "sqr")); err == nil {
		t.Fatal("expected bbb rebase to conflict")
	}

	// 4) resolve + continue: must finish only the rebase, NOT a stale finish.
	trunkBefore := revParse(t, dir, "develop")
	writeFile(t, dir, "shared.txt", "resolved")
	gitRun(t, dir, "add", "shared.txt")
	if err := runContinue(mustCtx(dir, "continue")); err != nil {
		t.Fatalf("continue: %v", err)
	}

	r, _ := openRepo(dir)
	if !r.Exists("feature/bbb") {
		t.Fatal("feature/bbb must NOT be deleted by a stale resume record")
	}
	if got := revParse(t, dir, "develop"); got != trunkBefore {
		t.Fatal("develop must NOT be fast-forwarded by a stale resume record")
	}
	if cur, _ := r.CurrentBranch(); cur != "feature/bbb" {
		t.Fatalf("expected to stay on feature/bbb, got %q", cur)
	}
}

func TestFinishConflictAbortOnFlag(t *testing.T) {
	dir := repoFixture(t)
	setupConflict(t, dir)
	featHead := revParse(t, dir, "feature/c")

	ctx, _, _ := newCtx(dir, "feature", "finish", ":no-push", ":abort-on-conflict")
	err := featureFinish(ctx)

	var ee cli.ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("expected ExitError{1}, got %v", err)
	}
	if rebaseInProgress(dir) {
		t.Fatal("expected the rebase to be aborted, but it is still in progress")
	}
	// The feature branch must be unchanged after the abort.
	if got := revParse(t, dir, "feature/c"); got != featHead {
		t.Fatalf("feature head changed after abort: %s != %s", got, featHead)
	}
}

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	r, err := openRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := r.RevParse(ref)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return sha
}
