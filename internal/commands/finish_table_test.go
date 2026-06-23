package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goforge.dev/tbd/internal/cli"
)

func writeConfig(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".tbd.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startFeatureWithCommit creates feature/NAME off develop and lands one commit.
func startFeatureWithCommit(t *testing.T, dir, name, file string) {
	t.Helper()
	ctx, _, _ := newCtx(dir, "feature", "start", name)
	if err := featureStart(ctx); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	writeAndCommit(t, dir, file, name+" work")
}

// TestFeatureFinishGuards is a data-driven matrix over the finish preconditions.
func TestFeatureFinishGuards(t *testing.T) {
	type result struct {
		wantExit1   bool   // expect cli.ExitError{1}
		wantErrSub  string // substring expected in a plain error
		wantNoError bool   // expect success
	}
	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string) []string // returns finish argv tail (flags)
		expect result
	}{
		{
			name: "refuses to finish the trunk branch",
			setup: func(t *testing.T, dir string) []string {
				// stay on develop
				return []string{":no-push"}
			},
			expect: result{wantErrSub: "refusing to finish the trunk"},
		},
		{
			name: "clean feature fast-forwards",
			setup: func(t *testing.T, dir string) []string {
				startFeatureWithCommit(t, dir, "clean", "clean.txt")
				return []string{":no-push"}
			},
			expect: result{wantNoError: true},
		},
		{
			name: "diverged with auto-rebase off is refused",
			setup: func(t *testing.T, dir string) []string {
				writeConfig(t, dir, "trunk-name: develop\nfeature-prefix: feature/\nauto-rebase: false\n")
				startFeatureWithCommit(t, dir, "x", "x.txt")
				gitRun(t, dir, "switch", "-q", "develop")
				writeAndCommit(t, dir, "trunk.txt", "trunk advances")
				gitRun(t, dir, "switch", "-q", "feature/x")
				return []string{":no-push"}
			},
			expect: result{wantExit1: true},
		},
		{
			name: "diverged with auto-rebase on succeeds",
			setup: func(t *testing.T, dir string) []string {
				startFeatureWithCommit(t, dir, "y", "y.txt")
				gitRun(t, dir, "switch", "-q", "develop")
				writeAndCommit(t, dir, "trunk.txt", "trunk advances")
				gitRun(t, dir, "switch", "-q", "feature/y")
				return []string{":no-push"}
			},
			expect: result{wantNoError: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := repoFixture(t)
			tail := tt.setup(t, dir)
			argv := append([]string{"feature", "finish"}, tail...)
			ctx, _, _ := newCtx(dir, argv...)
			err := featureFinish(ctx)

			switch {
			case tt.expect.wantNoError:
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
			case tt.expect.wantExit1:
				var ee cli.ExitError
				if !errors.As(err, &ee) || ee.Code != 1 {
					t.Fatalf("expected ExitError{1}, got %v", err)
				}
			case tt.expect.wantErrSub != "":
				if err == nil || !strings.Contains(err.Error(), tt.expect.wantErrSub) {
					t.Fatalf("expected error containing %q, got %v", tt.expect.wantErrSub, err)
				}
			}
		})
	}
}
