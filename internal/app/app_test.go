package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2InitDefaultsAndFeature(t *testing.T) {
	dir := newV2Repo(t)
	code, out, errOut := runIn(t, dir, "init", "--yes")
	if code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".tbd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"trunk-name: main", "release:", "strategy: tag", "dev-deploy"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf(".tbd.yaml missing %q:\n%s", want, data)
		}
	}
	if got := gitOut(t, dir, "config", "--get", "rerere.enabled"); got != "true" {
		t.Fatalf("rerere.enabled = %q", got)
	}

	code, out, errOut = runIn(t, dir, "feature", "--id", "JIRA-123", "--desc", "Add login")
	if code != 0 {
		t.Fatalf("feature failed: %s %s", out, errOut)
	}
	if got := gitOut(t, dir, "branch", "--show-current"); got != "feature/JIRA-123-add-login" {
		t.Fatalf("branch = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tbd", "state.json")); err != nil {
		t.Fatalf("state missing: %v", err)
	}
}

func TestV2ReleaseCompleteFastForwardsTrunkToConcreteCommit(t *testing.T) {
	dir := newV2Repo(t)
	if code, out, errOut := runIn(t, dir, "init", "--yes"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	if code, out, errOut := runIn(t, dir, "feature", "--id", "JIRA-1", "--desc", "Release me"); code != 0 {
		t.Fatalf("feature failed: %s %s", out, errOut)
	}
	write(t, dir, "work.txt", "work")
	if code, out, errOut := runIn(t, dir, "commit", "--no-edit"); code != 0 {
		t.Fatalf("commit failed: %s %s", out, errOut)
	}
	if code, out, errOut := runIn(t, dir, "release", "rc", "1.0.0"); code != 0 {
		t.Fatalf("rc failed: %s %s", out, errOut)
	}
	if code, out, errOut := runIn(t, dir, "release", "complete", "1.0.0"); code != 0 {
		t.Fatalf("complete failed: %s %s", out, errOut)
	}
	main := gitOut(t, dir, "rev-parse", "main")
	tag := gitOut(t, dir, "rev-parse", "v1.0.0^{commit}")
	if main != tag {
		t.Fatalf("main=%s tag=%s", main, tag)
	}
}

func newV2Repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "base.txt", "base")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "chore: base")
	return dir
}

func runIn(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	var out, errb bytes.Buffer
	code := Run(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
