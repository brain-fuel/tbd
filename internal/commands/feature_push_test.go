package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goforge.dev/tbd/v2/internal/cli"
)

// mustCtx builds a Context rooted at dir (color off), discarding the buffers.
func mustCtx(dir string, argv ...string) *cli.Context {
	ctx, _, _ := newCtx(dir, argv...)
	return ctx
}

// gitCapture runs git in dir and returns trimmed stdout.
func gitCapture(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// remoteFixture builds a bare origin plus a working clone on develop with a
// .tbd.yaml, and returns the clone dir. The clone has an "origin" remote, so
// load() treats commands as network-aware.
func remoteFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "clone")

	gitRun(t, root, "init", "-q", "--bare", "-b", "develop", origin)
	gitRun(t, root, "clone", "-q", origin, clone)
	gitRun(t, clone, "config", "user.email", "dev@example.com")
	gitRun(t, clone, "config", "user.name", "dev")
	gitRun(t, clone, "config", "commit.gpgsign", "false")
	gitRun(t, clone, "config", "tag.gpgsign", "false")
	writeAndCommit(t, clone, "seed.txt", "seed")
	gitRun(t, clone, "push", "-q", "-u", "origin", "develop")
	if err := os.WriteFile(filepath.Join(clone, ".tbd.yaml"),
		[]byte("trunk-name: develop\nfeature-prefix: feature/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestFeaturePushPublishesAndUpdates(t *testing.T) {
	dir := remoteFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// First publish.
	if err := featurePush(mustCtx(dir, "feature", "push")); err != nil {
		t.Fatalf("push: %v", err)
	}
	r, _ := openRepo(dir)
	if !r.RemoteHasBranch("origin", "feature/p") {
		t.Fatal("origin should have feature/p after push")
	}
	want, _ := r.RevParse("feature/p")
	if got := remoteBranchSha(t, dir, "feature/p"); got != want {
		t.Fatalf("origin feature/p = %s, want %s", got, want)
	}

	// Amend (rewrites history) then re-push: force-with-lease must succeed.
	writeFile(t, dir, "b.txt", "b")
	if err := runCommit(mustCtx(dir, "commit")); err != nil {
		t.Fatalf("amend commit: %v", err)
	}
	if err := featurePush(mustCtx(dir, "feature", "push")); err != nil {
		t.Fatalf("re-push after amend: %v", err)
	}
	want2, _ := r.RevParse("feature/p")
	if want2 == want {
		t.Fatal("amend should have changed the feature head")
	}
	if got := remoteBranchSha(t, dir, "feature/p"); got != want2 {
		t.Fatalf("origin feature/p = %s, want %s after amend", got, want2)
	}
}

// TestFeaturePushRejectsStaleClobber proves the force-with-lease survives the
// pre-push fetch: if a collaborator moved the remote feature branch since we
// last saw it, our push is rejected instead of clobbering their work.
func TestFeaturePushRejectsStaleClobber(t *testing.T) {
	dir := remoteFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := featurePush(mustCtx(dir, "feature", "push")); err != nil {
		t.Fatalf("initial push: %v", err)
	}

	// A collaborator clones, advances feature/p, and pushes.
	originURL, err := gitCapture(dir, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	mate := filepath.Join(t.TempDir(), "mate")
	gitRun(t, t.TempDir(), "clone", "-q", originURL, mate)
	gitRun(t, mate, "config", "user.email", "mate@example.com")
	gitRun(t, mate, "config", "user.name", "mate")
	gitRun(t, mate, "config", "commit.gpgsign", "false")
	gitRun(t, mate, "switch", "-q", "feature/p")
	writeAndCommit(t, mate, "mate.txt", "mate change")
	gitRun(t, mate, "push", "-q", "origin", "feature/p")

	// We (still stale) amend and try to push: must be rejected, not clobber.
	writeFile(t, dir, "c.txt", "c")
	if err := runCommit(mustCtx(dir, "commit")); err != nil {
		t.Fatal(err)
	}
	err = featurePush(mustCtx(dir, "feature", "push"))
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected stale push to be rejected, got %v", err)
	}

	// :force overrides (sometimes you really do want to win).
	if err := featurePush(mustCtx(dir, "feature", "push", ":force")); err != nil {
		t.Fatalf("forced push should succeed: %v", err)
	}
}

func TestFeaturePushRefusesTrunk(t *testing.T) {
	dir := remoteFixture(t)
	// stay on develop
	err := featurePush(mustCtx(dir, "feature", "push"))
	if err == nil || !strings.Contains(err.Error(), "refusing to push the trunk") {
		t.Fatalf("expected trunk refusal, got %v", err)
	}
}

func TestFeaturePushNoRemote(t *testing.T) {
	dir := repoFixture(t) // no remote
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	err := featurePush(mustCtx(dir, "feature", "push"))
	if err == nil || !strings.Contains(err.Error(), "no remote") {
		t.Fatalf("expected no-remote error, got %v", err)
	}
}

// remoteBranchSha returns the sha origin holds for a branch.
func remoteBranchSha(t *testing.T, dir, branch string) string {
	t.Helper()
	out, err := gitCapture(dir, "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		t.Fatalf("ls-remote: %v", err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
