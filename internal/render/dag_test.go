package render

import (
	"strings"
	"testing"
)

func TestRebasePlanRenderPlain(t *testing.T) {
	p := RebasePlan{
		Feature:   "feature/login",
		Trunk:     "develop",
		Fork:      Commit{Short: "0d4c", Subject: "shared base"},
		TrunkLine: []Commit{{Short: "a1b2", Subject: "release notes"}},
		FeatLine: []Commit{
			{Short: "f3a9", Subject: "add login form"},
			{Short: "9c2e", Subject: "scaffold auth"},
		},
	}
	out := p.Render(NewColors("none", false))

	// Structure: a "before" then an "after" section.
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Fatalf("missing before/after sections:\n%s", out)
	}
	// Fork point labeled in before.
	if !strings.Contains(out, "fork point") {
		t.Fatalf("missing fork point label:\n%s", out)
	}
	// Feature replayed label in after.
	if !strings.Contains(out, "replayed") {
		t.Fatalf("missing replayed label:\n%s", out)
	}
	// Both feature commits present.
	if !strings.Contains(out, "add login form") || !strings.Contains(out, "scaffold auth") {
		t.Fatalf("missing feature commits:\n%s", out)
	}
	// Trunk head label.
	if !strings.Contains(out, "trunk head: develop") {
		t.Fatalf("missing trunk head label:\n%s", out)
	}
	// Plain mode must not contain ANSI escapes.
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain render leaked ANSI codes:\n%q", out)
	}
}

func TestRebasePlanRenderColor(t *testing.T) {
	p := RebasePlan{Feature: "f", Trunk: "develop", Fork: Commit{Short: "0d4c"}}
	out := p.Render(NewColors("always", false))
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("color render should contain ANSI codes")
	}
}
