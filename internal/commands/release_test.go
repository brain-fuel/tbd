package commands

import (
	"strings"
	"testing"
)

// Regression for bug 0003: a version that would form an invalid git ref must be
// rejected with a clean tbd error before any ref is created or porcelain leaks.
func TestReleaseCutRejectsRefInvalidVersion(t *testing.T) {
	cases := []struct {
		strategy string
		wantWord string // "branch" or "tag" in the error
	}{
		{"branch", "branch"},
		{"tag", "tag"},
	}
	for _, tc := range cases {
		dir := repoFixture(t)
		ctx, _, _ := newCtx(dir, "release", "cut", "1 0", "strategy:"+tc.strategy, ":local")
		err := runRelease(ctx)
		if err == nil {
			t.Fatalf("strategy %s: expected rejection, got nil", tc.strategy)
		}
		if !strings.Contains(err.Error(), "not valid for a release") ||
			!strings.Contains(err.Error(), tc.wantWord) {
			t.Fatalf("strategy %s: want clean ref-validation error, got %v", tc.strategy, err)
		}
		r, _ := openRepo(dir)
		if r.Exists("release/1 0") || r.Exists("v1 0") {
			t.Fatalf("strategy %s: no ref should have been created", tc.strategy)
		}
	}
}

// A normal version still cuts the release.
func TestReleaseCutValidVersion(t *testing.T) {
	dir := repoFixture(t)
	ctx, _, _ := newCtx(dir, "release", "cut", "1.2.3", "strategy:branch,tag", ":local")
	if err := runRelease(ctx); err != nil {
		t.Fatalf("cut: %v", err)
	}
	r, _ := openRepo(dir)
	if !r.Exists("release/1.2.3") {
		t.Fatal("release branch should exist")
	}
	if !r.Exists("v1.2.3") {
		t.Fatal("release tag should exist")
	}
}
