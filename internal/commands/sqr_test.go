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

func TestSqrOntoMustDifferFromCurrentBranch(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w.txt", "w")
	err := runSqr(mustCtx(dir, "sqr", "onto:work"))
	if err == nil || !strings.Contains(err.Error(), "must differ from the current branch") {
		t.Fatalf("expected must-differ error, got %v", err)
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
