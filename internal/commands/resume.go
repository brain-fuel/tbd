package commands

import (
	"os"
	"path/filepath"
	"strings"
)

// resumeFile is tbd's per-repo record of an operation that was interrupted by a
// rebase conflict. When a higher-level command (currently feature finish) hits a
// conflict, it stores the argv that launched it here; once tbd continue finishes
// the rebase it replays that argv so the operation actually completes (trunk
// fast-forwarded, branch deleted) instead of being silently dropped.
const resumeFile = "tbd-resume"

// resumePath returns the path to the resume record inside the .git directory.
func (e env) resumePath() (string, error) {
	dir, err := e.repo.GitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, resumeFile), nil
}

// writeResume records the argv to replay after the in-progress rebase finishes.
func (e env) writeResume(argv []string) error {
	path, err := e.resumePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(argv, "\n")), 0o644)
}

// readResume returns the recorded argv, or (nil, false) if none is pending.
func (e env) readResume() ([]string, bool) {
	path, err := e.resumePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), true
}

// clearResume removes any pending resume record (no error if absent).
func (e env) clearResume() {
	if path, err := e.resumePath(); err == nil {
		_ = os.Remove(path)
	}
}
