package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ephemeralCfg = "trunk-name: develop\nfeature-prefix: feature/\n" +
	"lease-strategy: ephemeral-branch\nlease-branches: [deploy-now]\n"

func ephemeralFixture(t *testing.T) string {
	t.Helper()
	dir := remoteFixture(t)
	writeConfig(t, dir, ephemeralCfg)
	return dir
}

// Every lease deletes and recreates the branch at the caller's tip; it is a
// branch, never a tag.
func TestLeaseEphemeralRemakeEveryTime(t *testing.T) {
	dir := ephemeralFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(dir, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}
	r, _ := openRepo(dir)
	c1 := revOf(t, dir, "feature/p")
	if got := r.RemoteBranchSha("origin", "deploy-now"); got != c1 {
		t.Fatalf("remote deploy-now = %s, want %s", got, c1)
	}
	if !r.Exists("refs/heads/deploy-now") {
		t.Fatal("local deploy-now branch should exist after lease")
	}
	if _, ok := r.TagInfo("deploy-now"); ok {
		t.Fatal("ephemeral lease must be a branch, not a tag")
	}

	// Amend, lease again: branch is remade at the new tip.
	writeFile(t, dir, "b.txt", "b")
	if err := runCommit(mustCtx(dir, "commit")); err != nil {
		t.Fatal(err)
	}
	c2 := revOf(t, dir, "feature/p")
	if c2 == c1 {
		t.Fatal("amend should change the tip")
	}
	if err := runLease(mustCtx(dir, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}
	if got := r.RemoteBranchSha("origin", "deploy-now"); got != c2 {
		t.Fatalf("remote deploy-now = %s, want remade at %s", got, c2)
	}
}

// A teammate takes the slot to their branch.
func TestLeaseEphemeralForeignTake(t *testing.T) {
	dir := ephemeralFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(dir, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}

	originURL, err := gitCapture(dir, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	mate := filepath.Join(t.TempDir(), "mate")
	gitRun(t, t.TempDir(), "clone", "-q", originURL, mate)
	gitRun(t, mate, "config", "user.email", "mate@example.com")
	gitRun(t, mate, "config", "user.name", "mate")
	gitRun(t, mate, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(mate, ".tbd.yaml"), []byte(ephemeralCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	startFeatureBare(t, mate, "q")
	writeFile(t, mate, "q.txt", "q")
	if err := runCommit(mustCtx(mate, "commit", "message:q")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(mate, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}
	r, _ := openRepo(mate)
	q := revOf(t, mate, "feature/q")
	if got := r.RemoteBranchSha("origin", "deploy-now"); got != q {
		t.Fatalf("teammate take: remote deploy-now = %s, want %s", got, q)
	}
}

// Any activity (here, a lease status that fetches) mirrors the remote
// lease-branch into a local ref.
func TestLeaseEphemeralSyncMirrorsRemote(t *testing.T) {
	dir := ephemeralFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(dir, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}
	want := revOf(t, dir, "feature/p")

	// Fresh clone has no local deploy-now until an activity syncs it.
	originURL, _ := gitCapture(dir, "remote", "get-url", "origin")
	mate := filepath.Join(t.TempDir(), "mate")
	gitRun(t, t.TempDir(), "clone", "-q", originURL, mate)
	gitRun(t, mate, "config", "user.email", "m@e.x")
	gitRun(t, mate, "config", "user.name", "m")
	if err := os.WriteFile(filepath.Join(mate, ".tbd.yaml"), []byte(ephemeralCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// status triggers load()'s ephemeral fetch + sync.
	if err := runLease(mustCtx(mate, "lease", "status")); err != nil {
		t.Fatal(err)
	}
	r, _ := openRepo(mate)
	if got := r.CommitOf("deploy-now"); got != want {
		t.Fatalf("local deploy-now after sync = %s, want %s", got, want)
	}
}

func TestLeaseEphemeralRefusesWhenOnIt(t *testing.T) {
	dir := ephemeralFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(dir, "lease", "deploy-now")); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "switch", "-q", "deploy-now")
	err := runLease(mustCtx(dir, "lease", "deploy-now"))
	if err == nil || !strings.Contains(err.Error(), "on the ephemeral lease branch") {
		t.Fatalf("expected refusal while on the lease branch, got %v", err)
	}
}

func TestLeaseEphemeralUnknownBranch(t *testing.T) {
	dir := ephemeralFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	if err := runLease(mustCtx(dir, "lease", "nope")); err == nil ||
		!strings.Contains(err.Error(), "not a configured lease-branch") {
		t.Fatalf("expected unknown-branch error, got %v", err)
	}
}

func TestLeaseNoneDisabled(t *testing.T) {
	dir := repoFixture(t)
	writeConfig(t, dir, "trunk-name: develop\nfeature-prefix: feature/\nlease-strategy: none\n")
	if err := runLease(mustCtx(dir, "lease", "anything")); err == nil ||
		!strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected leasing-disabled error, got %v", err)
	}
}
