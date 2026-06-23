package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestRepo creates an initialized git repo in a temp dir with one commit on
// the default branch, returning the Repo. Tests are skipped if git is absent.
func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "develop")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commit(t, r, "a.txt", "one", "first")
	return r
}

// commit writes a file and commits it, returning the new sha.
func commit(t *testing.T, r *Repo, file, content, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, r, "add", "-A")
	mustRun(t, r, "commit", "-q", "-m", msg)
	sha, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return sha
}

func mustRun(t *testing.T, r *Repo, args ...string) string {
	t.Helper()
	out, err := r.run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func TestIsAncestorAndRevParse(t *testing.T) {
	r := newTestRepo(t)
	base, _ := r.RevParse("HEAD")
	second := commit(t, r, "b.txt", "two", "second")

	if !r.IsAncestor(base, second) {
		t.Fatal("base should be ancestor of second")
	}
	if r.IsAncestor(second, base) {
		t.Fatal("second should not be ancestor of base")
	}
	if _, err := r.RevParse("nope"); err == nil {
		t.Fatal("expected error for missing ref")
	}
}

func TestCurrentBranchAndDetached(t *testing.T) {
	r := newTestRepo(t)
	br, err := r.CurrentBranch()
	if err != nil || br != "develop" {
		t.Fatalf("branch = %q err = %v", br, err)
	}
	sha, _ := r.RevParse("HEAD")
	mustRun(t, r, "checkout", "-q", sha)
	if _, err := r.CurrentBranch(); err == nil {
		t.Fatal("expected detached HEAD error")
	}
}

func TestCleanAndDirty(t *testing.T) {
	r := newTestRepo(t)
	clean, err := r.IsClean()
	if err != nil || !clean {
		t.Fatalf("expected clean, got %v err %v", clean, err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean, _ = r.IsClean()
	if clean {
		t.Fatal("expected dirty after edit")
	}
}

func TestFFMerge(t *testing.T) {
	r := newTestRepo(t)
	// Make a feature branch with one extra commit.
	if err := r.BranchCreate("feature/x", "develop"); err != nil {
		t.Fatal(err)
	}
	mustRun(t, r, "switch", "-q", "feature/x")
	commit(t, r, "c.txt", "three", "feature work")
	mustRun(t, r, "switch", "-q", "develop")
	if err := r.FFMerge("feature/x"); err != nil {
		t.Fatalf("ff merge should succeed: %v", err)
	}

	// Now create divergence: a commit on develop and a separate one on a branch.
	r.BranchCreate("feature/y", "develop")
	commit(t, r, "d.txt", "d", "develop moves")
	mustRun(t, r, "switch", "-q", "feature/y")
	commit(t, r, "e.txt", "e", "branch moves")
	mustRun(t, r, "switch", "-q", "develop")
	if err := r.FFMerge("feature/y"); err == nil {
		t.Fatal("ff merge of diverged branch should fail")
	}
}

func TestTagAnnotatedAndInfo(t *testing.T) {
	r := newTestRepo(t)
	head, _ := r.RevParse("HEAD")
	if err := r.TagAnnotated("dev-deploy", head, "deploy lease"); err != nil {
		t.Fatal(err)
	}
	d, ok := r.TagInfo("dev-deploy")
	if !ok {
		t.Fatal("tag info not found")
	}
	if d.Tagger != "Test" {
		t.Fatalf("tagger = %q, want Test", d.Tagger)
	}
	if d.Subject != "deploy lease" {
		t.Fatalf("subject = %q", d.Subject)
	}
	if _, ok := r.TagInfo("absent"); ok {
		t.Fatal("expected absent tag to report not found")
	}
}

func TestAheadBehindAndLog(t *testing.T) {
	r := newTestRepo(t)
	r.BranchCreate("feature/x", "develop")
	commit(t, r, "d.txt", "d", "develop ahead")
	mustRun(t, r, "switch", "-q", "feature/x")
	commit(t, r, "e.txt", "e", "feature ahead")

	ahead, behind, err := r.AheadBehind("develop", "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	// develop...feature/x : develop has 1 unique, feature/x has 1 unique.
	if ahead != 1 || behind != 1 {
		t.Fatalf("ahead/behind = %d/%d, want 1/1", ahead, behind)
	}
	commits, err := r.LogRange("develop..feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].Subject != "feature ahead" {
		t.Fatalf("log range = %+v", commits)
	}
}
