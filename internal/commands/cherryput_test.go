package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPutReplaysOneCommitLinearly(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "base.txt", "base work")
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
	// Exactly one commit on top of base...
	if n, _ := gitCapture(dir, "rev-list", "--count", "base..result"); n != "1" {
		t.Fatalf("expected 1 commit over base, got %q", n)
	}
	// ...and NO merge commit (linear, as if rebased).
	if n, _ := gitCapture(dir, "rev-list", "--count", "--merges", "base..result"); n != "0" {
		t.Fatalf("result should have no merge commit, got %q merges", n)
	}
	// The source branch is squashed to a single commit.
	if n := commitCount(t, dir, "work"); n != 1 {
		t.Fatalf("work should be squashed to 1 commit, got %d", n)
	}
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

func TestCherryPutConflictThenContinue(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "shared.txt", "base version\n")
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "shared.txt", "work version\n")

	err := runCherryPut(mustCtx(dir, "cherry-put", "onto:base", "as:result"))
	if err == nil {
		t.Fatal("expected a cherry-pick conflict")
	}
	r, _ := openRepo(dir)
	if !r.CherryPickInProgress() {
		t.Fatal("expected cherry-pick left in progress")
	}
	if cur, _ := r.CurrentBranch(); cur != "result" {
		t.Fatalf("conflict should leave us on result, on %q", cur)
	}

	// Resolve and continue.
	writeFile(t, dir, "shared.txt", "resolved\n")
	gitRun(t, dir, "add", "shared.txt")
	if err := runContinue(mustCtx(dir, "continue")); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if r.CherryPickInProgress() {
		t.Fatal("cherry-pick should be finished")
	}
	if n, _ := gitCapture(dir, "rev-list", "--count", "base..result"); n != "1" {
		t.Fatalf("expected 1 commit on result after continue, got %q", n)
	}
}

func TestCherryPutKeepSource(t *testing.T) {
	dir := repoFixture(t)
	gitRun(t, dir, "switch", "-q", "-c", "base", "develop")
	writeAndCommit(t, dir, "base.txt", "base work")
	gitRun(t, dir, "switch", "-q", "-c", "work", "develop")
	writeAndCommit(t, dir, "work1.txt", "w1")
	writeAndCommit(t, dir, "work2.txt", "w2")

	if err := runCherryPut(mustCtx(dir, "cherry-put", "onto:base", "as:result", ":keep-source")); err != nil {
		t.Fatalf("cherry-put :keep-source: %v", err)
	}
	if n, _ := gitCapture(dir, "rev-list", "--count", "base..result"); n != "1" {
		t.Fatalf("expected 1 commit over base, got %q", n)
	}
	if n, _ := gitCapture(dir, "rev-list", "--count", "--merges", "base..result"); n != "0" {
		t.Fatalf("result should be linear, got %q merges", n)
	}
	// Source must be UNCHANGED (still its two commits).
	if n := commitCount(t, dir, "work"); n != 2 {
		t.Fatalf(":keep-source must leave work untouched (2 commits), got %d", n)
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
