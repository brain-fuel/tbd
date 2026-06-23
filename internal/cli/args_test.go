package cli

import "testing"

func TestParse(t *testing.T) {
	a := Parse([]string{"feature", "start", "login", "to:origin/develop", ":force"})
	if a.Command != "feature" {
		t.Fatalf("command = %q", a.Command)
	}
	if a.Pos(0) != "start" || a.Pos(1) != "login" {
		t.Fatalf("positionals = %v", a.Positional)
	}
	if a.GetOr("to", "") != "origin/develop" {
		t.Fatalf("to = %q", a.GetOr("to", ""))
	}
	if !a.Flag("force") {
		t.Fatal("force flag not set")
	}
	if a.Pos(5) != "" {
		t.Fatal("out-of-range positional should be empty")
	}
}

func TestParseEmpty(t *testing.T) {
	a := Parse(nil)
	if a.Command != "" {
		t.Fatalf("expected empty command, got %q", a.Command)
	}
}

func TestParseValueWithColon(t *testing.T) {
	a := Parse([]string{"lease", "take", "dev", "to:refs/heads/x:y"})
	if a.GetOr("to", "") != "refs/heads/x:y" {
		t.Fatalf("value with colon mishandled: %q", a.GetOr("to", ""))
	}
}
