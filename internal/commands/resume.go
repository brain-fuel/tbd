package commands

import (
	"os"
	"path/filepath"
	"strings"
)

// resumeFile is tbd's per-repo record of an operation that was interrupted by a
// rebase conflict. When a higher-level command (currently feature finish) hits a
// conflict, it stores the branch being finished plus the argv that launched it
// here; once tbd continue finishes the rebase it replays that argv so the
// operation actually completes (trunk fast-forwarded, branch deleted) instead of
// being silently dropped.
//
// The record is bound to its branch: tbd continue replays it only while that
// same branch is checked out, so a record orphaned by a raw "git rebase --abort"
// can never hijack an unrelated continue on a different branch.
//
// File format: first line is "branch <name>", remaining lines are the argv.
const resumeFile = "tbd-resume"

// resumePath returns the path to the resume record inside the .git directory.
func (e env) resumePath() (string, error) {
	dir, err := e.repo.GitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, resumeFile), nil
}

// writeResume records the branch and the argv to replay after the in-progress
// rebase of that branch finishes.
func (e env) writeResume(branch string, argv []string) error {
	path, err := e.resumePath()
	if err != nil {
		return err
	}
	lines := append([]string{"branch " + branch}, argv...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// readResume returns the recorded branch and argv, or ok=false if none pending
// or the record is malformed.
func (e env) readResume() (branch string, argv []string, ok bool) {
	path, err := e.resumePath()
	if err != nil {
		return "", nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "", nil, false
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "branch ") {
		return "", nil, false
	}
	return strings.TrimPrefix(lines[0], "branch "), lines[1:], true
}

// clearResume removes any pending resume record (no error if absent).
func (e env) clearResume() {
	if path, err := e.resumePath(); err == nil {
		_ = os.Remove(path)
	}
}
