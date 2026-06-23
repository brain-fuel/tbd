package argv

import (
	"strings"
	"testing"
)

// command-specific spec (no globals mingled in)
func leaseSpec() Spec {
	return Spec{
		Named: Opts("to"),
		Flags: Opts("no-advance", "force"),
		Hints: map[string]string{"strategy": "set lease-strategy in .tbd.yaml, not on the command line"},
	}
}

func globalSpec() Spec {
	return Spec{
		Named: Opts("color-mode"),
		Flags: Opts("local", "no-fetch"),
	}
}

func TestValidateAcceptsKnownAndGlobal(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", "to:HEAD", ":force", "color-mode:none", ":local"})
	if err := Validate(a, leaseSpec(), globalSpec(), "tbd"); err != nil {
		t.Fatalf("known + global options should validate: %v", err)
	}
}

func TestValidateUnknownNamedWithHintAndGroups(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", "strategy:random"})
	err := Validate(a, leaseSpec(), globalSpec(), "tbd")
	if err == nil {
		t.Fatal("expected error for strategy:random")
	}
	msg := err.Error()
	for _, want := range []string{
		`unknown option "strategy:random"`,
		"set lease-strategy in .tbd.yaml", // the hint
		"lease named: to",                 // command's own options, grouped
		"lease flags: force, no-advance",
		"global named: color-mode", // globals clearly separated
		"global flags: local, no-fetch",
		"tbd help lease",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n%s", want, msg)
		}
	}
	// color-mode must NOT appear among the command's own options.
	if strings.Contains(msg, "lease named: color-mode") || strings.Contains(msg, "lease named: to, color-mode") {
		t.Errorf("global color-mode leaked into command options:\n%s", msg)
	}
}

func TestValidateUnknownFlagSuggests(t *testing.T) {
	a := Parse([]string{"lease", "dev-deploy", ":forcee"})
	err := Validate(a, leaseSpec(), globalSpec(), "tbd")
	if err == nil || !strings.Contains(err.Error(), `did you mean "force"?`) {
		t.Fatalf("expected suggestion of force, got: %v", err)
	}
}

func TestValidateDeterministic(t *testing.T) {
	a := Parse([]string{"x", "zzz:1", "aaa:2"})
	err := Validate(a, Spec{}, Spec{}, "tbd")
	if err == nil || !strings.Contains(err.Error(), `"aaa:2"`) {
		t.Fatalf("expected aaa reported first, got %v", err)
	}
}

func TestGlobalHintResolves(t *testing.T) {
	g := Spec{Hints: map[string]string{"verbose": "tbd has no verbose flag"}}
	a := Parse([]string{"x", "verbose:1"})
	err := Validate(a, Spec{}, g, "tbd")
	if err == nil || !strings.Contains(err.Error(), "tbd has no verbose flag") {
		t.Fatalf("expected global hint, got %v", err)
	}
}
