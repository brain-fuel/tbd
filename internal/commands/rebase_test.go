package commands

import (
	"strings"
	"testing"
)

func TestRebaseSquashesAndRebases(t *testing.T) {
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

	if err := runRebase(mustCtx(dir, "rebase")); err != nil {
		t.Fatalf("rebase: %v", err)
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

// Regression for bug 0001: the "after" graph must show the real post-rebase
// SHA of the replayed commit, not the stale pre-rebase one. Before the fix the
// graph was rendered from a pre-rebase snapshot, so the "after" SHA never
// existed on trunk.
func TestRebaseAfterGraphShowsPostRebaseSha(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w1.txt", "one")
	writeAndCommit(t, dir, "w2.txt", "two")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "t.txt", "trunk")
	gitRun(t, dir, "switch", "-q", "work")

	ctx, out, _ := newCtx(dir, "rebase")
	if err := runRebase(ctx); err != nil {
		t.Fatalf("rebase: %v", err)
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

func TestRebaseRefusesTrunk(t *testing.T) {
	dir := repoFixture(t) // on develop
	err := runRebase(mustCtx(dir, "rebase"))
	if err == nil || !strings.Contains(err.Error(), "refusing to rebase the trunk") {
		t.Fatalf("expected trunk refusal, got %v", err)
	}
}

func TestRebaseRefusesDirty(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w.txt", "w")
	writeFile(t, dir, "w.txt", "dirty change") // uncommitted
	err := runRebase(mustCtx(dir, "rebase"))
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("expected dirty refusal, got %v", err)
	}
}
