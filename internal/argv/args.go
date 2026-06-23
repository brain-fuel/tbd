// Package argv is a self-contained command-line parser for the poly-style
// argument syntax used across the goforge tools:
//
//	<command> [positional...] key:value :flag
//
// It knows nothing about any particular application's commands. A command
// declares the options it accepts as a Spec; argv then both parses raw argv and
// validates it against that Spec, producing helpful errors for unknown options.
// The Spec is the single source of truth, so adding or removing an option is a
// local edit next to the command, never a codebase-wide audit.
package argv

import "strings"

// Args is a parsed invocation.
//
// Tokens are classified as:
//   - "key:value"  -> a named argument (Named["key"] = "value")
//   - ":flag"      -> a boolean flag (Flags["flag"] = true)
//   - bare word    -> a positional argument
//
// A value may itself contain colons (e.g. to:origin/develop); only the first
// colon separates the key from the value.
type Args struct {
	Command    string
	Positional []string
	Named      map[string]string
	Flags      map[string]bool
}

// Parse splits a raw argument vector (already stripped of the program name) into
// the command and its arguments. It performs no validation.
func Parse(argv []string) Args {
	a := Args{Named: map[string]string{}, Flags: map[string]bool{}}
	if len(argv) == 0 {
		return a
	}
	a.Command = argv[0]
	for _, tok := range argv[1:] {
		switch {
		case strings.HasPrefix(tok, ":"):
			a.Flags[tok[1:]] = true
		case strings.Contains(tok, ":"):
			k, v, _ := strings.Cut(tok, ":")
			a.Named[k] = v
		default:
			a.Positional = append(a.Positional, tok)
		}
	}
	return a
}

// Flag reports whether a boolean flag was set.
func (a Args) Flag(name string) bool { return a.Flags[name] }

// Get returns a named argument and whether it was present.
func (a Args) Get(name string) (string, bool) {
	v, ok := a.Named[name]
	return v, ok
}

// GetOr returns a named argument or a default when absent.
func (a Args) GetOr(name, def string) string {
	if v, ok := a.Named[name]; ok {
		return v
	}
	return def
}

// Pos returns the positional argument at index i, or "" if out of range.
func (a Args) Pos(i int) string {
	if i < 0 || i >= len(a.Positional) {
		return ""
	}
	return a.Positional[i]
}
