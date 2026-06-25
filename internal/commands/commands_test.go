package commands

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/git"
)

func openRepo(dir string) (*git.Repo, error) { return git.Open(dir) }

// repoFixture is a temp git repo with trunk "develop", one commit, and a written
// .tbd.yaml. No remote (so commands run offline / local).
func repoFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "develop")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	writeAndCommit(t, dir, "a.txt", "first")
	cfg := "trunk-name: develop\nfeature-prefix: feature/\nlease-tags: [dev-deploy]\n"
	if err := os.WriteFile(filepath.Join(dir, ".tbd.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeAndCommit(t *testing.T, dir, file, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

// newCtx builds a Context rooted at dir with captured output. color-mode:none
// keeps assertions free of ANSI codes.
func newCtx(dir string, argv ...string) (*cli.Context, *bytes.Buffer, *bytes.Buffer) {
	args := cli.Parse(append(argv, "color-mode:none"))
	var out, errb bytes.Buffer
	return &cli.Context{Args: args, Stdout: &out, Stderr: &errb, Dir: dir}, &out, &errb
}

func TestGuardPassesOnTrunk(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "guard")
	if err := runGuard(ctx); err != nil {
		t.Fatalf("guard should pass: %v", err)
	}
	if !strings.Contains(out.String(), "ancestor") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestGuardFailsOnDivergence(t *testing.T) {
	dir := repoFixture(t)
	// feature branch, then advance trunk so the feature diverges.
	gitRun(t, dir, "branch", "feature/x", "develop")
	gitRun(t, dir, "switch", "-q", "feature/x")
	writeAndCommit(t, dir, "b.txt", "feature work")
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "c.txt", "trunk moves")
	gitRun(t, dir, "switch", "-q", "feature/x")

	ctx, _, _ := newCtx(dir, "guard")
	err := runGuard(ctx)
	var ee cli.ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("expected ExitError{1}, got %v", err)
	}
}

func TestStatusShowsTrunkAndLease(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "status")
	if err := runStatus(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"trunk", "develop", "leases", "dev-deploy", "(unset)"} {
		if !strings.Contains(s, want) {
			t.Fatalf("status missing %q:\n%s", want, s)
		}
	}
}

func TestFeatureStartAndFinishLocal(t *testing.T) {
	dir := repoFixture(t)

	// start
	ctx, _, _ := newCtx(dir, "feature", "start", "login")
	if err := featureStart(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	writeAndCommit(t, dir, "login.txt", "login work")

	// finish (local, no push). Trunk should fast-forward and branch be deleted.
	ctx2, out2, _ := newCtx(dir, "feature", "finish", ":no-push")
	if err := featureFinish(ctx2); err != nil {
		t.Fatalf("finish: %v\n%s", err, out2.String())
	}
	r, _ := openRepo(dir)
	if r.Exists("feature/login") {
		t.Fatal("feature branch should be deleted")
	}
	// develop head subject should be the feature commit.
	if subj := r.Subject("develop"); subj != "login work" {
		t.Fatalf("develop head = %q, want feature commit", subj)
	}
}

// Regression for bug 0002: an invalid feature name must be rejected with a
// clean tbd error before any network fetch, not surfaced as raw git porcelain.
func TestFeatureStartRejectsInvalidName(t *testing.T) {
	dir := repoFixture(t)
	for _, bad := range []string{"has space", "a..b", "a/"} {
		ctx, _, _ := newCtx(dir, "feature", "start", bad, ":local")
		err := featureStart(ctx)
		if err == nil {
			t.Fatalf("name %q: expected rejection, got nil", bad)
		}
		if !strings.Contains(err.Error(), "not a valid feature name") {
			t.Fatalf("name %q: want clean validation error, got %v", bad, err)
		}
		if r, _ := openRepo(dir); r.Exists("feature/" + bad) {
			t.Fatalf("name %q: no branch should have been created", bad)
		}
	}
}

func TestFeatureFinishAutoRebaseVisualized(t *testing.T) {
	dir := repoFixture(t)
	ctx, _, _ := newCtx(dir, "feature", "start", "x")
	if err := featureStart(ctx); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, "x.txt", "x work")
	// Advance trunk behind the feature's back to force a rebase.
	gitRun(t, dir, "switch", "-q", "develop")
	writeAndCommit(t, dir, "t.txt", "trunk advances")
	gitRun(t, dir, "switch", "-q", "feature/x")

	ctx2, out2, _ := newCtx(dir, "feature", "finish", ":no-push")
	if err := featureFinish(ctx2); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !strings.Contains(out2.String(), "Rebasing") || !strings.Contains(out2.String(), "before") {
		t.Fatalf("expected rebase visualization, got:\n%s", out2.String())
	}
}

func TestLeaseTakeLocal(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "lease", "take", "dev-deploy")
	if err := runLease(ctx); err != nil {
		t.Fatalf("lease take: %v", err)
	}
	if !strings.Contains(out.String(), "dev-deploy ->") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	r, _ := openRepo(dir)
	if _, ok := r.TagInfo("dev-deploy"); !ok {
		t.Fatal("lease tag not created")
	}
}

func TestReleaseCutBranchAndTag(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "release", "cut", "1.0.0", "strategy:branch,tag")
	if err := releaseCut(ctx); err != nil {
		t.Fatalf("release cut: %v\n%s", err, out.String())
	}
	r, _ := openRepo(dir)
	if !r.Exists("release/1.0.0") {
		t.Fatal("release branch missing")
	}
	if !r.Exists("v1.0.0") {
		t.Fatal("release tag missing")
	}
}
