package commands

import (
	"strings"
	"testing"
)

// init's printed summary must be the actual file, and must not materialize the
// lease list the chosen strategy ignores.
func TestInitOutputMatchesStrategyEphemeral(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "init", "lease-strategy:ephemeral-branch", "lease-branches:deploy-now", ":force")
	if err := runInit(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"lease-strategy: ephemeral-branch", "deploy-now", "lease-tags: []"} {
		if !strings.Contains(s, want) {
			t.Errorf("init output missing %q\n%s", want, s)
		}
	}
	if strings.Contains(s, "dev-deploy") {
		t.Errorf("ephemeral init must not list the default lease-tags\n%s", s)
	}
}

func TestInitOutputMatchesStrategyTag(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "init", ":force")
	if err := runInit(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"lease-strategy: tag", "dev-deploy", "lease-branches: []"} {
		if !strings.Contains(s, want) {
			t.Errorf("init output missing %q\n%s", want, s)
		}
	}
}

func TestInitNoneClearsBothLists(t *testing.T) {
	dir := repoFixture(t)
	ctx, out, _ := newCtx(dir, "init", "lease-strategy:none", ":force")
	if err := runInit(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "lease-tags: []") || !strings.Contains(s, "lease-branches: []") {
		t.Errorf("none should clear both lists\n%s", s)
	}
}
