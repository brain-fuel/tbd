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
