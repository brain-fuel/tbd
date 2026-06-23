package commands

import (
	"strings"
	"testing"
)

func TestLearnFullCoversFeatureSet(t *testing.T) {
	ctx, out, _ := newCtx(t.TempDir(), "learn")
	if err := runLearn(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	// Every command in the tool should appear in the full walkthrough.
	for _, want := range []string{
		"tbd init", "tbd config list", "tbd feature start", "tbd commit",
		"tbd feature push", "tbd continue", "tbd lease deploy-now",
		"tbd lease status", "tbd release cut", "tbd feature finish",
		"tbd guard", "tbd status",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("walkthrough missing command %q", want)
		}
	}
	// The two-developer lease scenario.
	for _, want := range []string{
		"feature/payments", "feature/search", "Dana",
		"initializing", "advancing", "taking", "compare-and-swap",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lease scenario missing %q", want)
		}
	}
}

func TestLearnChapterAndTopics(t *testing.T) {
	ctx, out, _ := newCtx(t.TempDir(), "learn", "lease")
	if err := runLearn(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "deploy-now") {
		t.Fatal("lease chapter should mention deploy-now")
	}
	if strings.Contains(s, "tbd init trunk:develop") {
		t.Fatal("lease chapter should not include the setup chapter")
	}

	ctx2, out2, _ := newCtx(t.TempDir(), "learn", "topics")
	if err := runLearn(ctx2); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"setup", "feature", "lease", "finish"} {
		if !strings.Contains(out2.String(), key) {
			t.Errorf("topics missing %q", key)
		}
	}
}

func TestLearnUnknownChapter(t *testing.T) {
	ctx, _, _ := newCtx(t.TempDir(), "learn", "nope")
	if err := runLearn(ctx); err == nil {
		t.Fatal("expected error for unknown chapter")
	}
}
