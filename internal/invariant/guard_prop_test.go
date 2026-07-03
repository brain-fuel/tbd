package invariant

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"goforge.dev/tbd/v2/internal/git"
	"pgregory.net/rapid"
)

type tb interface {
	Helper()
	Fatalf(string, ...any)
}

// buildHistory makes a repo with develop + feature/x forked from a shared base,
// then numTrunk trunk-only commits and numFeat feature-only commits (uniquely
// named so rebases never conflict).
func buildHistory(t tb, dir string, numTrunk, numFeat int) *git.Repo {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	run("init", "-q", "-b", "develop")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	write("base.txt")
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	run("branch", "feature/x", "develop")
	for i := 0; i < numTrunk; i++ {
		write("t" + strconv.Itoa(i) + ".txt")
		run("add", "-A")
		run("commit", "-q", "-m", "trunk")
	}
	run("switch", "-q", "feature/x")
	for i := 0; i < numFeat; i++ {
		write("f" + strconv.Itoa(i) + ".txt")
		run("add", "-A")
		run("commit", "-q", "-m", "feat")
	}
	r, err := git.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return r
}

func tempRepo(t tb, numTrunk, numFeat int) (*git.Repo, func()) {
	dir, err := os.MkdirTemp("", "tbd-inv-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	return buildHistory(t, dir, numTrunk, numFeat), func() { os.RemoveAll(dir) }
}

// TestReportModel: Check's ahead/behind/diverged must match the constructed
// shape for any small fork.
func TestReportModel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	rapid.Check(t, func(rt *rapid.T) {
		numTrunk := rapid.IntRange(0, 4).Draw(rt, "numTrunk")
		numFeat := rapid.IntRange(0, 4).Draw(rt, "numFeat")
		r, cleanup := tempRepo(rt, numTrunk, numFeat)
		defer cleanup()

		g := Guard{Repo: r, Trunk: "develop"}
		rep, err := g.Check("feature/x")
		if err != nil {
			rt.Fatalf("check: %v", err)
		}
		if rep.Ahead != numFeat {
			rt.Fatalf("Ahead = %d, want %d", rep.Ahead, numFeat)
		}
		if rep.Behind != numTrunk {
			rt.Fatalf("Behind = %d, want %d", rep.Behind, numTrunk)
		}
		if rep.Diverged != (numTrunk > 0) {
			rt.Fatalf("Diverged = %v, want %v", rep.Diverged, numTrunk > 0)
		}
	})
}

// TestEnsureMatchesDivergence: Ensure passes iff trunk has not advanced past the
// fork (numTrunk == 0), for a clean tree.
func TestEnsureMatchesDivergence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	rapid.Check(t, func(rt *rapid.T) {
		numTrunk := rapid.IntRange(0, 4).Draw(rt, "numTrunk")
		numFeat := rapid.IntRange(0, 4).Draw(rt, "numFeat")
		r, cleanup := tempRepo(rt, numTrunk, numFeat)
		defer cleanup()

		g := Guard{Repo: r, Trunk: "develop", RequireClean: true}
		err := g.Ensure("feature/x")
		if numTrunk == 0 {
			if err != nil {
				rt.Fatalf("expected invariant to hold, got %v", err)
			}
		} else if err != ErrDiverged {
			rt.Fatalf("expected ErrDiverged, got %v", err)
		}
	})
}

// TestRebaseRestoresInvariant: after rebasing the feature onto trunk, the
// invariant always holds and the feature contains every trunk commit.
func TestRebaseRestoresInvariant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	rapid.Check(t, func(rt *rapid.T) {
		numTrunk := rapid.IntRange(0, 4).Draw(rt, "numTrunk")
		numFeat := rapid.IntRange(0, 4).Draw(rt, "numFeat")
		r, cleanup := tempRepo(rt, numTrunk, numFeat)
		defer cleanup()

		// Rebase feature/x (currently checked out) onto develop.
		if err := r.Rebase("develop"); err != nil {
			rt.Fatalf("rebase should not conflict (disjoint files): %v", err)
		}

		g := Guard{Repo: r, Trunk: "develop", RequireClean: true}
		if err := g.Ensure("feature/x"); err != nil {
			rt.Fatalf("invariant must hold after rebase, got %v", err)
		}
		trunkHead, _ := r.RevParse("develop")
		featHead, _ := r.RevParse("feature/x")
		if !r.IsAncestor(trunkHead, featHead) {
			rt.Fatalf("trunk head must be an ancestor of feature after rebase")
		}
		// Feature now carries trunk's commits plus its own beyond the base.
		ahead, behind, _ := r.AheadBehind("develop", "feature/x")
		if behind != numFeat {
			rt.Fatalf("feature-only after rebase = %d, want %d", behind, numFeat)
		}
		if ahead != 0 {
			rt.Fatalf("trunk should have no commits the feature lacks, got %d", ahead)
		}
	})
}
