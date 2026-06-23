package commands

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// writeFile writes (without committing) so the next tbd commit picks it up.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitCount(t *testing.T, dir, branch string) int {
	t.Helper()
	r, err := openRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := r.MergeBase("develop", branch)
	if err != nil {
		t.Fatalf("merge-base: %v", err)
	}
	commits, err := r.LogRange(fork + ".." + branch)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	return len(commits)
}

func TestCommitRefusesOnTrunk(t *testing.T) {
	dir := repoFixture(t)
	writeFile(t, dir, "x.txt", "x")
	ctx, _, _ := newCtx(dir, "commit", "message:x")
	err := runCommit(ctx)
	if err == nil || !strings.Contains(err.Error(), "refusing to commit on the trunk") {
		t.Fatalf("expected trunk refusal, got %v", err)
	}
}

func TestCommitFirstNeedsMessage(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "x.txt", "x")
	ctx, _, _ := newCtx(dir, "commit")
	if err := runCommit(ctx); err == nil || !strings.Contains(err.Error(), "first commit needs a message") {
		t.Fatalf("expected message-required error, got %v", err)
	}
}

func TestCommitAmendPreservesMessage(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	ctx, _, _ := newCtx(dir, "commit", "message:first work")
	if err := runCommit(ctx); err != nil {
		t.Fatal(err)
	}
	// Second commit, no message: amends, keeps message, still one commit.
	writeFile(t, dir, "b.txt", "b")
	ctx2, _, _ := newCtx(dir, "commit")
	if err := runCommit(ctx2); err != nil {
		t.Fatal(err)
	}
	if n := commitCount(t, dir, "feature/p"); n != 1 {
		t.Fatalf("expected 1 commit, got %d", n)
	}
	r, _ := openRepo(dir)
	if subj := r.Subject("feature/p"); subj != "first work" {
		t.Fatalf("message not preserved: %q", subj)
	}
	// Both files must be present in the single commit.
	for _, f := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
}

func TestCommitSquashesMany(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	// Three ad-hoc commits made outside tbd.
	writeAndCommit(t, dir, "1.txt", "one")
	writeAndCommit(t, dir, "2.txt", "two")
	writeAndCommit(t, dir, "3.txt", "three")
	if n := commitCount(t, dir, "feature/p"); n != 3 {
		t.Fatalf("setup expected 3 commits, got %d", n)
	}
	writeFile(t, dir, "4.txt", "four")
	ctx, _, _ := newCtx(dir, "commit")
	if err := runCommit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := commitCount(t, dir, "feature/p"); n != 1 {
		t.Fatalf("expected squash to 1 commit, got %d", n)
	}
}

// startFeatureBare creates feature/NAME off develop with no commit yet.
func startFeatureBare(t *testing.T, dir, name string) {
	t.Helper()
	ctx, _, _ := newCtx(dir, "feature", "start", name)
	if err := featureStart(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
}

// TestCommitAlwaysSingleAndRebased is the core property: no matter how many
// commits the feature already has or how far trunk has advanced, after
// `tbd commit` the feature is exactly one commit sitting on top of trunk.
func TestCommitAlwaysSingleAndRebased(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		pre := rapid.IntRange(0, 3).Draw(rt, "preCommits")
		adv := rapid.IntRange(0, 2).Draw(rt, "trunkAdvance")

		dir := repoFixture(t)
		startFeatureBare(t, dir, "p")
		for i := 0; i < pre; i++ {
			writeAndCommit(t, dir, "pre"+strconv.Itoa(i)+".txt", "pre"+strconv.Itoa(i))
		}
		if adv > 0 {
			gitRun(t, dir, "switch", "-q", "develop")
			for i := 0; i < adv; i++ {
				writeAndCommit(t, dir, "adv"+strconv.Itoa(i)+".txt", "adv"+strconv.Itoa(i))
			}
			gitRun(t, dir, "switch", "-q", "feature/p")
		}
		writeFile(t, dir, "wip.txt", "wip")

		ctx, _, _ := newCtx(dir, "commit", "message:work")
		if err := runCommit(ctx); err != nil {
			rt.Fatalf("commit (pre=%d adv=%d): %v", pre, adv, err)
		}

		if n := commitCount(t, dir, "feature/p"); n != 1 {
			rt.Fatalf("expected exactly 1 commit, got %d (pre=%d adv=%d)", n, pre, adv)
		}
		r, _ := openRepo(dir)
		th, _ := r.RevParse("develop")
		fh, _ := r.RevParse("feature/p")
		if !r.IsAncestor(th, fh) {
			rt.Fatalf("trunk head must be ancestor of feature after commit (pre=%d adv=%d)", pre, adv)
		}
	})
}
