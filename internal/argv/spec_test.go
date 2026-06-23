package argv

import (
	"strings"
	"testing"
)

func leaseSpec() Spec {
	return Spec{
		Named: Opts("to", "color-mode"),
		Flags: Opts("no-advance", "force", "local", "no-fetch"),
		Hints: map[string]string{"strategy": "set lease-strategy in .tbd.yaml, not on the command line"},
	}
}

func TestValidateAcceptsKnown(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", "to:HEAD", ":force", "color-mode:none"})
	if err := leaseSpec().Validate(a, "tbd"); err != nil {
		t.Fatalf("known options should validate: %v", err)
	}
}

func TestValidateUnknownNamedWithHint(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", "strategy:random"})
	err := leaseSpec().Validate(a, "tbd")
	if err == nil {
		t.Fatal("expected error for strategy:random")
	}
	msg := err.Error()
	for _, want := range []string{
		`unknown option "strategy:random"`,
		"set lease-strategy in .tbd.yaml", // the hint
		"named (pass as name:value): color-mode, to",
		"flags (pass as :name): force, local, no-advance, no-fetch",
		"tbd help lease",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n%s", want, msg)
		}
	}
}

func TestValidateUnknownFlagSuggests(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", ":forcee"})
	err := leaseSpec().Validate(a, "tbd")
	if err == nil {
		t.Fatal("expected error for :forcee")
	}
	if !strings.Contains(err.Error(), `did you mean "force"?`) {
		t.Fatalf("expected suggestion of force, got:\n%s", err.Error())
	}
}

func TestValidateDeterministic(t *testing.T) {
	// Multiple unknowns: report the lexicographically first for stable output.
	a := Parse([]string{"x", "zzz:1", "aaa:2"})
	err := Spec{}.Validate(a, "tbd")
	if err == nil || !strings.Contains(err.Error(), `"aaa:2"`) {
		t.Fatalf("expected aaa reported first, got %v", err)
	}
}

func TestMergeUnionsOptionsAndHints(t *testing.T) {
	global := Spec{Named: Opts("color-mode"), Flags: Opts("local")}
	cmd := Spec{Flags: Opts("force"), Hints: map[string]string{"k": "v"}}
	m := global.Merge(cmd)
	a := Parse([]string{"c", "color-mode:none", ":local", ":force"})
	if err := m.Validate(a, "tbd"); err != nil {
		t.Fatalf("merged spec should accept all: %v", err)
	}
	if m.Hints["k"] != "v" {
		t.Fatal("merge should carry hints")
	}
}
