package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployMutexLeaseStealRelinquish(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	alice := filepath.Join(root, "alice")
	bob := filepath.Join(root, "bob")

	gitCmd(t, root, "init", "-q", "--bare", "-b", "main", origin)
	gitCmd(t, root, "clone", "-q", origin, alice)
	configDemoUser(t, alice, "alice", "alice@example.com")
	write(t, alice, "README.md", "demo\n")
	gitCmd(t, alice, "add", "-A")
	gitCmd(t, alice, "commit", "-q", "-m", "chore: seed")
	if code, out, errOut := runIn(t, alice, "init", "--yes"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	gitCmd(t, alice, "add", "-A")
	gitCmd(t, alice, "commit", "-q", "-m", "chore: configure tbd")
	gitCmd(t, alice, "push", "-q", "-u", "origin", "main")

	gitCmd(t, root, "clone", "-q", origin, bob)
	configDemoUser(t, bob, "bob", "bob@example.com")

	if code, out, errOut := runIn(t, alice, "feature", "--id", "A-1", "--desc", "Payments"); code != 0 {
		t.Fatalf("alice feature failed: %s %s", out, errOut)
	}
	write(t, alice, "payments.txt", "payments v1\n")
	if code, out, errOut := runIn(t, alice, "commit", "--no-edit"); code != 0 {
		t.Fatalf("alice commit failed: %s %s", out, errOut)
	}
	if code, out, errOut := runIn(t, alice, "lease", "dev-deploy"); code != 0 {
		t.Fatalf("alice lease failed: %s %s", out, errOut)
	}
	aliceHead := gitOut(t, alice, "rev-parse", "HEAD")
	if got := remoteTagCommit(t, bob, "dev-deploy"); got != aliceHead {
		t.Fatalf("dev-deploy = %s, want alice %s", got, aliceHead)
	}

	if code, out, errOut := runIn(t, bob, "fix", "--desc", "Patch cache"); code != 0 {
		t.Fatalf("bob fix failed: %s %s", out, errOut)
	}
	write(t, bob, "cache.txt", "cache patch\n")
	if code, out, errOut := runIn(t, bob, "commit", "--no-edit"); code != 0 {
		t.Fatalf("bob commit failed: %s %s", out, errOut)
	}
	code, out, errOut := runIn(t, bob, "lease", "dev-deploy")
	if code == 0 || !strings.Contains(out+errOut, "use tbd steal dev-deploy") {
		t.Fatalf("bob lease should refuse foreign holder, code=%d out=%s err=%s", code, out, errOut)
	}
	if got := remoteTagCommit(t, bob, "dev-deploy"); got != aliceHead {
		t.Fatalf("failed lease moved dev-deploy: %s want %s", got, aliceHead)
	}

	if code, out, errOut := runIn(t, bob, "steal", "dev-deploy"); code != 0 {
		t.Fatalf("bob steal failed: %s %s", out, errOut)
	}
	bobHead := gitOut(t, bob, "rev-parse", "HEAD")
	if got := remoteTagCommit(t, bob, "dev-deploy"); got != bobHead {
		t.Fatalf("dev-deploy = %s, want bob %s", got, bobHead)
	}

	code, out, errOut = runIn(t, alice, "relinquish", "dev-deploy")
	if code == 0 || !strings.Contains(out+errOut, "held by") {
		t.Fatalf("alice relinquish should refuse foreign holder, code=%d out=%s err=%s", code, out, errOut)
	}
	if code, out, errOut := runIn(t, bob, "relinquish", "dev-deploy"); code != 0 {
		t.Fatalf("bob relinquish failed: %s %s", out, errOut)
	}
	trunk := gitOut(t, bob, "rev-parse", "origin/main")
	if got := remoteTagCommit(t, bob, "dev-deploy"); got != trunk {
		t.Fatalf("dev-deploy = %s, want trunk %s", got, trunk)
	}
}

func configDemoUser(t *testing.T, dir, name, email string) {
	t.Helper()
	gitCmd(t, dir, "config", "user.name", name)
	gitCmd(t, dir, "config", "user.email", email)
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	gitCmd(t, dir, "config", "tag.gpgsign", "false")
}

func remoteTagCommit(t *testing.T, dir, tag string) string {
	t.Helper()
	gitCmd(t, dir, "fetch", "-q", "--tags", "--force", "origin")
	return gitOut(t, dir, "rev-parse", tag+"^{commit}")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
