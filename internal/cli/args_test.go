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

func TestNormalizeArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "help short", in: []string{"-h"}, want: nil},
		{name: "help long", in: []string{"--help"}, want: nil},
		{name: "version short", in: []string{"-v"}, want: []string{"version"}},
		{name: "version long", in: []string{"--version"}, want: []string{"version"}},
		{name: "command help", in: []string{"feature", "--help"}, want: []string{"help", "feature"}},
		{name: "unchanged", in: []string{"feature", "start", "login"}, want: []string{"feature", "start", "login"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("normalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}
