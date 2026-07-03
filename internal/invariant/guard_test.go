package invariant

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"goforge.dev/tbd/v2/internal/git"
)

func newRepo(t *testing.T) *git.Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "develop")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commit(t, dir, "a.txt", "first")
	return r
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, dir, file, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

func TestEnsurePassesWhenRebased(t *testing.T) {
	r := newRepo(t)
	gitRun(t, r.Dir, "branch", "feature/x", "develop")
	gitRun(t, r.Dir, "switch", "-q", "feature/x")
	commit(t, r.Dir, "b.txt", "feature work")

	g := Guard{Repo: r, Trunk: "develop", RequireClean: true}
	if err := g.Ensure("feature/x"); err != nil {
		t.Fatalf("expected invariant to hold: %v", err)
	}
}

func TestEnsureDivergedWhenTrunkMovesAhead(t *testing.T) {
	r := newRepo(t)
	gitRun(t, r.Dir, "branch", "feature/x", "develop")
	gitRun(t, r.Dir, "switch", "-q", "feature/x")
	commit(t, r.Dir, "b.txt", "feature work")
	// Advance develop past the fork point.
	gitRun(t, r.Dir, "switch", "-q", "develop")
	commit(t, r.Dir, "c.txt", "trunk moves")

	g := Guard{Repo: r, Trunk: "develop"}
	if err := g.Ensure("feature/x"); !errors.Is(err, ErrDiverged) {
		t.Fatalf("expected ErrDiverged, got %v", err)
	}
}

func TestEnsureDirty(t *testing.T) {
	r := newRepo(t)
	if err := os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := Guard{Repo: r, Trunk: "develop", RequireClean: true}
	if err := g.Ensure("develop"); !errors.Is(err, ErrDirty) {
		t.Fatalf("expected ErrDirty, got %v", err)
	}
}

func TestEnsureNoTrunk(t *testing.T) {
	r := newRepo(t)
	g := Guard{Repo: r, Trunk: "nonexistent"}
	if err := g.Ensure("develop"); !errors.Is(err, ErrNoTrunk) {
		t.Fatalf("expected ErrNoTrunk, got %v", err)
	}
}

func TestOnTrunk(t *testing.T) {
	r := newRepo(t)
	// A release tag on a trunk commit is on trunk.
	head, _ := r.RevParse("develop")
	gitRun(t, r.Dir, "tag", "v1", head)
	g := Guard{Repo: r, Trunk: "develop"}
	on, err := g.OnTrunk("v1")
	if err != nil || !on {
		t.Fatalf("v1 should be on trunk: on=%v err=%v", on, err)
	}
	// A commit on a side branch not merged is off trunk.
	gitRun(t, r.Dir, "switch", "-q", "-c", "side")
	commit(t, r.Dir, "s.txt", "side work")
	on, _ = g.OnTrunk("side")
	if on {
		t.Fatal("side branch should be off trunk")
	}
}

func TestCheckReport(t *testing.T) {
	r := newRepo(t)
	gitRun(t, r.Dir, "branch", "feature/x", "develop")
	gitRun(t, r.Dir, "switch", "-q", "feature/x")
	commit(t, r.Dir, "b.txt", "feature work")

	g := Guard{Repo: r, Trunk: "develop"}
	rep, err := g.Check("feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Diverged {
		t.Fatal("should not be diverged")
	}
	if rep.Ahead != 1 {
		t.Fatalf("ahead = %d, want 1", rep.Ahead)
	}
}
