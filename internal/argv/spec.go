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

// Merge returns the union of two specs (used to fold global options into a
// command's own).
func (s Spec) Merge(o Spec) Spec {
	out := Spec{
		Named: append(append([]Opt{}, s.Named...), o.Named...),
		Flags: append(append([]Opt{}, s.Flags...), o.Flags...),
		Hints: map[string]string{},
	}
	for k, v := range s.Hints {
		out.Hints[k] = v
	}
	for k, v := range o.Hints {
		out.Hints[k] = v
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

// Validate checks every named argument and flag in a against the spec, returning
// a helpful error for the first unknown one (options are checked in sorted order
// so the message is deterministic). prog is the program name, used in the
// "run ... help" line.
func (s Spec) Validate(a Args, prog string) error {
	var badNamed, badFlags []string
	for k := range a.Named {
		if !has(s.Named, k) {
			badNamed = append(badNamed, k)
		}
	}
	for f := range a.Flags {
		if !has(s.Flags, f) {
			badFlags = append(badFlags, f)
		}
	}
	sort.Strings(badNamed)
	sort.Strings(badFlags)

	if len(badNamed) > 0 {
		k := badNamed[0]
		return s.unknown(prog, a.Command, k+":"+a.Named[k], k, true)
	}
	if len(badFlags) > 0 {
		f := badFlags[0]
		return s.unknown(prog, a.Command, ":"+f, f, false)
	}
	return nil
}

func (s Spec) unknown(prog, command, shown, bare string, named bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "unknown option %q", shown)

	if sugg := s.suggest(bare, named); sugg != "" {
		fmt.Fprintf(&b, "\n  did you mean %q?", sugg)
	}
	if hint := s.Hints[bare]; hint != "" {
		fmt.Fprintf(&b, "\n  %s", hint)
	}

	if named0 := names(s.Named); len(named0) > 0 {
		fmt.Fprintf(&b, "\n  named (pass as name:value): %s", strings.Join(named0, ", "))
	}
	if flags0 := names(s.Flags); len(flags0) > 0 {
		fmt.Fprintf(&b, "\n  flags (pass as :name): %s", strings.Join(flags0, ", "))
	}
	if command != "" {
		fmt.Fprintf(&b, "\n  run %q for usage", prog+" help "+command)
	}
	return fmt.Errorf("%s", b.String())
}

// suggest returns the closest accepted option name within edit distance 2,
// preferring the same kind (named vs flag) the user typed.
func (s Spec) suggest(bare string, named bool) string {
	first, second := s.Named, s.Flags
	if !named {
		first, second = s.Flags, s.Named
	}
	if c := closest(bare, names(first)); c != "" {
		return c
	}
	return closest(bare, names(second))
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
