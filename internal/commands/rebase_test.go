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
