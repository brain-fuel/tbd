package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPutCreatesNewBranch(t *testing.T) {
	dir := repoFixture(t)
	// A target branch with its own commit.
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "base.txt", "base work")
	// Our branch with two commits.
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "work1.txt", "w1")
	writeAndCommit(t, dir, "work2.txt", "w2")

	if err := runCherryPut(mustCtx(dir, "cherry-put", "onto:base", "as:result")); err != nil {
		t.Fatalf("cherry-put: %v", err)
	}
	r, _ := openRepo(dir)
	if !r.Exists("refs/heads/result") {
		t.Fatal("result branch should exist")
	}
	// result = base head + exactly one squashed commit.
	if n, _ := gitCapture(dir, "rev-list", "--count", "base..result"); n != "1" {
		t.Fatalf("expected 1 commit on result over base, got %q", n)
	}
	// Source is untouched (still its two commits).
	if n := commitCount(t, dir, "work"); n != 2 {
		t.Fatalf("work should be unchanged with 2 commits, got %d", n)
	}
	// We are left on result, carrying both base and work changes.
	cur, _ := r.CurrentBranch()
	if cur != "result" {
		t.Fatalf("expected to be on result, on %q", cur)
	}
	for _, f := range []string{"base.txt", "work1.txt", "work2.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("result should contain %s", f)
		}
	}
}

func TestCherryPutMissingArgs(t *testing.T) {
	dir := repoFixture(t)
	if err := runCherryPut(mustCtx(dir, "cherry-put", "onto:develop")); err == nil ||
		!strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error without as:, got %v", err)
	}
}

func TestCherryPutRefusesExistingTarget(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "w.txt", "w")
	err := runCherryPut(mustCtx(dir, "cherry-put", "onto:develop", "as:develop"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing-target error, got %v", err)
	}
}
