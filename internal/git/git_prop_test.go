package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// tb is the minimal subset of the testing API shared by *testing.T and
// *rapid.T, so history-building helpers work under both.
type tb interface {
	Helper()
	Fatalf(string, ...any)
}

// buildHistory creates a fresh repo with `develop` plus a `feature/x` branch
// forked from a common base, then lands numTrunk commits on develop and numFeat
// commits on the feature. Commit files are uniquely named so a later rebase
// never conflicts. Returns the repo.
func buildHistory(t tb, dir string, numTrunk, numFeat int) *Repo {
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
			t.Fatalf("write %s: %v", name, err)
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
		write("t" + itoa(i) + ".txt")
		run("add", "-A")
		run("commit", "-q", "-m", "trunk "+itoa(i))
	}
	run("switch", "-q", "feature/x")
	for i := 0; i < numFeat; i++ {
		write("f" + itoa(i) + ".txt")
		run("add", "-A")
		run("commit", "-q", "-m", "feat "+itoa(i))
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return r
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestAheadBehindModel: for a fork with numTrunk trunk-only commits and numFeat
// feature-only commits, AheadBehind(develop, feature) must report exactly
// (numTrunk, numFeat), and the log ranges must match those counts.
func TestAheadBehindModel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	rapid.Check(t, func(rt *rapid.T) {
		numTrunk := rapid.IntRange(0, 4).Draw(rt, "numTrunk")
		numFeat := rapid.IntRange(0, 4).Draw(rt, "numFeat")

		dir, err := os.MkdirTemp("", "tbd-prop-*")
		if err != nil {
			rt.Fatalf("tempdir: %v", err)
		}
		defer os.RemoveAll(dir)

		r := buildHistory(rt, dir, numTrunk, numFeat)

		ahead, behind, err := r.AheadBehind("develop", "feature/x")
		if err != nil {
			rt.Fatalf("ahead/behind: %v", err)
		}
		if ahead != numTrunk {
			rt.Fatalf("trunk-only = %d, want %d", ahead, numTrunk)
		}
		if behind != numFeat {
			rt.Fatalf("feature-only = %d, want %d", behind, numFeat)
		}

		trunkOnly, _ := r.LogRange("feature/x..develop")
		featOnly, _ := r.LogRange("develop..feature/x")
		if len(trunkOnly) != numTrunk {
			rt.Fatalf("log trunk-only = %d, want %d", len(trunkOnly), numTrunk)
		}
		if len(featOnly) != numFeat {
			rt.Fatalf("log feature-only = %d, want %d", len(featOnly), numFeat)
		}

		// IsAncestor of trunk head into feature holds exactly when trunk has not
		// advanced past the fork.
		trunkHead, _ := r.RevParse("develop")
		featHead, _ := r.RevParse("feature/x")
		gotAncestor := r.IsAncestor(trunkHead, featHead)
		wantAncestor := numTrunk == 0
		if gotAncestor != wantAncestor {
			rt.Fatalf("IsAncestor = %v, want %v (numTrunk=%d)", gotAncestor, wantAncestor, numTrunk)
		}
	})
}
