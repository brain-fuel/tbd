package argv

import (
	"fmt"
	"sort"
	"strings"
)

// Opt is one accepted option: a named argument ("key:") or a boolean flag
// (":flag"), with optional help text.
type Opt struct {
	Name string
	Help string
}

// Spec declares the options a command accepts. Validate uses it to reject
// anything else. Hints maps an unknown option name to a tailored message (e.g.
// "that lives in config, not on the command line").
type Spec struct {
	Named []Opt
	Flags []Opt
	Hints map[string]string
}

// Opts builds a slice of options from bare names (no help text). Handy for
// declaring a command's accepted named args or flags concisely.
func Opts(names ...string) []Opt {
	out := make([]Opt, len(names))
	for i, n := range names {
		out[i] = Opt{Name: n}
	}
	return out
}

func names(opts []Opt) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Name
	}
	sort.Strings(out)
	return out
}

func has(opts []Opt, name string) bool {
	for _, o := range opts {
		if o.Name == name {
			return true
		}
	}
	return false
}

// Validate checks every named argument and flag in a against the command's spec
// merged with the global spec, returning a helpful error for the first unknown
// one (checked in sorted order so the message is deterministic). The command's
// own options and the global options are reported as separate groups, so a global
// cosmetic toggle never masquerades as one of the command's real options. prog is
// the program name.
func Validate(a Args, cmd, global Spec, prog string) error {
	allowedNamed := func(n string) bool { return has(cmd.Named, n) || has(global.Named, n) }
	allowedFlags := func(f string) bool { return has(cmd.Flags, f) || has(global.Flags, f) }

	var badNamed, badFlags []string
	for k := range a.Named {
		if !allowedNamed(k) {
			badNamed = append(badNamed, k)
		}
	}
	for f := range a.Flags {
		if !allowedFlags(f) {
			badFlags = append(badFlags, f)
		}
	}
	sort.Strings(badNamed)
	sort.Strings(badFlags)

	if len(badNamed) > 0 {
		k := badNamed[0]
		return unknown(prog, a.Command, k+":"+a.Named[k], k, true, cmd, global)
	}
	if len(badFlags) > 0 {
		f := badFlags[0]
		return unknown(prog, a.Command, ":"+f, f, false, cmd, global)
	}
	return nil
}

func unknown(prog, command, shown, bare string, named bool, cmd, global Spec) error {
	var b strings.Builder
	fmt.Fprintf(&b, "unknown option %q", shown)

	if sugg := suggest(bare, named, cmd, global); sugg != "" {
		fmt.Fprintf(&b, "\n  did you mean %q?", sugg)
	}
	if hint := cmd.Hints[bare]; hint != "" {
		fmt.Fprintf(&b, "\n  %s", hint)
	} else if hint := global.Hints[bare]; hint != "" {
		fmt.Fprintf(&b, "\n  %s", hint)
	}

	groupLines(&b, command, cmd)
	groupLines(&b, "global", global)
	fmt.Fprintf(&b, "\n  pass named as name:value and flags as :name; see %q", prog+" help "+command)
	return fmt.Errorf("%s", b.String())
}

// groupLines appends "<label> named:" / "<label> flags:" lines for a spec.
func groupLines(b *strings.Builder, label string, s Spec) {
	if n := names(s.Named); len(n) > 0 {
		fmt.Fprintf(b, "\n  %s named: %s", label, strings.Join(n, ", "))
	}
	if f := names(s.Flags); len(f) > 0 {
		fmt.Fprintf(b, "\n  %s flags: %s", label, strings.Join(f, ", "))
	}
}

// suggest returns the closest accepted option name within edit distance 2,
// preferring the same kind (named vs flag) the user typed.
func suggest(bare string, named bool, cmd, global Spec) string {
	var sameKind, otherKind []string
	if named {
		sameKind = append(names(cmd.Named), names(global.Named)...)
		otherKind = append(names(cmd.Flags), names(global.Flags)...)
	} else {
		sameKind = append(names(cmd.Flags), names(global.Flags)...)
		otherKind = append(names(cmd.Named), names(global.Named)...)
	}
	if c := closest(bare, sameKind); c != "" {
		return c
	}
	return closest(bare, otherKind)
}

func closest(want string, candidates []string) string {
	best, bestD := "", 3 // threshold: only suggest within distance 2
	for _, c := range candidates {
		if d := levenshtein(want, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
