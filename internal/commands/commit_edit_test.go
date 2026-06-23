package commands

import (
	"testing"
)

// fakeEditor makes $EDITOR overwrite the commit-message file with text, so the
// :edit path can be tested non-interactively. git runs the editor as
// `sh -c "<GIT_EDITOR> <file>"`, so a trailing > redirects into the file.
func fakeEditor(t *testing.T, text string) {
	t.Helper()
	t.Setenv("GIT_EDITOR", "printf '"+text+"\\n' >")
}

func TestCommitEditReword(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:original")); err != nil {
		t.Fatal(err)
	}

	fakeEditor(t, "reworded via editor")
	if err := runCommit(mustCtx(dir, "commit", ":edit")); err != nil {
		t.Fatalf("commit :edit: %v", err)
	}
	r, _ := openRepo(dir)
	if subj := r.Subject("feature/p"); subj != "reworded via editor" {
		t.Fatalf("subject = %q, want reworded via editor", subj)
	}
	if n := commitCount(t, dir, "feature/p"); n != 1 {
		t.Fatalf("expected 1 commit, got %d", n)
	}
}

func TestCommitEditFirstCommit(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")

	fakeEditor(t, "written in editor")
	if err := runCommit(mustCtx(dir, "commit", ":edit")); err != nil {
		t.Fatalf("first commit :edit: %v", err)
	}
	r, _ := openRepo(dir)
	if subj := r.Subject("feature/p"); subj != "written in editor" {
		t.Fatalf("subject = %q", subj)
	}
	if n := commitCount(t, dir, "feature/p"); n != 1 {
		t.Fatalf("expected 1 commit, got %d", n)
	}
}

func TestCommitEditReplacesWithoutFileChange(t *testing.T) {
	// Rewording with no new file changes must still work (amend reword-only).
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:first")); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t, dir, "feature/p")

	fakeEditor(t, "reworded only")
	if err := runCommit(mustCtx(dir, "commit", ":edit")); err != nil {
		t.Fatalf("reword-only :edit: %v", err)
	}
	r, _ := openRepo(dir)
	if subj := r.Subject("feature/p"); subj != "reworded only" {
		t.Fatalf("subject = %q", subj)
	}
	if after := commitCount(t, dir, "feature/p"); after != before {
		t.Fatalf("commit count changed %d -> %d", before, after)
	}
}
